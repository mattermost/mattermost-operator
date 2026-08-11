//go:build envtest

// This suite needs a real apiserver+etcd (envtest). It is excluded from the
// default `go test` run and only executes via `make test-envtest`, which sets
// KUBEBUILDER_ASSETS and passes -tags envtest.
package envtest_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
)

// TestCallsMetricsCELValidation exercises the CRD's x-kubernetes-validations
// (CEL) against a real apiserver: callsMetrics.enabled=true must require a
// non-empty rtcdServiceSelector. envtest runs a real kube-apiserver+etcd from
// downloaded binaries (no Docker), so CEL actually executes.
func TestCallsMetricsCELValidation(t *testing.T) {
	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	require.NoError(t, err)
	t.Cleanup(func() { _ = testEnv.Stop() })

	require.NoError(t, mmv1beta.AddToScheme(scheme.Scheme))
	c, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("rejects callsMetrics enabled without rtcdServiceSelector", func(t *testing.T) {
		mm := &mmv1beta.Mattermost{
			ObjectMeta: metav1.ObjectMeta{Name: "bad-calls", Namespace: "default"},
			Spec: mmv1beta.MattermostSpec{
				Monitoring: &mmv1beta.Monitoring{
					CallsMetrics: &mmv1beta.CallsMetrics{Enabled: true},
				},
			},
		}
		err := c.Create(ctx, mm)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "rtcdServiceSelector is required")
	})

	t.Run("accepts callsMetrics enabled with rtcdServiceSelector", func(t *testing.T) {
		mm := &mmv1beta.Mattermost{
			ObjectMeta: metav1.ObjectMeta{Name: "good-calls", Namespace: "default"},
			Spec: mmv1beta.MattermostSpec{
				Monitoring: &mmv1beta.Monitoring{
					CallsMetrics: &mmv1beta.CallsMetrics{
						Enabled:             true,
						RtcdServiceSelector: map[string]string{"app": "rtcd"},
					},
				},
			},
		}
		require.NoError(t, c.Create(ctx, mm))
	})

	t.Run("accepts callsMetrics disabled without selector", func(t *testing.T) {
		mm := &mmv1beta.Mattermost{
			ObjectMeta: metav1.ObjectMeta{Name: "off-calls", Namespace: "default"},
			Spec: mmv1beta.MattermostSpec{
				Monitoring: &mmv1beta.Monitoring{
					CallsMetrics: &mmv1beta.CallsMetrics{Enabled: false},
				},
			},
		}
		require.NoError(t, c.Create(ctx, mm))
	})
}
