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
	dbPassword := ""
	dbUser := ""
	if mattermost.Spec.DatabaseType.ExternalDatabaseSecret != "" {
		err := r.checkSecret(mattermost.Spec.DatabaseType.ExternalDatabaseSecret, mattermost.Namespace)
		if err != nil {
			return errors.Wrap(err, "Error getting the external database secret.")
		}
		externalDB = true
	} else {
		var err error
		dbPassword, err = r.getMySQLSecrets(mattermost, reqLogger)
		if err != nil {
			return errors.Wrap(err, "Error getting the database password.")
		}
		dbUser = "root"
	}

	minioService, err := r.getMinioService(mattermost, reqLogger)
	if err != nil {
		return errors.Wrap(err, "Error getting the minio service.")
	}

	esService := ""
	if mattermost.Spec.EnableElasticSearch {
		var err error
		esService, err = r.getESService(mattermost, reqLogger)
		if err != nil {
			return errors.Wrap(err, "Error getting the elasticSearch service.")
		}
	}

	return r.createDeploymentIfNotExists(mattermost, mattermost.GenerateDeployment(dbUser, dbPassword, externalDB, minioService, esService), reqLogger)
}
