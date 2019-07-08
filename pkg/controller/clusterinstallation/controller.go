package clusterinstallation

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1beta1 "k8s.io/api/extensions/v1beta1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
)

var log = logf.Log.WithName("clusterinstallation.controller")

// Add creates a new ClusterInstallation Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileClusterInstallation{client: mgr.GetClient(), scheme: mgr.GetScheme(), state: mattermostv1alpha1.Reconciling}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("clusterinstallation", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource ClusterInstallation
	err = c.Watch(&source.Kind{Type: &mattermostv1alpha1.ClusterInstallation{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &mattermostv1alpha1.ClusterInstallation{},
	})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &mattermostv1alpha1.ClusterInstallation{},
	})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &v1beta1.Ingress{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &mattermostv1alpha1.ClusterInstallation{},
	})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &mattermostv1alpha1.ClusterInstallation{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileClusterInstallation{}

// ReconcileClusterInstallation reconciles a ClusterInstallation object
type ReconcileClusterInstallation struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver.
	client client.Client
	scheme *runtime.Scheme
	state  mattermostv1alpha1.RunningState
}

func (r *ReconcileClusterInstallation) setReconciling() {
	r.state = mattermostv1alpha1.Reconciling
}

func (r *ReconcileClusterInstallation) setStable() {
	r.state = mattermostv1alpha1.Stable
}

// Reconcile reads the state of the cluster for a ClusterInstallation object and
// makes changes to obtain the exact state defined in `ClusterInstallation.Spec`.
//
// Note:
// The Controller will requeue the Request to be processed again if the returned
// error is non-nil or Result.Requeue is true, otherwise upon completion it will
// remove the work from the queue.
func (r *ReconcileClusterInstallation) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling ClusterInstallation")

	// Fetch the ClusterInstallation instance
	mattermost := &mattermostv1alpha1.ClusterInstallation{}
	err := r.client.Get(context.TODO(), request.NamespacedName, mattermost)
	if err != nil && k8sErrors.IsNotFound(err) {
		// Request object not found, could have been deleted after reconcile request.
		// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
		// Return and don't requeue
		r.setReconciling()
		return reconcile.Result{}, nil
	} else if err != nil {
		// Error reading the object - requeue the request.
		r.setReconciling()
		return reconcile.Result{}, err
	}

	if mattermost.Status.State != r.state {
		status := mattermost.Status
		status.State = r.state
		err = r.updateStatus(mattermost, status, reqLogger)
		if err != nil {
			r.setReconciling()
			return reconcile.Result{}, err
		}
	}

	// Set defaults and update the resource with said defaults if anything is
	// different.
	originalMattermost := mattermost.DeepCopy()
	err = mattermost.SetDefaults()
	if err != nil {
		r.setReconciling()
		return reconcile.Result{}, err
	}

	softError := mattermost.SetReplicasAndResourcesFromSize()
	if softError != nil {
		reqLogger.Error(softError, "Error setting replicas and resources from user count")
	}

	if !reflect.DeepEqual(originalMattermost.Spec, mattermost.Spec) {
		reqLogger.Info(fmt.Sprintf("Updating spec"),
			"Old", fmt.Sprintf("%+v", originalMattermost.Spec),
			"New", fmt.Sprintf("%+v", mattermost.Spec),
		)
		err = r.client.Update(context.TODO(), mattermost)
		if err != nil {
			reqLogger.Error(err, "failed to update the clusterinstallation spec")
			r.setReconciling()
			return reconcile.Result{}, err
		}
	}

	err = r.checkDatabase(mattermost, reqLogger)
	if err != nil {
		r.setReconciling()
		return reconcile.Result{}, err
	}

	err = r.checkMinio(mattermost, reqLogger)
	if err != nil {
		r.setReconciling()
		return reconcile.Result{}, err
	}

	err = r.checkMattermost(mattermost, reqLogger)
	if err != nil {
		r.setReconciling()
		return reconcile.Result{}, err
	}

	status, err := r.checkClusterInstallation(mattermost)
	if err != nil {
		r.setReconciling()
		r.updateStatus(mattermost, status, reqLogger)
		return reconcile.Result{RequeueAfter: time.Second * 3}, err
	}
	err = r.updateStatus(mattermost, status, reqLogger)
	if err != nil {
		r.setReconciling()
		return reconcile.Result{}, err
	}
	r.setStable()

	return reconcile.Result{}, nil
}

func (r *ReconcileClusterInstallation) checkDatabase(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	if mattermost.Spec.Database.ExternalSecret != "" {
		err := r.checkSecret(mattermost.Spec.Database.ExternalSecret, "externalDB", mattermost.Namespace)
		if err != nil {
			return errors.Wrap(err, "Error getting the external database secret.")
		}
	}

	switch mattermost.Spec.Database.Type {
	case "mysql":
		return r.checkMySQLCluster(mattermost, reqLogger)
	case "postgres":
		return r.checkPostgres(mattermost, reqLogger)
	}

	return k8sErrors.NewInvalid(mattermostv1alpha1.SchemeGroupVersion.WithKind("ClusterInstallation").GroupKind(), "Database type invalid", nil)
}
