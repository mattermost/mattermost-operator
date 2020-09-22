package e2e

import (
	"context"
	"fmt"
	operator "github.com/mattermost/mattermost-operator/apis/mattermost/v1alpha1"
	"github.com/mattermost/mattermost-operator/pkg/components/utils"
	operatortest "github.com/mattermost/mattermost-operator/test"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var (
	// retryInterval is an interval between check attempts
	retryInterval = time.Second * 5
	// timeout to wait for k8s objects to be created
	timeout = time.Second * 900

	mmNamespace = "mattermost-operator"
)

func TestMattermost(t *testing.T) {
	// Setup testenv
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{printer.NewlineReporter{}})

	k8sTypedClient, err := kubernetes.NewForConfig(cfg)
	require.NoError(t, err)

	t.Run("mysql operator ready", func(t *testing.T) {
		err = waitForStatefulSet(t, k8sClient, "mysql-operator", "mysql-operator", 1, retryInterval, timeout)
		require.NoError(t, err)
	})
	t.Run("minio operator ready", func(t *testing.T) {
		err = waitForDeployment(t, k8sTypedClient, "minio-operator", "minio-operator", 1, retryInterval, timeout)
		require.NoError(t, err)
	})
	t.Run("mattermost operator ready", func(t *testing.T) {
		err = waitForDeployment(t, k8sTypedClient, mmNamespace, "mattermost-operator", 1, retryInterval, timeout)
		require.NoError(t, err)
	})

	t.Run("mattermost scale test", func(t *testing.T) {
		mattermostScaleTest(t, k8sClient, k8sTypedClient)
	})

	t.Run("mattermost upgrade test", func(t *testing.T) {
		mattermostUpgradeTest(t, k8sClient, k8sTypedClient)
	})

	t.Run("mattermost with mysql replicas", func(t *testing.T) {
		mattermostWithMySQLReplicas(t, k8sClient, k8sTypedClient)
	})
}

func mattermostScaleTest(t *testing.T, k8sClient client.Client, k8sTypedClient kubernetes.Interface) {
	// create ClusterInstallation custom resource
	exampleMattermost := &operator.ClusterInstallation{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterInstallation",
			APIVersion: "mattermost.com/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-mm",
			Namespace: mmNamespace,
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

	err := k8sClient.Create(context.TODO(), exampleMattermost)
	require.NoError(t, err)

	err = waitForStatefulSet(t, k8sClient, mmNamespace, "test-mm-minio", 1, retryInterval, timeout)
	require.NoError(t, err)

	err = waitForStatefulSet(t, k8sClient, mmNamespace, fmt.Sprintf("%s-mysql", utils.HashWithPrefix("db", "test-mm")), 1, retryInterval, timeout)
	require.NoError(t, err)

	// wait for test-mm to reach 1 replicas
	err = waitForDeployment(t, k8sTypedClient, mmNamespace, "test-mm", 1, retryInterval, timeout)
	require.NoError(t, err)

	err = k8sClient.Get(context.TODO(), types.NamespacedName{Name: "test-mm", Namespace: mmNamespace}, exampleMattermost)
	require.NoError(t, err)

	exampleMattermost.Spec.Replicas = 2
	err = k8sClient.Update(context.TODO(), exampleMattermost)
	require.NoError(t, err)

	// wait for test-mm to reach 2 replicas
	err = waitForDeployment(t, k8sTypedClient, mmNamespace, "test-mm", 2, retryInterval, timeout)
	require.NoError(t, err)

	err = waitForReconcilicationComplete(t, k8sClient, mmNamespace, "test-mm", retryInterval, timeout)
	require.NoError(t, err)

	// scale down again
	err = k8sClient.Get(context.TODO(), types.NamespacedName{Name: "test-mm", Namespace: mmNamespace}, exampleMattermost)
	require.NoError(t, err)

	exampleMattermost.Spec.Replicas = 1
	err = k8sClient.Update(context.TODO(), exampleMattermost)
	require.NoError(t, err)

	// wait for test-mm to reach 1 replicas
	err = waitForDeployment(t, k8sTypedClient, mmNamespace, "test-mm", 1, retryInterval, timeout)
	require.NoError(t, err)

	err = waitForReconcilicationComplete(t, k8sClient, mmNamespace, "test-mm", retryInterval, timeout)
	require.NoError(t, err)

	err = k8sClient.Delete(context.TODO(), exampleMattermost)
	require.NoError(t, err)
}

func mattermostUpgradeTest(t *testing.T, k8sClient client.Client, k8sTypedClient kubernetes.Interface) {
	testName := "test-mm2"

	// create ClusterInstallation custom resource
	exampleMattermost := &operator.ClusterInstallation{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterInstallation",
			APIVersion: "mattermost.com/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      testName,
			Namespace: mmNamespace,
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

	err := k8sClient.Create(context.TODO(), exampleMattermost)
	require.NoError(t, err)

	err = waitForStatefulSet(t, k8sClient, mmNamespace, fmt.Sprintf("%s-minio", testName), 1, retryInterval, timeout)
	require.NoError(t, err)

	err = waitForStatefulSet(t, k8sClient, mmNamespace, fmt.Sprintf("%s-mysql", utils.HashWithPrefix("db", testName)), 1, retryInterval, timeout)
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
		client.MatchingLabels(map[string]string{"app": "mattermost"}),
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

	err = waitForReconcilicationComplete(t, k8sClient, mmNamespace, testName, retryInterval, timeout)
	require.NoError(t, err)

	newMattermost := &operator.ClusterInstallation{}
	err = k8sClient.Get(context.TODO(), types.NamespacedName{Name: testName, Namespace: mmNamespace}, newMattermost)
	require.NoError(t, err)
	require.Equal(t, "mattermost/mattermost-enterprise-edition", newMattermost.Status.Image)
	require.Equal(t, operatortest.LatestStableMattermostVersion, newMattermost.Status.Version)

	mmDeployment := &appsv1.Deployment{}
	err = k8sClient.Get(context.TODO(), types.NamespacedName{Name: testName, Namespace: mmNamespace}, mmDeployment)
	require.NoError(t, err)
	require.Equal(t, "mattermost/mattermost-enterprise-edition:"+operatortest.LatestStableMattermostVersion, mmDeployment.Spec.Template.Spec.Containers[0].Image)

	err = k8sClient.Delete(context.TODO(), exampleMattermost)
	require.NoError(t, err)
}

func mattermostWithMySQLReplicas(t *testing.T, client client.Client, typedClient kubernetes.Interface) {
	testName := "test-mm3"

	// create ClusterInstallation custom resource
	exampleMattermost := &operator.ClusterInstallation{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterInstallation",
			APIVersion: "mattermost.com/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      testName,
			Namespace: mmNamespace,
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

	// use Context's create helper to create the object and add a cleanup function for the new object
	err := client.Create(context.TODO(), exampleMattermost)
	require.NoError(t, err)

	err = waitForStatefulSet(t, client, mmNamespace, fmt.Sprintf("%s-minio", testName), 1, retryInterval, timeout)
	require.NoError(t, err)

	err = waitForStatefulSet(t, client, mmNamespace, fmt.Sprintf("%s-mysql", utils.HashWithPrefix("db", testName)), 2, retryInterval, timeout)
	require.NoError(t, err)

	err = waitForMySQLStatusReady(t, client, mmNamespace, utils.HashWithPrefix("db", testName), 2, retryInterval, timeout)
	require.NoError(t, err)

	err = client.Delete(context.TODO(), exampleMattermost)
	require.NoError(t, err)
}
