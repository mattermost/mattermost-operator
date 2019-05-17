package e2e

import (
	"context"
	goctx "context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apis "github.com/mattermost/mattermost-operator/pkg/apis"
	operator "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
	mysqlOperator "github.com/oracle/mysql-operator/pkg/apis/mysql/v1alpha1"

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

	mysqlList := &mysqlOperator.ClusterList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Cluster",
			APIVersion: "mysql.oracle.com/v1alpha1",
		},
	}
	err := framework.AddToFrameworkScheme(mysqlOperator.AddToScheme, mysqlList)
	assert.Nil(t, err)

	mattermostList := &operator.ClusterInstallationList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterInstallation",
			APIVersion: "mattermost.com/v1alpha1",
		},
	}
	err = framework.AddToFrameworkScheme(apis.AddToScheme, mattermostList)
	assert.Nil(t, err)

	// run subtests
	t.Run("mattermost-group", func(t *testing.T) {
		t.Run("Cluster", MattermostCluster)
	})
}

func mattermostScaleTest(t *testing.T, f *framework.Framework, ctx *framework.TestCtx) error {
	namespace, err := ctx.GetNamespace()
	assert.Nil(t, err)

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
			IngressName:      "test-example.mattermost.dev",
			Replicas:         1,
			MinioStorageSize: "1Gi",
		},
	}

	// use TestCtx's create helper to create the object and add a cleanup function for the new object
	err = f.Client.Create(goctx.TODO(), exampleMattermost, &framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	assert.Nil(t, err)

	// wait for test-mm to reach 1 replicas
	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, "test-mm", 1, retryInterval, timeout)
	assert.Nil(t, err)

	err = f.Client.Get(goctx.TODO(), types.NamespacedName{Name: "test-mm", Namespace: namespace}, exampleMattermost)
	assert.Nil(t, err)

	// exampleMattermost.Spec.Replicas = 3
	// err = f.Client.Update(goctx.TODO(), exampleMattermost)
	// if err != nil {
	// 	return err
	// }

	// wait for test-mm to reach 3 replicas
	// return e2eutil.WaitForDeployment(t, f.KubeClient, namespace, "test-mm", 3, retryInterval, timeout)

	err = f.Client.Delete(goctx.TODO(), exampleMattermost)
	assert.Nil(t, err)

	return nil
}

func mattermostUpgradeTest(t *testing.T, f *framework.Framework, ctx *framework.TestCtx) error {
	namespace, err := ctx.GetNamespace()
	assert.Nil(t, err)

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
			Image:            "mattermost/mattermost-enterprise-edition",
			Version:          "5.10.0",
			IngressName:      "test-example2.mattermost.dev",
			Replicas:         1,
			MinioStorageSize: "1Gi",
		},
	}

	// use TestCtx's create helper to create the object and add a cleanup function for the new object
	err = f.Client.Create(goctx.TODO(), exampleMattermost, &framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	assert.Nil(t, err)

	// wait for test-mm2 to reach 1 replicas
	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, "test-mm2", 1, retryInterval, timeout)
	assert.Nil(t, err)

	err = f.Client.Get(goctx.TODO(), types.NamespacedName{Name: "test-mm2", Namespace: namespace}, exampleMattermost)
	assert.Nil(t, err)

	// Get the current pod
	pods := corev1.PodList{}
	opts := client.ListOptions{Namespace: namespace}
	opts.SetLabelSelector("app=mattermost")
	err = f.Client.List(context.TODO(), &opts, &pods)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(pods.Items))

	mmOldPod := &corev1.Pod{}
	err = f.Client.Get(goctx.TODO(), types.NamespacedName{Name: pods.Items[0].Name, Namespace: namespace}, mmOldPod)
	assert.Nil(t, err)

	// Apply the new version
	exampleMattermost.Spec.Version = "5.11.0"
	err = f.Client.Update(goctx.TODO(), exampleMattermost)
	assert.Nil(t, err)

	// Wait for this pod be terminated
	err = e2eutil.WaitForDeletion(t, f.Client.Client, mmOldPod, retryInterval, timeout)
	assert.Nil(t, err)

	// wait for deployment be completed
	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, "test-mm2", 1, retryInterval, timeout)
	assert.Nil(t, err)

	err = waitForReconcilicationComplete(t, f.Client.Client, namespace, "test-mm2", retryInterval, timeout)
	assert.Nil(t, err)

	newMattermost := &operator.ClusterInstallation{}
	err = f.Client.Get(goctx.TODO(), types.NamespacedName{Name: "test-mm2", Namespace: namespace}, newMattermost)
	assert.Nil(t, err)
	assert.Equal(t, "mattermost/mattermost-enterprise-edition", newMattermost.Status.Image)
	assert.Equal(t, "5.11.0", newMattermost.Status.Version)

	mmDeployment := &appsv1.Deployment{}
	err = f.Client.Get(goctx.TODO(), types.NamespacedName{Name: "test-mm2", Namespace: namespace}, mmDeployment)
	assert.Nil(t, err)
	assert.Equal(t, "mattermost/mattermost-enterprise-edition:5.11.0", mmDeployment.Spec.Template.Spec.Containers[0].Image)

	err = f.Client.Delete(goctx.TODO(), newMattermost)
	assert.Nil(t, err)

	return err
}

func MattermostCluster(t *testing.T) {
	ctx := framework.NewTestCtx(t)
	defer ctx.Cleanup()

	err := ctx.InitializeClusterResources(&framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	assert.Nil(t, err)

	t.Log("Initialized cluster resources")
	namespace, err := ctx.GetNamespace()
	assert.Nil(t, err)

	// get global framework variables
	f := framework.Global

	// wait for mysql-operator to be ready
	err = e2eutil.WaitForDeployment(t, f.KubeClient, "mysql-operator", "mysql-operator", 1, retryInterval, timeout)
	assert.Nil(t, err)

	// wait for minio-operator to be ready
	err = e2eutil.WaitForDeployment(t, f.KubeClient, "minio-operator-ns", "minio-operator", 1, retryInterval, timeout)
	assert.Nil(t, err)

	// wait for mattermost-operator to be ready
	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, "mattermost-operator", 1, retryInterval, timeout)
	assert.Nil(t, err)

	err = mattermostScaleTest(t, f, ctx)
	assert.Nil(t, err)

	err = mattermostUpgradeTest(t, f, ctx)
	assert.Nil(t, err)
}
