package clusterinstallation

import (
	"context"
	"fmt"
	"reflect"
	"time"

	batchv1 "k8s.io/api/batch/v1"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1beta1 "k8s.io/api/extensions/v1beta1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/apis/mattermost/v1alpha1"
	"github.com/mattermost/mattermost-operator/pkg/database"
)

const healthCheckRequeueDelay = 6 * time.Second

// ClusterInstallationReconciler reconciles a ClusterInstallation object
type ClusterInstallationReconciler struct {
	client.Client
	NonCachedAPIReader  client.Reader
	Log                 logr.Logger
	Scheme              *runtime.Scheme
	MaxReconciling      int
	RequeueOnLimitDelay time.Duration
}

// +kubebuilder:rbac:groups=mattermost.com,resources=clusterinstallations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mattermost.com,resources=clusterinstallations/status,verbs=get;update;patch

func (r *ClusterInstallationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mattermostv1alpha1.ClusterInstallation{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Secret{}).
		Owns(&v1beta1.Ingress{}).
		Owns(&appsv1.Deployment{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}

// Reconcile reads the state of the cluster for a ClusterInstallation object and
// makes changes to obtain the exact state defined in `ClusterInstallation.Spec`.
//
// Note:
// The Controller will requeue the Request to be processed again if the returned
// error is non-nil or Result.Requeue is true, otherwise upon completion it will
// remove the work from the queue.
func (r *ClusterInstallationReconciler) Reconcile(request ctrl.Request) (ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling ClusterInstallation")

	// Fetch the ClusterInstallation.
	mattermost := &mattermostv1alpha1.ClusterInstallation{}
	err := r.Client.Get(context.TODO(), request.NamespacedName, mattermost)
	if err != nil && k8sErrors.IsNotFound(err) {
		// Request object not found, could have been deleted after reconcile
		// request. Owned objects are automatically garbage collected. For
		// additional cleanup logic use finalizers. Return and don't requeue.
		return reconcile.Result{}, nil
	} else if err != nil {
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	if mattermost.Status.State != mattermostv1alpha1.Reconciling {
		var clusterInstallations mattermostv1alpha1.ClusterInstallationList
		err = r.Client.List(context.TODO(), &clusterInstallations)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to list ClusterInstallations")
		}

		// Check if limit of Cluster Installations reconciling at the same time is reached.
		if countReconciling(clusterInstallations.Items) >= r.MaxReconciling {
			reqLogger.Info(fmt.Sprintf("Reached limit of reconciling installations, requeuing in %s", r.RequeueOnLimitDelay.String()))
			return ctrl.Result{RequeueAfter: r.RequeueOnLimitDelay}, nil
		}
	}

	// Set a new ClusterInstallation's state to reconciling.
	if len(mattermost.Status.State) == 0 {
		err = r.setStateReconciling(mattermost, reqLogger)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	// Set defaults and update the resource with said defaults if anything is
	// different.
	originalMattermost := mattermost.DeepCopy()
	err = mattermost.SetDefaults()
	if err != nil {
		r.setStateReconcilingAndLogError(mattermost, reqLogger)
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
		err = r.Client.Update(context.TODO(), mattermost)
		if err != nil {
			reqLogger.Error(err, "failed to update the clusterinstallation spec")
			r.setStateReconcilingAndLogError(mattermost, reqLogger)
			return reconcile.Result{}, err
		}
	}

	err = r.checkDatabase(mattermost, reqLogger)
	if err != nil {
		r.setStateReconcilingAndLogError(mattermost, reqLogger)
		return reconcile.Result{}, err
	}

	err = r.checkMinio(mattermost, reqLogger)
	if err != nil {
		r.setStateReconcilingAndLogError(mattermost, reqLogger)
		return reconcile.Result{}, err
	}

	err = r.checkMattermost(mattermost, reqLogger)
	if err != nil {
		r.setStateReconcilingAndLogError(mattermost, reqLogger)
		return reconcile.Result{}, err
	}

	err = r.checkBlueGreen(mattermost, reqLogger)
	if err != nil {
		r.setStateReconcilingAndLogError(mattermost, reqLogger)
		return reconcile.Result{}, err
	}

	err = r.checkCanary(mattermost, reqLogger)
	if err != nil {
		r.setStateReconcilingAndLogError(mattermost, reqLogger)
		return reconcile.Result{}, err
	}

	status, err := r.handleCheckClusterInstallation(mattermost)
	if err != nil {
		statusErr := r.updateStatus(mattermost, status, reqLogger)
		if statusErr != nil {
			reqLogger.Error(statusErr, "Error updating status")
		}
		reqLogger.Error(err, "Error checking ClusterInstallation health")
		return reconcile.Result{RequeueAfter: healthCheckRequeueDelay}, nil
	}

	err = r.updateStatus(mattermost, status, reqLogger)
	if err != nil {
		r.setStateReconcilingAndLogError(mattermost, reqLogger)
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ClusterInstallationReconciler) checkDatabase(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	// Check for an existing secret and determine which type it is (User-Managed
	// or Operator-Manged). See the Database spec to learn more on this.
	if mattermost.Spec.Database.Secret != "" {
		databaseSecret := &corev1.Secret{}
		err := r.Client.Get(context.TODO(), types.NamespacedName{Name: mattermost.Spec.Database.Secret, Namespace: mattermost.Namespace}, databaseSecret)
		if err != nil {
			return errors.Wrap(err, "failed to get database secret")
		}

		dbInfo := database.GenerateDatabaseInfoFromSecret(databaseSecret)
		err = dbInfo.IsValid()
		if err != nil {
			return errors.Wrap(err, "database secret is not valid")
		}

		if dbInfo.External {
			return nil
		}
	}

	switch mattermost.Spec.Database.Type {
	case "mysql":
		return r.checkMySQLCluster(mattermost, reqLogger)
	case "postgres":
		return r.checkPostgres(mattermost, reqLogger)
	}

	return k8sErrors.NewInvalid(mattermostv1alpha1.GroupVersion.WithKind("ClusterInstallation").GroupKind(), "Database type invalid", nil)
}

func countReconciling(clusterInstallations []mattermostv1alpha1.ClusterInstallation) int {
	sum := 0
	for _, ci := range clusterInstallations {
		if ci.Status.State == mattermostv1alpha1.Reconciling {
			sum++
		}
	}
	return sum
}
