package mattermost

import (
	"embed"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	sigsyaml "sigs.k8s.io/yaml"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
)

// PrometheusRule generation. The alert expressions are the embedded template
// prometheusrules/mattermost_monitoring_prometheus_rules.yaml. Mimir delivery
// (mattermost_monitoring_mimir_rules.go) loads that same file; it does not have
// a second copy.

// prometheusRuleFS embeds the Prometheus alerting/recording rules shipped with
// the Operator. Drop additional *.yaml files (each a `groups:` document) into
// the prometheusrules/ directory and their groups are merged automatically.
//
//go:embed prometheusrules/*.yaml
var prometheusRuleFS embed.FS

// Rule placeholder tokens. Because the Operator generates the PrometheusRule per
// Mattermost CR, we substitute these in the embedded rules so alert expressions
// scope to this instance's pods by name/label — never by static IP or CIDR.
const (
	rulePlaceholderNamespace = "__NAMESPACE__"
	// rulePlaceholderInstallation is the CR name. The ServiceMonitor copies the
	// pod label installation.mattermost.com/installation onto scraped series, so
	// rules match this instead of the Service name. Always paired with the
	// namespace: CR names are unique per namespace only.
	rulePlaceholderInstallation = "__INSTALLATION__"
	// rulePlaceholderPodSelector is a regex matching this instance's pods, used to
	// scope kube-state-metrics / cAdvisor series that carry a `pod` label.
	rulePlaceholderPodSelector = "__POD_SELECTOR__"

	rulePlaceholderHTTPErrorRatio     = "__HTTP_ERROR_RATIO__"
	rulePlaceholderHTTPErrorPercent   = "__HTTP_ERROR_PERCENT__"
	rulePlaceholderHTTPMinRPS         = "__HTTP_MIN_RPS__"
	rulePlaceholderLoginFailRatio     = "__LOGIN_FAILURE_RATIO__"
	rulePlaceholderLoginFailPercent   = "__LOGIN_FAILURE_PERCENT__"
	rulePlaceholderLoginMinFailures   = "__LOGIN_MIN_FAILURES__"
	rulePlaceholderCPULimitRatio      = "__CPU_LIMIT_RATIO__"
	rulePlaceholderCPULimitPercent    = "__CPU_LIMIT_PERCENT__"
	rulePlaceholderMemoryLimitRatio   = "__MEMORY_LIMIT_RATIO__"
	rulePlaceholderMemoryLimitPercent = "__MEMORY_LIMIT_PERCENT__"
	rulePlaceholderDBReplicaLagSecs   = "__DB_REPLICA_LAG_SECONDS__"
)

// alertDBReplicaLag is the one shipped alert that is off unless the CR asks for
// it. Mattermost only reports replica lag once SqlSettings.ReplicaLagSettings
// holds a lag query for each replica. That setting is a JSON array in the
// server config and cannot be set through an MM_SQLSETTINGS_* environment
// variable, so the Operator cannot see or enable it. Without those queries the
// metric has no series and the alert can never fire.
const alertDBReplicaLag = "MattermostDBReplicaLag"

// Defaults for prometheusRule.thresholds and mimirRules.thresholds. These match
// a real installation; a quiet or very large one should override them on the CR.
const (
	defaultHTTPErrorPercent    = 5
	defaultHTTPMinRPS          = 1
	defaultLoginFailurePercent = 50
	defaultLoginMinFailures    = 20
	defaultCPULimitPercent     = 85
	defaultMemoryLimitPercent  = 90
	// defaultDBReplicaLagSeconds only fills the placeholder. The alert is dropped
	// unless dbReplicaLagSeconds is set (see disabledAlerts).
	defaultDBReplicaLagSeconds = 5
)

// GeneratePrometheusRuleV1Beta builds a Prometheus Operator PrometheusRule from
// the embedded alerting/recording rules. It is created in the Mattermost
// namespace; the cluster's Prometheus reaches in to load it via its ruleSelector
// (Prometheus -> Mattermost). Rule expressions are scoped to this instance's
// pods via placeholder substitution — see rulePlaceholders.
func GeneratePrometheusRuleV1Beta(mattermost *mmv1beta.Mattermost) (*monitoringv1.PrometheusRule, error) {
	if mattermost.Spec.Monitoring == nil {
		return nil, nil
	}

	thresholds := prometheusRuleThresholds(mattermost)
	groups, err := loadEmbeddedPrometheusRuleGroups(rulePlaceholders(mattermost, thresholds), thresholds)
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
func rulePlaceholders(mattermost *mmv1beta.Mattermost, thresholds *mmv1beta.PrometheusRuleThresholds) map[string]string {
	httpErrorPercent := defaultHTTPErrorPercent
	httpMinRPS := defaultHTTPMinRPS
	loginFailurePercent := defaultLoginFailurePercent
	loginMinFailures := defaultLoginMinFailures
	cpuLimitPercent := defaultCPULimitPercent
	memoryLimitPercent := defaultMemoryLimitPercent
	dbReplicaLagSeconds := defaultDBReplicaLagSeconds
	if thresholds != nil {
		httpErrorPercent = intOr(thresholds.HTTPErrorPercent, defaultHTTPErrorPercent)
		httpMinRPS = intOr(thresholds.HTTPMinRequestsPerSecond, defaultHTTPMinRPS)
		loginFailurePercent = intOr(thresholds.LoginFailurePercent, defaultLoginFailurePercent)
		loginMinFailures = intOr(thresholds.LoginMinFailures, defaultLoginMinFailures)
		cpuLimitPercent = intOr(thresholds.CPULimitPercent, defaultCPULimitPercent)
		memoryLimitPercent = intOr(thresholds.MemoryLimitPercent, defaultMemoryLimitPercent)
		dbReplicaLagSeconds = intOr(thresholds.DBReplicaLagSeconds, defaultDBReplicaLagSeconds)
	}

	return map[string]string{
		rulePlaceholderNamespace:    mattermost.Namespace,
		rulePlaceholderInstallation: mattermost.Name,
		// Deployment pods are "<name>-<replicaset-hash>-<pod-suffix>". Prometheus
		// label matches are fully anchored, so "<name>-[^-]+-[^-]+" matches only
		// this instance's pods — not a differently-named install that happens to
		// share this name as a prefix (e.g. "mm" must not match "mm-test").
		// QuoteMeta escapes regex metacharacters ("test.mm" must not match
		// "testXmm-..."). The rules embed this inside a PromQL double-quoted
		// string in a YAML literal block, so backslashes are not unescaped by
		// YAML. PromQL still treats `\.` as an illegal escape. Two backslashes
		// survive into the expression, and PromQL reduces them to one for the
		// regex engine.
		rulePlaceholderPodSelector:        strings.ReplaceAll(regexp.QuoteMeta(mattermost.Name), `\`, `\\`) + "-[^-]+-[^-]+",
		rulePlaceholderHTTPErrorRatio:     fractionString(httpErrorPercent),
		rulePlaceholderHTTPErrorPercent:   strconv.Itoa(httpErrorPercent),
		rulePlaceholderHTTPMinRPS:         strconv.Itoa(httpMinRPS),
		rulePlaceholderLoginFailRatio:     fractionString(loginFailurePercent),
		rulePlaceholderLoginFailPercent:   strconv.Itoa(loginFailurePercent),
		rulePlaceholderLoginMinFailures:   strconv.Itoa(loginMinFailures),
		rulePlaceholderCPULimitRatio:      fractionString(cpuLimitPercent),
		rulePlaceholderCPULimitPercent:    strconv.Itoa(cpuLimitPercent),
		rulePlaceholderMemoryLimitRatio:   fractionString(memoryLimitPercent),
		rulePlaceholderMemoryLimitPercent: strconv.Itoa(memoryLimitPercent),
		rulePlaceholderDBReplicaLagSecs:   strconv.Itoa(dbReplicaLagSeconds),
	}
}

func prometheusRuleThresholds(mattermost *mmv1beta.Mattermost) *mmv1beta.PrometheusRuleThresholds {
	if mattermost.Spec.Monitoring == nil || mattermost.Spec.Monitoring.PrometheusRule == nil {
		return nil
	}
	return mattermost.Spec.Monitoring.PrometheusRule.Thresholds
}

// fractionString renders a 0-100 percent as the fraction PromQL compares against.
// 5 becomes "0.05".
func fractionString(percent int) string {
	return strconv.FormatFloat(float64(percent)/100, 'f', -1, 64)
}

func intOr(value *int, fallback int) int {
	if value != nil {
		return *value
	}
	return fallback
}

// disabledAlerts lists the shipped alerts the CR has not opted into.
func disabledAlerts(thresholds *mmv1beta.PrometheusRuleThresholds) map[string]bool {
	disabled := map[string]bool{}
	if thresholds == nil || thresholds.DBReplicaLagSeconds == nil {
		disabled[alertDBReplicaLag] = true
	}
	return disabled
}

// dropAlerts removes the named alerts and any group left empty.
func dropAlerts(groups []monitoringv1.RuleGroup, drop map[string]bool) []monitoringv1.RuleGroup {
	kept := make([]monitoringv1.RuleGroup, 0, len(groups))
	for _, group := range groups {
		rules := make([]monitoringv1.Rule, 0, len(group.Rules))
		for _, rule := range group.Rules {
			if !drop[rule.Alert] {
				rules = append(rules, rule)
			}
		}
		if len(rules) > 0 {
			group.Rules = rules
			kept = append(kept, group)
		}
	}
	return kept
}

// loadEmbeddedPrometheusRuleGroups reads and merges the rule groups from every
// embedded rules file (deterministic order for stable diffs), substituting the
// per-instance placeholders before parsing. Alerts the CR has not opted into are
// dropped.
func loadEmbeddedPrometheusRuleGroups(replacements map[string]string, thresholds *mmv1beta.PrometheusRuleThresholds) ([]monitoringv1.RuleGroup, error) {
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
	return dropAlerts(groups, disabledAlerts(thresholds)), nil
}

// newPlaceholderReplacer builds a strings.Replacer from the token->value map.
func newPlaceholderReplacer(replacements map[string]string) *strings.Replacer {
	pairs := make([]string, 0, len(replacements)*2)
	for k, v := range replacements {
		pairs = append(pairs, k, v)
	}
	return strings.NewReplacer(pairs...)
}
