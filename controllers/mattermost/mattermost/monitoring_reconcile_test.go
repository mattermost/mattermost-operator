package mattermost

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	"github.com/mattermost/mattermost-operator/pkg/resources"
)

// TestCheckMattermostMonitoringReconcile drives the monitoring reconcile against a
// fake client to cover the create and cleanup-on-disable paths that the generator
// unit tests don't: the dedicated metrics Service, the ServiceMonitor, and the
// PrometheusRule are created when enabled and deleted when disabled. Uses a fake
// client (Prometheus Operator types registered in the scheme, as in main.go), so
// it runs in the standard `go test` suite without envtest.
func TestCheckMattermostMonitoringReconcile(t *testing.T) {
	s := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(s))
	require.NoError(t, mmv1beta.AddToScheme(s))
	require.NoError(t, monitoringv1.AddToScheme(s))

	mm := &mmv1beta.Mattermost{
		ObjectMeta: metav1.ObjectMeta{Name: "mm", Namespace: "test", UID: "test-uid"},
		Spec: mmv1beta.MattermostSpec{
			LicenseSecret: "license",
			Monitoring: &mmv1beta.Monitoring{
				ServiceMonitor: &mmv1beta.ServiceMonitor{Enabled: true},
				PrometheusRule: &mmv1beta.PrometheusRule{Enabled: true},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(mm).Build()
	r := &MattermostReconciler{
		Client:    c,
		Scheme:    s,
		Resources: resources.NewResourceHelper(c, s),
	}
	ctx := context.TODO()
	log := logr.Discard()

	metricsSvc := types.NamespacedName{Name: "mm-metrics", Namespace: "test"}
	sm := types.NamespacedName{Name: "mm", Namespace: "test"}
	rule := types.NamespacedName{Name: "mm-rules", Namespace: "test"}

	// --- Enabled: resources are created ---
	require.NoError(t, r.checkMattermostMonitoring(mm, log))

	require.NoError(t, c.Get(ctx, metricsSvc, &corev1.Service{}), "metrics Service should be created")
	require.NoError(t, c.Get(ctx, sm, &monitoringv1.ServiceMonitor{}), "ServiceMonitor should be created")
	require.NoError(t, c.Get(ctx, rule, &monitoringv1.PrometheusRule{}), "PrometheusRule should be created")

	// A second reconcile is a stable no-op (idempotent).
	require.NoError(t, r.checkMattermostMonitoring(mm, log))

	// --- Disabled: resources are cleaned up ---
	mm.Spec.Monitoring.ServiceMonitor.Enabled = false
	mm.Spec.Monitoring.PrometheusRule.Enabled = false
	require.NoError(t, r.checkMattermostMonitoring(mm, log))

	assert.True(t, apierrors.IsNotFound(c.Get(ctx, metricsSvc, &corev1.Service{})), "metrics Service should be deleted")
	assert.True(t, apierrors.IsNotFound(c.Get(ctx, sm, &monitoringv1.ServiceMonitor{})), "ServiceMonitor should be deleted")
	assert.True(t, apierrors.IsNotFound(c.Get(ctx, rule, &monitoringv1.PrometheusRule{})), "PrometheusRule should be deleted")
}
