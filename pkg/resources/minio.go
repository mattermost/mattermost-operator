package resources

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	minioOperator "github.com/minio/minio-operator/pkg/apis/miniocontroller/v1beta1"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (r *ResourceHelper) CreateMinioInstanceIfNotExists(owner v1.Object, instance *minioOperator.MinIOInstance, logger logr.Logger) error {
	foundInstance := &minioOperator.MinIOInstance{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, foundInstance)
	if err != nil && kerrors.IsNotFound(err) {
		logger.Info("Creating minio instance")
		return r.Create(owner, instance, logger)
	} else if err != nil {
		logger.Error(err, "Unable to get minio instance")
		return err
	}

	return nil
}

func (r *ResourceHelper) CreateOrUpdateMinioSecret(owner v1.Object, desired *corev1.Secret, logger logr.Logger) error {
	current := &corev1.Secret{}

	err := r.client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, current)
	if err != nil {
		if kerrors.IsNotFound(err) {
			return r.createMinioSecret(owner, desired, logger)
		}
		logger.Error(err, "failed to check if secret exists")
		return errors.Wrap(err, "failed to check Minio Secret")
	}

	// Validate secret required fields, if not exist recreate.
	if _, ok := current.Data["accesskey"]; !ok {
		logger.Info("minio secret does not have an 'accesskey' value, overriding", "name", desired.Name)
		return r.Update(current, desired, logger)
	}
	if _, ok := current.Data["secretkey"]; !ok {
		logger.Info("minio secret does not have an 'secretkey' value, overriding", "name", desired.Name)
		return r.Update(current, desired, logger)
	}
	// Preserve data fields
	desired.Data = current.Data
	return r.Update(current, desired, logger)
}

func (r *ResourceHelper) createMinioSecret(owner v1.Object, desired *corev1.Secret, logger logr.Logger) error {
	logger.Info("creating minio secret", "name", desired.Name, "namespace", desired.Namespace)
	err := r.Create(owner, desired, logger)
	if err != nil {
		return errors.Wrap(err, "failed to create Minio secret")
	}

	return nil
}

func (r *ResourceHelper) GetMinioService(mmName, mmNamespace string) (string, error) {
	minioServiceName := fmt.Sprintf("%s-minio-hl-svc", mmName)
	minioService := &corev1.Service{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: minioServiceName, Namespace: mmNamespace}, minioService)
	if err != nil {
		return "", err
	}

	connectionString := fmt.Sprintf("%s.%s.svc.cluster.local:%d", minioService.Name, mmNamespace, minioService.Spec.Ports[0].Port)
	return connectionString, nil
}
