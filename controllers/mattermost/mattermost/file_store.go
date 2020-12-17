package mattermost

import (
	"context"
	mattermostMinio "github.com/mattermost/mattermost-operator/pkg/components/minio"
	minioOperator "github.com/minio/minio-operator/pkg/apis/miniocontroller/v1beta1"

	"github.com/go-logr/logr"
	mattermostv1beta1 "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	mattermostApp "github.com/mattermost/mattermost-operator/pkg/mattermost"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (r *MattermostReconciler) checkFileStore(mattermost *mattermostv1beta1.Mattermost, reqLogger logr.Logger) (*mattermostApp.FileStoreInfo, error) {
	reqLogger = reqLogger.WithValues("Reconcile", "fileStore")

	if mattermost.Spec.FileStore.IsExternal() {
		return r.checkExternalFileStore(mattermost, reqLogger)
	}

	return r.checkOperatorManagedMinio(mattermost, reqLogger)
}

func (r *MattermostReconciler) checkExternalFileStore(mattermost *mattermostv1beta1.Mattermost, reqLogger logr.Logger) (*mattermostApp.FileStoreInfo, error) {
	secret := &corev1.Secret{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{Name: mattermost.Spec.FileStore.External.Secret, Namespace: mattermost.Namespace}, secret)
	if err != nil {
		reqLogger.Error(err, "failed to check if external file store secret exists")
		return nil, err
	}

	return mattermostApp.NewExternalFileStoreInfo(mattermost, *secret)
}

func (r *MattermostReconciler) checkOperatorManagedMinio(mattermost *mattermostv1beta1.Mattermost, reqLogger logr.Logger) (*mattermostApp.FileStoreInfo, error) {
	secret, err := r.checkMattermostMinioSecret(mattermost, reqLogger)
	if err != nil {
		return nil, errors.Wrap(err, "failed to check Minio secret")
	}

	err = r.checkMinioInstance(mattermost, reqLogger)
	if err != nil {
		return nil, errors.Wrap(err, "failed to check Minio instance")
	}

	url, err := r.Resources.GetMinioService(mattermost.Name, mattermost.Namespace)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get Minio URL")
	}

	return mattermostApp.NewOperatorManagedFileStoreInfo(mattermost, secret.Name, url), nil
}

func (r *MattermostReconciler) checkMattermostMinioSecret(mattermost *mattermostv1beta1.Mattermost, logger logr.Logger) (*corev1.Secret, error) {
	desired := mattermostMinio.SecretV1Beta(mattermost)
	err := r.Resources.CreateOrUpdateMinioSecret(mattermost, desired, logger)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create or update Minio Secret")
	}
	return desired, nil
}

func (r *MattermostReconciler) checkMinioInstance(mattermost *mattermostv1beta1.Mattermost, reqLogger logr.Logger) error {
	desired := mattermostMinio.InstanceV1Beta(mattermost)

	err := r.Resources.CreateMinioInstanceIfNotExists(mattermost, desired, reqLogger)
	if err != nil {
		return err
	}

	current := &minioOperator.MinIOInstance{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, current)
	if err != nil {
		return err
	}

	// Note:
	// For some reason, our current minio operator seems to remove labels on
	// the instance resource when we add them. For that reason, trying to
	// ensure the labels are correct doesn't work.
	return r.Resources.Update(current, desired, reqLogger)
}
