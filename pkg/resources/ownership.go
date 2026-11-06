package resources

import (
	"github.com/pkg/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	mmv1alpha1 "github.com/mattermost/mattermost-operator/apis/mattermost/v1alpha1"
)

// isControlledBy reports whether the controller reference of object points to owner.
func isControlledBy(object, owner v1.Object) bool {
	return v1.IsControlledBy(object, owner)
}

// isUpdatable reports whether the operator may overwrite the live object on
// behalf of owner. The object must be controlled by owner. Objects in the
// owner namespace are also accepted when they have no controller or when a
// ClusterInstallation with the name of owner controls them. Migration from
// ClusterInstallation to Mattermost leaves objects in these two states.
func isUpdatable(current, owner v1.Object) bool {
	if isControlledBy(current, owner) {
		return true
	}
	if current.GetNamespace() != owner.GetNamespace() {
		return false
	}

	controller := v1.GetControllerOf(current)
	if controller == nil {
		return true
	}

	gv, err := schema.ParseGroupVersion(controller.APIVersion)
	if err != nil {
		return false
	}

	return controller.Kind == "ClusterInstallation" &&
		controller.Name == owner.GetName() &&
		gv.Group == mmv1alpha1.GroupVersion.Group
}

func notOwnedError(object, owner v1.Object) error {
	return errors.Errorf(
		"refusing to modify %T %s/%s: it is not controlled by %s/%s",
		object, object.GetNamespace(), object.GetName(), owner.GetNamespace(), owner.GetName(),
	)
}
