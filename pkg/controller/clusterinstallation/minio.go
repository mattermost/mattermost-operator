package clusterinstallation

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
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

	return r.checkMinioInstance(mattermost, reqLogger)
}

func (r *ReconcileClusterInstallation) checkMinioSecret(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	// Check if custom secret was specified
	if mattermost.Spec.Minio.Secret != "" {
		// Check if the Secret exists
		var secret *corev1.Secret
		err := r.client.Get(context.TODO(), types.NamespacedName{Name: mattermost.Spec.Minio.Secret, Namespace: mattermost.Namespace}, secret)
		if err != nil {
			// Secret does not exist
			return errors.Wrap(err, "unable to locate custom minio secret")
		}

		// Check if the Secret has required fields
		if _, ok := secret.Data["accesskey"]; !ok {
			return fmt.Errorf("custom Minio Secret %s does not have an 'accesskey' value", mattermost.Spec.Minio.Secret)
		}
		if _, ok := secret.Data["secretkey"]; !ok {
			return fmt.Errorf("custom Minio Secret %s does not have an 'secretkey' value", mattermost.Spec.Minio.Secret)
		}

		reqLogger.Info("Skipping minio secret creation, using custom secret")
		return nil
	}

	secret := mattermostMinio.Secret(mattermost)

	err := r.createSecretIfNotExists(mattermost, secret, reqLogger)
	if err != nil {
		return err
	}

	foundSecret := &corev1.Secret{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, foundSecret)
	if err != nil {
		return err
	}

	updatedLabels := ensureLabels(secret.Labels, foundSecret.Labels)
	if !reflect.DeepEqual(updatedLabels, foundSecret.Labels) {
		reqLogger.Info("Updating minio secret labels")
		foundSecret.Labels = updatedLabels
		return r.client.Update(context.TODO(), foundSecret)
	}

	return nil
}

func (r *ReconcileClusterInstallation) checkMinioInstance(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	instance := mattermostMinio.Instance(mattermost)

	err := r.createMinioInstanceIfNotExists(mattermost, instance, reqLogger)
	if err != nil {
		return err
	}

	foundInstance := &minioOperator.MinIOInstance{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, foundInstance)
	if err != nil {
		return err
	}

	var update bool

	// Note:
	// For some reason, our current minio operator seems to remove labels on
	// the instance resource when we add them. For that reason, trying to
	// ensure the labels are correct doesn't work.
	updatedLabels := ensureLabels(instance.Labels, foundInstance.Labels)
	if !reflect.DeepEqual(updatedLabels, foundInstance.Labels) {
		reqLogger.Info("Updating minio instance labels")
		foundInstance.Labels = updatedLabels
		update = true
	}

	if !reflect.DeepEqual(instance.Spec, foundInstance.Spec) {
		reqLogger.Info("Updating minio instance spec")
		foundInstance.Spec = instance.Spec
		update = true
	}

	if update {
		return r.client.Update(context.TODO(), foundInstance)
	}

	return nil
}

func (r *ReconcileClusterInstallation) createMinioInstanceIfNotExists(mattermost *mattermostv1alpha1.ClusterInstallation, instance *minioOperator.MinIOInstance, reqLogger logr.Logger) error {
	foundInstance := &minioOperator.MinIOInstance{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, foundInstance)
	if err != nil && kerrors.IsNotFound(err) {
		reqLogger.Info("Creating minio instance")
		return r.createResource(mattermost, instance, reqLogger)
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
