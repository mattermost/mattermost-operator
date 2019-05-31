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

// checkClusterInstallation checks the health and correctness of the k8s
// objects that make up a MattermostInstallation.
//
// NOTE: this is a vital health check. Every reconciliation loop should run this
// check at the very end to ensure that everything in the installation is as it
// should be. Over time, more types of checks should be added here as needed.
func (r *ReconcileClusterInstallation) checkClusterInstallation(mattermost *mattermostv1alpha1.ClusterInstallation) (*mattermostv1alpha1.ClusterInstallationStatus, error) {
	pods := &corev1.PodList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
	}
	sel := mattermostv1alpha1.ClusterInstallationLabels(mattermost.Name)
	opts := &client.ListOptions{LabelSelector: labels.SelectorFromSet(sel)}

	err := r.client.List(context.TODO(), opts, pods)
	if err != nil {
		return nil, err
	}

	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning || pod.DeletionTimestamp != nil {
			return nil, fmt.Errorf("mattermost pod %s is in state '%s'", pod.Name, pod.Status.Phase)
		}
		if len(pod.Spec.Containers) == 0 {
			return nil, fmt.Errorf("mattermost pod %s has no containers", pod.Name)
		}
		if pod.Spec.Containers[0].Image != mattermost.GetImageName() {
			return nil, fmt.Errorf("mattermost pod %s is running incorrect image", pod.Name)
		}
	}

	if int32(len(pods.Items)) != mattermost.Spec.Replicas {
		return nil, fmt.Errorf("found %d pods, but wanted %d", len(pods.Items), mattermost.Spec.Replicas)
	}

	var mmEndpoint string
	if mattermost.Spec.UseServiceLoadBalancer {
		svc := &corev1.ServiceList{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Service",
				APIVersion: "v1",
			},
		}
		err := r.client.List(context.TODO(), opts, svc)
		if err != nil {
			return nil, err
		}
		if len(svc.Items) > 1 {
			return nil, fmt.Errorf("should return just one service but returned %d", len(svc.Items))
		}
		if svc.Items[0].Status.LoadBalancer.Ingress == nil {
			return nil, fmt.Errorf("waiting for the Load Balancers be active")
		}
		if svc.Items[0].Status.LoadBalancer.Ingress[0].Hostname != "" {
			mmEndpoint = svc.Items[0].Status.LoadBalancer.Ingress[0].Hostname
		} else if svc.Items[0].Status.LoadBalancer.Ingress[0].IP != "" {
			mmEndpoint = svc.Items[0].Status.LoadBalancer.Ingress[0].IP
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
			return nil, err
		}
		if len(ingress.Items) > 1 {
			return nil, fmt.Errorf("should return just one ingress but returned %d", len(ingress.Items))
		}
		mmEndpoint = ingress.Items[0].Spec.Rules[0].Host
	}

	return &mattermostv1alpha1.ClusterInstallationStatus{
		State:    mattermostv1alpha1.Stable,
		Image:    mattermost.Spec.Image,
		Version:  mattermost.Spec.Version,
		Replicas: mattermost.Spec.Replicas,
		Endpoint: mmEndpoint,
	}, nil
}

func (r *ReconcileClusterInstallation) checkSecret(secretName, namespace string) error {
	foundSecret := &corev1.Secret{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: namespace}, foundSecret)
	if err != nil {
		return errors.Wrap(err, "Error getting secret")
	}

	return nil
}

func (r *ReconcileClusterInstallation) updateStatus(mattermost *mattermostv1alpha1.ClusterInstallation, status mattermostv1alpha1.ClusterInstallationStatus, reqLogger logr.Logger) error {
	if !reflect.DeepEqual(mattermost.Status, status) {
		reqLogger.Info(fmt.Sprintf("Updating status"),
			"Old", fmt.Sprintf("%+v", mattermost.Status),
			"New", fmt.Sprintf("%+v", status),
		)
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
