package mattermost

import (
	"fmt"
	"testing"

	"k8s.io/api/networking/v1beta1"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	"github.com/mattermost/mattermost-operator/pkg/database"
	"github.com/mattermost/mattermost-operator/pkg/utils"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

// Going forward, when new logic is added for constructing a kubernetes resource
// to run Mattermost, the test for that resource should be updated to check that
// the right configuration is present.

func TestGenerateService_V1Beta(t *testing.T) {
	tests := []struct {
		name string
		spec mmv1beta.MattermostSpec
	}{
		{
			name: "type headless",
			spec: mmv1beta.MattermostSpec{},
		},
		{
			name: "type load-balancer",
			spec: mmv1beta.MattermostSpec{
				UseServiceLoadBalancer: true,
			},
		},
	}

	expectPort := func(t *testing.T, service *corev1.Service, portNumber int32) {
		t.Helper()
		for _, port := range service.Spec.Ports {
			if port.Port == portNumber {
				return
			}
		}
		assert.Fail(t, fmt.Sprintf("failed to find port %d on service", portNumber))
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mattermost := &mmv1beta.Mattermost{
				Spec: tt.spec,
			}

			service := GenerateServiceV1Beta(mattermost)
			require.NotNil(t, service)

			if mattermost.Spec.UseServiceLoadBalancer {
				assert.Equal(t, service.Spec.Type, corev1.ServiceTypeLoadBalancer)
				expectPort(t, service, 80)
				expectPort(t, service, 443)
			} else {
				expectPort(t, service, 8065)
				expectPort(t, service, 8067)
			}
		})
	}
}

func TestGenerateIngress_V1Beta(t *testing.T) {
	tests := []struct {
		name string
		spec mmv1beta.MattermostSpec
	}{
		{
			name: "no tls",
			spec: mmv1beta.MattermostSpec{},
		},
		{
			name: "use tls",
			spec: mmv1beta.MattermostSpec{
				UseIngressTLS: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mattermost := &mmv1beta.Mattermost{
				Spec: tt.spec,
			}

			ingress := GenerateIngressV1Beta(mattermost)
			require.NotNil(t, ingress)

			assert.Equal(t, v1beta1.PathTypeImplementationSpecific, *ingress.Spec.Rules[0].HTTP.Paths[0].PathType)

			if mattermost.Spec.UseIngressTLS {
				assert.NotNil(t, ingress.Spec.TLS)
			} else {
				assert.Nil(t, ingress.Spec.TLS)
			}
		})
	}
}

func TestGenerateDeployment_V1Beta(t *testing.T) {
	tests := []struct {
		name            string
		spec            mmv1beta.MattermostSpec
		database        DatabaseConfig
		fileStore       *FileStoreInfo
		want            *appsv1.Deployment
		requiredEnv     []string
		requiredEnvVals map[string]string
	}{
		{
			name: "has license",
			spec: mmv1beta.MattermostSpec{
				LicenseSecret: "license-secret",
			},
			want: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Volumes: []corev1.Volume{
								{
									Name: "mattermost-license",
									VolumeSource: corev1.VolumeSource{
										Secret: &corev1.SecretVolumeSource{
											SecretName: "license-secret",
										},
									},
								},
							},
						},
					},
				},
			},
			requiredEnvVals: map[string]string{"MM_SERVICESETTINGS_LICENSEFILELOCATION": "/mattermost-license/license"},
		},
		{
			name:     "external database",
			spec:     mmv1beta.MattermostSpec{},
			database: &ExternalDBConfig{secretName: "database-secret"},
			want:     &appsv1.Deployment{},
		},
		{
			name:        "external database with reader endpoints",
			spec:        mmv1beta.MattermostSpec{},
			database:    &ExternalDBConfig{secretName: "database-secret", hasReaderEndpoints: true},
			want:        &appsv1.Deployment{},
			requiredEnv: []string{"MM_SQLSETTINGS_DATASOURCEREPLICAS"},
		},
		{
			name: "external known database with check url",
			spec: mmv1beta.MattermostSpec{},
			database: &ExternalDBConfig{
				secretName:    "database-secret",
				hasDBCheckURL: true,
				dbType:        database.PostgreSQLDatabase,
			},
			want: &appsv1.Deployment{},
		},
		{
			name: "external unknown database with check url",
			spec: mmv1beta.MattermostSpec{},
			database: &ExternalDBConfig{
				secretName:    "database-secret",
				hasDBCheckURL: true,
				dbType:        "cockroach",
			},
			want: &appsv1.Deployment{},
		},
		{
			name: "external file store",
			spec: mmv1beta.MattermostSpec{},
			fileStore: &FileStoreInfo{
				secretName: "file-store-secret",
				bucketName: "file-store-bucket",
				url:        "s3.amazon.com",
				useS3SSL:   true,
				config:     &ExternalFileStore{},
			},
			want:            &appsv1.Deployment{},
			requiredEnvVals: map[string]string{"MM_FILESETTINGS_AMAZONS3SSL": "true"},
		},
		{
			name: "operator managed file store",
			spec: mmv1beta.MattermostSpec{},
			fileStore: &FileStoreInfo{
				secretName: "file-store-secret",
				bucketName: "file-store-bucket",
				url:        "minio.local.com",
				useS3SSL:   false,
				config:     &OperatorManagedMinioConfig{},
			},
			want:            &appsv1.Deployment{},
			requiredEnvVals: map[string]string{"MM_FILESETTINGS_AMAZONS3SSL": "false"},
		},
		{
			name: "override envs set by default with ones in MM spec",
			spec: mmv1beta.MattermostSpec{
				MattermostEnv: []corev1.EnvVar{
					{Name: "MM_FILESETTINGS_AMAZONS3SSL", Value: "false"},
				},
			},
			fileStore: &FileStoreInfo{
				secretName: "file-store-secret",
				bucketName: "file-store-bucket",
				url:        "s3.amazon.com",
				useS3SSL:   true,
				config:     &ExternalFileStore{},
			},
			want:            &appsv1.Deployment{},
			requiredEnvVals: map[string]string{"MM_FILESETTINGS_AMAZONS3SSL": "false"},
		},
		{
			name: "image pull policy",
			spec: mmv1beta.MattermostSpec{
				ImagePullPolicy: corev1.PullAlways,
			},
			want: &appsv1.Deployment{},
		},
		{
			name: "node selector 1",
			spec: mmv1beta.MattermostSpec{
				Scheduling: mmv1beta.Scheduling{
					NodeSelector: map[string]string{"type": "compute"},
				},
			},
			want: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							NodeSelector: map[string]string{"type": "compute"},
						},
					},
				},
			},
		},
		{
			name: "node selector 2",
			spec: mmv1beta.MattermostSpec{
				Scheduling: mmv1beta.Scheduling{
					NodeSelector: map[string]string{"type": "compute", "size": "big", "region": "iceland"},
				},
			},
			want: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							NodeSelector: map[string]string{"type": "compute", "size": "big", "region": "iceland"},
						},
					},
				},
			},
		},
		{
			name: "node selector nil",
			spec: mmv1beta.MattermostSpec{
				Scheduling: mmv1beta.Scheduling{
					NodeSelector: nil,
				},
			},
			want: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							NodeSelector: nil,
						},
					},
				},
			},
		},
		{
			name: "affinity 1",
			spec: mmv1beta.MattermostSpec{
				Scheduling: mmv1beta.Scheduling{
					Affinity: &corev1.Affinity{
						PodAffinity: &corev1.PodAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{"key": "value"},
									},
								},
							},
						},
					},
				},
			},
			want: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Affinity: &corev1.Affinity{
								PodAffinity: &corev1.PodAffinity{
									RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
										{
											LabelSelector: &metav1.LabelSelector{
												MatchLabels: map[string]string{"key": "value"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "affinity nil",
			spec: mmv1beta.MattermostSpec{
				Scheduling: mmv1beta.Scheduling{
					Affinity: nil,
				},
			},
			want: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Affinity: nil,
						},
					},
				},
			},
		},
		{
			name: "negative app replica",
			spec: mmv1beta.MattermostSpec{
				Replicas: utils.NewInt32(-1),
			},
			want: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Replicas: utils.NewInt32(0),
				},
			},
		},
		{
			name: "nil replicas",
			spec: mmv1beta.MattermostSpec{
				Replicas: nil,
			},
			want: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Replicas: utils.NewInt32(0),
				},
			},
		},
		{
			name: "volumes",
			spec: mmv1beta.MattermostSpec{
				Volumes:      []corev1.Volume{fixVolume()},
				VolumeMounts: []corev1.VolumeMount{fixVolumeMount()},
			},
			want: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Volumes: []corev1.Volume{fixVolume()},
						},
					},
				},
			},
		},
		{
			name: "volumes and licence",
			spec: mmv1beta.MattermostSpec{
				LicenseSecret: "license-secret",
				Volumes:       []corev1.Volume{fixVolume()},
				VolumeMounts:  []corev1.VolumeMount{fixVolumeMount()},
			},
			want: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Volumes: []corev1.Volume{
								fixVolume(),
								{
									Name: "mattermost-license",
									VolumeSource: corev1.VolumeSource{
										Secret: &corev1.SecretVolumeSource{
											SecretName: "license-secret",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "elastic search",
			spec: mmv1beta.MattermostSpec{
				ElasticSearch: mmv1beta.ElasticSearch{
					Host:     "http://elastic",
					UserName: "user",
					Password: "password",
				},
			},
			want: &appsv1.Deployment{},
			requiredEnvVals: map[string]string{
				"MM_ELASTICSEARCHSETTINGS_ENABLEINDEXING":  "true",
				"MM_ELASTICSEARCHSETTINGS_ENABLESEARCHING": "true",
				"MM_ELASTICSEARCHSETTINGS_CONNECTIONURL":   "http://elastic",
				"MM_ELASTICSEARCHSETTINGS_USERNAME":        "user",
				"MM_ELASTICSEARCHSETTINGS_PASSWORD":        "password",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mattermost := &mmv1beta.Mattermost{
				Spec: tt.spec,
			}
			databaseConfig := tt.database
			if databaseConfig == nil {
				databaseConfig = &MySQLDBConfig{}
			}
			fileStoreInfo := tt.fileStore
			if fileStoreInfo == nil {
				fileStoreInfo = NewOperatorManagedFileStoreInfo(mattermost, "minio-secret", "http://minio")
			}

			deployment := GenerateDeploymentV1Beta(mattermost, databaseConfig, fileStoreInfo, "", "", "service-account", "")
			require.NotNil(t, deployment)

			assert.Equal(t, "service-account", deployment.Spec.Template.Spec.ServiceAccountName)
			assert.Equal(t, tt.want.Spec.Template.Spec.NodeSelector, deployment.Spec.Template.Spec.NodeSelector)
			assert.Equal(t, tt.want.Spec.Template.Spec.Affinity, deployment.Spec.Template.Spec.Affinity)
			assert.Equal(t, tt.want.Spec.Template.Spec.Volumes, deployment.Spec.Template.Spec.Volumes)
			assert.Equal(t, len(tt.want.Spec.Template.Spec.Volumes), len(deployment.Spec.Template.Spec.Containers[0].VolumeMounts))

			mattermostAppContainer := mmv1beta.GetMattermostAppContainerFromDeployment(deployment)
			require.NotNil(t, mattermostAppContainer)

			if mattermost.Spec.ImagePullPolicy != "" {
				assert.Equal(t, mattermost.Spec.ImagePullPolicy, mattermostAppContainer.ImagePullPolicy)
			}

			// Basic env var check to ensure the key exists.
			assertEnvVarExists(t, "MM_CONFIG", mattermostAppContainer.Env)
			assertEnvVarExists(t, "MM_SERVICESETTINGS_SITEURL", mattermostAppContainer.Env)
			assertEnvVarExists(t, "MM_METRICSSETTINGS_LISTENADDRESS", mattermostAppContainer.Env)
			assertEnvVarExists(t, "MM_METRICSSETTINGS_ENABLE", mattermostAppContainer.Env)
			assertEnvVarExists(t, "MM_PLUGINSETTINGS_ENABLEUPLOADS", mattermostAppContainer.Env)
			assertEnvVarExists(t, "MM_CLUSTERSETTINGS_ENABLE", mattermostAppContainer.Env)
			assertEnvVarExists(t, "MM_CLUSTERSETTINGS_CLUSTERNAME", mattermostAppContainer.Env)
			assertEnvVarExists(t, "MM_FILESETTINGS_MAXFILESIZE", mattermostAppContainer.Env)
			assertEnvVarExists(t, "MM_INSTALL_TYPE", mattermostAppContainer.Env)

			for _, env := range tt.requiredEnv {
				assertEnvVarExists(t, env, mattermostAppContainer.Env)
			}

			for env, val := range tt.requiredEnvVals {
				assertEnvVarEqual(t, env, val, mattermostAppContainer.Env)
			}

			// External db check.
			expectedInitContainers := 0 // Due to disabling DB setup job we start with 0 init containers

			if externalDB, ok := tt.database.(*ExternalDBConfig); ok {
				if externalDB.hasDBCheckURL {
					if externalDB.dbType == database.MySQLDatabase ||
						externalDB.dbType == database.PostgreSQLDatabase {
						expectedInitContainers++
					}
				}
			} else {
				expectedInitContainers++
				assertEnvVarExists(t, "MYSQL_USERNAME", mattermostAppContainer.Env)
				assertEnvVarExists(t, "MYSQL_PASSWORD", mattermostAppContainer.Env)
			}

			if _, ok := fileStoreInfo.config.(*OperatorManagedMinioConfig); ok {
				expectedInitContainers += 2
			}

			assert.Equal(t, expectedInitContainers, len(deployment.Spec.Template.Spec.InitContainers))

			// Container check.
			assert.Equal(t, 1, len(deployment.Spec.Template.Spec.Containers))
		})
	}

	t.Run("custom DB check init containers", func(t *testing.T) {
		customInitContainers := []corev1.Container{
			{Image: "my-check-image", Name: "custom-check"},
			{Image: "my-other-check-image", Name: "other-custom-check"},
		}
		defaultExternalPostgresInitContainers := []corev1.Container{
			{
				Name:            "init-check-database",
				Image:           "postgres:13",
				ImagePullPolicy: corev1.PullIfNotPresent,
				Command:         []string{"sh", "-c", "until pg_isready --dbname=\"$DB_CONNECTION_CHECK_URL\"; do echo waiting for database; sleep 5; done;"},
				Env:             []corev1.EnvVar{{Name: "DB_CONNECTION_CHECK_URL", Value: "", ValueFrom: EnvSourceFromSecret("secret", "DB_CONNECTION_CHECK_URL")}},
			},
		}

		for _, testCase := range []struct {
			description            string
			dbSpec                 mmv1beta.Database
			dbConfig               DatabaseConfig
			expectedInitContainers []corev1.Container
		}{
			{
				description: "custom init container external DB",
				dbSpec: mmv1beta.Database{
					External: &mmv1beta.ExternalDatabase{},
					ReadinessCheck: &mmv1beta.CheckExtensions{
						InitContainers: customInitContainers,
					},
				},
				dbConfig:               &ExternalDBConfig{dbType: database.PostgreSQLDatabase, hasDBCheckURL: true},
				expectedInitContainers: customInitContainers,
			},
			{
				description: "custom init container operator managed DB",
				dbSpec: mmv1beta.Database{
					OperatorManaged: &mmv1beta.OperatorManagedDatabase{},
					ReadinessCheck: &mmv1beta.CheckExtensions{
						InitContainers: customInitContainers,
					},
				},
				dbConfig:               &MySQLDBConfig{},
				expectedInitContainers: customInitContainers,
			},
			{
				description: "empty init containers slice",
				dbSpec: mmv1beta.Database{
					External: &mmv1beta.ExternalDatabase{},
					ReadinessCheck: &mmv1beta.CheckExtensions{
						InitContainers: []corev1.Container{},
					},
				},
				dbConfig:               &ExternalDBConfig{dbType: database.PostgreSQLDatabase, hasDBCheckURL: true},
				expectedInitContainers: []corev1.Container{},
			},
			{
				description: "nil init containers slice",
				dbSpec: mmv1beta.Database{
					External:       &mmv1beta.ExternalDatabase{},
					ReadinessCheck: &mmv1beta.CheckExtensions{},
				},
				dbConfig:               &ExternalDBConfig{dbType: database.PostgreSQLDatabase, hasDBCheckURL: true, secretName: "secret"},
				expectedInitContainers: defaultExternalPostgresInitContainers,
			},
			{
				description: "nil readiness check",
				dbSpec: mmv1beta.Database{
					External: &mmv1beta.ExternalDatabase{},
				},
				dbConfig:               &ExternalDBConfig{dbType: database.PostgreSQLDatabase, hasDBCheckURL: true, secretName: "secret"},
				expectedInitContainers: defaultExternalPostgresInitContainers,
			},
		} {
			t.Run(testCase.description, func(t *testing.T) {
				mattermost := &mmv1beta.Mattermost{
					Spec: mmv1beta.MattermostSpec{
						Database: testCase.dbSpec,
					},
				}
				deployment := GenerateDeploymentV1Beta(mattermost, testCase.dbConfig, &FileStoreInfo{config: &ExternalFileStore{}}, "", "", "", "image")
				assert.Equal(t, testCase.expectedInitContainers, deployment.Spec.Template.Spec.InitContainers)
			})
		}
	})
}

func TestGenerateRBACResources_V1Beta(t *testing.T) {
	roleName := "role"
	saName := "service-account"
	mattermost := &mmv1beta.Mattermost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-mm",
			Namespace: "test-namespace",
		},
	}

	serviceAccount := GenerateServiceAccountV1Beta(mattermost, saName)
	require.Equal(t, saName, serviceAccount.Name)
	require.Equal(t, mattermost.Namespace, serviceAccount.Namespace)
	require.Equal(t, 1, len(serviceAccount.OwnerReferences))

	role := GenerateRoleV1Beta(mattermost, roleName)
	require.Equal(t, roleName, role.Name)
	require.Equal(t, mattermost.Namespace, role.Namespace)
	require.Equal(t, 1, len(role.OwnerReferences))
	require.Equal(t, 1, len(role.Rules))

	roleBinding := GenerateRoleBindingV1Beta(mattermost, roleName, saName)
	require.Equal(t, roleName, roleBinding.Name)
	require.Equal(t, mattermost.Namespace, roleBinding.Namespace)
	require.Equal(t, 1, len(roleBinding.OwnerReferences))
	require.Equal(t, 1, len(roleBinding.Subjects))
	require.Equal(t, saName, roleBinding.Subjects[0].Name)
	require.Equal(t, roleName, roleBinding.RoleRef.Name)
}

func fixVolume() corev1.Volume {
	return corev1.Volume{
		Name: "test-volume",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: "mounted-secret",
			},
		},
	}
}

func fixVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      "test-volume",
		MountPath: "/etc/test",
	}
}

func assertEnvVarEqual(t *testing.T, name, val string, env []corev1.EnvVar) {
	for _, e := range env {
		if e.Name == name {
			assert.Equal(t, e.Value, val)
			return
		}
	}

	assert.Fail(t, fmt.Sprintf("failed to find env var %s", name))
}
