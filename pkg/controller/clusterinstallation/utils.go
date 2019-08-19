package clusterinstallation

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	v1beta1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *ReconcileClusterInstallation) handleCheckClusterInstallation(mattermost *mattermostv1alpha1.ClusterInstallation) (mattermostv1alpha1.ClusterInstallationStatus, error) {
	if !mattermost.Spec.BlueGreen.Enable {
		return r.checkClusterInstallation(
			mattermost.Name,
			mattermost.GetImageName(),
			mattermost.Spec.Image,
			mattermost.Spec.Version,
			mattermost.Spec.Replicas,
			mattermost.Spec.UseServiceLoadBalancer,
		)
	}

	// BlueGreen is a bit tricky. To properly check for errors and also to return
	// the correct status, we should check both and then manually return status.
	blueStatus, blueErr := r.checkClusterInstallation(
		mattermost.Spec.BlueGreen.BlueInstallationName,
		mattermost.GetBlueGreenImageName(mattermostv1alpha1.BlueName),
		mattermost.Spec.Image, mattermost.Spec.BlueGreen.BlueVersion,
		mattermost.Spec.Replicas,
		mattermost.Spec.UseServiceLoadBalancer,
	)
	greenStatus, greenErr := r.checkClusterInstallation(
		mattermost.Spec.BlueGreen.GreenInstallationName,
		mattermost.GetBlueGreenImageName(mattermostv1alpha1.GreenName),
		mattermost.Spec.Image, mattermost.Spec.BlueGreen.GreenVersion,
		mattermost.Spec.Replicas,
		mattermost.Spec.UseServiceLoadBalancer,
	)

	var status mattermostv1alpha1.ClusterInstallationStatus
	if mattermost.Spec.BlueGreen.ProductionDeployment == mattermostv1alpha1.BlueName {
		status = blueStatus
	} else {
		status = greenStatus
	}

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
func (r *ReconcileClusterInstallation) checkClusterInstallation(name, imageName, image, version string, replicas int32, useServiceLoadBalancer bool) (mattermostv1alpha1.ClusterInstallationStatus, error) {
	status := mattermostv1alpha1.ClusterInstallationStatus{
		State:           mattermostv1alpha1.Reconciling,
		Replicas:        0,
		UpdatedReplicas: 0,
	}

	sel := mattermostv1alpha1.ClusterInstallationLabels(name)
	opts := &client.ListOptions{LabelSelector: labels.SelectorFromSet(sel)}

	pods := &corev1.PodList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
	}

	err := r.client.List(context.TODO(), opts, pods)
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
		status.UpdatedReplicas++
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
		err := r.client.List(context.TODO(), opts, svc)
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
		err := r.client.List(context.TODO(), opts, ingress)
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
		return errors.Wrap(err, "Error getting secret")
	}

	for key := range foundSecret.Data {
		if keyName == key {
			return nil
		}
	}

	msg := fmt.Sprintf("Missing required secret data. Want: %s", keyName)
	return errors.Wrap(err, msg)
}

func (r *ReconcileClusterInstallation) updateStatus(mattermost *mattermostv1alpha1.ClusterInstallation, status mattermostv1alpha1.ClusterInstallationStatus, reqLogger logr.Logger) error {
	if !reflect.DeepEqual(mattermost.Status, status) {
		mattermost.Status = status
		err := r.client.Status().Update(context.TODO(), mattermost)
		if err != nil {
			reqLogger.Error(err, "failed to update the clusterinstallation status")
			return err
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
