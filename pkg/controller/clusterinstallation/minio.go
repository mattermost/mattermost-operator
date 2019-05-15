package clusterinstallation

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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
	return r.createSecretIfNotExists(mattermost, mattermostMinio.MinioSecret(mattermost), reqLogger)
}

func (r *ReconcileClusterInstallation) checkMinioInstance(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	instance := mattermostMinio.MinioInstance(mattermost)

	err := r.createMinioInstanceIfNotExists(mattermost, instance, reqLogger)
	if err != nil {
		return err
	}

	foundInstance := &minioOperator.MinioInstance{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, foundInstance)
	if err != nil {
		return err
	}

	if !reflect.DeepEqual(instance.Spec, foundInstance.Spec) {
		reqLogger.Info("Updating Minio instance")
		foundInstance.Spec = instance.Spec
		return r.client.Update(context.TODO(), foundInstance)
	}

	return nil
}

func (r *ReconcileClusterInstallation) createMinioInstanceIfNotExists(mattermost *mattermostv1alpha1.ClusterInstallation, instance *minioOperator.MinioInstance, reqLogger logr.Logger) error {
	foundInstance := &minioOperator.MinioInstance{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, foundInstance)
	if err != nil && errors.IsNotFound(err) {
		reqLogger.Info("Creating minio instance")
		return r.createResource(mattermost, instance, reqLogger)
	} else if err != nil {
		reqLogger.Error(err, "ClusterInstallation Minio")
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
