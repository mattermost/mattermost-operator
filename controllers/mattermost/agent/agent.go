package agent

import (
	"context"

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

// checkAgent ensures all resources that make up an Agent. The NetworkPolicy is
// created before the Deployment so agent pods never start without their egress
// policy.
func (r *AgentReconciler) checkAgent(ctx context.Context, agent *mmv1beta.Agent, reqLogger logr.Logger) error {
	reqLogger = reqLogger.WithValues("Reconcile", "agent")

	err := r.checkAgentServiceAccount(ctx, agent, reqLogger)
	if err != nil {
		return err
	}

	err = r.checkHookSecret(ctx, agent, reqLogger)
	if err != nil {
		return err
	}

	err = r.checkAgentNetworkPolicy(ctx, agent, reqLogger)
	if err != nil {
		return err
	}

	err = r.checkAgentService(ctx, agent, reqLogger)
	if err != nil {
		return err
	}

	err = r.checkAgentPVC(ctx, agent, reqLogger)
	if err != nil {
		return err
	}

	err = r.checkExternallyProvisionedSecrets(ctx, agent)
	if err != nil {
		return err
	}

	return r.checkAgentDeployment(ctx, agent, reqLogger)
}

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
	secretValue, err := utils.RandomHex(32)
	if err != nil {
		return errors.Wrap(err, "failed to generate random hook secret")
	}

	desired := mattermostApp.GenerateAgentHookSecret(agent, secretValue)
	return errors.Wrap(r.Resources.CreateIfNotExists(agent, desired, reqLogger), "failed to create hook secret")
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

// checkExternallyProvisionedSecrets verifies Secrets that the agents plugin
// (not the operator) must create before the agent Deployment can start. A
// missing Secret or missing/empty key is a config issue (errConfigIssue);
// transient API failures are real errors.
func (r *AgentReconciler) checkExternallyProvisionedSecrets(ctx context.Context, agent *mmv1beta.Agent) error {
	type requiredSecret struct{ name, key string }

	required := []requiredSecret{{agent.BotTokenSecretName(), mmv1beta.SecretKeyBotToken}}
	if _, keySecretName, ok := agent.GatewayEndpoint(); ok {
		required = append(required, requiredSecret{keySecretName, mmv1beta.SecretKeyAPIKey})
	}

	for _, req := range required {
		existing := &corev1.Secret{}
		err := r.Client.Get(ctx, types.NamespacedName{Name: req.name, Namespace: agent.Namespace}, existing)
		if k8sErrors.IsNotFound(err) {
			return errors.Wrapf(errConfigIssue, "required Secret %q not found: it must be provisioned externally (the Mattermost agents plugin creates it); the operator does not manage this secret", req.name)
		}
		if err != nil {
			return errors.Wrapf(err, "failed to check for required secret %q", req.name)
		}
		if len(existing.Data[req.key]) == 0 {
			return errors.Wrapf(errConfigIssue, "required Secret %q must contain a non-empty %q key; it is provisioned externally (the Mattermost agents plugin creates it)", req.name, req.key)
		}
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
