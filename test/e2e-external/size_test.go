package e2e

import (
	"context"
	"testing"

	mmv1alpha "github.com/mattermost/mattermost-operator/apis/mattermost/v1alpha1"
	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	"github.com/mattermost/mattermost-operator/test/e2e"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// TestMattermostSize checks defaulting & updating replicas & resources from size.
func TestMattermostSize(t *testing.T) {
	namespace := "e2e-test-size"
	name := "test-mm"
	mmNamespaceName := types.NamespacedName{Namespace: namespace, Name: name}

	testEnv, setupErr := SetupTestEnv(k8sClient, namespace)
	require.NoError(t, setupErr)
	defer testEnv.CleanupFunc()

	mattermost := &mmv1beta.Mattermost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: mmv1beta.MattermostSpec{
			Ingress: &mmv1beta.Ingress{
				Host: "e2e-test-example.mattermost.dev",
			},
			FileStore: mmv1beta.FileStore{
				External: &testEnv.FileStoreConfig,
			},
			Database: mmv1beta.Database{
				External: &testEnv.DBConfig,
			},
		},
	}

	mmSize := mmv1alpha.CloudSize10String
	mattermost.Spec.Size = mmSize
	instance := e2e.NewMattermostInstance(t, k8sClient, mattermost)

	t.Run("checking mattermost replicas & resources", func(t *testing.T) {
		clusterSize, err := mmv1alpha.GetClusterSize(mmSize)
		require.NoError(t, err)

		t.Log("create and waiting for Mattermost to be stable")
		instance.CreateAndWait()

		t.Log("checking mattermost replicas & resources")
		var newMattermost mmv1beta.Mattermost
		err = k8sClient.Get(context.TODO(), mmNamespaceName, &newMattermost)
		require.NoError(t, err)
		// Size should be erased
		require.Empty(t, newMattermost.Spec.Size)
		// Check Replicas & Resources, set by Size
		require.NotNil(t, newMattermost.Spec.Replicas)
		t.Logf("mattermost replicas & resources should match %s\n", mmSize)
		require.Equal(t, clusterSize.App.Replicas, *newMattermost.Spec.Replicas)
		compareResources(t, clusterSize.App.Resources, newMattermost.Spec.Scheduling.Resources)

		// compare replicas & resources
		checkResourcesAndReplicas(t, k8sClient, mmNamespaceName, clusterSize.App.Resources, clusterSize.App.Replicas)
	})

	t.Run("updating scheduling resources in mattermost object", func(t *testing.T) {
		mmSize := mmv1alpha.CloudSize100String
		clusterSize, err := mmv1alpha.GetClusterSize(mmSize)
		require.NoError(t, err)

		t.Logf("updating scheduling resources in mattermost object with %s\n", mmSize)
		var newMattermost mmv1beta.Mattermost
		err = k8sClient.Get(context.TODO(), mmNamespaceName, &newMattermost)
		require.NoError(t, err)
		newMattermost.Spec.Scheduling.Resources = clusterSize.App.Resources
		instance.UpdateAndWait(&newMattermost)

		// compare replicas & resources
		checkResourcesAndReplicas(t, k8sClient, mmNamespaceName, clusterSize.App.Resources, 2)
	})

	t.Run("updating size in mattermost object", func(t *testing.T) {
		mmSize := mmv1alpha.Size100String
		clusterSize, err := mmv1alpha.GetClusterSize(mmSize)
		require.NoError(t, err)

		t.Logf("updating size in mattermost object with %s\n", mmSize)
		var newMattermost mmv1beta.Mattermost
		err = k8sClient.Get(context.TODO(), mmNamespaceName, &newMattermost)
		require.NoError(t, err)

		// update size in mattermost
		newMattermost.Spec.Size = mmSize
		instance.UpdateAndWait(&newMattermost)

		// compare replicas & resources
		checkResourcesAndReplicas(t, k8sClient, mmNamespaceName, clusterSize.App.Resources, clusterSize.App.Replicas)
	})

	instance.Destroy()
}
