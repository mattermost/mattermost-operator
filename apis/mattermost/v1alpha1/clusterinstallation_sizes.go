package v1alpha1

import (
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// ClusterInstallationSize is sizing configuration used to convert user count to replica and resource requirements.
type ClusterInstallationSize struct {
	App      ComponentSize
	Minio    MinioSize
	Database ComponentSize
}

// ComponentSize is sizing configuration for different components of a ClusterInstallation.
type ComponentSize struct {
	Replicas  int32
	Resources corev1.ResourceRequirements
}

type MinioSize struct {
	ComponentSize

	Servers          int32
	VolumesPerServer int32
}

// Size100String represents estimated installation sizing for 100 users.
const Size100String = "100users"

var size100 = ClusterInstallationSize{
	App: ComponentSize{
		Replicas: 1,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2000m"),
				corev1.ResourceMemory: resource.MustParse("4Gi"),
			},
		},
	},
	Minio: MinioSize{
		VolumesPerServer: 4,
		Servers:          1,
		ComponentSize: ComponentSize{
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
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

var cloudSize10 = ClusterInstallationSize{
	App: ComponentSize{
		Replicas: 2,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("10m"),
				corev1.ResourceMemory: resource.MustParse("150Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2000m"),
				corev1.ResourceMemory: resource.MustParse("4Gi"),
			},
		},
	},
	Minio: MinioSize{
		VolumesPerServer: 2,
		Servers:          4,
		ComponentSize: ComponentSize{
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
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

var cloudSize100 = ClusterInstallationSize{
	App: ComponentSize{
		Replicas: 2,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("50m"),
				corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2000m"),
				corev1.ResourceMemory: resource.MustParse("4Gi"),
			},
		},
	},
	Minio: MinioSize{
		VolumesPerServer: 2,
		Servers:          4,
		ComponentSize: ComponentSize{
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
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

var size1000 = ClusterInstallationSize{
	App: ComponentSize{
		Replicas: 2,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("150m"),
				corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2000m"),
				corev1.ResourceMemory: resource.MustParse("4Gi"),
			},
		},
	},
	Minio: MinioSize{
		VolumesPerServer: 2,
		Servers:          4,
		ComponentSize: ComponentSize{
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
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

var size5000 = ClusterInstallationSize{
	App: ComponentSize{
		Replicas: 2,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("500Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4000m"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
			},
		},
	},
	Minio: MinioSize{
		VolumesPerServer: 2,
		Servers:          4,
		ComponentSize: ComponentSize{
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
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

var size10000 = ClusterInstallationSize{
	App: ComponentSize{
		Replicas: 2,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("500Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4000m"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
			},
		},
	},
	Minio: MinioSize{
		VolumesPerServer: 2,
		Servers:          4,
		ComponentSize: ComponentSize{
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
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

var size25000 = ClusterInstallationSize{
	App: ComponentSize{
		Replicas: 2,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("500Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4000m"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
			},
		},
	},
	Minio: MinioSize{
		VolumesPerServer: 2,
		Servers:          4,
		ComponentSize: ComponentSize{
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
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

var sizeMiniSingleton = ClusterInstallationSize{
	App: ComponentSize{
		Replicas: 1,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2000m"),
				corev1.ResourceMemory: resource.MustParse("4Gi"),
			},
		},
	},
	Minio: MinioSize{
		VolumesPerServer: 4,
		Servers:          1,
		ComponentSize: ComponentSize{
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
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

var sizeMiniHA = ClusterInstallationSize{
	App: ComponentSize{
		Replicas: 2,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2000m"),
				corev1.ResourceMemory: resource.MustParse("4Gi"),
			},
		},
	},
	Minio: MinioSize{
		VolumesPerServer: 2,
		Servers:          4,
		ComponentSize: ComponentSize{
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
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

var validSizes = map[string]ClusterInstallationSize{
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

var DefaultSize = size5000

// CalculateResourceMilliRequirements returns the milli values for the CPU and
// memory requests of the cluster size.
func (cis *ClusterInstallationSize) CalculateResourceMilliRequirements(includeDatabase, includeMinio bool) (int64, int64) {
	return cis.CalculateCPUMilliRequirement(includeDatabase, includeMinio), cis.CalculateMemoryMilliRequirement(includeDatabase, includeMinio)
}

// CalculateCPUMilliRequirement returns the milli value for the CPU request of
// the cluster size.
func (cis *ClusterInstallationSize) CalculateCPUMilliRequirement(includeDatabase, includeMinio bool) int64 {
	cpuRequirement := (cis.App.Resources.Requests.Cpu().MilliValue() * int64(cis.App.Replicas))
	if includeDatabase {
		cpuRequirement += (cis.Database.Resources.Requests.Cpu().MilliValue() * int64(cis.Database.Replicas))
	}
	if includeMinio {
		cpuRequirement += (cis.Minio.Resources.Requests.Cpu().MilliValue() * int64(cis.Minio.Servers))
	}

	return cpuRequirement
}

// CalculateMemoryMilliRequirement returns the milli value for the memory
// request of the cluster size.
func (cis *ClusterInstallationSize) CalculateMemoryMilliRequirement(includeDatabase, includeMinio bool) int64 {
	memRequirement := (cis.App.Resources.Requests.Memory().MilliValue() * int64(cis.App.Replicas))
	if includeDatabase {
		memRequirement += (cis.Database.Resources.Requests.Memory().MilliValue() * int64(cis.Database.Replicas))
	}
	if includeMinio {
		memRequirement += (cis.Minio.Resources.Requests.Memory().MilliValue() * int64(cis.Minio.Servers))
	}

	return memRequirement
}

// GetClusterSize returns a ClusterInstallationSize based on the provided
// size key.
func GetClusterSize(key string) (ClusterInstallationSize, error) {
	size, ok := validSizes[key]
	if !ok {
		return ClusterInstallationSize{}, errors.New("invalid cluster size")
	}

	return size, nil
}

// SetReplicasAndResourcesFromSize will use the Size field to determine the number of replicas
// and resource requests to set for a ClusterInstallation. If the Size field is not set, values for default size will be used.
// Setting Size to new value will override current values for Replicas and Resources.
// The Size field is erased after adjusting the values.
func (mattermost *ClusterInstallation) SetReplicasAndResourcesFromSize() error {
	if mattermost.Spec.Size == "" {
		mattermost.setDefaultReplicasAndResources()
		return nil
	}

	size, err := GetClusterSize(mattermost.Spec.Size)
	if err != nil {
		err = errors.Wrap(err, "using default")
		mattermost.setDefaultReplicasAndResources()
		return err
	}

	mattermost.overrideReplicasAndResourcesFromSize(size)

	return nil
}

func (mattermost *ClusterInstallation) setDefaultReplicasAndResources() {
	mattermost.Spec.Size = ""

	if mattermost.Spec.Replicas == 0 {
		mattermost.Spec.Replicas = DefaultSize.App.Replicas
	}
	if mattermost.Spec.Resources.Size() == 0 {
		mattermost.Spec.Resources = DefaultSize.App.Resources
	}

	if mattermost.Spec.Minio.Servers == 0 {
		mattermost.Spec.Minio.Servers = DefaultSize.Minio.Servers
	}
	if mattermost.Spec.Minio.VolumesPerServer == 0 {
		mattermost.Spec.Minio.VolumesPerServer = DefaultSize.Minio.VolumesPerServer
	}
	if mattermost.Spec.Minio.Resources.Size() == 0 {
		mattermost.Spec.Minio.Resources = DefaultSize.Minio.Resources
	}

	if mattermost.Spec.Database.Replicas == 0 && mattermost.Spec.Database.InitBucketURL == "" {
		mattermost.Spec.Database.Replicas = DefaultSize.Database.Replicas
	}
	if mattermost.Spec.Database.Resources.Size() == 0 {
		mattermost.Spec.Database.Resources = DefaultSize.Database.Resources
	}
}

func (mattermost *ClusterInstallation) overrideReplicasAndResourcesFromSize(size ClusterInstallationSize) {
	mattermost.Spec.Size = ""

	mattermost.Spec.Replicas = size.App.Replicas
	mattermost.Spec.Resources = size.App.Resources
	mattermost.Spec.Minio.Servers = size.Minio.Servers
	mattermost.Spec.Minio.VolumesPerServer = size.Minio.VolumesPerServer
	mattermost.Spec.Minio.Resources = size.Minio.Resources
	mattermost.Spec.Database.Replicas = size.Database.Replicas
	mattermost.Spec.Database.Resources = size.Database.Resources
}
