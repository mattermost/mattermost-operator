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
func (r *MattermostReconciler) checkMattermostHealth(mattermost *mmv1beta.Mattermost, currentStatus mmv1beta.MattermostStatus, logger logr.Logger) (mmv1beta.MattermostStatus, error) {
	status := mmv1beta.MattermostStatus{
		State:              mmv1beta.Reconciling,
		ObservedGeneration: mattermost.Generation,
		Replicas:           0,
		UpdatedReplicas:    0,
		// Rewrite Resource Patch status to not lose it.
		// It is cleared when appropriate by resource patch logic.
		ResourcePatch: currentStatus.ResourcePatch,
	}

	labels := mattermost.MattermostPodLabels(mattermost.Name)
	listOptions := []client.ListOption{
		client.InNamespace(mattermost.Namespace),
		client.MatchingLabels(labels),
	}

	healthChecker := healthcheck.NewHealthChecker(r.NonCachedAPIReader, listOptions, logger)

	podsStatus, err := healthChecker.CheckReplicaSetRollout(mattermost.Name, mattermost.Namespace)
	if err != nil {
		return status, errors.Wrap(err, "rollout not yet started")
	}

	status.UpdatedReplicas = podsStatus.UpdatedReplicas
	status.Replicas = podsStatus.Replicas

	var replicas int32 = 1
	if mattermost.Spec.Replicas != nil {
		replicas = *mattermost.Spec.Replicas
	}

	if replicas > 0 && podsStatus.UpdatedReplicas == 0 {
		return status, fmt.Errorf("mattermost pods not yet updated")
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
	} else if mattermost.IngressEnabled() {
		endpoint, err = healthChecker.CheckIngressLoadBalancer()
		if err != nil {
			return status, errors.Wrap(err, "failed to check ingress load balancer")
		}
	}

	if endpoint != "" {
		status.Endpoint = endpoint
	}

	if replicas > 0 {
		// At least one pod is updated and LB/Ingress is ready therefore we are at
		// least ready to server traffic.
		status.State = mmv1beta.Ready
	}

	if podsStatus.UpdatedReplicas != replicas {
		return status, fmt.Errorf("found %d updated replicas, but wanted %d", podsStatus.UpdatedReplicas, replicas)
	}
	if podsStatus.Replicas != replicas {
		return status, fmt.Errorf("found %d pods, but wanted %d", podsStatus.Replicas, replicas)
	}

	if mattermost.Spec.JobServer != nil && mattermost.Spec.JobServer.DedicatedJobServer {
		err = r.checkMattermostJobServerHealth(mattermost, logger)
		if err != nil {
			return status, errors.Wrap(err, "failed to check job server health")
		}
	}

	// Everything checks out. The installation is stable.
	status.State = mmv1beta.Stable

	return status, nil
}

func (r *MattermostReconciler) checkMattermostJobServerHealth(mattermost *mmv1beta.Mattermost, logger logr.Logger) error {
	labels := mattermost.MattermostJobServerPodLabels(mattermost.Name)
	listOptions := []client.ListOption{
		client.InNamespace(mattermost.Namespace),
		client.MatchingLabels(labels),
	}

	healthChecker := healthcheck.NewHealthChecker(r.NonCachedAPIReader, listOptions, logger)

	jobServerPodsStatus, err := healthChecker.CheckReplicaSetRollout(mattermost.DedicatedJobServerName(), mattermost.Namespace)
	if err != nil {
		return errors.Wrap(err, "job server pod rollout not yet started")
	}

	if jobServerPodsStatus.UpdatedReplicas == 0 {
		return errors.New("mattermost job server pods not yet updated")
	}

	if jobServerPodsStatus.UpdatedReplicas != 1 {
		return fmt.Errorf("found %d updated job server replicas, but wanted 1", jobServerPodsStatus.UpdatedReplicas)
	}
	if jobServerPodsStatus.Replicas != 1 {
		return fmt.Errorf("found job server %d pods, but wanted 1", jobServerPodsStatus.Replicas)
	}

	return nil
}
