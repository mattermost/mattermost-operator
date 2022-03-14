package mattermost

import (
	"testing"

	"github.com/stretchr/testify/assert"

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

func TestDetermineMaxBodySize(t *testing.T) {
	defaultSize := "1000"

	for _, testCase := range []struct {
		description        string
		ingressAnnotations map[string]string
		expectedSize       string
	}{
		{
			description:        "use default size when no annotation",
			ingressAnnotations: nil,
			expectedSize:       defaultSize,
		},
		{
			description: "use default size when cannot parse size",
			ingressAnnotations: map[string]string{
				"nginx.ingress.kubernetes.io/proxy-body-size": "nan",
			},
			expectedSize: defaultSize,
		},
		{
			description: "use default size when unit not recognized",
			ingressAnnotations: map[string]string{
				"nginx.ingress.kubernetes.io/proxy-body-size": "800K",
			},
			expectedSize: defaultSize,
		},
		{
			description: "use GB size when lowercase 'g'",
			ingressAnnotations: map[string]string{
				"nginx.ingress.kubernetes.io/proxy-body-size": "1g",
			},
			expectedSize: "1048576000",
		},
		{
			description: "use GB size when capital 'G",
			ingressAnnotations: map[string]string{
				"nginx.ingress.kubernetes.io/proxy-body-size": "1G",
			},
			expectedSize: "1048576000",
		},
		{
			description: "use MB size when lowercase 'm",
			ingressAnnotations: map[string]string{
				"nginx.ingress.kubernetes.io/proxy-body-size": "10m",
			},
			expectedSize: "10485760",
		},
		{
			description: "use MB size when capital 'M",
			ingressAnnotations: map[string]string{
				"nginx.ingress.kubernetes.io/proxy-body-size": "10M",
			},
			expectedSize: "10485760",
		},
	} {
		t.Run(testCase.description, func(t *testing.T) {
			size := determineMaxBodySize(testCase.ingressAnnotations, defaultSize)
			assert.Equal(t, testCase.expectedSize, size)
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
				ProbeHandler: corev1.ProbeHandler{
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
				ProbeHandler: corev1.ProbeHandler{
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
				ProbeHandler: corev1.ProbeHandler{
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
				ProbeHandler: corev1.ProbeHandler{
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
				ProbeHandler: corev1.ProbeHandler{
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
				ProbeHandler: corev1.ProbeHandler{
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
			name: "ProbeHandler changed",
			customLiveness: corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/api/v4/system/pong",
						Port: intstr.FromInt(8080),
					},
				},
				InitialDelaySeconds: 120,
			},
			customReadiness: corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/api/v4/system/pingpong",
						Port: intstr.FromInt(1234),
					},
				},
			},
			wantLiveness: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
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
				ProbeHandler: corev1.ProbeHandler{
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
