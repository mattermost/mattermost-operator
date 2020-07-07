package v1alpha1

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
	DefaultMattermostVersion = "5.23.1"
	// DefaultMattermostSize is the default number of users
	DefaultMattermostSize = "5000users"
	// DefaultMattermostDatabaseType is the default Mattermost database
	DefaultMattermostDatabaseType = "mysql"
	// DefaultMinioStorageSize is the default Storage size for Minio
	DefaultMinioStorageSize = "50Gi"
	// DefaultStorageSize is the default Storage size for the Database
	DefaultStorageSize = "50Gi"

	// ClusterLabel is the label applied across all compoments
	ClusterLabel = "v1alpha1.mattermost.com/installation"
	// ClusterResourceLabel is the label applied to a given ClusterInstallation
	// as well as all other resources created to support it.
	ClusterResourceLabel = "v1alpha1.mattermost.com/resource"

	// BlueName is the name of the blue Mattermmost installation in a blue/green
	// deployment type.
	BlueName = "blue"
	// GreenName is the name of the green Mattermmost installation in a blue/green
	// deployment type.
	GreenName = "green"

	// SizeMB is the number of bytes that make a megabyte
	SizeMB = 1048576
	// SizeGB is the number of bytes that make a gigabyte
	SizeGB = 1048576000
	// DefaultMaxFileSize is the default maximum file size configuration value that will be used unless nginx annotation is set
	DefaultMaxFileSize = 1000

	// defaultRevHistoryLimit is the default RevisionHistoryLimit - number of possible roll-back points
	// More details: https://kubernetes.io/docs/concepts/workloads/controllers/deployment/#rolling-back-a-deployment
	defaultRevHistoryLimit = 5
	// defaultMaxUnavailable is the default max number of unavailable pods out of specified `Replicas` during rolling update.
	// More details: https://kubernetes.io/docs/concepts/workloads/controllers/deployment/#max-unavailable
	// Recommended to be as low as possible - in order to have number of available pod as close to `Replicas` as possible
	defaultMaxUnavailable = 0
	// defaultMaxSurge is the default max number of extra pods over specified `Replicas` during rolling update.
	// More details: https://kubernetes.io/docs/concepts/workloads/controllers/deployment/#max-surge
	// Recommended not to be too high - in order to have not too many extra pods over requested `Replicas` number
	defaultMaxSurge = 1

	// MattermostAppContainerName is the name of the container which runs the
	// Mattermost application
	MattermostAppContainerName = "mattermost"
)

// SetDefaults set the missing values in the manifest to the default ones
func (mattermost *ClusterInstallation) SetDefaults() error {
	if mattermost.Spec.IngressName == "" {
		return errors.New("IngressName required, but not set")
	}
	if mattermost.Spec.Image == "" {
		mattermost.Spec.Image = DefaultMattermostImage
	}
	if mattermost.Spec.Version == "" {
		mattermost.Spec.Version = DefaultMattermostVersion
	}
	if mattermost.Spec.Size == "" {
		mattermost.Spec.Size = DefaultMattermostSize
	}

	mattermost.Spec.Minio.SetDefaults()
	mattermost.Spec.Database.SetDefaults()
	err := mattermost.Spec.BlueGreen.SetDefaults(mattermost)
	if err != nil {
		return err
	}

	err = mattermost.Spec.Canary.SetDefaults(mattermost)

	return err
}

// SetDefaults sets the missing values in Canary to the default ones
func (canary *Canary) SetDefaults(mattermost *ClusterInstallation) error {
	if canary.Enable {
		if canary.Deployment.Version == "" {
			return errors.New("Canary version required, but not set")
		}
		if canary.Deployment.Image == "" {
			return errors.New("Canary deployment image required, but not set")
		}
		if canary.Deployment.Name == "" {
			canary.Deployment.Name = fmt.Sprintf("%s-canary", mattermost.Name)
		}
	}

	return nil
}

// SetDefaults sets the missing values in BlueGreen to the default ones
func (bg *BlueGreen) SetDefaults(mattermost *ClusterInstallation) error {
	if bg.Enable {
		bg.ProductionDeployment = strings.ToLower(bg.ProductionDeployment)
		if bg.ProductionDeployment != BlueName && bg.ProductionDeployment != GreenName {
			return fmt.Errorf("%s is not a valid ProductionDeployment value, must be 'blue' or 'green'", bg.ProductionDeployment)
		}
		if bg.Green.Version == "" || bg.Blue.Version == "" {
			return errors.New("Both Blue and Green deployment versions required, but not set")
		}
		if bg.Blue.Image == "" || bg.Green.Image == "" {
			return errors.New("Both Blue and Green deployment images required, but not set")
		}

		if bg.Green.Name == "" {
			bg.Green.Name = fmt.Sprintf("%s-green", mattermost.Name)
		}
		if bg.Blue.Name == "" {
			bg.Blue.Name = fmt.Sprintf("%s-blue", mattermost.Name)
		}
		if bg.Green.IngressName == "" {
			bg.Green.IngressName = fmt.Sprintf("green.%s", mattermost.Spec.IngressName)
		}
		if bg.Blue.IngressName == "" {
			bg.Blue.IngressName = fmt.Sprintf("blue.%s", mattermost.Spec.IngressName)
		}
	}

	return nil
}

// SetDefaults sets the missing values in Minio to the default ones
func (mi *Minio) SetDefaults() {
	if mi.StorageSize == "" {
		mi.StorageSize = DefaultMinioStorageSize
	}
}

// IsExternal returns true if the MinIO/S3 instance is external
func (mi *Minio) IsExternal() bool {
	return mi.ExternalURL != ""
}

// SetDefaults sets the missing values in Database to the default ones
func (db *Database) SetDefaults() {
	if len(db.Type) == 0 {
		db.Type = DefaultMattermostDatabaseType
	}
	if db.StorageSize == "" {
		db.StorageSize = DefaultStorageSize
	}
}

// GetContainerByName gets container from a deployment by name
func (mattermost *ClusterInstallation) GetContainerByName(deployment *appsv1.Deployment, containerName string) *corev1.Container {
	for i := range deployment.Spec.Template.Spec.Containers {
		container := &deployment.Spec.Template.Spec.Containers[i]
		if container.Name == containerName {
			return container
		}
	}
	return nil
}

// GetMainContainer gets container which runs Mattermost application from a deployment
func (mattermost *ClusterInstallation) GetMainContainer(deployment *appsv1.Deployment) *corev1.Container {
	// Check new-style - fixed name
	container := mattermost.GetContainerByName(deployment, MattermostAppContainerName)
	if container == nil {
		// Check old-style - name of the container == name of the deployment
		container = mattermost.GetContainerByName(deployment, deployment.Name)
	}
	return container
}

// GetImageName returns the container image name that matches the spec of the
// ClusterInstallation.
func (mattermost *ClusterInstallation) GetImageName() string {
	return fmt.Sprintf("%s:%s", mattermost.Spec.Image, mattermost.Spec.Version)
}

// GetProductionDeploymentName returns the name of the deployment that is
// currently designated as production.
func (mattermost *ClusterInstallation) GetProductionDeploymentName() string {
	if mattermost.Spec.BlueGreen.Enable {
		if mattermost.Spec.BlueGreen.ProductionDeployment == BlueName {
			return mattermost.Spec.BlueGreen.Blue.Name
		}
		if mattermost.Spec.BlueGreen.ProductionDeployment == GreenName {
			return mattermost.Spec.BlueGreen.Green.Name
		}
	}

	return mattermost.Name
}

// GetDeploymentImageName returns the container image name that matches the spec
// of the deployment.
func (d *AppDeployment) GetDeploymentImageName() string {
	return fmt.Sprintf("%s:%s", d.Image, d.Version)
}

// ClusterInstallationSelectorLabels returns the selector labels for selecting the resources
// belonging to the given mattermost clusterinstallation.
func ClusterInstallationSelectorLabels(name string) map[string]string {
	l := ClusterInstallationResourceLabels(name)
	l[ClusterLabel] = name
	l["app"] = "mattermost"
	return l
}

// ClusterInstallationLabels returns the labels for selecting the resources
// belonging to the given mattermost clusterinstallation.
func (mattermost *ClusterInstallation) ClusterInstallationLabels(name string) map[string]string {
	l := ClusterInstallationResourceLabels(name)
	l[ClusterLabel] = name
	l["app"] = "mattermost"

	labels := map[string]string{}
	if mattermost.Spec.BlueGreen.Enable {
		if mattermost.Spec.BlueGreen.ProductionDeployment == BlueName {
			labels = mattermost.Spec.BlueGreen.Blue.ResourceLabels
		}
		if mattermost.Spec.BlueGreen.ProductionDeployment == GreenName {
			labels = mattermost.Spec.BlueGreen.Green.ResourceLabels
		}
	} else {
		labels = mattermost.Spec.ResourceLabels
	}

	for k, v := range labels {
		l[k] = v
	}
	return l
}

// MySQLLabels returns the labels for selecting the resources belonging to the
// given mysql cluster.
func MySQLLabels() map[string]string {
	l := map[string]string{}
	l["app.kubernetes.io/component"] = "database"
	l["app.kubernetes.io/instance"] = "db"
	l["app.kubernetes.io/managed-by"] = "mysql.presslabs.org"
	l["app.kubernetes.io/name"] = "mysql"

	return l
}

// ClusterInstallationResourceLabels returns the labels for selecting a given
// ClusterInstallation as well as any external dependency resources that were
// created for the installation.
func ClusterInstallationResourceLabels(name string) map[string]string {
	return map[string]string{ClusterResourceLabel: name}
}
