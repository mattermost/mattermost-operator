// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package v1beta1

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TODO: tests can be extended to common patches and requests.

func TestResourcePatch_ApplyToDeployment(t *testing.T) {
	for _, testCase := range []struct {
		description string
		initial     *appsv1.Deployment
		patch       Patch
		expected    *appsv1.Deployment
	}{
		{
			description: "should apply series of patches",
			initial: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"oldKey": "some-val",
						"key1":   "initialVal",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(1),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"podKey1": "podInitialVal",
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "my-container-1",
									Image: "my-image-1",
									Args:  []string{"arg1"},
								},
								{
									Name:  "my-container-2",
									Image: "my-image-2",
								},
							},
							ServiceAccountName: "initialSA",
						},
					},
				},
			},
			patch: Patch{
				Patch: loadFile(t, "testdata/deploy_patch.json"),
			},
			expected: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"key1":     "initialVal",
						"addedKey": "newVal",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(1),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"podKey1": "podModifiedVal",
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "modified-container-1",
									Image: "modified-image-1",
									Args:  []string{},
								},
								{
									Name:  "my-container-2",
									Image: "my-image-2",
								},
							},
							ServiceAccountName: "initialSA",
						},
					},
				},
			},
		},
		{
			description: "patch add port",
			initial: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(1),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "my-container-1",
									Image: "my-image-1",
									Ports: []corev1.ContainerPort{
										{Name: "http", ContainerPort: 80},
									},
								},
								{
									Name:  "my-container-2",
									Image: "my-image-2",
									Ports: []corev1.ContainerPort{
										{Name: "http", ContainerPort: 80},
									},
								},
							},
						},
					},
				},
			},
			patch: Patch{
				Disable: false,
				Patch:   loadFile(t, "testdata/port_patch.json"),
			},
			expected: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(1),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "my-container-1",
									Image: "my-image-1",
									Ports: []corev1.ContainerPort{
										{Name: "http", ContainerPort: 80},
										{Name: "calls", ContainerPort: 8443, Protocol: corev1.ProtocolUDP},
									},
								},
								{
									Name:  "my-container-2",
									Image: "my-image-2",
									Ports: []corev1.ContainerPort{
										{Name: "http", ContainerPort: 80},
									},
								},
							},
						},
					},
				},
			},
		},
	} {
		t.Run(testCase.description, func(t *testing.T) {
			resPatch := ResourcePatch{
				Deployment: &testCase.patch,
			}

			patched, applied, err := resPatch.ApplyToDeployment(testCase.initial)
			require.NoError(t, err)
			assert.True(t, applied)
			assert.Equal(t, testCase.expected, patched)
		})
	}

	deploy := &appsv1.Deployment{}

	t.Run("empty patch", func(t *testing.T) {
		resPatch := ResourcePatch{
			Deployment: &Patch{},
		}
		_, applied, err := resPatch.ApplyToDeployment(deploy)
		require.NoError(t, err)
		assert.False(t, applied)
	})

	t.Run("invalid patch", func(t *testing.T) {
		patch := `[{"op":"add","path":"/spec/template/spec/spec/serviceAccountName","value":"newSA"}]`

		resPatch := ResourcePatch{
			Deployment: &Patch{Patch: patch},
		}

		_, applied, err := resPatch.ApplyToDeployment(deploy)
		require.Error(t, err)
		assert.False(t, applied)
	})

	t.Run("invalid patch format", func(t *testing.T) {
		patch := `{"patch":"invalid"}`

		resPatch := ResourcePatch{
			Deployment: &Patch{Patch: patch},
		}

		_, applied, err := resPatch.ApplyToDeployment(deploy)
		require.Error(t, err)
		assert.False(t, applied)
	})
}

func TestResourcePatch_ApplyToService(t *testing.T) {
	for _, testCase := range []struct {
		description string
		initial     *corev1.Service
		patch       Patch
		expected    *corev1.Service
	}{
		{
			description: "",
			initial: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"key": "val",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name: "http",
							Port: 80,
						},
						{
							Name: "https",
							Port: 443,
						},
					},
				},
			},
			patch: Patch{
				Patch: loadFile(t, "testdata/service_patch.json"),
			},
			expected: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"newKey": "newVal",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name: "http",
							Port: 80,
						},
						{
							Name: "metrics",
							Port: 9000,
						},
					},
				},
			},
		},
	} {
		t.Run(testCase.description, func(t *testing.T) {
			resPatch := ResourcePatch{
				Service: &testCase.patch,
			}

			patched, applied, err := resPatch.ApplyToService(testCase.initial)
			require.NoError(t, err)
			assert.True(t, applied)
			assert.Equal(t, testCase.expected, patched)
		})
	}

	service := &corev1.Service{}

	t.Run("empty patch", func(t *testing.T) {
		resPatch := ResourcePatch{
			Service: &Patch{},
		}
		_, applied, err := resPatch.ApplyToService(service)
		require.NoError(t, err)
		assert.False(t, applied)
	})

	t.Run("invalid patch", func(t *testing.T) {
		patch := `[{"op":"add","path":"/spec/ports/ports", "value": {"name": "metrics", "port": 9000}}]`

		resPatch := ResourcePatch{
			Service: &Patch{Patch: patch},
		}

		_, applied, err := resPatch.ApplyToService(service)
		require.Error(t, err)
		assert.False(t, applied)
	})

	t.Run("invalid patch format", func(t *testing.T) {
		patch := `{"patch":"invalid"}`

		resPatch := ResourcePatch{
			Service: &Patch{Patch: patch},
		}

		_, applied, err := resPatch.ApplyToService(service)
		require.Error(t, err)
		assert.False(t, applied)
	})
}

func Test_SetPatchStatus(t *testing.T) {
	t.Run("set deployment patch status", func(t *testing.T) {
		mmStatus := &MattermostStatus{}

		mmStatus.SetDeploymentPatchStatus(false, fmt.Errorf("error"))
		assert.Equal(t, "error", mmStatus.ResourcePatch.DeploymentPatch.Error)
		assert.Equal(t, false, mmStatus.ResourcePatch.DeploymentPatch.Applied)

		mmStatus.SetDeploymentPatchStatus(true, nil)
		assert.Equal(t, "", mmStatus.ResourcePatch.DeploymentPatch.Error)
		assert.Equal(t, true, mmStatus.ResourcePatch.DeploymentPatch.Applied)

		mmStatus.ClearDeploymentPatchStatus()
		assert.Nil(t, mmStatus.ResourcePatch.DeploymentPatch)
	})

	t.Run("set service patch status", func(t *testing.T) {
		mmStatus := &MattermostStatus{}

		mmStatus.SetServicePatchStatus(false, fmt.Errorf("error"))
		assert.Equal(t, "error", mmStatus.ResourcePatch.ServicePatch.Error)
		assert.Equal(t, false, mmStatus.ResourcePatch.ServicePatch.Applied)

		mmStatus.SetServicePatchStatus(true, nil)
		assert.Equal(t, "", mmStatus.ResourcePatch.ServicePatch.Error)
		assert.Equal(t, true, mmStatus.ResourcePatch.ServicePatch.Applied)

		mmStatus.ClearServicePatchStatus()
		assert.Nil(t, mmStatus.ResourcePatch.ServicePatch)
	})
}

func TestPatch(t *testing.T) {
	t.Run("replace type=NodePort", func(t *testing.T) {
		p := Patch{
			Disable: false,
			Patch:   `[{"op":"replace","path":"/spec/type", "value": "NodePort"}]`,
		}

		obj1 := &corev1.Service{}
		obj2 := &corev1.Service{}
		gvk := obj1.GroupVersionKind()
		err := p.applyPatch(obj1, obj2, &gvk)
		assert.NoError(t, err)
		assert.NotEqual(t, &obj1, &obj2)
		assert.NotEqual(t, obj1.Spec.Type, obj2.Spec.Type)
	})

	t.Run("empty patch, replace pointer", func(t *testing.T) {
		p := Patch{
			Disable: true,
		}

		obj1 := &corev1.Service{}
		obj2 := &corev1.Service{}
		gvk := obj1.GroupVersionKind()
		err := p.applyPatch(obj1, obj2, &gvk)
		assert.NoError(t, err)
		assert.Equal(t, obj1.Spec.Type, obj2.Spec.Type)
		assert.Equal(t, &obj1, &obj2)
	})
}

func TestValidateDeploymentPatch_ForbiddenPaths(t *testing.T) {
	deploy := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app", Image: "img"},
					},
				},
			},
		},
	}

	forbiddenPatches := []struct {
		description string
		patch       string
	}{
		{
			description: "hostNetwork",
			patch:       `[{"op":"add","path":"/spec/template/spec/hostNetwork","value":true}]`,
		},
		{
			description: "hostPID",
			patch:       `[{"op":"add","path":"/spec/template/spec/hostPID","value":true}]`,
		},
		{
			description: "hostIPC",
			patch:       `[{"op":"add","path":"/spec/template/spec/hostIPC","value":true}]`,
		},
		{
			description: "privileged securityContext",
			patch:       `[{"op":"add","path":"/spec/template/spec/containers/0/securityContext/privileged","value":true}]`,
		},
		{
			description: "allowPrivilegeEscalation",
			patch:       `[{"op":"add","path":"/spec/template/spec/containers/0/securityContext/allowPrivilegeEscalation","value":true}]`,
		},
		{
			description: "capabilities",
			patch:       `[{"op":"add","path":"/spec/template/spec/containers/0/securityContext/capabilities","value":{"add":["SYS_ADMIN"]}}]`,
		},
		{
			description: "volumes",
			patch:       `[{"op":"add","path":"/spec/template/spec/volumes","value":[{"name":"host","hostPath":{"path":"/"}}]}]`,
		},
		{
			description: "volumes subpath",
			patch:       `[{"op":"add","path":"/spec/template/spec/volumes/0","value":{"name":"host","hostPath":{"path":"/"}}}]`,
		},
		{
			description: "serviceAccountName",
			patch:       `[{"op":"replace","path":"/spec/template/spec/serviceAccountName","value":"cluster-admin"}]`,
		},
		{
			description: "serviceAccount (legacy)",
			patch:       `[{"op":"replace","path":"/spec/template/spec/serviceAccount","value":"cluster-admin"}]`,
		},
		{
			description: "nodeSelector",
			patch:       `[{"op":"add","path":"/spec/template/spec/nodeSelector","value":{"target":"master"}}]`,
		},
		{
			description: "nodeName",
			patch:       `[{"op":"add","path":"/spec/template/spec/nodeName","value":"master-0"}]`,
		},
		{
			description: "initContainers",
			patch:       `[{"op":"add","path":"/spec/template/spec/initContainers","value":[{"name":"evil","image":"evil"}]}]`,
		},
		{
			description: "runAsUser in securityContext",
			patch:       `[{"op":"add","path":"/spec/template/spec/containers/0/securityContext/runAsUser","value":0}]`,
		},
	}

	for _, tc := range forbiddenPatches {
		t.Run("should block "+tc.description, func(t *testing.T) {
			resPatch := ResourcePatch{
				Deployment: &Patch{Patch: tc.patch},
			}
			_, applied, err := resPatch.ApplyToDeployment(deploy)
			require.Error(t, err)
			assert.False(t, applied)
			assert.Contains(t, err.Error(), "forbidden path")
		})
	}

	deployWithLabels := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{"existing": "val"},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app", Image: "img"},
					},
				},
			},
		},
	}

	allowedPatches := []struct {
		description string
		patch       string
		deploy      *appsv1.Deployment
	}{
		{
			description: "labels",
			patch:       `[{"op":"add","path":"/metadata/labels/foo","value":"bar"}]`,
			deploy:      deployWithLabels,
		},
		{
			description: "annotations",
			patch:       `[{"op":"add","path":"/metadata/annotations","value":{"key":"val"}}]`,
			deploy:      deploy,
		},
		{
			description: "container image",
			patch:       `[{"op":"replace","path":"/spec/template/spec/containers/0/image","value":"new-img"}]`,
			deploy:      deploy,
		},
		{
			description: "container env",
			patch:       `[{"op":"add","path":"/spec/template/spec/containers/0/env","value":[{"name":"FOO","value":"bar"}]}]`,
			deploy:      deploy,
		},
		{
			description: "replicas",
			patch:       `[{"op":"replace","path":"/spec/replicas","value":3}]`,
			deploy:      deploy,
		},
	}

	for _, tc := range allowedPatches {
		t.Run("should allow "+tc.description, func(t *testing.T) {
			resPatch := ResourcePatch{
				Deployment: &Patch{Patch: tc.patch},
			}
			_, applied, err := resPatch.ApplyToDeployment(tc.deploy)
			require.NoError(t, err)
			assert.True(t, applied)
		})
	}
}

func loadFile(t *testing.T, path string) string {
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}

func int32Ptr(i int32) *int32 {
	return &i
}
