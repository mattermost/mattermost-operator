package mattermost

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	mattermostApp "github.com/mattermost/mattermost-operator/pkg/mattermost"
)

// checkMattermostMonitoring reconciles the opt-in monitoring resources and warns
// when metrics are requested without an Enterprise license (the /metrics endpoint
// is Enterprise-gated, so scrape targets would stay permanently down otherwise).
//
// Every check both creates when its flag is on and deletes when it is off, so
// turning a capability off cleans up after itself (mirrors the Ingress pattern).
func (r *MattermostReconciler) checkMattermostMonitoring(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) error {
	// Nothing to do — and nothing to clean up — for installations that never
	// declare a monitoring block. This avoids per-reconcile API calls for the
	// common case. Cleanup-on-disable still works via the `enabled: false` path,
	// which keeps the block present.
	if mattermost.Spec.Monitoring == nil {
		return nil
	}

	r.warnMonitoringWithoutLicense(mattermost, reqLogger)

	if err := r.checkMattermostServiceMonitor(mattermost, reqLogger); err != nil {
		return err
	}
	if err := r.checkMattermostRtcdServiceMonitor(mattermost, reqLogger); err != nil {
		return err
	}
	if err := r.checkMattermostPrometheusRule(mattermost, reqLogger); err != nil {
		return err
	}
	return r.checkMattermostGrafanaDashboard(mattermost, reqLogger)
}

// checkMattermostGrafanaDashboard reconciles one ConfigMap per embedded Grafana
// dashboard, labelled for discovery by the Grafana dashboard sidecar. When the
// flag is off every owned dashboard ConfigMap is deleted; when on, ConfigMaps no
// longer in the embedded set are pruned (e.g. a dashboard was removed/renamed).
func (r *MattermostReconciler) checkMattermostGrafanaDashboard(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) error {
	existing := &corev1.ConfigMapList{}
	if err := r.Client.List(context.TODO(), existing,
		client.InNamespace(mattermost.Namespace),
		client.MatchingLabels(mattermostApp.DashboardConfigMapSelector(mattermost)),
	); err != nil {
		return errors.Wrap(err, "failed to list grafana dashboard config maps")
	}

	if !grafanaDashboardEnabled(mattermost) {
		return r.deleteConfigMaps(existing.Items, reqLogger)
	}

	desired, err := mattermostApp.GenerateGrafanaDashboardConfigMapsV1Beta(mattermost)
	if err != nil {
		return errors.Wrap(err, "failed to generate grafana dashboard config maps")
	}

	desiredNames := make(map[string]bool, len(desired))
	for _, cm := range desired {
		desiredNames[cm.Name] = true

		if err = r.Resources.CreateConfigMapIfNotExists(mattermost, cm, reqLogger); err != nil {
			return err
		}

		current := &corev1.ConfigMap{}
		if err = r.Client.Get(context.TODO(), types.NamespacedName{Name: cm.Name, Namespace: cm.Namespace}, current); err != nil {
			return errors.Wrap(err, "failed to fetch current grafana dashboard config map")
		}

		// Don't adopt or overwrite a pre-existing ConfigMap that this Mattermost
		// does not own — a name collision with an unrelated object.
		if !metav1.IsControlledBy(current, mattermost) {
			return errors.Errorf("config map %s/%s already exists and is not owned by this Mattermost; refusing to overwrite", current.Namespace, current.Name)
		}

		if err = r.Resources.Update(current, cm, reqLogger); err != nil {
			return err
		}
	}

	// Prune dashboard ConfigMaps we own that are no longer in the embedded set.
	stale := existing.Items[:0]
	for i := range existing.Items {
		if !desiredNames[existing.Items[i].Name] {
			stale = append(stale, existing.Items[i])
		}
	}
	return r.deleteConfigMaps(stale, reqLogger)
}

// deleteConfigMaps deletes the given ConfigMaps, ignoring already-gone ones.
func (r *MattermostReconciler) deleteConfigMaps(items []corev1.ConfigMap, reqLogger logr.Logger) error {
	for i := range items {
		cm := &items[i]
		reqLogger.Info("Deleting grafana dashboard config map", "name", cm.Name)
		if err := r.Client.Delete(context.TODO(), cm); err != nil && !k8sErrors.IsNotFound(err) {
			return errors.Wrap(err, "failed to delete grafana dashboard config map")
		}
	}
	return nil
}

// checkMattermostServiceMonitor reconciles the dedicated metrics Service and the
// ServiceMonitor that scrapes it. When the flag is off both are deleted. Degrades
// gracefully (logged skip) when the Prometheus Operator CRDs are absent.
func (r *MattermostReconciler) checkMattermostServiceMonitor(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) error {
	metricsSvcKey := types.NamespacedName{Name: mattermost.Name + "-metrics", Namespace: mattermost.Namespace}
	smKey := types.NamespacedName{Name: mattermost.Name, Namespace: mattermost.Namespace}

	if !serviceMonitorEnabled(mattermost) {
		if err := r.Resources.DeleteServiceMonitor(smKey, reqLogger); err != nil {
			return err
		}
		return r.Resources.DeleteService(metricsSvcKey, reqLogger)
	}

	// The dedicated metrics Service must exist before the ServiceMonitor targets it.
	if err := r.reconcileService(mattermost, mattermostApp.GenerateMetricsServiceV1Beta(mattermost), reqLogger); err != nil {
		return err
	}

	return r.reconcileServiceMonitor(mattermost, mattermostApp.GenerateServiceMonitorV1Beta(mattermost), reqLogger)
}

// checkMattermostRtcdServiceMonitor reconciles a ServiceMonitor targeting an rtcd
// (Calls real-time daemon) Service that the user runs separately. Deleted when
// callsMetrics is off; if enabled without an rtcd Service selector the generator
// returns nil and we log rather than fail.
func (r *MattermostReconciler) checkMattermostRtcdServiceMonitor(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) error {
	rtcdKey := types.NamespacedName{Name: mattermost.Name + "-rtcd", Namespace: mattermost.Namespace}

	if !callsMetricsEnabled(mattermost) {
		return r.Resources.DeleteServiceMonitor(rtcdKey, reqLogger)
	}

	desired := mattermostApp.GenerateRtcdServiceMonitorV1Beta(mattermost)
	if desired == nil {
		reqLogger.Info("callsMetrics is enabled but rtcdServiceSelector is empty; skipping rtcd ServiceMonitor")
		return nil
	}

	return r.reconcileServiceMonitor(mattermost, desired, reqLogger)
}

// checkMattermostPrometheusRule reconciles the PrometheusRule holding Mattermost's
// alerting rules. Deleted when the flag is off; degrades gracefully when the
// Prometheus Operator CRDs are absent.
func (r *MattermostReconciler) checkMattermostPrometheusRule(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) error {
	ruleKey := types.NamespacedName{Name: mattermost.Name + "-rules", Namespace: mattermost.Namespace}

	if !prometheusRuleEnabled(mattermost) {
		return r.Resources.DeletePrometheusRule(ruleKey, reqLogger)
	}

	desired, err := mattermostApp.GeneratePrometheusRuleV1Beta(mattermost)
	if err != nil {
		return errors.Wrap(err, "failed to generate prometheus rule")
	}

	if err = r.Resources.CreatePrometheusRuleIfNotExists(mattermost, desired, reqLogger); err != nil {
		return err
	}

	current := &monitoringv1.PrometheusRule{}
	err = r.Client.Get(context.TODO(), ruleKey, current)
	if err != nil {
		// CRD absent (creation was skipped) or not yet present — nothing to update.
		if apimeta.IsNoMatchError(err) || k8sErrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrap(err, "failed to fetch current prometheus rule")
	}

	return r.Resources.Update(current, desired, reqLogger)
}

// reconcileService creates-or-updates a Service.
func (r *MattermostReconciler) reconcileService(mattermost *mmv1beta.Mattermost, desired *corev1.Service, reqLogger logr.Logger) error {
	if err := r.Resources.CreateServiceIfNotExists(mattermost, desired, reqLogger); err != nil {
		return err
	}

	current := &corev1.Service{}
	if err := r.Client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, current); err != nil {
		return errors.Wrap(err, "failed to fetch current metrics service")
	}

	return r.Resources.Update(current, desired, reqLogger)
}

// reconcileServiceMonitor creates-or-updates a ServiceMonitor, treating an absent
// Prometheus Operator CRD as a logged no-op.
func (r *MattermostReconciler) reconcileServiceMonitor(mattermost *mmv1beta.Mattermost, desired *monitoringv1.ServiceMonitor, reqLogger logr.Logger) error {
	if err := r.Resources.CreateServiceMonitorIfNotExists(mattermost, desired, reqLogger); err != nil {
		return err
	}

	current := &monitoringv1.ServiceMonitor{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, current)
	if err != nil {
		// CRD absent (creation was skipped) or not yet present — nothing to update.
		if apimeta.IsNoMatchError(err) || k8sErrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrap(err, "failed to fetch current service monitor")
	}

	return r.Resources.Update(current, desired, reqLogger)
}

// warnMonitoringWithoutLicense surfaces the Enterprise-license requirement. The
// operator always enables MM_METRICSSETTINGS_ENABLE, but the /metrics endpoint is
// only served under an Enterprise license — so monitoring without a licenseSecret
// yields a permanently-down scrape target. Best-effort: log + Event (no status
// condition, since MattermostStatus has no conditions slice).
func (r *MattermostReconciler) warnMonitoringWithoutLicense(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) {
	if !anyMonitoringEnabled(mattermost) || mattermost.Spec.LicenseSecret != "" {
		return
	}

	const msg = "monitoring is enabled but spec.licenseSecret is empty; the Mattermost /metrics endpoint requires an Enterprise license, so scrape targets will stay down until a license is configured"
	reqLogger.Info("WARNING: " + msg)
	if r.Recorder != nil {
		r.Recorder.Event(mattermost, corev1.EventTypeWarning, "MonitoringRequiresEnterpriseLicense", msg)
	}
}

func serviceMonitorEnabled(mattermost *mmv1beta.Mattermost) bool {
	return mattermost.Spec.Monitoring != nil &&
		mattermost.Spec.Monitoring.ServiceMonitor != nil &&
		mattermost.Spec.Monitoring.ServiceMonitor.Enabled
}

func prometheusRuleEnabled(mattermost *mmv1beta.Mattermost) bool {
	return mattermost.Spec.Monitoring != nil &&
		mattermost.Spec.Monitoring.PrometheusRule != nil &&
		mattermost.Spec.Monitoring.PrometheusRule.Enabled
}

func grafanaDashboardEnabled(mattermost *mmv1beta.Mattermost) bool {
	return mattermost.Spec.Monitoring != nil &&
		mattermost.Spec.Monitoring.GrafanaDashboard != nil &&
		mattermost.Spec.Monitoring.GrafanaDashboard.Enabled
}

func callsMetricsEnabled(mattermost *mmv1beta.Mattermost) bool {
	return mattermost.Spec.Monitoring != nil &&
		mattermost.Spec.Monitoring.CallsMetrics != nil &&
		mattermost.Spec.Monitoring.CallsMetrics.Enabled
}

// anyMonitoringEnabled reports whether any monitoring capability that depends on
// the (Enterprise-gated) Mattermost /metrics endpoint is turned on. callsMetrics
// is intentionally excluded: it targets a separately-deployed rtcd whose metrics
// are not gated by the Mattermost license.
func anyMonitoringEnabled(mattermost *mmv1beta.Mattermost) bool {
	return serviceMonitorEnabled(mattermost) ||
		prometheusRuleEnabled(mattermost)
}
