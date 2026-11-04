package mattermost

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	mattermostApp "github.com/mattermost/mattermost-operator/pkg/mattermost"
	"github.com/mattermost/mattermost-operator/pkg/resources"
)

// PrometheusRule reconcile. The alert template is
// pkg/mattermost/prometheusrules/mattermost_monitoring_prometheus_rules.yaml.

// checkMattermostPrometheusRule reconciles the PrometheusRule holding Mattermost's
// alerting rules. Deleted when the flag is off; degrades gracefully when the
// Prometheus Operator CRDs are absent.
func (r *MattermostReconciler) checkMattermostPrometheusRule(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) error {
	ruleKey := types.NamespacedName{Name: mattermost.Name + "-rules", Namespace: mattermost.Namespace}

	if !prometheusRuleEnabled(mattermost) {
		return r.deleteOwnedResource(mattermost, ruleKey, &monitoringv1.PrometheusRule{}, reqLogger)
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
		if apimeta.IsNoMatchError(err) {
			return resources.ErrPrometheusCRDAbsent
		}
		if k8sErrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrap(err, "failed to fetch current prometheus rule")
	}

	if err := ensureOwnedForUpdate(mattermost, current); err != nil {
		return err
	}
	return r.Resources.Update(current, desired, reqLogger)
}

func prometheusRuleEnabled(mattermost *mmv1beta.Mattermost) bool {
	return mattermost.Spec.Monitoring != nil &&
		mattermost.Spec.Monitoring.PrometheusRule != nil &&
		mattermost.Spec.Monitoring.PrometheusRule.Enabled
}
