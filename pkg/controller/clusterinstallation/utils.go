package clusterinstallation

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// checkClusterInstallation checks the health and correctness of the k8s
// objects that make up a MattermostInstallation.
//
// NOTE: this is a vital health check. Every reconciliation loop should run this
// check at the very end to ensure that everything in the installation is as it
// should be. Over time, more types of checks should be added here as needed.
func (r *ReconcileClusterInstallation) checkClusterInstallation(mattermost *mattermostv1alpha1.ClusterInstallation) (*mattermostv1alpha1.ClusterInstallationStatus, error) {
	pods := &v1.PodList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
	}
	sel := mattermostv1alpha1.LabelsForClusterInstallation(mattermost.Name)
	opts := &client.ListOptions{LabelSelector: labels.SelectorFromSet(sel)}

	err := r.client.List(context.TODO(), opts, pods)
	if err != nil {
		return nil, err
	}

	for _, pod := range pods.Items {
		if pod.Status.Phase != v1.PodRunning || pod.DeletionTimestamp != nil {
			return nil, fmt.Errorf("mattermost pod %q is terminating", pod.Name)
		}
		if len(pod.Spec.Containers) == 0 {
			return nil, fmt.Errorf("mattermost pod %q has no containers", pod.Name)
		}
		if pod.Spec.Containers[0].Image != mattermost.GetImageName() {
			return nil, fmt.Errorf("mattermost pod %q is running incorrect version", pod.Name)
		}
	}

	if int32(len(pods.Items)) != mattermost.Spec.Replicas {
		return nil, fmt.Errorf("found %d pods, but wanted %d", len(pods.Items), mattermost.Spec.Replicas)
	}

	return &mattermostv1alpha1.ClusterInstallationStatus{
		State:    mattermostv1alpha1.Stable,
		Image:    mattermost.Spec.Image,
		Version:  mattermost.Spec.Version,
		Replicas: mattermost.Spec.Replicas,
	}, nil
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
