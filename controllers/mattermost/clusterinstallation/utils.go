package clusterinstallation

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/apis/mattermost/v1alpha1"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1beta1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *ClusterInstallationReconciler) handleCheckClusterInstallation(mattermost *mattermostv1alpha1.ClusterInstallation) (mattermostv1alpha1.ClusterInstallationStatus, error) {
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
func (r *ClusterInstallationReconciler) checkClusterInstallation(namespace, name, imageName, image, version string, replicas int32, useServiceLoadBalancer bool, labels map[string]string) (mattermostv1alpha1.ClusterInstallationStatus, error) {
	status := mattermostv1alpha1.ClusterInstallationStatus{
		State:           mattermostv1alpha1.Reconciling,
		Replicas:        0,
		UpdatedReplicas: 0,
	}

	listOptions := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels(labels),
	}

	err := r.checkRolloutStarted(name, namespace, listOptions)
	if err != nil {
		return status, errors.Wrap(err, "rollout not yet started")
	}

	pods := &corev1.PodList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
	}

	// We use non-cached client to make sure we get the real state of pods instead of
	// potentially outdated cached data.
	err = r.NonCachedAPIReader.List(context.TODO(), pods, listOptions...)
	if err != nil {
		return status, errors.Wrap(err, "unable to get pod list")
	}

	status.Replicas = int32(len(pods.Items))

	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning || pod.DeletionTimestamp != nil {
			return status, fmt.Errorf("mattermost pod %s is in state '%s'", pod.Name, pod.Status.Phase)
		}
		if len(pod.Spec.Containers) == 0 {
			return status, fmt.Errorf("mattermost pod %s has no containers", pod.Name)
		}
		if pod.Spec.Containers[0].Image != imageName {
			return status, fmt.Errorf("mattermost pod %s is running incorrect image", pod.Name)
		}

		podIsReady := false
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady {
				if condition.Status == corev1.ConditionTrue {
					podIsReady = true
					break
				} else {
					return status, fmt.Errorf("mattermost pod %s is not ready", pod.Name)
				}
			}
		}
		if !podIsReady {
			return status, fmt.Errorf("mattermost pod %s is not ready", pod.Name)
		}

		status.UpdatedReplicas++
	}

	if replicas < 0 {
		replicas = 0
	}

	if int32(len(pods.Items)) != replicas {
		return status, fmt.Errorf("found %d pods, but wanted %d", len(pods.Items), replicas)
	}

	status.Image = image
	status.Version = version

	status.Endpoint = "not available"
	if useServiceLoadBalancer {
		svc := &corev1.ServiceList{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Service",
				APIVersion: "v1",
			},
		}
		err := r.Client.List(context.TODO(), svc, listOptions...)
		if err != nil {
			return status, errors.Wrap(err, "unable to get service list")
		}
		if len(svc.Items) != 1 {
			return status, fmt.Errorf("should return just one service, but returned %d", len(svc.Items))
		}
		if svc.Items[0].Status.LoadBalancer.Ingress == nil {
			return status, errors.New("waiting for the Load Balancer to be active")
		}
		lbIngress := svc.Items[0].Status.LoadBalancer.Ingress[0]
		if lbIngress.Hostname != "" {
			status.Endpoint = lbIngress.Hostname
		} else if lbIngress.IP != "" {
			status.Endpoint = lbIngress.IP
		}
	} else {
		ingress := &v1beta1.IngressList{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Ingress",
				APIVersion: "v1",
			},
		}
		err := r.Client.List(context.TODO(), ingress, listOptions...)
		if err != nil {
			return status, errors.Wrap(err, "unable to get ingress list")
		}
		if len(ingress.Items) != 1 {
			return status, fmt.Errorf("should return just one ingress, but returned %d", len(ingress.Items))
		}
		status.Endpoint = ingress.Items[0].Spec.Rules[0].Host
	}

	// Everything checks out. The installation is stable.
	status.State = mattermostv1alpha1.Stable

	return status, nil
}

func (r *ClusterInstallationReconciler) checkRolloutStarted(name, namespace string, listOpts []client.ListOption) error {
	// To prevent race condition that new pods did not start rolling out and
	// old ones are still ready, we need to check if Deployment was picked up by controller.
	// We use non-cached client to make sure it won't return old Deployment where
	// the generation and observedGeneration still match.
	deployment := &appsv1.Deployment{}
	deploymentKey := types.NamespacedName{Name: name, Namespace: namespace}
	err := r.NonCachedAPIReader.Get(context.TODO(), deploymentKey, deployment)
	if err != nil {
		return errors.Wrap(err, "Failed to get deployment")
	}
	if deployment.Generation != deployment.Status.ObservedGeneration {
		return fmt.Errorf("mattermost deployment not yet picked up by the Deployment controller")
	}

	// We check if new ReplicaSet was created and it was observed by the controller
	// to guarantee that new pods are created.
	replicaSets := &appsv1.ReplicaSetList{}
	err = r.Client.List(context.TODO(), replicaSets, listOpts...)
	if err != nil {
		return errors.Wrap(err, "failed to list replicaSets")
	}

	replicaSetReady := false
	for _, rep := range replicaSets.Items {
		if getRevision(rep.Annotations) == getRevision(deployment.Annotations) {
			if rep.Status.ObservedGeneration > 0 {
				replicaSetReady = true
				break
			}
		}
	}
	if !replicaSetReady {
		return fmt.Errorf("replicaSet did not start rolling pods")
	}

	return nil
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

func getRevision(annotations map[string]string) string {
	if annotations == nil {
		return ""
	}
	return annotations["deployment.kubernetes.io/revision"]
}
