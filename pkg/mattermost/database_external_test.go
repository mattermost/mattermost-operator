package mattermost

import (
	"testing"
	"time"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	"github.com/mattermost/mattermost-operator/pkg/database"
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

func TestValidateDBCheckURL(t *testing.T) {
	t.Run("valid URLs for MySQL", func(t *testing.T) {
		validURLs := []string{
			"http://my-db:3306",
			"https://my-db:3306",
			"http://10.0.0.1:3306",
		}
		for _, u := range validURLs {
			assert.NoError(t, validateDBCheckURL(u, database.MySQLDatabase), "expected valid: %s", u)
		}
	})

	t.Run("valid URLs for PostgreSQL", func(t *testing.T) {
		validURLs := []string{
			"http://my-db:5432",
			"https://my-db:5432",
			"postgres://my-db:5432/mydb",
			"http://10.0.0.1:5432",
		}
		for _, u := range validURLs {
			assert.NoError(t, validateDBCheckURL(u, database.PostgreSQLDatabase), "expected valid: %s", u)
		}
	})

	t.Run("MySQL rejects mysql scheme", func(t *testing.T) {
		err := validateDBCheckURL("mysql://my-db:3306/mydb", database.MySQLDatabase)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not allowed")
		assert.Contains(t, err.Error(), "mysql")
	})

	t.Run("blocked schemes", func(t *testing.T) {
		blockedURLs := []string{
			"file:///etc/passwd",
			"gopher://evil.com",
			"ftp://my-db:21/file",
			"javascript:alert(1)",
		}
		for _, u := range blockedURLs {
			err := validateDBCheckURL(u, database.MySQLDatabase)
			assert.Error(t, err, "expected blocked: %s", u)
			assert.Contains(t, err.Error(), "not allowed")
		}
	})

	t.Run("empty and invalid", func(t *testing.T) {
		assert.Error(t, validateDBCheckURL("", database.MySQLDatabase))
		assert.Error(t, validateDBCheckURL("://no-scheme", database.MySQLDatabase))
	})
}

func TestNewExternalDBConfig_InvalidCheckURL(t *testing.T) {
	mattermost := &mmv1beta.Mattermost{
		ObjectMeta: metav1.ObjectMeta{Name: "mm-test"},
		Spec: mmv1beta.MattermostSpec{
			Database: mmv1beta.Database{
				External: &mmv1beta.ExternalDatabase{Secret: "secret"},
			},
		},
	}

	t.Run("rejects disallowed scheme", func(t *testing.T) {
		secret := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "secret"},
			Data: map[string][]byte{
				"DB_CONNECTION_STRING":    []byte("postgres://my-postgres"),
				"DB_CONNECTION_CHECK_URL": []byte("file:///etc/passwd"),
			},
		}
		_, err := NewExternalDBConfig(mattermost, secret)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid DB_CONNECTION_CHECK_URL")
	})

	t.Run("rejects mysql scheme for MySQL db type", func(t *testing.T) {
		secret := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "secret"},
			Data: map[string][]byte{
				"DB_CONNECTION_STRING":    []byte("mysql://user:pass@tcp(host:3306)/db"),
				"DB_CONNECTION_CHECK_URL": []byte("mysql://host:3306/db"),
			},
		}
		_, err := NewExternalDBConfig(mattermost, secret)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid DB_CONNECTION_CHECK_URL")
		assert.Contains(t, err.Error(), "not allowed")
	})
}

func TestGetDBCheckInitContainer_QuotedURL(t *testing.T) {
	t.Run("mysql container quotes URL variable", func(t *testing.T) {
		container := getDBCheckInitContainer(&mmv1beta.Mattermost{}, "secret", "mysql", false)
		require.NotNil(t, container)
		// Verify the command uses double-quoted variable to prevent shell injection
		assert.Contains(t, container.Command[2], `"$DB_CONNECTION_CHECK_URL"`)
	})

	t.Run("postgres container quotes URL variable", func(t *testing.T) {
		container := getDBCheckInitContainer(&mmv1beta.Mattermost{}, "secret", "postgres", false)
		require.NotNil(t, container)
		assert.Contains(t, container.Command[2], `"$DB_CONNECTION_CHECK_URL"`)
	})
}

func TestGetDBCheckInitContainer_BuiltinMode(t *testing.T) {
	makeMattermost := func(rc *mmv1beta.DatabaseReadinessCheck) *mmv1beta.Mattermost {
		return &mmv1beta.Mattermost{
			Spec: mmv1beta.MattermostSpec{
				Image:   "mattermost/mattermost",
				Version: "10.8.1",
				Database: mmv1beta.Database{
					ReadinessCheck: rc,
				},
			},
		}
	}

	t.Run("builtin mode for postgres", func(t *testing.T) {
		mm := makeMattermost(&mmv1beta.DatabaseReadinessCheck{
			Mode: mmv1beta.DatabaseReadinessCheckModeBuiltin,
		})

		container := getDBCheckInitContainer(mm, "secret", database.PostgreSQLDatabase, false)
		require.NotNil(t, container)

		assert.Equal(t, "init-check-database", container.Name)
		assert.Equal(t, "mattermost/mattermost:10.8.1", container.Image)
		assert.Equal(t, []string{"/mattermost/bin/mattermost"}, container.Command)
		assert.Equal(t, []string{"db", "ping", "--timeout=5m0s"}, container.Args)

		mmConfigEnv := findEnvVar(container.Env, "MM_CONFIG")
		require.NotNil(t, mmConfigEnv)
		require.NotNil(t, mmConfigEnv.ValueFrom)
		require.NotNil(t, mmConfigEnv.ValueFrom.SecretKeyRef)
		assert.Equal(t, "secret", mmConfigEnv.ValueFrom.SecretKeyRef.Name)
		assert.Equal(t, "DB_CONNECTION_STRING", mmConfigEnv.ValueFrom.SecretKeyRef.Key)

		datasourceEnv := findEnvVar(container.Env, "MM_SQLSETTINGS_DATASOURCE")
		require.NotNil(t, datasourceEnv)
		require.NotNil(t, datasourceEnv.ValueFrom)
		require.NotNil(t, datasourceEnv.ValueFrom.SecretKeyRef)
		assert.Equal(t, "secret", datasourceEnv.ValueFrom.SecretKeyRef.Name)
		assert.Equal(t, "DB_CONNECTION_STRING", datasourceEnv.ValueFrom.SecretKeyRef.Key)
	})

	t.Run("builtin mode honors separate datasource key", func(t *testing.T) {
		mm := makeMattermost(&mmv1beta.DatabaseReadinessCheck{
			Mode: mmv1beta.DatabaseReadinessCheckModeBuiltin,
		})

		container := getDBCheckInitContainer(mm, "secret", database.PostgreSQLDatabase, true)
		require.NotNil(t, container)

		datasourceEnv := findEnvVar(container.Env, "MM_SQLSETTINGS_DATASOURCE")
		require.NotNil(t, datasourceEnv)
		require.NotNil(t, datasourceEnv.ValueFrom)
		require.NotNil(t, datasourceEnv.ValueFrom.SecretKeyRef)
		assert.Equal(t, "MM_SQLSETTINGS_DATASOURCE", datasourceEnv.ValueFrom.SecretKeyRef.Key)
	})

	t.Run("builtin mode honors custom timeout", func(t *testing.T) {
		mm := makeMattermost(&mmv1beta.DatabaseReadinessCheck{
			Mode:    mmv1beta.DatabaseReadinessCheckModeBuiltin,
			Timeout: &metav1.Duration{Duration: 10 * time.Minute},
		})

		container := getDBCheckInitContainer(mm, "secret", database.PostgreSQLDatabase, false)
		require.NotNil(t, container)

		assert.Contains(t, container.Args, "--timeout=10m0s")
	})

	t.Run("builtin mode for mysql", func(t *testing.T) {
		mm := makeMattermost(&mmv1beta.DatabaseReadinessCheck{
			Mode: mmv1beta.DatabaseReadinessCheckModeBuiltin,
		})

		container := getDBCheckInitContainer(mm, "secret", database.MySQLDatabase, false)
		require.NotNil(t, container)

		// builtin mode is dbType-agnostic: it always uses the Mattermost image,
		// not appropriate/curl:latest.
		assert.Equal(t, "mattermost/mattermost:10.8.1", container.Image)
		assert.NotEqual(t, "appropriate/curl:latest", container.Image)
		assert.Equal(t, []string{"/mattermost/bin/mattermost"}, container.Command)
		assert.Equal(t, []string{"db", "ping", "--timeout=5m0s"}, container.Args)
	})

	t.Run("builtin mode honors imagePullPolicy", func(t *testing.T) {
		mm := makeMattermost(&mmv1beta.DatabaseReadinessCheck{
			Mode: mmv1beta.DatabaseReadinessCheckModeBuiltin,
		})
		mm.Spec.ImagePullPolicy = corev1.PullAlways

		container := getDBCheckInitContainer(mm, "secret", database.PostgreSQLDatabase, false)
		require.NotNil(t, container)

		assert.Equal(t, corev1.PullAlways, container.ImagePullPolicy)
	})

	t.Run("builtin mode digest version", func(t *testing.T) {
		mm := makeMattermost(&mmv1beta.DatabaseReadinessCheck{
			Mode: mmv1beta.DatabaseReadinessCheckModeBuiltin,
		})
		mm.Spec.Version = "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"

		container := getDBCheckInitContainer(mm, "secret", database.PostgreSQLDatabase, false)
		require.NotNil(t, container)

		assert.Equal(
			t,
			"mattermost/mattermost@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
			container.Image,
		)
	})

	t.Run("external mode explicit", func(t *testing.T) {
		mm := makeMattermost(&mmv1beta.DatabaseReadinessCheck{
			Mode: mmv1beta.DatabaseReadinessCheckModeExternal,
		})

		container := getDBCheckInitContainer(mm, "secret", database.PostgreSQLDatabase, false)
		require.NotNil(t, container)
		assert.Equal(t, "postgres:13", container.Image)
	})

	t.Run("empty readiness check mode falls back to external", func(t *testing.T) {
		mm := makeMattermost(&mmv1beta.DatabaseReadinessCheck{Mode: ""})

		container := getDBCheckInitContainer(mm, "secret", database.PostgreSQLDatabase, false)
		require.NotNil(t, container)
		assert.Equal(t, "postgres:13", container.Image)
	})

	t.Run("nil readiness check falls back to external", func(t *testing.T) {
		mm := makeMattermost(nil)

		container := getDBCheckInitContainer(mm, "secret", database.PostgreSQLDatabase, false)
		require.NotNil(t, container)
		assert.Equal(t, "postgres:13", container.Image)
	})
}

func TestExternalDBConfig_InitContainers_BuiltinMode(t *testing.T) {
	t.Run("builtin mode emits init container without DB_CONNECTION_CHECK_URL", func(t *testing.T) {
		mm := &mmv1beta.Mattermost{
			Spec: mmv1beta.MattermostSpec{
				Image:   "mattermost/mattermost",
				Version: "10.8.1",
				Database: mmv1beta.Database{
					External: &mmv1beta.ExternalDatabase{Secret: "db-secret"},
					ReadinessCheck: &mmv1beta.DatabaseReadinessCheck{
						Mode: mmv1beta.DatabaseReadinessCheckModeBuiltin,
					},
				},
			},
		}
		secret := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "db-secret"},
			Data: map[string][]byte{
				"DB_CONNECTION_STRING": []byte("postgres://user:pass@host:5432/db"),
			},
		}

		config, err := NewExternalDBConfig(mm, secret)
		require.NoError(t, err)

		containers := config.InitContainers(mm)
		require.Len(t, containers, 1)
		assert.Equal(t, "mattermost/mattermost:10.8.1", containers[0].Image)
		assert.Equal(t, []string{"/mattermost/bin/mattermost"}, containers[0].Command)
	})

	t.Run("builtin mode + DB_CONNECTION_CHECK_URL still uses mattermost image", func(t *testing.T) {
		mm := &mmv1beta.Mattermost{
			Spec: mmv1beta.MattermostSpec{
				Image:   "mattermost/mattermost",
				Version: "10.8.1",
				Database: mmv1beta.Database{
					External: &mmv1beta.ExternalDatabase{Secret: "db-secret"},
					ReadinessCheck: &mmv1beta.DatabaseReadinessCheck{
						Mode: mmv1beta.DatabaseReadinessCheckModeBuiltin,
					},
				},
			},
		}
		secret := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "db-secret"},
			Data: map[string][]byte{
				"DB_CONNECTION_STRING":    []byte("postgres://user:pass@host:5432/db"),
				"DB_CONNECTION_CHECK_URL": []byte("postgres://host:5432/db"),
			},
		}

		config, err := NewExternalDBConfig(mm, secret)
		require.NoError(t, err)

		containers := config.InitContainers(mm)
		require.Len(t, containers, 1)
		assert.Equal(t, "mattermost/mattermost:10.8.1", containers[0].Image)
		assert.NotEqual(t, "postgres:13", containers[0].Image)
	})

	t.Run("external mode skips init container when DB_CONNECTION_CHECK_URL absent", func(t *testing.T) {
		mm := &mmv1beta.Mattermost{
			Spec: mmv1beta.MattermostSpec{
				Database: mmv1beta.Database{
					External: &mmv1beta.ExternalDatabase{Secret: "db-secret"},
					ReadinessCheck: &mmv1beta.DatabaseReadinessCheck{
						Mode: mmv1beta.DatabaseReadinessCheckModeExternal,
					},
				},
			},
		}
		secret := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "db-secret"},
			Data: map[string][]byte{
				"DB_CONNECTION_STRING": []byte("postgres://user:pass@host:5432/db"),
			},
		}

		config, err := NewExternalDBConfig(mm, secret)
		require.NoError(t, err)

		containers := config.InitContainers(mm)
		assert.Empty(t, containers)
	})

	t.Run("disableReadinessCheck overrides builtin", func(t *testing.T) {
		mm := &mmv1beta.Mattermost{
			Spec: mmv1beta.MattermostSpec{
				Image:   "mattermost/mattermost",
				Version: "10.8.1",
				Database: mmv1beta.Database{
					External:              &mmv1beta.ExternalDatabase{Secret: "db-secret"},
					DisableReadinessCheck: true,
					ReadinessCheck: &mmv1beta.DatabaseReadinessCheck{
						Mode: mmv1beta.DatabaseReadinessCheckModeBuiltin,
					},
				},
			},
		}
		secret := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "db-secret"},
			Data: map[string][]byte{
				"DB_CONNECTION_STRING": []byte("postgres://user:pass@host:5432/db"),
			},
		}

		config, err := NewExternalDBConfig(mm, secret)
		require.NoError(t, err)

		containers := config.InitContainers(mm)
		assert.Empty(t, containers)
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
