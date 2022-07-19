package mattermost

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sync"
	"time"

	"github.com/mattermost/mattermost-operator/pkg/resources"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"

	batchv1 "k8s.io/api/batch/v1"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const healthCheckRequeueDelay = 6 * time.Second
const resourcesReadyDelay = 10 * time.Second

// MattermostReconciler reconciles a Mattermost object
type MattermostReconciler struct {
	client.Client
	NonCachedAPIReader     client.Reader
	Log                    logr.Logger
	Scheme                 *runtime.Scheme
	MaxReconciling         int
	RequeueOnLimitDelay    time.Duration
	Resources              *resources.ResourceHelper
	reconcilingRateLimiter unstableInstallationsRateLimiter
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
		reconcilingRateLimiter: unstableInstallationsRateLimiter{
			nonReconcilingBeingProcessed: 0,
			Mutex:                        sync.Mutex{},
		},
	}
}

func (r *MattermostReconciler) SetupWithManager(mgr ctrl.Manager, maxConcurrency int) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mmv1beta.Mattermost{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Secret{}).
		Owns(&networkingv1.Ingress{}).
		Owns(&appsv1.Deployment{}).
		Owns(&batchv1.Job{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: maxConcurrency,
		}).
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
		var canProcess bool
		canProcess, err = r.startNonReconcilingMMProcessing(ctx, reqLogger)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to verify reconciliation limit")
		}
		if !canProcess {
			reqLogger.Info(fmt.Sprintf("Reached limit of reconciling installations, requeuing in %s", r.RequeueOnLimitDelay.String()))
			return reconcile.Result{RequeueAfter: r.RequeueOnLimitDelay}, nil
		}
		defer func() {
			// We only count MMs that are being processed but are not in
			// `reconciling` state, therefore when the function exists,
			// regardless if the MM will be marked as `reconciling` or not we
			// can decrement the counter. Status update will occur before
			// decrement, so we are not risking races.
			r.reconcilingRateLimiter.decrementProcessing()
		}()
	}

	// We copy status to not to refetch the resource
	status := mattermost.Status
	// Indicate that the newest generation of the resource has been observed.
	status.ObservedGeneration = mattermost.Generation

	// Set a new Mattermost's state to reconciling.
	if len(mattermost.Status.State) == 0 {
		err = r.updateStatusReconciling(mattermost, status, reqLogger)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	// Set defaults and update the resource with said defaults if anything is
	// different.
	originalMattermost := mattermost.DeepCopy()
	err = mattermost.SetDefaults()
	if err != nil {
		r.updateStatusReconcilingAndLogError(mattermost, status, reqLogger, err)
		return reconcile.Result{}, err
	}

	softError := mattermost.SetReplicasAndResourcesFromSize()
	if softError != nil {
		reqLogger.Error(softError, "Error setting replicas and resources from size. Using default values")
	}

	if !reflect.DeepEqual(originalMattermost.Spec, mattermost.Spec) {
		mattermost.Status = status
		err = r.updateSpec(ctx, reqLogger, mattermost)
		if err != nil {
			r.updateStatusReconcilingAndLogError(originalMattermost, status, reqLogger, err)
			return reconcile.Result{}, err
		}
	}

	dbConfig, err := r.checkDatabase(mattermost, reqLogger)
	if err != nil {
		r.updateStatusReconcilingAndLogError(mattermost, status, reqLogger, err)
		return reconcile.Result{}, err
	}

	fileStoreConfig, err := r.checkFileStore(mattermost, reqLogger)
	if err != nil {
		r.updateStatusReconcilingAndLogError(mattermost, status, reqLogger, err)
		return reconcile.Result{}, err
	}

	recStatus, err := r.checkMattermost(mattermost, dbConfig, fileStoreConfig, &status, reqLogger)
	if err != nil {
		r.updateStatusReconcilingAndLogError(mattermost, status, reqLogger, err)
		return reconcile.Result{}, err
	}

	if !recStatus.ResourcesReady {
		reqLogger.Info("Mattermost resources not ready, delaying for 10 seconds!")
		return ctrl.Result{RequeueAfter: resourcesReadyDelay}, nil
	}

	status, err = r.checkMattermostHealth(mattermost, status, reqLogger)
	if err != nil {
		statusErr := r.updateStatus(mattermost, status, reqLogger)
		if statusErr != nil {
			reqLogger.Error(statusErr, "Error updating status")
		}
		reqLogger.Info("Mattermost instance not healthy", "msg", err.Error())
		return reconcile.Result{RequeueAfter: healthCheckRequeueDelay}, nil
	}

	err = r.updateStatus(mattermost, status, reqLogger)
	if err != nil {
		r.updateStatusReconcilingAndLogError(mattermost, status, reqLogger, err)
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *MattermostReconciler) updateSpec(ctx context.Context, reqLogger logr.Logger, updated *mmv1beta.Mattermost) error {
	reqLogger.Info("Updating Mattermost spec")
	return r.Client.Update(ctx, updated)
}

// startNonReconcilingMMProcessing verifies if the new Mattermost in
// non-reconciling state can be currently processed by the Operator by checking
// if the rate limit has been reached.
// Returns false when rate limit is reached and processing cannot be started.
func (r *MattermostReconciler) startNonReconcilingMMProcessing(ctx context.Context, reqLogger logr.Logger) (bool, error) {
	r.reconcilingRateLimiter.Lock()
	defer r.reconcilingRateLimiter.Unlock()

	var mmListInstallations mmv1beta.MattermostList
	err := r.Client.List(ctx, &mmListInstallations)
	if err != nil {
		return false, errors.Wrap(err, "failed to list Mattermosts")
	}

	// Check if limit of Mattermosts reconciling at the same time is reached.
	if countReconciling(mmListInstallations.Items)+r.reconcilingRateLimiter.nonReconcilingBeingProcessed >= r.MaxReconciling {
		reqLogger.Info(fmt.Sprintf("Reached limit of reconciling or processing installations, requeuing in %s", r.RequeueOnLimitDelay.String()))
		return false, nil
	}

	r.reconcilingRateLimiter.nonReconcilingBeingProcessed += 1

	return true, nil
}

type unstableInstallationsRateLimiter struct {
	// Number of CRs that are being actively processed by the reconciler but are
	// not (yet) in Reconciling state. To respect the rate limit with multiple
	// reconcilers we need to sync with mutex.
	nonReconcilingBeingProcessed int
	sync.Mutex
}

func (rl *unstableInstallationsRateLimiter) decrementProcessing() {
	rl.Lock()
	defer rl.Unlock()
	rl.nonReconcilingBeingProcessed -= 1
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
