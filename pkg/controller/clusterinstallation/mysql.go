package clusterinstallation

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	mysqlOperator "github.com/presslabs/mysql-operator/pkg/apis/mysql/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
	mattermostmysql "github.com/mattermost/mattermost-operator/pkg/components/mysql"
)

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
	if err != nil && kerrors.IsNotFound(err) {
		reqLogger.Info("Creating mysql cluster")
		return r.createResource(mattermost, cluster, reqLogger)
	} else if err != nil {
		reqLogger.Error(err, "Failed to check if mysql cluster exists")
		return err
	}

	return nil
}

func (r *ReconcileClusterInstallation) getOrCreateMySQLSecrets(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) (string, error) {
	dbSecret := &corev1.Secret{}
	// Check if the custom MySQL secret is provided
	if mattermost.Spec.Database.Secret != "" {
		if err := r.client.Get(context.TODO(), types.NamespacedName{
			Namespace: mattermost.Namespace,
			Name:      mattermost.Spec.Database.Secret,
		}, dbSecret); err != nil {
			return "", errors.Wrap(err, "unable to locate custom MySQL secret")
		}

		if _, ok := dbSecret.Data["USER"]; !ok {
			return "", fmt.Errorf("custom MySQL Secret %s does not have an 'USER' value", mattermost.Spec.Database.Secret)
		}
		if _, ok := dbSecret.Data["DATABASE"]; !ok {
			return "", fmt.Errorf("custom MySQL Secret %s does not have an 'DATABASE' value", mattermost.Spec.Database.Secret)
		}
		if _, ok := dbSecret.Data["ROOT_PASSWORD"]; !ok {
			return "", fmt.Errorf("custom MySQL Secret %s does not have an 'ROOT_PASSWORD' value", mattermost.Spec.Database.Secret)
		}
		userPassword, ok := dbSecret.Data["PASSWORD"]
		if !ok {
			return "", fmt.Errorf("custom MySQL Secret %s does not have an 'PASSWORD' value", mattermost.Spec.Database.Secret)
		}
		reqLogger.Info("Skipping MySQL secret creation, using user provided secret")
		return string(userPassword), nil
	}
	dbSecretName := fmt.Sprintf("%s-mysql-root-password", mattermost.Name)
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: dbSecretName, Namespace: mattermost.Namespace}, dbSecret)
	if err != nil && kerrors.IsNotFound(err) {
		// create a new secret
		dbSecret = mattermostmysql.Secret(mattermost)
		userPassword, ok := dbSecret.Data["PASSWORD"]
		if !ok {
			return "", fmt.Errorf("failed to create MySQL Secret %s does not have an 'PASSWORD' value", mattermost.Spec.Database.Secret)
		}
		return string(userPassword), r.createResource(mattermost, dbSecret, reqLogger)
	} else if err != nil {
		reqLogger.Error(err, "Failed to check if mysql secret exists")
		return "", err
	}
	return string(dbSecret.Data["PASSWORD"]), nil
}
