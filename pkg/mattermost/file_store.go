package mattermost

import (
	"errors"
	"fmt"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	corev1 "k8s.io/api/core/v1"
)

const (
	fileStoreSecretAccessKey = "accesskey"
	fileStoreSecretSecretKey = "secretkey"
)

type FileStoreInfo struct {
	secretName string
	bucketName string
	url        string
	useS3SSL   bool
	config     FileStoreConfig
}

type ExternalFileStore struct{}

func (e *ExternalFileStore) InitContainers(_ *mmv1beta.Mattermost) []corev1.Container {
	return []corev1.Container{}
}

type LocalFileStore struct{}

func (e *LocalFileStore) InitContainers(_ *mmv1beta.Mattermost) []corev1.Container {
	return []corev1.Container{}
}

type OperatorManagedMinioConfig struct {
	secretName string
	minioURL   string
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
				fmt.Sprintf("mc config host add localminio http://%s $(MINIO_ACCESS_KEY) $(MINIO_SECRET_KEY) && mc mb localminio/%s -q -p", e.minioURL, mattermost.Name),
			},
			Env: []corev1.EnvVar{
				{
					Name:      "MINIO_ACCESS_KEY",
					ValueFrom: EnvSourceFromSecret(e.secretName, fileStoreSecretAccessKey),
				},
				{
					Name:      "MINIO_SECRET_KEY",
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

func NewExternalFileStoreInfo(mattermost *mmv1beta.Mattermost, secret corev1.Secret) (*FileStoreInfo, error) {
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

	if _, ok := secret.Data["accesskey"]; !ok {
		return nil, fmt.Errorf("external filestore Secret %s does not have an 'accesskey' value", secret.Name)
	}
	if _, ok := secret.Data["secretkey"]; !ok {
		return nil, fmt.Errorf("external filestore Secret %s does not have an 'secretkey' value", secret.Name)
	}

	return &FileStoreInfo{
		secretName: secret.Name,
		bucketName: bucket,
		url:        url,
		useS3SSL:   true,
		config:     &ExternalFileStore{},
	}, nil
}

func NewOperatorManagedFileStoreInfo(mattermost *mmv1beta.Mattermost, secret, minioURL string) *FileStoreInfo {
	return &FileStoreInfo{
		secretName: secret,
		bucketName: mattermost.Name,
		url:        minioURL,
		useS3SSL:   false,
		config:     &OperatorManagedMinioConfig{minioURL: minioURL, secretName: secret},
	}
}

func NewLocalFileStoreInfo() *FileStoreInfo {
	return &FileStoreInfo{
		secretName: "",
		bucketName: "",
		url:        "",
		useS3SSL:   false,
		config:     &LocalFileStore{},
	}
}
