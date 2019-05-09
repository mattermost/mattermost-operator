package clusterinstallation

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
	mattermostMinio "github.com/mattermost/mattermost-operator/pkg/components/minio"

	minioOperator "github.com/minio/minio-operator/pkg/apis/miniocontroller/v1beta1"
)

func (r *ReconcileClusterInstallation) createMinioDeploymentIfNotExists(mattermost *mattermostv1alpha1.ClusterInstallation, deployment *minioOperator.MinioInstance, reqLogger logr.Logger) error {
	foundDeployment := &minioOperator.MinioInstance{}
	errGet := r.client.Get(context.TODO(), types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, foundDeployment)
	if errGet != nil && errors.IsNotFound(errGet) {
		return r.createResource(mattermost, deployment, reqLogger)
	} else if errGet != nil {
		reqLogger.Error(errGet, "ClusterInstallation Minio")
		return errGet
	}

	if !reflect.DeepEqual(foundDeployment.Spec, deployment.Spec) {
		foundDeployment.Spec = deployment.Spec
		reqLogger.Info("Updating Minio deployment", deployment.Namespace, deployment.Name)
		err := r.client.Update(context.TODO(), foundDeployment)
		if err != nil {
			return err
		}
		_ = controllerutil.SetControllerReference(mattermost, foundDeployment, r.scheme)
	}

	return nil
}

func (r *ReconcileClusterInstallation) checkMinioDeployment(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	return r.createMinioDeploymentIfNotExists(mattermost, mattermostMinio.MinioInstance(mattermost), reqLogger)
}

func (r *ReconcileClusterInstallation) checkMinioSecret(mattermost *mattermostv1alpha1.ClusterInstallation, reqLogger logr.Logger) error {
	return r.createSecretIfNotExists(mattermost, mattermostMinio.MinioSecret(mattermost), reqLogger)
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
