package e2e

import (
	"context"
	"testing"
	"time"

	operator "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func waitForReconcilicationComplete(t *testing.T, dynclient client.Client, namespace, name string, retryInterval, timeout time.Duration) error {
	newMattermost := &operator.ClusterInstallation{}
	err := wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		errClient := dynclient.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, newMattermost)
		if errClient != nil {
			return false, err
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
