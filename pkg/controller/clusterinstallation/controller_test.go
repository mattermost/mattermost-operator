package clusterinstallation

import (
	"fmt"
	"testing"

	"github.com/mattermost/mattermost-operator/pkg/apis"
	logmo "github.com/mattermost/mattermost-operator/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
	minioOperator "github.com/minio/minio-operator/pkg/apis/miniocontroller/v1beta1"
	mysqlOperator "github.com/oracle/mysql-operator/pkg/apis/mysql/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	v1beta1 "k8s.io/api/extensions/v1beta1"
	rbacv1beta1 "k8s.io/api/rbac/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestReconcile(t *testing.T) {
	// Setup logging for the reconciler so we can see what happened on failure.
	logger := logmo.InitLogger()
	logger = logger.WithName("test.opr")
	logf.SetLogger(logger)

	ciName := "foo"
	ciNamespace := "default"
	replicas := int32(4)
	ci := &mattermostv1alpha1.ClusterInstallation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ciName,
			Namespace: ciNamespace,
		},
		Spec: mattermostv1alpha1.ClusterInstallationSpec{
			Replicas:    replicas,
			Image:       "mattermost/mattermost-enterprise-edition",
			Version:     "5.11.0",
			IngressName: "foo.mattermost.dev",
		},
	}

	// Register operator types with the runtime scheme.
	apis.AddToScheme(scheme.Scheme)
	s := scheme.Scheme
	s.AddKnownTypes(mattermostv1alpha1.SchemeGroupVersion, ci)
	// Create a fake client to mock API calls.
	c := fake.NewFakeClient()
	// Create a ReconcileClusterInstallation object with the scheme and fake
	// client.
	r := &ReconcileClusterInstallation{client: c, scheme: s}

	err := c.Create(context.TODO(), ci)
	require.NoError(t, err)

	// Create the resources that would normally be created by other operators
	// running on the kubernetes cluster.
	dbSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ci.Name + "-mysql-root-password",
			Namespace: ci.Namespace,
		},
		Data: map[string][]byte{
			"password": []byte("mysupersecure"),
		},
	}
	err = c.Create(context.TODO(), dbSecret)
	require.NoError(t, err)

	MinioSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ci.Name + "-minio",
			Namespace: ci.Namespace,
		},
		Data: map[string][]byte{
			"accesskey": []byte("mysupersecure"),
			"secretkey": []byte("mysupersecurekey"),
		},
	}
	err = c.Create(context.TODO(), MinioSecret)
	require.NoError(t, err)

	minioService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ci.Name + "-minio",
			Namespace: ci.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports:     []corev1.ServicePort{{Port: 9000}},
			ClusterIP: corev1.ClusterIPNone,
		},
	}
	err = c.Create(context.TODO(), minioService)
	require.NoError(t, err)

	// Mock request to simulate Reconcile() being called on an event for a
	// watched resource .
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: ciName, Namespace: ciNamespace}}
	// Run Reconcile
	// We expect an error on the first reconciliation due to the deployment pods
	// not running yet.
	res, err := r.Reconcile(req)
	require.Error(t, err)
	require.Equal(t, res, reconcile.Result{})

	// Define the NamespacedName objects that will be used to lookup the
	// cluster resources.
	ciKey := types.NamespacedName{Name: ciName, Namespace: ciNamespace}
	ciIngressKey := types.NamespacedName{Name: ciName + "-ingress", Namespace: ciNamespace}
	ciMysqlKey := types.NamespacedName{Name: ciName + "-mysql", Namespace: ciNamespace}
	ciMinioKey := types.NamespacedName{Name: ciName + "-minio", Namespace: ciNamespace}
	ciSvcAccountKey := types.NamespacedName{Name: "mysql-agent", Namespace: ciNamespace}

	t.Run("mysql", func(t *testing.T) {
		t.Run("cluster", func(t *testing.T) {
			mysql := &mysqlOperator.Cluster{}
			err = c.Get(context.TODO(), ciMysqlKey, mysql)
			require.NoError(t, err)
		})
		t.Run("service account", func(t *testing.T) {
			svcAccount := &corev1.ServiceAccount{}
			err = c.Get(context.TODO(), ciSvcAccountKey, svcAccount)
			require.NoError(t, err)
		})
		t.Run("role binding", func(t *testing.T) {
			roleBinding := &rbacv1beta1.RoleBinding{}
			err = c.Get(context.TODO(), ciSvcAccountKey, roleBinding)
			require.NoError(t, err)
		})
	})

	t.Run("minio", func(t *testing.T) {
		t.Run("instance", func(t *testing.T) {
			minio := &minioOperator.MinioInstance{}
			err = c.Get(context.TODO(), ciMinioKey, minio)
			require.NoError(t, err)
		})
	})

	t.Run("mattermost", func(t *testing.T) {
		t.Run("service", func(t *testing.T) {
			service := &corev1.Service{}
			err = c.Get(context.TODO(), ciKey, service)
			require.NoError(t, err)
		})
		t.Run("ingress", func(t *testing.T) {
			ingress := &v1beta1.Ingress{}
			err = c.Get(context.TODO(), ciIngressKey, ingress)
			require.NoError(t, err)
		})
		t.Run("deployment", func(t *testing.T) {
			deployment := &appsv1.Deployment{}
			err = c.Get(context.TODO(), ciKey, deployment)
			require.NoError(t, err)
			require.Equal(t, deployment.Spec.Replicas, &ci.Spec.Replicas)
		})
	})

	t.Run("final check", func(t *testing.T) {
		// Create expected mattermost pods.
		podTemplate := corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ciNamespace,
				Labels:    mattermostv1alpha1.ClusterInstallationLabels(ciName),
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Image: ci.GetImageName(),
					},
				},
			},
			Status: v1.PodStatus{
				Phase: v1.PodRunning,
			},
		}
		for i := 0; i < int(replicas); i++ {
			podTemplate.ObjectMeta.Name = fmt.Sprintf("%s-pod-%d", ciName, i)
			err = c.Create(context.TODO(), podTemplate.DeepCopy())
			require.NoError(t, err)
		}

		t.Run("no reconcile errors", func(t *testing.T) {
			// Reconcile again and check for errors this time.
			res, err = r.Reconcile(req)
			require.NoError(t, err)
			require.Equal(t, res, reconcile.Result{})
		})
		t.Run("correct status", func(t *testing.T) {
			finalCI := &mattermostv1alpha1.ClusterInstallation{}
			err = c.Get(context.TODO(), ciKey, finalCI)
			require.NoError(t, err)
			assert.Equal(t, finalCI.Status.State, mattermostv1alpha1.Stable)
			assert.Equal(t, finalCI.Status.Replicas, ci.Spec.Replicas)
			assert.Equal(t, finalCI.Status.Version, ci.Spec.Version)
			assert.Equal(t, finalCI.Status.Image, ci.Spec.Image)
		})
	})
}
