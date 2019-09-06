package clusterinstallation

import (
	"github.com/go-logr/logr"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
)

func (r *ReconcileClusterInstallation) checkCanary(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	c := mattermost.Spec.Canary
	if c.Enable {
		reqLogger = reqLogger.WithValues("Reconcile", "mattermost")

		err := r.checkMattermostService(mattermost, c.Name, c.Name, reqLogger)
		if err != nil {
			return err
		}
		err = r.checkMattermostIngress(mattermost, c.Name, mattermost.Spec.IngressName, mattermost.Spec.Canary.IngressAnnotations, reqLogger)
		if err != nil {
			return err
		}
		err = r.checkMattermostDeployment(mattermost, c.Name, mattermost.Spec.IngressName, c.GetCanaryImageName(), reqLogger)
		if err != nil {
			return err
		}
	}

	return nil
}
