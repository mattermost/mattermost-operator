package mattermost

import (
	"context"

	"github.com/go-logr/logr"
	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	mattermostApp "github.com/mattermost/mattermost-operator/pkg/mattermost"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (r *MattermostReconciler) checkDatabase(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) (mattermostApp.DatabaseConfig, error) {
	reqLogger = reqLogger.WithValues("Reconcile", "database")

	if mattermost.Spec.Database.IsExternal() {
		return r.readExternalDBSecret(mattermost)
	}

	// SetDefaults rejects a Mattermost without an external database, so this is
	// only reachable if that validation and this dispatch fall out of step.
	return nil, errors.New("no database configured: set database.external")
}

func (r *MattermostReconciler) readExternalDBSecret(mattermost *mmv1beta.Mattermost) (mattermostApp.DatabaseConfig, error) {
	secretName := types.NamespacedName{Name: mattermost.Spec.Database.External.Secret, Namespace: mattermost.Namespace}

	var secret corev1.Secret
	err := r.Client.Get(context.TODO(), secretName, &secret)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get external db Secret")
	}

	return mattermostApp.NewExternalDBConfig(mattermost, secret)
}
