package mattermost

import (
	"errors"
	"fmt"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	corev1 "k8s.io/api/core/v1"
)

const (
	// FileStoreDefaultVolumeName is the default volume name for Mattermost
	// filestore data.
	FileStoreDefaultVolumeName = "mattermost-data"

	fileStoreSecretAccessKey = "accesskey"
	fileStoreSecretSecretKey = "secretkey"
)

type FileStoreInfo struct {
	secretName string
	bucketName string
	url        string
	useS3SSL   bool
}

type ExternalFileStore struct {
	fsInfo FileStoreInfo
}

func (e *ExternalFileStore) EnvVars(_ *mmv1beta.Mattermost) []corev1.EnvVar {
	return s3EnvVars(&e.fsInfo)
}

func (e *ExternalFileStore) InitContainers(_ *mmv1beta.Mattermost) []corev1.Container {
	return []corev1.Container{}
}

func (e *ExternalFileStore) Volumes(_ *mmv1beta.Mattermost) ([]corev1.Volume, []corev1.VolumeMount) {
	return []corev1.Volume{}, []corev1.VolumeMount{}
}

type ExternalVolumeFileStore struct {
	VolumeClaimName string
}

func (fs *ExternalVolumeFileStore) EnvVars(_ *mmv1beta.Mattermost) []corev1.EnvVar {
	return localFileEnvVars(mmv1beta.DefaultLocalFilePath)
}

func (fs *ExternalVolumeFileStore) InitContainers(_ *mmv1beta.Mattermost) []corev1.Container {
	return []corev1.Container{}
}

func (fs *ExternalVolumeFileStore) Volumes(mm *mmv1beta.Mattermost) ([]corev1.Volume, []corev1.VolumeMount) {
	volumes := []corev1.Volume{
		{
			Name: FileStoreDefaultVolumeName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: fs.VolumeClaimName,
				},
			},
		},
	}

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      FileStoreDefaultVolumeName,
			MountPath: mmv1beta.DefaultLocalFilePath,
		},
	}
	return volumes, volumeMounts
}

type LocalFileStore struct{}

func (e *LocalFileStore) EnvVars(_ *mmv1beta.Mattermost) []corev1.EnvVar {
	return localFileEnvVars(mmv1beta.DefaultLocalFilePath)
}

func (e *LocalFileStore) InitContainers(_ *mmv1beta.Mattermost) []corev1.Container {
	return []corev1.Container{}
}

func (e *LocalFileStore) Volumes(mm *mmv1beta.Mattermost) ([]corev1.Volume, []corev1.VolumeMount) {
	volumes := []corev1.Volume{
		{
			Name: FileStoreDefaultVolumeName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: mm.Name,
				},
			},
		},
	}

	volumeMounts := []corev1.VolumeMount{
		{
			MountPath: mmv1beta.DefaultLocalFilePath,
			Name:      FileStoreDefaultVolumeName,
		},
	}
	return volumes, volumeMounts
}

type OperatorManagedMinioConfig struct {
	fsInfo     FileStoreInfo
	secretName string
	minioURL   string
}

func (e *OperatorManagedMinioConfig) EnvVars(_ *mmv1beta.Mattermost) []corev1.EnvVar {
	return s3EnvVars(&e.fsInfo)
}

func (e *OperatorManagedMinioConfig) InitContainers(mattermost *mmv1beta.Mattermost) []corev1.Container {
	initContainers := []corev1.Container{
		// Create the init container to create the MinIO bucket
		{
			Name:            "create-minio-bucket",
			Image:           "minio/mc:latest",
			ImagePullPolicy: corev1.PullIfNotPresent,
			Command: []string{
				"/bin/sh", "-c",
				fmt.Sprintf("mc config host add localminio http://%s $(accesskey) $(secretkey) && mc mb localminio/%s -q -p", e.minioURL, mattermost.Name),
			},
			Env: []corev1.EnvVar{
				{
					Name:      "accesskey",
					ValueFrom: EnvSourceFromSecret(e.secretName, fileStoreSecretAccessKey),
				},
				{
					Name:      "secretkey",
					ValueFrom: EnvSourceFromSecret(e.secretName, fileStoreSecretSecretKey),
				},
			},
		},
		// Create the init container to check that MinIO is up and running
		{
			Name:            "init-check-minio",
			Image:           "appropriate/curl:latest",
			ImagePullPolicy: corev1.PullIfNotPresent,
			Command: []string{
				"sh", "-c",
				fmt.Sprintf("until curl --max-time 5 http://%s/minio/health/ready; do echo waiting for minio; sleep 5; done;", e.minioURL),
			},
		},
	}

	return initContainers
}

func (e *OperatorManagedMinioConfig) Volumes(_ *mmv1beta.Mattermost) ([]corev1.Volume, []corev1.VolumeMount) {
	return []corev1.Volume{}, []corev1.VolumeMount{}
}

func NewExternalFileStoreInfo(mattermost *mmv1beta.Mattermost, secret *corev1.Secret) (FileStoreConfig, error) {
	if mattermost.Spec.FileStore.External == nil {
		return nil, errors.New("external file store configuration not provided")
	}
	bucket := mattermost.Spec.FileStore.External.Bucket
	if bucket == "" {
		return nil, errors.New("external file store bucket is empty")
	}
	url := mattermost.Spec.FileStore.External.URL
	if url == "" {
		return nil, errors.New("external file store URL is empty")
	}

	if secret == nil {
		return &ExternalFileStore{
			fsInfo: FileStoreInfo{
				bucketName: bucket,
				url:        url,
				useS3SSL:   true,
			},
		}, nil
	}

	if _, ok := secret.Data["accesskey"]; !ok {
		return nil, fmt.Errorf("external filestore Secret %s does not have an 'accesskey' value", secret.Name)
	}
	if _, ok := secret.Data["secretkey"]; !ok {
		return nil, fmt.Errorf("external filestore Secret %s does not have an 'secretkey' value", secret.Name)
	}

	return &ExternalFileStore{
		fsInfo: FileStoreInfo{
			secretName: secret.Name,
			bucketName: bucket,
			url:        url,
			useS3SSL:   true,
		},
	}, nil
}

func NewExternalVolumeFileStoreInfo(mattermost *mmv1beta.Mattermost) (FileStoreConfig, error) {
	if mattermost.Spec.FileStore.ExternalVolume == nil {
		return nil, errors.New("external volume file store configuration not provided")
	}

	volumeClaimName := mattermost.Spec.FileStore.ExternalVolume.VolumeClaimName
	if volumeClaimName == "" {
		return nil, errors.New("external volume claim name is empty")
	}

	return &ExternalVolumeFileStore{
		VolumeClaimName: volumeClaimName,
	}, nil
}

func NewOperatorManagedFileStoreInfo(mattermost *mmv1beta.Mattermost, secret, minioURL string) FileStoreConfig {
	return &OperatorManagedMinioConfig{
		fsInfo: FileStoreInfo{
			secretName: secret,
			bucketName: mattermost.Name,
			url:        minioURL,
			useS3SSL:   false,
		},
		minioURL:   minioURL,
		secretName: secret,
	}
}

func NewLocalFileStoreInfo() FileStoreConfig {
	return &LocalFileStore{}
}
