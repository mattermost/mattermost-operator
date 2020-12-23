package mattermost

import (
	"testing"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestFileStore(t *testing.T) {
	mattermost := &mmv1beta.Mattermost{
		ObjectMeta: metav1.ObjectMeta{Name: "mm-test"},
		Spec:       mmv1beta.MattermostSpec{},
	}

	secret := "file-store-secret"
	minioURL := "http://minio"

	t.Run("operator managed Minio", func(t *testing.T) {
		mattermost.Spec.FileStore = mmv1beta.FileStore{
			OperatorManaged: &mmv1beta.OperatorManagedMinio{
				StorageSize: "10GB",
				Replicas:    nil,
				Resources:   corev1.ResourceRequirements{},
			},
		}

		fileStore := NewOperatorManagedFileStoreInfo(mattermost, secret, minioURL)
		initContainers := fileStore.config.InitContainers(mattermost)
		assert.Equal(t, 2, len(initContainers))
		assert.Equal(t, secret, fileStore.secretName)
		assert.Equal(t, minioURL, fileStore.url)
		assert.Equal(t, "mm-test", fileStore.bucketName)
	})

	t.Run("external file store", func(t *testing.T) {
		mattermost.Spec.FileStore = mmv1beta.FileStore{
			External: &mmv1beta.ExternalFileStore{
				URL:    minioURL,
				Bucket: "test-bucket",
				Secret: "external-file-store",
			},
		}

		secret := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "external-file-store"},
			Data: map[string][]byte{
				"accesskey": []byte("key"),
				"secretkey": []byte("secret"),
			},
		}

		fileStore, err := NewExternalFileStoreInfo(mattermost, secret)
		require.NoError(t, err)
		initContainers := fileStore.config.InitContainers(mattermost)
		assert.Equal(t, 0, len(initContainers))
		assert.Equal(t, "external-file-store", fileStore.secretName)
		assert.Equal(t, minioURL, fileStore.url)
		assert.Equal(t, "test-bucket", fileStore.bucketName)
	})
}
