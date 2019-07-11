package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apis "github.com/mattermost/mattermost-operator/pkg/apis"
	operator "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"

	framework "github.com/operator-framework/operator-sdk/pkg/test"
	"github.com/operator-framework/operator-sdk/pkg/test/e2eutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var (
	retryInterval        = time.Second * 5
	timeout              = time.Second * 300
	cleanupRetryInterval = time.Second * 5
	cleanupTimeout       = time.Second * 60
)

func TestMattermost(t *testing.T) {
	mattermostList := &operator.ClusterInstallationList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterInstallation",
			APIVersion: "mattermost.com/v1alpha1",
		},
	}
	err := framework.AddToFrameworkScheme(apis.AddToScheme, mattermostList)
	require.NoError(t, err)

	ctx := framework.NewTestCtx(t)
	defer ctx.Cleanup()

	t.Run("initialize cluster resrouces", func(t *testing.T) {
		err = ctx.InitializeClusterResources(&framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
		require.NoError(t, err)
	})

	namespace, err := ctx.GetNamespace()
	require.NoError(t, err)

	// get global framework variables
	f := framework.Global

	t.Run("mysql operator ready", func(t *testing.T) {
		err = e2eutil.WaitForDeployment(t, f.KubeClient, "mysql-operator", "mysql-operator", 1, retryInterval, timeout)
		require.NoError(t, err)
	})
	t.Run("minio operator ready", func(t *testing.T) {
		err = e2eutil.WaitForDeployment(t, f.KubeClient, "minio-operator", "minio-operator", 1, retryInterval, timeout)
		require.NoError(t, err)
	})
	t.Run("mattermost operator ready", func(t *testing.T) {
		err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, "mattermost-operator", 1, retryInterval, timeout)
		require.NoError(t, err)
	})

	t.Run("mattermost scale test", func(t *testing.T) {
		mattermostScaleTest(t, f, ctx)
	})

	t.Run("mattermost upgrade test", func(t *testing.T) {
		mattermostUpgradeTest(t, f, ctx)
	})
}

func mattermostScaleTest(t *testing.T, f *framework.Framework, ctx *framework.TestCtx) {
	namespace, err := ctx.GetNamespace()
	require.NoError(t, err)

	// create ClusterInstallation custom resource
	exampleMattermost := &operator.ClusterInstallation{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterInstallation",
			APIVersion: "mattermost.com/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-mm",
			Namespace: namespace,
		},
		Spec: operator.ClusterInstallationSpec{
			IngressName: "test-example.mattermost.dev",
			Replicas:    1,
			Minio: operator.Minio{
				StorageSize: "1Gi",
				Replicas:    1,
			},
			Database: operator.Database{
				StorageSize: "1Gi",
				Replicas:    1,
			},
		},
	}

	// use TestCtx's create helper to create the object and add a cleanup function for the new object
	err = f.Client.Create(context.TODO(), exampleMattermost, &framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	require.NoError(t, err)

	err = waitForStatefulSet(t, f.Client.Client, namespace, "test-mm-minio", 1, retryInterval, timeout)
	require.NoError(t, err)

	err = waitForStatefulSet(t, f.Client.Client, namespace, "db", 1, retryInterval, timeout)
	require.NoError(t, err)

	// wait for test-mm to reach 1 replicas
	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, "test-mm", 1, retryInterval, timeout)
	require.NoError(t, err)

	err = f.Client.Get(context.TODO(), types.NamespacedName{Name: "test-mm", Namespace: namespace}, exampleMattermost)
	require.NoError(t, err)

	exampleMattermost.Spec.Replicas = 2
	err = f.Client.Update(context.TODO(), exampleMattermost)
	require.NoError(t, err)

	// wait for test-mm to reach 2 replicas
	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, "test-mm", 2, retryInterval, timeout)
	require.NoError(t, err)

	err = waitForReconcilicationComplete(t, f.Client.Client, namespace, "test-mm", retryInterval, timeout)
	require.NoError(t, err)

	// scale down again
	err = f.Client.Get(context.TODO(), types.NamespacedName{Name: "test-mm", Namespace: namespace}, exampleMattermost)
	require.NoError(t, err)

	exampleMattermost.Spec.Replicas = 1
	err = f.Client.Update(context.TODO(), exampleMattermost)
	require.NoError(t, err)

	// wait for test-mm to reach 1 replicas
	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, "test-mm", 1, retryInterval, timeout)
	require.NoError(t, err)

	err = waitForReconcilicationComplete(t, f.Client.Client, namespace, "test-mm", retryInterval, timeout)
	require.NoError(t, err)

	err = f.Client.Delete(context.TODO(), exampleMattermost)
	require.NoError(t, err)
}

func mattermostUpgradeTest(t *testing.T, f *framework.Framework, ctx *framework.TestCtx) {
	namespace, err := ctx.GetNamespace()
	require.NoError(t, err)

	// create ClusterInstallation custom resource
	exampleMattermost := &operator.ClusterInstallation{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterInstallation",
			APIVersion: "mattermost.com/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-mm2",
			Namespace: namespace,
		},
		Spec: operator.ClusterInstallationSpec{
			Image:       "mattermost/mattermost-enterprise-edition",
			Version:     "5.10.0",
			IngressName: "test-example2.mattermost.dev",
			Replicas:    1,
			Minio: operator.Minio{
				StorageSize: "1Gi",
			},
			Database: operator.Database{
				StorageSize: "1Gi",
			},
		},
	}

	// use TestCtx's create helper to create the object and add a cleanup function for the new object
	err = f.Client.Create(context.TODO(), exampleMattermost, &framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	require.NoError(t, err)

	// wait for test-mm2 to reach 1 replicas
	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, "test-mm2", 1, retryInterval, timeout)
	require.NoError(t, err)

	err = f.Client.Get(context.TODO(), types.NamespacedName{Name: "test-mm2", Namespace: namespace}, exampleMattermost)
	require.NoError(t, err)

	// Get the current pod
	pods := corev1.PodList{}
	opts := client.ListOptions{Namespace: namespace}
	opts.SetLabelSelector("app=mattermost")
	err = f.Client.List(context.TODO(), &opts, &pods)
	require.NoError(t, err)
	require.Equal(t, 1, len(pods.Items))

	mmOldPod := &corev1.Pod{}
	err = f.Client.Get(context.TODO(), types.NamespacedName{Name: pods.Items[0].Name, Namespace: namespace}, mmOldPod)
	require.NoError(t, err)

	// Apply the new version
	exampleMattermost.Spec.Version = "5.11.0"
	err = f.Client.Update(context.TODO(), exampleMattermost)
	require.NoError(t, err)

	// Wait for this pod be terminated
	err = e2eutil.WaitForDeletion(t, f.Client.Client, mmOldPod, retryInterval, timeout)
	require.NoError(t, err)

	// wait for deployment be completed
	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, "test-mm2", 1, retryInterval, timeout)
	require.NoError(t, err)

	err = waitForReconcilicationComplete(t, f.Client.Client, namespace, "test-mm2", retryInterval, timeout)
	require.NoError(t, err)

	newMattermost := &operator.ClusterInstallation{}
	err = f.Client.Get(context.TODO(), types.NamespacedName{Name: "test-mm2", Namespace: namespace}, newMattermost)
	require.NoError(t, err)
	require.Equal(t, "mattermost/mattermost-enterprise-edition", newMattermost.Status.Image)
	require.Equal(t, "5.11.0", newMattermost.Status.Version)

	mmDeployment := &appsv1.Deployment{}
	err = f.Client.Get(context.TODO(), types.NamespacedName{Name: "test-mm2", Namespace: namespace}, mmDeployment)
	require.NoError(t, err)
	require.Equal(t, "mattermost/mattermost-enterprise-edition:5.11.0", mmDeployment.Spec.Template.Spec.Containers[0].Image)

	err = f.Client.Delete(context.TODO(), newMattermost)
	require.NoError(t, err)
}
