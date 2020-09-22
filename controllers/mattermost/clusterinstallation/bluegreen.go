package clusterinstallation

import (
	"github.com/go-logr/logr"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/apis/mattermost/v1alpha1"
)

func (r *ClusterInstallationReconciler) checkBlueGreen(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	if mattermost.Spec.BlueGreen.Enable {
		reqLogger = reqLogger.WithValues("Reconcile", "mattermost")

		blueGreen := []mattermostv1alpha1.AppDeployment{mattermost.Spec.BlueGreen.Blue, mattermost.Spec.BlueGreen.Green}
		for _, deployment := range blueGreen {
			err := r.checkMattermostService(mattermost, deployment.Name, deployment.Name, reqLogger)
			if err != nil {
				return err
			}
			if !mattermost.Spec.UseServiceLoadBalancer {
				err = r.checkMattermostIngress(mattermost, deployment.Name, deployment.IngressName, mattermost.Spec.IngressAnnotations, reqLogger)
				if err != nil {
					return err
				}
			}
			err = r.checkMattermostDeployment(mattermost, deployment.Name, deployment.IngressName, deployment.GetDeploymentImageName(), reqLogger)
			if err != nil {
				return err
			}
		}

		err := r.deleteMattermostDeployment(mattermost, mattermost.GetName(), reqLogger)
		if err != nil {
			return err
		}
	} else {
		if mattermost.Status.BlueName != "" {
			err := r.deleteAllMattermostComponents(mattermost, mattermost.Status.BlueName, reqLogger)
			if err != nil {
				return err
			}
		}
		if mattermost.Status.GreenName != "" {
			err := r.deleteAllMattermostComponents(mattermost, mattermost.Status.GreenName, reqLogger)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
