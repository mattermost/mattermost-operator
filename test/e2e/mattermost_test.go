package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apis "github.com/mattermost/mattermost-operator/pkg/apis"
	operator "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
	"github.com/mattermost/mattermost-operator/pkg/components/utils"
	operatortest "github.com/mattermost/mattermost-operator/test"

	framework "github.com/operator-framework/operator-sdk/pkg/test"
	"github.com/operator-framework/operator-sdk/pkg/test/e2eutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var (
	// retryInterval is an interval between check attempts
	retryInterval = time.Second * 5
	// timeout to wait for k8s objects to be created
	timeout              = time.Second * 900
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

	t.Run("initialize cluster resources", func(t *testing.T) {
		err = ctx.InitializeClusterResources(&framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
		require.NoError(t, err)
	})

	namespace, err := ctx.GetNamespace()
	require.NoError(t, err)

	// get global framework variables
	f := framework.Global

	t.Run("mysql operator ready", func(t *testing.T) {
		err = waitForStatefulSet(t, f.Client.Client, "mysql-operator", "mysql-operator", 1, retryInterval, timeout)
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

	t.Run("mattermost with mysql replicas", func(t *testing.T) {
		mattermostWithMySQLReplicas(t, f, ctx)
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
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("100Mi"),
				},
			},
			Minio: operator.Minio{
				StorageSize: "1Gi",
				Replicas:    1,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("100Mi"),
					},
				},
			},
			Database: operator.Database{
				StorageSize: "1Gi",
				Replicas:    1,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("100Mi"),
					},
				},
			},
		},
	}

	// use TestCtx's create helper to create the object and add a cleanup function for the new object
	err = f.Client.Create(context.TODO(), exampleMattermost, &framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	require.NoError(t, err)

	err = waitForStatefulSet(t, f.Client.Client, namespace, "test-mm-minio", 1, retryInterval, timeout)
	require.NoError(t, err)

	err = waitForStatefulSet(t, f.Client.Client, namespace, fmt.Sprintf("%s-mysql", utils.HashWithPrefix("db", "test-mm")), 1, retryInterval, timeout)
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

	testName := "test-mm2"

	// create ClusterInstallation custom resource
	exampleMattermost := &operator.ClusterInstallation{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterInstallation",
			APIVersion: "mattermost.com/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      testName,
			Namespace: namespace,
		},
		Spec: operator.ClusterInstallationSpec{
			Image:       "mattermost/mattermost-enterprise-edition",
			Version:     operatortest.PreviousStableMattermostVersion,
			IngressName: "test-example2.mattermost.dev",
			Replicas:    1,
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("100Mi"),
				},
			},
			Minio: operator.Minio{
				StorageSize: "1Gi",
				Replicas:    1,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("100Mi"),
					},
				},
			},
			Database: operator.Database{
				StorageSize: "1Gi",
				Replicas:    1,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("100Mi"),
					},
				},
			},
		},
	}

	// use TestCtx's create helper to create the object and add a cleanup function for the new object
	err = f.Client.Create(context.TODO(), exampleMattermost, &framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	require.NoError(t, err)

	err = waitForStatefulSet(t, f.Client.Client, namespace, fmt.Sprintf("%s-minio", testName), 1, retryInterval, timeout)
	require.NoError(t, err)

	err = waitForStatefulSet(t, f.Client.Client, namespace, fmt.Sprintf("%s-mysql", utils.HashWithPrefix("db", testName)), 1, retryInterval, timeout)
	require.NoError(t, err)

	// wait for test-mm2 to reach 1 replicas
	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, testName, 1, retryInterval, timeout)
	require.NoError(t, err)

	err = f.Client.Get(context.TODO(), types.NamespacedName{Name: testName, Namespace: namespace}, exampleMattermost)
	require.NoError(t, err)

	// Get the current pod
	pods := corev1.PodList{}
	listOptions := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels(map[string]string{"app": "mattermost"}),
	}

	err = f.Client.List(context.TODO(), &pods, listOptions...)
	require.NoError(t, err)
	require.Equal(t, 1, len(pods.Items))

	mmOldPod := &corev1.Pod{}
	err = f.Client.Get(context.TODO(), types.NamespacedName{Name: pods.Items[0].Name, Namespace: namespace}, mmOldPod)
	require.NoError(t, err)

	// Apply the new version
	exampleMattermost.Spec.Version = operatortest.LatestStableMattermostVersion
	err = f.Client.Update(context.TODO(), exampleMattermost)
	require.NoError(t, err)

	// Wait for this pod be terminated
	err = e2eutil.WaitForDeletion(t, f.Client.Client, mmOldPod, retryInterval, timeout)
	require.NoError(t, err)

	// wait for deployment be completed
	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, testName, 1, retryInterval, timeout)
	require.NoError(t, err)

	err = waitForReconcilicationComplete(t, f.Client.Client, namespace, testName, retryInterval, timeout)
	require.NoError(t, err)

	newMattermost := &operator.ClusterInstallation{}
	err = f.Client.Get(context.TODO(), types.NamespacedName{Name: testName, Namespace: namespace}, newMattermost)
	require.NoError(t, err)
	require.Equal(t, "mattermost/mattermost-enterprise-edition", newMattermost.Status.Image)
	require.Equal(t, operatortest.LatestStableMattermostVersion, newMattermost.Status.Version)

	mmDeployment := &appsv1.Deployment{}
	err = f.Client.Get(context.TODO(), types.NamespacedName{Name: testName, Namespace: namespace}, mmDeployment)
	require.NoError(t, err)
	require.Equal(t, "mattermost/mattermost-enterprise-edition:"+operatortest.LatestStableMattermostVersion, mmDeployment.Spec.Template.Spec.Containers[0].Image)

	err = f.Client.Delete(context.TODO(), newMattermost)
	require.NoError(t, err)
}

func mattermostWithMySQLReplicas(t *testing.T, f *framework.Framework, ctx *framework.TestCtx) {
	namespace, err := ctx.GetNamespace()
	require.NoError(t, err)

	testName := "test-mm3"

	// create ClusterInstallation custom resource
	exampleMattermost := &operator.ClusterInstallation{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterInstallation",
			APIVersion: "mattermost.com/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      testName,
			Namespace: namespace,
		},
		Spec: operator.ClusterInstallationSpec{
			IngressName: "test-example.mattermost.dev",
			Replicas:    1,
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("100Mi"),
				},
			},
			Minio: operator.Minio{
				StorageSize: "1Gi",
				Replicas:    1,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("100Mi"),
					},
				},
			},
			Database: operator.Database{
				StorageSize: "1Gi",
				Replicas:    2,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("100Mi"),
					},
				},
			},
		},
	}

	// use TestCtx's create helper to create the object and add a cleanup function for the new object
	err = f.Client.Create(context.TODO(), exampleMattermost, &framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	require.NoError(t, err)

	err = waitForStatefulSet(t, f.Client.Client, namespace, fmt.Sprintf("%s-minio", testName), 1, retryInterval, timeout)
	require.NoError(t, err)

	err = waitForStatefulSet(t, f.Client.Client, namespace, fmt.Sprintf("%s-mysql", utils.HashWithPrefix("db", testName)), 2, retryInterval, timeout)
	require.NoError(t, err)

	err = waitForMySQLStatusReady(t, f.Client.Client, namespace, utils.HashWithPrefix("db", testName), 2, retryInterval, timeout)
	require.NoError(t, err)

	err = f.Client.Delete(context.TODO(), exampleMattermost)
	require.NoError(t, err)
}
