// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package v1beta1

import (
	"github.com/mattermost/mattermost-operator/pkg/utils"
	"github.com/pkg/errors"
)

// For now we reuse sizes from ClusterInstallation to make transition easier.

// SetReplicasAndResourcesFromSize will use the Size field to determine the number of replicas
// and resource requests to set for a ClusterInstallation. If the Size field is not set, values for default size will be used.
// Setting Size to new value will override current values for Replicas and Resources.
// The Size field is erased after adjusting the values.
func (mm *Mattermost) SetReplicasAndResourcesFromSize() error {
	if mm.Spec.Size == "" {
		mm.setDefaultReplicasAndResources()
		return nil
	}

	size, err := GetMattermostSize(mm.Spec.Size)
	if err != nil {
		err = errors.Wrap(err, "using default")
		mm.setDefaultReplicasAndResources()
		return err
	}

	mm.overrideReplicasAndResourcesFromSize(size)

	return nil
}

// The size presets are package-level values shared by every Mattermost, so both
// functions below must copy out of them rather than alias them. Taking the address
// of a preset field, or assigning a ResourceRequirements (whose ResourceLists are
// maps), would let one resource's replica count or resource requests mutate the
// preset itself and leak into every Mattermost reconciled afterwards.

func (mm *Mattermost) setDefaultReplicasAndResources() {
	mm.Spec.Size = ""

	if mm.Spec.Replicas == nil {
		mm.Spec.Replicas = new(DefaultSize.App.Replicas)
	}
	if mm.Spec.Scheduling.Resources.Size() == 0 {
		mm.Spec.Scheduling.Resources = *DefaultSize.App.Resources.DeepCopy()
	}
}

func (mm *Mattermost) overrideReplicasAndResourcesFromSize(size Size) {
	mm.Spec.Size = ""

	mm.Spec.Replicas = utils.NewInt32(size.App.Replicas)
	mm.Spec.Scheduling.Resources = *size.App.Resources.DeepCopy()
}
