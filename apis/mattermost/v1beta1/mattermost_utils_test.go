package v1beta1

import (
	"testing"

	pkgUtils "github.com/mattermost/mattermost-operator/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
