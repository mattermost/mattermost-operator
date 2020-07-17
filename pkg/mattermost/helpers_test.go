package mattermost

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestMergeStringMaps(t *testing.T) {
	tests := []struct {
		name     string
		original map[string]string
		new      map[string]string
		want     map[string]string
	}{
		{
			name:     "nil",
			original: nil,
			new:      nil,
			want:     map[string]string{},
		},
		{
			name:     "empty",
			original: map[string]string{},
			new:      map[string]string{},
			want:     map[string]string{},
		},
		{
			name:     "append",
			original: map[string]string{},
			new:      map[string]string{"key1": "value1"},
			want:     map[string]string{"key1": "value1"},
		},
		{
			name:     "merge",
			original: map[string]string{"key1": "value1"},
			new:      map[string]string{"key1": "value2"},
			want:     map[string]string{"key1": "value2"},
		},
		{
			name:     "append and merge",
			original: map[string]string{"key1": "value1"},
			new:      map[string]string{"key1": "value2", "key2": "value1"},
			want:     map[string]string{"key1": "value2", "key2": "value1"},
		},
		{
			name:     "complex",
			original: map[string]string{"key1": "value1", "key3": "value1"},
			new:      map[string]string{"key1": "value2", "key2": "value1"},
			want:     map[string]string{"key1": "value2", "key2": "value1", "key3": "value1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, mergeStringMaps(tt.original, tt.new))
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
		customStartup   corev1.Probe
		customReadiness corev1.Probe
		wantLiveness    *corev1.Probe
		wantStartup     *corev1.Probe
		wantReadiness   *corev1.Probe
	}{
		{
			name:            "No Custom probes",
			customLiveness:  corev1.Probe{},
			customStartup:   corev1.Probe{},
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
			wantStartup: &corev1.Probe{
				Handler: corev1.Handler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/api/v4/system/ping",
						Port: intstr.FromInt(8065),
					},
				},
				InitialDelaySeconds: 1,
				PeriodSeconds:       10,
				FailureThreshold:    60,
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
			customStartup: corev1.Probe{
				InitialDelaySeconds: 1,
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
			wantStartup: &corev1.Probe{
				Handler: corev1.Handler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/api/v4/system/ping",
						Port: intstr.FromInt(8065),
					},
				},
				InitialDelaySeconds: 1,
				PeriodSeconds:       10,
				FailureThreshold:    60,
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
			customStartup: corev1.Probe{
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
			wantStartup: &corev1.Probe{
				Handler: corev1.Handler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/api/v4/system/ping",
						Port: intstr.FromInt(8065),
					},
				},
				InitialDelaySeconds: 20,
				PeriodSeconds:       20,
				FailureThreshold:    60,
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
			customStartup: corev1.Probe{
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
			wantStartup: &corev1.Probe{
				Handler: corev1.Handler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/api/v4/system/pong",
						Port: intstr.FromInt(8080),
					},
				},
				InitialDelaySeconds: 120,
				PeriodSeconds:       10,
				FailureThreshold:    60,
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
			liveness, startUp, readiness := setProbes(tt.customLiveness, tt.customStartup, tt.customReadiness)
			require.Equal(t, tt.wantLiveness, liveness)
			require.Equal(t, tt.wantStartup, startUp)
			require.Equal(t, tt.wantReadiness, readiness)
		})
	}
}
