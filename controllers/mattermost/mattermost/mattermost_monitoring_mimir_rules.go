package mattermost

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	mattermostApp "github.com/mattermost/mattermost-operator/pkg/mattermost"
)

// Mimir rules ConfigMap reconcile. The rule body is the Prometheus rules
// template, not a separate file. See
// pkg/mattermost/mattermost_monitoring_mimir_rules.go.

// checkMattermostMimirRules reconciles a ConfigMap holding Mattermost's alerting
// rules in Prometheus rules format, for a Grafana Mimir ruler-sync to load (the
// Prometheus-free alternative to PrometheusRule). Deleted when the flag is off.
func (r *MattermostReconciler) checkMattermostMimirRules(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) error {
	if !mimirRulesEnabled(mattermost) {
		return r.deleteMimirRulesConfigMaps(mattermost, reqLogger)
	}

	desired, err := mattermostApp.GenerateMimirRulesConfigMapV1Beta(mattermost)
	if err != nil {
		return errors.Wrap(err, "failed to generate mimir rules config map")
	}

	if err = r.Resources.CreateConfigMapIfNotExists(mattermost, desired, reqLogger); err != nil {
		return err
	}

	current := &corev1.ConfigMap{}
	if err = r.Client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, current); err != nil {
		return errors.Wrap(err, "failed to fetch current mimir rules config map")
	}

	// Don't adopt or overwrite a pre-existing ConfigMap that this Mattermost does
	// not own — a name collision with an unrelated object. ResourceHelper.Update
	// would otherwise replace its data wholesale.
	if !metav1.IsControlledBy(current, mattermost) {
		return errors.Errorf("config map %s/%s already exists and is not owned by this Mattermost; refusing to overwrite", current.Namespace, current.Name)
	}

	return r.Resources.Update(current, desired, reqLogger)
}

// deleteMimirRulesConfigMaps deletes only the Mimir rules ConfigMaps this
// Mattermost owns (matched by the operator's marker label), so an unrelated
// ConfigMap that merely shares the derived name is never touched.
func (r *MattermostReconciler) deleteMimirRulesConfigMaps(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) error {
	owned := &corev1.ConfigMapList{}
	if err := r.Client.List(context.TODO(), owned,
		client.InNamespace(mattermost.Namespace),
		client.MatchingLabels(mattermostApp.MimirRulesConfigMapSelector(mattermost)),
	); err != nil {
		return errors.Wrap(err, "failed to list mimir rules config maps")
	}
	for i := range owned.Items {
		cm := &owned.Items[i]
		// Belt-and-suspenders on top of the marker-label list: only delete objects
		// this Mattermost actually controls.
		if !metav1.IsControlledBy(cm, mattermost) {
			reqLogger.Info("Skipping cleanup of config map not owned by this Mattermost", "name", cm.Name)
			continue
		}
		reqLogger.Info("Deleting mimir rules config map", "name", cm.Name)
		if err := r.Client.Delete(context.TODO(), cm); err != nil && !k8sErrors.IsNotFound(err) {
			return errors.Wrap(err, "failed to delete mimir rules config map")
		}
	}
	return nil
}

func mimirRulesEnabled(mattermost *mmv1beta.Mattermost) bool {
	return mattermost.Spec.Monitoring != nil &&
		mattermost.Spec.Monitoring.MimirRules != nil &&
		mattermost.Spec.Monitoring.MimirRules.Enabled
}
