package v1alpha1

import (
	"github.com/pkg/errors"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// ClusterInstallationSize is sizing configuration used to convert user count to replica and resource requirements.
type ClusterInstallationSize struct {
	App      ComponentSize
	Minio    ComponentSize
	Database ComponentSize
}

// ComponentSize is sizing configuration for different components of a ClusterInstallation.
type ComponentSize struct {
	Replicas  int32
	Resources corev1.ResourceRequirements
}

const Size100String = "100users"

var size100 = ClusterInstallationSize{
	App: ComponentSize{
		Replicas: 1,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("250m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("1"),
				corev1.ResourceMemory: resource.MustParse("1Gi"),
			},
		},
	},
	Minio: ComponentSize{
		Replicas: 1,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("250m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
		},
	},
	Database: ComponentSize{
		Replicas: 1,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("1Gi"),
			},
		},
	},
}

const Size1000String = "1000users"

var size1000 = ClusterInstallationSize{
	App: ComponentSize{
		Replicas: 2,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("1Gi"),
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
				corev1.ResourceCPU:    resource.MustParse("250m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
		},
	},
	Database: ComponentSize{
		Replicas: 2,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("1"),
				corev1.ResourceMemory: resource.MustParse("1.5Gi"),
			},
		},
	},
}

const Size5000String = "5000users"

var size5000 = ClusterInstallationSize{
	App: ComponentSize{
		Replicas: 2,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("1.5"),
				corev1.ResourceMemory: resource.MustParse("3.5Gi"),
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
				corev1.ResourceCPU:    resource.MustParse("1"),
				corev1.ResourceMemory: resource.MustParse("1.5Gi"),
			},
		},
	},
	Database: ComponentSize{
		Replicas: 3,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("3.5"),
				corev1.ResourceMemory: resource.MustParse("7.5Gi"),
			},
		},
	},
}

const Size10000String = "10000users"

var size10000 = ClusterInstallationSize{
	App: ComponentSize{
		Replicas: 3,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("3.5"),
				corev1.ResourceMemory: resource.MustParse("7.5Gi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("8"),
				corev1.ResourceMemory: resource.MustParse("16Gi"),
			},
		},
	},
	Minio: ComponentSize{
		Replicas: 4,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("1.5"),
				corev1.ResourceMemory: resource.MustParse("3.5Gi"),
			},
		},
	},
	Database: ComponentSize{
		Replicas: 3,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("3.5"),
				corev1.ResourceMemory: resource.MustParse("15.5Gi"),
			},
		},
	},
}

const Size25000String = "25000users"

var size25000 = ClusterInstallationSize{
	App: ComponentSize{
		Replicas: 4,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("3.5"),
				corev1.ResourceMemory: resource.MustParse("15.5Gi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("8"),
				corev1.ResourceMemory: resource.MustParse("32Gi"),
			},
		},
	},
	Minio: ComponentSize{
		Replicas: 4,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("3.5"),
				corev1.ResourceMemory: resource.MustParse("15.5Gi"),
			},
		},
	},
	Database: ComponentSize{
		Replicas: 4,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("3.5"),
				corev1.ResourceMemory: resource.MustParse("15.5Gi"),
			},
		},
	},
}

// Sizes used for development and testing

const SizeMiniSingletonString = "miniSingleton"

var sizeMiniSingleton = ClusterInstallationSize{
	App: ComponentSize{
		Replicas: 1,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("250m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("1"),
				corev1.ResourceMemory: resource.MustParse("1Gi"),
			},
		},
	},
	Minio: ComponentSize{
		Replicas: 1,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("250m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
		},
	},
	Database: ComponentSize{
		Replicas: 1,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
		},
	},
}

const SizeMiniHAString = "miniHA"

var sizeMiniHA = ClusterInstallationSize{
	App: ComponentSize{
		Replicas: 2,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("250m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("1"),
				corev1.ResourceMemory: resource.MustParse("1Gi"),
			},
		},
	},
	Minio: ComponentSize{
		Replicas: 4,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("250m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
		},
	},
	Database: ComponentSize{
		Replicas: 2,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
		},
	},
}

var validSizes = map[string]ClusterInstallationSize{
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
		cpuRequirement += (cis.Minio.Resources.Requests.Cpu().MilliValue() * int64(cis.Minio.Replicas))
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
		memRequirement += (cis.Minio.Resources.Requests.Memory().MilliValue() * int64(cis.Minio.Replicas))
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
// and resource requests to set for a ClusterInstallation. If Replicas or Resources for any components are
// manually set in the spec then those values will not be changed.
func (mattermost *ClusterInstallation) SetReplicasAndResourcesFromSize() error {
	size, err := GetClusterSize(mattermost.Spec.Size)
	if err != nil {
		err = errors.Wrap(err, "using default")
		size = defaultSize
	}

	if mattermost.Spec.Replicas == 0 {
		mattermost.Spec.Replicas = size.App.Replicas
	}
	if mattermost.Spec.Resources.Size() == 0 {
		mattermost.Spec.Resources = size.App.Resources
	}

	if mattermost.Spec.Minio.Replicas == 0 {
		mattermost.Spec.Minio.Replicas = size.Minio.Replicas
	}
	if mattermost.Spec.Minio.Resources.Size() == 0 {
		mattermost.Spec.Minio.Resources = size.Minio.Resources
	}

	if mattermost.Spec.Database.Resources.Size() == 0 {
		mattermost.Spec.Database.Resources = size.Database.Resources
	}

	return err
}
