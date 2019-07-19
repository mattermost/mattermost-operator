package bluegreen

import (
	"context"

	objectMatcher "github.com/banzaicloud/k8s-objectmatcher/patch"
	"github.com/go-logr/logr"
	// appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1beta1 "k8s.io/api/extensions/v1beta1"
	// rbacv1beta1 "k8s.io/api/rbac/v1beta1"
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
func (r *ReconcileBlueGreen) createResource(owner v1.Object, resource Object, reqLogger logr.Logger) error {
	// adding the last applied annotation to use the object matcher later
	// see: https://github.com/banzaicloud/k8s-objectmatcher
	err := objectMatcher.DefaultAnnotator.SetLastAppliedAnnotation(resource)
	if err != nil {
		reqLogger.Error(err, "Error applying the annotation in the resource")
		return err
	}
	err = r.client.Create(context.TODO(), resource)
	if err != nil {
		reqLogger.Error(err, "Error creating resource")
		return err
	}

	return controllerutil.SetControllerReference(owner, resource, r.scheme)
}

func (r *ReconcileBlueGreen) createServiceIfNotExists(owner v1.Object, service *corev1.Service, reqLogger logr.Logger) error {
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

func (r *ReconcileBlueGreen) createIngressIfNotExists(owner v1.Object, ingress *v1beta1.Ingress, reqLogger logr.Logger) error {
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
