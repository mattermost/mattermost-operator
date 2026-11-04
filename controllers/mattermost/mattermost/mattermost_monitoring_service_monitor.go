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
	"github.com/mattermost/mattermost-operator/pkg/resources"
)

// ServiceMonitor reconcile, including the dedicated metrics Service and the
// optional rtcd ServiceMonitor. Generators are in
// pkg/mattermost/mattermost_monitoring_service_monitor.go.

// checkMattermostServiceMonitor reconciles the dedicated metrics Service and the
// ServiceMonitor that scrapes it. When the flag is off both are deleted. Degrades
// gracefully (logged skip) when the Prometheus Operator CRDs are absent.
func (r *MattermostReconciler) checkMattermostServiceMonitor(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) error {
	metricsSvcKey := types.NamespacedName{Name: mattermost.Name + "-metrics", Namespace: mattermost.Namespace}
	smKey := types.NamespacedName{Name: mattermost.Name, Namespace: mattermost.Namespace}

	if !serviceMonitorEnabled(mattermost) {
		if err := r.deleteOwnedResource(mattermost, smKey, &monitoringv1.ServiceMonitor{}, reqLogger); err != nil {
			return err
		}
		return r.deleteOwnedResource(mattermost, metricsSvcKey, &corev1.Service{}, reqLogger)
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
		return r.deleteOwnedResource(mattermost, rtcdKey, &monitoringv1.ServiceMonitor{}, reqLogger)
	}

	desired := mattermostApp.GenerateRtcdServiceMonitorV1Beta(mattermost)
	if desired == nil {
		// callsMetrics is enabled but the selector was cleared: tear down any rtcd
		// ServiceMonitor we previously created rather than leaving it orphaned.
		reqLogger.Info("callsMetrics is enabled but rtcdServiceSelector is empty; removing any owned rtcd ServiceMonitor")
		return r.deleteOwnedResource(mattermost, rtcdKey, &monitoringv1.ServiceMonitor{}, reqLogger)
	}

	return r.reconcileServiceMonitor(mattermost, desired, reqLogger)
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

	if err := ensureOwnedForUpdate(mattermost, current); err != nil {
		return err
	}

	// Preserve the API-assigned, immutable clusterIP/clusterIPs (the metrics
	// Service is headless, so clusterIPs is ["None"]); otherwise the update tries
	// to clear them and fails on the next reconcile.
	resources.CopyServiceEmptyAutoAssignedFields(desired, current)
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
		if apimeta.IsNoMatchError(err) {
			return resources.ErrPrometheusCRDAbsent
		}
		// Created in this reconcile and not yet visible, or already removed.
		if k8sErrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrap(err, "failed to fetch current service monitor")
	}

	if err := ensureOwnedForUpdate(mattermost, current); err != nil {
		return err
	}
	return r.Resources.Update(current, desired, reqLogger)
}

func serviceMonitorEnabled(mattermost *mmv1beta.Mattermost) bool {
	return mattermost.Spec.Monitoring != nil &&
		mattermost.Spec.Monitoring.ServiceMonitor != nil &&
		mattermost.Spec.Monitoring.ServiceMonitor.Enabled
}

func callsMetricsEnabled(mattermost *mmv1beta.Mattermost) bool {
	return mattermost.Spec.Monitoring != nil &&
		mattermost.Spec.Monitoring.CallsMetrics != nil &&
		mattermost.Spec.Monitoring.CallsMetrics.Enabled
}
