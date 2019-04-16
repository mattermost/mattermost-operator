package clusterinstallation

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	mysqlOperator "github.com/oracle/mysql-operator/pkg/apis/mysql/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
	mattermostmysql "github.com/mattermost/mattermost-operator/pkg/components/mysql"
)

func (r *ReconcileClusterInstallation) checkMySQLServiceAccount(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	return r.createServiceAccountIfNotExists(mattermost, mattermostmysql.ServiceAccount(mattermost), reqLogger)
}

func (r *ReconcileClusterInstallation) checkMySQLRoleBinding(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	return r.createRoleBindingIfNotExists(mattermost, mattermostmysql.RoleBinding(mattermost), reqLogger)
}

func (r *ReconcileClusterInstallation) createMySQLDeployment(mattermost *mattermostv1alpha1.ClusterInstallation, deployment *mysqlOperator.Cluster, reqLogger logr.Logger) error {
	reqLogger.Info("Creating MySQL deployment", "Members", deployment.Spec.Members)
	if err := r.client.Create(context.TODO(), deployment); err != nil {
		reqLogger.Info("Error creating MySQL deployment", "Error", err.Error())
		return err
	}
	reqLogger.Info("Completed creating MySQL deployment")
	if err := controllerutil.SetControllerReference(mattermost, deployment, r.scheme); err != nil {
		return err
	}

	// TODO compare found deployment versus expected

	return nil
}

func (r *ReconcileClusterInstallation) createMySQLDeploymentIfNotExists(mattermost *mattermostv1alpha1.ClusterInstallation, deployment *mysqlOperator.Cluster, reqLogger logr.Logger) error {
	foundDeployment := &mysqlOperator.Cluster{}
	errGet := r.client.Get(context.TODO(), types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, foundDeployment)
	if errGet != nil && errors.IsNotFound(errGet) {
		return r.createMySQLDeployment(mattermost, deployment, reqLogger)
	} else if errGet != nil {
		reqLogger.Error(errGet, "ClusterInstallation Database")
		return errGet
	}

	// Set MattermostService instance as the owner and controller
	if !reflect.DeepEqual(deployment.Spec, deployment.Spec) {
		foundDeployment.Spec = deployment.Spec
		reqLogger.Info("Updating MySQL deployment", deployment.Namespace, deployment.Name)
		err := r.client.Update(context.TODO(), foundDeployment)
		if err != nil {
			return err
		}
		_ = controllerutil.SetControllerReference(mattermost, foundDeployment, r.scheme)
	}

	return nil
}

func (r *ReconcileClusterInstallation) checkMySQLDeployment(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	return r.createMySQLDeploymentIfNotExists(mattermost, mattermostmysql.Cluster(mattermost), reqLogger)
}

func (r *ReconcileClusterInstallation) getMySQLSecrets(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) (string, error) {
	dbSecretName := fmt.Sprintf("%s-mysql-root-password", mattermost.Name)
	dbSecret := &corev1.Secret{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: dbSecretName, Namespace: mattermost.Namespace}, dbSecret)
	if err != nil {
		return "", err
	}
	return string(dbSecret.Data["password"]), nil
}
