package mattermost

import (
	"embed"
	"fmt"
	"path"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
)

// dashboardFS embeds the Grafana dashboard JSON shipped with the Operator. Drop
// additional *.json files into the dashboards/ directory and they are picked up
// automatically, one ConfigMap per file.
//
//go:embed dashboards/*.json
var dashboardFS embed.FS

const (
	// defaultDashboardDiscoveryLabel/Value is what the Grafana dashboard sidecar
	// selects on by default (kiwigrid/k8s-sidecar convention).
	defaultDashboardDiscoveryLabel      = "grafana_dashboard"
	defaultDashboardDiscoveryLabelValue = "1"

	// dashboardConfigMapLabel marks every dashboard ConfigMap the Operator owns,
	// independent of the (configurable) sidecar discovery label. Used to find and
	// prune stale dashboard ConfigMaps regardless of discovery-label changes.
	dashboardConfigMapLabel = "mattermost.com/grafana-dashboard"
)

// GenerateGrafanaDashboardConfigMapsV1Beta builds one ConfigMap per embedded
// dashboard, each labelled for discovery by the Grafana dashboard sidecar. One
// ConfigMap per dashboard keeps every object well under the 1MB ConfigMap limit
// (the combined dashboards exceed 400KB) and matches the sidecar's
// one-file-per-ConfigMap convention. Created in the Mattermost namespace; a
// Grafana running elsewhere reads them.
func GenerateGrafanaDashboardConfigMapsV1Beta(mattermost *mmv1beta.Mattermost) ([]*corev1.ConfigMap, error) {
	if mattermost.Spec.Monitoring == nil {
		return nil, nil
	}

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

	// The ownership marker shares the label map with the discovery label, so a
	// collision would overwrite the value the sidecar expects.
	if discoveryLabel == dashboardConfigMapLabel {
		return nil, fmt.Errorf("grafanaDashboard.discoveryLabel must not be the reserved key %q", dashboardConfigMapLabel)
	}

	dashboards, err := loadEmbeddedDashboards()
	if err != nil {
		return nil, err
	}

	configMaps := make([]*corev1.ConfigMap, 0, len(dashboards))
	for _, d := range dashboards {
		labels := mattermost.MattermostLabels(mattermost.Name)
		labels[discoveryLabel] = discoveryValue
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
// dashboard ConfigMap owned by this Mattermost, used to prune stale ones.
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
// order so ConfigMap diffs stay stable.
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
