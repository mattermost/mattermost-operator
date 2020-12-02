// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package v1beta1

import "github.com/mattermost/mattermost-operator/pkg/utils"

// FileStore utils

// SetDefaults sets the missing values in FileStore to the default ones.
func (fs *FileStore) SetDefaults() {
	if fs.IsExternal() {
		return
	}

	fs.ensureDefault()
	fs.OperatorManaged.SetDefaults()
}

// IsExternal returns true if the MinIO/S3 instance is external.
func (fs *FileStore) IsExternal() bool {
	return fs.External != nil && fs.External.URL != ""
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
	if fs.IsExternal() {
		return
	}
	fs.ensureDefault()
	fs.OperatorManaged.SetDefaultReplicasAndResources()
}

func (omm *OperatorManagedMinio) SetDefaultReplicasAndResources() {
	if omm.Replicas == nil {
		omm.Replicas = &defaultSize.Minio.Replicas
	}
	if omm.Resources.Size() == 0 {
		omm.Resources = defaultSize.Minio.Resources
	}
}

func (fs *FileStore) OverrideReplicasAndResourcesFromSize(size MattermostSize) {
	if fs.IsExternal() {
		return
	}
	fs.ensureDefault()
	fs.OperatorManaged.OverrideReplicasAndResourcesFromSize(size)
}

func (omm *OperatorManagedMinio) OverrideReplicasAndResourcesFromSize(size MattermostSize) {
	omm.Replicas = utils.NewInt32(size.Minio.Replicas)
	omm.Resources = size.Minio.Resources
}
