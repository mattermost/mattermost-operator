package clusterinstallation

import (
	"context"

	corev1 "k8s.io/api/core/v1"

	"github.com/go-logr/logr"
	"github.com/mattermost/mattermost-operator/pkg/database"
	mysqlv1alpha1 "github.com/mattermost/mattermost-operator/pkg/database/mysql_operator/v1alpha1"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/apis/mattermost/v1alpha1"
	mattermostmysql "github.com/mattermost/mattermost-operator/pkg/components/mysql"
)

func (r *ClusterInstallationReconciler) checkMySQLCluster(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	reqLogger = reqLogger.WithValues("Reconcile", "mysql")
	desired := mattermostmysql.Cluster(mattermost)

	err := r.Resources.CreateMySQLClusterIfNotExists(mattermost, desired, reqLogger)
	if err != nil {
		return err
	}

	current := &mysqlv1alpha1.MysqlCluster{}
	if err := r.Client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, current); err != nil {
		return err
	}

	return r.Resources.Update(current, desired, reqLogger)
}

func (r *ClusterInstallationReconciler) getOrCreateMySQLSecrets(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) (*database.Info, error) {
	var err error
	dbSecret := &corev1.Secret{}
	dbInfo := &database.Info{}
	dbSecretName := mattermostmysql.DefaultDatabaseSecretName(mattermost.Name)

	if mattermost.Spec.Database.Secret != "" {
		dbSecretName = mattermost.Spec.Database.Secret

		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: dbSecretName, Namespace: mattermost.Namespace}, dbSecret)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get custom database secret")
		}

		dbInfo = database.GenerateDatabaseInfoFromSecret(dbSecret)

		return dbInfo, dbInfo.IsValid()
	}

	dbSecret, err = r.Resources.GetOrCreateMySQLSecrets(mattermost, dbSecretName, reqLogger)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get or create MySQL secret")
	}
	dbInfo = database.GenerateDatabaseInfoFromSecret(dbSecret)

	return dbInfo, dbInfo.IsValid()
}
