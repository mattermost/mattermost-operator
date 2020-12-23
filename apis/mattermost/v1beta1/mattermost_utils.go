// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package v1beta1

import (
	"errors"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

const (
	// OperatorName is the name of the Mattermost operator
	OperatorName = "mattermost-operator"
	// DefaultMattermostImage is the default Mattermost docker image
	DefaultMattermostImage = "mattermost/mattermost-enterprise-edition"
	// DefaultMattermostVersion is the default Mattermost docker tag
	DefaultMattermostVersion = "5.28.0"
	// DefaultMattermostSize is the default number of users
	DefaultMattermostSize = "5000users"
	// DefaultMattermostDatabaseType is the default Mattermost database
	DefaultMattermostDatabaseType = "mysql"
	// DefaultFilestoreStorageSize is the default Storage size for Minio
	DefaultFilestoreStorageSize = "50Gi"
	// DefaultStorageSize is the default Storage size for the Database
	DefaultStorageSize = "50Gi"

	// TODO: we should consider updating label to something like "v1beta1.installation.mattermost.com/mattermost"
	// but this may cause some issues on existing clusters
	// ClusterLabel is the label applied across all components
	ClusterLabel = "v1alpha1.mattermost.com/installation"

	// ClusterResourceLabel is the label applied to a given Mattermost
	// as well as all other resources created to support it.
	ClusterResourceLabel = "v1alpha1.mattermost.com/resource"

	// MattermostAppContainerName is the name of the container which runs the
	// Mattermost application
	MattermostAppContainerName = "mattermost"
)

// SetDefaults set the missing values in the manifest to the default ones
func (mm *Mattermost) SetDefaults() error {
	if mm.Spec.IngressName == "" {
		return errors.New("IngressName required, but not set")
	}
	if mm.Spec.Image == "" {
		mm.Spec.Image = DefaultMattermostImage
	}
	if mm.Spec.Version == "" {
		mm.Spec.Version = DefaultMattermostVersion
	}

	mm.Spec.FileStore.SetDefaults()
	mm.Spec.Database.SetDefaults()

	return nil
}

// GetMattermostAppContainerFromDeployment gets container from Deployment which runs Mattermost application
// from a deployment.
func GetMattermostAppContainerFromDeployment(deployment *appsv1.Deployment) *corev1.Container {
	container := getDeploymentContainerByName(deployment, MattermostAppContainerName)
	return container
}

// GetMattermostAppContainer gets container from PodSpec which runs Mattermost application
// from a deployment.
func GetMattermostAppContainer(containers []corev1.Container) *corev1.Container {
	container := getContainerByName(containers, MattermostAppContainerName)
	return container
}

// getDeploymentContainerByName gets container from a deployment by name
func getDeploymentContainerByName(deployment *appsv1.Deployment, containerName string) *corev1.Container {
	return getContainerByName(deployment.Spec.Template.Spec.Containers, containerName)
}

// getContainerByName gets container from a slice of containers by name
func getContainerByName(containers []corev1.Container, containerName string) *corev1.Container {
	for _, container := range containers {
		if container.Name == containerName {
			return &container
		}
	}
	return nil
}

// GetImageName returns the container image name that matches the spec of the
// ClusterInstallation.
func (mm *Mattermost) GetImageName() string {
	// if user set the version using the Digest instead of tag like
	// sha256:dd15a51ac7dafd213744d1ef23394e7532f71a90f477c969b94600e46da5a0cf
	// we need to set the @ instead of : to split the image name and "tag"
	if strings.Contains(mm.Spec.Version, "sha256:") {
		return fmt.Sprintf("%s@%s", mm.Spec.Image, mm.Spec.Version)
	}
	return fmt.Sprintf("%s:%s", mm.Spec.Image, mm.Spec.Version)
}

// GetProductionDeploymentName returns the name of the deployment that is
// currently designated as production.
func (mm *Mattermost) GetProductionDeploymentName() string {
	return mm.Name
}

// MattermostSelectorLabels returns the selector labels for selecting the resources
// belonging to the given mattermost instance.
func MattermostSelectorLabels(name string) map[string]string {
	l := MattermostResourceLabels(name)
	l[ClusterLabel] = name
	l["app"] = MattermostAppContainerName
	return l
}

// MattermostLabels returns the labels for selecting the resources
// belonging to the given mattermost.
func (mm *Mattermost) MattermostLabels(name string) map[string]string {
	l := MattermostResourceLabels(name)
	l[ClusterLabel] = name
	l["app"] = MattermostAppContainerName

	labels := mm.Spec.ResourceLabels

	for k, v := range labels {
		l[k] = v
	}
	return l
}

// MattermostResourceLabels returns the labels for selecting a given
// Mattermost as well as any external dependency resources that were
// created for the installation.
func MattermostResourceLabels(name string) map[string]string {
	return map[string]string{ClusterResourceLabel: name}
}
