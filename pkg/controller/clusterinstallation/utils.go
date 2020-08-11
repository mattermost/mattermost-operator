package clusterinstallation

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/blang/semver/v4"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	v1beta1 "k8s.io/api/extensions/v1beta1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const semVerRegex string = `^?([0-9]+)(\.[0-9]+)?(\.[0-9]+)`

func (r *ReconcileClusterInstallation) handleCheckClusterInstallation(mattermost *mattermostv1alpha1.ClusterInstallation) (mattermostv1alpha1.ClusterInstallationStatus, error) {
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
func (r *ReconcileClusterInstallation) checkClusterInstallation(namespace, name, imageName, image, version string, replicas int32, useServiceLoadBalancer bool, labels map[string]string) (mattermostv1alpha1.ClusterInstallationStatus, error) {
	status := mattermostv1alpha1.ClusterInstallationStatus{
		State:           mattermostv1alpha1.Reconciling,
		Replicas:        0,
		UpdatedReplicas: 0,
	}

	pods := &corev1.PodList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
	}

	listOptions := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels(labels),
	}
	err := r.client.List(context.TODO(), pods, listOptions...)
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
		err := r.client.List(context.TODO(), svc, listOptions...)
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
		err := r.client.List(context.TODO(), ingress, listOptions...)
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

func (r *ReconcileClusterInstallation) checkSecret(secretName, keyName, namespace string) error {
	foundSecret := &corev1.Secret{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: namespace}, foundSecret)
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

func (r *ReconcileClusterInstallation) getImageVersion(mattermost *mattermostv1alpha1.ClusterInstallation, image string) (string, error) {
	// start a one shot pod to get the version from the mattermost image, if fails we assume that is a bad image
	mmVersionPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mm-version-pod",
			Namespace: mattermost.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(mattermost, schema.GroupVersionKind{
					Group:   mattermostv1alpha1.SchemeGroupVersion.Group,
					Version: mattermostv1alpha1.SchemeGroupVersion.Version,
					Kind:    "ClusterInstallation",
				}),
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:            "mm-version-pod",
					Image:           image,
					ImagePullPolicy: corev1.PullIfNotPresent,
					Command: []string{
						"sleep",
						"3600",
					},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("10m"),
							corev1.ResourceMemory: resource.MustParse("10M"),
						},
					},
				},
			},
			TerminationGracePeriodSeconds: pointer.Int64Ptr(5),
		},
	}

	err := r.client.Create(context.TODO(), mmVersionPod)
	if err != nil && !k8sErrors.IsAlreadyExists(err) {
		return "", errors.Wrap(err, "failed to create the temporary pod")
	}

	command := []string{"./bin/mattermost", "version", "--skip-server-start"}
	stdOut, err := r.execPod(mmVersionPod, command)
	if err != nil {
		return "", errors.Wrap(err, "failed to execute the command in the temporary pod")
	}

	if stdOut != "" {
		versionRegex := regexp.MustCompile(semVerRegex)
		actualMMVersion := versionRegex.FindString(strings.TrimSpace(stdOut))

		v, err := semver.Parse(actualMMVersion)
		if err != nil {
			return "", errors.Wrap(err, "failed to parse the version")
		}

		expectedRange, err := semver.ParseRange(">=5.28.0")
		if err != nil {
			return "", errors.Wrapf(err, "failed to parse the version range for %s", actualMMVersion)
		}
		if !expectedRange(v) {
			return "", errors.Errorf("invalid Version option %s, must be greater than 5.26.0", actualMMVersion)
		}

		return v.String(), nil
	}

	return "", errors.New("failed to get the actual version")
}

func (r *ReconcileClusterInstallation) updateStatus(mattermost *mattermostv1alpha1.ClusterInstallation, status mattermostv1alpha1.ClusterInstallationStatus, reqLogger logr.Logger) error {
	if !reflect.DeepEqual(mattermost.Status, status) {
		if mattermost.Status.State != status.State {
			reqLogger.Info(fmt.Sprintf("Updating ClusterInstallation state from '%s' to '%s'", mattermost.Status.State, status.State))
		}

		mattermost.Status = status
		err := r.client.Status().Update(context.TODO(), mattermost)
		if err != nil {
			return errors.Wrap(err, "failed to update the clusterinstallation status")
		}
	}

	return nil
}

// setStateReconciling sets the ClusterInstallation state to reconciling.
func (r *ReconcileClusterInstallation) setStateReconciling(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	return r.setState(mattermost, mattermostv1alpha1.Reconciling, reqLogger)
}

// setStateReconcilingAndLogError attempts to set the ClusterInstallation state
// to reconciling. Any errors attempting this are logged, but not returned. This
// should only be used when the outcome of setting the state can be ignored.
func (r *ReconcileClusterInstallation) setStateReconcilingAndLogError(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) {
	err := r.setState(mattermost, mattermostv1alpha1.Reconciling, reqLogger)
	if err != nil {
		reqLogger.Error(err, "Failed to set state to reconciling")
	}
}

// setState sets the provided ClusterInstallation to the provided state if that
// is different from the current state.
func (r *ReconcileClusterInstallation) setState(mattermost *mattermostv1alpha1.ClusterInstallation, desired mattermostv1alpha1.RunningState, reqLogger logr.Logger) error {
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

func (r *ReconcileClusterInstallation) cleanSupportPods(namespace string) error {
	mmPod := &corev1.Pod{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: "mm-version-pod", Namespace: namespace}, mmPod)
	if err != nil {
		return errors.Wrap(err, "failed to get the temporary pod")
	}

	policy := metav1.DeletePropagationForeground
	opts := &client.DeleteOptions{
		GracePeriodSeconds: pointer.Int64Ptr(0),
		PropagationPolicy:  &policy,
	}
	err = r.client.Delete(context.TODO(), mmPod, opts)
	if err != nil {
		return errors.Wrap(err, "failed to delete the temporary pod")
	}

	return nil
}

func (r *ReconcileClusterInstallation) execPod(inputPod *corev1.Pod, command []string) (string, error) {
	err := wait.Poll(500*time.Millisecond, 5*time.Minute, func() (bool, error) {
		pod := &corev1.Pod{}
		errPod := r.client.Get(context.TODO(), types.NamespacedName{Name: inputPod.GetName(), Namespace: inputPod.GetNamespace()}, pod)
		if errPod != nil {
			// This could be a connection error so we want to retry.
			return false, nil
		}
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady {
				if condition.Status == corev1.ConditionTrue {
					return true, nil
				}
			}
		}
		return false, nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to check the pod: %v", err)
	}

	req := r.restClient.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(inputPod.GetName()).
		Namespace(inputPod.GetNamespace()).
		SubResource("exec")

	option := &corev1.PodExecOptions{
		Command: command,
		Stdin:   false,
		Stdout:  true,
		Stderr:  true,
		TTY:     true,
	}
	req.VersionedParams(
		option,
		scheme.ParameterCodec,
	)

	exec, err := remotecommand.NewSPDYExecutor(r.config, "POST", req.URL())
	if err != nil {
		return "", errors.Wrap(err, "failed to init the executor")
	}

	var (
		execOut bytes.Buffer
		execErr bytes.Buffer
	)

	err = exec.Stream(remotecommand.StreamOptions{
		Stdout: &execOut,
		Stderr: &execErr,
		Tty:    false,
	})

	if err != nil {
		return "", errors.Wrap(err, "could not execute the command")
	}

	if execErr.Len() > 0 {
		return "", errors.Wrapf(err, "error executing the command, maybe not available in the version: %s", execErr.String())
	}

	return execOut.String(), nil
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
