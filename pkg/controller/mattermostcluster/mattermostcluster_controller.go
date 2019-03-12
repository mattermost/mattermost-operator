package mattermostcluster

import (
	"context"
	"fmt"
	"strconv"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
	"github.com/mattermost/mattermost-operator/pkg/controller/mattermostcluster/constants"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_mattermostcluster")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new MattermostCluster Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileMattermostCluster{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("mattermostcluster-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource MattermostCluster
	err = c.Watch(&source.Kind{Type: &mattermostv1alpha1.MattermostCluster{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner MattermostCluster
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &mattermostv1alpha1.MattermostCluster{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileMattermostCluster{}

// ReconcileMattermostCluster reconciles a MattermostCluster object
type ReconcileMattermostCluster struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a MattermostCluster object and makes changes based on the state read
// and what is in the MattermostCluster.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileMattermostCluster) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling MattermostCluster")

	// Fetch the MattermostCluster instance
	mattermost := &mattermostv1alpha1.MattermostCluster{}
	err := r.client.Get(context.TODO(), request.NamespacedName, mattermost)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	err = r.setDefaults(mattermost, reqLogger)
	if err != nil {
		return reconcile.Result{}, err
	}

	if mattermost.Spec.DatabaseType.Type == "mysql" {
		err = r.checkDBMySQLDeployment(mattermost, reqLogger)
		if err != nil {
			return reconcile.Result{}, err
		}
	} else {
		err = r.checkDBPostgresDeployment(mattermost, reqLogger)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	err = r.checkMinioDeployment(mattermost, reqLogger)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = r.getSecrets(mattermost, reqLogger)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = r.checkMattermostDeployment(mattermost, reqLogger)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Already exists - don't requeue
	return reconcile.Result{}, nil
}

func (r *ReconcileMattermostCluster) setDefaults(mattermost *mattermostv1alpha1.MattermostCluster, reqLogger logr.Logger) error {
	if len(mattermost.Spec.IngressName) == 0 {
		return fmt.Errorf("need to set the IngressName")
	}

	if len(mattermost.Spec.Image) == 0 {
		reqLogger.Info("Setting default Mattermost image: " + constants.DefaultMattermostImage)
		mattermost.Spec.Image = constants.DefaultMattermostImage
	}

	if mattermost.Spec.Replicas == 0 {
		reqLogger.Info("Setting default Mattermost replicas: " + strconv.Itoa(constants.DefaultAmountOfPods))
		mattermost.Spec.Replicas = constants.DefaultAmountOfPods
	}

	if len(mattermost.Spec.DatabaseType.Type) == 0 {
		reqLogger.Info("Setting default Mattermost database type: " + constants.DefaultMattermostDatabaseType)
		mattermost.Spec.DatabaseType.Type = constants.DefaultMattermostDatabaseType
	}

	return nil
}

func (r *ReconcileMattermostCluster) checkDBMySQLDeployment(mattermost *mattermostv1alpha1.MattermostCluster, reqLogger logr.Logger) error {
	dbExist := false

	//TODO: Create the logic to check if the DB MySQL already exist or changed otherwise create
	if dbExist {
		return r.client.Update(context.TODO(), mattermost)
	}
	return nil
}

func (r *ReconcileMattermostCluster) checkDBPostgresDeployment(mattermost *mattermostv1alpha1.MattermostCluster, reqLogger logr.Logger) error {
	dbExist := false

	//TODO: Create the logic to check if the DB Postgres already exist or changed otherwise create
	if dbExist {
		return r.client.Update(context.TODO(), mattermost)
	}
	return nil
}

func (r *ReconcileMattermostCluster) checkMinioDeployment(mattermost *mattermostv1alpha1.MattermostCluster, reqLogger logr.Logger) error {
	minioExist := false

	//TODO: Create the logic to check if the Minio already exist or changed otherwise create
	if minioExist {
		return r.client.Update(context.TODO(), mattermost)
	}
	return nil
}

func (r *ReconcileMattermostCluster) getSecrets(mattermost *mattermostv1alpha1.MattermostCluster, reqLogger logr.Logger) error {
	//TODO: Create the logic to get the secretsc created by DB / Minio or read from the spec
	return nil
}

func (r *ReconcileMattermostCluster) checkMattermostDeployment(mattermost *mattermostv1alpha1.MattermostCluster, reqLogger logr.Logger) error {
	//TODO: Create the logic to deploy MM including service and ingress
	return nil
}
