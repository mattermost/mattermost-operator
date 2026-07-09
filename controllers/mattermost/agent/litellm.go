// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package agent

// LiteLLM resources (Deployment, Service, ConfigMap) are shared across all
// Agents in a namespace that use spec.llmGateway.operatorManaged. Because
// they're shared, they cannot be owned by any single Agent CR; cleanup is
// handled by a finalizer (see controller.go) that tears them down when the
// last operatorManaged Agent in the namespace is deleted. The master-key
// Secret is retained alongside the user-managed DB credentials Secret.

import (
	"context"
	"fmt"

	objectMatcher "github.com/banzaicloud/k8s-objectmatcher/patch"
	"github.com/go-logr/logr"
	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	mattermostApp "github.com/mattermost/mattermost-operator/pkg/mattermost"
	"github.com/mattermost/mattermost-operator/pkg/resources"
	pkgerrors "github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var litellmAnnotator = objectMatcher.NewAnnotator(resources.LastAppliedConfig)

// ensureUnownedResource creates or updates a shared (non-owned) resource.
func (r *AgentReconciler) ensureUnownedResource(ctx context.Context, desired, current client.Object, reqLogger logr.Logger) error {
	err := r.Client.Get(ctx, types.NamespacedName{Name: desired.GetName(), Namespace: desired.GetNamespace()}, current)
	if err != nil && k8sErrors.IsNotFound(err) {
		reqLogger.Info("Creating resource", "kind", fmt.Sprintf("%T", desired), "name", desired.GetName())
		if annotErr := litellmAnnotator.SetLastAppliedAnnotation(desired); annotErr != nil {
			return pkgerrors.Wrap(annotErr, "failed to annotate resource")
		}
		if createErr := r.Client.Create(ctx, desired); createErr != nil {
			return pkgerrors.Wrap(createErr, "failed to create resource")
		}
		return nil
	} else if err != nil {
		return pkgerrors.Wrap(err, "failed to get resource")
	}

	return r.Resources.Update(current, desired, reqLogger)
}

// checkLiteLLMDeployment ensures the LiteLLM ConfigMap and Deployment exist and are up to date.
func (r *AgentReconciler) checkLiteLLMDeployment(ctx context.Context, agent *mmv1beta.Agent, reqLogger logr.Logger) error {
	om := agent.Spec.LLMGateway.OperatorManaged

	desiredCM := mattermostApp.GenerateLiteLLMConfigMap(agent.Namespace)
	if err := r.ensureUnownedResource(ctx, desiredCM, &corev1.ConfigMap{}, reqLogger); err != nil {
		return pkgerrors.Wrap(err, "failed to ensure litellm configmap")
	}

	desiredDeploy := mattermostApp.GenerateLiteLLMDeployment(agent.Namespace, om.Image)
	if err := r.ensureUnownedResource(ctx, desiredDeploy, &appsv1.Deployment{}, reqLogger); err != nil {
		return pkgerrors.Wrap(err, "failed to ensure litellm deployment")
	}

	return nil
}

// checkLiteLLMService ensures the LiteLLM Service exists and is up to date.
func (r *AgentReconciler) checkLiteLLMService(ctx context.Context, agent *mmv1beta.Agent, reqLogger logr.Logger) error {
	desiredSvc := mattermostApp.GenerateLiteLLMService(agent.Namespace)
	if err := r.ensureUnownedResource(ctx, desiredSvc, &corev1.Service{}, reqLogger); err != nil {
		return pkgerrors.Wrap(err, "failed to ensure litellm service")
	}
	return nil
}

// checkLiteLLMReady reports whether LiteLLM has at least one ready replica.
func (r *AgentReconciler) checkLiteLLMReady(ctx context.Context, agent *mmv1beta.Agent, reqLogger logr.Logger) (bool, error) {
	deploy := &appsv1.Deployment{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMDeploymentName,
		Namespace: agent.Namespace,
	}, deploy)
	if err != nil {
		return false, pkgerrors.Wrap(err, "failed to get litellm deployment for readiness check")
	}
	if deploy.Status.ReadyReplicas < 1 {
		reqLogger.Info("LiteLLM not ready yet, will requeue", "readyReplicas", deploy.Status.ReadyReplicas)
		return false, nil
	}
	return true, nil
}

// checkLiteLLMMasterKey ensures the LiteLLM master-key Secret exists, generating
// one if necessary. The Secret is shared across all operatorManaged Agents in the
// namespace and is not owned by any single Agent. It is retained on teardown
// because the LiteLLM database outlives the Deployment and virtual keys minted
// under this master key remain valid in it.
func (r *AgentReconciler) checkLiteLLMMasterKey(ctx context.Context, agent *mmv1beta.Agent, reqLogger logr.Logger) error {
	existing := &corev1.Secret{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMMasterKeySecretName,
		Namespace: agent.Namespace,
	}, existing)
	if err == nil {
		return nil
	}
	if !k8sErrors.IsNotFound(err) {
		return pkgerrors.Wrap(err, "failed to check litellm master key secret")
	}

	hexKey, err := randomHex(32)
	if err != nil {
		return pkgerrors.Wrap(err, "failed to generate litellm master key")
	}
	masterKey := "sk-" + hexKey

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmv1beta.AgentLiteLLMMasterKeySecretName,
			Namespace: agent.Namespace,
			Labels:    mattermostApp.LiteLLMLabels(),
		},
		Data: map[string][]byte{"masterKey": []byte(masterKey)},
	}

	if annotErr := litellmAnnotator.SetLastAppliedAnnotation(secret); annotErr != nil {
		return pkgerrors.Wrap(annotErr, "failed to annotate litellm master key secret")
	}
	if createErr := r.Client.Create(ctx, secret); createErr != nil {
		return pkgerrors.Wrap(createErr, "failed to create litellm master key secret")
	}
	reqLogger.Info("Created LiteLLM master key secret", "name", secret.Name)
	return nil
}

// checkLiteLLMDBCredentials verifies that the externally-provisioned PostgreSQL
// connection-string Secret exists. The operator never creates this Secret; it must
// be provided by the cluster operator before operatorManaged LiteLLM can run.
func (r *AgentReconciler) checkLiteLLMDBCredentials(ctx context.Context, agent *mmv1beta.Agent) error {
	existing := &corev1.Secret{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMDBCredentialsSecret,
		Namespace: agent.Namespace,
	}, existing)
	if err == nil {
		return nil
	}
	if k8sErrors.IsNotFound(err) {
		return fmt.Errorf("required Secret %q (key %q) not found in namespace %s; operatorManaged LiteLLM requires an externally-provisioned PostgreSQL connection string",
			mmv1beta.AgentLiteLLMDBCredentialsSecret, "connectionString", agent.Namespace)
	}
	return pkgerrors.Wrap(err, "failed to check litellm db credentials secret")
}

// deleteLiteLLMResources removes the shared LiteLLM Deployment, Service, and
// ConfigMap from the namespace. The master-key Secret is retained alongside the
// user-managed DB credentials Secret, because the LiteLLM database outlives
// teardown and virtual keys minted under that master key remain valid in it —
// deleting the key would strand plugin-issued keys' admin access on recreate.
func (r *AgentReconciler) deleteLiteLLMResources(ctx context.Context, namespace string, reqLogger logr.Logger) error {
	toDelete := []client.Object{
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: mmv1beta.AgentLiteLLMDeploymentName, Namespace: namespace}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: mmv1beta.AgentLiteLLMServiceName, Namespace: namespace}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: mmv1beta.AgentLiteLLMConfigMapName, Namespace: namespace}},
	}
	for _, obj := range toDelete {
		if err := client.IgnoreNotFound(r.Client.Delete(ctx, obj)); err != nil {
			return pkgerrors.Wrapf(err, "failed to delete %T %q", obj, obj.GetName())
		}
		reqLogger.Info("Deleted LiteLLM resource", "kind", fmt.Sprintf("%T", obj), "name", obj.GetName())
	}
	return nil
}

// cleanupLiteLLMIfLast tears down the shared LiteLLM resources iff the Agent being
// deleted is the last one in the namespace that uses spec.llmGateway.operatorManaged.
// Other Agents currently being deleted are not counted as siblings.
func (r *AgentReconciler) cleanupLiteLLMIfLast(ctx context.Context, agent *mmv1beta.Agent, reqLogger logr.Logger) error {
	agents := &mmv1beta.AgentList{}
	if err := r.Client.List(ctx, agents, client.InNamespace(agent.Namespace)); err != nil {
		return pkgerrors.Wrap(err, "failed to list agents for litellm cleanup decision")
	}

	siblings := 0
	for i := range agents.Items {
		a := &agents.Items[i]
		if a.UID == agent.UID {
			continue
		}
		if !a.DeletionTimestamp.IsZero() {
			continue
		}
		if a.HasOperatorManagedGateway() {
			siblings++
		}
	}

	if siblings > 0 {
		reqLogger.Info("Other operatorManaged agents remain; leaving LiteLLM in place", "siblings", siblings)
		return nil
	}

	reqLogger.Info("Last operatorManaged agent removed; tearing down LiteLLM")
	return r.deleteLiteLLMResources(ctx, agent.Namespace, reqLogger)
}
