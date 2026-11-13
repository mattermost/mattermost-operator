package mattermost

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
)

// checkMattermostMonitoring reconciles the opt-in monitoring resources and warns
// when metrics are requested without an Enterprise license (the /metrics endpoint
// is Enterprise-gated, so scrape targets would stay permanently down otherwise).
//
// Each capability lives in its own file:
//
//	mattermost_monitoring_service_monitor.go
//	mattermost_monitoring_prometheus_rules.go
//	mattermost_monitoring_mimir_rules.go
//
// Every check both creates when its flag is on and deletes when it is off, so
// turning a capability off cleans up after itself (mirrors the Ingress pattern).
// A missing monitoring block is treated as disabled: owned resources from a
// previous reconcile are removed, and nothing new is created.
func (r *MattermostReconciler) checkMattermostMonitoring(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) error {
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
	return r.checkMattermostMimirRules(mattermost, reqLogger)
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

// deleteOwnedResource deletes the object at key only if this Mattermost is its
// controller owner, so cleanup-on-disable never removes an identically-named
// resource the operator did not create. An absent Prometheus Operator CRD or an
// already-gone object is a no-op.
func (r *MattermostReconciler) deleteOwnedResource(mattermost *mmv1beta.Mattermost, key types.NamespacedName, obj client.Object, reqLogger logr.Logger) error {
	if err := r.Client.Get(context.TODO(), key, obj); err != nil {
		if apimeta.IsNoMatchError(err) || k8sErrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrap(err, "failed to fetch resource for cleanup")
	}
	if !metav1.IsControlledBy(obj, mattermost) {
		reqLogger.Info("Skipping cleanup of resource not owned by this Mattermost", "name", key.Name)
		return nil
	}
	if err := r.Client.Delete(context.TODO(), obj); err != nil && !k8sErrors.IsNotFound(err) {
		return errors.Wrap(err, "failed to delete resource")
	}
	return nil
}

// ensureOwnedForUpdate refuses to overwrite a pre-existing object with the
// generated name that this Mattermost does not control (a name collision).
func ensureOwnedForUpdate(mattermost *mmv1beta.Mattermost, obj metav1.Object) error {
	if !metav1.IsControlledBy(obj, mattermost) {
		return errors.Errorf("%s/%s already exists and is not owned by this Mattermost; refusing to overwrite", obj.GetNamespace(), obj.GetName())
	}
	return nil
}

// anyMonitoringEnabled reports whether any monitoring capability that depends on
// the (Enterprise-gated) Mattermost /metrics endpoint is turned on. callsMetrics
// is intentionally excluded: it targets a separately-deployed rtcd whose metrics
// are not gated by the Mattermost license.
func anyMonitoringEnabled(mattermost *mmv1beta.Mattermost) bool {
	return serviceMonitorEnabled(mattermost) ||
		prometheusRuleEnabled(mattermost) ||
		mimirRulesEnabled(mattermost)
}
