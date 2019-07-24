// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package bluegreen

import (
	"context"
	"time"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	v1beta1 "k8s.io/api/extensions/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_bluegreen")

// Add creates a new BlueGreen Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileBlueGreen{client: mgr.GetClient(), scheme: mgr.GetScheme(), state: mattermostv1alpha1.Reconciling}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("bluegreen-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource BlueGreen
	err = c.Watch(&source.Kind{Type: &mattermostv1alpha1.BlueGreen{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &mattermostv1alpha1.BlueGreen{},
	})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &v1beta1.Ingress{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &mattermostv1alpha1.BlueGreen{},
	})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &mattermostv1alpha1.BlueGreen{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileBlueGreen implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileBlueGreen{}

// ReconcileBlueGreen reconciles a BlueGreen object
type ReconcileBlueGreen struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
	state  mattermostv1alpha1.RunningState
}


func (r *ReconcileBlueGreen) setReconciling() {
	r.state = mattermostv1alpha1.Reconciling
}

func (r *ReconcileBlueGreen) setStable() {
	r.state = mattermostv1alpha1.Stable
}

// Reconcile reads that state of the cluster for a BlueGreen object and makes changes based on the state read
// and what is in the BlueGreen.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileBlueGreen) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling BlueGreen")

	// Fetch the BlueGreen instance
	blueGreen := &mattermostv1alpha1.BlueGreen{}
	err := r.client.Get(context.TODO(), request.NamespacedName, blueGreen)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			r.setReconciling()
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		r.setReconciling()
		return reconcile.Result{}, err
	}

	mattermost := &mattermostv1alpha1.ClusterInstallation{}
	key := types.NamespacedName{Namespace: request.Namespace, Name: blueGreen.Spec.InstallationName}
	err = r.client.Get(context.TODO(), key, mattermost)
	if err != nil && errors.IsNotFound(err) {
		// Request object not found, could have been deleted after reconcile request.
		// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
		// Return and don't requeue
		return reconcile.Result{}, nil
	} else if err != nil {
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	if blueGreen.Status.State != r.state {
		status := blueGreen.Status
		status.State = r.state
		err = r.updateStatus(blueGreen, status, reqLogger)
		if err != nil {
			r.setReconciling()
			return reconcile.Result{}, err
		}
	}

	err = r.checkBlueGreen(blueGreen, mattermost, reqLogger)
	if err != nil {
		r.setReconciling()
		return reconcile.Result{}, err
	}

	status, err := r.checkBlueGreenStatus(blueGreen, mattermost)
	if err != nil {
		r.setReconciling()
		r.updateStatus(blueGreen, status, reqLogger)
		return reconcile.Result{RequeueAfter: time.Second * 3}, err
	}

	err = r.updateStatus(blueGreen, status, reqLogger)
	if err != nil {
		r.setReconciling()
		return reconcile.Result{}, err
	}
	r.setStable()
	return reconcile.Result{}, nil
}
