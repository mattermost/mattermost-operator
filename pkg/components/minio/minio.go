package minio

import (
	"fmt"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
	"github.com/mattermost/mattermost-operator/pkg/components/utils"

	minioOperator "github.com/minio/minio-operator/pkg/apis/miniocontroller/v1beta1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	resource "k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Instance returns the Minio component to deploy
func Instance(mattermost *mattermostv1alpha1.ClusterInstallation) *minioOperator.MinioInstance {
	minioName := fmt.Sprintf("%s-minio", mattermost.Name)

	return &minioOperator.MinioInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      minioName,
			Namespace: mattermost.Namespace,
			Labels:    mattermostv1alpha1.ClusterInstallationResourceLabels(mattermost.Name),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(mattermost, schema.GroupVersionKind{
					Group:   mattermostv1alpha1.SchemeGroupVersion.Group,
					Version: mattermostv1alpha1.SchemeGroupVersion.Version,
					Kind:    "ClusterInstallation",
				}),
			},
		},
		Spec: minioOperator.MinioInstanceSpec{
			Version:     "RELEASE.2018-11-22T02-51-56Z",
			Replicas:    4,
			Mountpath:   "/export",
			CredsSecret: &corev1.LocalObjectReference{Name: minioName},
			VolumeClaimTemplate: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: minioName,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						"ReadWriteOnce",
					},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse(mattermost.Spec.MinioStorageSize),
						},
					},
				},
			},
		},
	}
}

// Secret returns the secret name created to use togehter with Minio deployment
func Secret(mattermost *mattermostv1alpha1.ClusterInstallation) *corev1.Secret {
	secretName := fmt.Sprintf("%s-minio", mattermost.Name)
	data := make(map[string][]byte)
	data["accesskey"] = utils.New16ID()
	data["secretkey"] = utils.New28ID()

	return mattermost.GenerateSecret(
		secretName,
		mattermostv1alpha1.ClusterInstallationResourceLabels(mattermost.Name),
		data,
	)
}
