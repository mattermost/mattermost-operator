// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package v1beta1

import (
	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/apis/mattermost/v1alpha1"
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

	size, err := mattermostv1alpha1.GetClusterSize(mm.Spec.Size)
	if err != nil {
		err = errors.Wrap(err, "using default")
		mm.setDefaultReplicasAndResources()
		return err
	}

	mm.overrideReplicasAndResourcesFromSize(size)

	return nil
}

func (mm *Mattermost) setDefaultReplicasAndResources() {
	mm.Spec.Size = ""

	if mm.Spec.Replicas == nil {
		mm.Spec.Replicas = &mattermostv1alpha1.DefaultSize.App.Replicas
	}
	if mm.Spec.Scheduling.Resources.Size() == 0 {
		mm.Spec.Scheduling.Resources = mattermostv1alpha1.DefaultSize.App.Resources
	}

	mm.Spec.FileStore.SetDefaultReplicasAndResources()
	mm.Spec.Database.SetDefaultReplicasAndResources()
}

func (mm *Mattermost) overrideReplicasAndResourcesFromSize(size mattermostv1alpha1.ClusterInstallationSize) {
	mm.Spec.Size = ""

	mm.Spec.Replicas = utils.NewInt32(size.App.Replicas)
	mm.Spec.Scheduling.Resources = size.App.Resources
	mm.Spec.FileStore.OverrideReplicasAndResourcesFromSize(size)
	mm.Spec.Database.OverrideReplicasAndResourcesFromSize(size)
}
