// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package v1beta1

import (
	"github.com/pkg/errors"
)

// FileStore utils

// IsValid validates the file store configuration.
// A file store must be explicitly configured; there is no default.
func (fs *FileStore) IsValid() error {
	if fs.IsExternal() || fs.IsExternalVolume() || fs.IsLocal() {
		return nil
	}

	return errors.New("a file store is required: configure one of fileStore.external, fileStore.local or fileStore.externalVolume")
}

// IsExternal returns true if an external S3 compatible file store is configured.
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
