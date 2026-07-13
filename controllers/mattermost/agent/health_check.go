package agent

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	mattermostApp "github.com/mattermost/mattermost-operator/pkg/mattermost"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
)

// checkAgentHealth checks that the agent Deployment finished its rollout and
// computes the resulting status. It only inspects health; status bookkeeping
// such as ObservedGeneration is handled by Reconcile.
func (r *AgentReconciler) checkAgentHealth(ctx context.Context, agent *mmv1beta.Agent, prior mmv1beta.AgentStatus, reqLogger logr.Logger) (mmv1beta.AgentStatus, error) {
	status := prior
	status.State = mmv1beta.Reconciling
	status.Endpoint = mattermostApp.AgentServiceURL(agent)

	deployment := &appsv1.Deployment{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      mattermostApp.AgentDeploymentName(agent),
		Namespace: agent.Namespace,
	}, deployment)
	if err != nil {
		return status, errors.Wrap(err, "failed to get agent deployment for health check")
	}

	err = checkDeploymentRolloutComplete(deployment)
	if err != nil {
		return status, err
	}

	status.State = mmv1beta.Stable
	status.ReadyReplicas = deployment.Status.ReadyReplicas
	status.Error = ""
	return status, nil
}

// checkDeploymentRolloutComplete returns an error describing why a Deployment
// rollout has not completed, or nil once the Deployment controller observed
// the latest spec and all updated replicas are ready. ReadyReplicas alone is
// not enough: mid-rollout the old ReplicaSet can still be serving.
func checkDeploymentRolloutComplete(deployment *appsv1.Deployment) error {
	if deployment.Status.ObservedGeneration < deployment.Generation {
		return fmt.Errorf("deployment generation %d not yet observed by the deployment controller (observed %d)",
			deployment.Generation, deployment.Status.ObservedGeneration)
	}

	var replicas int32 = 1
	if deployment.Spec.Replicas != nil {
		replicas = *deployment.Spec.Replicas
	}
	if deployment.Status.UpdatedReplicas != replicas {
		return fmt.Errorf("deployment has %d updated replicas, want %d", deployment.Status.UpdatedReplicas, replicas)
	}

	if deployment.Status.ReadyReplicas < 1 {
		return fmt.Errorf("deployment has %d ready replicas, need at least 1", deployment.Status.ReadyReplicas)
	}

	return nil
}
