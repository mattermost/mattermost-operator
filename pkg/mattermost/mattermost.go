package mattermost

import (
	"fmt"
	"strconv"
	"strings"

	rbacv1 "k8s.io/api/rbac/v1"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/apis/mattermost/v1alpha1"
	"github.com/mattermost/mattermost-operator/pkg/components/utils"
	"github.com/mattermost/mattermost-operator/pkg/database"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	SetupJobName                = "mattermost-db-setup"
	WaitForDBSetupContainerName = "init-wait-for-db-setup"
)

var defaultIngressPathType = networkingv1.PathTypeImplementationSpecific

// GenerateService returns the service for the Mattermost app.
func GenerateService(mattermost *mattermostv1alpha1.ClusterInstallation, serviceName, selectorName string) *corev1.Service {
	baseAnnotations := make(map[string]string)

	if mattermost.Spec.UseServiceLoadBalancer {
		// Create a LoadBalancer service with additional annotations provided in
		// the Mattermost Spec. The LoadBalancer is directly accessible from
		// outside the cluster thus exposes ports 80 and 443.
		service := newService(mattermost, serviceName, selectorName,
			mergeStringMaps(baseAnnotations, mattermost.Spec.ServiceAnnotations),
		)
		return configureMattermostLoadBalancerService(service)
	}

	// Create a headless service which is not directly accessible from outside
	// the cluster and thus exposes a custom port.
	service := newService(mattermost, serviceName, selectorName, baseAnnotations)
	return configureMattermostService(service)
}

// GenerateIngress returns the ingress for the Mattermost app.
func GenerateIngress(mattermost *mattermostv1alpha1.ClusterInstallation, name, ingressName string, ingressAnnotations map[string]string) *networkingv1.Ingress {
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: mattermost.Namespace,
			Labels:    mattermost.ClusterInstallationLabels(name),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(mattermost, schema.GroupVersionKind{
					Group:   mattermostv1alpha1.GroupVersion.Group,
					Version: mattermostv1alpha1.GroupVersion.Version,
					Kind:    "ClusterInstallation",
				}),
			},
			Annotations: ingressAnnotations,
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: ingressName,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path: "/",
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: mattermost.Name,
											Port: networkingv1.ServiceBackendPort{
												Number: 8065,
											},
										},
									},
									PathType: &defaultIngressPathType,
								},
							},
						},
					},
				},
			},
		},
	}

	if mattermost.Spec.UseIngressTLS {
		ingress.Spec.TLS = []networkingv1.IngressTLS{
			{
				Hosts:      []string{ingressName},
				SecretName: strings.ReplaceAll(ingressName, ".", "-") + "-tls-cert",
			},
		}
	}

	return ingress
}

// GenerateDeployment returns the deployment for Mattermost app.
func GenerateDeployment(mattermost *mattermostv1alpha1.ClusterInstallation, dbInfo *database.Info, deploymentName, ingressName, serviceAccountName, containerImage string, minioURL string) *appsv1.Deployment {
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

		if dbInfo.HasReaderEndpoints() {
			envVarDB = append(envVarDB, corev1.EnvVar{
				Name: "MM_SQLSETTINGS_DATASOURCEREPLICAS",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: mattermost.Spec.Database.Secret,
						},
						Key: "MM_SQLSETTINGS_DATASOURCEREPLICAS",
					},
				},
			})
		}

		if dbInfo.HasDatabaseCheckURL() {
			dbCheckContainer := getDBCheckInitContainer(dbInfo.SecretName, dbInfo.ExternalDBType)
			if dbCheckContainer != nil {
				initContainers = append(initContainers, *dbCheckContainer)
			}
		}
	} else {
		mysqlName := utils.HashWithPrefix("db", mattermost.Name)

		masterDBEnvVar.Value = fmt.Sprintf(
			"mysql://$(MYSQL_USERNAME):$(MYSQL_PASSWORD)@tcp(%s-mysql-master.%s.svc.cluster.local:3306)/%s?charset=utf8mb4,utf8&readTimeout=30s&writeTimeout=30s",
			mysqlName, mattermost.Namespace, dbInfo.DatabaseName,
		)

		mysqlOperatorEnv := []corev1.EnvVar{
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
					"$(MYSQL_USERNAME):$(MYSQL_PASSWORD)@tcp(%s-mysql.%s.svc.cluster.local:3306)/%s?readTimeout=30s&writeTimeout=30s",
					mysqlName, mattermost.Namespace, dbInfo.DatabaseName,
				),
			},
		}
		envVarDB = append(envVarDB, mysqlOperatorEnv...)

		// Create the init container to check that the DB is up and running
		initContainers = append(initContainers, corev1.Container{
			Name:            "init-check-operator-mysql",
			Image:           "appropriate/curl:latest",
			ImagePullPolicy: corev1.PullIfNotPresent,
			Command: []string{
				"sh", "-c",
				fmt.Sprintf("until curl --max-time 5 http://%s-mysql-master.%s.svc.cluster.local:3306; do echo waiting for mysql; sleep 5; done;",
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
			Image:           "minio/mc:RELEASE.2025-04-16T18-13-26Z",
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

	// TODO: DB setup job is temporarily disabled as `mattermost version` command
	// does not account for the custom configuration
	// Add init container to wait for DB setup job to complete
	// initContainers = append(initContainers, waitForSetupJobContainer())

	// ES section vars
	envVarES := []corev1.EnvVar{}
	if mattermost.Spec.ElasticSearch.Host != "" {
		envVarES = elasticSearchEnvVars(
			mattermost.Spec.ElasticSearch.Host,
			mattermost.Spec.ElasticSearch.UserName,
			mattermost.Spec.ElasticSearch.Password,
		)
	}

	envVarGeneral := generalMattermostEnvVars(siteURLFromHost(ingressName))

	valueSize := strconv.Itoa(defaultMaxFileSize * sizeMB)
	if !mattermost.Spec.UseServiceLoadBalancer {
		valueSize = determineMaxBodySize(mattermost.Spec.IngressAnnotations, valueSize)
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
		env, vMount, volume, annotations := mattermostLicenceConfig(mattermost.Spec.MattermostLicenseSecret)
		envVarGeneral = append(envVarGeneral, env)
		volumeMountLicense = append(volumeMountLicense, vMount)
		volumeLicense = append(volumeLicense, volume)
		podAnnotations = annotations
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
					Group:   mattermostv1alpha1.GroupVersion.Group,
					Version: mattermostv1alpha1.GroupVersion.Version,
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
					ServiceAccountName:                 serviceAccountName,
					TerminationGracePeriodSeconds:      mattermost.Spec.PodTerminationGracePeriodSeconds,
					InitContainers:                     initContainers,
					Containers: []corev1.Container{
						{
							Name:                     mattermostv1alpha1.MattermostAppContainerName,
							Image:                    containerImage,
							ImagePullPolicy:          mattermost.Spec.ImagePullPolicy,
							TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
							Command:                  []string{"mattermost"},
							Env:                      envVars,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8065,
									Name:          "app",
								},
								{
									ContainerPort: 8067,
									Name:          "metrics",
								},
							},
							ReadinessProbe: readiness,
							LivenessProbe:  liveness,
							VolumeMounts:   volumeMountLicense,
							Resources:      mattermost.Spec.Resources,
						},
					},
					Volumes:                        volumeLicense,
					Affinity:                       mattermost.Spec.Affinity,
					NodeSelector:                   mattermost.Spec.NodeSelector,
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
					Group:   mattermostv1alpha1.GroupVersion.Group,
					Version: mattermostv1alpha1.GroupVersion.Version,
					Kind:    "ClusterInstallation",
				}),
			},
		},
		Data: values,
	}
}

// GenerateServiceAccount returns the Service Account for Mattermost
func GenerateServiceAccount(mattermost *mattermostv1alpha1.ClusterInstallation, saName string) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:            saName,
			Namespace:       mattermost.Namespace,
			OwnerReferences: ClusterInstallationOwnerReference(mattermost),
		},
	}
}

// GenerateRole returns the Role for Mattermost
func GenerateRole(mattermost *mattermostv1alpha1.ClusterInstallation, roleName string) *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:            roleName,
			Namespace:       mattermost.Namespace,
			OwnerReferences: ClusterInstallationOwnerReference(mattermost),
		},
		Rules: mattermostRolePermissions(),
	}
}

// GenerateRoleBinding returns the RoleBinding for Mattermost
func GenerateRoleBinding(mattermost *mattermostv1alpha1.ClusterInstallation, roleName, saName string) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:            roleName,
			Namespace:       mattermost.Namespace,
			OwnerReferences: ClusterInstallationOwnerReference(mattermost),
		},
		Subjects: []rbacv1.Subject{
			{Kind: "ServiceAccount", Name: saName, Namespace: mattermost.Namespace},
		},
		RoleRef: rbacv1.RoleRef{Kind: "Role", Name: roleName},
	}
}

func ClusterInstallationOwnerReference(mattermost *mattermostv1alpha1.ClusterInstallation) []metav1.OwnerReference {
	return []metav1.OwnerReference{
		*metav1.NewControllerRef(mattermost, schema.GroupVersionKind{
			Group:   mattermostv1alpha1.GroupVersion.Group,
			Version: mattermostv1alpha1.GroupVersion.Version,
			Kind:    "ClusterInstallation",
		}),
	}
}

// newService returns semi-finished service with common parts filled.
// Returned service is expected to be completed by the caller.
func newService(mattermost *mattermostv1alpha1.ClusterInstallation, serviceName, selectorName string, annotations map[string]string) *corev1.Service {
	// Default to false if not specified
	publishNotReady := false
	if mattermost.Spec.PublishNotReadyAddresses != nil {
		publishNotReady = *mattermost.Spec.PublishNotReadyAddresses
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels:    mattermost.ClusterInstallationLabels(serviceName),
			Name:      serviceName,
			Namespace: mattermost.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(mattermost, schema.GroupVersionKind{
					Group:   mattermostv1alpha1.GroupVersion.Group,
					Version: mattermostv1alpha1.GroupVersion.Version,
					Kind:    "ClusterInstallation",
				}),
			},
			Annotations: annotations,
		},
		Spec: corev1.ServiceSpec{
			Selector:                 mattermostv1alpha1.ClusterInstallationSelectorLabels(selectorName),
			PublishNotReadyAddresses: publishNotReady,
		},
	}
}
