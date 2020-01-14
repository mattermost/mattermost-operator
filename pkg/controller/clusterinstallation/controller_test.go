package clusterinstallation

import (
	"fmt"
	"testing"
	"time"

	"github.com/mattermost/mattermost-operator/pkg/components/utils"

	blubr "github.com/mattermost/blubr"
	"github.com/mattermost/mattermost-operator/pkg/apis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
	minioOperator "github.com/minio/minio-operator/pkg/apis/miniocontroller/v1beta1"
	mysqlOperator "github.com/presslabs/mysql-operator/pkg/apis/mysql/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1beta1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestReconcile(t *testing.T) {
	// Setup logging for the reconciler so we can see what happened on failure.
	logger := blubr.InitLogger()
	logger = logger.WithName("test.opr")
	logf.SetLogger(logger)

	ciName := "foo"
	ciNamespace := "default"
	replicas := int32(4)
	ci := &mattermostv1alpha1.ClusterInstallation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ciName,
			Namespace: ciNamespace,
			UID:       types.UID("test"),
		},
		Spec: mattermostv1alpha1.ClusterInstallationSpec{
			Replicas:    replicas,
			Image:       "mattermost/mattermost-enterprise-edition",
			Version:     "5.18.0",
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
	err = prepAllDependencyTestResources(r.client, ci)
	require.NoError(t, err)

	// Mock request to simulate Reconcile() being called on an event for a
	// watched resource .
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: ciName, Namespace: ciNamespace}}
	// Run Reconcile
	// We expect an error on the first reconciliation due to the deployment pods
	// not running yet.
	res, err := r.Reconcile(req)
	require.Error(t, err)
	require.Equal(t, res, reconcile.Result{RequeueAfter: time.Second * 3})

	// Define the NamespacedName objects that will be used to lookup the
	// cluster resources.
	ciKey := types.NamespacedName{Name: ciName, Namespace: ciNamespace}
	ciMysqlKey := types.NamespacedName{Name: utils.HashWithPrefix("db", ciName), Namespace: ciNamespace}
	ciMinioKey := types.NamespacedName{Name: ciName + "-minio", Namespace: ciNamespace}

	t.Run("mysql", func(t *testing.T) {
		t.Run("cluster", func(t *testing.T) {
			mysql := &mysqlOperator.MysqlCluster{}
			err = c.Get(context.TODO(), ciMysqlKey, mysql)
			require.NoError(t, err)
		})
	})

	t.Run("minio", func(t *testing.T) {
		t.Run("instance", func(t *testing.T) {
			minio := &minioOperator.MinIOInstance{}
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
			err = c.Get(context.TODO(), ciKey, ingress)
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
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Image: ci.GetImageName(),
					},
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					corev1.PodCondition{
						Type:   corev1.PodReady,
						Status: corev1.ConditionFalse,
					},
				},
			},
		}
		for i := 0; i < int(replicas); i++ {
			podTemplate.ObjectMeta.Name = fmt.Sprintf("%s-pod-%d", ciName, i)
			err = c.Create(context.TODO(), podTemplate.DeepCopy())
			require.NoError(t, err)
		}

		t.Run("pods not ready", func(t *testing.T) {
			res, err = r.Reconcile(req)
			require.Error(t, err)
		})

		// Make pods ready
		for i := 0; i < int(replicas); i++ {
			podTemplate.ObjectMeta.Name = fmt.Sprintf("%s-pod-%d", ciName, i)
			podTemplate.Status.Conditions = []corev1.PodCondition{
				corev1.PodCondition{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				},
			}
			err = c.Update(context.TODO(), podTemplate.DeepCopy())
			require.NoError(t, err)
		}

		t.Run("no reconcile errors", func(t *testing.T) {
			res, err = r.Reconcile(req)
			require.NoError(t, err)
			require.Equal(t, res, reconcile.Result{})
		})

		// Make pods not running
		for i := 0; i < int(replicas); i++ {
			podTemplate.ObjectMeta.Name = fmt.Sprintf("%s-pod-%d", ciName, i)
			podTemplate.Status.Phase = corev1.PodPending
			err = c.Update(context.TODO(), podTemplate.DeepCopy())
			require.NoError(t, err)
		}

		t.Run("pods not running", func(t *testing.T) {
			res, err = r.Reconcile(req)
			require.Error(t, err)
		})

		// Make pods running
		for i := 0; i < int(replicas); i++ {
			podTemplate.ObjectMeta.Name = fmt.Sprintf("%s-pod-%d", ciName, i)
			podTemplate.Status.Phase = corev1.PodRunning
			err = c.Update(context.TODO(), podTemplate.DeepCopy())
			require.NoError(t, err)
		}

		t.Run("no reconcile errors", func(t *testing.T) {
			res, err = r.Reconcile(req)
			require.NoError(t, err)
			require.Equal(t, res, reconcile.Result{})
		})
		t.Run("correct status", func(t *testing.T) {
			err = c.Get(context.TODO(), ciKey, ci)
			require.NoError(t, err)
			assert.Equal(t, ci.Status.State, mattermostv1alpha1.Stable)
			assert.Equal(t, ci.Status.Replicas, ci.Spec.Replicas)
			assert.Equal(t, ci.Status.Version, ci.Spec.Version)
			assert.Equal(t, ci.Status.Image, ci.Spec.Image)
			assert.Equal(t, ci.Status.Endpoint, ci.Spec.IngressName)
		})
	})

	t.Run("bluegreen", func(t *testing.T) {
		ci.Spec.BlueGreen = mattermostv1alpha1.BlueGreen{
			Enable:               true,
			ProductionDeployment: mattermostv1alpha1.BlueName,
			Blue: mattermostv1alpha1.AppDeployment{
				Name:        "blue-installation",
				IngressName: "blue-ingress",
				Image:       "mattermost/mattermost-blue-edition",
				Version:     "5.17.2",
			},
			Green: mattermostv1alpha1.AppDeployment{
				Name:        "green-installation",
				IngressName: "green-ingress",
				Image:       "mattermost/mattermost-green-edition",
				Version:     "5.18.0",
			},
		}

		svcKey := types.NamespacedName{Name: ci.Name, Namespace: ciNamespace}
		svc := &corev1.Service{}
		blueKey := types.NamespacedName{Name: ci.Spec.BlueGreen.Blue.Name, Namespace: ciNamespace}
		blueSvc := &corev1.Service{}
		greenKey := types.NamespacedName{Name: ci.Spec.BlueGreen.Green.Name, Namespace: ciNamespace}
		greenSvc := &corev1.Service{}

		err = c.Update(context.TODO(), ci)
		require.NoError(t, err)

		blueGreen := []mattermostv1alpha1.AppDeployment{ci.Spec.BlueGreen.Blue, ci.Spec.BlueGreen.Green}
		for _, deployment := range blueGreen {
			podTemplate := corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ciNamespace,
					Labels:    mattermostv1alpha1.ClusterInstallationLabels(deployment.Name),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image: deployment.GetDeploymentImageName(),
						},
					},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					Conditions: []corev1.PodCondition{
						corev1.PodCondition{
							Type:   corev1.PodReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			}

			for i := 0; i < int(replicas); i++ {
				podTemplate.ObjectMeta.Name = fmt.Sprintf("%s-pod-%d", deployment.Name, i)
				err = c.Create(context.TODO(), podTemplate.DeepCopy())
				require.NoError(t, err)
			}

			podList := &corev1.PodList{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Pod",
					APIVersion: "v1",
				},
			}
			listOptions := []client.ListOption{
				client.InNamespace(ciNamespace),
				client.MatchingLabels(mattermostv1alpha1.ClusterInstallationLabels(deployment.Name)),
			}
			err = c.List(context.TODO(), podList, listOptions...)
			require.NoError(t, err)
			require.Equal(t, int(replicas), len(podList.Items))
		}

		t.Run("blue", func(t *testing.T) {
			t.Run("no reconcile errors", func(t *testing.T) {
				res, err = r.Reconcile(req)
				require.NoError(t, err)
				require.Equal(t, res, reconcile.Result{})
			})
			t.Run("correct status", func(t *testing.T) {
				err = c.Get(context.TODO(), ciKey, ci)
				require.NoError(t, err)
				assert.Equal(t, ci.Status.State, mattermostv1alpha1.Stable)
				assert.Equal(t, ci.Status.Replicas, ci.Spec.Replicas)
				assert.Equal(t, ci.Status.Version, ci.Spec.BlueGreen.Blue.Version)
				assert.Equal(t, ci.Status.Image, ci.Spec.BlueGreen.Blue.Image)
				assert.Equal(t, ci.Status.Endpoint, ci.Spec.BlueGreen.Blue.IngressName)
				assert.Equal(t, ci.Status.BlueName, ci.Spec.BlueGreen.Blue.Name)
				assert.Equal(t, ci.Status.GreenName, ci.Spec.BlueGreen.Green.Name)
			})
			t.Run("service", func(t *testing.T) {
				err = c.Get(context.TODO(), svcKey, svc)
				require.NoError(t, err)
				assert.Equal(t, ci.Spec.BlueGreen.Blue.Name, svc.Spec.Selector["v1alpha1.mattermost.com/installation"])
				err = c.Get(context.TODO(), blueKey, blueSvc)
				require.NoError(t, err)
				assert.Equal(t, ci.Spec.BlueGreen.Blue.Name, blueSvc.Spec.Selector["v1alpha1.mattermost.com/installation"])
				err = c.Get(context.TODO(), greenKey, greenSvc)
				require.NoError(t, err)
				assert.Equal(t, ci.Spec.BlueGreen.Green.Name, greenSvc.Spec.Selector["v1alpha1.mattermost.com/installation"])
			})
			t.Run("check normal deployment", func(t *testing.T) {
				deployment := &appsv1.Deployment{}
				err = c.Get(context.TODO(), ciKey, deployment)
				require.Error(t, err)
				assert.True(t, k8sErrors.IsNotFound(err))
			})
		})

		t.Run("green", func(t *testing.T) {
			ci.Spec.BlueGreen.ProductionDeployment = mattermostv1alpha1.GreenName
			err = c.Update(context.TODO(), ci)
			require.NoError(t, err)

			t.Run("no reconcile errors", func(t *testing.T) {
				res, err = r.Reconcile(req)
				require.NoError(t, err)
				require.Equal(t, res, reconcile.Result{})
			})
			t.Run("correct status", func(t *testing.T) {
				err = c.Get(context.TODO(), ciKey, ci)
				require.NoError(t, err)
				assert.Equal(t, ci.Status.State, mattermostv1alpha1.Stable)
				assert.Equal(t, ci.Status.Replicas, ci.Spec.Replicas)
				assert.Equal(t, ci.Status.Version, ci.Spec.BlueGreen.Green.Version)
				assert.Equal(t, ci.Status.Image, ci.Spec.BlueGreen.Green.Image)
				assert.Equal(t, ci.Status.Endpoint, ci.Spec.BlueGreen.Green.IngressName)
				assert.Equal(t, ci.Status.BlueName, ci.Spec.BlueGreen.Blue.Name)
				assert.Equal(t, ci.Status.GreenName, ci.Spec.BlueGreen.Green.Name)
			})
			t.Run("service", func(t *testing.T) {
				err = c.Get(context.TODO(), svcKey, svc)
				require.NoError(t, err)
				assert.Equal(t, ci.Spec.BlueGreen.Green.Name, svc.Spec.Selector["v1alpha1.mattermost.com/installation"])
				err = c.Get(context.TODO(), blueKey, blueSvc)
				require.NoError(t, err)
				assert.Equal(t, ci.Spec.BlueGreen.Blue.Name, blueSvc.Spec.Selector["v1alpha1.mattermost.com/installation"])
				err = c.Get(context.TODO(), greenKey, greenSvc)
				require.NoError(t, err)
				assert.Equal(t, ci.Spec.BlueGreen.Green.Name, greenSvc.Spec.Selector["v1alpha1.mattermost.com/installation"])
			})
		})

		t.Run("clean up", func(t *testing.T) {
			ci.Spec.BlueGreen.Enable = false
			err = c.Update(context.TODO(), ci)
			require.NoError(t, err)

			t.Run("no reconcile errors", func(t *testing.T) {
				res, err = r.Reconcile(req)
				require.NoError(t, err)
				require.Equal(t, res, reconcile.Result{})
			})
			t.Run("deployments", func(t *testing.T) {
				blueDeploy := &appsv1.Deployment{}
				greenDeploy := &appsv1.Deployment{}
				err = c.Get(context.TODO(), blueKey, blueDeploy)
				require.Error(t, err)
				assert.True(t, k8sErrors.IsNotFound(err))
				err = c.Get(context.TODO(), greenKey, greenDeploy)
				require.Error(t, err)
				assert.True(t, k8sErrors.IsNotFound(err))
			})
			t.Run("services", func(t *testing.T) {
				err = c.Get(context.TODO(), blueKey, blueSvc)
				require.Error(t, err)
				assert.True(t, k8sErrors.IsNotFound(err))
				err = c.Get(context.TODO(), greenKey, greenSvc)
				require.Error(t, err)
				assert.True(t, k8sErrors.IsNotFound(err))
			})
			t.Run("ingress", func(t *testing.T) {
				blueIngress := &v1beta1.Ingress{}
				greenIngress := &v1beta1.Ingress{}
				err = c.Get(context.TODO(), blueKey, blueIngress)
				require.Error(t, err)
				assert.True(t, k8sErrors.IsNotFound(err))
				err = c.Get(context.TODO(), greenKey, greenIngress)
				require.Error(t, err)
				assert.True(t, k8sErrors.IsNotFound(err))
			})
		})
	})
}

func prepAllDependencyTestResources(client client.Client, ci *mattermostv1alpha1.ClusterInstallation) error {
	minioService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ci.Name + "-minio-hl-svc",
			Namespace: ci.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports:     []corev1.ServicePort{{Port: 9000}},
			ClusterIP: corev1.ClusterIPNone,
		},
	}

	return client.Create(context.TODO(), minioService)
}
