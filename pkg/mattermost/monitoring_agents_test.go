package mattermost

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
)

func TestGenerateAgentsServiceMonitorV1Beta(t *testing.T) {
	t.Run("nil unless enabled", func(t *testing.T) {
		assert.Nil(t, GenerateAgentsServiceMonitorV1Beta(newMattermostWithMonitoring(nil)))
		assert.Nil(t, GenerateAgentsServiceMonitorV1Beta(newMattermostWithMonitoring(&mmv1beta.Monitoring{})))
		assert.Nil(t, GenerateAgentsServiceMonitorV1Beta(newMattermostWithMonitoring(&mmv1beta.Monitoring{
			AgentsMetrics: &mmv1beta.AgentsMetrics{Enabled: false},
		})))
	})

	t.Run("scrapes the plugin path through the metrics Service", func(t *testing.T) {
		sm := GenerateAgentsServiceMonitorV1Beta(newMattermostWithMonitoring(&mmv1beta.Monitoring{
			AgentsMetrics: &mmv1beta.AgentsMetrics{Enabled: true},
		}))
		require.NotNil(t, sm)

		assert.Equal(t, "test-mm-agents", sm.Name)
		assert.Equal(t, "mattermost", sm.Namespace)
		require.Len(t, sm.OwnerReferences, 1)
		assert.Equal(t, "Mattermost", sm.OwnerReferences[0].Kind)
		assert.Equal(t, map[string]string{metricsServiceLabel: "test-mm"}, sm.Spec.Selector.MatchLabels)

		require.Len(t, sm.Spec.Endpoints, 1)
		ep := sm.Spec.Endpoints[0]
		assert.Equal(t, metricsPortName, ep.Port)
		assert.Equal(t, "/plugins/mattermost-ai/metrics", ep.Path)
		assert.EqualValues(t, defaultScrapeInterval, ep.Interval)

		require.Len(t, ep.RelabelConfigs, 1)
		assert.Equal(t, "job", ep.RelabelConfigs[0].TargetLabel)
		require.NotNil(t, ep.RelabelConfigs[0].Replacement)
		assert.Equal(t, "test-mm-agents", *ep.RelabelConfigs[0].Replacement)
	})

	t.Run("no installation label, so a failed scrape cannot trip MattermostServerDown", func(t *testing.T) {
		sm := GenerateAgentsServiceMonitorV1Beta(newMattermostWithMonitoring(&mmv1beta.Monitoring{
			AgentsMetrics: &mmv1beta.AgentsMetrics{Enabled: true},
		}))
		require.NotNil(t, sm)
		assert.Empty(t, sm.Spec.PodTargetLabels)
	})

	t.Run("interval falls back to serviceMonitor, own interval wins", func(t *testing.T) {
		fallback := GenerateAgentsServiceMonitorV1Beta(newMattermostWithMonitoring(&mmv1beta.Monitoring{
			ServiceMonitor: &mmv1beta.ServiceMonitor{Interval: "15s"},
			AgentsMetrics:  &mmv1beta.AgentsMetrics{Enabled: true},
		}))
		assert.EqualValues(t, "15s", fallback.Spec.Endpoints[0].Interval)

		own := GenerateAgentsServiceMonitorV1Beta(newMattermostWithMonitoring(&mmv1beta.Monitoring{
			ServiceMonitor: &mmv1beta.ServiceMonitor{Interval: "15s"},
			AgentsMetrics:  &mmv1beta.AgentsMetrics{Enabled: true, Interval: "60s"},
		}))
		assert.EqualValues(t, "60s", own.Spec.Endpoints[0].Interval)
	})

	t.Run("discovery labels fall back to serviceMonitor, own labels win", func(t *testing.T) {
		fallback := GenerateAgentsServiceMonitorV1Beta(newMattermostWithMonitoring(&mmv1beta.Monitoring{
			ServiceMonitor: &mmv1beta.ServiceMonitor{Labels: map[string]string{"release": "kps"}},
			AgentsMetrics:  &mmv1beta.AgentsMetrics{Enabled: true},
		}))
		assert.Equal(t, "kps", fallback.Labels["release"])

		own := GenerateAgentsServiceMonitorV1Beta(newMattermostWithMonitoring(&mmv1beta.Monitoring{
			ServiceMonitor: &mmv1beta.ServiceMonitor{Labels: map[string]string{"release": "kps"}},
			AgentsMetrics:  &mmv1beta.AgentsMetrics{Enabled: true, Labels: map[string]string{"release": "agents"}},
		}))
		assert.Equal(t, "agents", own.Labels["release"])
	})
}

func agentsRuleExprs(t *testing.T, agents *mmv1beta.AgentsMetrics) map[string]string {
	t.Helper()
	pr, err := GeneratePrometheusRuleV1Beta(newMattermostWithMonitoring(&mmv1beta.Monitoring{
		PrometheusRule: &mmv1beta.PrometheusRule{Enabled: true},
		AgentsMetrics:  agents,
	}))
	require.NoError(t, err)
	out := map[string]string{}
	for _, g := range pr.Spec.Groups {
		assert.NotEmpty(t, g.Rules, "group %s should not be emitted empty", g.Name)
		for _, r := range g.Rules {
			out[r.Alert] = r.Expr.String()
		}
	}
	return out
}

func TestAgentsAlerts(t *testing.T) {
	errorPercent := 30
	tokens := int64(500000)

	t.Run("none without agentsMetrics", func(t *testing.T) {
		for _, agents := range []*mmv1beta.AgentsMetrics{nil, {Enabled: false}} {
			got := agentsRuleExprs(t, agents)
			assert.NotContains(t, got, "MattermostAgentsMetricsDown")
			assert.Contains(t, got, "MattermostServerDown")
		}
	})

	t.Run("only MattermostAgentsMetricsDown by default", func(t *testing.T) {
		for _, thresholds := range []*mmv1beta.AgentsMetricsThresholds{nil, {}} {
			got := agentsRuleExprs(t, &mmv1beta.AgentsMetrics{Enabled: true, Thresholds: thresholds})
			assert.Contains(t, got, "MattermostAgentsMetricsDown")
			assert.NotContains(t, got, alertAgentsHighErrorRate)
			assert.NotContains(t, got, alertAgentsTokenUsageHigh)
		}
	})

	t.Run("thresholds turn on their alerts", func(t *testing.T) {
		got := agentsRuleExprs(t, &mmv1beta.AgentsMetrics{
			Enabled: true,
			Thresholds: &mmv1beta.AgentsMetricsThresholds{
				HTTPErrorPercent: &errorPercent,
				TokensPerHour:    &tokens,
			},
		})
		assert.Contains(t, got[alertAgentsHighErrorRate], "> 0.3")
		assert.Contains(t, got[alertAgentsTokenUsageHigh], "> 500000")
	})

	t.Run("every Agents expression matches the Agents job, never the installation label", func(t *testing.T) {
		got := agentsRuleExprs(t, &mmv1beta.AgentsMetrics{
			Enabled: true,
			Thresholds: &mmv1beta.AgentsMetricsThresholds{
				HTTPErrorPercent: &errorPercent,
				TokensPerHour:    &tokens,
			},
		})
		for _, alert := range []string{"MattermostAgentsMetricsDown", alertAgentsHighErrorRate, alertAgentsTokenUsageHigh} {
			expr := got[alert]
			require.NotEmpty(t, expr, alert)
			assert.NotRegexp(t, `__[A-Z_]+__`, expr, "%s has an unsubstituted placeholder", alert)
			assert.Contains(t, expr, `namespace="mattermost"`, alert)
			assert.Contains(t, expr, `job="test-mm-agents"`, alert)
			assert.NotContains(t, expr, "installation_mattermost_com_installation", alert)
		}
		assert.Contains(t, got["MattermostServerDown"], `installation_mattermost_com_installation="test-mm"`)
	})
}
