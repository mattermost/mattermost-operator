package mattermost

import (
	"testing"

	mattermostv1beta1 "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewMySQLDB(t *testing.T) {
	mattermost := &mattermostv1beta1.Mattermost{
		ObjectMeta: metav1.ObjectMeta{Name: "mm-test"},
		Spec: mattermostv1beta1.MattermostSpec{
			Database: mattermostv1beta1.Database{
				OperatorManaged: &mattermostv1beta1.OperatorManagedDatabase{},
			},
		},
	}

	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "secret"},
		Data: map[string][]byte{
			"ROOT_PASSWORD": []byte("root-pass"),
			"USER":          []byte("user"),
			"PASSWORD":      []byte("pass"),
			"DATABASE":      []byte("db"),
		},
	}

	t.Run("create config", func(t *testing.T) {
		config, err := NewMySQLDBConfig(secret)
		require.NoError(t, err)
		assert.Equal(t, "secret", config.secretName)
		assert.Equal(t, "root-pass", config.rootPassword)
		assert.Equal(t, "user", config.userName)
		assert.Equal(t, "pass", config.userPassword)
		assert.Equal(t, "db", config.databaseName)

		envs := config.EnvVars(mattermost)
		assert.Equal(t, 4, len(envs))

		initContainers := config.InitContainers(mattermost)
		assert.Equal(t, 1, len(initContainers))
	})

	t.Run("should fail if missing key", func(t *testing.T) {
		for _, testCase := range []struct {
			description string
			missingKey  string
		}{
			{
				description: "root pass",
				missingKey:  "ROOT_PASSWORD",
			},
			{
				description: "user",
				missingKey:  "USER",
			},
			{
				description: "pass",
				missingKey:  "PASSWORD",
			},
			{
				description: "db",
				missingKey:  "DATABASE",
			},
		} {
			t.Run(testCase.description, func(t *testing.T) {
				secret := corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "secret"},
					Data: map[string][]byte{
						"ROOT_PASSWORD": []byte("root-pass"),
						"USER":          []byte("user"),
						"PASSWORD":      []byte("pass"),
						"DATABASE":      []byte("db"),
					},
				}

				delete(secret.Data, testCase.missingKey)

				_, err := NewMySQLDBConfig(secret)
				require.Error(t, err)
			})
		}
	})
}
