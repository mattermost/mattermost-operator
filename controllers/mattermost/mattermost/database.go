package mattermost

import (
	"context"
	"fmt"

	mattermostmysql "github.com/mattermost/mattermost-operator/pkg/components/mysql"
	mysqlv1alpha1 "github.com/mattermost/mattermost-operator/pkg/database/mysql_operator/v1alpha1"

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

	return r.checkOperatorManagedDB(mattermost, reqLogger)
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

func (r *MattermostReconciler) checkOperatorManagedDB(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) (mattermostApp.DatabaseConfig, error) {
	if mattermost.Spec.Database.OperatorManaged == nil {
		return nil, fmt.Errorf("configuration for Operator managed database not provided")
	}

	switch mattermost.Spec.Database.OperatorManaged.Type {
	case "mysql":
		return r.checkOperatorManagedMySQL(mattermost, reqLogger)
	case "postgres":
		return nil, errors.New("database type 'postgres' not yet implemented")
	}

	return nil, fmt.Errorf("database of type '%s' is not supported", mattermost.Spec.Database.OperatorManaged.Type)
}

func (r *MattermostReconciler) checkOperatorManagedMySQL(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) (mattermostApp.DatabaseConfig, error) {
	reqLogger = reqLogger.WithValues("Reconcile", "mysql")

	err := r.checkMySQLCluster(mattermost, reqLogger)
	if err != nil {
		return nil, errors.Wrap(err, "error while checking MySQL cluster")
	}

	dbSecretName := mattermostmysql.DefaultDatabaseSecretName(mattermost.Name)

	dbSecret, err := r.Resources.GetOrCreateMySQLSecrets(mattermost, dbSecretName, reqLogger)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get or create MySQL database secret")
	}

	return mattermostApp.NewMySQLDBConfig(*dbSecret)
}

func (r *MattermostReconciler) checkMySQLCluster(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) error {
	desired := mattermostmysql.ClusterV1Beta(mattermost)

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
