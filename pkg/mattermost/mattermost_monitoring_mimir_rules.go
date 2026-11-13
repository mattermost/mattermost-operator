package mattermost

import (
	"fmt"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	sigsyaml "sigs.k8s.io/yaml"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
)

// Mimir rules ConfigMap. The rule body is not a separate template: it is
// prometheusrules/mattermost_monitoring_prometheus_rules.yaml, rendered with
// this install's mimirRules.thresholds.

const (
	// defaultMimirRulesDiscoveryLabel/Value is what a Mimir ruler-sync sidecar
	// selects on by default when loading rules ConfigMaps.
	defaultMimirRulesDiscoveryLabel      = "mimir_rules"
	defaultMimirRulesDiscoveryLabelValue = "1"

	// mimirRulesConfigMapLabel marks the Mimir rules ConfigMap the Operator owns,
	// independent of the (configurable) discovery label, so it can be cleaned up.
	mimirRulesConfigMapLabel = "mattermost.com/mimir-rules"

	// mimirTenantAnnotation carries the target Mimir tenant/org id for a
	// multi-tenant ruler-sync to load the rules under.
	mimirTenantAnnotation = "monitoring.grafana.com/mimir-tenant"
)

func mimirRuleThresholds(mattermost *mmv1beta.Mattermost) *mmv1beta.PrometheusRuleThresholds {
	if mattermost.Spec.Monitoring == nil || mattermost.Spec.Monitoring.MimirRules == nil {
		return nil
	}
	return mattermost.Spec.Monitoring.MimirRules.Thresholds
}

// GenerateMimirRulesConfigMapV1Beta renders the same per-instance, pod-scoped
// alerting rules as GeneratePrometheusRuleV1Beta, but as a plain ConfigMap in
// Prometheus rules format (`groups:`) for a Grafana Mimir ruler. This is the
// Prometheus-free delivery path: a Mimir ruler-sync (mimirtool or a sidecar)
// loads the ConfigMap into Mimir, instead of the Prometheus Operator consuming a
// PrometheusRule CRD. The ServiceMonitor + metrics Service are unchanged — Grafana
// Alloy scrapes them and remote-writes to Mimir.
func GenerateMimirRulesConfigMapV1Beta(mattermost *mmv1beta.Mattermost) (*corev1.ConfigMap, error) {
	// Monitoring is an opt-in add-on; nothing to generate when it is unset.
	if mattermost.Spec.Monitoring == nil {
		return nil, nil
	}

	discoveryLabel := defaultMimirRulesDiscoveryLabel
	discoveryValue := defaultMimirRulesDiscoveryLabelValue
	var tenant string

	if mr := mattermost.Spec.Monitoring.MimirRules; mr != nil {
		if mr.DiscoveryLabel != "" {
			// The marker label is reserved for cleanup; a discovery label that
			// collides with it would be silently overwritten below, breaking
			// ruler-sync. Reject it instead of producing a broken ConfigMap.
			if mr.DiscoveryLabel == mimirRulesConfigMapLabel {
				return nil, fmt.Errorf("mimirRules.discoveryLabel must not be the reserved key %q", mimirRulesConfigMapLabel)
			}
			discoveryLabel = mr.DiscoveryLabel
		}
		if mr.DiscoveryLabelValue != "" {
			discoveryValue = mr.DiscoveryLabelValue
		}
		tenant = mr.Tenant
	}

	thresholds := mimirRuleThresholds(mattermost)
	groups, err := loadEmbeddedPrometheusRuleGroups(rulePlaceholders(mattermost, thresholds), thresholds)
	if err != nil {
		return nil, err
	}

	// Marshal to Prometheus rules format (a top-level `groups:` doc) — exactly what
	// a Mimir ruler / mimirtool expects.
	rulesYAML, err := sigsyaml.Marshal(monitoringv1.PrometheusRuleSpec{Groups: groups})
	if err != nil {
		return nil, err
	}

	labels := mattermost.MattermostLabels(mattermost.Name)
	// Discovery label the ruler-sync watches for.
	labels[discoveryLabel] = discoveryValue
	// Stable marker (independent of the discovery label) for cleanup.
	labels[mimirRulesConfigMapLabel] = mattermost.Name

	var annotations map[string]string
	if tenant != "" {
		annotations = map[string]string{mimirTenantAnnotation: tenant}
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            mattermost.Name + "-mimir-rules",
			Namespace:       mattermost.Namespace,
			Labels:          labels,
			Annotations:     annotations,
			OwnerReferences: MattermostOwnerReference(mattermost),
		},
		Data: map[string]string{mattermost.Name + "-rules.yaml": string(rulesYAML)},
	}, nil
}

// MimirRulesConfigMapSelector returns the label selector identifying the Mimir
// rules ConfigMap owned by this Mattermost — used for safe (owned-only) cleanup.
func MimirRulesConfigMapSelector(mattermost *mmv1beta.Mattermost) map[string]string {
	return map[string]string{mimirRulesConfigMapLabel: mattermost.Name}
}
