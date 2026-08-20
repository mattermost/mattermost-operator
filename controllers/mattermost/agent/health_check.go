package agent

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	mattermostApp "github.com/mattermost/mattermost-operator/pkg/mattermost"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
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
		// The deployment is gone, so don't carry over a stale count. Other
		// errors (e.g. API blips) keep the prior count rather than misreport 0.
		if k8sErrors.IsNotFound(err) {
			status.ReadyReplicas = 0
		}
		return status, errors.Wrap(err, "failed to get agent deployment for health check")
	}

	// Refresh replica counts even when the rollout is incomplete, so the
	// persisted status never carries stale numbers.
	status.ReadyReplicas = deployment.Status.ReadyReplicas

	err = checkDeploymentRolloutComplete(deployment)
	if err != nil {
		return status, err
	}

	status.State = mmv1beta.Stable
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

	// Matches kubectl rollout-status semantics: old replicas still
	// terminating mean the rollout has not completed.
	if deployment.Status.Replicas != replicas {
		return fmt.Errorf("deployment has %d total replicas, want %d (old replicas may still be terminating)", deployment.Status.Replicas, replicas)
	}

	if deployment.Status.ReadyReplicas < 1 {
		return fmt.Errorf("deployment has %d ready replicas, need at least 1", deployment.Status.ReadyReplicas)
	}

	return nil
}
