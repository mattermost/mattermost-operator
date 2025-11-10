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

func TestExternalDBConfig_SeparateDatasourceKey(t *testing.T) {
	mattermost := &mmv1beta.Mattermost{
		ObjectMeta: metav1.ObjectMeta{Name: "mm-test"},
		Spec: mmv1beta.MattermostSpec{
			Database: mmv1beta.Database{
				External: &mmv1beta.ExternalDatabase{Secret: "db-secret"},
			},
		},
	}

	t.Run("uses separate MM_SQLSETTINGS_DATASOURCE when provided", func(t *testing.T) {
		secret := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "db-secret"},
			Data: map[string][]byte{
				"DB_CONNECTION_STRING":      []byte("mysql://user:pass@tcp(host:3306)/db?charset=utf8mb4"),
				"MM_SQLSETTINGS_DATASOURCE": []byte("user:pass@tcp(host:3306)/db?charset=utf8mb4"),
			},
		}

		config, err := NewExternalDBConfig(mattermost, secret)
		require.NoError(t, err)
		assert.True(t, config.hasSeparateDatasourceKey)
		assert.Equal(t, "mysql", config.dbType)

		envs := config.EnvVars(mattermost)
		assert.Equal(t, 2, len(envs))

		// Verify MM_CONFIG uses DB_CONNECTION_STRING
		mmConfigEnv := findEnvVar(envs, "MM_CONFIG")
		require.NotNil(t, mmConfigEnv)
		assert.Equal(t, "DB_CONNECTION_STRING", mmConfigEnv.ValueFrom.SecretKeyRef.Key)

		// Verify MM_SQLSETTINGS_DATASOURCE uses the separate key
		datasourceEnv := findEnvVar(envs, "MM_SQLSETTINGS_DATASOURCE")
		require.NotNil(t, datasourceEnv)
		assert.Equal(t, "MM_SQLSETTINGS_DATASOURCE", datasourceEnv.ValueFrom.SecretKeyRef.Key)
	})

	t.Run("falls back to DB_CONNECTION_STRING for backward compatibility", func(t *testing.T) {
		secret := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "db-secret"},
			Data: map[string][]byte{
				"DB_CONNECTION_STRING": []byte("postgres://user:pass@host:5432/db"),
			},
		}

		config, err := NewExternalDBConfig(mattermost, secret)
		require.NoError(t, err)
		assert.False(t, config.hasSeparateDatasourceKey)
		assert.Equal(t, "postgres", config.dbType)

		envs := config.EnvVars(mattermost)
		assert.Equal(t, 2, len(envs))

		// Both should use DB_CONNECTION_STRING
		mmConfigEnv := findEnvVar(envs, "MM_CONFIG")
		require.NotNil(t, mmConfigEnv)
		assert.Equal(t, "DB_CONNECTION_STRING", mmConfigEnv.ValueFrom.SecretKeyRef.Key)

		datasourceEnv := findEnvVar(envs, "MM_SQLSETTINGS_DATASOURCE")
		require.NotNil(t, datasourceEnv)
		assert.Equal(t, "DB_CONNECTION_STRING", datasourceEnv.ValueFrom.SecretKeyRef.Key)
	})

	t.Run("works with all optional keys", func(t *testing.T) {
		secret := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "db-secret"},
			Data: map[string][]byte{
				"DB_CONNECTION_STRING":              []byte("mysql://user:pass@tcp(master:3306)/db"),
				"MM_SQLSETTINGS_DATASOURCE":         []byte("user:pass@tcp(master:3306)/db"),
				"MM_SQLSETTINGS_DATASOURCEREPLICAS": []byte("user:pass@tcp(replica:3306)/db"),
				"DB_CONNECTION_CHECK_URL":           []byte("http://master:3306"),
			},
		}

		config, err := NewExternalDBConfig(mattermost, secret)
		require.NoError(t, err)
		assert.True(t, config.hasSeparateDatasourceKey)
		assert.True(t, config.hasReaderEndpoints)
		assert.True(t, config.hasDBCheckURL)

		envs := config.EnvVars(mattermost)
		assert.Equal(t, 3, len(envs))

		// Verify all environment variables are correctly set
		mmConfigEnv := findEnvVar(envs, "MM_CONFIG")
		require.NotNil(t, mmConfigEnv)
		assert.Equal(t, "DB_CONNECTION_STRING", mmConfigEnv.ValueFrom.SecretKeyRef.Key)

		datasourceEnv := findEnvVar(envs, "MM_SQLSETTINGS_DATASOURCE")
		require.NotNil(t, datasourceEnv)
		assert.Equal(t, "MM_SQLSETTINGS_DATASOURCE", datasourceEnv.ValueFrom.SecretKeyRef.Key)

		replicasEnv := findEnvVar(envs, "MM_SQLSETTINGS_DATASOURCEREPLICAS")
		require.NotNil(t, replicasEnv)
		assert.Equal(t, "MM_SQLSETTINGS_DATASOURCEREPLICAS", replicasEnv.ValueFrom.SecretKeyRef.Key)

		// Verify init container is created
		initContainers := config.InitContainers(mattermost)
		assert.Equal(t, 1, len(initContainers))
		assert.Equal(t, "init-check-database", initContainers[0].Name)
	})
}

// Helper function to find an environment variable by name
func findEnvVar(envs []corev1.EnvVar, name string) *corev1.EnvVar {
	for i := range envs {
		if envs[i].Name == name {
			return &envs[i]
		}
	}
	return nil
}
