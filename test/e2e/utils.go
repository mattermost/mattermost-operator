package e2e

import (
	"context"
	"testing"
	"time"

	operator "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func waitForReconcilicationComplete(t *testing.T, dynclient client.Client, namespace, name string, retryInterval, timeout time.Duration) error {
	newMattermost := &operator.ClusterInstallation{}
	err := wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		errClient := dynclient.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, newMattermost)
		if errClient != nil {
			return false, errClient
		}

		if newMattermost.Status.State == "stable" {
			return true, nil
		}
		t.Logf("Waiting for Reconcilication finish (Status:%s)\n", newMattermost.Status.State)
		return false, nil
	})
	if err != nil {
		return err
	}
	t.Logf("Reconcilication completed (%s)\n", newMattermost.Status.State)
	return nil
}

func waitForStatefulSet(t *testing.T, dynclient client.Client, namespace, name string, replicas int, retryInterval, timeout time.Duration) error {
	statefulset := &appsv1.StatefulSet{}
	err := wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		err = dynclient.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, statefulset)
		if err != nil {
			if apierrors.IsNotFound(err) {
				t.Logf("Waiting for availability of %s StatefulSet\n", name)
				return false, nil
			}
			return false, err
		}

		if statefulset.Status.ReadyReplicas == statefulset.Status.Replicas && replicas == int(statefulset.Status.Replicas) {
			return true, nil
		}
		t.Logf("Waiting for full availability of %s StatefulSet with %v of %v replicas available\n", name, statefulset.Status.ReadyReplicas, statefulset.Status.Replicas)
		return false, nil
	})

	if err != nil {
		return err
	}
	t.Logf("%s Pod available\n", name)
	return nil
}
