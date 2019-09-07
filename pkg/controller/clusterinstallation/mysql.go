package clusterinstallation

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	mysqlOperator "github.com/presslabs/mysql-operator/pkg/apis/mysql/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
	mattermostmysql "github.com/mattermost/mattermost-operator/pkg/components/mysql"
	"github.com/mattermost/mattermost-operator/pkg/components/utils"
)

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

	return r.update(current, desired, reqLogger)
}

func (r *ReconcileClusterInstallation) createMySQLClusterIfNotExists(mattermost *mattermostv1alpha1.ClusterInstallation, cluster *mysqlOperator.MysqlCluster, reqLogger logr.Logger) error {
	foundCluster := &mysqlOperator.MysqlCluster{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, foundCluster)
	if err != nil && errors.IsNotFound(err) {
		reqLogger.Info("Creating mysql cluster")
		return r.create(mattermost, cluster, reqLogger)
	} else if err != nil {
		reqLogger.Error(err, "Failed to check if mysql cluster exists")
		return err
	}

	return nil
}

func (r *ReconcileClusterInstallation) getOrCreateMySQLSecrets(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) (string, error) {
	dbSecretName := fmt.Sprintf("%s-mysql-root-password", mattermost.Name)
	dbSecret := &corev1.Secret{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: dbSecretName, Namespace: mattermost.Namespace}, dbSecret)
	if err != nil && errors.IsNotFound(err) {
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

		return userPassword, r.create(mattermost, dbSecret, reqLogger)
	} else if err != nil {
		reqLogger.Error(err, "Failed to check if mysql secret exists")
		return "", err
	}
	return string(dbSecret.Data["PASSWORD"]), nil
}
