//go:build envtest

package mattermost

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	"github.com/mattermost/mattermost-operator/pkg/mattermost"
	"github.com/mattermost/mattermost-operator/pkg/resources"
)

// TestMonitoringReconcileCreateAndCleanup drives the monitoring reconcile against
// a real apiserver (envtest, no Docker) to validate the dedicated metrics Service
// (#2) and the create/delete lifecycle (#1). The Prometheus Operator CRDs are not
// installed, so ServiceMonitor/PrometheusRule creation degrades to a graceful
// skip — this test focuses on the core-type resources (Service + dashboard
// ConfigMaps) that envtest supports out of the box.
//
// Run with: KUBEBUILDER_ASSETS=... go test -tags envtest ./controllers/mattermost/mattermost/ -run TestMonitoringReconcile
func TestMonitoringReconcileCreateAndCleanup(t *testing.T) {
	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := testEnv.Start()
	require.NoError(t, err)
	t.Cleanup(func() { _ = testEnv.Stop() })

	require.NoError(t, mmv1beta.AddToScheme(scheme.Scheme))
	// Mirror main.go: registering the Prometheus Operator scheme is client-side
	// only. The CRDs are NOT installed in this envtest, so ServiceMonitor/
	// PrometheusRule creation degrades to a graceful IsNoMatchError skip.
	require.NoError(t, monitoringv1.AddToScheme(scheme.Scheme))
	c, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	require.NoError(t, err)

	ctx := context.Background()
	log := logr.Discard()

	r := &MattermostReconciler{
		Client:    c,
		Scheme:    scheme.Scheme,
		Resources: resources.NewResourceHelper(c, scheme.Scheme),
		// Recorder intentionally nil — warnMonitoringWithoutLicense guards on it.
	}

	enabled := &mmv1beta.Mattermost{
		ObjectMeta: metav1.ObjectMeta{Name: "mm1", Namespace: "default"},
		Spec: mmv1beta.MattermostSpec{
			LicenseSecret: "mattermost-license",
			Monitoring: &mmv1beta.Monitoring{
				ServiceMonitor:   &mmv1beta.ServiceMonitor{Enabled: true},
				GrafanaDashboard: &mmv1beta.GrafanaDashboard{Enabled: true},
			},
		},
	}
	require.NoError(t, c.Create(ctx, enabled))
	// Re-fetch so owner references resolve (UID populated).
	require.NoError(t, c.Get(ctx, types.NamespacedName{Name: "mm1", Namespace: "default"}, enabled))

	// --- Reconcile with monitoring enabled ---
	require.NoError(t, r.checkMattermostMonitoring(enabled, log))

	// #2: dedicated metrics Service exists and is headless on port 8067.
	svc := &corev1.Service{}
	require.NoError(t, c.Get(ctx, types.NamespacedName{Name: "mm1-metrics", Namespace: "default"}, svc))
	assert.Equal(t, corev1.ClusterIPNone, svc.Spec.ClusterIP)
	require.Len(t, svc.Spec.Ports, 1)
	assert.EqualValues(t, 8067, svc.Spec.Ports[0].Port)

	// Branch 2: one dashboard ConfigMap per embedded dashboard.
	cms := &corev1.ConfigMapList{}
	require.NoError(t, c.List(ctx, cms,
		client.InNamespace("default"),
		client.MatchingLabels(mattermost.DashboardConfigMapSelector(enabled)),
	))
	assert.GreaterOrEqual(t, len(cms.Items), 9, "expected one ConfigMap per embedded dashboard")

	// --- Flip everything off and reconcile again (cleanup-on-disable) ---
	require.NoError(t, c.Get(ctx, types.NamespacedName{Name: "mm1", Namespace: "default"}, enabled))
	enabled.Spec.Monitoring.ServiceMonitor.Enabled = false
	enabled.Spec.Monitoring.GrafanaDashboard.Enabled = false
	require.NoError(t, c.Update(ctx, enabled))

	require.NoError(t, r.checkMattermostMonitoring(enabled, log))

	// #1: metrics Service deleted.
	err = c.Get(ctx, types.NamespacedName{Name: "mm1-metrics", Namespace: "default"}, &corev1.Service{})
	assert.True(t, apierrors.IsNotFound(err), "metrics Service should be deleted on disable")

	// #1: dashboard ConfigMaps deleted.
	cms = &corev1.ConfigMapList{}
	require.NoError(t, c.List(ctx, cms,
		client.InNamespace("default"),
		client.MatchingLabels(mattermost.DashboardConfigMapSelector(enabled)),
	))
	assert.Empty(t, cms.Items, "dashboard ConfigMaps should be deleted on disable")
}
