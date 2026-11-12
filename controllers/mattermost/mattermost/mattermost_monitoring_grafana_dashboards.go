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
		return r.deleteConfigMaps(mattermost, existing.Items, reqLogger)
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

		if err = ensureOwnedForUpdate(mattermost, current); err != nil {
			return err
		}

		if err = r.Resources.Update(current, cm, reqLogger); err != nil {
			return err
		}
	}

	var stale []corev1.ConfigMap
	for i := range existing.Items {
		if !desiredNames[existing.Items[i].Name] {
			stale = append(stale, existing.Items[i])
		}
	}
	return r.deleteConfigMaps(mattermost, stale, reqLogger)
}

// deleteConfigMaps deletes the given ConfigMaps, but only those this Mattermost
// controls, so an unrelated ConfigMap that merely carries the discovery or marker
// label is never removed. Already-gone objects are ignored.
func (r *MattermostReconciler) deleteConfigMaps(mattermost *mmv1beta.Mattermost, items []corev1.ConfigMap, reqLogger logr.Logger) error {
	for i := range items {
		cm := &items[i]
		if !metav1.IsControlledBy(cm, mattermost) {
			reqLogger.Info("Skipping cleanup of config map not owned by this Mattermost", "name", cm.Name)
			continue
		}
		reqLogger.Info("Deleting grafana dashboard config map", "name", cm.Name)
		if err := r.Client.Delete(context.TODO(), cm); err != nil && !k8sErrors.IsNotFound(err) {
			return errors.Wrap(err, "failed to delete grafana dashboard config map")
		}
	}
	return nil
}

func grafanaDashboardEnabled(mattermost *mmv1beta.Mattermost) bool {
	return mattermost.Spec.Monitoring != nil &&
		mattermost.Spec.Monitoring.GrafanaDashboard != nil &&
		mattermost.Spec.Monitoring.GrafanaDashboard.Enabled
}
