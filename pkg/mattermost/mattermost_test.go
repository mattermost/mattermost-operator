package mattermost

import (
	"fmt"
	"testing"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
	"github.com/mattermost/mattermost-operator/pkg/database"
	"github.com/mattermost/mattermost-operator/pkg/utils"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TODO: improve these tests beyond basic checks
// Going forward, when new logic is added for constructing a kubernetes resource
// to run Mattermost, the test for that resource should be updated to check that
// the right configuration is present.

func TestGenerateService(t *testing.T) {
	tests := []struct {
		name string
		spec mattermostv1alpha1.ClusterInstallationSpec
	}{
		{
			name: "type headless",
			spec: mattermostv1alpha1.ClusterInstallationSpec{},
		},
		{
			name: "type load-balancer",
			spec: mattermostv1alpha1.ClusterInstallationSpec{
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
			mattermost := &mattermostv1alpha1.ClusterInstallation{
				Spec: tt.spec,
			}

			service := GenerateService(mattermost, "", "")
			require.NotNil(t, service)

			if mattermost.Spec.UseServiceLoadBalancer {
				assert.Equal(t, service.Spec.Type, corev1.ServiceTypeLoadBalancer)
				expectPort(t, service, 80)
				expectPort(t, service, 443)
			} else {
				expectPort(t, service, 8065)
			}
		})
	}
}

func TestGenerateIngress(t *testing.T) {
	tests := []struct {
		name string
		spec mattermostv1alpha1.ClusterInstallationSpec
	}{
		{
			name: "no tls",
			spec: mattermostv1alpha1.ClusterInstallationSpec{},
		},
		{
			name: "use tls",
			spec: mattermostv1alpha1.ClusterInstallationSpec{
				UseIngressTLS: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mattermost := &mattermostv1alpha1.ClusterInstallation{
				Spec: tt.spec,
			}

			ingress := GenerateIngress(mattermost, "", "", nil)
			require.NotNil(t, ingress)

			if mattermost.Spec.UseIngressTLS {
				assert.NotNil(t, ingress.Spec.TLS)
			} else {
				assert.Nil(t, ingress.Spec.TLS)
			}
		})
	}
}

func TestGenerateDeployment(t *testing.T) {
	tests := []struct {
		name     string
		spec     mattermostv1alpha1.ClusterInstallationSpec
		database *database.Info
		want     *appsv1.Deployment
	}{
		{
			name: "has license",
			spec: mattermostv1alpha1.ClusterInstallationSpec{
				MattermostLicenseSecret: "license-secret",
			},
			want: &appsv1.Deployment{},
		},
		{
			name: "has license and version 5.26.0",
			spec: mattermostv1alpha1.ClusterInstallationSpec{
				MattermostLicenseSecret: "license-secret",
				Version:                 "5.26.0",
				Image:                   "mattermost/mattermost-team-edition",
			},
			want: &appsv1.Deployment{},
		},
		{
			name: "external database",
			spec: mattermostv1alpha1.ClusterInstallationSpec{
				Database: mattermostv1alpha1.Database{
					Secret: "database-secret",
				},
			},
			database: &database.Info{External: true},
			want:     &appsv1.Deployment{},
		},
		{
			name: "external database with reader endpoints",
			spec: mattermostv1alpha1.ClusterInstallationSpec{
				Database: mattermostv1alpha1.Database{
					Secret: "database-secret",
				},
			},
			database: &database.Info{External: true, ReaderEndpoints: true},
			want:     &appsv1.Deployment{},
		},
		{
			name: "external database with check url",
			spec: mattermostv1alpha1.ClusterInstallationSpec{
				Database: mattermostv1alpha1.Database{
					Secret: "database-secret",
				},
			},
			database: &database.Info{External: true, DatabaseCheckURL: true},
			want:     &appsv1.Deployment{},
		},
		{
			name: "node selector 1",
			spec: mattermostv1alpha1.ClusterInstallationSpec{
				NodeSelector: map[string]string{"type": "compute"},
			},
			want: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
							NodeSelector: map[string]string{"type": "compute"},
						},
					},
				},
			},
		},
		{
			name: "node selector 2",
			spec: mattermostv1alpha1.ClusterInstallationSpec{
				NodeSelector: map[string]string{"type": "compute", "size": "big", "region": "iceland"},
			},
			want: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
							NodeSelector: map[string]string{"type": "compute", "size": "big", "region": "iceland"},
						},
					},
				},
			},
		},
		{
			name: "node selector nil",
			spec: mattermostv1alpha1.ClusterInstallationSpec{
				NodeSelector: nil,
			},
			want: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
							NodeSelector: nil,
						},
					},
				},
			},
		},
		{
			name: "affinity 1",
			spec: mattermostv1alpha1.ClusterInstallationSpec{
				Affinity: &v1.Affinity{
					PodAffinity: &v1.PodAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
							{
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"key": "value"},
								},
							},
						},
					},
				},
			},
			want: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
							Affinity: &v1.Affinity{
								PodAffinity: &v1.PodAffinity{
									RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
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
			spec: mattermostv1alpha1.ClusterInstallationSpec{
				Affinity: nil,
			},
			want: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
							Affinity: nil,
						},
					},
				},
			},
		},
		{
			name: "negative app replica",
			spec: mattermostv1alpha1.ClusterInstallationSpec{
				Replicas: -1,
			},
			want: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Replicas: utils.NewInt32(0),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mattermost := &mattermostv1alpha1.ClusterInstallation{
				Spec: tt.spec,
			}
			databaseInfo := tt.database
			if databaseInfo == nil {
				databaseInfo = &database.Info{}
			}

			deployment := &appsv1.Deployment{}
			if mattermost.Spec.Version != "5.26.0" {
				deployment = GenerateDeployment(mattermost, databaseInfo, "", "", "", "", "")
			} else {
				deployment = GenerateDeployment(mattermost, databaseInfo, "", "", "", "honk", "")
			}

			require.NotNil(t, deployment)

			assert.Equal(t, tt.want.Spec.Template.Spec.NodeSelector, deployment.Spec.Template.Spec.NodeSelector)
			assert.Equal(t, tt.want.Spec.Template.Spec.Affinity, deployment.Spec.Template.Spec.Affinity)

			mattermostAppContainer := mattermost.GetMattermostAppContainer(deployment)
			require.NotNil(t, mattermostAppContainer)

			// Basic env var check to ensure the key exists.
			assertEnvVarExists(t, "MM_CONFIG", mattermostAppContainer.Env)
			assertEnvVarExists(t, "MM_SERVICESETTINGS_SITEURL", mattermostAppContainer.Env)
			assertEnvVarExists(t, "MM_METRICSSETTINGS_LISTENADDRESS", mattermostAppContainer.Env)
			assertEnvVarExists(t, "MM_METRICSSETTINGS_ENABLE", mattermostAppContainer.Env)
			assertEnvVarExists(t, "MM_PLUGINSETTINGS_ENABLEUPLOADS", mattermostAppContainer.Env)
			assertEnvVarExists(t, "MM_CLUSTERSETTINGS_ENABLE", mattermostAppContainer.Env)
			assertEnvVarExists(t, "MM_CLUSTERSETTINGS_CLUSTERNAME", mattermostAppContainer.Env)
			assertEnvVarExists(t, "MM_FILESETTINGS_MAXFILESIZE", mattermostAppContainer.Env)

			if databaseInfo.HasReaderEndpoints() {
				assertEnvVarExists(t, "MM_SQLSETTINGS_DATASOURCEREPLICAS", mattermostAppContainer.Env)
			}

			if !databaseInfo.IsExternal() {
				assertEnvVarExists(t, "MYSQL_USERNAME", mattermostAppContainer.Env)
				assertEnvVarExists(t, "MYSQL_PASSWORD", mattermostAppContainer.Env)
			}

			// License check.
			if len(mattermost.Spec.MattermostLicenseSecret) != 0 {
				var hasClusterEnableEnv, hasClusterNameEnv, hasOldMMLicenseEnv, hasMMLicenseEnv bool

				for _, env := range mattermostAppContainer.Env {
					switch env.Name {
					case "MM_CLUSTERSETTINGS_ENABLE":
						hasClusterEnableEnv = true
					case "MM_CLUSTERSETTINGS_CLUSTERNAME":
						hasClusterNameEnv = true
					case "MM_LICENSE":
						hasMMLicenseEnv = true
					case "MM_SERVICESETTINGS_LICENSEFILELOCATION":
						hasOldMMLicenseEnv = true
					}
				}

				assert.Truef(t, hasClusterEnableEnv, "Should have cluster enable env set")
				assert.Truef(t, hasClusterNameEnv, "Should have cluster name env set")
				assert.Truef(t, hasMMLicenseEnv, "Should have MM_LICENSE env set")
				if mattermost.Spec.Version != "5.26.0" {
					assert.Truef(t, hasOldMMLicenseEnv, "Should have MM_SERVICESETTINGS_LICENSEFILELOCATION env set")
				} else {
					assert.Falsef(t, hasOldMMLicenseEnv, "Should not have MM_SERVICESETTINGS_LICENSEFILELOCATION env set")
				}
			}

			// Init container check.
			var expectedInitContainers int
			if !databaseInfo.IsExternal() {
				expectedInitContainers++
			} else if databaseInfo.IsExternal() && databaseInfo.HasDatabaseCheckURL() {
				expectedInitContainers++
			}
			if !mattermost.Spec.Minio.IsExternal() {
				expectedInitContainers += 2
			}
			assert.Equal(t, expectedInitContainers, len(deployment.Spec.Template.Spec.InitContainers))

			// Container check.
			assert.Equal(t, 1, len(deployment.Spec.Template.Spec.Containers))
		})
	}
}

func assertEnvVarExists(t *testing.T, name string, env []corev1.EnvVar) {
	for _, e := range env {
		if e.Name == name {
			return
		}
	}

	assert.Fail(t, fmt.Sprintf("failed to find env var %s", name))
}
