package resources

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/pkg/errors"

	objectMatcher "github.com/banzaicloud/k8s-objectmatcher/patch"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const lastAppliedConfig = "mattermost.com/last-applied"

var defaultAnnotator = objectMatcher.NewAnnotator(lastAppliedConfig)

// Object combines the interfaces that all Kubernetes objects must implement.
type Object interface {
	runtime.Object
	v1.Object
}

// ResourceHelper provides helper methods to create, updated and fetch different resources.
type ResourceHelper struct {
	client client.Client
	scheme *runtime.Scheme
}

func NewResourceHelper(client client.Client, scheme *runtime.Scheme) *ResourceHelper {
	return &ResourceHelper{
		client: client,
		scheme: scheme,
	}
}

// Create creates the provided resource and sets the owner
func (r *ResourceHelper) Create(owner v1.Object, desired Object, reqLogger logr.Logger) error {
	// adding the last applied annotation to use the object matcher later
	// see: https://github.com/banzaicloud/k8s-objectmatcher
	err := defaultAnnotator.SetLastAppliedAnnotation(desired)
	if err != nil {
		return errors.Wrap(err, "failed to apply annotation to the resource")
	}

	err = controllerutil.SetControllerReference(owner, desired, r.scheme)
	if err != nil {
		return errors.Wrap(err, "failed to set owner reference")
	}

	return r.client.Create(context.TODO(), desired)
}

func (r *ResourceHelper) Update(current, desired Object, reqLogger logr.Logger) error {
	patchResult, err := objectMatcher.NewPatchMaker(
		defaultAnnotator,
		&objectMatcher.K8sStrategicMergePatcher{},
		&objectMatcher.BaseJSONMergePatcher{},
	).Calculate(current, desired)
	if err != nil {
		return errors.Wrap(err, "failed to determine if resources differ")
	}
	if !patchResult.IsEmpty() {
		if err := defaultAnnotator.SetLastAppliedAnnotation(desired); err != nil {
			return errors.Wrap(err, "failed to apply annotation to the resource")
		}

		reqLogger.Info("Updating resource", "name", desired.GetName(), "kind", desired.GetObjectKind(), "namespace", desired.GetNamespace(), "patch", string(patchResult.Patch))

		// Resource version is required for the update, but need to be set after
		// the last applied annotation to avoid unnecessary diffs
		desired.SetResourceVersion(current.GetResourceVersion())
		return r.client.Update(context.TODO(), desired)
	}

	return nil
}

func (r *ResourceHelper) CreateServiceAccountIfNotExists(owner v1.Object, serviceAccount *corev1.ServiceAccount, reqLogger logr.Logger) error {
	foundServiceAccount := &corev1.ServiceAccount{}

	err := r.client.Get(context.TODO(), types.NamespacedName{Name: serviceAccount.Name, Namespace: serviceAccount.Namespace}, foundServiceAccount)
	if err != nil && k8sErrors.IsNotFound(err) {
		reqLogger.Info("Creating service account", "name", serviceAccount.Name)
		return r.Create(owner, serviceAccount, reqLogger)
	} else if err != nil {
		return errors.Wrap(err, "failed to check if service account exists")
	}

	return nil
}

func (r *ResourceHelper) CreateRoleBindingIfNotExists(owner v1.Object, roleBinding *rbacv1.RoleBinding, reqLogger logr.Logger) error {
	foundRoleBinding := &rbacv1.RoleBinding{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: roleBinding.Name, Namespace: roleBinding.Namespace}, foundRoleBinding)
	if err != nil && k8sErrors.IsNotFound(err) {
		reqLogger.Info("Creating role binding", "name", roleBinding.Name)
		return r.Create(owner, roleBinding, reqLogger)
	} else if err != nil {
		return errors.Wrap(err, "failed to check if role binding exists")
	}

	return nil
}

func (r *ResourceHelper) CreateServiceIfNotExists(owner v1.Object, service *corev1.Service, reqLogger logr.Logger) error {
	foundService := &corev1.Service{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, foundService)
	if err != nil && k8sErrors.IsNotFound(err) {
		reqLogger.Info("Creating service", "name", service.Name)
		return r.Create(owner, service, reqLogger)
	} else if err != nil {
		return errors.Wrap(err, "failed to check if service exists")
	}

	return nil
}

func (r *ResourceHelper) CreateIngressIfNotExists(owner v1.Object, ingress *networkingv1.Ingress, reqLogger logr.Logger) error {
	foundIngress := &networkingv1.Ingress{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: ingress.Name, Namespace: ingress.Namespace}, foundIngress)
	if err != nil && k8sErrors.IsNotFound(err) {
		reqLogger.Info("Creating ingress", "name", ingress.Name)
		return r.Create(owner, ingress, reqLogger)
	} else if err != nil {
		return errors.Wrap(err, "failed to check if ingress exists")
	}

	return nil
}

func (r *ResourceHelper) CreateIngressClassIfNotExists(owner v1.Object, ingressClass *networkingv1.IngressClass, reqLogger logr.Logger) error {
	foundIngressClass := &networkingv1.IngressClass{}

	err := r.client.Get(context.TODO(), types.NamespacedName{Name: ingressClass.Name, Namespace: ingressClass.Namespace}, foundIngressClass)
	if err != nil && k8sErrors.IsNotFound(err) {
		reqLogger.Info("Creating ingressClass", "name", ingressClass.Name)
		return r.Create(owner, ingressClass, reqLogger)
	} else if err != nil {
		return errors.Wrap(err, "failed to check if ingressClass exists")
	}

	return nil
}

func (r *ResourceHelper) CreateDeploymentIfNotExists(owner v1.Object, deployment *appsv1.Deployment, reqLogger logr.Logger) error {
	foundDeployment := &appsv1.Deployment{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, foundDeployment)
	if err != nil && k8sErrors.IsNotFound(err) {
		reqLogger.Info("Creating deployment", "name", deployment.Name)
		return r.Create(owner, deployment, reqLogger)
	} else if err != nil {
		return errors.Wrap(err, "failed to check if deployment exists")
	}

	return nil
}

func (r *ResourceHelper) CreateRoleIfNotExists(owner v1.Object, role *rbacv1.Role, reqLogger logr.Logger) error {
	foundRole := &rbacv1.Role{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: role.Name, Namespace: role.Namespace}, foundRole)
	if err != nil && k8sErrors.IsNotFound(err) {
		reqLogger.Info("Creating role", "name", role.Name)
		return r.Create(owner, role, reqLogger)
	} else if err != nil {
		return errors.Wrap(err, "failed to check if role exists")
	}

	return nil
}

func (r *ResourceHelper) DeleteIngressClass(key types.NamespacedName, reqLogger logr.Logger) error {
	foundIngressClass := &networkingv1.IngressClass{}
	err := r.client.Get(context.TODO(), key, foundIngressClass)
	if err != nil && k8sErrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Wrap(err, "failed to check if ingressClass exists")
	}

	reqLogger.Info("Deleting ingressClass", "name", foundIngressClass.Name)
	err = r.client.Delete(context.TODO(), foundIngressClass)
	if err != nil {
		return errors.Wrap(err, "failed to delete ingressClass")
	}

	return nil
}

func (r *ResourceHelper) CreatePvcIfNotExists(owner v1.Object, pvc *corev1.PersistentVolumeClaim, reqLogger logr.Logger) error {
	foundPvc := &corev1.PersistentVolumeClaim{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: pvc.Name, Namespace: pvc.Namespace}, foundPvc)
	if err != nil && k8sErrors.IsNotFound(err) {
		reqLogger.Info("Creating pvc", "name", pvc.Name)
		return r.Create(owner, pvc, reqLogger)
	} else if err != nil {
		return errors.Wrap(err, "failed to check if pvc exists")
	}

	return nil
}

func (r *ResourceHelper) DeleteIngress(key types.NamespacedName, reqLogger logr.Logger) error {
	foundIngress := &networkingv1.Ingress{}
	err := r.client.Get(context.TODO(), key, foundIngress)
	if err != nil && k8sErrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Wrap(err, "failed to check if ingress exists")
	}

	reqLogger.Info("Deleting ingress", "name", foundIngress.Name)
	err = r.client.Delete(context.TODO(), foundIngress)
	if err != nil {
		return errors.Wrap(err, "failed to delete ingress")
	}
	return nil
}

func (r *ResourceHelper) DeleteService(key types.NamespacedName, reqLogger logr.Logger) error {
	foundService := &corev1.Service{}
	err := r.client.Get(context.TODO(), key, foundService)
	if err != nil && k8sErrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Wrap(err, "failed to check if service exists")
	}

	reqLogger.Info("Deleting service", "name", foundService.Name)
	err = r.client.Delete(context.TODO(), foundService)
	if err != nil {
		return errors.Wrap(err, "failed to delete service")
	}
	return nil
}
