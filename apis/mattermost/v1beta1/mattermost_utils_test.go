package v1beta1

import (
	"testing"

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
		description string
		mmSpec      MattermostSpec
		enabled     bool
		host        string
		annotations map[string]string
		tlsSecret   string
	}{
		{
			description: "respect only old values",
			mmSpec: MattermostSpec{
				IngressName:        "test-mm.com",
				IngressAnnotations: map[string]string{"test": "val"},
				UseIngressTLS:      true,
			},
			enabled:     true,
			host:        "test-mm.com",
			annotations: map[string]string{"test": "val"},
			tlsSecret:   "test-mm-com-tls-cert",
		},
		{
			description: "respect only new values - enabled",
			mmSpec: MattermostSpec{
				Ingress: &Ingress{
					Enabled:     true,
					Host:        "test-mm.com",
					Annotations: map[string]string{"test2": "val2"},
					TLSSecret:   "my-tls-secret",
				},
			},
			enabled:     true,
			host:        "test-mm.com",
			annotations: map[string]string{"test2": "val2"},
			tlsSecret:   "my-tls-secret",
		},
		{
			description: "respect only new values - disabled",
			mmSpec: MattermostSpec{
				Ingress: &Ingress{
					Enabled: false,
				},
			},
			enabled:   false,
			host:      "",
			tlsSecret: "",
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
			enabled:     true,
			host:        "test-mm.com",
			annotations: map[string]string{"test2": "val2"},
			tlsSecret:   "",
		},
	} {
		t.Run(testCase.description, func(t *testing.T) {
			mm := &Mattermost{Spec: testCase.mmSpec}

			assert.Equal(t, testCase.enabled, mm.IngressEnabled())
			assert.Equal(t, testCase.host, mm.GetIngressHost())
			assert.Equal(t, testCase.annotations, mm.GetIngresAnnotations())
			assert.Equal(t, testCase.tlsSecret, mm.GetIngressTLSSecret())
		})
	}
}
