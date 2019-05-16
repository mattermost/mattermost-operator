package clusterinstallation

import (
	"errors"

	"github.com/go-logr/logr"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
)

// TODO: implement postgres
func (r *ReconcileClusterInstallation) checkPostgres(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	// reqLogger := reqLogger.WithValues("Reconcile", "postgres")

	return errors.New("Database type 'postgres' not yet implemented")
}
