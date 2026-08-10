package mattermost

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	mattermostApp "github.com/mattermost/mattermost-operator/pkg/mattermost"
)

// checkMattermostMonitoring reconciles the opt-in monitoring resources and warns
// when metrics are requested without an Enterprise license (the /metrics endpoint
// is Enterprise-gated, so scrape targets would stay permanently down otherwise).
func (r *MattermostReconciler) checkMattermostMonitoring(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) error {
	r.warnMonitoringWithoutLicense(mattermost, reqLogger)

	if err := r.checkMattermostServiceMonitor(mattermost, reqLogger); err != nil {
		return err
	}
	if err := r.checkMattermostRtcdServiceMonitor(mattermost, reqLogger); err != nil {
		return err
	}
	return r.checkMattermostPrometheusRule(mattermost, reqLogger)
}

// checkMattermostServiceMonitor reconciles a Prometheus Operator ServiceMonitor
// for the Mattermost metrics endpoint. It is a no-op unless the flag is set, and
// it degrades gracefully (logged skip) when the Prometheus Operator CRDs are not
// installed in the cluster.
func (r *MattermostReconciler) checkMattermostServiceMonitor(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) error {
	if !serviceMonitorEnabled(mattermost) {
		return nil
	}

	desired := mattermostApp.GenerateServiceMonitorV1Beta(mattermost)

	return r.reconcileServiceMonitor(mattermost, desired, reqLogger)
}

// checkMattermostRtcdServiceMonitor reconciles a ServiceMonitor targeting an rtcd
// (Calls real-time daemon) Service that the user runs separately. It is a no-op
// unless callsMetrics is enabled; if enabled without an rtcd Service selector the
// generator returns nil and we log the misconfiguration rather than fail.
func (r *MattermostReconciler) checkMattermostRtcdServiceMonitor(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) error {
	if !callsMetricsEnabled(mattermost) {
		return nil
	}

	desired := mattermostApp.GenerateRtcdServiceMonitorV1Beta(mattermost)
	if desired == nil {
		reqLogger.Info("callsMetrics is enabled but rtcdServiceSelector is empty; skipping rtcd ServiceMonitor")
		return nil
	}

	return r.reconcileServiceMonitor(mattermost, desired, reqLogger)
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

// checkMattermostPrometheusRule reconciles a Prometheus Operator PrometheusRule
// holding Mattermost's alerting/recording rules. It is a no-op unless the flag
// is set and degrades gracefully when the Prometheus Operator CRDs are absent.
func (r *MattermostReconciler) checkMattermostPrometheusRule(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) error {
	if !prometheusRuleEnabled(mattermost) {
		return nil
	}

	desired, err := mattermostApp.GeneratePrometheusRuleV1Beta(mattermost)
	if err != nil {
		return errors.Wrap(err, "failed to generate prometheus rule")
	}

	if err = r.Resources.CreatePrometheusRuleIfNotExists(mattermost, desired, reqLogger); err != nil {
		return err
	}

	current := &monitoringv1.PrometheusRule{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, current)
	if err != nil {
		// CRD absent (creation was skipped) or not yet present — nothing to update.
		if apimeta.IsNoMatchError(err) || k8sErrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrap(err, "failed to fetch current prometheus rule")
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

func callsMetricsEnabled(mattermost *mmv1beta.Mattermost) bool {
	return mattermost.Spec.Monitoring != nil &&
		mattermost.Spec.Monitoring.CallsMetrics != nil &&
		mattermost.Spec.Monitoring.CallsMetrics.Enabled
}

// anyMonitoringEnabled reports whether any monitoring capability that depends on
// the (Enterprise-gated) /metrics endpoint is turned on.
func anyMonitoringEnabled(mattermost *mmv1beta.Mattermost) bool {
	return serviceMonitorEnabled(mattermost) ||
		prometheusRuleEnabled(mattermost) ||
		callsMetricsEnabled(mattermost)
}
