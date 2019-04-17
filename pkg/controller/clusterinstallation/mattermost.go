package clusterinstallation

import (
	"github.com/go-logr/logr"
	"github.com/pkg/errors"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
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
	externalDB := false
	var dbPassword string
	var dbUser string
	var err error
	if mattermost.Spec.DatabaseType.ExternalDatabase != "" {
		externalDB = true
		dbPassword = ""
		dbUser = ""
	} else {
		dbPassword, err = r.getMySQLSecrets(mattermost, reqLogger)
		if err != nil {
			return errors.Wrap(err, "Error getting the database password.")
		}
		dbUser = "root"
	}

	return r.createDeploymentIfNotExists(mattermost, mattermost.GenerateDeployment(dbUser, dbPassword, externalDB), reqLogger)
}
