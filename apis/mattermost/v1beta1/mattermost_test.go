// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package v1beta1

import (
	"fmt"
	"testing"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/apis/mattermost/v1alpha1"
	"github.com/mattermost/mattermost-operator/pkg/utils"
	appsv1 "k8s.io/api/apps/v1"

	operatortest "github.com/mattermost/mattermost-operator/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestMattermost(t *testing.T) {
	mm := &Mattermost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: MattermostSpec{
			Replicas:    utils.NewInt32(7),
			Image:       "mattermost/mattermost-enterprise-edition",
			Version:     operatortest.LatestStableMattermostVersion,
			IngressName: "foo.mattermost.dev",
		},
	}

	t.Run("scheme", func(t *testing.T) {
		err := SchemeBuilder.AddToScheme(scheme.Scheme)
		require.NoError(t, err)
	})

	t.Run("deepcopy", func(t *testing.T) {
		t.Run("cluster installation", func(t *testing.T) {
			require.Equal(t, mm, mm.DeepCopy())
		})
		t.Run("cluster installation list", func(t *testing.T) {
			cil := &MattermostList{
				Items: []Mattermost{
					*mm,
				},
			}
			require.Equal(t, cil, cil.DeepCopy())
		})
	})

	t.Run("set replicas and resources with user count", func(t *testing.T) {
		mm = &Mattermost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "default",
			},
			Spec: MattermostSpec{
				Image:       "mattermost/mattermost-enterprise-edition",
				Version:     operatortest.LatestStableMattermostVersion,
				IngressName: "foo.mattermost.dev",
				Size:        "1000users",
			},
		}

		size1000, err := mattermostv1alpha1.GetClusterSize(mattermostv1alpha1.Size1000String)
		require.NoError(t, err)

		t.Run("should set correctly", func(t *testing.T) {
			tmm := mm.DeepCopy()
			err := tmm.SetReplicasAndResourcesFromSize()
			require.NoError(t, err)
			assert.Equal(t, size1000.App.Replicas, *tmm.Spec.Replicas)
			assert.Equal(t, size1000.App.Resources.String(), tmm.Spec.Scheduling.Resources.String())
			assert.Equal(t, size1000.Minio.Replicas, *tmm.Spec.FileStore.OperatorManaged.Replicas)
			assert.Equal(t, size1000.Minio.Resources.String(), tmm.Spec.FileStore.OperatorManaged.Resources.String())
			assert.Equal(t, size1000.Database.Replicas, *tmm.Spec.Database.OperatorManaged.Replicas)
			assert.Equal(t, size1000.Database.Resources.String(), tmm.Spec.Database.OperatorManaged.Resources.String())
			assert.Equal(t, "", tmm.Spec.Size)
		})

		t.Run("should override manually set replicas or resources when setting Size", func(t *testing.T) {
			tmm := mm.DeepCopy()
			resources := corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("100Mi"),
				},
			}

			overriddenReplicas := int32(7)
			tmm.Spec.Replicas = utils.NewInt32(overriddenReplicas)
			tmm.Spec.Scheduling.Resources = resources
			tmm.Spec.FileStore.OperatorManaged = &OperatorManagedMinio{
				Resources: resources,
				Replicas:  utils.NewInt32(overriddenReplicas),
			}
			tmm.Spec.Database.OperatorManaged = &OperatorManagedDatabase{
				Resources: resources,
				Replicas:  utils.NewInt32(overriddenReplicas),
			}

			err := tmm.SetReplicasAndResourcesFromSize()
			require.NoError(t, err)
			assert.Equal(t, size1000.App.Replicas, *tmm.Spec.Replicas)
			assert.Equal(t, size1000.App.Resources.String(), tmm.Spec.Scheduling.Resources.String())
			assert.Equal(t, size1000.Minio.Replicas, *tmm.Spec.FileStore.OperatorManaged.Replicas)
			assert.Equal(t, size1000.Minio.Resources.String(), tmm.Spec.FileStore.OperatorManaged.Resources.String())
			assert.Equal(t, size1000.Database.Replicas, *tmm.Spec.Database.OperatorManaged.Replicas)
			assert.Equal(t, size1000.Database.Resources.String(), tmm.Spec.Database.OperatorManaged.Resources.String())
			assert.Equal(t, "", tmm.Spec.Size)
		})

		t.Run("should set defaults on size not specified", func(t *testing.T) {
			tmm := mm.DeepCopy()
			tmm.Spec.Size = ""
			err := tmm.SetReplicasAndResourcesFromSize()
			assert.NoError(t, err)
			assert.Equal(t, mattermostv1alpha1.DefaultSize.App.Replicas, *tmm.Spec.Replicas)
			assert.Equal(t, mattermostv1alpha1.DefaultSize.App.Resources.String(), tmm.Spec.Scheduling.Resources.String())
			assert.Equal(t, mattermostv1alpha1.DefaultSize.Minio.Replicas, *tmm.Spec.FileStore.OperatorManaged.Replicas)
			assert.Equal(t, mattermostv1alpha1.DefaultSize.Minio.Resources.String(), tmm.Spec.FileStore.OperatorManaged.Resources.String())
			assert.Equal(t, mattermostv1alpha1.DefaultSize.Database.Replicas, *tmm.Spec.Database.OperatorManaged.Replicas)
			assert.Equal(t, mattermostv1alpha1.DefaultSize.Database.Resources.String(), tmm.Spec.Database.OperatorManaged.Resources.String())
			assert.Equal(t, "", tmm.Spec.Size)
		})

		t.Run("should error on bad user count but set to default size", func(t *testing.T) {
			tmm := mm.DeepCopy()
			tmm.Spec.Size = "junk"
			err := tmm.SetReplicasAndResourcesFromSize()
			assert.Error(t, err)
			assert.Equal(t, mattermostv1alpha1.DefaultSize.App.Replicas, *tmm.Spec.Replicas)
			assert.Equal(t, mattermostv1alpha1.DefaultSize.App.Resources.String(), tmm.Spec.Scheduling.Resources.String())
			assert.Equal(t, mattermostv1alpha1.DefaultSize.Minio.Replicas, *tmm.Spec.FileStore.OperatorManaged.Replicas)
			assert.Equal(t, mattermostv1alpha1.DefaultSize.Minio.Resources.String(), tmm.Spec.FileStore.OperatorManaged.Resources.String())
			assert.Equal(t, mattermostv1alpha1.DefaultSize.Database.Replicas, *tmm.Spec.Database.OperatorManaged.Replicas)
			assert.Equal(t, mattermostv1alpha1.DefaultSize.Database.Resources.String(), tmm.Spec.Database.OperatorManaged.Resources.String())
			assert.Equal(t, "", tmm.Spec.Size)
		})
	})

	t.Run("correct image", func(t *testing.T) {
		assert.Contains(t, mm.GetImageName(), mm.Spec.Image)
		assert.Contains(t, mm.GetImageName(), mm.Spec.Version)
		assert.Contains(t, mm.GetImageName(), ":")
		assert.Equal(t, mm.GetImageName(), fmt.Sprintf("%s:%s", mm.Spec.Image, mm.Spec.Version))
	})

	t.Run("using digest", func(t *testing.T) {
		mm.Spec.Version = "sha256:dd15a51ac7dafd213744d1ef23394e7532f71a90f477c969b94600e46da5a0cf"
		assert.Contains(t, mm.GetImageName(), mm.Spec.Image)
		assert.Contains(t, mm.GetImageName(), mm.Spec.Version)
		assert.Equal(t, mm.GetImageName(), fmt.Sprintf("%s@%s", mm.Spec.Image, mm.Spec.Version))
	})
}

func TestOtherUtils(t *testing.T) {
	mm := &Mattermost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: MattermostSpec{
			Replicas:    utils.NewInt32(7),
			Image:       "mattermost/mattermost-enterprise-edition",
			Version:     operatortest.LatestStableMattermostVersion,
			IngressName: "foo.mattermost.dev",
		},
	}

	t.Run("get image name", func(t *testing.T) {
		assert.Equal(t, "mattermost/mattermost-enterprise-edition:6.2.1", mm.GetImageName())

		mm.Spec.Version = "sha256:3c37"

		assert.Equal(t, "mattermost/mattermost-enterprise-edition@sha256:3c37", mm.GetImageName())
	})

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "mm",
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "some container", Image: "test"},
						{Name: MattermostAppContainerName, Image: "mattermost/mattermost-enterprise-edition:5.28"},
					},
				},
			},
		},
	}

	t.Run("get mm container from deployment", func(t *testing.T) {
		container := GetMattermostAppContainerFromDeployment(deployment)
		assert.Equal(t, "mattermost/mattermost-enterprise-edition:5.28", container.Image)
		assert.Equal(t, MattermostAppContainerName, container.Name)
	})

	t.Run("get mm container from slice", func(t *testing.T) {
		container := GetMattermostAppContainer(deployment.Spec.Template.Spec.Containers)
		assert.Equal(t, "mattermost/mattermost-enterprise-edition:5.28", container.Image)
		assert.Equal(t, MattermostAppContainerName, container.Name)
	})
}
