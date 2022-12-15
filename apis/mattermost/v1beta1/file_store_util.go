// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package v1beta1

import (
	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/apis/mattermost/v1alpha1"
	"github.com/mattermost/mattermost-operator/pkg/utils"
)

// FileStore utils

// SetDefaults sets the missing values in FileStore to the default ones.
func (fs *FileStore) SetDefaults() {
	if fs.isAnyExceptOperatorManaged() {
		return
	}

	fs.ensureDefault()
	fs.OperatorManaged.SetDefaults()
}

// IsExternal returns true if the MinIO/S3 instance is external.
func (fs *FileStore) IsExternal() bool {
	return fs.External != nil && fs.External.URL != ""
}

// IsExternalVolume returns true if the filestore requested is an externally
// managed volume.
func (fs *FileStore) IsExternalVolume() bool {
	return fs.ExternalVolume != nil && fs.ExternalVolume.VolumeClaimName != ""
}

// IsLocal returns true if the filestore requested is local (PVC backed).
func (fs *FileStore) IsLocal() bool {
	return fs.Local != nil && fs.Local.Enabled
}

// isAnyExceptOperatorManaged checks if any filestore types are configurated
// except the operator managed type. This is generally used to see if defaults
// should be applied.
func (fs *FileStore) isAnyExceptOperatorManaged() bool {
	return fs.IsExternal() || fs.IsExternalVolume() || fs.IsLocal()
}

func (fs *FileStore) ensureDefault() {
	if fs.OperatorManaged == nil {
		fs.OperatorManaged = &OperatorManagedMinio{}
	}
}

// SetDefaults sets the missing values in OperatorManagedMinio to the default ones.
func (omm *OperatorManagedMinio) SetDefaults() {
	if omm.StorageSize == "" {
		omm.StorageSize = DefaultFilestoreStorageSize
	}
}

func (fs *FileStore) SetDefaultReplicasAndResources() {
	if fs.isAnyExceptOperatorManaged() {
		return
	}
	fs.ensureDefault()
	fs.OperatorManaged.SetDefaultReplicasAndResources()
}

func (omm *OperatorManagedMinio) SetDefaultReplicasAndResources() {
	if omm.Replicas == nil {
		omm.Replicas = &mattermostv1alpha1.DefaultSize.Minio.Replicas
		omm.Servers = &mattermostv1alpha1.DefaultSize.Minio.Replicas
	} else {
		omm.Servers = omm.Replicas
	}
	if omm.Resources.Size() == 0 {
		omm.Resources = mattermostv1alpha1.DefaultSize.Minio.Resources
	}
	if omm.VolumesPerServer == nil {
		omm.VolumesPerServer = &mattermostv1alpha1.DefaultSize.Minio.VolumesPerReplica
	}
}

func (fs *FileStore) OverrideReplicasAndResourcesFromSize(size mattermostv1alpha1.ClusterInstallationSize) {
	if fs.isAnyExceptOperatorManaged() {
		return
	}
	fs.ensureDefault()
	fs.OperatorManaged.OverrideReplicasAndResourcesFromSize(size)
}

func (omm *OperatorManagedMinio) OverrideReplicasAndResourcesFromSize(size mattermostv1alpha1.ClusterInstallationSize) {
	omm.Replicas = utils.NewInt32(size.Minio.Replicas)
	omm.Resources = size.Minio.Resources
}
