package mattermost

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/mattermost/mattermost-operator/pkg/resources"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"

	batchv1 "k8s.io/api/batch/v1"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1beta1 "k8s.io/api/networking/v1beta1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const healthCheckRequeueDelay = 6 * time.Second

// MattermostReconciler reconciles a Mattermost object
type MattermostReconciler struct {
	client.Client
	NonCachedAPIReader  client.Reader
	Log                 logr.Logger
	Scheme              *runtime.Scheme
	MaxReconciling      int
	RequeueOnLimitDelay time.Duration
	Resources           *resources.ResourceHelper
}

func NewMattermostReconciler(mgr ctrl.Manager, maxReconciling int, requeueOnLimitDelay time.Duration) *MattermostReconciler {
	return &MattermostReconciler{
		Client:              mgr.GetClient(),
		NonCachedAPIReader:  mgr.GetAPIReader(),
		Log:                 ctrl.Log.WithName("controllers").WithName("Mattermost"),
		Scheme:              mgr.GetScheme(),
		MaxReconciling:      maxReconciling,
		RequeueOnLimitDelay: requeueOnLimitDelay,
		Resources:           resources.NewResourceHelper(mgr.GetClient(), mgr.GetScheme()),
	}
}

func (r *MattermostReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mmv1beta.Mattermost{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Secret{}).
		Owns(&v1beta1.Ingress{}).
		Owns(&appsv1.Deployment{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}

// Reconcile reads the state of the cluster for a Mattermost object and
// makes changes to obtain the exact state defined in `Mattermost.Spec`.
//
// Note:
// The Controller will requeue the Request to be processed again if the returned
// error is non-nil or Result. Requeue is true, otherwise upon completion it will
// remove the work from the queue.
func (r *MattermostReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Mattermost")

	// Fetch the Mattermost.
	mattermost := &mmv1beta.Mattermost{}
	err := r.Client.Get(ctx, request.NamespacedName, mattermost)
	if err != nil && k8sErrors.IsNotFound(err) {
		// Request object not found, could have been deleted after reconcile
		// request. Owned objects are automatically garbage collected.
		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, err
	}

	if mattermost.Status.State != mmv1beta.Reconciling {
		var mmListInstallations mmv1beta.MattermostList
		err = r.Client.List(ctx, &mmListInstallations)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to list Mattermosts")
		}

		// Check if limit of Mattermosts reconciling at the same time is reached.
		if countReconciling(mmListInstallations.Items) >= r.MaxReconciling {
			reqLogger.Info(fmt.Sprintf("Reached limit of reconciling installations, requeuing in %s", r.RequeueOnLimitDelay.String()))
			return ctrl.Result{RequeueAfter: r.RequeueOnLimitDelay}, nil
		}
	}

	// Set a new Mattermost's state to reconciling.
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
		reqLogger.Error(softError, "Error setting replicas and resources from size. Using default values")
	}

	if !reflect.DeepEqual(originalMattermost.Spec, mattermost.Spec) {
		err = r.updateSpec(ctx, reqLogger, originalMattermost, mattermost)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	dbConfig, err := r.checkDatabase(mattermost, reqLogger)
	if err != nil {
		r.setStateReconcilingAndLogError(mattermost, reqLogger)
		return reconcile.Result{}, err
	}

	fileStoreConfig, err := r.checkFileStore(mattermost, reqLogger)
	if err != nil {
		r.setStateReconcilingAndLogError(mattermost, reqLogger)
		return reconcile.Result{}, err
	}

	err = r.checkMattermost(mattermost, dbConfig, fileStoreConfig, reqLogger)
	if err != nil {
		r.setStateReconcilingAndLogError(mattermost, reqLogger)
		return reconcile.Result{}, err
	}

	status, err := r.checkMattermostHealth(mattermost, reqLogger)
	if err != nil {
		statusErr := r.updateStatus(mattermost, status, reqLogger)
		if statusErr != nil {
			reqLogger.Error(statusErr, "Error updating status")
		}
		reqLogger.Error(err, "Error checking Mattermost health")
		return reconcile.Result{RequeueAfter: healthCheckRequeueDelay}, nil
	}

	err = r.updateStatus(mattermost, status, reqLogger)
	if err != nil {
		r.setStateReconcilingAndLogError(mattermost, reqLogger)
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *MattermostReconciler) updateSpec(ctx context.Context, reqLogger logr.Logger, originalMattermost *mmv1beta.Mattermost, updated *mmv1beta.Mattermost) error {
	reqLogger.Info(fmt.Sprintf("Updating spec"),
		"Old", fmt.Sprintf("%+v", originalMattermost.Spec),
		"New", fmt.Sprintf("%+v", updated.Spec),
	)
	err := r.Client.Update(ctx, updated)
	if err != nil {
		reqLogger.Error(err, "failed to update the Mattermost spec")
		r.setStateReconcilingAndLogError(updated, reqLogger)
		return err
	}
	return nil
}

func countReconciling(mattermosts []mmv1beta.Mattermost) int {
	sum := 0
	for _, ci := range mattermosts {
		if ci.Status.State == mmv1beta.Reconciling {
			sum++
		}
	}
	return sum
}
