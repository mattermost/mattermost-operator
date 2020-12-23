package resources

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/mattermost/mattermost-operator/pkg/components/utils"
	"github.com/pkg/errors"
	mysqlOperator "github.com/presslabs/mysql-operator/pkg/apis/mysql/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (r *ResourceHelper) CreateMySQLClusterIfNotExists(owner v1.Object, cluster *mysqlOperator.MysqlCluster, reqLogger logr.Logger) error {
	foundCluster := &mysqlOperator.MysqlCluster{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, foundCluster)
	if err != nil && k8sErrors.IsNotFound(err) {
		reqLogger.Info("Creating mysql cluster")
		return r.Create(owner, cluster, reqLogger)
	} else if err != nil {
		reqLogger.Error(err, "Failed to check if mysql cluster exists")
		return err
	}

	return nil
}

func (r *ResourceHelper) GetOrCreateMySQLSecrets(owner v1.Object, name string, reqLogger logr.Logger) (*corev1.Secret, error) {
	var err error
	dbSecret := &corev1.Secret{}

	err = r.client.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: owner.GetNamespace()}, dbSecret)
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			return r.createMySQLSecret(owner, name, reqLogger)
		}

		reqLogger.Error(err, "failed to check if mysql secret exists")
		return nil, err
	}

	return dbSecret, nil
}

func (r *ResourceHelper) createMySQLSecret(owner v1.Object, secretName string, reqLogger logr.Logger) (*corev1.Secret, error) {
	reqLogger.Info("Creating new mysql secret")

	dbSecret := &corev1.Secret{}

	dbSecret.SetName(secretName)
	dbSecret.SetNamespace(owner.GetNamespace())
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

	err := r.Create(owner, dbSecret, reqLogger)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create mysql secret")
	}

	return dbSecret, nil
}
