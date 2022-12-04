// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package v1beta1

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

const (
	// OperatorName is the name of the Mattermost operator
	OperatorName = "mattermost-operator"
	// DefaultMattermostImage is the default Mattermost docker image
	DefaultMattermostImage = "mattermost/mattermost-enterprise-edition"
	// DefaultMattermostVersion is the default Mattermost docker tag
	DefaultMattermostVersion = "7.5.1"
	// DefaultMattermostSize is the default number of users
	DefaultMattermostSize = "5000users"
	// DefaultMattermostDatabaseType is the default Mattermost database
	DefaultMattermostDatabaseType = "mysql"
	// DefaultFilestoreStorageSize is the default Storage size for Minio or Local Storage
	DefaultFilestoreStorageSize = "50Gi"
	// DefaultStorageSize is the default Storage size for the Database
	DefaultStorageSize = "50Gi"
	// DefaultPullPolicy is the default Pull Policy used by Mattermost app container
	DefaultPullPolicy = corev1.PullIfNotPresent
	// DefaultLocalFilePath is the default file path used with local (PVC) storage
	DefaultLocalFilePath = "/mattermost/data"

	// ClusterLabel is the label applied across all components
	ClusterLabel = "installation.mattermost.com/installation"

	// ClusterResourceLabel is the label applied to a given Mattermost
	// as well as all other resources created to support it.
	ClusterResourceLabel = "installation.mattermost.com/resource"

	// MattermostAppContainerName is the name of the container which runs the
	// Mattermost application
	MattermostAppContainerName = "mattermost"
)

// SetDefaults set the missing values in the manifest to the default ones
func (mm *Mattermost) SetDefaults() error {
	if mm.IngressEnabled() && mm.GetIngressHost() == "" {
		return errors.New("ingress.host required, but not set")
	}
	if mm.Spec.Image == "" {
		mm.Spec.Image = DefaultMattermostImage
	}
	if mm.Spec.Version == "" {
		mm.Spec.Version = DefaultMattermostVersion
	}
	if mm.Spec.ImagePullPolicy == "" {
		mm.Spec.ImagePullPolicy = DefaultPullPolicy
	}

	mm.Spec.FileStore.SetDefaults()
	mm.Spec.Database.SetDefaults()

	return nil
}

// IngressEnabled determines whether Mattermost Ingress should be created.
func (mm *Mattermost) IngressEnabled() bool {
	if mm.Spec.Ingress != nil {
		return mm.Spec.Ingress.Enabled
	}
	return true
}

// GetIngressHost returns Mattermost primary Ingress host.
func (mm *Mattermost) GetIngressHost() string {
	if mm.Spec.Ingress == nil {
		return mm.Spec.IngressName
	}
	return mm.Spec.Ingress.Host
}

// GetIngressHostNames returns all Ingress host names configured for Mattermost.
// It skips duplicated entries.
func (mm *Mattermost) GetIngressHostNames() []string {
	initialHost := mm.GetIngressHost()
	// TODO: If we decide to deprecates Host in favor of Hosts
	// we might need to adjust this part.
	if initialHost == "" {
		return []string{}
	}

	// Create set of hosts to avoid duplicates
	hostsSet := map[string]struct{}{
		initialHost: {},
	}
	// We do it this way to retain the order of hosts specified in CR
	// while still eliminating duplicates.
	// We do it to avoid unnecessary Ingress updates.
	hosts := []string{initialHost}

	if mm.Spec.Ingress != nil {
		for _, host := range mm.Spec.Ingress.Hosts {
			if _, found := hostsSet[host.HostName]; !found {
				hosts = append(hosts, host.HostName)
				hostsSet[host.HostName] = struct{}{}
			}
		}
	}

	return hosts
}

// GetIngresAnnotations returns Mattermost Ingress annotations.
func (mm *Mattermost) GetIngresAnnotations() map[string]string {
	if mm.Spec.Ingress == nil {
		return mm.Spec.IngressAnnotations
	}
	return mm.Spec.Ingress.Annotations
}

// GetIngressTLSSecret returns Mattermost Ingress TLS secret.
func (mm *Mattermost) GetIngressTLSSecret() string {
	if mm.Spec.Ingress != nil {
		return mm.Spec.Ingress.TLSSecret
	}
	if mm.Spec.UseIngressTLS {
		return defaultTLSSecret(mm)
	}
	return ""
}

// GetIngressClass returns IngressClass for Mattermost Ingress.
func (mm *Mattermost) GetIngressClass() *string {
	if mm.Spec.Ingress == nil {
		return nil
	}
	return mm.Spec.Ingress.IngressClass
}

func defaultTLSSecret(mm *Mattermost) string {
	return strings.ReplaceAll(mm.GetIngressHost(), ".", "-") + "-tls-cert"
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
	l := map[string]string{}
	// Set resourceLabels ("global") as the initial labels
	if mm.Spec.ResourceLabels != nil {
		l = mm.Spec.ResourceLabels
	}
	// Overwrite with default labels
	for k, v := range MattermostResourceLabels(name) {
		l[k] = v
	}
	l[ClusterLabel] = name
	l["app"] = MattermostAppContainerName

	return l
}

// MattermostPodLabels returns the labels for selecting the pods
// belonging to the given mattermost.
func (mm *Mattermost) MattermostPodLabels(name string) map[string]string {
	l := map[string]string{}
	// Set resourceLabels ("global") as the initial labels
	if mm.Spec.ResourceLabels != nil {
		l = mm.Spec.ResourceLabels
	}
	if mm.Spec.PodTemplate != nil {
		// Overwrite with pod specific labels
		for k, v := range mm.Spec.PodTemplate.ExtraLabels {
			l[k] = v
		}
	}
	// Overwrite with default labels
	for k, v := range MattermostResourceLabels(name) {
		l[k] = v
	}
	l[ClusterLabel] = name
	l["app"] = MattermostAppContainerName

	return l
}

// MattermostResourceLabels returns the labels for selecting a given
// Mattermost as well as any external dependency resources that were
// created for the installation.
func MattermostResourceLabels(name string) map[string]string {
	return map[string]string{ClusterResourceLabel: name}
}
