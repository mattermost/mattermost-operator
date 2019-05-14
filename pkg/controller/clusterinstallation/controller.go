package clusterinstallation

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1beta1 "k8s.io/api/extensions/v1beta1"
	rbacv1beta1 "k8s.io/api/rbac/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
	if err != nil && errors.IsNotFound(err) {
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

	err = r.checkMinioSecret(mattermost, reqLogger.WithValues("Reconcile.Minio", "secret"))
	if err != nil {
		r.setReconciling()
		return reconcile.Result{}, err
	}
	err = r.checkMinioDeployment(mattermost, reqLogger.WithValues("Reconcile.Minio", "deployment"))
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
		return reconcile.Result{}, err
	}
	err = r.updateStatus(mattermost, *status, reqLogger)
	if err != nil {
		r.setReconciling()
		return reconcile.Result{}, err
	}
	r.setStable()

	return reconcile.Result{}, nil
}

// Object combines the interfaces that all Kubernetes objects must implement.
type Object interface {
	runtime.Object
	v1.Object
}

func (r *ReconcileClusterInstallation) checkDatabase(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	if mattermost.Spec.DatabaseType.ExternalDatabaseSecret != "" {
		errSecret := r.checkSecret(mattermost.Spec.DatabaseType.ExternalDatabaseSecret, mattermost.Namespace)
		if errSecret != nil {
			return errSecret
		}
	} else {
		switch mattermost.Spec.DatabaseType.Type {
		case "mysql":
			dbLogger := reqLogger.WithValues("Reconcile.Database", "mysql")

			err := r.checkMySQLServiceAccount(mattermost, dbLogger)
			if err != nil {
				return err
			}

			err = r.checkMySQLRoleBinding(mattermost, dbLogger)
			if err != nil {
				return err
			}

			err = r.checkMySQLDeployment(mattermost, dbLogger)
			if err != nil {
				return err
			}
		case "postgres":
			dbLogger := reqLogger.WithValues("Reconcile.Database", "postgres")

			err := r.checkDBPostgresDeployment(mattermost, dbLogger)
			if err != nil {
				return err
			}
		case "default":
			errInvalid := errors.NewInvalid(mattermostv1alpha1.SchemeGroupVersion.WithKind("ClusterInstallation").GroupKind(), "Database type invalid", nil)
			return errInvalid
		}
	}

	return nil
}

// createResource creates the provided resource and sets the owner
func (r *ReconcileClusterInstallation) createResource(owner v1.Object, resource Object, reqLogger logr.Logger) error {
	err := r.client.Create(context.TODO(), resource)
	if err != nil {
		reqLogger.Error(err, "Error creating resource")
		return err
	}

	return controllerutil.SetControllerReference(owner, resource, r.scheme)
}

func (r *ReconcileClusterInstallation) createServiceAccountIfNotExists(owner v1.Object, serviceAccount *corev1.ServiceAccount, reqLogger logr.Logger) error {
	foundServiceAccount := &corev1.ServiceAccount{}

	err := r.client.Get(context.TODO(), types.NamespacedName{Name: serviceAccount.Name, Namespace: serviceAccount.Namespace}, foundServiceAccount)
	if err != nil && errors.IsNotFound(err) {
		reqLogger.Info("Creating service account", "name", serviceAccount.Name)
		return r.createResource(owner, serviceAccount, reqLogger)
	} else if err != nil {
		reqLogger.Error(err, "Failed to check if service account exists")
		return err
	}

	// TODO compare found service account versus expected

	return nil
}

func (r *ReconcileClusterInstallation) createRoleBindingIfNotExists(owner v1.Object, roleBinding *rbacv1beta1.RoleBinding, reqLogger logr.Logger) error {
	foundRoleBinding := &rbacv1beta1.RoleBinding{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: roleBinding.Name, Namespace: roleBinding.Namespace}, foundRoleBinding)
	if err != nil && errors.IsNotFound(err) {
		reqLogger.Info("Creating role binding", "name", roleBinding.Name)
		return r.createResource(owner, roleBinding, reqLogger)
	} else if err != nil {
		reqLogger.Error(err, "Failed to check if role binding exists")
		return err
	}

	// TODO compare found role binding versus expected

	return nil
}

func (r *ReconcileClusterInstallation) createServiceIfNotExists(owner v1.Object, service *corev1.Service, reqLogger logr.Logger) error {
	foundService := &corev1.Service{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, foundService)
	if err != nil && errors.IsNotFound(err) {
		reqLogger.Info("Creating service", "name", service.Name)
		return r.createResource(owner, service, reqLogger)
	} else if err != nil {
		reqLogger.Error(err, "Failed to check if service exists")
		return err
	}

	// TODO check how to do the update

	return nil
}

func (r *ReconcileClusterInstallation) createIngressIfNotExists(owner v1.Object, ingress *v1beta1.Ingress, reqLogger logr.Logger) error {
	foundIngress := &v1beta1.Ingress{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: ingress.Name, Namespace: ingress.Namespace}, foundIngress)
	if err != nil && errors.IsNotFound(err) {
		reqLogger.Info("Creating ingress", "name", ingress.Name)
		return r.createResource(owner, ingress, reqLogger)
	} else if err != nil {
		reqLogger.Error(err, "Failed to check if ingress exists")
		return err
	}

	// TODO check how to do the update

	return nil
}

func (r *ReconcileClusterInstallation) createDeploymentIfNotExists(owner v1.Object, deployment *appsv1.Deployment, reqLogger logr.Logger) error {
	foundDeployment := &appsv1.Deployment{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, foundDeployment)
	if err != nil && errors.IsNotFound(err) {
		reqLogger.Info("Creating deployment", "name", deployment.Name)
		return r.createResource(owner, deployment, reqLogger)
	} else if err != nil {
		reqLogger.Error(err, "Failed to check if deployment exists")
		return err
	}

	return nil
}

func (r *ReconcileClusterInstallation) createSecretIfNotExists(owner v1.Object, secret *corev1.Secret, reqLogger logr.Logger) error {
	foundSecret := &corev1.Secret{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, foundSecret)
	if err != nil && errors.IsNotFound(err) {
		reqLogger.Info("Creating secret", "name", secret.Name)
		return r.createResource(owner, secret, reqLogger)
	} else if err != nil {
		reqLogger.Error(err, "Failed to check if secret exists")
		return err
	}

	return nil
}
