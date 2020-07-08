package v1alpha1

import (
	"testing"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
	"github.com/mattermost/mattermost-operator/pkg/database"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestClusterInstallationGenerateDeployment(t *testing.T) {
	tests := []struct {
		name string
		Spec mattermostv1alpha1.ClusterInstallationSpec
		want *appsv1.Deployment
	}{
		{
			name: "node selector 1",
			Spec: mattermostv1alpha1.ClusterInstallationSpec{
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
			Spec: mattermostv1alpha1.ClusterInstallationSpec{
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
			Spec: mattermostv1alpha1.ClusterInstallationSpec{
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
			Spec: mattermostv1alpha1.ClusterInstallationSpec{
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
			Spec: mattermostv1alpha1.ClusterInstallationSpec{
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
			Spec: mattermostv1alpha1.ClusterInstallationSpec{
				Replicas: -1,
			},
			want: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Replicas: i32p(0),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mattermost := &mattermostv1alpha1.ClusterInstallation{
				Spec: tt.Spec,
			}

			deployment := GenerateDeployment(mattermost, "", "", "", false, "", &database.Info{})
			require.Equal(t, tt.want.Spec.Template.Spec.NodeSelector, deployment.Spec.Template.Spec.NodeSelector)
			require.Equal(t, tt.want.Spec.Template.Spec.Affinity, deployment.Spec.Template.Spec.Affinity)
		})
	}
}

func TestMergeEnvVars(t *testing.T) {
	tests := []struct {
		name     string
		original []corev1.EnvVar
		new      []corev1.EnvVar
		want     []corev1.EnvVar
	}{
		{
			name:     "empty",
			original: []corev1.EnvVar{},
			new:      []corev1.EnvVar{},
			want:     []corev1.EnvVar{},
		},
		{
			name:     "append",
			original: []corev1.EnvVar{},
			new:      []corev1.EnvVar{{Name: "env1", Value: "value1"}},
			want:     []corev1.EnvVar{{Name: "env1", Value: "value1"}},
		},
		{
			name:     "merge",
			original: []corev1.EnvVar{{Name: "env1", Value: "value1"}},
			new:      []corev1.EnvVar{{Name: "env1", Value: "value2"}},
			want:     []corev1.EnvVar{{Name: "env1", Value: "value2"}},
		},
		{
			name:     "append and merge",
			original: []corev1.EnvVar{{Name: "env1", Value: "value1"}},
			new:      []corev1.EnvVar{{Name: "env1", Value: "value2"}, {Name: "env2", Value: "value1"}},
			want:     []corev1.EnvVar{{Name: "env1", Value: "value2"}, {Name: "env2", Value: "value1"}},
		},
		{
			name:     "complex",
			original: []corev1.EnvVar{{Name: "env1", Value: "value1"}, {Name: "env2", Value: "value1"}},
			new:      []corev1.EnvVar{{Name: "env1", Value: "value2"}, {Name: "env3", Value: "value1"}},
			want:     []corev1.EnvVar{{Name: "env1", Value: "value2"}, {Name: "env2", Value: "value1"}, {Name: "env3", Value: "value1"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, mergeEnvVars(tt.original, tt.new))
		})
	}
}

func TestSetProbes(t *testing.T) {
	tests := []struct {
		name            string
		customLiveness  corev1.Probe
		customReadiness corev1.Probe
		wantLiveness    *corev1.Probe
		wantReadiness   *corev1.Probe
	}{
		{
			name:            "No Custom probes",
			customLiveness:  corev1.Probe{},
			customReadiness: corev1.Probe{},
			wantLiveness: &corev1.Probe{
				Handler: corev1.Handler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/api/v4/system/ping",
						Port: intstr.FromInt(8065),
					},
				},
				InitialDelaySeconds: 10,
				PeriodSeconds:       10,
				FailureThreshold:    3,
			},
			wantReadiness: &corev1.Probe{
				Handler: corev1.Handler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/api/v4/system/ping",
						Port: intstr.FromInt(8065),
					},
				},
				InitialDelaySeconds: 10,
				PeriodSeconds:       5,
				FailureThreshold:    6,
			},
		},
		{
			name: "Only InitialDelaySeconds changed",
			customLiveness: corev1.Probe{
				InitialDelaySeconds: 120,
			},
			customReadiness: corev1.Probe{
				InitialDelaySeconds: 90,
			},
			wantLiveness: &corev1.Probe{
				Handler: corev1.Handler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/api/v4/system/ping",
						Port: intstr.FromInt(8065),
					},
				},
				InitialDelaySeconds: 120,
				PeriodSeconds:       10,
				FailureThreshold:    3,
			},
			wantReadiness: &corev1.Probe{
				Handler: corev1.Handler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/api/v4/system/ping",
						Port: intstr.FromInt(8065),
					},
				},
				InitialDelaySeconds: 90,
				PeriodSeconds:       5,
				FailureThreshold:    6,
			},
		},
		{
			name: "Different changes for live and readiness",
			customLiveness: corev1.Probe{
				InitialDelaySeconds: 20,
				PeriodSeconds:       20,
			},
			customReadiness: corev1.Probe{
				InitialDelaySeconds: 10,
				FailureThreshold:    10,
			},
			wantLiveness: &corev1.Probe{
				Handler: corev1.Handler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/api/v4/system/ping",
						Port: intstr.FromInt(8065),
					},
				},
				InitialDelaySeconds: 20,
				PeriodSeconds:       20,
				FailureThreshold:    3,
			},
			wantReadiness: &corev1.Probe{
				Handler: corev1.Handler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/api/v4/system/ping",
						Port: intstr.FromInt(8065),
					},
				},
				InitialDelaySeconds: 10,
				PeriodSeconds:       5,
				FailureThreshold:    10,
			},
		},
		{
			name: "Handler changed",
			customLiveness: corev1.Probe{
				Handler: corev1.Handler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/api/v4/system/pong",
						Port: intstr.FromInt(8080),
					},
				},
				InitialDelaySeconds: 120,
			},
			customReadiness: corev1.Probe{
				Handler: corev1.Handler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/api/v4/system/pingpong",
						Port: intstr.FromInt(1234),
					},
				},
			},
			wantLiveness: &corev1.Probe{
				Handler: corev1.Handler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/api/v4/system/pong",
						Port: intstr.FromInt(8080),
					},
				},
				InitialDelaySeconds: 120,
				PeriodSeconds:       10,
				FailureThreshold:    3,
			},
			wantReadiness: &corev1.Probe{
				Handler: corev1.Handler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/api/v4/system/pingpong",
						Port: intstr.FromInt(1234),
					},
				},
				InitialDelaySeconds: 10,
				PeriodSeconds:       5,
				FailureThreshold:    6,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			liveness, readiness := setProbes(tt.customLiveness, tt.customReadiness)
			require.Equal(t, tt.wantLiveness, liveness)
			require.Equal(t, tt.wantReadiness, readiness)
		})
	}
}

func i32p(i int32) *int32 {
	return &i
}
