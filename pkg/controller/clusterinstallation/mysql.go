package clusterinstallation

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/mattermost/mattermost-operator/pkg/components/utils"
	"github.com/pkg/errors"
	mysqlOperator "github.com/presslabs/mysql-operator/pkg/apis/mysql/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
	mattermostmysql "github.com/mattermost/mattermost-operator/pkg/components/mysql"
)

type databaseInfo struct {
	rootPassword string
	userName     string
	userPassword string
	dbName       string
}

func (db *databaseInfo) IsValid() error {
	if db.rootPassword == "" {
		return errors.New("database root password shouldn't be empty")
	}
	if db.userName == "" {
		return errors.New("database username shouldn't be empty")
	}
	if db.userPassword == "" {
		return errors.New("database password shouldn't be empty")
	}
	if db.dbName == "" {
		return errors.New("database name shouldn't be empty")
	}

	return nil
}

func getDatabaseInfoFromSecret(secret *corev1.Secret) *databaseInfo {
	return &databaseInfo{
		rootPassword: string(secret.Data["ROOT_PASSWORD"]),
		userName:     string(secret.Data["USER"]),
		userPassword: string(secret.Data["PASSWORD"]),
		dbName:       string(secret.Data["DATABASE"]),
	}
}

func (r *ReconcileClusterInstallation) checkMySQLCluster(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	reqLogger = reqLogger.WithValues("Reconcile", "mysql")
	desired := mattermostmysql.Cluster(mattermost)

	err := r.createMySQLClusterIfNotExists(mattermost, desired, reqLogger)
	if err != nil {
		return err
	}

	current := &mysqlOperator.MysqlCluster{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, current); err != nil {
		return err
	}

	return r.update(current, desired, reqLogger)
}

func (r *ReconcileClusterInstallation) createMySQLClusterIfNotExists(mattermost *mattermostv1alpha1.ClusterInstallation, cluster *mysqlOperator.MysqlCluster, reqLogger logr.Logger) error {
	foundCluster := &mysqlOperator.MysqlCluster{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, foundCluster)
	if err != nil && k8sErrors.IsNotFound(err) {
		reqLogger.Info("Creating mysql cluster")
		return r.create(mattermost, cluster, reqLogger)
	} else if err != nil {
		reqLogger.Error(err, "Failed to check if mysql cluster exists")
		return err
	}

	return nil
}

func (r *ReconcileClusterInstallation) getOrCreateMySQLSecrets(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) (*databaseInfo, error) {
	var err error
	dbSecret := &corev1.Secret{}
	dbInfo := &databaseInfo{}

	dbSecretName := mattermostmysql.DefaultDatabaseSecretName(mattermost.Name)

	if mattermost.Spec.Database.Secret != "" {
		dbSecretName = mattermost.Spec.Database.Secret

		err = r.client.Get(context.TODO(), types.NamespacedName{Name: dbSecretName, Namespace: mattermost.Namespace}, dbSecret)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get custom database secret")
		}

		dbInfo = getDatabaseInfoFromSecret(dbSecret)

		return dbInfo, dbInfo.IsValid()
	}

	err = r.client.Get(context.TODO(), types.NamespacedName{Name: dbSecretName, Namespace: mattermost.Namespace}, dbSecret)
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

		dbInfo = getDatabaseInfoFromSecret(dbSecret)

		return dbInfo, dbInfo.IsValid()
	} else if err != nil {
		reqLogger.Error(err, "failed to check if mysql secret exists")
		return nil, err
	}

	dbInfo = getDatabaseInfoFromSecret(dbSecret)

	return dbInfo, dbInfo.IsValid()
}
