package apis

import (
	v1alpha1MySQL "github.com/presslabs/mysql-operator/pkg/apis/mysql/v1alpha1"
)

func init() {
	// Register the types with the Scheme so the components can map objects to GroupVersionKinds and back
	AddToSchemes = append(AddToSchemes, v1alpha1MySQL.SchemeBuilder.AddToScheme)
}
