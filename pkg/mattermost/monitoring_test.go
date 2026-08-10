package mattermost

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
)

func newMattermostWithMonitoring(monitoring *mmv1beta.Monitoring) *mmv1beta.Mattermost {
	return &mmv1beta.Mattermost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-mm",
			Namespace: "mattermost",
		},
		Spec: mmv1beta.MattermostSpec{
			Monitoring: monitoring,
		},
	}
}

func TestGenerateServiceMonitorV1Beta(t *testing.T) {
	t.Run("defaults when interval empty", func(t *testing.T) {
		mm := newMattermostWithMonitoring(&mmv1beta.Monitoring{
			ServiceMonitor: &mmv1beta.ServiceMonitor{Enabled: true},
		})

		sm := GenerateServiceMonitorV1Beta(mm)
		require.NotNil(t, sm)

		assert.Equal(t, "test-mm", sm.Name)
		assert.Equal(t, "mattermost", sm.Namespace)
		require.Len(t, sm.OwnerReferences, 1)
		assert.Equal(t, "Mattermost", sm.OwnerReferences[0].Kind)

		// Selector must match the dedicated metrics Service (unique label) so it
		// scrapes only that Service, not the main app Service.
		assert.Equal(t, map[string]string{metricsServiceLabel: "test-mm"}, sm.Spec.Selector.MatchLabels)

		require.Len(t, sm.Spec.Endpoints, 1)
		ep := sm.Spec.Endpoints[0]
		assert.Equal(t, metricsPortName, ep.Port)
		assert.Equal(t, metricsPath, ep.Path)
		assert.EqualValues(t, defaultScrapeInterval, ep.Interval)
	})

	t.Run("honors custom interval and extra labels", func(t *testing.T) {
		mm := newMattermostWithMonitoring(&mmv1beta.Monitoring{
			ServiceMonitor: &mmv1beta.ServiceMonitor{
				Enabled:  true,
				Interval: "15s",
				Labels:   map[string]string{"release": "kube-prometheus-stack"},
			},
		})

		sm := GenerateServiceMonitorV1Beta(mm)
		require.NotNil(t, sm)

		assert.EqualValues(t, "15s", sm.Spec.Endpoints[0].Interval)
		// Extra label present so the customer's Prometheus serviceMonitorSelector can match.
		assert.Equal(t, "kube-prometheus-stack", sm.Labels["release"])
	})
}

func TestGenerateMetricsServiceV1Beta(t *testing.T) {
	mm := newMattermostWithMonitoring(&mmv1beta.Monitoring{
		ServiceMonitor: &mmv1beta.ServiceMonitor{Enabled: true},
	})

	svc := GenerateMetricsServiceV1Beta(mm)
	require.NotNil(t, svc)

	assert.Equal(t, "test-mm-metrics", svc.Name)
	assert.Equal(t, "mattermost", svc.Namespace)
	// Distinct label the ServiceMonitor selects on.
	assert.Equal(t, "test-mm", svc.Labels[metricsServiceLabel])
	// Routes to the Mattermost pods; internal (headless) only.
	assert.Equal(t, mmv1beta.MattermostSelectorLabels("test-mm"), svc.Spec.Selector)
	assert.Equal(t, corev1.ClusterIPNone, svc.Spec.ClusterIP)
	require.Len(t, svc.Spec.Ports, 1)
	assert.Equal(t, metricsPortName, svc.Spec.Ports[0].Name)
	assert.EqualValues(t, metricsPort, svc.Spec.Ports[0].Port)
}

func TestClientMetricsPointer(t *testing.T) {
	// nil Enabled must leave server defaults untouched (no env emitted).
	mm := newMattermostWithMonitoring(&mmv1beta.Monitoring{
		ClientMetrics: &mmv1beta.ClientMetrics{},
	})
	assert.Nil(t, mm.Spec.Monitoring.ClientMetrics.Enabled)
}

func TestGenerateRtcdServiceMonitorV1Beta(t *testing.T) {
	t.Run("nil when no rtcd selector provided", func(t *testing.T) {
		mm := newMattermostWithMonitoring(&mmv1beta.Monitoring{
			CallsMetrics: &mmv1beta.CallsMetrics{Enabled: true},
		})
		assert.Nil(t, GenerateRtcdServiceMonitorV1Beta(mm))
	})

	t.Run("numeric port becomes targetPort and defaults to 8045", func(t *testing.T) {
		mm := newMattermostWithMonitoring(&mmv1beta.Monitoring{
			CallsMetrics: &mmv1beta.CallsMetrics{
				Enabled:             true,
				RtcdServiceSelector: map[string]string{"app": "rtcd"},
			},
		})

		sm := GenerateRtcdServiceMonitorV1Beta(mm)
		require.NotNil(t, sm)
		assert.Equal(t, "test-mm-rtcd", sm.Name)
		assert.Equal(t, map[string]string{"app": "rtcd"}, sm.Spec.Selector.MatchLabels)
		require.Len(t, sm.Spec.Endpoints, 1)
		require.NotNil(t, sm.Spec.Endpoints[0].TargetPort)
		assert.Equal(t, int32(8045), sm.Spec.Endpoints[0].TargetPort.IntVal)
		assert.Equal(t, metricsPath, sm.Spec.Endpoints[0].Path)
	})

	t.Run("named port becomes Port", func(t *testing.T) {
		mm := newMattermostWithMonitoring(&mmv1beta.Monitoring{
			CallsMetrics: &mmv1beta.CallsMetrics{
				Enabled:             true,
				RtcdServiceSelector: map[string]string{"app": "rtcd"},
				Port:                "metrics",
			},
		})

		sm := GenerateRtcdServiceMonitorV1Beta(mm)
		require.NotNil(t, sm)
		assert.Equal(t, "metrics", sm.Spec.Endpoints[0].Port)
		assert.Nil(t, sm.Spec.Endpoints[0].TargetPort)
	})

	t.Run("discovery labels fall back to serviceMonitor labels", func(t *testing.T) {
		// callsMetrics has no own labels, but serviceMonitor does — the rtcd SM
		// must inherit them so Prometheus selects it.
		mm := newMattermostWithMonitoring(&mmv1beta.Monitoring{
			ServiceMonitor: &mmv1beta.ServiceMonitor{
				Enabled: true,
				Labels:  map[string]string{"release": "kube-prometheus-stack"},
			},
			CallsMetrics: &mmv1beta.CallsMetrics{
				Enabled:             true,
				RtcdServiceSelector: map[string]string{"app": "rtcd"},
			},
		})

		sm := GenerateRtcdServiceMonitorV1Beta(mm)
		require.NotNil(t, sm)
		assert.Equal(t, "kube-prometheus-stack", sm.Labels["release"])
	})

	t.Run("own labels win over serviceMonitor labels", func(t *testing.T) {
		mm := newMattermostWithMonitoring(&mmv1beta.Monitoring{
			ServiceMonitor: &mmv1beta.ServiceMonitor{
				Enabled: true,
				Labels:  map[string]string{"release": "sm-stack"},
			},
			CallsMetrics: &mmv1beta.CallsMetrics{
				Enabled:             true,
				RtcdServiceSelector: map[string]string{"app": "rtcd"},
				Labels:              map[string]string{"release": "calls-stack"},
			},
		})

		sm := GenerateRtcdServiceMonitorV1Beta(mm)
		require.NotNil(t, sm)
		assert.Equal(t, "calls-stack", sm.Labels["release"])
	})
}

func TestGeneratePrometheusRuleV1Beta(t *testing.T) {
	t.Run("ships embedded rule groups with owner ref", func(t *testing.T) {
		mm := newMattermostWithMonitoring(&mmv1beta.Monitoring{
			PrometheusRule: &mmv1beta.PrometheusRule{Enabled: true},
		})

		pr, err := GeneratePrometheusRuleV1Beta(mm)
		require.NoError(t, err)
		require.NotNil(t, pr)

		assert.Equal(t, "test-mm-rules", pr.Name)
		assert.Equal(t, "mattermost", pr.Namespace)
		require.Len(t, pr.OwnerReferences, 1)
		assert.Equal(t, "Mattermost", pr.OwnerReferences[0].Kind)

		// The embedded starter rules must parse into at least one group/rule.
		require.NotEmpty(t, pr.Spec.Groups)
		assert.NotEmpty(t, pr.Spec.Groups[0].Rules)

		// Per-instance placeholders must be substituted so alerts scope to this
		// installation's pods (never left as raw tokens or static IPs/CIDR).
		var sawServerDown bool
		for _, g := range pr.Spec.Groups {
			for _, rule := range g.Rules {
				expr := rule.Expr.String()
				assert.NotContains(t, expr, "__", "placeholder token left unsubstituted: %s", expr)
				if rule.Alert == "MattermostServerDown" {
					sawServerDown = true
					assert.Contains(t, expr, `namespace="mattermost"`)
					assert.Contains(t, expr, `service="test-mm"`)
				}
			}
		}
		assert.True(t, sawServerDown, "expected MattermostServerDown alert")
	})

	t.Run("honors extra labels for ruleSelector matching", func(t *testing.T) {
		mm := newMattermostWithMonitoring(&mmv1beta.Monitoring{
			PrometheusRule: &mmv1beta.PrometheusRule{
				Enabled: true,
				Labels:  map[string]string{"role": "alert-rules"},
			},
		})

		pr, err := GeneratePrometheusRuleV1Beta(mm)
		require.NoError(t, err)
		require.NotNil(t, pr)

		assert.Equal(t, "alert-rules", pr.Labels["role"])
	})
}
