package clusterinstallation

import (
	"context"

	objectMatcher "github.com/banzaicloud/k8s-objectmatcher/patch"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1beta1 "k8s.io/api/extensions/v1beta1"
	rbacv1beta1 "k8s.io/api/rbac/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// Object combines the interfaces that all Kubernetes objects must implement.
type Object interface {
	runtime.Object
	v1.Object
}

// createResource creates the provided resource and sets the owner
func (r *ReconcileClusterInstallation) createResource(owner v1.Object, resource Object, reqLogger logr.Logger) error {
	// adding the last applied annotation to use the object matcher later
	// see: https://github.com/banzaicloud/k8s-objectmatcher
	errMatcher := objectMatcher.DefaultAnnotator.SetLastAppliedAnnotation(resource)
	if errMatcher != nil {
		reqLogger.Error(errMatcher, "Error applying the annotation in the resource")
		return errMatcher
	}
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
