package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func compareResources(t *testing.T, clusterResources, deployResources corev1.ResourceRequirements) {
	require.True(t, clusterResources.Limits.Cpu().Equal(*deployResources.Limits.Cpu()))
	require.True(t, clusterResources.Limits.Memory().Equal(*deployResources.Limits.Memory()))
	require.True(t, clusterResources.Requests.Cpu().Equal(*deployResources.Requests.Cpu()))
	require.True(t, clusterResources.Requests.Memory().Equal(*deployResources.Requests.Memory()))
}

func checkResourcesAndReplicas(t *testing.T,
	k8sClient client.Client,
	mmKey types.NamespacedName,
	clusterResources corev1.ResourceRequirements,
	replicas int32,
) {
	t.Log("checking deployment replicas & resources")

	var mmDeployment appsv1.Deployment
	err := k8sClient.Get(context.TODO(), mmKey, &mmDeployment)
	require.NoError(t, err)

	require.Equal(t, replicas, *mmDeployment.Spec.Replicas)
	compareResources(t, clusterResources, mmDeployment.Spec.Template.Spec.Containers[0].Resources)
}

func waitForDeploymentWithDefaultTimes(t *testing.T, k8sClient client.Client, nn types.NamespacedName, replicas int) error {
	defaultRetryInterval := 5 * time.Second
	defaultTimeout := 3 * time.Minute
	return waitForDeployment(t, k8sClient, nn, replicas, defaultRetryInterval, defaultTimeout)
}

func waitForDeployment(t *testing.T,
	k8sClient client.Client,
	nn types.NamespacedName,
	replicas int,
	retryInterval, timeout time.Duration,
) error {
	ctx := context.TODO()

	err := wait.PollUntilContextTimeout(ctx, retryInterval, timeout, false, func(context.Context) (done bool, err error) {
		var deployment appsv1.Deployment
		err = k8sClient.Get(context.TODO(), nn, &deployment)
		if err != nil {
			if apierrors.IsNotFound(err) {
				t.Logf("Waiting for availability of deployment: %s", nn.String())
				return false, nil
			}
			return false, err
		}

		if int(deployment.Status.AvailableReplicas) >= replicas {
			return true, nil
		}
		t.Logf("Waiting for full availability of %s deployment (%d/%d)",
			nn.Name, deployment.Status.AvailableReplicas, replicas,
		)

		return false, nil
	})
	if err != nil {
		return err
	}

	t.Logf("Deployment %s available with %d replicas", nn.Name, replicas)
	return nil
}
