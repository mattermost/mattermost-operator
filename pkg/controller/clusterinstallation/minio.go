package clusterinstallation

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
	mattermostMinio "github.com/mattermost/mattermost-operator/pkg/components/minio"

	minioOperator "github.com/minio/minio-operator/pkg/apis/miniocontroller/v1beta1"
)

func (r *ReconcileClusterInstallation) checkMinio(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	reqLogger = reqLogger.WithValues("Reconcile", "minio")

	err := r.checkMinioSecret(mattermost, reqLogger)
	if err != nil {
		return err
	}

	if mattermost.Spec.Minio.ExternalURL != "" {
		return nil
	}

	return r.checkMinioInstance(mattermost, reqLogger)
}

func (r *ReconcileClusterInstallation) checkCustomMinioSecret(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	secret := &corev1.Secret{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: mattermost.Spec.Minio.Secret, Namespace: mattermost.Namespace}, secret)
	if err != nil {
		reqLogger.Error(err, "failed to check if custom minio secret exists")
		return err
	}
	// Validate custom secret required fields
	if _, ok := secret.Data["accesskey"]; !ok {
		return fmt.Errorf("custom Minio Secret %s does not have an 'accesskey' value", mattermost.Spec.Minio.Secret)
	}
	if _, ok := secret.Data["secretkey"]; !ok {
		return fmt.Errorf("custom Minio Secret %s does not have an 'secretkey' value", mattermost.Spec.Minio.Secret)
	}
	return nil
}

func (r *ReconcileClusterInstallation) checkMattermostMinioSecret(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	current := &corev1.Secret{}
	desired := mattermostMinio.Secret(mattermost)
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, current)
	switch {
	case err != nil && kerrors.IsNotFound(err):
		// Create new secret
		reqLogger.Info("creating minio secret", "name", desired.Name, "namespace", desired.Namespace)
		return r.create(mattermost, desired, reqLogger)
	case err != nil:
		// Something go wrong badly
		reqLogger.Error(err, "failed to check if secret exists")
		return err
	}
	// Validate secret required fields, if not exist recreate.
	if _, ok := current.Data["accesskey"]; !ok {
		reqLogger.Info("minio secret does not have an 'accesskey' value, overriding", "name", desired.Name)
		return r.update(current, desired, reqLogger)
	}
	if _, ok := current.Data["secretkey"]; !ok {
		reqLogger.Info("minio secret does not have an 'secretkey' value, overriding", "name", desired.Name)
		return r.update(current, desired, reqLogger)
	}
	// Preserve data fields
	desired.Data = current.Data
	return r.update(current, desired, reqLogger)
}

func (r *ReconcileClusterInstallation) checkMinioSecret(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	if mattermost.Spec.Minio.Secret != "" {
		reqLogger.Info("skipping minio secret creation, using custom secret")
		return r.checkCustomMinioSecret(mattermost, reqLogger)
	}
	return r.checkMattermostMinioSecret(mattermost, reqLogger)
}

func (r *ReconcileClusterInstallation) checkMinioInstance(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	desired := mattermostMinio.Instance(mattermost)

	err := r.createMinioInstanceIfNotExists(mattermost, desired, reqLogger)
	if err != nil {
		return err
	}

	current := &minioOperator.MinIOInstance{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, current)
	if err != nil {
		return err
	}

	// Note:
	// For some reason, our current minio operator seems to remove labels on
	// the instance resource when we add them. For that reason, trying to
	// ensure the labels are correct doesn't work.
	return r.update(current, desired, reqLogger)
}

func (r *ReconcileClusterInstallation) createMinioInstanceIfNotExists(mattermost *mattermostv1alpha1.ClusterInstallation, instance *minioOperator.MinIOInstance, reqLogger logr.Logger) error {
	foundInstance := &minioOperator.MinIOInstance{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, foundInstance)
	if err != nil && kerrors.IsNotFound(err) {
		reqLogger.Info("Creating minio instance")
		return r.create(mattermost, instance, reqLogger)
	} else if err != nil {
		reqLogger.Error(err, "Unable to get minio instance")
		return err
	}

	return nil
}

func (r *ReconcileClusterInstallation) getMinioService(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) (string, error) {
	minioServiceName := fmt.Sprintf("%s-minio", mattermost.Name)
	minioService := &corev1.Service{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: minioServiceName, Namespace: mattermost.Namespace}, minioService)
	if err != nil {
		return "", err
	}

	connectionString := fmt.Sprintf("%s.%s.svc.cluster.local:%d", minioService.Name, mattermost.Namespace, minioService.Spec.Ports[0].Port)
	return connectionString, nil
}
