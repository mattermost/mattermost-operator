package clusterinstallation

import (
	"fmt"
	"testing"
	"time"

	blubr "github.com/mattermost/blubr"
	"github.com/mattermost/mattermost-operator/pkg/components/utils"
	operatortest "github.com/mattermost/mattermost-operator/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
	"k8s.io/api/extensions/v1beta1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/apis/mattermost/v1alpha1"
	minioOperator "github.com/minio/minio-operator/pkg/apis/miniocontroller/v1beta1"
	v1beta1Minio "github.com/minio/minio-operator/pkg/apis/miniocontroller/v1beta1"
	mysqlOperator "github.com/presslabs/mysql-operator/pkg/apis/mysql/v1alpha1"
	v1alpha1MySQL "github.com/presslabs/mysql-operator/pkg/apis/mysql/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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
			Version:     operatortest.LatestStableMattermostVersion,
			IngressName: "foo.mattermost.dev",
		},
	}

	// Register operator types with the runtime scheme.
	s := prepareSchema(t, scheme.Scheme)
	s.AddKnownTypes(mattermostv1alpha1.GroupVersion, ci)
	// Create a fake client to mock API calls.
	c := fake.NewFakeClient()
	// Create a ReconcileClusterInstallation object with the scheme and fake
	// client.
	r := &ClusterInstallationReconciler{
		Client:             c,
		NonCachedAPIReader: c,
		Scheme:             s,
		Log:                logger,
		MaxReconciling:     5,
	}

	err := c.Create(context.TODO(), ci)
	require.NoError(t, err)

	// Create the resources that would normally be created by other operators
	// running on the kubernetes cluster.
	err = prepAllDependencyTestResources(r.Client, ci)
	require.NoError(t, err)

	// Mock request to simulate Reconcile() being called on an event for a
	// watched resource .
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: ciName, Namespace: ciNamespace}}
	// Run Reconcile
	// We expect an error on the first reconciliation due to the deployment pods
	// not running yet.
	res, err := r.Reconcile(req)
	require.NoError(t, err)
	require.Equal(t, res, reconcile.Result{RequeueAfter: 6 * time.Second})

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

		t.Run("replica set does not exist", func(t *testing.T) {
			res, err = r.Reconcile(req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "replicaSet did not start rolling pods")
		})

		replicaSet := &appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ciName,
				Namespace: ciNamespace,
				Labels:    ci.ClusterInstallationLabels(ciName),
			},
			Status: appsv1.ReplicaSetStatus{},
		}
		err = c.Create(context.TODO(), replicaSet)
		require.NoError(t, err)

		t.Run("replica set not observed", func(t *testing.T) {
			res, err = r.Reconcile(req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "replicaSet did not start rolling pods")
		})
		replicaSet.Status.ObservedGeneration = 1
		err = c.Update(context.TODO(), replicaSet)
		require.NoError(t, err)

		// Create expected mattermost pods.
		podTemplate := corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ciNamespace,
				Labels:    ci.ClusterInstallationLabels(ciName),
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
					{
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
			require.NoError(t, err)
			require.Equal(t, res, reconcile.Result{RequeueAfter: 6 * time.Second})
		})

		// Make pods ready
		pods := &corev1.PodList{}
		err = c.List(context.TODO(), pods)
		require.NoError(t, err)

		for _, pod := range pods.Items {
			for i := 0; i < int(replicas); i++ {
				if pod.ObjectMeta.Name == fmt.Sprintf("%s-pod-%d", ciName, i) {
					pod.Status.Conditions = []corev1.PodCondition{
						{
							Type:   corev1.PodReady,
							Status: corev1.ConditionTrue,
						},
					}
					err = c.Update(context.TODO(), pod.DeepCopy())
					require.NoError(t, err)
				}
			}
		}

		t.Run("no reconcile errors", func(t *testing.T) {
			res, err = r.Reconcile(req)
			require.NoError(t, err)
			require.Equal(t, res, reconcile.Result{})
		})

		// Make pods not running
		pods = &corev1.PodList{}
		err = c.List(context.TODO(), pods)
		require.NoError(t, err)

		for _, pod := range pods.Items {
			for i := 0; i < int(replicas); i++ {
				if pod.ObjectMeta.Name == fmt.Sprintf("%s-pod-%d", ciName, i) {
					pod.Status.Phase = corev1.PodPending
					err = c.Update(context.TODO(), pod.DeepCopy())
					require.NoError(t, err)
				}
			}
		}

		t.Run("pods not running", func(t *testing.T) {
			res, err = r.Reconcile(req)
			require.NoError(t, err)
			require.Equal(t, res, reconcile.Result{RequeueAfter: 6 * time.Second})
		})

		// Make pods running
		pods = &corev1.PodList{}
		err = c.List(context.TODO(), pods)
		require.NoError(t, err)

		for _, pod := range pods.Items {
			for i := 0; i < int(replicas); i++ {
				if pod.ObjectMeta.Name == fmt.Sprintf("%s-pod-%d", ciName, i) {
					pod.Status.Phase = corev1.PodRunning
					err = c.Update(context.TODO(), pod.DeepCopy())
					require.NoError(t, err)
				}
			}
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
				Version:     operatortest.PreviousStableMattermostVersion,
			},
			Green: mattermostv1alpha1.AppDeployment{
				Name:        "green-installation",
				IngressName: "green-ingress",
				Image:       "mattermost/mattermost-green-edition",
				Version:     operatortest.LatestStableMattermostVersion,
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
			replicaSet := &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      deployment.Name,
					Namespace: ciNamespace,
					Labels:    ci.ClusterInstallationLabels(deployment.Name),
				},
				Status: appsv1.ReplicaSetStatus{ObservedGeneration: 1},
			}
			err = c.Create(context.TODO(), replicaSet)
			require.NoError(t, err)

			podTemplate := corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ciNamespace,
					Labels:    ci.ClusterInstallationLabels(deployment.Name),
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
						{
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
				client.MatchingLabels(ci.ClusterInstallationLabels(deployment.Name)),
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

func TestReconcilingLimit(t *testing.T) {
	// Setup logging for the reconciler so we can see what happened on failure.
	logger := blubr.InitLogger()
	logger = logger.WithName("test.opr")
	logf.SetLogger(logger)

	ciNamespace := "default"
	replicas := int32(4)
	requeueOnLimitDelay := 35 * time.Second

	newClusterInstallation := func(name string, uid string, state mattermostv1alpha1.RunningState) *mattermostv1alpha1.ClusterInstallation {
		return &mattermostv1alpha1.ClusterInstallation{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ciNamespace,
				UID:       types.UID(uid),
			},
			Spec: mattermostv1alpha1.ClusterInstallationSpec{
				Replicas:    replicas,
				Image:       "mattermost/mattermost-enterprise-edition",
				Version:     operatortest.LatestStableMattermostVersion,
				IngressName: "foo.mattermost.dev",
			},
			Status: mattermostv1alpha1.ClusterInstallationStatus{State: state},
		}
	}

	ci1 := newClusterInstallation("first", "1", mattermostv1alpha1.Reconciling)

	// Register operator types with the runtime scheme.
	s := prepareSchema(t, scheme.Scheme)
	s.AddKnownTypes(mattermostv1alpha1.GroupVersion, ci1)
	// Create a fake client to mock API calls.
	c := fake.NewFakeClient()
	// Create a ReconcileClusterInstallation object with the scheme and fake client.
	r := &ClusterInstallationReconciler{
		Client:              c,
		Scheme:              s,
		Log:                 logger,
		MaxReconciling:      2,
		RequeueOnLimitDelay: requeueOnLimitDelay,
	}

	assertInstallationsCount := func(t *testing.T, expectedCIs, expectedReconciling int) {
		var ciList mattermostv1alpha1.ClusterInstallationList
		err := c.List(context.TODO(), &ciList)
		require.NoError(t, err)

		assert.Equal(t, expectedCIs, len(ciList.Items))
		assert.Equal(t, expectedReconciling, countReconciling(ciList.Items))
	}

	err := c.Create(context.TODO(), ci1)
	require.NoError(t, err)

	ci2 := newClusterInstallation("second", "2", mattermostv1alpha1.Reconciling)
	err = c.Create(context.TODO(), ci2)
	require.NoError(t, err)

	ci3 := newClusterInstallation("third", "3", mattermostv1alpha1.Reconciling)
	err = c.Create(context.TODO(), ci3)
	require.NoError(t, err)

	ci4 := newClusterInstallation("forth", "4", "")
	err = c.Create(context.TODO(), ci4)
	require.NoError(t, err)

	ci5 := newClusterInstallation("fifth", "5", mattermostv1alpha1.Stable)
	err = c.Create(context.TODO(), ci5)
	require.NoError(t, err)

	req1 := requestForCI(ci1)
	_, err = r.Reconcile(req1)
	require.Error(t, err)
	assertInstallationsCount(t, 5, 3)

	req2 := requestForCI(ci2)
	_, err = r.Reconcile(req2)
	require.Error(t, err)

	t.Run("should pick up Installation in Reconciling state even if limit reached", func(t *testing.T) {
		req3 := requestForCI(ci3)
		_, err = r.Reconcile(req3)
		require.Error(t, err)
	})

	var result reconcile.Result
	t.Run("should not pick up Installation without state if limit reached", func(t *testing.T) {
		req4 := requestForCI(ci4)
		result, err = r.Reconcile(req4)
		require.NoError(t, err)
		assert.Equal(t, requeueOnLimitDelay, result.RequeueAfter)

		result, err = r.Reconcile(req4)
		require.NoError(t, err)
		assert.Equal(t, requeueOnLimitDelay, result.RequeueAfter)
	})

	t.Run("should not pick up Installation in Stable state if limit reached", func(t *testing.T) {
		req5 := requestForCI(ci5)
		result, err = r.Reconcile(req5)
		require.NoError(t, err)
		assert.Equal(t, requeueOnLimitDelay, result.RequeueAfter)
	})

	err = c.Delete(context.TODO(), ci1)
	require.NoError(t, err)
	_, err = r.Reconcile(req1)
	require.NoError(t, err)
	assertInstallationsCount(t, 4, 2)

	err = c.Delete(context.TODO(), ci2)
	require.NoError(t, err)
	_, err = r.Reconcile(req2)
	require.NoError(t, err)
	assertInstallationsCount(t, 3, 1)

	t.Run("should pick up Installation without state when cache freed", func(t *testing.T) {
		req4 := requestForCI(ci4)
		_, err = r.Reconcile(req4)
		require.Error(t, err)
		assertInstallationsCount(t, 3, 2)
	})

	err = c.Delete(context.TODO(), ci4)
	require.NoError(t, err)
	req4 := requestForCI(ci4)
	_, err = r.Reconcile(req4)
	require.NoError(t, err)
	assertInstallationsCount(t, 2, 1)

	t.Run("should pick up Installation in Stable state when cache freed", func(t *testing.T) {
		req5 := requestForCI(ci5)
		_, err = r.Reconcile(req5)
		require.Error(t, err)
		assertInstallationsCount(t, 2, 2)
	})

	err = c.Delete(context.TODO(), ci5)
	require.NoError(t, err)
	req5 := requestForCI(ci5)
	_, err = r.Reconcile(req5)
	require.NoError(t, err)
	assertInstallationsCount(t, 1, 1)

	t.Run("should add new installations to cache", func(t *testing.T) {
		// Pick up first for reconciling
		ci6 := newClusterInstallation("sixth", "6", "")
		err = c.Create(context.TODO(), ci6)
		require.NoError(t, err)
		req6 := requestForCI(ci6)
		_, err = r.Reconcile(req6)
		require.Error(t, err)
		assertInstallationsCount(t, 2, 2)

		// Do not pick up second
		ci7 := newClusterInstallation("seventh", "7", "")
		err = c.Create(context.TODO(), ci7)
		require.NoError(t, err)
		req7 := requestForCI(ci7)
		result, err = r.Reconcile(req7)
		require.NoError(t, err)
		assert.Equal(t, requeueOnLimitDelay, result.RequeueAfter)
		assertInstallationsCount(t, 3, 2)
	})
}

func requestForCI(ci *mattermostv1alpha1.ClusterInstallation) reconcile.Request {
	return reconcile.Request{NamespacedName: types.NamespacedName{Name: ci.Name, Namespace: ci.Namespace}}
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

func prepareSchema(t *testing.T, scheme *runtime.Scheme) *runtime.Scheme {
	err := mattermostv1alpha1.AddToScheme(scheme)
	require.NoError(t, err)
	err = v1beta1Minio.AddToScheme(scheme)
	require.NoError(t, err)
	err = v1alpha1MySQL.SchemeBuilder.AddToScheme(scheme)
	require.NoError(t, err)

	return scheme
}
