package e2e

import (
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"testing"
)

func compareResources(t *testing.T, clusterResources, deployResources corev1.ResourceRequirements) {
	require.True(t, clusterResources.Limits.Cpu().Equal(*deployResources.Limits.Cpu()))
	require.True(t, clusterResources.Limits.Memory().Equal(*deployResources.Limits.Memory()))
	require.True(t, clusterResources.Requests.Cpu().Equal(*deployResources.Requests.Cpu()))
	require.True(t, clusterResources.Requests.Memory().Equal(*deployResources.Requests.Memory()))
}
