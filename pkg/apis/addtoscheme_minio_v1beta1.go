package apis

import (
	v1beta1Minio "github.com/minio/minio-operator/pkg/apis/miniocontroller/v1beta1"
)

func init() {
	// Register the types with the Scheme so the components can map objects to GroupVersionKinds and back
	AddToSchemes = append(AddToSchemes, v1beta1Minio.SchemeBuilder.AddToScheme)
}
