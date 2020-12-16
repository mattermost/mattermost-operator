package minio

import (
	"fmt"

	mattermostv1beta1 "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/apis/mattermost/v1alpha1"
	"github.com/mattermost/mattermost-operator/pkg/components/utils"
	mattermostApp "github.com/mattermost/mattermost-operator/pkg/mattermost"

	minioOperator "github.com/minio/minio-operator/pkg/apis/miniocontroller/v1beta1"

	corev1 "k8s.io/api/core/v1"
	resource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Instance returns the Minio component to deploy
func Instance(mattermost *mattermostv1alpha1.ClusterInstallation) *minioOperator.MinIOInstance {
	minioName := fmt.Sprintf("%s-minio", mattermost.Name)

	// Check if custom secret was passed
	if mattermost.Spec.Minio.Secret != "" {
		minioName = mattermost.Spec.Minio.Secret
	}

	return newMinioInstance(
		minioName,
		mattermost.Namespace,
		mattermostv1alpha1.ClusterInstallationResourceLabels(mattermost.Name),
		mattermostApp.ClusterInstallationOwnerReference(mattermost),
		mattermost.Spec.Minio.Replicas,
		mattermost.Spec.Minio.StorageSize,
	)
}

// Secret returns the secret name created to use together with Minio deployment
func Secret(mattermost *mattermostv1alpha1.ClusterInstallation) *corev1.Secret {
	secretName := DefaultMinioSecretName(mattermost.Name)
	data := minioSecretData()

	return mattermostApp.GenerateSecret(
		mattermost,
		secretName,
		mattermostv1alpha1.ClusterInstallationResourceLabels(mattermost.Name),
		data,
	)
}

// Instance returns the Minio component to deploy
func InstanceV1Beta(mattermost *mattermostv1beta1.Mattermost) *minioOperator.MinIOInstance {
	minioName := fmt.Sprintf("%s-minio", mattermost.Name)

	return newMinioInstance(
		minioName,
		mattermost.Namespace,
		mattermostv1beta1.MattermostResourceLabels(mattermost.Name),
		mattermostApp.MattermostOwnerReference(mattermost),
		*mattermost.Spec.FileStore.OperatorManaged.Replicas,
		mattermost.Spec.FileStore.OperatorManaged.StorageSize,
	)
}

// Secret returns the secret name created to use together with Minio deployment
func SecretV1Beta(mattermost *mattermostv1beta1.Mattermost) *corev1.Secret {
	secretName := DefaultMinioSecretName(mattermost.Name)
	data := minioSecretData()

	return mattermostApp.GenerateSecretV1Beta(
		mattermost,
		secretName,
		mattermostv1alpha1.ClusterInstallationResourceLabels(mattermost.Name),
		data,
	)
}

// DefaultMinioSecretName returns the default minio secret name based on
// the provided installation name.
func DefaultMinioSecretName(installationName string) string {
	return fmt.Sprintf("%s-minio", installationName)
}

func newMinioInstance(
	name string,
	namespace string,
	labels map[string]string,
	ownerRefs []metav1.OwnerReference,
	replicas int32,
	storageSize string,
) *minioOperator.MinIOInstance {
	return &minioOperator.MinIOInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: ownerRefs,
		},
		Spec: minioOperator.MinIOInstanceSpec{
			Replicas:    replicas,
			Mountpath:   "/export",
			CredsSecret: &corev1.LocalObjectReference{Name: name},
			VolumeClaimTemplate: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						"ReadWriteOnce",
					},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse(storageSize),
						},
					},
				},
			},
		},
	}
}

func minioSecretData() map[string][]byte {
	data := make(map[string][]byte, 2)
	data["accesskey"] = utils.New16ID()
	data["secretkey"] = utils.New28ID()
	return data
}
