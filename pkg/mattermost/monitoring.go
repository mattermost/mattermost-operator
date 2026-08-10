package mattermost

import (
	"embed"
	"path"
	"sort"
	"strconv"
	"strings"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	sigsyaml "sigs.k8s.io/yaml"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
)

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

	// defaultRtcdMetricsPort is the rtcd metrics port (Calls real-time daemon).
	defaultRtcdMetricsPort = "8045"
)

// Rule placeholder tokens. Because the Operator generates the PrometheusRule per
// Mattermost CR, we substitute these in the embedded rules so alert expressions
// scope to this instance's pods by name/label — never by static IP/CIDR.
const (
	rulePlaceholderNamespace = "__NAMESPACE__"
	// rulePlaceholderService matches the metrics Service (== CR name), used to
	// scope the ServiceMonitor-labelled series (up, mattermost_*).
	rulePlaceholderService = "__SERVICE__"
	// rulePlaceholderPodSelector is a regex matching this instance's pods, used to
	// scope kube-state-metrics / cAdvisor series that carry a `pod` label.
	rulePlaceholderPodSelector = "__POD_SELECTOR__"
)

// GenerateServiceMonitorV1Beta builds a Prometheus Operator ServiceMonitor that
// scrapes the Mattermost metrics endpoint. It is created in the Mattermost
// namespace; the cluster's Prometheus reaches in to scrape it (Prometheus ->
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

// GenerateRtcdServiceMonitorV1Beta builds a ServiceMonitor targeting an rtcd
// (Calls real-time daemon) Service that the user runs alongside Mattermost.
// rtcd is not deployed by this Operator, so the caller supplies the rtcd Service
// selector and (optionally) the metrics port. Returns nil if no selector is set,
// since a ServiceMonitor with an empty selector would scrape unrelated Services.
func GenerateRtcdServiceMonitorV1Beta(mattermost *mmv1beta.Mattermost) *monitoringv1.ServiceMonitor {
	cm := mattermost.Spec.Monitoring.CallsMetrics
	if cm == nil || len(cm.RtcdServiceSelector) == 0 {
		return nil
	}

	interval := defaultScrapeInterval
	labels := mattermost.MattermostLabels(mattermost.Name)
	if sm := mattermost.Spec.Monitoring.ServiceMonitor; sm != nil {
		if sm.Interval != "" {
			interval = sm.Interval
		}
		// Reuse the same discovery labels so the rtcd ServiceMonitor matches the
		// same Prometheus serviceMonitorSelector as the Mattermost one.
		for k, v := range sm.Labels {
			labels[k] = v
		}
	}

	endpoint := monitoringv1.Endpoint{
		Path:     metricsPath,
		Interval: monitoringv1.Duration(interval),
	}
	// Accept either a numeric port (targetPort) or a named Service port.
	port := cm.Port
	if port == "" {
		port = defaultRtcdMetricsPort
	}
	if n, err := strconv.Atoi(port); err == nil {
		tp := intstr.FromInt(n)
		endpoint.TargetPort = &tp
	} else {
		endpoint.Port = port
	}

	return &monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:            mattermost.Name + "-rtcd",
			Namespace:       mattermost.Namespace,
			Labels:          labels,
			OwnerReferences: MattermostOwnerReference(mattermost),
		},
		Spec: monitoringv1.ServiceMonitorSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: cm.RtcdServiceSelector,
			},
			Endpoints: []monitoringv1.Endpoint{endpoint},
		},
	}
}

// GeneratePrometheusRuleV1Beta builds a Prometheus Operator PrometheusRule from
// the embedded alerting/recording rules. It is created in the Mattermost
// namespace; the cluster's Prometheus reaches in to load it via its ruleSelector
// (Prometheus -> Mattermost). Rule expressions are scoped to this instance's
// pods via placeholder substitution — see rulePlaceholders.
func GeneratePrometheusRuleV1Beta(mattermost *mmv1beta.Mattermost) (*monitoringv1.PrometheusRule, error) {
	groups, err := loadEmbeddedPrometheusRuleGroups(rulePlaceholders(mattermost))
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

// rulePlaceholders maps the embedded-rule tokens to this instance's identity so
// the shipped alerts fire only for this Mattermost's pods, not every workload in
// the cluster. The pod selector is the Deployment's pod-name prefix ("<name>-.*").
func rulePlaceholders(mattermost *mmv1beta.Mattermost) map[string]string {
	return map[string]string{
		rulePlaceholderNamespace:   mattermost.Namespace,
		rulePlaceholderService:     mattermost.Name,
		rulePlaceholderPodSelector: mattermost.Name + "-.*",
	}
}

// loadEmbeddedPrometheusRuleGroups reads and merges the rule groups from every
// embedded rules file (deterministic order for stable diffs), substituting the
// per-instance placeholders before parsing.
func loadEmbeddedPrometheusRuleGroups(replacements map[string]string) ([]monitoringv1.RuleGroup, error) {
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

	replacer := newPlaceholderReplacer(replacements)

	var groups []monitoringv1.RuleGroup
	for _, name := range names {
		content, readErr := prometheusRuleFS.ReadFile(path.Join("prometheusrules", name))
		if readErr != nil {
			return nil, readErr
		}

		var spec monitoringv1.PrometheusRuleSpec
		if unmarshalErr := sigsyaml.Unmarshal([]byte(replacer.Replace(string(content))), &spec); unmarshalErr != nil {
			return nil, unmarshalErr
		}
		groups = append(groups, spec.Groups...)
	}
	return groups, nil
}

// newPlaceholderReplacer builds a strings.Replacer from the token->value map.
func newPlaceholderReplacer(replacements map[string]string) *strings.Replacer {
	pairs := make([]string, 0, len(replacements)*2)
	for k, v := range replacements {
		pairs = append(pairs, k, v)
	}
	return strings.NewReplacer(pairs...)
}
