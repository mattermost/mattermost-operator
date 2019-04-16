package clusterinstallation

import (
	"context"

	"github.com/go-logr/logr"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
)

func (r *ReconcileClusterInstallation) checkDBPostgresDeployment(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	dbExist := false

	// TODO implement
	if dbExist {
		return r.client.Update(context.TODO(), mattermost)
	}
	return nil
}
