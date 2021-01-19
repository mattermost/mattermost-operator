package e2e

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"

	operator "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"

	mysqlOperator "github.com/presslabs/mysql-operator/pkg/apis/mysql/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func waitForMySQLStatusReady(t *testing.T, dynclient client.Client, namespace, name string, replicas int, retryInterval, timeout time.Duration) error {
	mysql := &mysqlOperator.MysqlCluster{}
	err := wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		errClient := dynclient.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, mysql)
		if errClient != nil {
			return false, errClient
		}

		if mysql.Status.ReadyNodes == replicas {
			return true, nil
		}
		t.Logf("Waiting for MySQL cluster ReadyNodes: %d\n", mysql.Status.ReadyNodes)
		return false, nil
	})
	if err != nil {
		return err
	}
	t.Logf("All MySQL cluster nodes (%d) are ready!\n", mysql.Status.ReadyNodes)
	return nil
}

func waitForReconcilicationComplete(t *testing.T, dynclient client.Client, namespace, name string, retryInterval, timeout time.Duration) error {
	newMattermost := &operator.Mattermost{}
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

func waitForDeployment(t *testing.T, kubeclient kubernetes.Interface, namespace, name string, replicas int,
	retryInterval, timeout time.Duration) error {
	err := wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		deployment, err := kubeclient.AppsV1().Deployments(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				t.Logf("Waiting for availability of Deployment: %s in Namespace: %s \n", name, namespace)
				return false, nil
			}
			return false, err
		}

		if int(deployment.Status.AvailableReplicas) >= replicas {
			return true, nil
		}
		t.Logf("Waiting for full availability of %s deployment (%d/%d)\n", name,
			deployment.Status.AvailableReplicas, replicas)
		return false, nil
	})
	if err != nil {
		return err
	}
	t.Logf("Deployment available (%d/%d)\n", replicas, replicas)
	return nil
}

func waitForDeletion(t *testing.T, dynclient client.Client, obj runtime.Object, retryInterval,
	timeout time.Duration) error {
	key, err := client.ObjectKeyFromObject(obj)
	if err != nil {
		return err
	}

	kind := obj.GetObjectKind().GroupVersionKind().Kind
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	err = wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		err = dynclient.Get(ctx, key, obj)
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		t.Logf("Waiting for %s %s to be deleted\n", kind, key)
		return false, nil
	})
	if err != nil {
		return err
	}
	t.Logf("%s %s was deleted\n", kind, key)
	return nil
}
