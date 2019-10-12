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
)

type databaseInfo struct {
	userName     string
	userPassword string
	dbName       string
}

func (r *ReconcileClusterInstallation) checkMySQLCluster(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	reqLogger = reqLogger.WithValues("Reconcile", "mysql")
	desired := mattermostmysql.Cluster(mattermost)

	err := r.createMySQLClusterIfNotExists(mattermost, desired, reqLogger)
	if err != nil {
		return err
	}

	current := &mysqlOperator.MysqlCluster{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, current)
	if err != nil {
		return err
	}

	// Updating mysql.cluster with objectMatcher breaks mysql operator.
	// Only fields that are expected to be changed by mattermost-operator should be included here.
	var update bool

	updatedLabels := ensureLabels(desired.Labels, current.Labels)
	if !reflect.DeepEqual(updatedLabels, current.Labels) {
		reqLogger.Info("Updating mysql cluster labels")
		current.Labels = updatedLabels
		update = true
	}

	if !reflect.DeepEqual(desired.Spec, current.Spec) {
		reqLogger.Info("Updating mysql cluster spec")
		current.Spec = desired.Spec
		update = true
	}

	if update {
		return r.client.Update(context.TODO(), current)
	}

	return nil
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

func (r *ReconcileClusterInstallation) getOrCreateMySQLSecrets(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) (databaseInfo, error) {
	dbSecret := &corev1.Secret{}
	dbInfo := databaseInfo{
		userName:     "",
		userPassword: "",
		dbName:       "",
	}
	// Check if the custom(existing) MySQL secret is provided
	if mattermost.Spec.Database.ExistingSecret != "" {
		if err := r.client.Get(context.TODO(), types.NamespacedName{
			Namespace: mattermost.Namespace,
			Name:      mattermost.Spec.Database.ExistingSecret,
		}, dbSecret); err != nil {
			return dbInfo, errors.Wrap(err, "unable to locate custom/existing MySQL secret")
		}
		dbInfo = databaseInfo{
			userName:     string(dbSecret.Data["USER"]),
			userPassword: string(dbSecret.Data["PASSWORD"]),
			dbName:       string(dbSecret.Data["DATABASE"]),
		}
		return dbInfo, dbInfo.Valid()
	}

	dbSecretName := fmt.Sprintf("%s-mysql-root-password", mattermost.Name)
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: dbSecretName, Namespace: mattermost.Namespace}, dbSecret)
	if err != nil && k8sErrors.IsNotFound(err) {
		// create a new secret
		dbSecret = mattermostmysql.CreateSecret(mattermost)
		dbInfo = databaseInfo{
			userName:     string(dbSecret.Data["USER"]),
			userPassword: string(dbSecret.Data["PASSWORD"]),
			dbName:       string(dbSecret.Data["DATABASE"]),
		}
		return dbInfo, r.create(mattermost, dbSecret, reqLogger)
	} else if err != nil {
		reqLogger.Error(err, "Failed to check if mysql secret exists")
		return dbInfo, err
	}
	dbInfo = databaseInfo{
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
