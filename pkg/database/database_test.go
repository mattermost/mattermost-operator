package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestDatabaseInfo(t *testing.T) {
	tests := []struct {
		name                string
		secret              *corev1.Secret
		expectedInfo        *Info
		isExternal          bool
		hasDatabaseCheckURL bool
		isValid             bool
	}{
		{
			name:                "empty",
			secret:              &corev1.Secret{},
			expectedInfo:        &Info{DatabaseCheckURL: true},
			isExternal:          false,
			hasDatabaseCheckURL: true,
			isValid:             false,
		},
		{
			name: "external",
			secret: &corev1.Secret{Data: map[string][]byte{
				"DB_CONNECTION_STRING": []byte("mysql://endpoint"),
			}},
			expectedInfo:        &Info{External: true},
			isExternal:          true,
			hasDatabaseCheckURL: false,
			isValid:             true,
		},
		{
			name: "external with endpoint check",
			secret: &corev1.Secret{Data: map[string][]byte{
				"DB_CONNECTION_STRING":    []byte("mysql://endpoint"),
				"DB_CONNECTION_CHECK_URL": []byte("http://endpoint"),
			}},
			expectedInfo:        &Info{External: true, DatabaseCheckURL: true},
			isExternal:          true,
			hasDatabaseCheckURL: true,
			isValid:             true,
		},
		{
			name: "internal",
			secret: &corev1.Secret{Data: map[string][]byte{
				"ROOT_PASSWORD": []byte("root"),
				"USER":          []byte("user"),
				"PASSWORD":      []byte("pass"),
				"DATABASE":      []byte("database1"),
			}},
			expectedInfo: &Info{
				rootPassword:     "root",
				UserName:         "user",
				UserPassword:     "pass",
				DatabaseName:     "database1",
				DatabaseCheckURL: true,
			},
			isExternal:          false,
			hasDatabaseCheckURL: true,
			isValid:             true,
		},
		{
			name: "internal with no username set",
			secret: &corev1.Secret{Data: map[string][]byte{
				"ROOT_PASSWORD": []byte("root"),
				"PASSWORD":      []byte("pass"),
				"DATABASE":      []byte("database1"),
			}},
			expectedInfo: &Info{
				rootPassword:     "root",
				UserPassword:     "pass",
				DatabaseName:     "database1",
				DatabaseCheckURL: true,
			},
			isExternal:          false,
			hasDatabaseCheckURL: true,
			isValid:             false,
		},
		{
			name: "internal with no user password set",
			secret: &corev1.Secret{Data: map[string][]byte{
				"ROOT_PASSWORD": []byte("root"),
				"USER":          []byte("user"),
				"DATABASE":      []byte("database1"),
			}},
			expectedInfo: &Info{
				rootPassword:     "root",
				UserName:         "user",
				DatabaseName:     "database1",
				DatabaseCheckURL: true,
			},
			isExternal:          false,
			hasDatabaseCheckURL: true,
			isValid:             false,
		},
		{
			name: "internal with no database set",
			secret: &corev1.Secret{Data: map[string][]byte{
				"ROOT_PASSWORD": []byte("root"),
				"USER":          []byte("user"),
				"PASSWORD":      []byte("pass"),
			}},
			expectedInfo: &Info{
				rootPassword:     "root",
				UserName:         "user",
				UserPassword:     "pass",
				DatabaseCheckURL: true,
			},
			isExternal:          false,
			hasDatabaseCheckURL: true,
			isValid:             false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := GenerateDatabaseInfoFromSecret(tt.secret)
			assert.Equal(t, tt.expectedInfo, info)
			assert.Equal(t, tt.isExternal, info.IsExternal())
			assert.Equal(t, tt.hasDatabaseCheckURL, info.HasDatabaseCheckURL())
			if !tt.isValid {
				assert.Error(t, info.IsValid())
			} else {
				assert.NoError(t, info.IsValid())
			}
		})
	}
}
