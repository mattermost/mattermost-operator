package mattermost

import (
	"embed"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
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

// dashboardFS embeds the Grafana dashboard JSON shipped with the Operator. Drop
// additional *.json files into the dashboards/ directory and they are picked up
// automatically — one ConfigMap per file.
//
//go:embed dashboards/*.json
var dashboardFS embed.FS

const (
	// metricsPortName matches the Mattermost Service port exposing /metrics (8067).
	metricsPortName = "metrics"
	metricsPath     = "/metrics"

	defaultScrapeInterval = "30s"

	// metricsPort is the Mattermost metrics container port (8067).
	metricsPort = 8067

	// metricsServiceLabel marks the dedicated metrics Service so the
	// ServiceMonitor can target it uniquely (the main app Service does not carry
	// this label). This is what makes scraping work in every service mode —
	// including useServiceLoadBalancer, where the app Service drops port 8067.
	metricsServiceLabel = "mattermost.com/scrape-metrics"

	// defaultRtcdMetricsPort is the rtcd metrics port (Calls real-time daemon).
	defaultRtcdMetricsPort = "8045"

	// defaultDashboardDiscoveryLabel/Value is what the Grafana dashboard sidecar
	// selects on by default (kiwigrid/k8s-sidecar convention).
	defaultDashboardDiscoveryLabel      = "grafana_dashboard"
	defaultDashboardDiscoveryLabelValue = "1"

	// dashboardConfigMapLabel marks every dashboard ConfigMap the Operator owns,
	// independent of the (configurable) sidecar discovery label. Used to find and
	// prune stale dashboard ConfigMaps regardless of discovery-label changes.
	dashboardConfigMapLabel = "mattermost.com/grafana-dashboard"
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

// GenerateMetricsServiceV1Beta builds a dedicated, internal (headless ClusterIP)
// Service exposing only the Mattermost metrics port. The ServiceMonitor targets
// THIS Service rather than the main app Service, so scraping works in every
// service mode — including useServiceLoadBalancer, where the app Service exposes
// only 80/443 and drops the metrics port. Metrics stay internal to the cluster;
// they are never published through the LoadBalancer.
func GenerateMetricsServiceV1Beta(mattermost *mmv1beta.Mattermost) *corev1.Service {
	labels := mattermost.MattermostLabels(mattermost.Name)
	// Distinct label so the ServiceMonitor selects only this Service.
	labels[metricsServiceLabel] = mattermost.Name

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            metricsServiceName(mattermost.Name),
			Namespace:       mattermost.Namespace,
			Labels:          labels,
			OwnerReferences: MattermostOwnerReference(mattermost),
		},
		Spec: corev1.ServiceSpec{
			// Headless: we only need per-pod endpoints for Prometheus to scrape.
			ClusterIP: corev1.ClusterIPNone,
			Type:      corev1.ServiceTypeClusterIP,
			// Route to the Mattermost pods (same selector the app Service uses).
			Selector:                 mmv1beta.MattermostSelectorLabels(mattermost.Name),
			PublishNotReadyAddresses: true,
			Ports: []corev1.ServicePort{
				{
					Name:       metricsPortName,
					Port:       metricsPort,
					TargetPort: intstr.FromString(metricsPortName),
				},
			},
		},
	}
}

func metricsServiceName(mattermostName string) string {
	return mattermostName + "-metrics"
}

// rtcdDiscoveryLabels resolves the Prometheus discovery labels for the rtcd
// ServiceMonitor: callsMetrics.labels win, otherwise fall back to the main
// serviceMonitor.labels.
func rtcdDiscoveryLabels(mattermost *mmv1beta.Mattermost) map[string]string {
	if cm := mattermost.Spec.Monitoring.CallsMetrics; cm != nil && len(cm.Labels) > 0 {
		return cm.Labels
	}
	if sm := mattermost.Spec.Monitoring.ServiceMonitor; sm != nil {
		return sm.Labels
	}
	return nil
}

// GenerateServiceMonitorV1Beta builds a Prometheus Operator ServiceMonitor that
// scrapes the dedicated metrics Service. It is created in the Mattermost
// namespace; the cluster's Prometheus reaches in to scrape it (Prometheus ->
// Mattermost), so the Operator never touches the monitoring namespace.
func GenerateServiceMonitorV1Beta(mattermost *mmv1beta.Mattermost) *monitoringv1.ServiceMonitor {
	if mattermost.Spec.Monitoring == nil {
		return nil
	}

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
			// Select the dedicated metrics Service (unique label).
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{metricsServiceLabel: mattermost.Name},
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
	if mattermost.Spec.Monitoring == nil {
		return nil
	}

	cm := mattermost.Spec.Monitoring.CallsMetrics
	if cm == nil || len(cm.RtcdServiceSelector) == 0 {
		return nil
	}

	interval := defaultScrapeInterval
	labels := mattermost.MattermostLabels(mattermost.Name)
	if sm := mattermost.Spec.Monitoring.ServiceMonitor; sm != nil && sm.Interval != "" {
		interval = sm.Interval
	}
	// Discovery labels so the cluster Prometheus serviceMonitorSelector matches
	// this ServiceMonitor: prefer callsMetrics.labels, fall back to the
	// serviceMonitor labels (so enabling calls alone still gets selected when the
	// main ServiceMonitor is configured).
	for k, v := range rtcdDiscoveryLabels(mattermost) {
		labels[k] = v
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

// GenerateGrafanaDashboardConfigMapsV1Beta builds one ConfigMap per embedded
// dashboard, each labelled for discovery by the Grafana dashboard sidecar. One
// ConfigMap per dashboard keeps every object well under the 1MB ConfigMap limit
// (the combined dashboards exceed 400KB) and matches the sidecar's
// one-file-per-ConfigMap convention. Created in the Mattermost namespace; a
// Grafana running elsewhere reads them (Grafana -> Mattermost).
func GenerateGrafanaDashboardConfigMapsV1Beta(mattermost *mmv1beta.Mattermost) ([]*corev1.ConfigMap, error) {
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

	dashboards, err := loadEmbeddedDashboards()
	if err != nil {
		return nil, err
	}

	configMaps := make([]*corev1.ConfigMap, 0, len(dashboards))
	for _, d := range dashboards {
		labels := mattermost.MattermostLabels(mattermost.Name)
		// The discovery label is what the Grafana sidecar watches for.
		labels[discoveryLabel] = discoveryValue
		// Stable marker (independent of the discovery label) for pruning.
		labels[dashboardConfigMapLabel] = mattermost.Name

		configMaps = append(configMaps, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:            dashboardConfigMapName(mattermost.Name, d.name),
				Namespace:       mattermost.Namespace,
				Labels:          labels,
				OwnerReferences: MattermostOwnerReference(mattermost),
			},
			Data: map[string]string{d.name: d.content},
		})
	}
	return configMaps, nil
}

// DashboardConfigMapSelector returns the label selector identifying every
// dashboard ConfigMap owned by this Mattermost — used to prune stale ones.
func DashboardConfigMapSelector(mattermost *mmv1beta.Mattermost) map[string]string {
	return map[string]string{dashboardConfigMapLabel: mattermost.Name}
}

// dashboardConfigMapName derives a stable, DNS-safe ConfigMap name from the
// Mattermost name and the dashboard file name.
func dashboardConfigMapName(mattermostName, fileName string) string {
	slug := strings.TrimSuffix(fileName, ".json")
	slug = strings.NewReplacer("_", "-", ".", "-").Replace(slug)
	return fmt.Sprintf("%s-grafana-%s", mattermostName, slug)
}

type embeddedDashboard struct {
	name    string
	content string
}

// loadEmbeddedDashboards reads every embedded dashboard file in deterministic
// order (stable ConfigMap diffs).
func loadEmbeddedDashboards() ([]embeddedDashboard, error) {
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

	dashboards := make([]embeddedDashboard, 0, len(names))
	for _, name := range names {
		content, readErr := dashboardFS.ReadFile(path.Join("dashboards", name))
		if readErr != nil {
			return nil, readErr
		}
		dashboards = append(dashboards, embeddedDashboard{name: name, content: string(content)})
	}
	return dashboards, nil
}

// GeneratePrometheusRuleV1Beta builds a Prometheus Operator PrometheusRule from
// the embedded alerting/recording rules. It is created in the Mattermost
// namespace; the cluster's Prometheus reaches in to load it via its ruleSelector
// (Prometheus -> Mattermost). Rule expressions are scoped to this instance's
// pods via placeholder substitution — see rulePlaceholders.
func GeneratePrometheusRuleV1Beta(mattermost *mmv1beta.Mattermost) (*monitoringv1.PrometheusRule, error) {
	if mattermost.Spec.Monitoring == nil {
		return nil, nil
	}

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
		rulePlaceholderNamespace: mattermost.Namespace,
		// The ServiceMonitor scrapes the dedicated <name>-metrics Service, so the
		// Prometheus Operator sets the `service` target label to that Service name.
		rulePlaceholderService: metricsServiceName(mattermost.Name),
		// Deployment pods are "<name>-<replicaset-hash>-<pod-suffix>". Prometheus
		// label matches are fully anchored, so "<name>-[^-]+-[^-]+" matches only
		// this instance's pods — not a differently-named install that happens to
		// share this name as a prefix (e.g. "mm" must not match "mm-test").
		rulePlaceholderPodSelector: mattermost.Name + "-[^-]+-[^-]+",
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
