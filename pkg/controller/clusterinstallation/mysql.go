package clusterinstallation

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	mysqlOperator "github.com/oracle/mysql-operator/pkg/apis/mysql/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/rbac/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
	mattermostmysql "github.com/mattermost/mattermost-operator/pkg/components/mysql"
)

func (r *ReconcileClusterInstallation) checkMySQL(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	reqLogger = reqLogger.WithValues("Reconcile", "mysql")

	err := r.checkMySQLServiceAccount(mattermost, reqLogger)
	if err != nil {
		return err
	}

	err = r.checkMySQLRoleBinding(mattermost, reqLogger)
	if err != nil {
		return err
	}

	return r.checkMySQLCluster(mattermost, reqLogger)
}

func (r *ReconcileClusterInstallation) checkMySQLServiceAccount(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	serviceAccount := mattermostmysql.ServiceAccount(mattermost)

	err := r.createServiceAccountIfNotExists(mattermost, serviceAccount, reqLogger)
	if err != nil {
		return err
	}

	foundServiceAccount := &corev1.ServiceAccount{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: serviceAccount.Name, Namespace: serviceAccount.Namespace}, foundServiceAccount)
	if err != nil {
		return err
	}

	updatedLabels := ensureLabels(serviceAccount.Labels, foundServiceAccount.Labels)
	if !reflect.DeepEqual(updatedLabels, foundServiceAccount.Labels) {
		reqLogger.Info("Updating mysql service account labels")
		foundServiceAccount.Labels = updatedLabels
		return r.client.Update(context.TODO(), foundServiceAccount)
	}

	return nil
}

func (r *ReconcileClusterInstallation) checkMySQLRoleBinding(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	roleBinding := mattermostmysql.RoleBinding(mattermost)

	err := r.createRoleBindingIfNotExists(mattermost, roleBinding, reqLogger)
	if err != nil {
		return err
	}

	foundRoleBinding := &v1beta1.RoleBinding{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: roleBinding.Name, Namespace: roleBinding.Namespace}, foundRoleBinding)
	if err != nil {
		return err
	}

	updatedLabels := ensureLabels(roleBinding.Labels, foundRoleBinding.Labels)
	if !reflect.DeepEqual(updatedLabels, foundRoleBinding.Labels) {
		reqLogger.Info("Updating mysql role binding labels")
		foundRoleBinding.Labels = updatedLabels
		return r.client.Update(context.TODO(), foundRoleBinding)
	}

	return nil
}

func (r *ReconcileClusterInstallation) checkMySQLCluster(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	cluster := mattermostmysql.Cluster(mattermost)

	err := r.createMySQLClusterIfNotExists(mattermost, cluster, reqLogger)
	if err != nil {
		return err
	}

	foundCluster := &mysqlOperator.Cluster{}
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

func (r *ReconcileClusterInstallation) createMySQLClusterIfNotExists(mattermost *mattermostv1alpha1.ClusterInstallation, cluster *mysqlOperator.Cluster, reqLogger logr.Logger) error {
	foundCluster := &mysqlOperator.Cluster{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, foundCluster)
	if err != nil && errors.IsNotFound(err) {
		reqLogger.Info("Creating mysql cluster")
		return r.createResource(mattermost, cluster, reqLogger)
	} else if err != nil {
		reqLogger.Error(err, "Failed to check if mysql cluster exists")
		return err
	}

	return nil
}

func (r *ReconcileClusterInstallation) getMySQLSecrets(mattermost *mattermostv1alpha1.ClusterInstallation) (string, error) {
	dbSecretName := fmt.Sprintf("%s-mysql-root-password", mattermost.Name)
	dbSecret := &corev1.Secret{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: dbSecretName, Namespace: mattermost.Namespace}, dbSecret)
	if err != nil {
		return "", err
	}
	return string(dbSecret.Data["password"]), nil
}
