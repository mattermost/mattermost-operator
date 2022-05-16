package healthcheck

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	v1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type HealthChecker struct {
	// apiReader is API server client. It is important that it does not use cache
	// as it may cause health checks to not be accurate.
	apiReader client.Reader
	logger    logr.Logger

	listOptions []client.ListOption
}

func NewHealthChecker(apiReader client.Reader, listOpts []client.ListOption, logger logr.Logger) *HealthChecker {
	return &HealthChecker{
		apiReader:   apiReader,
		logger:      logger.WithName("health-check"),
		listOptions: listOpts,
	}
}

type PodRolloutStatus struct {
	Replicas        int32
	UpdatedReplicas int32
}

func (hc *HealthChecker) CheckPodsRollOut(desiredImage string) (PodRolloutStatus, error) {
	pods := &corev1.PodList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
	}

	err := hc.apiReader.List(context.TODO(), pods, hc.listOptions...)
	if err != nil {
		return PodRolloutStatus{}, errors.Wrap(err, "unable to get pod list")
	}

	var replicas = int32(len(pods.Items))
	var updatedReplicas int32

	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning || pod.DeletionTimestamp != nil {
			if pod.DeletionTimestamp != nil {
				hc.logger.Info(fmt.Sprintf("mattermost pod not terminated: pod %s is in state '%s'", pod.Name, pod.Status.Phase))
				continue
			}
			hc.logger.Info(fmt.Sprintf("mattermost pod not ready: pod %s is in state '%s'", pod.Name, pod.Status.Phase))
			continue
		}
		if len(pod.Spec.Containers) == 0 {
			hc.logger.Info(fmt.Sprintf("mattermost pod %s has no containers", pod.Name))
			continue
		}
		if v1beta.GetMattermostAppContainer(pod.Spec.Containers).Image != desiredImage {
			hc.logger.Info(fmt.Sprintf("mattermost pod %s is running incorrect image", pod.Name))
			continue
		}
		if !isPodReady(pod) {
			hc.logger.Info(fmt.Sprintf("mattermost pod %s is not ready", pod.Name))
			continue
		}

		updatedReplicas++
	}

	return PodRolloutStatus{Replicas: replicas, UpdatedReplicas: updatedReplicas}, nil
}

func (hc *HealthChecker) AssertDeploymentRolloutStarted(name, namespace string) error {
	// To prevent race condition that new pods did not start rolling out and
	// old ones are still ready, we need to check if Deployment was picked up by controller.
	deployment := &appsv1.Deployment{}
	deploymentKey := types.NamespacedName{Name: name, Namespace: namespace}
	err := hc.apiReader.Get(context.TODO(), deploymentKey, deployment)
	if err != nil {
		return errors.Wrap(err, "failed to get deployment")
	}
	if deployment.Generation != deployment.Status.ObservedGeneration {
		return errors.New("mattermost deployment not yet picked up by the Deployment controller")
	}

	// We check if new ReplicaSet was created and it was observed by the controller
	// to guarantee that new pods are created.
	replicaSets := &appsv1.ReplicaSetList{}
	err = hc.apiReader.List(context.TODO(), replicaSets, hc.listOptions...)
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
		return errors.New("replicaSet did not start rolling pods")
	}

	return nil
}

func (hc *HealthChecker) CheckServiceLoadBalancer() (string, error) {
	svc := &corev1.ServiceList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
	}

	err := hc.apiReader.List(context.TODO(), svc, hc.listOptions...)
	if err != nil {
		return "", errors.Wrap(err, "unable to get service list")
	}
	if len(svc.Items) != 1 {
		return "", fmt.Errorf("expected one service, but found %d", len(svc.Items))
	}
	if svc.Items[0].Status.LoadBalancer.Ingress == nil {
		return "", errors.New("waiting for the Load Balancer to be active")
	}

	lbIngress := &svc.Items[0].Status.LoadBalancer.Ingress[0]
	if lbIngress.Hostname != "" {
		return lbIngress.Hostname, nil
	}

	return lbIngress.IP, nil
}

func (hc *HealthChecker) CheckIngressLoadBalancer() (string, error) {
	ingress := &networkingv1.IngressList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Ingress",
			APIVersion: "v1",
		},
	}
	err := hc.apiReader.List(context.TODO(), ingress, hc.listOptions...)
	if err != nil {
		return "", errors.Wrap(err, "unable to get ingress list")
	}
	if len(ingress.Items) != 1 {
		return "", fmt.Errorf("expected one ingress, but found %d", len(ingress.Items))
	}

	return ingress.Items[0].Spec.Rules[0].Host, nil
}

func isPodReady(pod corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			if condition.Status == corev1.ConditionTrue {
				return true
			}
			return false
		}
	}
	return false
}

func getRevision(annotations map[string]string) string {
	if annotations == nil {
		return ""
	}
	return annotations["deployment.kubernetes.io/revision"]
}
