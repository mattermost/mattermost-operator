package clusterinstallation

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	mysqlOperator "github.com/presslabs/mysql-operator/pkg/apis/mysql/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
	mattermostmysql "github.com/mattermost/mattermost-operator/pkg/components/mysql"
	"github.com/mattermost/mattermost-operator/pkg/components/utils"
)

type databaseInfo struct {
	userName     string
	userPassword string
	dbName       string
}

func (r *ReconcileClusterInstallation) checkMySQLCluster(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	reqLogger = reqLogger.WithValues("Reconcile", "mysql")
	cluster := mattermostmysql.Cluster(mattermost)

	err := r.createMySQLClusterIfNotExists(mattermost, cluster, reqLogger)
	if err != nil {
		return err
	}

	foundCluster := &mysqlOperator.MysqlCluster{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, foundCluster)
	if err != nil {
		return err
	}

	var update bool

	updatedLabels := ensureLabels(cluster.Labels, foundCluster.Labels)
	if !reflect.DeepEqual(updatedLabels, foundCluster.Labels) {
		reqLogger.Info("Updating mysql cluster labels")
		foundCluster.Labels = updatedLabels
		update = true
	}

	if !reflect.DeepEqual(cluster.Spec, foundCluster.Spec) {
		reqLogger.Info("Updating mysql cluster spec")
		foundCluster.Spec = cluster.Spec
		update = true
	}

	if update {
		return r.client.Update(context.TODO(), foundCluster)
	}

	return nil
}

func (r *ReconcileClusterInstallation) createMySQLClusterIfNotExists(mattermost *mattermostv1alpha1.ClusterInstallation, cluster *mysqlOperator.MysqlCluster, reqLogger logr.Logger) error {
	foundCluster := &mysqlOperator.MysqlCluster{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, foundCluster)
	if err != nil && k8sErrors.IsNotFound(err) {
		reqLogger.Info("Creating mysql cluster")
		return r.createResource(mattermost, cluster, reqLogger)
	} else if err != nil {
		reqLogger.Error(err, "Failed to check if mysql cluster exists")
		return err
	}

	return nil
}

// getOrCreateMySQLSecrets get or create the MySQL secrets used by MySQL Operator to spin the cluster and
// also used by mattermost to get the credentials to use to access the DB.
// dbData is a []string -> { DBUserName, DBUserPassword, DBName }
func (r *ReconcileClusterInstallation) getOrCreateMySQLSecrets(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) (databaseInfo, error) {
	dbSecretName := fmt.Sprintf("%s-mysql-root-password", mattermost.Name)
	dbSecret := &corev1.Secret{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: dbSecretName, Namespace: mattermost.Namespace}, dbSecret)
	if err != nil && k8sErrors.IsNotFound(err) {
		reqLogger.Info("Creating mysql secret")

		dbSecret.SetName(dbSecretName)
		dbSecret.SetNamespace(mattermost.Namespace)
		rootPassword := string(utils.New16ID())
		userPassword := string(utils.New16ID())

		dbSecret.Data = map[string][]byte{
			"ROOT_PASSWORD": []byte(rootPassword),
			"USER":          []byte("mmuser"),
			"PASSWORD":      []byte(userPassword),
			"DATABASE":      []byte("mattermost"),
		}

		dbInfo := databaseInfo{
			userName:     "mmuser",
			userPassword: userPassword,
			dbName:       "mattermost",
		}
		return dbInfo, r.createResource(mattermost, dbSecret, reqLogger)
	} else if err != nil {
		reqLogger.Error(err, "Failed to check if mysql secret exists")
		dbInfo := databaseInfo{
			userName:     "",
			userPassword: "",
			dbName:       "",
		}
		return dbInfo, err
	}

	dbInfo := databaseInfo{
		userName:     string(dbSecret.Data["USER"]),
		userPassword: string(dbSecret.Data["PASSWORD"]),
		dbName:       string(dbSecret.Data["DATABASE"]),
	}

	return dbInfo, nil
}

func (db *databaseInfo) Valid() error {
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
