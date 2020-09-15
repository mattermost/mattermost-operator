package mattermost

import (
	"fmt"
	"strconv"
	"strings"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/pkg/apis/mattermost/v1alpha1"
	"github.com/mattermost/mattermost-operator/pkg/components/utils"
	"github.com/mattermost/mattermost-operator/pkg/database"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1beta1 "k8s.io/api/extensions/v1beta1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// GenerateService returns the service for the Mattermost app.
func GenerateService(mattermost *mattermostv1alpha1.ClusterInstallation, serviceName, selectorName string) *corev1.Service {
	baseAnnotations := map[string]string{
		"service.alpha.kubernetes.io/tolerate-unready-endpoints": "true",
	}

	if mattermost.Spec.UseServiceLoadBalancer {
		// Create a LoadBalancer service with additional annotations provided in
		// the Mattermost Spec. The LoadBalancer is directly accessible from
		// outside the cluster thus exposes ports 80 and 443.
		service := newService(mattermost, serviceName, selectorName,
			mergeStringMaps(baseAnnotations, mattermost.Spec.ServiceAnnotations),
		)
		service.Spec.Ports = []corev1.ServicePort{
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
		}
		service.Spec.Type = corev1.ServiceTypeLoadBalancer

		return service
	}

	// Create a headless service which is not directly accessible from outside
	// the cluster and thus exposes a custom port.
	service := newService(mattermost, serviceName, selectorName, baseAnnotations)
	service.Spec.Ports = []corev1.ServicePort{
		{
			Port:       8065,
			TargetPort: intstr.FromString("app"),
		},
	}
	service.Spec.ClusterIP = corev1.ClusterIPNone

	return service
}

// GenerateIngress returns the ingress for the Mattermost app.
func GenerateIngress(mattermost *mattermostv1alpha1.ClusterInstallation, name, ingressName string, ingressAnnotations map[string]string) *v1beta1.Ingress {
	ingress := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: mattermost.Namespace,
			Labels:    mattermost.ClusterInstallationLabels(name),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(mattermost, schema.GroupVersionKind{
					Group:   mattermostv1alpha1.SchemeGroupVersion.Group,
					Version: mattermostv1alpha1.SchemeGroupVersion.Version,
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

	if mattermost.Spec.UseIngressTLS {
		ingress.Spec.TLS = []v1beta1.IngressTLS{
			{
				Hosts:      []string{ingressName},
				SecretName: strings.ReplaceAll(ingressName, ".", "-") + "-tls-cert",
			},
		}
	}

	return ingress
}

// GenerateDeployment returns the deployment for Mattermost app.
func GenerateDeployment(mattermost *mattermostv1alpha1.ClusterInstallation, dbInfo *database.Info, deploymentName, ingressName, containerImage string, minioURL string) *appsv1.Deployment {
	var envVarDB []corev1.EnvVar

	masterDBEnvVar := corev1.EnvVar{
		Name: "MM_CONFIG",
	}

	var initContainers []corev1.Container
	if dbInfo.IsExternal() {
		masterDBEnvVar.ValueFrom = &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: mattermost.Spec.Database.Secret,
				},
				Key: "DB_CONNECTION_STRING",
			},
		}

		if dbInfo.HasDatabaseCheckURL() {
			initContainers = append(initContainers, corev1.Container{
				Name:            "init-check-database",
				Image:           "appropriate/curl:latest",
				ImagePullPolicy: corev1.PullIfNotPresent,
				Env: []corev1.EnvVar{
					{
						Name: "DB_CONNECTION_CHECK_URL",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: mattermost.Spec.Database.Secret,
								},
								Key: "DB_CONNECTION_CHECK_URL",
							},
						},
					},
				},
				Command: []string{
					"sh", "-c",
					"until curl --max-time 5 $DB_CONNECTION_CHECK_URL; do echo waiting for database; sleep 5; done;",
				},
			})
		}
	} else {
		mysqlName := utils.HashWithPrefix("db", mattermost.Name)

		masterDBEnvVar.Value = fmt.Sprintf(
			"mysql://$(MYSQL_USERNAME):$(MYSQL_PASSWORD)@tcp(%s-mysql-master.%s:3306)/%s?charset=utf8mb4,utf8&readTimeout=30s&writeTimeout=30s",
			mysqlName, mattermost.Namespace, dbInfo.DatabaseName,
		)

		operatorEnv := []corev1.EnvVar{
			{
				Name: "MYSQL_USERNAME",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: dbInfo.SecretName,
						},
						Key: "USER",
					},
				},
			},
			{
				Name: "MYSQL_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: dbInfo.SecretName,
						},
						Key: "PASSWORD",
					},
				},
			},
			{
				Name: "MM_SQLSETTINGS_DATASOURCEREPLICAS",
				Value: fmt.Sprintf(
					"$(MYSQL_USERNAME):$(MYSQL_PASSWORD)@tcp(%s-mysql.%s:3306)/%s?readTimeout=30s&writeTimeout=30s",
					mysqlName, mattermost.Namespace, dbInfo.DatabaseName,
				),
			},
		}
		envVarDB = append(envVarDB, operatorEnv...)

		// Create the init container to check that the DB is up and running
		initContainers = append(initContainers, corev1.Container{
			Name:            "init-check-operator-mysql",
			Image:           "appropriate/curl:latest",
			ImagePullPolicy: corev1.PullIfNotPresent,
			Command: []string{
				"sh", "-c",
				fmt.Sprintf("until curl --max-time 5 http://%s-mysql-master.%s:3306; do echo waiting for mysql; sleep 5; done;",
					mysqlName, mattermost.Namespace,
				),
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
		{
			Name:  "MM_CLUSTERSETTINGS_ENABLE",
			Value: "true",
		},
		{
			Name:  "MM_CLUSTERSETTINGS_CLUSTERNAME",
			Value: "production",
		},
	}

	valueSize := strconv.Itoa(defaultMaxFileSize * sizeMB)
	if !mattermost.Spec.UseServiceLoadBalancer {
		if _, ok := mattermost.Spec.IngressAnnotations["nginx.ingress.kubernetes.io/proxy-body-size"]; ok {
			size := mattermost.Spec.IngressAnnotations["nginx.ingress.kubernetes.io/proxy-body-size"]
			if strings.HasSuffix(size, "M") {
				maxFileSize, _ := strconv.Atoi(strings.TrimSuffix(size, "M"))
				valueSize = strconv.Itoa(maxFileSize * sizeMB)
			} else if strings.HasSuffix(size, "m") {
				maxFileSize, _ := strconv.Atoi(strings.TrimSuffix(size, "m"))
				valueSize = strconv.Itoa(maxFileSize * sizeMB)
			} else if strings.HasSuffix(size, "G") {
				maxFileSize, _ := strconv.Atoi(strings.TrimSuffix(size, "G"))
				valueSize = strconv.Itoa(maxFileSize * sizeGB)
			} else if strings.HasSuffix(size, "g") {
				maxFileSize, _ := strconv.Atoi(strings.TrimSuffix(size, "g"))
				valueSize = strconv.Itoa(maxFileSize * sizeGB)
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
	if len(mattermost.Spec.MattermostLicenseSecret) != 0 {
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

	// Merge our custom env vars in.
	envVars = mergeEnvVars(envVars, mattermost.Spec.MattermostEnv)

	revHistoryLimit := int32(defaultRevHistoryLimit)
	maxUnavailable := intstr.FromInt(defaultMaxUnavailable)
	maxSurge := intstr.FromInt(defaultMaxSurge)

	liveness, readiness := setProbes(mattermost.Spec.LivenessProbe, mattermost.Spec.ReadinessProbe)

	replicas := mattermost.Spec.Replicas
	if replicas < 0 {
		replicas = 0
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: mattermost.Namespace,
			Labels:    mattermost.ClusterInstallationLabels(deploymentName),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(mattermost, schema.GroupVersionKind{
					Group:   mattermostv1alpha1.SchemeGroupVersion.Group,
					Version: mattermostv1alpha1.SchemeGroupVersion.Version,
					Kind:    "ClusterInstallation",
				}),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxUnavailable: &maxUnavailable,
					MaxSurge:       &maxSurge,
				},
			},
			RevisionHistoryLimit: &revHistoryLimit,
			Replicas:             &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: mattermostv1alpha1.ClusterInstallationSelectorLabels(deploymentName),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      mattermost.ClusterInstallationLabels(deploymentName),
					Annotations: podAnnotations,
				},
				Spec: corev1.PodSpec{
					InitContainers: initContainers,
					Containers: []corev1.Container{
						{
							Name:                     mattermostv1alpha1.MattermostAppContainerName,
							Image:                    containerImage,
							ImagePullPolicy:          corev1.PullIfNotPresent,
							TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
							Command:                  []string{"mattermost"},
							Env:                      envVars,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8065,
									Name:          "app",
								},
							},
							ReadinessProbe: readiness,
							LivenessProbe:  liveness,
							VolumeMounts:   volumeMountLicense,
							Resources:      mattermost.Spec.Resources,
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

// GenerateSecret returns the secret for Mattermost
func GenerateSecret(mattermost *mattermostv1alpha1.ClusterInstallation, secretName string, labels map[string]string, values map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Labels:    labels,
			Name:      secretName,
			Namespace: mattermost.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(mattermost, schema.GroupVersionKind{
					Group:   mattermostv1alpha1.SchemeGroupVersion.Group,
					Version: mattermostv1alpha1.SchemeGroupVersion.Version,
					Kind:    "ClusterInstallation",
				}),
			},
		},
		Data: values,
	}
}

// newService returns semi-finished service with common parts filled.
// Returned service is expected to be completed by the caller.
func newService(mattermost *mattermostv1alpha1.ClusterInstallation, serviceName, selectorName string, annotations map[string]string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels:    mattermost.ClusterInstallationLabels(serviceName),
			Name:      serviceName,
			Namespace: mattermost.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(mattermost, schema.GroupVersionKind{
					Group:   mattermostv1alpha1.SchemeGroupVersion.Group,
					Version: mattermostv1alpha1.SchemeGroupVersion.Version,
					Kind:    "ClusterInstallation",
				}),
			},
			Annotations: annotations,
		},
		Spec: corev1.ServiceSpec{
			Selector: mattermostv1alpha1.ClusterInstallationSelectorLabels(selectorName),
		},
	}
}
