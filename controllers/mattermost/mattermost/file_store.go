package mattermost

import (
	"context"
	"fmt"
	"slices"

	mattermostMinio "github.com/mattermost/mattermost-operator/pkg/components/minio"
	minioOperator "github.com/minio/minio-operator/pkg/apis/miniocontroller/v1beta1"

	"github.com/go-logr/logr"
	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	mattermostApp "github.com/mattermost/mattermost-operator/pkg/mattermost"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (r *MattermostReconciler) checkFileStore(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) (mattermostApp.FileStoreConfig, error) {
	reqLogger = reqLogger.WithValues("Reconcile", "fileStore")

	if mattermost.Spec.FileStore.IsExternal() {
		return r.checkExternalFileStore(mattermost, reqLogger)
	}

	if mattermost.Spec.FileStore.IsExternalVolume() {
		return r.checkExternalVolumeFileStore(mattermost, reqLogger)
	}

	if mattermost.Spec.FileStore.IsLocal() {
		return r.checkLocalFileStore(mattermost, reqLogger)
	}

	return r.checkOperatorManagedMinio(mattermost, reqLogger)
}

func (r *MattermostReconciler) checkExternalFileStore(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) (mattermostApp.FileStoreConfig, error) {
	if mattermost.Spec.FileStore.External.UseServiceAccount {
		current := &corev1.ServiceAccount{}
		err := r.Client.Get(context.TODO(), types.NamespacedName{Name: mattermost.Name, Namespace: mattermost.Namespace}, current)
		if err != nil && k8sErrors.IsNotFound(err) {
			return nil, errors.Wrap(err, "service account needs to be created manually if fileStore.external.useServiceAccount is true")
		} else if err != nil {
			return nil, errors.Wrap(err, "failed to check if service account exists")
		}

		if _, ok := current.Annotations["eks.amazonaws.com/role-arn"]; !ok {
			return nil, fmt.Errorf(`service account does not have "eks.amazonaws.com/role-arn" annotation, which is required if fileStore.external.useServiceAccount is true`)
		}

		return mattermostApp.NewExternalFileStoreInfo(mattermost, nil)
	}

	secret := &corev1.Secret{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{Name: mattermost.Spec.FileStore.External.Secret, Namespace: mattermost.Namespace}, secret)
	if err != nil {
		reqLogger.Error(err, "failed to check if external file store secret exists")
		return nil, err
	}

	return mattermostApp.NewExternalFileStoreInfo(mattermost, secret)
}

func (r *MattermostReconciler) checkExternalVolumeFileStore(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) (mattermostApp.FileStoreConfig, error) {
	fsc, err := mattermostApp.NewExternalVolumeFileStoreInfo(mattermost)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create external volume FileStoreConfig")
	}

	// Ensure that the PVC exists and is in a valid state.
	pvc := &corev1.PersistentVolumeClaim{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{
		Name:      mattermost.Spec.FileStore.ExternalVolume.VolumeClaimName,
		Namespace: mattermost.Namespace,
	}, pvc)
	if err != nil {
		reqLogger.Error(err, "failed to get specified PVC for external volume storage")
		return nil, err
	}
	if pvc.Status.Phase != corev1.ClaimBound && pvc.Status.Phase != corev1.ClaimPending {
		err := errors.Errorf("specified PVC for external volume storage is not %s or %s (%s)", corev1.ClaimBound, corev1.ClaimPending, pvc.Status.Phase)
		reqLogger.Error(err, "failed checking PVC status")
		return nil, err
	}

	return fsc, nil
}

func (r *MattermostReconciler) checkLocalFileStore(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) (mattermostApp.FileStoreConfig, error) {
	storageSize := mmv1beta.DefaultFilestoreStorageSize
	if mattermost.Spec.FileStore.Local.StorageSize != "" {
		storageSize = mattermost.Spec.FileStore.Local.StorageSize
	}

	// Check if PVC already exists to handle backwards compatibility
	current := &corev1.PersistentVolumeClaim{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{Name: mattermost.Name, Namespace: mattermost.Namespace}, current)
	pvcExists := err == nil

	// Determine access modes based on configuration and backwards compatibility
	accessModes := r.determineAccessModes(mattermost, current, pvcExists, reqLogger)

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mattermost.Name,
			Namespace: mattermost.Namespace,
			Labels:    mattermost.MattermostLabels(mattermost.Name),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: accessModes,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(storageSize),
				},
			},
		},
	}

	// Create PVC if it doesn't exist
	if !pvcExists {
		err = r.Resources.CreatePvcIfNotExists(mattermost, pvc, reqLogger)
		if err != nil {
			reqLogger.Error(err, "failed to create PVC for local storage")
			return nil, err
		}

		// Get the newly created PVC
		err = r.Client.Get(context.TODO(), types.NamespacedName{Name: pvc.Name, Namespace: pvc.Namespace}, current)
		if err != nil {
			reqLogger.Error(err, "failed to get existing PVC for local storage")
			return nil, err
		}
	} else {
		// Check if user is trying to change access modes - this is not supported
		if len(mattermost.Spec.FileStore.Local.AccessModes) > 0 && !r.accessModesEqual(current.Spec.AccessModes, accessModes) {
			return nil, fmt.Errorf("cannot change PVC access modes from %v to %v as this requires PVC recreation which may cause data loss. Please manually migrate data and recreate the PVC with a different name if you need to change access modes",
				current.Spec.AccessModes, accessModes)
		}

		// Update PVC to ensure we match the current spec (e.g., storage size changes)
		err = r.Resources.Update(current, pvc, reqLogger)
		if err != nil {
			reqLogger.Error(err, "failed to update PVC for local storage")
			return nil, err
		}
	}

	return mattermostApp.NewLocalFileStoreInfo(), nil
}

// determineAccessModes determines the appropriate access modes for the PVC based on configuration and backwards compatibility
func (r *MattermostReconciler) determineAccessModes(mattermost *mmv1beta.Mattermost, currentPVC *corev1.PersistentVolumeClaim, pvcExists bool, reqLogger logr.Logger) []corev1.PersistentVolumeAccessMode {
	// If user explicitly specified access modes, use them
	if len(mattermost.Spec.FileStore.Local.AccessModes) > 0 {
		return mattermost.Spec.FileStore.Local.AccessModes
	}

	// For backwards compatibility: if PVC already exists, preserve its current access modes
	if pvcExists {
		reqLogger.Info("Preserving existing PVC access modes for backwards compatibility", "accessModes", currentPVC.Spec.AccessModes)
		return currentPVC.Spec.AccessModes
	}

	// For new installations, default to ReadWriteMany
	reqLogger.Info("Using default access mode ReadWriteMany for new PVC")
	return []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}
}

// accessModesEqual compares two slices of access modes for equality
func (r *MattermostReconciler) accessModesEqual(a, b []corev1.PersistentVolumeAccessMode) bool {
	if len(a) != len(b) {
		return false
	}

	// Check if every element in 'a' exists in 'b'
	for _, mode := range a {
		if !slices.Contains(b, mode) {
			return false
		}
	}

	return true
}

func (r *MattermostReconciler) checkOperatorManagedMinio(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) (mattermostApp.FileStoreConfig, error) {
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

func (r *MattermostReconciler) checkMattermostMinioSecret(mattermost *mmv1beta.Mattermost, logger logr.Logger) (*corev1.Secret, error) {
	desired := mattermostMinio.SecretV1Beta(mattermost)
	err := r.Resources.CreateOrUpdateMinioSecret(mattermost, desired, logger)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create or update Minio Secret")
	}
	return desired, nil
}

func (r *MattermostReconciler) checkMinioInstance(mattermost *mmv1beta.Mattermost, reqLogger logr.Logger) error {
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
