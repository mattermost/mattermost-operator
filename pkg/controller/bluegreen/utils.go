package bluegreen

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	v1beta1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"github.com/pkg/errors"
	errs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *ReconcileBlueGreen) checkBlueGreenStatus(blueGreen *mattermostv1alpha1.BlueGreen, mattermost *mattermostv1alpha1.ClusterInstallation) (mattermostv1alpha1.BlueGreenStatus, error) {
	status := mattermostv1alpha1.BlueGreenStatus{
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
	sel := mattermostv1alpha1.BlueGreenLabels(blueGreen.Name)
	opts := &client.ListOptions{LabelSelector: labels.SelectorFromSet(sel)}

	err := r.client.List(context.TODO(), opts, pods)
	if err != nil {
		return status, errors.Wrap(err, "unable to get pod list")
	}

	status.Replicas = int32(len(pods.Items))

	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning || pod.DeletionTimestamp != nil {
			return status, fmt.Errorf("Μattermost pod %s is in state '%s'", pod.Name, pod.Status.Phase)
		}
		if len(pod.Spec.Containers) == 0 {
			return status, fmt.Errorf("Μattermost pod %s has no containers", pod.Name)
		}
		if pod.Spec.Containers[0].Image != blueGreen.GetImageName(mattermost) {
			return status, fmt.Errorf("Μattermost pod %s is running incorrect image", pod.Name)
		}
		status.UpdatedReplicas++
	}

	if int32(len(pods.Items)) != mattermost.Spec.Replicas {
		return status, fmt.Errorf("found %d pods, but wanted %d", len(pods.Items), mattermost.Spec.Replicas)
	}

	status.Image = mattermost.Spec.Image
	status.Version = blueGreen.Spec.Version

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

func (r *ReconcileBlueGreen) updateStatus(blueGreen *mattermostv1alpha1.BlueGreen, status mattermostv1alpha1.BlueGreenStatus, reqLogger logr.Logger) error {
	if !reflect.DeepEqual(blueGreen.Status, status) {
		blueGreen.Status = status
		err := r.client.Status().Update(context.TODO(), blueGreen)
		if err != nil {
			reqLogger.Error(err, "failed to update the BlueGreen status")
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

func (r *ReconcileBlueGreen) checkSecret(secretName, keyName, namespace string) error {
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

func (r *ReconcileBlueGreen) getMySQLSecrets(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) (string, error) {
	dbSecretName := fmt.Sprintf("%s-mysql-root-password", mattermost.Name)
	dbSecret := &corev1.Secret{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: dbSecretName, Namespace: mattermost.Namespace}, dbSecret)
	if err != nil && errs.IsNotFound(err) {
		reqLogger.Error(err, "My sql secret does not exist. Check Mattermost Installation.")
		return "", err
	} else if err != nil {
		reqLogger.Error(err, "Failed to check if mysql secret exists")
		return "", err
	}
	return string(dbSecret.Data["PASSWORD"]), nil
}

func (r *ReconcileBlueGreen) getMinioService(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) (string, error) {
	minioServiceName := fmt.Sprintf("%s-minio", mattermost.Name)
	minioService := &corev1.Service{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: minioServiceName, Namespace: mattermost.Namespace}, minioService)
	if err != nil {
		return "", err
	}

	connectionString := fmt.Sprintf("%s.%s.svc.cluster.local:%d", minioService.Name, mattermost.Namespace, minioService.Spec.Ports[0].Port)
	return connectionString, nil
}
