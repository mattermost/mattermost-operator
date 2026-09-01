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
		FileStore:   FileStore{External: &ExternalFileStore{URL: "s3.example.com"}},
		Database:    Database{External: &ExternalDatabase{Secret: "db-secret"}},
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
	t.Run("a file store must be configured explicitly", func(t *testing.T) {
		t.Run("external", func(t *testing.T) {
			mm := &Mattermost{Spec: MattermostSpec{
				Ingress:   &Ingress{Enabled: false},
				FileStore: FileStore{External: &ExternalFileStore{URL: "test"}},
				Database:  Database{External: &ExternalDatabase{Secret: "db-secret"}},
			}}
			err := mm.SetDefaults()
			require.NoError(t, err)
			require.True(t, mm.Spec.FileStore.IsExternal())
		})
		t.Run("externalVolume", func(t *testing.T) {
			mm := &Mattermost{Spec: MattermostSpec{
				Ingress:   &Ingress{Enabled: false},
				FileStore: FileStore{ExternalVolume: &ExternalVolumeFileStore{VolumeClaimName: "test"}},
				Database:  Database{External: &ExternalDatabase{Secret: "db-secret"}},
			}}
			err := mm.SetDefaults()
			require.NoError(t, err)
			require.True(t, mm.Spec.FileStore.IsExternalVolume())
		})
		t.Run("local", func(t *testing.T) {
			mm := &Mattermost{Spec: MattermostSpec{
				Ingress:   &Ingress{Enabled: false},
				FileStore: FileStore{Local: &LocalFileStore{Enabled: true}},
				Database:  Database{External: &ExternalDatabase{Secret: "db-secret"}},
			}}
			err := mm.SetDefaults()
			require.NoError(t, err)
			require.True(t, mm.Spec.FileStore.IsLocal())
		})
		t.Run("empty file store is rejected", func(t *testing.T) {
			mm := &Mattermost{Spec: MattermostSpec{
				Ingress:  &Ingress{Enabled: false},
				Database: Database{External: &ExternalDatabase{Secret: "db-secret"}},
			}}
			err := mm.SetDefaults()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "a file store is required")
		})
	})

	t.Run("database validation", func(t *testing.T) {
		t.Run("file store present but database missing is rejected", func(t *testing.T) {
			mm := &Mattermost{Spec: MattermostSpec{
				Ingress:   &Ingress{Enabled: false},
				FileStore: FileStore{External: &ExternalFileStore{URL: "s3.example.com"}},
			}}
			err := mm.SetDefaults()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "a database is required")
		})
	})

	t.Run("both file store and database missing returns a combined error", func(t *testing.T) {
		mm := &Mattermost{Spec: MattermostSpec{
			Ingress: &Ingress{Enabled: false},
		}}
		err := mm.SetDefaults()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "a file store is required")
		assert.Contains(t, err.Error(), "a database is required")
	})

}

func TestValidateVolumes(t *testing.T) {
	baseSpec := func() MattermostSpec {
		return MattermostSpec{
			Ingress:   &Ingress{Enabled: false},
			FileStore: FileStore{External: &ExternalFileStore{URL: "s3.example.com"}},
			Database:  Database{External: &ExternalDatabase{Secret: "db-secret"}},
		}
	}

	t.Run("allow safe volume types", func(t *testing.T) {
		mm := &Mattermost{Spec: baseSpec()}
		mm.Spec.Volumes = []v1.Volume{
			{Name: "config", VolumeSource: v1.VolumeSource{ConfigMap: &v1.ConfigMapVolumeSource{LocalObjectReference: v1.LocalObjectReference{Name: "my-config"}}}},
			{Name: "secret", VolumeSource: v1.VolumeSource{Secret: &v1.SecretVolumeSource{SecretName: "my-secret"}}},
			{Name: "tmp", VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}},
			{Name: "data", VolumeSource: v1.VolumeSource{PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{ClaimName: "my-pvc"}}},
			{Name: "projected", VolumeSource: v1.VolumeSource{Projected: &v1.ProjectedVolumeSource{Sources: []v1.VolumeProjection{}}}},
			{Name: "downward", VolumeSource: v1.VolumeSource{DownwardAPI: &v1.DownwardAPIVolumeSource{Items: []v1.DownwardAPIVolumeFile{}}}},
			{Name: "csi", VolumeSource: v1.VolumeSource{CSI: &v1.CSIVolumeSource{Driver: "test-driver"}}},
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
		assert.Contains(t, err.Error(), "unsupported")
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
		assert.Contains(t, err.Error(), "unsupported")
	})

	t.Run("reject NFS volume", func(t *testing.T) {
		mm := &Mattermost{Spec: baseSpec()}
		mm.Spec.Volumes = []v1.Volume{
			{Name: "nfs-vol", VolumeSource: v1.VolumeSource{NFS: &v1.NFSVolumeSource{Server: "nfs.example.com", Path: "/export"}}},
		}
		err := mm.SetDefaults()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nfs-vol")
		assert.Contains(t, err.Error(), "unsupported")
	})

	t.Run("reject ISCSI volume", func(t *testing.T) {
		mm := &Mattermost{Spec: baseSpec()}
		mm.Spec.Volumes = []v1.Volume{
			{Name: "iscsi-vol", VolumeSource: v1.VolumeSource{ISCSI: &v1.ISCSIVolumeSource{TargetPortal: "10.0.0.1", IQN: "iqn.test", Lun: 0}}},
		}
		err := mm.SetDefaults()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "iscsi-vol")
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
			// This table is about Ingress, but SetDefaults now requires a file store.
			mm.Spec.FileStore = FileStore{External: &ExternalFileStore{URL: "s3.example.com"}}
			mm.Spec.Database = Database{External: &ExternalDatabase{Secret: "db-secret"}}
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

// validHTTPRoute returns an HTTPRouteSpec that passes SetDefaults validation.
func validHTTPRoute() *HTTPRouteSpec {
	return &HTTPRouteSpec{
		Enabled:    true,
		Host:       "mm.example.com",
		GatewayRef: GatewayReference{Name: "shared-gateway"},
	}
}

// validMMBase returns the minimum FileStore+Database config required by SetDefaults.
func validMMBase() MattermostSpec {
	return MattermostSpec{
		FileStore: FileStore{External: &ExternalFileStore{URL: "s3.example.com"}},
		Database:  Database{External: &ExternalDatabase{Secret: "db-secret"}},
	}
}

func TestMattermost_SetDefaults_HTTPRoute(t *testing.T) {
	t.Run("HTTPRoute only, no ingress section, is accepted", func(t *testing.T) {
		spec := validMMBase()
		spec.HTTPRoute = validHTTPRoute()
		mm := &Mattermost{Spec: spec}
		require.NoError(t, mm.SetDefaults())
	})

	t.Run("require host when enabled", func(t *testing.T) {
		spec := validMMBase()
		spec.HTTPRoute = &HTTPRouteSpec{
			Enabled:    true,
			GatewayRef: GatewayReference{Name: "shared-gateway"},
		}
		err := (&Mattermost{Spec: spec}).SetDefaults()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "httpRoute.host")
	})

	t.Run("require gatewayRef.name when enabled", func(t *testing.T) {
		spec := validMMBase()
		spec.HTTPRoute = &HTTPRouteSpec{
			Enabled: true,
			Host:    "mm.example.com",
		}
		err := (&Mattermost{Spec: spec}).SetDefaults()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "httpRoute.gatewayRef.name")
	})

	t.Run("no validation when disabled", func(t *testing.T) {
		// An operator removing an HTTPRoute must be able to flip enabled to false
		// without still supplying a host and Gateway reference.
		spec := validMMBase()
		spec.Ingress = &Ingress{Enabled: false}
		spec.HTTPRoute = &HTTPRouteSpec{Enabled: false}
		require.NoError(t, (&Mattermost{Spec: spec}).SetDefaults())
	})

	t.Run("ingress still validated when explicitly enabled alongside HTTPRoute", func(t *testing.T) {
		spec := validMMBase()
		spec.Ingress = &Ingress{Enabled: true}
		spec.HTTPRoute = validHTTPRoute()
		err := (&Mattermost{Spec: spec}).SetDefaults()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ingress.host")
	})
}

func TestMattermost_IngressEnabled_HTTPRouteInteraction(t *testing.T) {
	for _, testCase := range []struct {
		description string
		mmSpec      MattermostSpec
		enabled     bool
	}{
		{
			description: "no ingress section and no HTTPRoute keeps legacy default",
			mmSpec:      MattermostSpec{},
			enabled:     true,
		},
		{
			description: "HTTPRoute opts out of the implicit ingress default",
			mmSpec:      MattermostSpec{HTTPRoute: validHTTPRoute()},
			enabled:     false,
		},
		{
			description: "deprecated IngressName is preserved for side-by-side migration",
			mmSpec: MattermostSpec{
				IngressName: "legacy.example.com",
				HTTPRoute:   validHTTPRoute(),
			},
			enabled: true,
		},
		{
			description: "explicit ingress enabled wins over HTTPRoute opt out",
			mmSpec: MattermostSpec{
				Ingress:   &Ingress{Enabled: true, Host: "mm.example.com"},
				HTTPRoute: validHTTPRoute(),
			},
			enabled: true,
		},
		{
			description: "explicit ingress disabled stays disabled",
			mmSpec: MattermostSpec{
				Ingress:   &Ingress{Enabled: false},
				HTTPRoute: validHTTPRoute(),
			},
			enabled: false,
		},
		{
			description: "disabled HTTPRoute does not opt out of the ingress default",
			mmSpec: MattermostSpec{
				IngressName: "legacy.example.com",
				HTTPRoute:   &HTTPRouteSpec{Enabled: false},
			},
			enabled: true,
		},
	} {
		t.Run(testCase.description, func(t *testing.T) {
			mm := &Mattermost{Spec: testCase.mmSpec}
			assert.Equal(t, testCase.enabled, mm.IngressEnabled())
		})
	}
}

func TestMattermost_HTTPRouteAccessors(t *testing.T) {
	t.Run("absent section", func(t *testing.T) {
		mm := &Mattermost{}
		assert.False(t, mm.HTTPRouteEnabled())
		assert.Equal(t, "", mm.GetHTTPRouteHost())
		assert.Equal(t, []string{}, mm.GetHTTPRouteHostNames())
		assert.Nil(t, mm.GetHTTPRouteAnnotations())
	})

	t.Run("present but disabled", func(t *testing.T) {
		mm := &Mattermost{Spec: MattermostSpec{HTTPRoute: &HTTPRouteSpec{
			Enabled: false,
			Host:    "mm.example.com",
		}}}
		assert.False(t, mm.HTTPRouteEnabled())
		// Accessors report configuration regardless of enablement.
		assert.Equal(t, "mm.example.com", mm.GetHTTPRouteHost())
	})

	t.Run("enabled with annotations", func(t *testing.T) {
		mm := &Mattermost{Spec: MattermostSpec{HTTPRoute: &HTTPRouteSpec{
			Enabled:     true,
			Host:        "mm.example.com",
			Annotations: map[string]string{"team": "sre"},
		}}}
		assert.True(t, mm.HTTPRouteEnabled())
		assert.Equal(t, "mm.example.com", mm.GetHTTPRouteHost())
		assert.Equal(t, map[string]string{"team": "sre"}, mm.GetHTTPRouteAnnotations())
	})
}

func TestMattermost_GetHTTPRouteHostNames(t *testing.T) {
	for _, testCase := range []struct {
		description   string
		mmSpec        MattermostSpec
		expectedHosts []string
	}{
		{
			description:   "no HTTPRoute section",
			mmSpec:        MattermostSpec{},
			expectedHosts: []string{},
		},
		{
			description: "no primary host yields no hosts even if extras are set",
			mmSpec: MattermostSpec{HTTPRoute: &HTTPRouteSpec{
				Enabled: true,
				Hosts:   []IngressHost{{HostName: "extra.example.com"}},
			}},
			expectedHosts: []string{},
		},
		{
			description: "only primary host",
			mmSpec: MattermostSpec{HTTPRoute: &HTTPRouteSpec{
				Enabled: true,
				Host:    "primary.example.com",
			}},
			expectedHosts: []string{"primary.example.com"},
		},
		{
			description: "multiple hosts retain order and skip duplicates",
			mmSpec: MattermostSpec{HTTPRoute: &HTTPRouteSpec{
				Enabled: true,
				Host:    "primary.example.com",
				Hosts: []IngressHost{
					{HostName: "b.example.com"},
					{HostName: "a.example.com"},
					{HostName: "b.example.com"},
					{HostName: "primary.example.com"},
				},
			}},
			expectedHosts: []string{"primary.example.com", "b.example.com", "a.example.com"},
		},
	} {
		t.Run(testCase.description, func(t *testing.T) {
			mm := &Mattermost{Spec: testCase.mmSpec}
			assert.Equal(t, testCase.expectedHosts, mm.GetHTTPRouteHostNames())
		})
	}
}

func TestMattermost_GetSiteURLHost(t *testing.T) {
	for _, testCase := range []struct {
		description  string
		mmSpec       MattermostSpec
		expectedHost string
	}{
		// The following cases predate HTTPRoute support. Before GetSiteURLHost
		// existed the deployment always used GetIngressHost(), so these pin that
		// behavior against regressions.
		{
			description:  "legacy IngressName",
			mmSpec:       MattermostSpec{IngressName: "legacy.example.com"},
			expectedHost: "legacy.example.com",
		},
		{
			description:  "ingress enabled",
			mmSpec:       MattermostSpec{Ingress: &Ingress{Enabled: true, Host: "mm.example.com"}},
			expectedHost: "mm.example.com",
		},
		{
			description: "ALB with no ingress section falls back to legacy host",
			mmSpec: MattermostSpec{
				AWSLoadBalancerController: &AWSLoadBalancerController{
					Enabled: true,
					Hosts:   []IngressHost{{HostName: "alb.example.com"}},
				},
			},
			expectedHost: "",
		},

		// HTTPRoute cases.
		{
			description:  "HTTPRoute only",
			mmSpec:       MattermostSpec{HTTPRoute: validHTTPRoute()},
			expectedHost: "mm.example.com",
		},
		{
			description: "ingress wins over HTTPRoute when both enabled",
			mmSpec: MattermostSpec{
				Ingress:   &Ingress{Enabled: true, Host: "ingress.example.com"},
				HTTPRoute: validHTTPRoute(),
			},
			expectedHost: "ingress.example.com",
		},
		{
			description: "HTTPRoute used when ingress explicitly disabled",
			mmSpec: MattermostSpec{
				Ingress:   &Ingress{Enabled: false, Host: "ignored.example.com"},
				HTTPRoute: validHTTPRoute(),
			},
			expectedHost: "mm.example.com",
		},
		{
			// Pins the pre-HTTPRoute behavior: the site URL comes from ingress.host
			// even for ALB installations. Changing this would alter SITEURL, and with
			// it force a rollout, on existing installations.
			description: "ALB with ingress disabled still uses the ingress host",
			mmSpec: MattermostSpec{
				Ingress: &Ingress{Enabled: false, Host: "mm.example.com"},
				AWSLoadBalancerController: &AWSLoadBalancerController{
					Enabled: true,
					Hosts:   []IngressHost{{HostName: "alb.example.com"}},
				},
			},
			expectedHost: "mm.example.com",
		},
	} {
		t.Run(testCase.description, func(t *testing.T) {
			mm := &Mattermost{Spec: testCase.mmSpec}
			assert.Equal(t, testCase.expectedHost, mm.GetSiteURLHost())
		})
	}
}

// TestGetSiteURLHost_IdenticalToLegacyWithoutHTTPRoute proves the invariant that
// makes swapping GetIngressHost for GetSiteURLHost in the deployment safe: across
// every combination of the routing fields that existed before HTTPRoute, the two
// return the same value. If this holds, no existing installation can see its
// MM_SERVICESETTINGS_SITEURL change, and so none can be forced into a rollout.
func TestGetSiteURLHost_IdenticalToLegacyWithoutHTTPRoute(t *testing.T) {
	ingresses := []*Ingress{
		nil,
		{Enabled: true, Host: "mm.example.com"},
		{Enabled: true},
		{Enabled: false},
		{Enabled: false, Host: "mm.example.com"},
	}
	albs := []*AWSLoadBalancerController{
		nil,
		{Enabled: false},
		{Enabled: true, Hosts: []IngressHost{{HostName: "alb.example.com"}}},
	}
	legacyNames := []string{"", "legacy.example.com"}
	serviceLBs := []bool{false, true}

	combinations := 0
	for _, ingress := range ingresses {
		for _, alb := range albs {
			for _, legacyName := range legacyNames {
				for _, serviceLB := range serviceLBs {
					mm := &Mattermost{Spec: MattermostSpec{
						Ingress:                   ingress,
						AWSLoadBalancerController: alb,
						IngressName:               legacyName,
						UseServiceLoadBalancer:    serviceLB,
					}}

					require.False(t, mm.HTTPRouteEnabled(), "fixture must not enable HTTPRoute")
					assert.Equal(t, mm.GetIngressHost(), mm.GetSiteURLHost(),
						"ingress=%+v alb=%+v ingressName=%q serviceLB=%v",
						ingress, alb, legacyName, serviceLB)
					combinations++
				}
			}
		}
	}

	assert.Equal(t, len(ingresses)*len(albs)*len(legacyNames)*len(serviceLBs), combinations)
}
