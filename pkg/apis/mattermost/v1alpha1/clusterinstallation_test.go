package v1alpha1

import (
	"testing"

	operatortest "github.com/mattermost/mattermost-operator/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestClusterInstallation(t *testing.T) {
	ci := &ClusterInstallation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: ClusterInstallationSpec{
			Replicas:    7,
			Image:       "mattermost/mattermost-enterprise-edition",
			Version:     operatortest.LatestStableMattermostVersion,
			IngressName: "foo.mattermost.dev",
		},
	}

	t.Run("scheme", func(t *testing.T) {
		err := SchemeBuilder.AddToScheme(scheme.Scheme)
		require.NoError(t, err)
	})

	t.Run("deepcopy", func(t *testing.T) {
		t.Run("cluster installation", func(t *testing.T) {
			require.Equal(t, ci, ci.DeepCopy())
		})
		t.Run("cluster installation list", func(t *testing.T) {
			cil := &ClusterInstallationList{
				Items: []ClusterInstallation{
					*ci,
				},
			}
			require.Equal(t, cil, cil.DeepCopy())
		})
	})

	t.Run("set replicas and resources with user count", func(t *testing.T) {
		ci = &ClusterInstallation{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "default",
			},
			Spec: ClusterInstallationSpec{
				Image:       "mattermost/mattermost-enterprise-edition",
				Version:     operatortest.LatestStableMattermostVersion,
				IngressName: "foo.mattermost.dev",
				Size:        "1000users",
			},
		}

		t.Run("should set correctly", func(t *testing.T) {
			tci := ci.DeepCopy()
			err := tci.SetReplicasAndResourcesFromSize()
			require.NoError(t, err)
			assert.Equal(t, size1000.App.Replicas, tci.Spec.Replicas)
			assert.Equal(t, size1000.App.Resources.String(), tci.Spec.Resources.String())
			assert.Equal(t, size1000.Minio.Replicas, tci.Spec.Minio.Replicas)
			assert.Equal(t, size1000.Minio.Resources.String(), tci.Spec.Minio.Resources.String())
			assert.Equal(t, size1000.Database.Replicas, tci.Spec.Database.Replicas)
			assert.Equal(t, size1000.Database.Resources.String(), tci.Spec.Database.Resources.String())
		})

		t.Run("should not override manually set replicas or resources", func(t *testing.T) {
			tci := ci.DeepCopy()
			resources := corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("100Mi"),
				},
			}
			expectedResources := resources.String()

			expectedReplicas := int32(7)
			tci.Spec.Replicas = expectedReplicas
			tci.Spec.Resources = resources
			tci.Spec.Minio.Replicas = expectedReplicas
			tci.Spec.Minio.Resources = resources
			tci.Spec.Database.Replicas = expectedReplicas
			tci.Spec.Database.Resources = resources

			err := tci.SetReplicasAndResourcesFromSize()
			require.NoError(t, err)
			assert.Equal(t, expectedReplicas, tci.Spec.Replicas)
			assert.Equal(t, expectedResources, tci.Spec.Resources.String())
			assert.Equal(t, expectedReplicas, tci.Spec.Minio.Replicas)
			assert.Equal(t, expectedResources, tci.Spec.Minio.Resources.String())
			assert.Equal(t, expectedReplicas, tci.Spec.Database.Replicas)
			assert.Equal(t, expectedResources, tci.Spec.Database.Resources.String())
		})

		t.Run("should error on bad user count but set to default size", func(t *testing.T) {
			tci := ci.DeepCopy()
			tci.Spec.Size = "junk"
			err := tci.SetReplicasAndResourcesFromSize()
			assert.Error(t, err)
			assert.Equal(t, defaultSize.App.Replicas, tci.Spec.Replicas)
			assert.Equal(t, defaultSize.App.Resources.String(), tci.Spec.Resources.String())
			assert.Equal(t, defaultSize.Minio.Replicas, tci.Spec.Minio.Replicas)
			assert.Equal(t, defaultSize.Minio.Resources.String(), tci.Spec.Minio.Resources.String())
			assert.Equal(t, defaultSize.Database.Replicas, tci.Spec.Database.Replicas)
			assert.Equal(t, defaultSize.Database.Resources.String(), tci.Spec.Database.Resources.String())
		})
	})

	t.Run("correct image", func(t *testing.T) {
		assert.Contains(t, ci.GetImageName(), ci.Spec.Image)
		assert.Contains(t, ci.GetImageName(), ci.Spec.Version)
		assert.Contains(t, ci.GetImageName(), ":")
	})

	t.Run("bluegreen", func(t *testing.T) {

		t.Run("correct production deployment name", func(t *testing.T) {
			ci.Spec.BlueGreen.Blue = AppDeployment{
				Name: "blue",
			}
			ci.Spec.BlueGreen.Green = AppDeployment{
				Name: "green",
			}
			ci.Spec.BlueGreen.ProductionDeployment = BlueName

			assert.Equal(t, ci.GetProductionDeploymentName(), ci.Name)

			ci.Spec.BlueGreen.Enable = true
			assert.Equal(t, ci.GetProductionDeploymentName(), ci.Spec.BlueGreen.Blue.Name)

			ci.Spec.BlueGreen.ProductionDeployment = GreenName
			assert.Equal(t, ci.GetProductionDeploymentName(), ci.Spec.BlueGreen.Green.Name)
		})

	})
}

func TestDeployment(t *testing.T) {
	d := AppDeployment{
		Image:   "mattermost/mattermost-enterprise-edition",
		Version: operatortest.LatestStableMattermostVersion,
	}

	t.Run("correct image", func(t *testing.T) {
		assert.Contains(t, d.GetDeploymentImageName(), d.Image)
		assert.Contains(t, d.GetDeploymentImageName(), d.Version)
		assert.Contains(t, d.GetDeploymentImageName(), ":")
	})
}

func TestCalculateResourceMilliRequirements(t *testing.T) {
	cis := ClusterInstallationSize{
		App: ComponentSize{
			Replicas: 3,
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("100k"),
				},
			},
		},
		Minio: ComponentSize{
			Replicas: 6,
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("100k"),
				},
			},
		},
		Database: ComponentSize{
			Replicas: 2,
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("100k"),
				},
			},
		},
	}

	t.Run("baseline", func(t *testing.T) {
		t.Run("all components", func(t *testing.T) {
			cpu, memory := cis.CalculateResourceMilliRequirements(true, true)
			assert.Equal(t, int64(1100), cpu)
			assert.Equal(t, int64(1100000000), memory)
		})
		t.Run("no database", func(t *testing.T) {
			cpu, memory := cis.CalculateResourceMilliRequirements(false, true)
			assert.Equal(t, int64(900), cpu)
			assert.Equal(t, int64(900000000), memory)
		})
		t.Run("no minio", func(t *testing.T) {
			cpu, memory := cis.CalculateResourceMilliRequirements(true, false)
			assert.Equal(t, int64(500), cpu)
			assert.Equal(t, int64(500000000), memory)
		})
		t.Run("no database or minio", func(t *testing.T) {
			cpu, memory := cis.CalculateResourceMilliRequirements(false, false)
			assert.Equal(t, int64(300), cpu)
			assert.Equal(t, int64(300000000), memory)
		})
	})

	t.Run("updated", func(t *testing.T) {
		cis.App.Replicas = 10
		cis.App.Resources.Requests = corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("100G"),
		}

		cpu, memory := cis.CalculateResourceMilliRequirements(false, false)
		assert.Equal(t, int64(10000), cpu)
		assert.Equal(t, int64(1000000000000000), memory)
	})
}

// This is a basic sanity check on any cluster size we define as valid.
func TestCalculateResourceMilliRequirementsOnAllValidClusterSizes(t *testing.T) {
	for name, cis := range validSizes {
		t.Run(name, func(t *testing.T) {
			cpu, memory := cis.CalculateResourceMilliRequirements(true, true)
			assert.True(t, cpu > 0)
			assert.True(t, memory > 0)
			assert.Equal(t, cpu, cis.CalculateCPUMilliRequirement(true, true))
			assert.Equal(t, memory, cis.CalculateMemoryMilliRequirement(true, true))
		})
	}
}

func TestClusterInstallationGenerateDeployment(t *testing.T) {
	tests := []struct {
		name string
		Spec ClusterInstallationSpec
		want *appsv1.Deployment
	}{
		{
			name: "node selector 1",
			Spec: ClusterInstallationSpec{
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
			Spec: ClusterInstallationSpec{
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
			Spec: ClusterInstallationSpec{
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
			Spec: ClusterInstallationSpec{
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
			Spec: ClusterInstallationSpec{
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mattermost := &ClusterInstallation{
				Spec: tt.Spec,
			}

			deployment := mattermost.GenerateDeployment("", "", "", "", "", "", false, false, "")
			require.Equal(t, tt.want.Spec.Template.Spec.NodeSelector, deployment.Spec.Template.Spec.NodeSelector)
			require.Equal(t, tt.want.Spec.Template.Spec.Affinity, deployment.Spec.Template.Spec.Affinity)
		})
	}
}
