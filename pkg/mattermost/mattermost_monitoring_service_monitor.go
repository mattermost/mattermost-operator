package mattermost

import (
	"strconv"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
)

// ServiceMonitor generation. The metrics Service this scrapes is created here
// too; there is no YAML template for either object.

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

	// defaultRtcdServicePort is the Service port name used when callsMetrics.port
	// is empty. A named port is matched against the Service. A numeric targetPort
	// is matched against a declared Pod container port, which rtcd Pods often omit.
	defaultRtcdServicePort = metricsPortName
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
	// A name selects the Service port. A number selects the Pod container port,
	// which Prometheus Operator ignores unless the container declares it.
	port := cm.Port
	if port == "" {
		port = defaultRtcdServicePort
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
