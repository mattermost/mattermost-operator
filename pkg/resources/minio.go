package resources

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	minioComponent "github.com/mattermost/mattermost-operator/pkg/components/minio"
	minioOperator "github.com/minio/operator/pkg/apis/minio.min.io/v2"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (r *ResourceHelper) CreateMinioTenantIfNotExists(owner v1.Object, tenant *minioOperator.Tenant, logger logr.Logger) error {
	foundInstance := &minioOperator.Tenant{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: tenant.Name, Namespace: tenant.Namespace}, foundInstance)
	if err != nil && kerrors.IsNotFound(err) {
		logger.Info("Creating minio instance")
		return r.Create(owner, tenant, logger)
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
	if err := r.ValidateMinioSecret(current, logger); err != nil {
		logger.Info("minio secret validation error", "name", desired.Name, "error", err)
		return r.Update(current, desired, logger)
	}

	// Preserve data fields
	desired.Data = current.Data
	return r.Update(current, desired, logger)
}

func (r *ResourceHelper) ValidateMinioSecret(secret *corev1.Secret, logger logr.Logger) error {
	// Validate custom secret required fields
	if _, ok := secret.Data["config.env"]; !ok {
		return fmt.Errorf("custom minio Secret %s/%s does not have an 'config.env' key", secret.Namespace, secret.Name)
	}

	if len(secret.Data["config.env"]) == 0 {
		return fmt.Errorf("custom minio Secret %s/%s 'config.env' value is empty", secret.Namespace, secret.Name)
	}

	// Validate keys used to connect and run commands internally
	if _, ok := secret.Data["accesskey"]; !ok {
		return fmt.Errorf("custom minio Secret %s/%s 'accesskey' value is not present", secret.Namespace, secret.Name)
	}
	if len(secret.Data["accesskey"]) == 0 {
		return fmt.Errorf("custom minio Secret %s/%s 'accesskey' value is empty", secret.Namespace, secret.Name)
	}

	if _, ok := secret.Data["secretkey"]; !ok {
		return fmt.Errorf("custom minio Secret %s/%s 'secretkey' value is not present", secret.Namespace, secret.Name)
	}
	if len(secret.Data["secretkey"]) == 0 {
		return fmt.Errorf("custom minio Secret %s/%s 'secretkey' value is empty", secret.Namespace, secret.Name)
	}
	return nil
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
	minioServiceName := fmt.Sprintf("%s%s%s", mmName, minioComponent.MinioNameSuffix, minioOperator.MinIOHLSvcNameSuffix)
	minioService := &corev1.Service{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: minioServiceName, Namespace: mmNamespace}, minioService)
	if err != nil {
		return "", err
	}

	connectionString := fmt.Sprintf("%s.%s.svc.cluster.local:%d", minioService.Name, mmNamespace, minioService.Spec.Ports[0].Port)
	return connectionString, nil
}
