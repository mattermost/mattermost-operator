package minio

import (
	"fmt"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/apis/mattermost/v1alpha1"
	"github.com/mattermost/mattermost-operator/pkg/components/utils"
	mattermostApp "github.com/mattermost/mattermost-operator/pkg/mattermost"

	ptrUtil "github.com/mattermost/mattermost-operator/pkg/utils"
	minioOperator "github.com/minio/operator/pkg/apis/minio.min.io/v2"
	corev1 "k8s.io/api/core/v1"
	resource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Instance returns the Minio component to deploy
func Instance(mattermost *mattermostv1alpha1.ClusterInstallation) *minioOperator.Tenant {
	minioName := DefaultMinioSecretName(mattermost.Name)

	// Check if custom secret was passed
	if mattermost.Spec.Minio.Secret != "" {
		minioName = mattermost.Spec.Minio.Secret
	}

	return newMinioTenant(
		minioName,
		mattermost.Namespace,
		mattermostv1alpha1.ClusterInstallationResourceLabels(mattermost.Name),
		mattermostApp.ClusterInstallationOwnerReference(mattermost),
		mattermost.Spec.Minio.Servers,
		mattermost.Spec.Minio.VolumesPerServer,
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
func InstanceV1Beta(mattermost *mmv1beta.Mattermost) *minioOperator.Tenant {
	minioName := fmt.Sprintf("%s-minio", mattermost.Name)

	return newMinioTenant(
		minioName,
		mattermost.Namespace,
		mmv1beta.MattermostResourceLabels(mattermost.Name),
		mattermostApp.MattermostOwnerReference(mattermost),
		*mattermost.Spec.FileStore.OperatorManaged.Servers,
		*mattermost.Spec.FileStore.OperatorManaged.VolumesPerServer,
		mattermost.Spec.FileStore.OperatorManaged.StorageSize,
	)
}

// Secret returns the secret name created to use together with Minio deployment
func SecretV1Beta(mattermost *mmv1beta.Mattermost) *corev1.Secret {
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

func newMinioTenant(
	name string,
	namespace string,
	labels map[string]string,
	ownerRefs []metav1.OwnerReference,
	servers int32,
	volumesPerServer int32,
	storageSize string,
) *minioOperator.Tenant {
	return &minioOperator.Tenant{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: ownerRefs,
		},
		Spec: minioOperator.TenantSpec{
			RequestAutoCert: ptrUtil.NewBool(false),
			Pools: []minioOperator.Pool{
				{
					Servers:          servers,
					VolumesPerServer: volumesPerServer,
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
			},
			Mountpath:     "/export",
			Configuration: &corev1.LocalObjectReference{Name: name},
		},
	}
}

func minioSecretData() map[string][]byte {
	accessKey := utils.New16ID()
	secretKey := utils.New28ID()
	// credentials can also be generated using minioOperator.GenerateCredentials() but the original
	// method was left to allow us more control.
	data := make(map[string][]byte, 1)

	// convig.env is the way the minio operator now needs the credentials
	data["config.env"] = []byte(minioOperator.GenerateTenantConfigurationFile(map[string]string{
		"MINIO_ROOT_USER":     string(accessKey),
		"MINIO_ROOT_PASSWORD": string(secretKey),
	}))

	// we are going to store it the old way too, so we can retireve it for our purposes as well
	data["accesskey"] = accessKey
	data["secretkey"] = secretKey

	return data
}
