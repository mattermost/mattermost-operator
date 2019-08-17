package v1alpha1

import (
	"reflect"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestClusterInstallation_GenerateDeployment_nodeSelector(t *testing.T) {
	tests := []struct {
		name string
		Spec ClusterInstallationSpec
		want *appsv1.Deployment
	}{
		{
			name: "check if node selector is propagated",
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
			name: "check if node selector is propagated",
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
			name: "nil",
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mattermost := &ClusterInstallation{
				Spec: tt.Spec,
			}
			if got := mattermost.GenerateDeployment("", "", false, false, ""); !reflect.DeepEqual(got.Spec.Template.Spec.NodeSelector, tt.want.Spec.Template.Spec.NodeSelector) {
				t.Errorf("GenerateDeployment() = %v, want %v", got.Spec.Template.Spec.NodeSelector, tt.want.Spec.Template.Spec.NodeSelector)
			}
		})
	}
}

func TestClusterInstallation_GenerateDeployment_affinity(t *testing.T) {
	tests := []struct {
		name string
		Spec ClusterInstallationSpec
		want *appsv1.Deployment
	}{
		{
			name: "check if affinity is propagated",
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
			name: "nil",
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
			if got := mattermost.GenerateDeployment("", "", false, false, ""); !reflect.DeepEqual(got.Spec.Template.Spec.NodeSelector, tt.want.Spec.Template.Spec.NodeSelector) {
				t.Errorf("GenerateDeployment() = %v, want %v", got.Spec.Template.Spec.NodeSelector, tt.want.Spec.Template.Spec.NodeSelector)
			}
		})
	}
}
