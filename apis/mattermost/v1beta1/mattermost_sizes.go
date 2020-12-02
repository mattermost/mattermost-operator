// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package v1beta1

import (
	"github.com/mattermost/mattermost-operator/pkg/utils"
	"github.com/pkg/errors"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// MattermostSize is sizing configuration used to convert user count to replica and resource requirements.
type MattermostSize struct {
	App      ComponentSize
	Minio    ComponentSize
	Database ComponentSize
}

// ComponentSize is sizing configuration for different components of a Mattermost.
type ComponentSize struct {
	Replicas  int32
	Resources corev1.ResourceRequirements
}

// Size100String represents estimated installation sizing for 100 users.
const Size100String = "100users"

var size100 = MattermostSize{
	App: ComponentSize{
		Replicas: 1,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2"),
				corev1.ResourceMemory: resource.MustParse("4Gi"),
			},
		},
	},
	Minio: ComponentSize{
		Replicas: 1,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
		},
	},
	Database: ComponentSize{
		Replicas: 1,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
		},
	},
}

// CloudSize10String represents estimated Mattermost Cloud installation sizing for 10 users.
const CloudSize10String = "cloud10users"

var cloudSize10 = MattermostSize{
	App: ComponentSize{
		Replicas: 2,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("10m"),
				corev1.ResourceMemory: resource.MustParse("200Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2"),
				corev1.ResourceMemory: resource.MustParse("4Gi"),
			},
		},
	},
	Minio: ComponentSize{
		Replicas: 4,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
		},
	},
	Database: ComponentSize{
		Replicas: 2,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
		},
	},
}

// CloudSize100String represents estimated Mattermost Cloud installation sizing for 100 users.
const CloudSize100String = "cloud100users"

var cloudSize100 = MattermostSize{
	App: ComponentSize{
		Replicas: 2,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("50m"),
				corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2"),
				corev1.ResourceMemory: resource.MustParse("4Gi"),
			},
		},
	},
	Minio: ComponentSize{
		Replicas: 4,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
		},
	},
	Database: ComponentSize{
		Replicas: 2,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
		},
	},
}

// Size1000String represents estimated installation sizing for 1000 users.
const Size1000String = "1000users"

var size1000 = MattermostSize{
	App: ComponentSize{
		Replicas: 2,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("150m"),
				corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2"),
				corev1.ResourceMemory: resource.MustParse("4Gi"),
			},
		},
	},
	Minio: ComponentSize{
		Replicas: 4,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("150m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
		},
	},
	Database: ComponentSize{
		Replicas: 2,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("250m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
		},
	},
}

// Size5000String represents estimated installation sizing for 5000 users.
const Size5000String = "5000users"

var size5000 = MattermostSize{
	App: ComponentSize{
		Replicas: 2,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("500Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
			},
		},
	},
	Minio: ComponentSize{
		Replicas: 4,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("500Mi"),
			},
		},
	},
	Database: ComponentSize{
		Replicas: 3,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("500Mi"),
			},
		},
	},
}

// Size10000String represents estimated installation sizing for 10000 users.
const Size10000String = "10000users"

var size10000 = MattermostSize{
	App: ComponentSize{
		Replicas: 2,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("500Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
			},
		},
	},
	Minio: ComponentSize{
		Replicas: 4,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("500Mi"),
			},
		},
	},
	Database: ComponentSize{
		Replicas: 3,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("500Mi"),
			},
		},
	},
}

// Size25000String represents estimated installation sizing for 25000 users.
const Size25000String = "25000users"

var size25000 = MattermostSize{
	App: ComponentSize{
		Replicas: 2,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("500Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
			},
		},
	},
	Minio: ComponentSize{
		Replicas: 4,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("500Mi"),
			},
		},
	},
	Database: ComponentSize{
		Replicas: 3,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("500Mi"),
			},
		},
	},
}

// Sizes used for development and testing

// SizeMiniSingletonString represents a very small dev installation.
const SizeMiniSingletonString = "miniSingleton"

var sizeMiniSingleton = MattermostSize{
	App: ComponentSize{
		Replicas: 1,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2"),
				corev1.ResourceMemory: resource.MustParse("4Gi"),
			},
		},
	},
	Minio: ComponentSize{
		Replicas: 1,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
		},
	},
	Database: ComponentSize{
		Replicas: 1,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
		},
	},
}

// SizeMiniHAString represents a very small dev installation with multiple replicas.
const SizeMiniHAString = "miniHA"

var sizeMiniHA = MattermostSize{
	App: ComponentSize{
		Replicas: 2,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2"),
				corev1.ResourceMemory: resource.MustParse("4Gi"),
			},
		},
	},
	Minio: ComponentSize{
		Replicas: 4,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
		},
	},
	Database: ComponentSize{
		Replicas: 2,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
		},
	},
}

var validSizes = map[string]MattermostSize{
	CloudSize10String:       cloudSize10,
	CloudSize100String:      cloudSize100,
	Size100String:           size100,
	Size1000String:          size1000,
	Size5000String:          size5000,
	Size10000String:         size10000,
	Size25000String:         size25000,
	SizeMiniSingletonString: sizeMiniSingleton,
	SizeMiniHAString:        sizeMiniHA,
}

var defaultSize = size5000

// CalculateResourceMilliRequirements returns the milli values for the CPU and
// memory requests of the cluster size.
func (cis *MattermostSize) CalculateResourceMilliRequirements(includeDatabase, includeMinio bool) (int64, int64) {
	return cis.CalculateCPUMilliRequirement(includeDatabase, includeMinio), cis.CalculateMemoryMilliRequirement(includeDatabase, includeMinio)
}

// CalculateCPUMilliRequirement returns the milli value for the CPU request of
// the cluster size.
func (cis *MattermostSize) CalculateCPUMilliRequirement(includeDatabase, includeMinio bool) int64 {
	cpuRequirement := cis.App.Resources.Requests.Cpu().MilliValue() * int64(cis.App.Replicas)
	if includeDatabase {
		cpuRequirement += cis.Database.Resources.Requests.Cpu().MilliValue() * int64(cis.Database.Replicas)
	}
	if includeMinio {
		cpuRequirement += cis.Minio.Resources.Requests.Cpu().MilliValue() * int64(cis.Minio.Replicas)
	}

	return cpuRequirement
}

// CalculateMemoryMilliRequirement returns the milli value for the memory
// request of the cluster size.
func (cis *MattermostSize) CalculateMemoryMilliRequirement(includeDatabase, includeMinio bool) int64 {
	memRequirement := cis.App.Resources.Requests.Memory().MilliValue() * int64(cis.App.Replicas)
	if includeDatabase {
		memRequirement += cis.Database.Resources.Requests.Memory().MilliValue() * int64(cis.Database.Replicas)
	}
	if includeMinio {
		memRequirement += cis.Minio.Resources.Requests.Memory().MilliValue() * int64(cis.Minio.Replicas)
	}

	return memRequirement
}

// GetClusterSize returns a MattermostSize based on the provided
// size key.
func GetClusterSize(key string) (MattermostSize, error) {
	size, ok := validSizes[key]
	if !ok {
		return MattermostSize{}, errors.New("invalid cluster size")
	}

	return size, nil
}

// SetReplicasAndResourcesFromSize will use the Size field to determine the number of replicas
// and resource requests to set for a ClusterInstallation. If the Size field is not set, values for default size will be used.
// Setting Size to new value will override current values for Replicas and Resources.
// The Size field is erased after adjusting the values.
func (mm *Mattermost) SetReplicasAndResourcesFromSize() error {
	if mm.Spec.Size == "" {
		mm.setDefaultReplicasAndResources()
		return nil
	}

	size, err := GetClusterSize(mm.Spec.Size)
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
		mm.Spec.Replicas = &defaultSize.App.Replicas
	}
	if mm.Spec.Scheduling.Resources.Size() == 0 {
		mm.Spec.Scheduling.Resources = defaultSize.App.Resources
	}

	mm.Spec.FileStore.SetDefaultReplicasAndResources()
	mm.Spec.Database.SetDefaultReplicasAndResources()
}

func (mm *Mattermost) overrideReplicasAndResourcesFromSize(size MattermostSize) {
	mm.Spec.Size = ""

	mm.Spec.Replicas = utils.NewInt32(size.App.Replicas)
	mm.Spec.Scheduling.Resources = size.App.Resources
	mm.Spec.FileStore.OverrideReplicasAndResourcesFromSize(size)
	mm.Spec.Database.OverrideReplicasAndResourcesFromSize(size)
}
