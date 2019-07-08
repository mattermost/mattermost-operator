package v1alpha1

import (
	"errors"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// ClusterInstallationSize is sizing configuration used to convert user count to replica and resource requirements.
type ClusterInstallationSize struct {
	App      ComponentSize
	Minio    ComponentSize
	Database ComponentSize
}

// ComponentSize is sizing configuartion for different components of a ClusterInstallation.
type ComponentSize struct {
	Replicas  int32
	Resources corev1.ResourceRequirements
}

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

var validSizes = map[string]ClusterInstallationSize{
	"100users":   size100,
	"1000users":  size1000,
	"5000users":  size5000,
	"10000users": size10000,
	"25000users": size25000,
}

var defaultSize = size5000

// SetReplicasAndResourcesFromSize will use the Size field to determine the number of replicas
// and resource requests to set for a ClusterInstallation. If Replicas or Resources for any components are
// manually set in the spec then those values will not be changed.
func (mattermost *ClusterInstallation) SetReplicasAndResourcesFromSize() error {
	var err error
	size, ok := validSizes[mattermost.Spec.Size]
	if !ok {
		err = errors.New("Invalid size, using default")
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

	if mattermost.Spec.Database.Replicas == 0 {
		mattermost.Spec.Database.Replicas = size.Database.Replicas
	}
	if mattermost.Spec.Database.Resources.Size() == 0 {
		mattermost.Spec.Database.Resources = size.Database.Resources
	}

	return err
}
