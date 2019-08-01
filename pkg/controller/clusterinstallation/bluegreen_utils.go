package clusterinstallation

import (
	"context"
	"fmt"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	v1beta1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// checkGreenInstallation checks the health and correctness of the k8s
// objects that make up a GreenInstallation.
//
// NOTE: this is a vital health check. Every reconciliation loop should run this
// check at the very end to ensure that everything in the installation is as it
// should be. Over time, more types of checks should be added here as needed.
func (r *ReconcileClusterInstallation) checkGreenInstallation(mattermost *mattermostv1alpha1.ClusterInstallation) (mattermostv1alpha1.ClusterInstallationStatus, error) {
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
	sel := mattermostv1alpha1.ClusterInstallationLabels(mattermost.Spec.BlueGreen.GreenInstallationName)
	opts := &client.ListOptions{LabelSelector: labels.SelectorFromSet(sel)}

	err := r.client.List(context.TODO(), opts, pods)
	if err != nil {
		return status, errors.Wrap(err, "unable to get pod list")
	}

	status.Replicas = int32(len(pods.Items))

	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning || pod.DeletionTimestamp != nil {
			return status, fmt.Errorf("mattermost green pod %s is in state '%s'", pod.Name, pod.Status.Phase)
		}
		if len(pod.Spec.Containers) == 0 {
			return status, fmt.Errorf("mattermost green pod %s has no containers", pod.Name)
		}
		if pod.Spec.Containers[0].Image != mattermost.GetBlueGreenImageName("green") {
			return status, fmt.Errorf("mattermost green pod %s is running incorrect image", pod.Name)
		}
		status.UpdatedReplicas++
	}

	if int32(len(pods.Items)) != mattermost.Spec.Replicas {
		return status, fmt.Errorf("found %d green pods, but wanted %d", len(pods.Items), mattermost.Spec.Replicas)
	}

	status.Image = mattermost.Spec.Image
	status.Version = mattermost.Spec.BlueGreen.GreenVersion

	status.Endpoint = "not available"
	if mattermost.Spec.UseServiceLoadBalancer {
		svc := &corev1.ServiceList{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Service",
				APIVersion: "v1",
			},
		}
		err = r.client.List(context.TODO(), opts, svc)
		if err != nil {
			return status, errors.Wrap(err, "unable to get service list")
		}
		if len(svc.Items) != 1 {
			return status, fmt.Errorf("should return just one green service, but returned %d", len(svc.Items))
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
		err = r.client.List(context.TODO(), opts, ingress)
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

// checkBlueInstallation checks the health and correctness of the k8s
// objects that make up a BlueInstallation.
//
// NOTE: this is a vital health check. Every reconciliation loop should run this
// check at the very end to ensure that everything in the installation is as it
// should be. Over time, more types of checks should be added here as needed.
func (r *ReconcileClusterInstallation) checkBlueInstallation(mattermost *mattermostv1alpha1.ClusterInstallation) (mattermostv1alpha1.ClusterInstallationStatus, error) {
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
	sel := mattermostv1alpha1.ClusterInstallationLabels(mattermost.Spec.BlueGreen.BlueInstallationName)
	opts := &client.ListOptions{LabelSelector: labels.SelectorFromSet(sel)}

	err := r.client.List(context.TODO(), opts, pods)
	if err != nil {
		return status, errors.Wrap(err, "unable to get pod list")
	}

	status.Replicas = int32(len(pods.Items))

	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning || pod.DeletionTimestamp != nil {
			return status, fmt.Errorf("mattermost blue pod %s is in state '%s'", pod.Name, pod.Status.Phase)
		}
		if len(pod.Spec.Containers) == 0 {
			return status, fmt.Errorf("mattermost blue pod %s has no containers", pod.Name)
		}
		if pod.Spec.Containers[0].Image != mattermost.GetBlueGreenImageName("blue") {
			return status, fmt.Errorf("mattermost blue pod %s is running incorrect image", pod.Name)
		}
		status.UpdatedReplicas++
	}

	if int32(len(pods.Items)) != mattermost.Spec.Replicas {
		return status, fmt.Errorf("found %d blue pods, but wanted %d", len(pods.Items), mattermost.Spec.Replicas)
	}

	status.Image = mattermost.Spec.Image
	status.Version = mattermost.Spec.BlueGreen.BlueVersion

	status.Endpoint = "not available"
	if mattermost.Spec.UseServiceLoadBalancer {
		svc := &corev1.ServiceList{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Service",
				APIVersion: "v1",
			},
		}
		err = r.client.List(context.TODO(), opts, svc)
		if err != nil {
			return status, errors.Wrap(err, "unable to get service list")
		}
		if len(svc.Items) != 1 {
			return status, fmt.Errorf("should return just one blue service, but returned %d", len(svc.Items))
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
		err = r.client.List(context.TODO(), opts, ingress)
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
