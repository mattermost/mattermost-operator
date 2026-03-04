// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package v1beta1

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	k8sjson "k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/client-go/kubernetes/scheme"
)

// forbiddenDeploymentPatchPrefixes defines path prefixes that are not allowed
// in JSON patches applied to Deployments. These paths can be used to escalate
// privileges or break security boundaries.
var forbiddenDeploymentPatchPrefixes = []string{
	"/spec/template/spec/hostNetwork",
	"/spec/template/spec/hostPID",
	"/spec/template/spec/hostIPC",
	"/spec/template/spec/volumes",
	"/spec/template/spec/serviceAccountName",
	"/spec/template/spec/serviceAccount",
	"/spec/template/spec/nodeSelector",
	"/spec/template/spec/nodeName",
	"/spec/template/spec/initContainers",
}

// forbiddenDeploymentPatchContains defines path segments that are forbidden
// anywhere in a Deployment patch path. This catches per-container security
// settings regardless of the container index.
var forbiddenDeploymentPatchContains = []string{
	"/securityContext/privileged",
	"/securityContext/allowPrivilegeEscalation",
	"/securityContext/runAsUser",
	"/securityContext/capabilities",
}

// patchOperation represents a single JSON Patch operation for validation purposes.
type patchOperation struct {
	Op   string `json:"op"`
	Path string `json:"path"`
}

// validateDeploymentPatch checks that no patch operations target forbidden paths.
func validateDeploymentPatch(rawPatch string) error {
	var ops []patchOperation
	if err := json.Unmarshal([]byte(rawPatch), &ops); err != nil {
		return errors.Wrap(err, "failed to decode patch operations for validation")
	}

	for _, op := range ops {
		for _, prefix := range forbiddenDeploymentPatchPrefixes {
			if op.Path == prefix || strings.HasPrefix(op.Path, prefix+"/") {
				return fmt.Errorf("patch operation %q on forbidden path %q is not allowed", op.Op, op.Path)
			}
		}
		for _, segment := range forbiddenDeploymentPatchContains {
			if strings.Contains(op.Path, segment) {
				return fmt.Errorf("patch operation %q on forbidden path %q is not allowed", op.Op, op.Path)
			}
		}
	}
	return nil
}

var decoder runtime.Decoder
var encoder runtime.Encoder

func (rp *ResourcePatch) IsEmpty() bool {
	return rp == nil || (rp.Service == nil && rp.Deployment == nil)
}

// ApplyToDeployment applies patch and returns resulting deployment.
func (rp *ResourcePatch) ApplyToDeployment(deployment *appsv1.Deployment) (*appsv1.Deployment, bool, error) {
	if rp == nil || rp.Deployment == nil || rp.Deployment.Disable || rp.Deployment.Patch == "" {
		return deployment, false, nil
	}

	if err := validateDeploymentPatch(rp.Deployment.Patch); err != nil {
		return nil, false, errors.Wrap(err, "deployment patch validation failed")
	}

	patched := appsv1.Deployment{}
	gvk := patched.GroupVersionKind()

	err := rp.Deployment.applyPatch(deployment, &patched, &gvk)
	if err != nil {
		return nil, false, errors.Wrap(err, "failed to apply patch to deployment")
	}

	return &patched, true, nil
}

// SetDeploymentPatchStatus sets status of deployment patch.
func (s *MattermostStatus) SetDeploymentPatchStatus(applied bool, err error) {
	if s.ResourcePatch == nil {
		s.ResourcePatch = &ResourcePatchStatus{}
	}
	if s.ResourcePatch.DeploymentPatch == nil {
		s.ResourcePatch.DeploymentPatch = &PatchStatus{}
	}
	s.ResourcePatch.DeploymentPatch.set(applied, err)
}

func (s *MattermostStatus) ClearDeploymentPatchStatus() {
	if s.ResourcePatch == nil {
		return
	}
	s.ResourcePatch.DeploymentPatch = nil
}

// ApplyToService applies patch and returns resulting service.
func (rp *ResourcePatch) ApplyToService(service *v1.Service) (*v1.Service, bool, error) {
	if rp == nil || rp.Service == nil || rp.Service.Disable || rp.Service.Patch == "" {
		return service, false, nil
	}

	patched := v1.Service{}
	gvk := patched.GroupVersionKind()

	err := rp.Service.applyPatch(service, &patched, &gvk)
	if err != nil {
		return nil, false, errors.Wrap(err, "failed to apply patch to service")
	}

	return &patched, true, nil
}

// SetServicePatchStatus sets status of service patch.
func (s *MattermostStatus) SetServicePatchStatus(applied bool, err error) {
	if s.ResourcePatch == nil {
		s.ResourcePatch = &ResourcePatchStatus{}
	}
	if s.ResourcePatch.ServicePatch == nil {
		s.ResourcePatch.ServicePatch = &PatchStatus{}
	}
	s.ResourcePatch.ServicePatch.set(applied, err)
}

func (s *MattermostStatus) ClearServicePatchStatus() {
	if s.ResourcePatch == nil {
		return
	}
	s.ResourcePatch.ServicePatch = nil
}

func (p Patch) applyPatch(resource, destination runtime.Object, gvk *schema.GroupVersionKind) error {
	if p.Disable || len(p.Patch) == 0 {
		destination = resource //nolint:staticcheck,ineffassign
		return nil
	}

	return p.applyToResource(resource, destination, gvk)
}

func (ps *PatchStatus) set(applied bool, err error) {
	ps.Applied = applied
	ps.Error = ""
	if err != nil {
		ps.Error = err.Error()
	}
}

func (p Patch) applyToResource(
	resource runtime.Object,
	destination runtime.Object,
	gvk *schema.GroupVersionKind) error {
	enc, err := lazyEncoder()
	if err != nil {
		return errors.Wrap(err, "failed to initialize encoder")
	}

	marshalledBuff := &bytes.Buffer{}
	err = enc.Encode(resource, marshalledBuff)
	if err != nil {
		return errors.Wrap(err, "failed to encode object")
	}

	encodedResource := marshalledBuff.Bytes()

	encodedResource, err = p.applyPatches(encodedResource)
	if err != nil {
		return errors.Wrap(err, "failed to apply JSON patches")
	}

	dec, err := lazyDecoder()
	if err != nil {
		return errors.Wrap(err, "failed to initialize decoder")
	}

	_, _, err = dec.Decode(encodedResource, gvk, destination)
	if err != nil {
		return errors.Wrap(err, "failed to decode patched object")
	}

	return nil
}

func (p Patch) applyPatches(encodedResource []byte) ([]byte, error) {
	jsonPatch, err := jsonpatch.DecodePatch([]byte(p.Patch))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to encode patch: %s", p.Patch)
	}

	encodedResource, err = jsonPatch.Apply(encodedResource)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to apply patch: %s", p.Patch)
	}
	return encodedResource, nil
}

func lazyDecoder() (runtime.Decoder, error) {
	if decoder != nil {
		return decoder, nil
	}
	return defaultDecoder()
}

func lazyEncoder() (runtime.Encoder, error) {
	if encoder != nil {
		return encoder, nil
	}
	return defaultEncoder()
}

func defaultScheme() (*runtime.Scheme, error) {
	resourcesSchema := runtime.NewScheme()

	var addToSchemes = []func(*runtime.Scheme) error{
		scheme.AddToScheme,
	}

	for _, f := range addToSchemes {
		err := f(resourcesSchema)
		if err != nil {
			return nil, errors.Wrap(err, "failed to add types to schema")
		}
	}

	return resourcesSchema, nil
}

func defaultDecoder() (runtime.Decoder, error) {
	resourceScheme, err := defaultScheme()
	if err != nil {
		return nil, err
	}
	codecs := serializer.NewCodecFactory(resourceScheme)
	decoder := codecs.UniversalDeserializer()

	return decoder, nil
}

func defaultEncoder() (runtime.Encoder, error) {
	resourceScheme, err := defaultScheme()
	if err != nil {
		return nil, err
	}

	jsonSerializer := k8sjson.NewSerializerWithOptions(k8sjson.DefaultMetaFactory, resourceScheme, resourceScheme, k8sjson.SerializerOptions{})

	return jsonSerializer, nil
}
