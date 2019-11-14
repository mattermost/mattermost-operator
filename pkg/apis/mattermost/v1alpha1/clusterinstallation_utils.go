package v1alpha1

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1beta1 "k8s.io/api/extensions/v1beta1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	// OperatorName is the name of the Mattermost operator
	OperatorName = "mattermost-operator"
	// DefaultMattermostImage is the default Mattermost docker image
	DefaultMattermostImage = "mattermost/mattermost-enterprise-edition"
	// DefaultMattermostVersion is the default Mattermost docker tag
	DefaultMattermostVersion = "5.15.0"
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

// GenerateService returns the service for Mattermost
func (mattermost *ClusterInstallation) GenerateService(serviceName, selectorName string) *corev1.Service {
	svcAnnotations := map[string]string{
		"service.alpha.kubernetes.io/tolerate-unready-endpoints": "true",
	}
	if mattermost.Spec.UseServiceLoadBalancer {
		for k, v := range mattermost.Spec.ServiceAnnotations {
			svcAnnotations[k] = v
		}
		return &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Labels:    ClusterInstallationLabels(serviceName),
				Name:      serviceName,
				Namespace: mattermost.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(mattermost, schema.GroupVersionKind{
						Group:   SchemeGroupVersion.Group,
						Version: SchemeGroupVersion.Version,
						Kind:    "ClusterInstallation",
					}),
				},
				Annotations: svcAnnotations,
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Name:       "http",
						Port:       80,
						TargetPort: intstr.FromString("app"),
					},
					{
						Name:       "https",
						Port:       443,
						TargetPort: intstr.FromString("app"),
					},
				},
				Selector: ClusterInstallationLabels(selectorName),
				Type:     corev1.ServiceTypeLoadBalancer,
			},
		}
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels:    ClusterInstallationLabels(serviceName),
			Name:      serviceName,
			Namespace: mattermost.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(mattermost, schema.GroupVersionKind{
					Group:   SchemeGroupVersion.Group,
					Version: SchemeGroupVersion.Version,
					Kind:    "ClusterInstallation",
				}),
			},
			Annotations: svcAnnotations,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port:       8065,
					TargetPort: intstr.FromString("app"),
				},
			},
			Selector:  ClusterInstallationLabels(selectorName),
			ClusterIP: corev1.ClusterIPNone,
		},
	}
}

// GenerateIngress returns the ingress for Mattermost
func (mattermost *ClusterInstallation) GenerateIngress(name, ingressName string, ingressAnnotations map[string]string) *v1beta1.Ingress {
	return &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: mattermost.Namespace,
			Labels:    ClusterInstallationLabels(name),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(mattermost, schema.GroupVersionKind{
					Group:   SchemeGroupVersion.Group,
					Version: SchemeGroupVersion.Version,
					Kind:    "ClusterInstallation",
				}),
			},
			Annotations: ingressAnnotations,
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{
				{
					Host: ingressName,
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{
								{
									Path: "/",
									Backend: v1beta1.IngressBackend{
										ServiceName: name,
										ServicePort: intstr.FromInt(8065),
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// GenerateDeployment returns the deployment spec for Mattermost
func (mattermost *ClusterInstallation) GenerateDeployment(deploymentName, ingressName, containerImage, dbUser, dbPassword, dbName string, externalDB, isLicensed bool, minioURL string) *appsv1.Deployment {
	envVarDB := []corev1.EnvVar{}

	masterDBEnvVar := corev1.EnvVar{
		Name: "MM_CONFIG",
	}

	var initContainers []corev1.Container
	if externalDB {
		masterDBEnvVar.ValueFrom = &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: mattermost.Spec.Database.Secret,
				},
				Key: "DB_CONNECTION_STRING",
			},
		}
	} else {
		masterDBEnvVar.Value = fmt.Sprintf(
			"mysql://%s:%s@tcp(db-mysql-master.%s:3306)/%s?charset=utf8mb4,utf8&readTimeout=30s&writeTimeout=30s",
			dbUser, dbPassword, mattermost.Namespace, dbName,
		)

		envVarDB = append(envVarDB, corev1.EnvVar{
			Name: "MM_SQLSETTINGS_DATASOURCEREPLICAS",
			Value: fmt.Sprintf(
				"%s:%s@tcp(db-mysql.%s:3306)/%s?readTimeout=30s&writeTimeout=30s",
				dbUser, dbPassword, mattermost.Namespace, dbName,
			),
		})

		// Create the init container to check that the DB is up and running
		initContainers = append(initContainers, corev1.Container{
			Name:            "init-check-mysql",
			Image:           "appropriate/curl:latest",
			ImagePullPolicy: corev1.PullIfNotPresent,
			Command: []string{
				"sh", "-c",
				fmt.Sprintf("until curl --max-time 5 http://db-mysql-master.%s:3306; do echo waiting for mysql; sleep 5; done;", mattermost.Namespace),
			},
		})
	}

	envVarDB = append(envVarDB, masterDBEnvVar)

	minioName := fmt.Sprintf("%s-minio", mattermost.Name)

	// Check if custom secret was passed
	if mattermost.Spec.Minio.Secret != "" {
		minioName = mattermost.Spec.Minio.Secret
	}

	minioAccessEnv := &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: minioName,
			},
			Key: "accesskey",
		},
	}

	minioSecretEnv := &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: minioName,
			},
			Key: "secretkey",
		},
	}

	if !mattermost.Spec.Minio.IsExternal() {
		// Create the init container to create the MinIO bucker
		initContainers = append(initContainers, corev1.Container{
			Name:            "create-minio-bucket",
			Image:           "minio/mc:latest",
			ImagePullPolicy: corev1.PullIfNotPresent,
			Command: []string{
				"/bin/sh", "-c",
				fmt.Sprintf("mc config host add localminio http://%s $(MINIO_ACCESS_KEY) $(MINIO_SECRET_KEY) && mc mb localminio/%s -q -p", minioURL, mattermost.Name),
			},
			Env: []corev1.EnvVar{
				{
					Name:      "MINIO_ACCESS_KEY",
					ValueFrom: minioAccessEnv,
				},
				{
					Name:      "MINIO_SECRET_KEY",
					ValueFrom: minioSecretEnv,
				},
			},
		})

		// Create the init container to check that MinIO is up and running
		initContainers = append(initContainers, corev1.Container{
			Name:            "init-check-minio",
			Image:           "appropriate/curl:latest",
			ImagePullPolicy: corev1.PullIfNotPresent,
			Command: []string{
				"sh", "-c",
				fmt.Sprintf("until curl --max-time 5 http://%s/minio/health/ready; do echo waiting for minio; sleep 5; done;", minioURL),
			},
		})
	}

	bucket := mattermost.Name
	if mattermost.Spec.Minio.ExternalBucket != "" {
		bucket = mattermost.Spec.Minio.ExternalBucket
	}

	// Generate Minio config
	envVarMinio := []corev1.EnvVar{
		{
			Name:  "MM_FILESETTINGS_DRIVERNAME",
			Value: "amazons3",
		},
		{
			Name:      "MM_FILESETTINGS_AMAZONS3ACCESSKEYID",
			ValueFrom: minioAccessEnv,
		},
		{
			Name:      "MM_FILESETTINGS_AMAZONS3SECRETACCESSKEY",
			ValueFrom: minioSecretEnv,
		},
		{
			Name:  "MM_FILESETTINGS_AMAZONS3BUCKET",
			Value: bucket,
		},
		{
			Name:  "MM_FILESETTINGS_AMAZONS3ENDPOINT",
			Value: minioURL,
		},
		{
			Name:  "MM_FILESETTINGS_AMAZONS3SSL",
			Value: "false",
		},
	}

	// ES section vars
	envVarES := []corev1.EnvVar{}
	if mattermost.Spec.ElasticSearch.Host != "" {
		envVarES = []corev1.EnvVar{
			{
				Name:  "MM_ELASTICSEARCHSETTINGS_ENABLEINDEXING",
				Value: "true",
			},
			{
				Name:  "MM_ELASTICSEARCHSETTINGS_ENABLESEARCHING",
				Value: "true",
			},
			{
				Name:  "MM_ELASTICSEARCHSETTINGS_CONNECTIONURL",
				Value: mattermost.Spec.ElasticSearch.Host,
			},
			{
				Name:  "MM_ELASTICSEARCHSETTINGS_USERNAME",
				Value: mattermost.Spec.ElasticSearch.UserName,
			},
			{
				Name:  "MM_ELASTICSEARCHSETTINGS_PASSWORD",
				Value: mattermost.Spec.ElasticSearch.Password,
			},
		}
	}

	siteURL := fmt.Sprintf("https://%s", ingressName)
	envVarGeneral := []corev1.EnvVar{
		{
			Name:  "MM_SERVICESETTINGS_SITEURL",
			Value: siteURL,
		},
		{
			Name:  "MM_PLUGINSETTINGS_ENABLEUPLOADS",
			Value: "true",
		},
		{
			Name:  "MM_METRICSSETTINGS_ENABLE",
			Value: "true",
		},
		{
			Name:  "MM_METRICSSETTINGS_LISTENADDRESS",
			Value: ":8067",
		},
	}

	valueSize := strconv.Itoa(DefaultMaxFileSize * SizeMB)
	if !mattermost.Spec.UseServiceLoadBalancer {
		if _, ok := mattermost.Spec.IngressAnnotations["nginx.ingress.kubernetes.io/proxy-body-size"]; ok {
			size := mattermost.Spec.IngressAnnotations["nginx.ingress.kubernetes.io/proxy-body-size"]
			if strings.HasSuffix(size, "M") {
				maxFileSize, _ := strconv.Atoi(strings.TrimSuffix(size, "M"))
				valueSize = strconv.Itoa(maxFileSize * SizeMB)
			} else if strings.HasSuffix(size, "m") {
				maxFileSize, _ := strconv.Atoi(strings.TrimSuffix(size, "m"))
				valueSize = strconv.Itoa(maxFileSize * SizeMB)
			} else if strings.HasSuffix(size, "G") {
				maxFileSize, _ := strconv.Atoi(strings.TrimSuffix(size, "G"))
				valueSize = strconv.Itoa(maxFileSize * SizeGB)
			} else if strings.HasSuffix(size, "g") {
				maxFileSize, _ := strconv.Atoi(strings.TrimSuffix(size, "g"))
				valueSize = strconv.Itoa(maxFileSize * SizeGB)
			}
		}
	}
	envVarGeneral = append(envVarGeneral, corev1.EnvVar{
		Name:  "MM_FILESETTINGS_MAXFILESIZE",
		Value: valueSize,
	})

	// Mattermost License
	volumeLicense := []corev1.Volume{}
	volumeMountLicense := []corev1.VolumeMount{}
	podAnnotations := map[string]string{}
	if isLicensed {
		envVarGeneral = append(envVarGeneral, corev1.EnvVar{
			Name:  "MM_SERVICESETTINGS_LICENSEFILELOCATION",
			Value: "/mattermost-license/license",
		})

		volumeMountLicense = append(volumeMountLicense, corev1.VolumeMount{
			MountPath: "/mattermost-license",
			Name:      "mattermost-license",
			ReadOnly:  true,
		})

		volumeLicense = append(volumeLicense, corev1.Volume{
			Name: "mattermost-license",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: mattermost.Spec.MattermostLicenseSecret,
				},
			},
		})

		clusterEnvVars := []corev1.EnvVar{
			{
				Name:  "MM_CLUSTERSETTINGS_ENABLE",
				Value: "true",
			},
			{
				Name:  "MM_CLUSTERSETTINGS_CLUSTERNAME",
				Value: "production",
			},
		}

		envVarGeneral = append(envVarGeneral, clusterEnvVars...)

		podAnnotations = map[string]string{
			"prometheus.io/scrape": "true",
			"prometheus.io/path":   "/metrics",
			"prometheus.io/port":   "8067",
		}
	}

	// EnvVars Section
	envVars := []corev1.EnvVar{}
	envVars = append(envVars, envVarDB...)
	envVars = append(envVars, envVarMinio...)
	envVars = append(envVars, envVarES...)
	envVars = append(envVars, envVarGeneral...)

	revHistoryLimit := int32(5)
	maxUnavailable := intstr.FromInt(int(mattermost.Spec.Replicas - 1))
	if mattermost.Spec.Replicas == 1 {
		maxUnavailable = intstr.FromInt(1)
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: mattermost.Namespace,
			Labels:    ClusterInstallationLabels(deploymentName),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(mattermost, schema.GroupVersionKind{
					Group:   SchemeGroupVersion.Group,
					Version: SchemeGroupVersion.Version,
					Kind:    "ClusterInstallation",
				}),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxUnavailable: &maxUnavailable,
				},
			},
			RevisionHistoryLimit: &revHistoryLimit,
			Replicas:             &mattermost.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: ClusterInstallationLabels(deploymentName),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      ClusterInstallationLabels(deploymentName),
					Annotations: podAnnotations,
				},
				Spec: corev1.PodSpec{
					InitContainers: initContainers,
					Containers: []corev1.Container{
						{
							Name:                     deploymentName,
							Image:                    containerImage,
							ImagePullPolicy:          corev1.PullAlways,
							TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
							Command:                  []string{"mattermost"},
							Env:                      envVars,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8065,
									Name:          "app",
								},
							},
							ReadinessProbe: &corev1.Probe{
								Handler: corev1.Handler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/api/v4/system/ping",
										Port: intstr.FromInt(8065),
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       5,
								FailureThreshold:    6,
							},
							LivenessProbe: &corev1.Probe{
								Handler: corev1.Handler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/api/v4/system/ping",
										Port: intstr.FromInt(8065),
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       10,
								FailureThreshold:    3,
							},
							VolumeMounts: volumeMountLicense,
							Resources:    mattermost.Spec.Resources,
						},
					},
					Volumes:      volumeLicense,
					Affinity:     mattermost.Spec.Affinity,
					NodeSelector: mattermost.Spec.NodeSelector,
				},
			},
		},
	}
}

// GenerateSecret returns the service for Mattermost
func (mattermost *ClusterInstallation) GenerateSecret(secretName string, labels map[string]string, values map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Labels:    labels,
			Name:      secretName,
			Namespace: mattermost.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(mattermost, schema.GroupVersionKind{
					Group:   SchemeGroupVersion.Group,
					Version: SchemeGroupVersion.Version,
					Kind:    "ClusterInstallation",
				}),
			},
		},
		Data: values,
	}
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

// ClusterInstallationLabels returns the labels for selecting the resources
// belonging to the given mattermost clusterinstallation.
func ClusterInstallationLabels(name string) map[string]string {
	l := ClusterInstallationResourceLabels(name)
	l[ClusterLabel] = name
	l["app"] = "mattermost"

	return l
}

// MySQLLabels returns the labels for selecting the resources
// belonging to the given mysql cluster.
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
