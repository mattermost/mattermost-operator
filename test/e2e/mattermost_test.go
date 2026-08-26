package e2e

import (
	"context"

	operator "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	ptrUtil "github.com/mattermost/mattermost-operator/pkg/utils"
	operatortest "github.com/mattermost/mattermost-operator/test"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"testing"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// retryInterval is an interval between check attempts
	retryInterval = time.Second * 30
	// timeout to wait for k8s objects to be created
	timeout = time.Second * 900

	mmNamespace = "mattermost-operator"
)

func TestMattermost(t *testing.T) {

	k8sTypedClient, err := kubernetes.NewForConfig(cfg)
	require.NoError(t, err)

	t.Run("mattermost operator ready", func(t *testing.T) {
		err = waitForDeployment(t, k8sTypedClient, mmNamespace, "mattermost-operator", 1, retryInterval, timeout)
		require.NoError(t, err)
	})

	cleanupPrereqs, err := SetupMattermostPrerequisites(context.TODO(), k8sClient, mmNamespace)
	require.NoError(t, err)
	defer cleanupPrereqs()

	t.Run("mattermost scale test", func(t *testing.T) {
		mattermostScaleTest(t, k8sClient, k8sTypedClient)
	})

	t.Run("mattermost upgrade test", func(t *testing.T) {
		mattermostUpgradeTest(t, k8sClient, k8sTypedClient)
	})

}

func mattermostScaleTest(t *testing.T, k8sClient client.Client, k8sTypedClient kubernetes.Interface) {
	// create ClusterInstallation custom resource
	exampleMattermost := &operator.Mattermost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-mm",
			Namespace: mmNamespace,
		},
		Spec: operator.MattermostSpec{
			IngressName: "test-example.mattermost.dev",
			Replicas:    ptrUtil.NewInt32(1),
			Scheduling: operator.Scheduling{
				Resources: testMattermostResources(),
			},
			FileStore: testFileStoreConfig(1),
			Database:  testDatabaseConfig(1),
		},
	}
	mmNamespaceName := types.NamespacedName{Namespace: exampleMattermost.Namespace, Name: exampleMattermost.Name}

	err := k8sClient.Create(context.TODO(), exampleMattermost)
	require.NoError(t, err)

	require.NoError(t, err)

	// wait for test-mm to reach 1 replicas
	err = waitForDeployment(t, k8sTypedClient, mmNamespace, "test-mm", 1, retryInterval, timeout)
	require.NoError(t, err)

	err = k8sClient.Get(context.TODO(), types.NamespacedName{Name: "test-mm", Namespace: mmNamespace}, exampleMattermost)
	require.NoError(t, err)

	exampleMattermost.Spec.Replicas = ptrUtil.NewInt32(2)
	err = k8sClient.Update(context.TODO(), exampleMattermost)
	require.NoError(t, err)

	// wait for test-mm to reach 2 replicas
	err = waitForDeployment(t, k8sTypedClient, mmNamespace, "test-mm", 2, retryInterval, timeout)
	require.NoError(t, err)

	err = WaitForMattermostStable(t, k8sClient, mmNamespaceName, timeout)
	require.NoError(t, err)

	// scale down again
	err = k8sClient.Get(context.TODO(), types.NamespacedName{Name: "test-mm", Namespace: mmNamespace}, exampleMattermost)
	require.NoError(t, err)

	exampleMattermost.Spec.Replicas = ptrUtil.NewInt32(1)
	err = k8sClient.Update(context.TODO(), exampleMattermost)
	require.NoError(t, err)

	// wait for test-mm to reach 1 replicas
	err = waitForDeployment(t, k8sTypedClient, mmNamespace, "test-mm", 1, retryInterval, timeout)
	require.NoError(t, err)

	err = WaitForMattermostStable(t, k8sClient, mmNamespaceName, timeout)
	require.NoError(t, err)

	err = k8sClient.Delete(context.TODO(), exampleMattermost)
	require.NoError(t, err)
}

func mattermostUpgradeTest(t *testing.T, k8sClient client.Client, k8sTypedClient kubernetes.Interface) {
	testName := "test-mm2"

	exampleMattermost := &operator.Mattermost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testName,
			Namespace: mmNamespace,
		},
		Spec: operator.MattermostSpec{
			Image:       "mattermost/mattermost-enterprise-edition",
			Version:     operatortest.PreviousStableMattermostVersion,
			IngressName: "test-example2.mattermost.dev",
			Replicas:    ptrUtil.NewInt32(1),
			Scheduling: operator.Scheduling{
				Resources: testMattermostResources(),
			},
			FileStore: testFileStoreConfig(1),
			Database:  testDatabaseConfig(1),
		},
	}
	mmNamespaceName := types.NamespacedName{Namespace: exampleMattermost.Namespace, Name: exampleMattermost.Name}

	err := k8sClient.Create(context.TODO(), exampleMattermost)
	require.NoError(t, err)

	require.NoError(t, err)

	// wait for test-mm2 to reach 1 replicas
	err = waitForDeployment(t, k8sTypedClient, mmNamespace, testName, 1, retryInterval, timeout)
	require.NoError(t, err)

	err = k8sClient.Get(context.TODO(), types.NamespacedName{Name: testName, Namespace: mmNamespace}, exampleMattermost)
	require.NoError(t, err)

	// Get the current pod
	pods := corev1.PodList{}
	listOptions := []client.ListOption{
		client.InNamespace(mmNamespace),
		client.MatchingLabels(map[string]string{"app": "mattermost", operator.ClusterResourceLabel: testName}),
	}

	err = k8sClient.List(context.TODO(), &pods, listOptions...)
	require.NoError(t, err)
	require.Equal(t, 1, len(pods.Items))

	mmOldPod := &corev1.Pod{}
	err = k8sClient.Get(context.TODO(), types.NamespacedName{Name: pods.Items[0].Name, Namespace: mmNamespace}, mmOldPod)
	require.NoError(t, err)

	// Apply the new version
	exampleMattermost.Spec.Version = operatortest.LatestStableMattermostVersion
	err = k8sClient.Update(context.TODO(), exampleMattermost)
	require.NoError(t, err)

	// Wait for this pod be terminated
	err = waitForDeletion(t, k8sClient, mmOldPod, retryInterval, timeout)
	require.NoError(t, err)

	// wait for deployment be completed
	err = waitForDeployment(t, k8sTypedClient, mmNamespace, testName, 1, retryInterval, timeout)
	require.NoError(t, err)

	err = WaitForMattermostStable(t, k8sClient, mmNamespaceName, timeout)
	require.NoError(t, err)

	var updatedMattermost operator.Mattermost
	err = k8sClient.Get(context.TODO(), types.NamespacedName{Name: testName, Namespace: mmNamespace}, &updatedMattermost)
	require.NoError(t, err)
	require.Equal(t, "mattermost/mattermost-enterprise-edition", updatedMattermost.Status.Image)
	require.Equal(t, operatortest.LatestStableMattermostVersion, updatedMattermost.Status.Version)

	var mmDeployment appsv1.Deployment
	err = k8sClient.Get(context.TODO(), types.NamespacedName{Name: testName, Namespace: mmNamespace}, &mmDeployment)
	require.NoError(t, err)
	require.Equal(t, "mattermost/mattermost-enterprise-edition:"+operatortest.LatestStableMattermostVersion, mmDeployment.Spec.Template.Spec.Containers[0].Image)

	err = k8sClient.Delete(context.TODO(), exampleMattermost)
	require.NoError(t, err)
}

func testMattermostResources() corev1.ResourceRequirements {
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("100Mi"),
		},
	}
}

// testFileStoreConfig returns an external file store pointing at the standalone
// MinIO from resources/minio.yaml, matching what the e2e-external suite uses.
//
// A local (PVC backed) file store is not usable here. New local file stores
// request ReadWriteMany, which the local-path provisioner in a kind cluster
// cannot satisfy, and even forced to ReadWriteOnce the scale test below runs two
// replicas, which cannot share a single RWO volume.
func testFileStoreConfig(_ int32) operator.FileStore {
	return operator.FileStore{
		External: &operator.ExternalFileStore{
			URL:    "minio:9000",
			Bucket: "test-bucket",
			Secret: "file-store-credentials",
		},
	}
}

func testDatabaseConfig(_ int32) operator.Database {
	// Operator-managed MySQL no longer exists, so the suite uses an external
	// database. Postgres comes from resources/postgres.yaml and the secret from
	// resources/mm-secrets.yaml, the same fixtures the e2e-external suite applies.
	return operator.Database{
		External: &operator.ExternalDatabase{Secret: "db-credentials"},
	}
}
