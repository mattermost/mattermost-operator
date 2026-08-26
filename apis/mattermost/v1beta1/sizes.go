// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package v1beta1

import (
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// Size is sizing configuration used to convert a user count into replica and
// resource requirements for the Mattermost app servers.
//
// It previously also sized an Operator-managed database and MinIO; both are gone,
// so only the app servers remain.
type Size struct {
	App ComponentSize
}

// ComponentSize is sizing configuration for a component of a Mattermost.
type ComponentSize struct {
	Replicas  int32
	Resources corev1.ResourceRequirements
}

// Size100String represents estimated installation sizing for 100 users.
const Size100String = "100users"

// CloudSize10String represents estimated Mattermost Cloud installation sizing for 10 users.
const CloudSize10String = "cloud10users"

// CloudSize100String represents estimated Mattermost Cloud installation sizing for 100 users.
const CloudSize100String = "cloud100users"

// Size1000String represents estimated installation sizing for 1000 users.
const Size1000String = "1000users"

// Size5000String represents estimated installation sizing for 5000 users.
const Size5000String = "5000users"

// Size10000String represents estimated installation sizing for 10000 users.
const Size10000String = "10000users"

// Size25000String represents estimated installation sizing for 25000 users.
const Size25000String = "25000users"

// SizeMiniSingletonString represents a very small dev installation.
const SizeMiniSingletonString = "miniSingleton"

// SizeMiniHAString represents a very small dev installation with multiple replicas.
const SizeMiniHAString = "miniHA"

var size100 = Size{
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
}

var cloudSize10 = Size{
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
}

var cloudSize100 = Size{
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
}

var size1000 = Size{
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
}

var size5000 = Size{
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
}

var size10000 = Size{
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
}

var size25000 = Size{
	App: ComponentSize{
		Replicas: 2,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("1000m"),
				corev1.ResourceMemory: resource.MustParse("4Gi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4000m"),
				corev1.ResourceMemory: resource.MustParse("16Gi"),
			},
		},
	},
}

var sizeMiniSingleton = Size{
	App: ComponentSize{
		Replicas: 1,
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
}

var sizeMiniHA = Size{
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
}

var validSizes = map[string]Size{
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

// DefaultSize is the size applied when spec.size is not set.
var DefaultSize = size5000

// CalculateResourceMilliRequirements returns the milli values for the CPU and
// memory requests of the size.
func (s *Size) CalculateResourceMilliRequirements() (int64, int64) {
	return s.CalculateCPUMilliRequirement(), s.CalculateMemoryMilliRequirement()
}

// CalculateCPUMilliRequirement returns the milli value for the CPU request of
// the size.
func (s *Size) CalculateCPUMilliRequirement() int64 {
	return s.App.Resources.Requests.Cpu().MilliValue() * int64(s.App.Replicas)
}

// CalculateMemoryMilliRequirement returns the milli value for the memory
// request of the size.
func (s *Size) CalculateMemoryMilliRequirement() int64 {
	return s.App.Resources.Requests.Memory().MilliValue() * int64(s.App.Replicas)
}

// GetMattermostSize returns a Size based on the provided size key.
func GetMattermostSize(key string) (Size, error) {
	size, ok := validSizes[key]
	if !ok {
		return Size{}, errors.New("invalid cluster size")
	}

	return size, nil
}
