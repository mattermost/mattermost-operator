package agent

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	mattermostApp "github.com/mattermost/mattermost-operator/pkg/mattermost"
	"github.com/mattermost/mattermost-operator/pkg/resources"
	"github.com/mattermost/mattermost-operator/pkg/utils"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
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
		return nil
	}
	if !k8sErrors.IsNotFound(err) {
		return errors.Wrap(err, "failed to check for existing hook secret")
	}

	secretValue, err := utils.RandomHex(32)
	if err != nil {
		return errors.Wrap(err, "failed to generate random hook secret")
	}

	desired := mattermostApp.GenerateAgentHookSecret(agent, secretValue)
	if err := r.Resources.CreateIfNotExists(agent, desired, reqLogger); err != nil {
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

// checkExternallyProvisionedSecrets verifies Secrets that the agents plugin (not the
// operator) must create before the agent Deployment can start. Missing secrets are a
// config/dependency issue — the operator never creates them.
func (r *AgentReconciler) checkExternallyProvisionedSecrets(ctx context.Context, agent *mmv1beta.Agent) error {
	required := []string{agent.BotTokenSecretName()}

	if agent.HasLLMGateway() {
		switch {
		case agent.HasOperatorManagedGateway():
			required = append(required, agent.LiteLLMKeySecretName())
		case agent.Spec.LLMGateway.External != nil:
			required = append(required, agent.Spec.LLMGateway.External.VirtualKeySecret)
		}
	}

	for _, name := range required {
		existing := &corev1.Secret{}
		err := r.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: agent.Namespace}, existing)
		if err == nil {
			continue
		}
		if k8sErrors.IsNotFound(err) {
			return fmt.Errorf("required Secret %q not found: it must be provisioned externally (the Mattermost agents plugin creates it); the operator does not manage this secret", name)
		}
		return errors.Wrapf(err, "failed to check for required secret %q", name)
	}

	return nil
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

	err := r.Resources.CreateIfNotExists(agent, desired, reqLogger)
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

// checkAgentPVC creates the agent storage PVC if configured. PVCs are create-only;
// size/class changes require manual intervention (PVC specs are mostly immutable).
func (r *AgentReconciler) checkAgentPVC(ctx context.Context, agent *mmv1beta.Agent, reqLogger logr.Logger) error {
	if agent.Spec.Storage == nil {
		return nil
	}

	desired := mattermostApp.GenerateAgentPVC(agent)
	if err := r.Resources.CreatePvcIfNotExists(agent, desired, reqLogger); err != nil {
		return errors.Wrap(err, "failed to create agent storage PVC")
	}
	return nil
}

func (r *AgentReconciler) checkAgentHealth(ctx context.Context, agent *mmv1beta.Agent, prior mmv1beta.AgentStatus, reqLogger logr.Logger) (mmv1beta.AgentStatus, error) {
	status := prior
	status.State = mmv1beta.Reconciling
	status.ObservedGeneration = agent.Generation
	status.Endpoint = fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", agent.Name, agent.Namespace, mmv1beta.AgentHTTPPort)

	deployment := &appsv1.Deployment{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, deployment)
	if err != nil {
		return status, errors.Wrap(err, "failed to get agent deployment for health check")
	}

	if deployment.Status.ReadyReplicas < 1 {
		return status, fmt.Errorf("agent deployment has %d ready replicas, need at least 1", deployment.Status.ReadyReplicas)
	}

	status.State = mmv1beta.Stable
	status.ReadyReplicas = deployment.Status.ReadyReplicas
	status.Error = ""
	return status, nil
}
