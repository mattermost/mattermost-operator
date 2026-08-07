package mattermost

import (
	"embed"
	"path"
	"sort"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	sigsyaml "sigs.k8s.io/yaml"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
)

// dashboardFS embeds the Grafana dashboard JSON shipped with the Operator. Drop
// additional *.json files into the dashboards/ directory and they are picked up
// automatically — one ConfigMap data key per file.
//
//go:embed dashboards/*.json
var dashboardFS embed.FS

// prometheusRuleFS embeds the Prometheus alerting/recording rules shipped with
// the Operator. Drop additional *.yaml files (each a `groups:` document) into
// the prometheusrules/ directory and their groups are merged automatically.
//
//go:embed prometheusrules/*.yaml
var prometheusRuleFS embed.FS

const (
	// metricsPortName matches the Mattermost Service port exposing /metrics (8067).
	metricsPortName = "metrics"
	metricsPath     = "/metrics"

	defaultScrapeInterval = "30s"

	// defaultDashboardDiscoveryLabel/Value is what the Grafana dashboard sidecar
	// selects on by default (kiwigrid/k8s-sidecar convention).
	defaultDashboardDiscoveryLabel      = "grafana_dashboard"
	defaultDashboardDiscoveryLabelValue = "1"
)

// GenerateServiceMonitorV1Beta builds a Prometheus Operator ServiceMonitor that
// scrapes the Mattermost metrics endpoint. It is created in the Mattermost
// namespace; the cluster's Prometheus reaches in to scrape it (Grafana ->
// Mattermost), so the Operator never touches the monitoring namespace.
func GenerateServiceMonitorV1Beta(mattermost *mmv1beta.Mattermost) *monitoringv1.ServiceMonitor {
	interval := defaultScrapeInterval
	labels := mattermost.MattermostLabels(mattermost.Name)

	if sm := mattermost.Spec.Monitoring.ServiceMonitor; sm != nil {
		if sm.Interval != "" {
			interval = sm.Interval
		}
		// Extra labels let the operator's ServiceMonitor match a customer's
		// Prometheus serviceMonitorSelector.
		for k, v := range sm.Labels {
			labels[k] = v
		}
	}

	return &monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:            mattermost.Name,
			Namespace:       mattermost.Namespace,
			Labels:          labels,
			OwnerReferences: MattermostOwnerReference(mattermost),
		},
		Spec: monitoringv1.ServiceMonitorSpec{
			// Select the Mattermost Service by its stable identity labels.
			Selector: metav1.LabelSelector{
				MatchLabels: mmv1beta.MattermostSelectorLabels(mattermost.Name),
			},
			Endpoints: []monitoringv1.Endpoint{
				{
					Port:     metricsPortName,
					Path:     metricsPath,
					Interval: monitoringv1.Duration(interval),
				},
			},
		},
	}
}

// GenerateGrafanaDashboardConfigMapV1Beta builds a ConfigMap containing every
// embedded dashboard JSON, labelled for discovery by the Grafana dashboard
// sidecar. It is created in the Mattermost namespace; a Grafana running
// elsewhere reads it (Grafana -> Mattermost).
func GenerateGrafanaDashboardConfigMapV1Beta(mattermost *mmv1beta.Mattermost) (*corev1.ConfigMap, error) {
	discoveryLabel := defaultDashboardDiscoveryLabel
	discoveryValue := defaultDashboardDiscoveryLabelValue

	if gd := mattermost.Spec.Monitoring.GrafanaDashboard; gd != nil {
		if gd.DiscoveryLabel != "" {
			discoveryLabel = gd.DiscoveryLabel
		}
		if gd.DiscoveryLabelValue != "" {
			discoveryValue = gd.DiscoveryLabelValue
		}
	}

	data, err := loadEmbeddedDashboards()
	if err != nil {
		return nil, err
	}

	labels := mattermost.MattermostLabels(mattermost.Name)
	// The discovery label is what the Grafana sidecar watches for.
	labels[discoveryLabel] = discoveryValue

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            mattermost.Name + "-grafana-dashboards",
			Namespace:       mattermost.Namespace,
			Labels:          labels,
			OwnerReferences: MattermostOwnerReference(mattermost),
		},
		Data: data,
	}, nil
}

// loadEmbeddedDashboards reads every embedded dashboard file into a map keyed by
// file name (deterministic order for stable ConfigMap diffs).
func loadEmbeddedDashboards() (map[string]string, error) {
	entries, err := dashboardFS.ReadDir("dashboards")
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	data := make(map[string]string, len(names))
	for _, name := range names {
		content, readErr := dashboardFS.ReadFile(path.Join("dashboards", name))
		if readErr != nil {
			return nil, readErr
		}
		data[name] = string(content)
	}
	return data, nil
}

// GeneratePrometheusRuleV1Beta builds a Prometheus Operator PrometheusRule from
// the embedded alerting/recording rules. It is created in the Mattermost
// namespace; the cluster's Prometheus reaches in to load it via its ruleSelector
// (Grafana/Prometheus -> Mattermost).
func GeneratePrometheusRuleV1Beta(mattermost *mmv1beta.Mattermost) (*monitoringv1.PrometheusRule, error) {
	groups, err := loadEmbeddedPrometheusRuleGroups()
	if err != nil {
		return nil, err
	}

	labels := mattermost.MattermostLabels(mattermost.Name)
	if pr := mattermost.Spec.Monitoring.PrometheusRule; pr != nil {
		// Extra labels let the operator's PrometheusRule match a customer's
		// Prometheus ruleSelector.
		for k, v := range pr.Labels {
			labels[k] = v
		}
	}

	return &monitoringv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:            mattermost.Name + "-rules",
			Namespace:       mattermost.Namespace,
			Labels:          labels,
			OwnerReferences: MattermostOwnerReference(mattermost),
		},
		Spec: monitoringv1.PrometheusRuleSpec{
			Groups: groups,
		},
	}, nil
}

// loadEmbeddedPrometheusRuleGroups reads and merges the rule groups from every
// embedded rules file (deterministic order for stable diffs).
func loadEmbeddedPrometheusRuleGroups() ([]monitoringv1.RuleGroup, error) {
	entries, err := prometheusRuleFS.ReadDir("prometheusrules")
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	var groups []monitoringv1.RuleGroup
	for _, name := range names {
		content, readErr := prometheusRuleFS.ReadFile(path.Join("prometheusrules", name))
		if readErr != nil {
			return nil, readErr
		}

		var spec monitoringv1.PrometheusRuleSpec
		if unmarshalErr := sigsyaml.Unmarshal(content, &spec); unmarshalErr != nil {
			return nil, unmarshalErr
		}
		groups = append(groups, spec.Groups...)
	}
	return groups, nil
}
