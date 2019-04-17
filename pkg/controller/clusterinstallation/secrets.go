package clusterinstallation

import (
	"github.com/go-logr/logr"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
)

func (r *ReconcileClusterInstallation) checkMattermostSecret(secretName, keyName, data string, mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {

	return r.createSecretIfNotExists(mattermost, mattermost.GenerateSecret(secretName, keyName, data), reqLogger)
}
