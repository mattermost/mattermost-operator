package resources

import (
	"github.com/pkg/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	mmv1alpha1 "github.com/mattermost/mattermost-operator/apis/mattermost/v1alpha1"
)

// isMigrationController reports whether a ClusterInstallation with the name of
// owner controls object in the namespace of owner. Migration from
// ClusterInstallation to Mattermost leaves objects in this state.
func isMigrationController(object, owner v1.Object) bool {
	if object.GetNamespace() != owner.GetNamespace() {
		return false
	}

	controller := v1.GetControllerOf(object)
	if controller == nil {
		return false
	}

	gv, err := schema.ParseGroupVersion(controller.APIVersion)
	if err != nil {
		return false
	}

	return controller.Kind == "ClusterInstallation" &&
		controller.Name == owner.GetName() &&
		gv.Group == mmv1alpha1.GroupVersion.Group
}

// isDeletable reports whether the operator may delete the live object on
// behalf of owner. The object must be controlled by owner, or by the
// ClusterInstallation that migration replaced. Objects without a controller
// are never deletable.
func isDeletable(object, owner v1.Object) bool {
	return v1.IsControlledBy(object, owner) || isMigrationController(object, owner)
}

// isUpdatable reports whether the operator may overwrite the live object on
// behalf of owner. It accepts every deletable object. It also accepts objects
// in the owner namespace that have no controller, because migration recreates
// the Deployment without one.
func isUpdatable(current, owner v1.Object) bool {
	if isDeletable(current, owner) {
		return true
	}

	return current.GetNamespace() == owner.GetNamespace() && v1.GetControllerOf(current) == nil
}

func notOwnedError(object, owner v1.Object) error {
	return errors.Errorf(
		"refusing to modify %T %s/%s: it is not controlled by %s/%s",
		object, object.GetNamespace(), object.GetName(), owner.GetNamespace(), owner.GetName(),
	)
}
