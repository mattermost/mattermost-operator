package mattermost

import (
	"testing"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewExternalDBInfo(t *testing.T) {
	mattermost := &mmv1beta.Mattermost{
		ObjectMeta: metav1.ObjectMeta{Name: "mm-test"},
		Spec: mmv1beta.MattermostSpec{
			Database: mmv1beta.Database{
				External: &mmv1beta.ExternalDatabase{Secret: "secret"},
			},
		},
	}

	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "secret"},
		Data: map[string][]byte{
			"DB_CONNECTION_STRING": []byte("postgres://my-postgres"),
		},
	}

	t.Run("connection string only", func(t *testing.T) {
		config, err := NewExternalDBConfig(mattermost, secret)
		require.NoError(t, err)
		assert.Equal(t, "secret", config.secretName)
		assert.Equal(t, "postgres", config.dbType)
		assert.False(t, config.hasDBCheckURL)
		assert.False(t, config.hasReaderEndpoints)

		envs := config.EnvVars(mattermost)
		assert.Equal(t, 2, len(envs))

		initContainers := config.InitContainers(mattermost)
		assert.Equal(t, 0, len(initContainers))
	})

	secret.Data["MM_SQLSETTINGS_DATASOURCEREPLICAS"] = []byte("postgres://my-postgres")
	secret.Data["DB_CONNECTION_CHECK_URL"] = []byte("postgres://my-postgres")

	t.Run("with db check url and reader and endpoints", func(t *testing.T) {
		config, err := NewExternalDBConfig(mattermost, secret)
		require.NoError(t, err)
		assert.Equal(t, "secret", config.secretName)
		assert.Equal(t, "postgres", config.dbType)
		assert.True(t, config.hasDBCheckURL)
		assert.True(t, config.hasReaderEndpoints)

		envs := config.EnvVars(mattermost)
		assert.Equal(t, 3, len(envs))

		initContainers := config.InitContainers(mattermost)
		assert.Equal(t, 1, len(initContainers))
		assert.Equal(t, "postgres:13", initContainers[0].Image)
	})

	t.Run("with disabled DB readiness check", func(t *testing.T) {
		mattermost.Spec.Database.DisableReadinessCheck = true
		config, err := NewExternalDBConfig(mattermost, secret)
		require.NoError(t, err)

		initContainers := config.InitContainers(mattermost)
		assert.Equal(t, 0, len(initContainers))
	})

	secret.Data["DB_CONNECTION_STRING"] = []byte{}
	t.Run("fail if connection string is empty", func(t *testing.T) {
		_, err := NewExternalDBConfig(mattermost, secret)
		require.Error(t, err)
	})

	delete(secret.Data, "DB_CONNECTION_STRING")
	t.Run("fail if connection string not present", func(t *testing.T) {
		_, err := NewExternalDBConfig(mattermost, secret)
		require.Error(t, err)
	})
}
