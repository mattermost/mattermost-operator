package clusterinstallation

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/mattermost/mattermost-operator/pkg/components/utils"
	"github.com/mattermost/mattermost-operator/pkg/database"
	"github.com/pkg/errors"
	mysqlOperator "github.com/presslabs/mysql-operator/pkg/apis/mysql/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/apis/mattermost/v1alpha1"
	mattermostmysql "github.com/mattermost/mattermost-operator/pkg/components/mysql"
)

func (r *ClusterInstallationReconciler) checkMySQLCluster(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	reqLogger = reqLogger.WithValues("Reconcile", "mysql")
	desired := mattermostmysql.Cluster(mattermost)

	err := r.createMySQLClusterIfNotExists(mattermost, desired, reqLogger)
	if err != nil {
		return err
	}

	current := &mysqlOperator.MysqlCluster{}
	if err := r.Client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, current); err != nil {
		return err
	}

	return r.update(current, desired, reqLogger)
}

func (r *ClusterInstallationReconciler) createMySQLClusterIfNotExists(mattermost *mattermostv1alpha1.ClusterInstallation, cluster *mysqlOperator.MysqlCluster, reqLogger logr.Logger) error {
	foundCluster := &mysqlOperator.MysqlCluster{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, foundCluster)
	if err != nil && k8sErrors.IsNotFound(err) {
		reqLogger.Info("Creating mysql cluster")
		return r.create(mattermost, cluster, reqLogger)
	} else if err != nil {
		reqLogger.Error(err, "Failed to check if mysql cluster exists")
		return err
	}

	return nil
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

	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: dbSecretName, Namespace: mattermost.Namespace}, dbSecret)
	if err != nil && k8sErrors.IsNotFound(err) {
		reqLogger.Info("Creating new mysql secret")

		dbSecret.SetName(dbSecretName)
		dbSecret.SetNamespace(mattermost.Namespace)
		userName := "mmuser"
		dbName := "mattermost"
		rootPassword := string(utils.New16ID())
		userPassword := string(utils.New16ID())

		dbSecret.Data = map[string][]byte{
			"ROOT_PASSWORD": []byte(rootPassword),
			"USER":          []byte(userName),
			"PASSWORD":      []byte(userPassword),
			"DATABASE":      []byte(dbName),
		}

		err = r.create(mattermost, dbSecret, reqLogger)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create mysql secret")
		}

		dbInfo = database.GenerateDatabaseInfoFromSecret(dbSecret)

		return dbInfo, dbInfo.IsValid()
	} else if err != nil {
		reqLogger.Error(err, "failed to check if mysql secret exists")
		return nil, err
	}

	dbInfo = database.GenerateDatabaseInfoFromSecret(dbSecret)

	return dbInfo, dbInfo.IsValid()
}
