package v1alpha1

import (
	// "errors"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1beta1 "k8s.io/api/extensions/v1beta1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// GenerateGreenService returns the Green service for Mattermost
func (mattermost *ClusterInstallation) GenerateGreenService() *corev1.Service {
	svcAnnotations := map[string]string{
		"service.alpha.kubernetes.io/tolerate-unready-endpoints": "true",
	}
	if mattermost.Spec.UseServiceLoadBalancer {
		for k, v := range mattermost.Spec.ServiceAnnotations {
			svcAnnotations[k] = v
		}

		return &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Labels:    ClusterInstallationLabels(mattermost.Spec.BlueGreen.GreenInstallationName),
				Name:      mattermost.Spec.BlueGreen.GreenInstallationName,
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
				Selector: ClusterInstallationLabels(mattermost.Spec.BlueGreen.GreenInstallationName),
				Type:     corev1.ServiceTypeLoadBalancer,
			},
		}
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels:    ClusterInstallationLabels(mattermost.Spec.BlueGreen.GreenInstallationName),
			Name:      mattermost.Spec.BlueGreen.GreenInstallationName,
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
			Selector:  ClusterInstallationLabels(mattermost.Spec.BlueGreen.GreenInstallationName),
			ClusterIP: corev1.ClusterIPNone,
		},
	}
}

// GenerateBlueService returns the Green service for Mattermost
func (mattermost *ClusterInstallation) GenerateBlueService() *corev1.Service {
	svcAnnotations := map[string]string{
		"service.alpha.kubernetes.io/tolerate-unready-endpoints": "true",
	}
	if mattermost.Spec.UseServiceLoadBalancer {
		for k, v := range mattermost.Spec.ServiceAnnotations {
			svcAnnotations[k] = v
		}

		return &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Labels:    ClusterInstallationLabels(mattermost.Spec.BlueGreen.BlueInstallationName),
				Name:      mattermost.Spec.BlueGreen.BlueInstallationName,
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
				Selector: ClusterInstallationLabels(mattermost.Spec.BlueGreen.BlueInstallationName),
				Type:     corev1.ServiceTypeLoadBalancer,
			},
		}
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels:    ClusterInstallationLabels(mattermost.Spec.BlueGreen.BlueInstallationName),
			Name:      mattermost.Spec.BlueGreen.BlueInstallationName,
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
			Selector:  ClusterInstallationLabels(mattermost.Spec.BlueGreen.BlueInstallationName),
			ClusterIP: corev1.ClusterIPNone,
		},
	}
}

// GenerateGreenIngress returns the green ingress for Mattermost
func (mattermost *ClusterInstallation) GenerateGreenIngress() *v1beta1.Ingress {
	return &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mattermost.Spec.BlueGreen.GreenInstallationName,
			Namespace: mattermost.Namespace,
			Labels:    ClusterInstallationLabels(mattermost.Spec.BlueGreen.GreenInstallationName),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(mattermost, schema.GroupVersionKind{
					Group:   SchemeGroupVersion.Group,
					Version: SchemeGroupVersion.Version,
					Kind:    "ClusterInstallation",
				}),
			},
			Annotations: mattermost.Spec.IngressAnnotations,
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{
				{
					Host: mattermost.Spec.IngressName,
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{
								{
									Path: "/",
									Backend: v1beta1.IngressBackend{
										ServiceName: mattermost.Spec.BlueGreen.GreenInstallationName,
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

// GenerateBlueIngress returns the green ingress for Mattermost
func (mattermost *ClusterInstallation) GenerateBlueIngress() *v1beta1.Ingress {
	return &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mattermost.Spec.BlueGreen.BlueInstallationName,
			Namespace: mattermost.Namespace,
			Labels:    ClusterInstallationLabels(mattermost.Spec.BlueGreen.BlueInstallationName),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(mattermost, schema.GroupVersionKind{
					Group:   SchemeGroupVersion.Group,
					Version: SchemeGroupVersion.Version,
					Kind:    "ClusterInstallation",
				}),
			},
			Annotations: mattermost.Spec.IngressAnnotations,
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{
				{
					Host: mattermost.Spec.IngressName,
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{
								{
									Path: "/",
									Backend: v1beta1.IngressBackend{
										ServiceName: mattermost.Spec.BlueGreen.BlueInstallationName,
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

// GenerateGreenDeployment returns the deployment spec for Mattermost
func (mattermost *ClusterInstallation) GenerateGreenDeployment(dbUser, dbPassword string, externalDB, isLicensed bool, minioService string) *appsv1.Deployment {
	envVarDB := []corev1.EnvVar{}

	masterDBEnvVar := corev1.EnvVar{
		Name: "MM_CONFIG",
	}
	var initContainers []corev1.Container
	if externalDB {
		masterDBEnvVar.ValueFrom = &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: mattermost.Spec.Database.ExternalSecret,
				},
				Key: "externalDB",
			},
		}
	} else {
		masterDBEnvVar.Value = fmt.Sprintf(
			"mysql://%s:%s@tcp(db-mysql-master.%s:3306)/mattermost?charset=utf8mb4,utf8&readTimeout=30s&writeTimeout=30s",
			dbUser, dbPassword, mattermost.Namespace,
		)

		envVarDB = append(envVarDB, corev1.EnvVar{
			Name: "MM_SQLSETTINGS_DATASOURCEREPLICAS",
			Value: fmt.Sprintf(
				"%s:%s@tcp(db-mysql-nodes.%s:3306)/mattermost?readTimeout=30s&writeTimeout=30s",
				dbUser, dbPassword, mattermost.Namespace,
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

	// Create the init container to check that MinIO is up and running
	initContainers = append(initContainers, corev1.Container{
		Name:            "init-check-minio",
		Image:           "appropriate/curl:latest",
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command: []string{
			"sh", "-c",
			fmt.Sprintf("until curl --max-time 5 http://%s/minio/health/ready; do echo waiting for minio; sleep 5; done;", minioService),
		},
	})

	minioName := fmt.Sprintf("%s-minio", mattermost.Name)
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
	// Create the init container to create the MinIO bucker
	initContainers = append(initContainers, corev1.Container{
		Name:            "create-minio-bucket",
		Image:           "minio/mc:latest",
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command: []string{
			"/bin/sh", "-c",
			fmt.Sprintf("mc config host add localminio http://%s $(MINIO_ACCESS_KEY) $(MINIO_SECRET_KEY) && mc mb localminio/%s -q -p", minioService, mattermost.Name),
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
			Value: mattermost.Name,
		},
		{
			Name:  "MM_FILESETTINGS_AMAZONS3ENDPOINT",
			Value: minioService,
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

	siteURL := fmt.Sprintf("https://%s", mattermost.Spec.BlueGreen.GreenIngressName)
	envVarGeneral := []corev1.EnvVar{
		{
			Name:  "MM_SERVICESETTINGS_SITEURL",
			Value: siteURL,
		},
		{
			Name:  "MM_PLUGINSETTINGS_ENABLEUPLOADS",
			Value: "true",
		},
	}

	// Mattermost License
	volumeLicense := []corev1.Volume{}
	volumeMountLicense := []corev1.VolumeMount{}
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
			Name:      mattermost.Spec.BlueGreen.GreenInstallationName,
			Namespace: mattermost.Namespace,
			Labels:    ClusterInstallationLabels(mattermost.Spec.BlueGreen.GreenInstallationName),
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
				MatchLabels: ClusterInstallationLabels(mattermost.Spec.BlueGreen.GreenInstallationName),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ClusterInstallationLabels(mattermost.Spec.BlueGreen.GreenInstallationName),
				},
				Spec: corev1.PodSpec{
					InitContainers: initContainers,
					Containers: []corev1.Container{
						{
							Image:                    mattermost.GetBlueGreenImageName("green"),
							Name:                     mattermost.Spec.BlueGreen.GreenInstallationName,
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
					Volumes: volumeLicense,
				},
			},
		},
	}
}

// GenerateGreenDeployment returns the deployment spec for Mattermost
func (mattermost *ClusterInstallation) GenerateBlueDeployment(dbUser, dbPassword string, externalDB, isLicensed bool, minioService string) *appsv1.Deployment {
	envVarDB := []corev1.EnvVar{}

	masterDBEnvVar := corev1.EnvVar{
		Name: "MM_CONFIG",
	}
	var initContainers []corev1.Container
	if externalDB {
		masterDBEnvVar.ValueFrom = &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: mattermost.Spec.Database.ExternalSecret,
				},
				Key: "externalDB",
			},
		}
	} else {
		masterDBEnvVar.Value = fmt.Sprintf(
			"mysql://%s:%s@tcp(db-mysql-master.%s:3306)/mattermost?charset=utf8mb4,utf8&readTimeout=30s&writeTimeout=30s",
			dbUser, dbPassword, mattermost.Namespace,
		)

		envVarDB = append(envVarDB, corev1.EnvVar{
			Name: "MM_SQLSETTINGS_DATASOURCEREPLICAS",
			Value: fmt.Sprintf(
				"%s:%s@tcp(db-mysql-nodes.%s:3306)/mattermost?readTimeout=30s&writeTimeout=30s",
				dbUser, dbPassword, mattermost.Namespace,
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

	// Create the init container to check that MinIO is up and running
	initContainers = append(initContainers, corev1.Container{
		Name:            "init-check-minio",
		Image:           "appropriate/curl:latest",
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command: []string{
			"sh", "-c",
			fmt.Sprintf("until curl --max-time 5 http://%s/minio/health/ready; do echo waiting for minio; sleep 5; done;", minioService),
		},
	})

	minioName := fmt.Sprintf("%s-minio", mattermost.Name)
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
	// Create the init container to create the MinIO bucker
	initContainers = append(initContainers, corev1.Container{
		Name:            "create-minio-bucket",
		Image:           "minio/mc:latest",
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command: []string{
			"/bin/sh", "-c",
			fmt.Sprintf("mc config host add localminio http://%s $(MINIO_ACCESS_KEY) $(MINIO_SECRET_KEY) && mc mb localminio/%s -q -p", minioService, mattermost.Name),
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
			Value: mattermost.Name,
		},
		{
			Name:  "MM_FILESETTINGS_AMAZONS3ENDPOINT",
			Value: minioService,
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

	siteURL := fmt.Sprintf("https://%s", mattermost.Spec.BlueGreen.BlueIngressName)
	envVarGeneral := []corev1.EnvVar{
		{
			Name:  "MM_SERVICESETTINGS_SITEURL",
			Value: siteURL,
		},
		{
			Name:  "MM_PLUGINSETTINGS_ENABLEUPLOADS",
			Value: "true",
		},
	}

	// Mattermost License
	volumeLicense := []corev1.Volume{}
	volumeMountLicense := []corev1.VolumeMount{}
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
			Name:      mattermost.Spec.BlueGreen.BlueInstallationName,
			Namespace: mattermost.Namespace,
			Labels:    ClusterInstallationLabels(mattermost.Spec.BlueGreen.BlueInstallationName),
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
				MatchLabels: ClusterInstallationLabels(mattermost.Spec.BlueGreen.BlueInstallationName),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ClusterInstallationLabels(mattermost.Spec.BlueGreen.BlueInstallationName),
				},
				Spec: corev1.PodSpec{
					InitContainers: initContainers,
					Containers: []corev1.Container{
						{
							Image:                    mattermost.GetBlueGreenImageName("blue"),
							Name:                     mattermost.Spec.BlueGreen.BlueInstallationName,
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
					Volumes: volumeLicense,
				},
			},
		},
	}
}

// GetBlueGreenImageName returns the container image name that matches the spec of the
// ClusterInstallation.
func (mattermost *ClusterInstallation) GetBlueGreenImageName(deployment string) string {
	if deployment == "green" {
		return fmt.Sprintf("%s:%s", mattermost.Spec.Image, mattermost.Spec.BlueGreen.GreenVersion)
	} else if deployment == "blue" {
		return fmt.Sprintf("%s:%s", mattermost.Spec.Image, mattermost.Spec.BlueGreen.BlueVersion)
	}
	return "Cannot return BlueGreen image name."
}

// GetBlueGreenInstallationName returns the BlueGreen installation name based on the deployment type
func (mattermost *ClusterInstallation) GetBlueGreenInstallationName(deployment string) string {
	if deployment == "green" {
		return mattermost.Spec.BlueGreen.GreenInstallationName
	} else if deployment == "blue" {
		return mattermost.Spec.BlueGreen.BlueInstallationName
	}
	return "Cannot return BlueGreen installation name."
}
