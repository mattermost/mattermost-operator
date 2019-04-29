package clusterinstallation

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
)

func (r *ReconcileClusterInstallation) checkMattermostSecret(secretName string, values map[string][]byte, mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	return r.createSecretIfNotExists(mattermost, mattermost.GenerateSecret(secretName, values), reqLogger)
}

func (r *ReconcileClusterInstallation) checkSecret(secretName, namespace string) error {
	foundSecret := &corev1.Secret{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: namespace}, foundSecret)
	if err != nil {
		return errors.Wrap(err, "Error getting secret")
	}

	return nil
}
