package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/go-logr/logr"
	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	mattermostApp "github.com/mattermost/mattermost-operator/pkg/mattermost"
	"github.com/mattermost/mattermost-operator/pkg/resources"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (r *AgentReconciler) checkAgentServiceAccount(ctx context.Context, agent *mmv1beta.Agent, reqLogger logr.Logger) error {
	desired := mattermostApp.GenerateAgentServiceAccount(agent)

	err := r.Resources.CreateServiceAccountIfNotExists(agent, desired, reqLogger)
	if err != nil {
		return errors.Wrap(err, "failed to create agent service account")
	}

	current := &corev1.ServiceAccount{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, current)
	if err != nil {
		return errors.Wrap(err, "failed to get agent service account")
	}

	return r.Resources.Update(current, desired, reqLogger)
}

func (r *AgentReconciler) checkHookSecret(ctx context.Context, agent *mmv1beta.Agent, reqLogger logr.Logger) error {
	secretName := agent.HookSecretName()
	existingSecret := &corev1.Secret{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: secretName, Namespace: agent.Namespace}, existingSecret)
	if err == nil {
		return nil // Secret already exists
	}
	if !k8sErrors.IsNotFound(err) {
		return errors.Wrap(err, "failed to check for existing hook secret")
	}

	// Generate a random 32-byte hex string.
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return errors.Wrap(err, "failed to generate random hook secret")
	}
	secretValue := hex.EncodeToString(b)

	desired := mattermostApp.GenerateAgentHookSecret(agent, secretValue)
	if err := r.Resources.CreateSecretIfNotExists(agent, desired, reqLogger); err != nil {
		return errors.Wrap(err, "failed to create hook secret")
	}

	reqLogger.Info("Created hook secret", "secret", secretName)
	return nil
}

func (r *AgentReconciler) checkAgentService(ctx context.Context, agent *mmv1beta.Agent, reqLogger logr.Logger) error {
	desired := mattermostApp.GenerateAgentService(agent)

	err := r.Resources.CreateServiceIfNotExists(agent, desired, reqLogger)
	if err != nil {
		return errors.Wrap(err, "failed to create agent service")
	}

	current := &corev1.Service{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, current)
	if err != nil {
		return errors.Wrap(err, "failed to get agent service")
	}

	resources.CopyServiceEmptyAutoAssignedFields(desired, current)

	return r.Resources.Update(current, desired, reqLogger)
}

func (r *AgentReconciler) checkAgentDeployment(ctx context.Context, agent *mmv1beta.Agent, reqLogger logr.Logger) error {
	desired := mattermostApp.GenerateAgentDeployment(agent)

	err := r.Resources.CreateDeploymentIfNotExists(agent, desired, reqLogger)
	if err != nil {
		return errors.Wrap(err, "failed to create agent deployment")
	}

	current := &appsv1.Deployment{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, current)
	if err != nil {
		return errors.Wrap(err, "failed to get agent deployment")
	}

	return r.Resources.Update(current, desired, reqLogger)
}

func (r *AgentReconciler) checkAgentNetworkPolicy(ctx context.Context, agent *mmv1beta.Agent, reqLogger logr.Logger) error {
	desired := mattermostApp.GenerateAgentNetworkPolicy(agent)

	err := r.Resources.CreateNetworkPolicyIfNotExists(agent, desired, reqLogger)
	if err != nil {
		return errors.Wrap(err, "failed to create agent network policy")
	}

	current := &networkingv1.NetworkPolicy{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, current)
	if err != nil {
		return errors.Wrap(err, "failed to get agent network policy")
	}

	return r.Resources.Update(current, desired, reqLogger)
}

func (r *AgentReconciler) checkAgentPVC(ctx context.Context, agent *mmv1beta.Agent, reqLogger logr.Logger) error {
	if agent.Spec.Storage == nil {
		return nil
	}

	desired := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:            agent.StoragePVCName(),
			Namespace:       agent.Namespace,
			Labels:          mmv1beta.AgentLabels(agent.Name),
			OwnerReferences: mattermostApp.AgentOwnerReference(agent),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: agent.Spec.Storage.Size,
				},
			},
		},
	}

	if agent.Spec.Storage.StorageClassName != nil {
		desired.Spec.StorageClassName = agent.Spec.Storage.StorageClassName
	}

	err := r.Resources.CreatePvcIfNotExists(agent, desired, reqLogger)
	if err != nil {
		return errors.Wrap(err, "failed to create agent storage PVC")
	}

	current := &corev1.PersistentVolumeClaim{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, current)
	if err != nil {
		return errors.Wrap(err, "failed to get agent storage PVC")
	}

	return r.Resources.Update(current, desired, reqLogger)
}

func (r *AgentReconciler) checkAgentHealth(ctx context.Context, agent *mmv1beta.Agent, currentStatus mmv1beta.AgentStatus, reqLogger logr.Logger) (mmv1beta.AgentStatus, error) {
	status := mmv1beta.AgentStatus{
		State:              mmv1beta.Reconciling,
		Phase:              mmv1beta.AgentPhaseDeploying,
		ObservedGeneration: agent.Generation,
		Endpoint:           fmt.Sprintf("%s.%s.svc.cluster.local:%d", agent.Name, agent.Namespace, mmv1beta.AgentHTTPPort),
	}

	deployment := &appsv1.Deployment{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, deployment)
	if err != nil {
		return status, errors.Wrap(err, "failed to get agent deployment for health check")
	}

	if deployment.Status.ReadyReplicas < 1 {
		return status, fmt.Errorf("agent deployment has %d ready replicas, need at least 1", deployment.Status.ReadyReplicas)
	}

	status.State = mmv1beta.Stable
	status.Phase = mmv1beta.AgentPhaseReady
	status.ReadyReplicas = deployment.Status.ReadyReplicas
	return status, nil
}
