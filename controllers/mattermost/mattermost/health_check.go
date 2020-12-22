package mattermost

import (
	"fmt"

	"github.com/go-logr/logr"
	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	"github.com/mattermost/mattermost-operator/pkg/mattermost/healthcheck"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// checkMattermostHealth checks the health and correctness of the k8s
// objects that make up a Mattermost installation.
//
// NOTE: this is a vital health check. Every reconciliation loop should run this
// check at the very end to ensure that everything in the installation is as it
// should be. Over time, more types of checks should be added here as needed.
func (r *MattermostReconciler) checkMattermostHealth(mattermost *mmv1beta.Mattermost, logger logr.Logger) (mmv1beta.MattermostStatus, error) {
	status := mmv1beta.MattermostStatus{
		State:           mmv1beta.Reconciling,
		Replicas:        0,
		UpdatedReplicas: 0,
	}

	labels := mattermost.MattermostLabels(mattermost.Name)
	listOptions := []client.ListOption{
		client.InNamespace(mattermost.Namespace),
		client.MatchingLabels(labels),
	}

	healthChecker := healthcheck.NewHealthChecker(r.NonCachedAPIReader, listOptions, logger)

	err := healthChecker.AssertDeploymentRolloutStarted(mattermost.Name, mattermost.Namespace)
	if err != nil {
		return status, errors.Wrap(err, "rollout not yet started")
	}

	podsStatus, err := healthChecker.CheckPodsRollOut(mattermost.GetImageName())
	if err != nil {
		return status, errors.Wrap(err, "failed to check pods status")
	}

	status.UpdatedReplicas = podsStatus.UpdatedReplicas
	status.Replicas = podsStatus.Replicas

	var replicas int32 = 1
	if mattermost.Spec.Replicas != nil {
		replicas = *mattermost.Spec.Replicas
	}

	if podsStatus.UpdatedReplicas != replicas {
		return status, fmt.Errorf("found %d updated replicas, but wanted %d", podsStatus.UpdatedReplicas, replicas)
	}
	if podsStatus.Replicas != replicas {
		return status, fmt.Errorf("found %d pods, but wanted %d", podsStatus.Replicas, replicas)
	}

	status.Image = mattermost.Spec.Image
	status.Version = mattermost.Spec.Version

	status.Endpoint = "not available"
	var endpoint string

	if mattermost.Spec.UseServiceLoadBalancer {
		endpoint, err = healthChecker.CheckServiceLoadBalancer()
		if err != nil {
			return status, errors.Wrap(err, "failed to check service load balancer")
		}
	} else {
		endpoint, err = healthChecker.CheckIngressLoadBalancer()
		if err != nil {
			return status, errors.Wrap(err, "failed to check ingress load balancer")
		}
	}

	if endpoint != "" {
		status.Endpoint = endpoint
	}

	// Everything checks out. The installation is stable.
	status.State = mmv1beta.Stable

	return status, nil
}
