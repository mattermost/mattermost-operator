package healthcheck

import (
	"context"
	"fmt"

	v1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"

	"github.com/go-logr/logr"
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

// CheckPodsRollOut checks if pods are running with new image.
// Deprecated: It is maintained only for purpose of old migration code.
// CheckReplicaSetRollout should be sufficient for other cases.
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
		mmContainer := v1beta.GetMattermostAppContainer(pod.Spec.Containers)
		if mmContainer == nil {
			hc.logger.Info(fmt.Sprintf("mattermost container not found in the pod %s", pod.Name))
			continue
		}
		if mmContainer.Image != desiredImage {
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

// CheckReplicaSetRollout checks if new deployment version was
// already rolled out.
func (hc *HealthChecker) CheckReplicaSetRollout(name, namespace string) (PodRolloutStatus, error) {
	// To prevent race condition that new pods did not start rolling out and
	// old ones are still ready, we need to check if Deployment was picked up by controller.
	deployment := &appsv1.Deployment{}
	deploymentKey := types.NamespacedName{Name: name, Namespace: namespace}
	err := hc.apiReader.Get(context.TODO(), deploymentKey, deployment)
	if err != nil {
		return PodRolloutStatus{}, errors.Wrap(err, "failed to get deployment")
	}
	if deployment.Generation != deployment.Status.ObservedGeneration {
		return PodRolloutStatus{}, errors.New("mattermost deployment not yet picked up by the Deployment controller")
	}

	// We check if new ReplicaSet was created and it was observed by the controller
	// to guarantee that new pods are created.
	replicaSets := &appsv1.ReplicaSetList{}
	err = hc.apiReader.List(context.TODO(), replicaSets, hc.listOptions...)
	if err != nil {
		return PodRolloutStatus{}, errors.Wrap(err, "failed to list replicaSets")
	}

	var replicaSet *appsv1.ReplicaSet
	for _, rep := range replicaSets.Items {
		if getRevision(rep.Annotations) == getRevision(deployment.Annotations) {
			if rep.Status.ObservedGeneration > 0 {
				replicaSet = &rep
				break
			}
		}
	}
	if replicaSet == nil {
		return PodRolloutStatus{}, errors.New("replicaSet did not start rolling pods")
	}

	return PodRolloutStatus{
		// To be compatible with previous behavior we take the number of
		// replicas from deployment as there might be still old
		// ReplicaSet instance, but we only care about updated replicas
		// that are available, therefore we check most current
		// ReplicaSet status.
		Replicas:        deployment.Status.Replicas,
		UpdatedReplicas: replicaSet.Status.AvailableReplicas,
	}, nil
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
			return condition.Status == corev1.ConditionTrue
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
