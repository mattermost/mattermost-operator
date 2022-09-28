package e2e

import (
	"context"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
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
