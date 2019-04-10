package clusterinstallation

import (
	"fmt"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"

	"github.com/go-logr/logr"
)

func (r *ReconcileClusterInstallation) checkMattermost(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	err := r.checkMattermostService(mattermost, reqLogger)
	if err != nil {
		return err
	}

	err = r.checkMattermostIngress(mattermost, reqLogger)
	if err != nil {
		return err
	}

	err = r.checkMattermostDeployment(mattermost, reqLogger)
	if err != nil {
		return err
	}
	return nil
}

func (r *ReconcileClusterInstallation) checkMattermostService(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	return r.createServiceIfNotExists(mattermost, mattermost.GenerateService(), reqLogger)
}

func (r *ReconcileClusterInstallation) checkMattermostIngress(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	return r.createIngressIfNotExists(mattermost, mattermost.GenerateIngress(), reqLogger)
}

func (r *ReconcileClusterInstallation) checkMattermostDeployment(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	dbPassword, err := r.getMySQLSecrets(mattermost, reqLogger)
	if err != nil {
		return fmt.Errorf("Error getting the database password. Err=%s", err.Error())
	}

	return r.createDeploymentIfNotExists(mattermost, mattermost.GenerateDeployment("", dbPassword), reqLogger)

}
