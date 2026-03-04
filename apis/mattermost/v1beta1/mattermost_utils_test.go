package v1beta1

import (
	"testing"

	pkgUtils "github.com/mattermost/mattermost-operator/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
)

func TestMattermost_SetDefaults(t *testing.T) {
	mm := &Mattermost{Spec: MattermostSpec{
		IngressName: "",
	}}

	t.Run("return error when ingress enabled but host not set", func(t *testing.T) {
		err := mm.SetDefaults()
		require.Error(t, err)
	})
	t.Run("allow empty host if ingress disabled", func(t *testing.T) {
		mm.Spec.Ingress = &Ingress{Enabled: false}
		err := mm.SetDefaults()
		require.NoError(t, err)
	})
	t.Run("minio only set if other filestores types are not", func(t *testing.T) {
		t.Run("external", func(t *testing.T) {
			mm := &Mattermost{Spec: MattermostSpec{
				Ingress:   &Ingress{Enabled: false},
				FileStore: FileStore{External: &ExternalFileStore{URL: "test"}},
			}}
			err := mm.SetDefaults()
			require.NoError(t, err)
			require.True(t, mm.Spec.FileStore.IsExternal())
			assert.Nil(t, mm.Spec.FileStore.OperatorManaged)
		})
		t.Run("externalVolume", func(t *testing.T) {
			mm := &Mattermost{Spec: MattermostSpec{
				Ingress:   &Ingress{Enabled: false},
				FileStore: FileStore{ExternalVolume: &ExternalVolumeFileStore{VolumeClaimName: "test"}},
			}}
			err := mm.SetDefaults()
			require.NoError(t, err)
			require.True(t, mm.Spec.FileStore.IsExternalVolume())
			assert.Nil(t, mm.Spec.FileStore.OperatorManaged)
		})
		t.Run("local", func(t *testing.T) {
			mm := &Mattermost{Spec: MattermostSpec{
				Ingress:   &Ingress{Enabled: false},
				FileStore: FileStore{Local: &LocalFileStore{Enabled: true}},
			}}
			err := mm.SetDefaults()
			require.NoError(t, err)
			require.True(t, mm.Spec.FileStore.IsLocal())
			assert.Nil(t, mm.Spec.FileStore.OperatorManaged)
		})
		t.Run("filestore empty", func(t *testing.T) {
			mm := &Mattermost{Spec: MattermostSpec{
				Ingress: &Ingress{Enabled: false},
			}}
			err := mm.SetDefaults()
			require.NoError(t, err)
			assert.NotNil(t, mm.Spec.FileStore.OperatorManaged)
		})
	})

}

func TestValidateVolumes(t *testing.T) {
	baseSpec := func() MattermostSpec {
		return MattermostSpec{
			Ingress: &Ingress{Enabled: false},
		}
	}

	t.Run("allow safe volume types", func(t *testing.T) {
		mm := &Mattermost{Spec: baseSpec()}
		mm.Spec.Volumes = []v1.Volume{
			{Name: "config", VolumeSource: v1.VolumeSource{ConfigMap: &v1.ConfigMapVolumeSource{LocalObjectReference: v1.LocalObjectReference{Name: "my-config"}}}},
			{Name: "secret", VolumeSource: v1.VolumeSource{Secret: &v1.SecretVolumeSource{SecretName: "my-secret"}}},
			{Name: "tmp", VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}},
			{Name: "data", VolumeSource: v1.VolumeSource{PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{ClaimName: "my-pvc"}}},
		}
		err := mm.SetDefaults()
		require.NoError(t, err)
	})

	t.Run("reject HostPath volume", func(t *testing.T) {
		mm := &Mattermost{Spec: baseSpec()}
		mm.Spec.Volumes = []v1.Volume{
			{Name: "host-mount", VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: "/etc"}}},
		}
		err := mm.SetDefaults()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "HostPath")
		assert.Contains(t, err.Error(), "host-mount")
	})

	t.Run("reject HostPath mixed with safe volumes", func(t *testing.T) {
		mm := &Mattermost{Spec: baseSpec()}
		mm.Spec.Volumes = []v1.Volume{
			{Name: "config", VolumeSource: v1.VolumeSource{ConfigMap: &v1.ConfigMapVolumeSource{LocalObjectReference: v1.LocalObjectReference{Name: "my-config"}}}},
			{Name: "bad-vol", VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: "/var/run/docker.sock"}}},
		}
		err := mm.SetDefaults()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "bad-vol")
	})

	t.Run("allow empty volumes list", func(t *testing.T) {
		mm := &Mattermost{Spec: baseSpec()}
		err := mm.SetDefaults()
		require.NoError(t, err)
	})
}

func TestMattermost_ImageTagWarnings(t *testing.T) {
	for _, tc := range []struct {
		description    string
		version        string
		expectWarnings bool
	}{
		{
			description:    "no warning for specific version tag",
			version:        "10.8.1",
			expectWarnings: false,
		},
		{
			description:    "no warning for digest reference",
			version:        "sha256:dd15a51ac7dafd213744d1ef23394e7532f71a90f477c969b94600e46da5a0cf",
			expectWarnings: false,
		},
		{
			description:    "warn for latest tag",
			version:        "latest",
			expectWarnings: true,
		},
		{
			description:    "warn for Latest tag (case insensitive)",
			version:        "Latest",
			expectWarnings: true,
		},
		{
			description:    "warn for master tag",
			version:        "master",
			expectWarnings: true,
		},
		{
			description:    "warn for main tag",
			version:        "main",
			expectWarnings: true,
		},
		{
			description:    "warn for nightly tag",
			version:        "nightly",
			expectWarnings: true,
		},
		{
			description:    "warn for edge tag",
			version:        "edge",
			expectWarnings: true,
		},
		{
			description:    "no warning for empty version (uses default)",
			version:        "",
			expectWarnings: false,
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			mm := &Mattermost{Spec: MattermostSpec{
				Version: tc.version,
			}}
			warnings := mm.ImageTagWarnings()
			if tc.expectWarnings {
				assert.NotEmpty(t, warnings)
				assert.Contains(t, warnings[0], "mutable tag")
			} else {
				assert.Empty(t, warnings)
			}
		})
	}
}

func TestMattermost_IngressAccessors(t *testing.T) {
	for _, testCase := range []struct {
		description  string
		mmSpec       MattermostSpec
		enabled      bool
		host         string
		annotations  map[string]string
		tlsSecret    string
		ingressClass *string
	}{
		{
			description: "respect only old values",
			mmSpec: MattermostSpec{
				IngressName:        "test-mm.com",
				IngressAnnotations: map[string]string{"test": "val"},
				UseIngressTLS:      true,
			},
			enabled:      true,
			host:         "test-mm.com",
			annotations:  map[string]string{"test": "val"},
			tlsSecret:    "test-mm-com-tls-cert",
			ingressClass: nil,
		},
		{
			description: "respect only new values - enabled",
			mmSpec: MattermostSpec{
				Ingress: &Ingress{
					Enabled:      true,
					Host:         "test-mm.com",
					Annotations:  map[string]string{"test2": "val2"},
					TLSSecret:    "my-tls-secret",
					IngressClass: pkgUtils.NewString("custom-nginx"),
				},
			},
			enabled:      true,
			host:         "test-mm.com",
			annotations:  map[string]string{"test2": "val2"},
			tlsSecret:    "my-tls-secret",
			ingressClass: pkgUtils.NewString("custom-nginx"),
		},
		{
			description: "respect only new values - disabled",
			mmSpec: MattermostSpec{
				Ingress: &Ingress{
					Enabled: false,
				},
			},
			enabled:      false,
			host:         "",
			tlsSecret:    "",
			ingressClass: nil,
		},
		{
			description: "prefer new values over old",
			mmSpec: MattermostSpec{
				IngressName:        "old-test-mm.com",
				IngressAnnotations: map[string]string{"test": "val"},
				UseIngressTLS:      true,
				Ingress: &Ingress{
					Enabled:     true,
					Host:        "test-mm.com",
					Annotations: map[string]string{"test2": "val2"},
					TLSSecret:   "",
				},
			},
			enabled:      true,
			host:         "test-mm.com",
			annotations:  map[string]string{"test2": "val2"},
			tlsSecret:    "",
			ingressClass: nil,
		},
	} {
		t.Run(testCase.description, func(t *testing.T) {
			mm := &Mattermost{Spec: testCase.mmSpec}
			err := mm.SetDefaults()
			require.NoError(t, err)

			assert.Equal(t, testCase.enabled, mm.IngressEnabled())
			assert.Equal(t, testCase.host, mm.GetIngressHost())
			assert.Equal(t, testCase.annotations, mm.GetIngresAnnotations())
			assert.Equal(t, testCase.tlsSecret, mm.GetIngressTLSSecret())
			assert.Equal(t, testCase.ingressClass, mm.GetIngressClass())
		})
	}
}

func TestMattermost_GetIngressHostNames(t *testing.T) {
	for _, testCase := range []struct {
		description   string
		mmSpec        MattermostSpec
		expectedHosts []string
	}{
		{
			description: "deprecated host",
			mmSpec: MattermostSpec{
				IngressName: "primary-host",
			},
			expectedHosts: []string{"primary-host"},
		},
		{
			description: "ingress disabled",
			mmSpec: MattermostSpec{
				Ingress: &Ingress{
					Enabled: false,
				},
			},
			expectedHosts: []string{},
		},
		{
			description: "only primary host",
			mmSpec: MattermostSpec{
				Ingress: &Ingress{
					Enabled: true,
					Host:    "primary-host",
				},
			},
			expectedHosts: []string{"primary-host"},
		},
		{
			description: "multiple hosts, skip duplicates",
			mmSpec: MattermostSpec{
				Ingress: &Ingress{
					Enabled: true,
					Host:    "primary-host",
					Hosts: []IngressHost{
						{HostName: "test-1"},
						{HostName: "test-1"},
						{HostName: "test-2"},
						{HostName: "test-2"},
						{HostName: "test-3"},
						{HostName: "test-3"},
					},
				},
			},
			expectedHosts: []string{"primary-host", "test-1", "test-2", "test-3"},
		},
	} {
		t.Run(testCase.description, func(t *testing.T) {
			mm := &Mattermost{
				Spec: testCase.mmSpec,
			}

			assert.Equal(t, testCase.expectedHosts, mm.GetIngressHostNames())
		})
	}
}
