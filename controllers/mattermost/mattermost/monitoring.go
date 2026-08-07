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

// checkMattermostServiceMonitor reconciles a Prometheus Operator ServiceMonitor
// for the Mattermost metrics endpoint. It is a no-op unless the flag is set, and
// it degrades gracefully (logged skip) when the Prometheus Operator CRDs are not
// installed in the cluster.
func (r *MattermostReconciler) checkMattermostServiceMonitor(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) error {
	if !serviceMonitorEnabled(mattermost) {
		return nil
	}

	desired := mattermostApp.GenerateServiceMonitorV1Beta(mattermost)

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

// checkMattermostGrafanaDashboard reconciles a ConfigMap holding the embedded
// Grafana dashboards, labelled for discovery by the Grafana dashboard sidecar.
// It is a no-op unless the flag is set.
func (r *MattermostReconciler) checkMattermostGrafanaDashboard(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) error {
	if !grafanaDashboardEnabled(mattermost) {
		return nil
	}

	desired, err := mattermostApp.GenerateGrafanaDashboardConfigMapV1Beta(mattermost)
	if err != nil {
		return errors.Wrap(err, "failed to generate grafana dashboard config map")
	}

	if err = r.Resources.CreateConfigMapIfNotExists(mattermost, desired, reqLogger); err != nil {
		return err
	}

	current := &corev1.ConfigMap{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, current)
	if err != nil {
		return errors.Wrap(err, "failed to fetch current grafana dashboard config map")
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

func serviceMonitorEnabled(mattermost *mmv1beta.Mattermost) bool {
	return mattermost.Spec.Monitoring != nil &&
		mattermost.Spec.Monitoring.ServiceMonitor != nil &&
		mattermost.Spec.Monitoring.ServiceMonitor.Enabled
}

func grafanaDashboardEnabled(mattermost *mmv1beta.Mattermost) bool {
	return mattermost.Spec.Monitoring != nil &&
		mattermost.Spec.Monitoring.GrafanaDashboard != nil &&
		mattermost.Spec.Monitoring.GrafanaDashboard.Enabled
}

func prometheusRuleEnabled(mattermost *mmv1beta.Mattermost) bool {
	return mattermost.Spec.Monitoring != nil &&
		mattermost.Spec.Monitoring.PrometheusRule != nil &&
		mattermost.Spec.Monitoring.PrometheusRule.Enabled
}
