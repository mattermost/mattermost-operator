package clusterinstallation

import (
	"context"
	"fmt"
	"reflect"

	"github.com/mattermost/mattermost-operator/pkg/mattermost/healthcheck"

	"github.com/go-logr/logr"
	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/apis/mattermost/v1alpha1"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *ClusterInstallationReconciler) handleCheckClusterInstallation(mattermost *mattermostv1alpha1.ClusterInstallation, logger logr.Logger) (mattermostv1alpha1.ClusterInstallationStatus, error) {
	if !mattermost.Spec.BlueGreen.Enable {
		return r.checkClusterInstallation(
			mattermost.GetNamespace(),
			mattermost.Name,
			mattermost.GetImageName(),
			mattermost.Spec.Image,
			mattermost.Spec.Version,
			mattermost.Spec.Replicas,
			mattermost.Spec.UseServiceLoadBalancer,
			mattermost.ClusterInstallationLabels(mattermost.Name),
			logger,
		)
	}

	// BlueGreen is a bit tricky. To properly check for errors and also to return
	// the correct status, we should check both and then manually return status.
	blueStatus, blueErr := r.checkClusterInstallation(
		mattermost.GetNamespace(),
		mattermost.Spec.BlueGreen.Blue.Name,
		mattermost.Spec.BlueGreen.Blue.GetDeploymentImageName(),
		mattermost.Spec.BlueGreen.Blue.Image,
		mattermost.Spec.BlueGreen.Blue.Version,
		mattermost.Spec.Replicas,
		mattermost.Spec.UseServiceLoadBalancer,
		mattermost.ClusterInstallationLabels(mattermost.Spec.BlueGreen.Blue.Name),
		logger,
	)
	greenStatus, greenErr := r.checkClusterInstallation(
		mattermost.GetNamespace(),
		mattermost.Spec.BlueGreen.Green.Name,
		mattermost.Spec.BlueGreen.Green.GetDeploymentImageName(),
		mattermost.Spec.BlueGreen.Green.Image,
		mattermost.Spec.BlueGreen.Green.Version,
		mattermost.Spec.Replicas,
		mattermost.Spec.UseServiceLoadBalancer,
		mattermost.ClusterInstallationLabels(mattermost.Spec.BlueGreen.Green.Name),
		logger,
	)

	var status mattermostv1alpha1.ClusterInstallationStatus
	if mattermost.Spec.BlueGreen.ProductionDeployment == mattermostv1alpha1.BlueName {
		status = blueStatus
	} else {
		status = greenStatus
	}

	status.BlueName = mattermost.Spec.BlueGreen.Blue.Name
	status.GreenName = mattermost.Spec.BlueGreen.Green.Name

	if blueErr != nil {
		return status, errors.Wrap(blueErr, "blue installation validation failed")
	}
	if greenErr != nil {
		return status, errors.Wrap(greenErr, "green installation validation failed")
	}

	return status, nil
}

// checkClusterInstallation checks the health and correctness of the k8s
// objects that make up a MattermostInstallation.
//
// NOTE: this is a vital health check. Every reconciliation loop should run this
// check at the very end to ensure that everything in the installation is as it
// should be. Over time, more types of checks should be added here as needed.
func (r *ClusterInstallationReconciler) checkClusterInstallation(namespace, name, imageName, image, version string, replicas int32, useServiceLoadBalancer bool, labels map[string]string, logger logr.Logger) (mattermostv1alpha1.ClusterInstallationStatus, error) {
	status := mattermostv1alpha1.ClusterInstallationStatus{
		State:           mattermostv1alpha1.Reconciling,
		Replicas:        0,
		UpdatedReplicas: 0,
	}

	listOptions := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels(labels),
	}

	healthChecker := healthcheck.NewHealthChecker(r.NonCachedAPIReader, listOptions, logger)

	err := healthChecker.AssertDeploymentRolloutStarted(name, namespace)
	if err != nil {
		return status, errors.Wrap(err, "rollout not yet started")
	}

	podsStatus, err := healthChecker.CheckPodsRollOut(imageName)
	if err != nil {
		return status, errors.Wrap(err, "failed to check pods status")
	}

	status.UpdatedReplicas = podsStatus.UpdatedReplicas
	status.Replicas = podsStatus.Replicas

	if replicas < 0 {
		replicas = 0
	}

	if podsStatus.UpdatedReplicas != replicas {
		return status, fmt.Errorf("found %d updated replicas, but wanted %d", podsStatus.UpdatedReplicas, replicas)
	}
	if podsStatus.Replicas != replicas {
		return status, fmt.Errorf("found %d pods, but wanted %d", podsStatus.Replicas, replicas)
	}

	status.Image = image
	status.Version = version

	status.Endpoint = "not available"
	var endpoint string

	if useServiceLoadBalancer {
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
	status.State = mattermostv1alpha1.Stable

	return status, nil
}

func (r *ClusterInstallationReconciler) checkSecret(secretName, keyName, namespace string) error {
	foundSecret := &corev1.Secret{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: namespace}, foundSecret)
	if err != nil {
		return errors.Wrap(err, "error getting secret")
	}

	for key := range foundSecret.Data {
		if keyName == key {
			return nil
		}
	}

	return fmt.Errorf("secret %s is missing data key: %s", secretName, keyName)
}

func (r *ClusterInstallationReconciler) updateStatus(mattermost *mattermostv1alpha1.ClusterInstallation, status mattermostv1alpha1.ClusterInstallationStatus, reqLogger logr.Logger) error {
	if !reflect.DeepEqual(mattermost.Status, status) {
		if mattermost.Status.State != status.State {
			reqLogger.Info(fmt.Sprintf("Updating ClusterInstallation state from '%s' to '%s'", mattermost.Status.State, status.State))
		}

		mattermost.Status = status
		err := r.Client.Status().Update(context.TODO(), mattermost)
		if err != nil {
			return errors.Wrap(err, "failed to update the clusterinstallation status")
		}
	}

	return nil
}

// setStateReconciling sets the ClusterInstallation state to reconciling.
func (r *ClusterInstallationReconciler) setStateReconciling(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	return r.setState(mattermost, mattermostv1alpha1.Reconciling, reqLogger)
}

// setStateReconcilingAndLogError attempts to set the ClusterInstallation state
// to reconciling. Any errors attempting this are logged, but not returned. This
// should only be used when the outcome of setting the state can be ignored.
func (r *ClusterInstallationReconciler) setStateReconcilingAndLogError(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) {
	err := r.setState(mattermost, mattermostv1alpha1.Reconciling, reqLogger)
	if err != nil {
		reqLogger.Error(err, "Failed to set state to reconciling")
	}
}

// setState sets the provided ClusterInstallation to the provided state if that
// is different from the current state.
func (r *ClusterInstallationReconciler) setState(mattermost *mattermostv1alpha1.ClusterInstallation, desired mattermostv1alpha1.RunningState, reqLogger logr.Logger) error {
	if mattermost.Status.State != desired {
		status := mattermost.Status
		status.State = desired
		err := r.updateStatus(mattermost, status, reqLogger)
		if err != nil {
			return errors.Wrapf(err, "failed to set state to %s", desired)
		}
	}

	return nil
}

func ensureLabels(required, final map[string]string) map[string]string {
	if required == nil {
		return final
	}

	if final == nil {
		final = make(map[string]string)
	}

	for key, value := range required {
		final[key] = value
	}

	return final
}
