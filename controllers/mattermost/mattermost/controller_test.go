package mattermost

import (
	"fmt"
	"testing"
	"time"

	"github.com/mattermost/mattermost-operator/pkg/resources"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"

	blubr "github.com/mattermost/blubr"
	"github.com/mattermost/mattermost-operator/pkg/components/utils"
	operatortest "github.com/mattermost/mattermost-operator/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

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

	mmName := "foo"
	mmNamespace := "default"
	replicas := int32(4)
	mm := &mmv1beta.Mattermost{
		ObjectMeta: metav1.ObjectMeta{
			Name:       mmName,
			Namespace:  mmNamespace,
			UID:        types.UID("test"),
			Generation: 1,
		},
		Spec: mmv1beta.MattermostSpec{
			Replicas:    &replicas,
			Image:       "mattermost/mattermost-enterprise-edition",
			Version:     operatortest.LatestStableMattermostVersion,
			IngressName: "foo.mattermost.dev",
		},
	}

	// Register operator types with the runtime scheme.
	s := prepareSchema(t, scheme.Scheme)
	s.AddKnownTypes(mmv1beta.GroupVersion, mm)
	// Create a fake client to mock API calls.
	c := fake.NewFakeClient()
	// Create a ReconcileMattermost object with the scheme and fake
	// client.
	r := &MattermostReconciler{
		Client:             c,
		NonCachedAPIReader: c,
		Scheme:             s,
		Log:                logger,
		MaxReconciling:     5,
		Resources:          resources.NewResourceHelper(c, s),
	}

	err := c.Create(context.TODO(), mm)
	require.NoError(t, err)

	// Create the resources that would normally be created by other operators
	// running on the kubernetes cluster.
	err = prepAllDependencyTestResources(r.Client, mm)
	require.NoError(t, err)

	// Mock request to simulate Reconcile() being called on an event for a
	// watched resource .
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: mmName, Namespace: mmNamespace}}
	// Run Reconcile
	// We expect health check delay on the first reconciliation due to the deployment pods
	// not running yet.
	res, err := r.Reconcile(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, res, reconcile.Result{RequeueAfter: 6 * time.Second})

	// Define the NamespacedName objects that will be used to lookup the
	// cluster resources.
	mmKey := types.NamespacedName{Name: mmName, Namespace: mmNamespace}
	mmMysqlKey := types.NamespacedName{Name: utils.HashWithPrefix("db", mmName), Namespace: mmNamespace}
	mmMinioKey := types.NamespacedName{Name: mmName + "-minio", Namespace: mmNamespace}

	t.Run("observed generation updated", func(t *testing.T) {
		var fetchedMM mmv1beta.Mattermost
		err = c.Get(context.Background(), mmKey, &fetchedMM)
		require.NoError(t, err)
		assert.Equal(t, int64(1), fetchedMM.Status.ObservedGeneration)
	})

	t.Run("mysql", func(t *testing.T) {
		t.Run("cluster", func(t *testing.T) {
			mysql := &mysqlOperator.MysqlCluster{}
			err = c.Get(context.TODO(), mmMysqlKey, mysql)
			require.NoError(t, err)
		})
	})

	t.Run("minio", func(t *testing.T) {
		t.Run("instance", func(t *testing.T) {
			minio := &minioOperator.MinIOInstance{}
			err = c.Get(context.TODO(), mmMinioKey, minio)
			require.NoError(t, err)
		})
	})

	t.Run("mattermost", func(t *testing.T) {
		t.Run("service", func(t *testing.T) {
			service := &corev1.Service{}
			err = c.Get(context.TODO(), mmKey, service)
			require.NoError(t, err)
		})
		t.Run("ingress", func(t *testing.T) {
			ingress := &networkingv1.Ingress{}
			err = c.Get(context.TODO(), mmKey, ingress)
			require.NoError(t, err)
		})
		t.Run("deployment", func(t *testing.T) {
			deployment := &appsv1.Deployment{}
			err = c.Get(context.TODO(), mmKey, deployment)
			require.NoError(t, err)
			require.Equal(t, deployment.Spec.Replicas, mm.Spec.Replicas)
		})
	})

	t.Run("final check", func(t *testing.T) {

		t.Run("replica set does not exist", func(t *testing.T) {
			res, err = r.Reconcile(context.Background(), req)
			require.NoError(t, err)
			require.Equal(t, res, reconcile.Result{RequeueAfter: 6 * time.Second})
		})

		replicaSet := &appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mmName,
				Namespace: mmNamespace,
				Labels:    mm.MattermostLabels(mmName),
			},
			Status: appsv1.ReplicaSetStatus{},
		}
		err = c.Create(context.TODO(), replicaSet)
		require.NoError(t, err)

		t.Run("replica set not observed", func(t *testing.T) {
			res, err = r.Reconcile(context.Background(), req)
			require.NoError(t, err)
			require.Equal(t, res, reconcile.Result{RequeueAfter: 6 * time.Second})
		})
		replicaSet.Status.ObservedGeneration = 1
		err = c.Update(context.TODO(), replicaSet)
		require.NoError(t, err)

		// Create expected mattermost pods.
		podTemplate := corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: mmNamespace,
				Labels:    mm.MattermostLabels(mmName),
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Image: mm.GetImageName(),
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
			podTemplate.ObjectMeta.Name = fmt.Sprintf("%s-pod-%d", mmName, i)
			err = c.Create(context.TODO(), podTemplate.DeepCopy())
			require.NoError(t, err)
		}

		t.Run("pods not ready", func(t *testing.T) {
			res, err = r.Reconcile(context.Background(), req)
			require.NoError(t, err)
			require.Equal(t, res, reconcile.Result{RequeueAfter: 6 * time.Second})
		})

		// Make pods ready
		pods := &corev1.PodList{}
		err = c.List(context.TODO(), pods)
		require.NoError(t, err)

		for _, pod := range pods.Items {
			for i := 0; i < int(replicas); i++ {
				if pod.ObjectMeta.Name == fmt.Sprintf("%s-pod-%d", mmName, i) {
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
			res, err = r.Reconcile(context.Background(), req)
			require.NoError(t, err)
			require.Equal(t, res, reconcile.Result{})
		})

		// Make pods not running
		pods = &corev1.PodList{}
		err = c.List(context.TODO(), pods)
		require.NoError(t, err)

		for _, pod := range pods.Items {
			for i := 0; i < int(replicas); i++ {
				if pod.ObjectMeta.Name == fmt.Sprintf("%s-pod-%d", mmName, i) {
					pod.Status.Phase = corev1.PodPending
					err = c.Update(context.TODO(), pod.DeepCopy())
					require.NoError(t, err)
				}
			}
		}

		t.Run("pods not running", func(t *testing.T) {
			res, err = r.Reconcile(context.Background(), req)
			require.NoError(t, err)
			require.Equal(t, res, reconcile.Result{RequeueAfter: 6 * time.Second})
		})

		// Make pods running
		pods = &corev1.PodList{}
		err = c.List(context.TODO(), pods)
		require.NoError(t, err)

		for _, pod := range pods.Items {
			for i := 0; i < int(replicas); i++ {
				if pod.ObjectMeta.Name == fmt.Sprintf("%s-pod-%d", mmName, i) {
					pod.Status.Phase = corev1.PodRunning
					err = c.Update(context.TODO(), pod.DeepCopy())
					require.NoError(t, err)
				}
			}
		}

		t.Run("no reconcile errors", func(t *testing.T) {
			res, err = r.Reconcile(context.Background(), req)
			require.NoError(t, err)
			require.Equal(t, res, reconcile.Result{})
		})
		t.Run("correct status", func(t *testing.T) {
			err = c.Get(context.TODO(), mmKey, mm)
			require.NoError(t, err)
			assert.Equal(t, mm.Status.State, mmv1beta.Stable)
			assert.Equal(t, mm.Status.Replicas, *mm.Spec.Replicas)
			assert.Equal(t, mm.Status.Version, mm.Spec.Version)
			assert.Equal(t, mm.Status.Image, mm.Spec.Image)
			assert.Equal(t, mm.Status.Endpoint, mm.Spec.IngressName)
		})
	})
}

func TestReconcilingLimit(t *testing.T) {
	// Setup logging for the reconciler so we can see what happened on failure.
	logger := blubr.InitLogger()
	logger = logger.WithName("test.opr")
	logf.SetLogger(logger)

	mmNamespace := "default"
	replicas := int32(4)
	requeueOnLimitDelay := 35 * time.Second

	newMattermost := func(name string, uid string, state mmv1beta.RunningState) *mmv1beta.Mattermost {
		return &mmv1beta.Mattermost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: mmNamespace,
				UID:       types.UID(uid),
			},
			Spec: mmv1beta.MattermostSpec{
				Replicas:    &replicas,
				Image:       "mattermost/mattermost-enterprise-edition",
				Version:     operatortest.LatestStableMattermostVersion,
				IngressName: "foo.mattermost.dev",
			},
			Status: mmv1beta.MattermostStatus{State: state},
		}
	}

	mm1 := newMattermost("first", "1", mmv1beta.Reconciling)

	// Register operator types with the runtime scheme.
	s := prepareSchema(t, scheme.Scheme)
	s.AddKnownTypes(mmv1beta.GroupVersion, mm1)
	// Create a fake client to mock API calls.
	c := fake.NewFakeClient()
	// Create a ReconcileMattermost object with the scheme and fake client.
	r := &MattermostReconciler{
		Client:              c,
		Scheme:              s,
		Log:                 logger,
		MaxReconciling:      2,
		RequeueOnLimitDelay: requeueOnLimitDelay,
		Resources:           resources.NewResourceHelper(c, s),
	}

	assertInstallationsCount := func(t *testing.T, expectedCIs, expectedReconciling int) {
		var mmList mmv1beta.MattermostList
		err := c.List(context.TODO(), &mmList)
		require.NoError(t, err)

		assert.Equal(t, expectedCIs, len(mmList.Items))
		assert.Equal(t, expectedReconciling, countReconciling(mmList.Items))
	}

	err := c.Create(context.TODO(), mm1)
	require.NoError(t, err)

	mm2 := newMattermost("second", "2", mmv1beta.Reconciling)
	err = c.Create(context.TODO(), mm2)
	require.NoError(t, err)

	mm3 := newMattermost("third", "3", mmv1beta.Reconciling)
	err = c.Create(context.TODO(), mm3)
	require.NoError(t, err)

	mm4 := newMattermost("forth", "4", "")
	err = c.Create(context.TODO(), mm4)
	require.NoError(t, err)

	mm5 := newMattermost("fifth", "5", mmv1beta.Stable)
	err = c.Create(context.TODO(), mm5)
	require.NoError(t, err)

	req1 := requestForCI(mm1)
	_, err = r.Reconcile(context.Background(), req1)
	require.Error(t, err)
	assertInstallationsCount(t, 5, 3)

	req2 := requestForCI(mm2)
	_, err = r.Reconcile(context.Background(), req2)
	require.Error(t, err)

	t.Run("should pick up Installation in Reconciling state even if limit reached", func(t *testing.T) {
		req3 := requestForCI(mm3)
		_, err = r.Reconcile(context.Background(), req3)
		require.Error(t, err)
	})

	var result reconcile.Result
	t.Run("should not pick up Installation without state if limit reached", func(t *testing.T) {
		req4 := requestForCI(mm4)
		result, err = r.Reconcile(context.Background(), req4)
		require.NoError(t, err)
		assert.Equal(t, requeueOnLimitDelay, result.RequeueAfter)

		result, err = r.Reconcile(context.Background(), req4)
		require.NoError(t, err)
		assert.Equal(t, requeueOnLimitDelay, result.RequeueAfter)
	})

	t.Run("should not pick up Installation in Stable state if limit reached", func(t *testing.T) {
		req5 := requestForCI(mm5)
		result, err = r.Reconcile(context.Background(), req5)
		require.NoError(t, err)
		assert.Equal(t, requeueOnLimitDelay, result.RequeueAfter)
	})

	err = c.Delete(context.TODO(), mm1)
	require.NoError(t, err)
	_, err = r.Reconcile(context.Background(), req1)
	require.NoError(t, err)
	assertInstallationsCount(t, 4, 2)

	err = c.Delete(context.TODO(), mm2)
	require.NoError(t, err)
	_, err = r.Reconcile(context.Background(), req2)
	require.NoError(t, err)
	assertInstallationsCount(t, 3, 1)

	t.Run("should pick up Installation without state when cache freed", func(t *testing.T) {
		req4 := requestForCI(mm4)
		_, err = r.Reconcile(context.Background(), req4)
		require.Error(t, err)
		assertInstallationsCount(t, 3, 2)
	})

	err = c.Delete(context.TODO(), mm4)
	require.NoError(t, err)
	req4 := requestForCI(mm4)
	_, err = r.Reconcile(context.Background(), req4)
	require.NoError(t, err)
	assertInstallationsCount(t, 2, 1)

	t.Run("should pick up Installation in Stable state when cache freed", func(t *testing.T) {
		req5 := requestForCI(mm5)
		_, err = r.Reconcile(context.Background(), req5)
		require.Error(t, err)
		assertInstallationsCount(t, 2, 2)
	})

	err = c.Delete(context.TODO(), mm5)
	require.NoError(t, err)
	req5 := requestForCI(mm5)
	_, err = r.Reconcile(context.Background(), req5)
	require.NoError(t, err)
	assertInstallationsCount(t, 1, 1)

	t.Run("should add new installations to cache", func(t *testing.T) {
		// Pick up first for reconciling
		mm6 := newMattermost("sixth", "6", "")
		err = c.Create(context.TODO(), mm6)
		require.NoError(t, err)
		req6 := requestForCI(mm6)
		_, err = r.Reconcile(context.Background(), req6)
		require.Error(t, err)
		assertInstallationsCount(t, 2, 2)

		// Do not pick up second
		mm7 := newMattermost("seventh", "7", "")
		err = c.Create(context.TODO(), mm7)
		require.NoError(t, err)
		req7 := requestForCI(mm7)
		result, err = r.Reconcile(context.Background(), req7)
		require.NoError(t, err)
		assert.Equal(t, requeueOnLimitDelay, result.RequeueAfter)
		assertInstallationsCount(t, 3, 2)
	})
}

func requestForCI(mattermost *mmv1beta.Mattermost) reconcile.Request {
	return reconcile.Request{NamespacedName: types.NamespacedName{Name: mattermost.Name, Namespace: mattermost.Namespace}}
}

func prepAllDependencyTestResources(client client.Client, mattermost *mmv1beta.Mattermost) error {
	minioService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mattermost.Name + "-minio-hl-svc",
			Namespace: mattermost.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports:     []corev1.ServicePort{{Port: 9000}},
			ClusterIP: corev1.ClusterIPNone,
		},
	}

	return client.Create(context.TODO(), minioService)
}

func prepareSchema(t *testing.T, scheme *runtime.Scheme) *runtime.Scheme {
	err := mmv1beta.AddToScheme(scheme)
	require.NoError(t, err)
	err = v1beta1Minio.AddToScheme(scheme)
	require.NoError(t, err)
	err = v1alpha1MySQL.SchemeBuilder.AddToScheme(scheme)
	require.NoError(t, err)

	return scheme
}
