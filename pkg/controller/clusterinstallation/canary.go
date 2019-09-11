package clusterinstallation

import (
	"github.com/go-logr/logr"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
)

func (r *ReconcileClusterInstallation) checkCanary(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	reqLogger = reqLogger.WithValues("Reconcile", "mattermost")
	if mattermost.Spec.Canary.Enable {
		ingressAnnotations := map[string]string{
			"kubernetes.io/ingress.class":                  "nginx",
			"nginx.ingress.kubernetes.io/canary":           "true",
			"nginx.ingress.kubernetes.io/canary-by-cookie": "canary",
		}
		c := mattermostv1alpha1.AppDeployment(mattermost.Spec.Canary.Deployment)
		err := r.checkMattermostService(mattermost, c.Name, c.Name, reqLogger)
		if err != nil {
			return err
		}
		err = r.checkMattermostIngress(mattermost, c.Name, mattermost.Spec.IngressName, ingressAnnotations, reqLogger)
		if err != nil {
			return err
		}
		err = r.checkMattermostDeployment(mattermost, c.Name, mattermost.Spec.IngressName, c.GetDeploymentImageName(), reqLogger)
		if err != nil {
			return err
		}
	}

	return nil
}
