package clusterinstallation

import (
	"context"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"

	"github.com/go-logr/logr"
)

func (r *ReconcileClusterInstallation) checkMinioDeployment(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	minioExist := false

	// TODO implement
	if minioExist {
		return r.client.Update(context.TODO(), mattermost)
	}
	return nil
}
