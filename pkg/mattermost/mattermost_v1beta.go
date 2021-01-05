package mattermost

import (
	"fmt"
	"strconv"
	"strings"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	pkgUtils "github.com/mattermost/mattermost-operator/pkg/utils"

	rbacv1 "k8s.io/api/rbac/v1"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/apis/mattermost/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1beta1 "k8s.io/api/networking/v1beta1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type DatabaseConfig interface {
	EnvVars(mattermost *mmv1beta.Mattermost) []corev1.EnvVar
	InitContainers(mattermost *mmv1beta.Mattermost) []corev1.Container
}

type FileStoreConfig interface {
	InitContainers(mattermost *mmv1beta.Mattermost) []corev1.Container
}

// GenerateService returns the service for the Mattermost app.
func GenerateServiceV1Beta(mattermost *mmv1beta.Mattermost) *corev1.Service {
	baseAnnotations := map[string]string{
		"service.alpha.kubernetes.io/tolerate-unready-endpoints": "true",
	}

	if mattermost.Spec.UseServiceLoadBalancer {
		// Create a LoadBalancer service with additional annotations provided in
		// the Mattermost Spec. The LoadBalancer is directly accessible from
		// outside the cluster thus exposes ports 80 and 443.
		service := newServiceV1Beta(mattermost, mergeStringMaps(baseAnnotations, mattermost.Spec.ServiceAnnotations))
		return configureMattermostLoadBalancerService(service)
	}

	// Create a headless service which is not directly accessible from outside
	// the cluster and thus exposes a custom port.
	service := newServiceV1Beta(mattermost, baseAnnotations)
	return configureMattermostService(service)
}

func configureMattermostLoadBalancerService(service *corev1.Service) *corev1.Service {
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

func configureMattermostService(service *corev1.Service) *corev1.Service {
	service.Spec.Ports = []corev1.ServicePort{
		{
			Port:       8065,
			Name:       "app",
			TargetPort: intstr.FromString("app"),
		},
		{
			Port:       8067,
			Name:       "metrics",
			TargetPort: intstr.FromString("metrics"),
		},
	}
	service.Spec.ClusterIP = corev1.ClusterIPNone

	return service
}

// GenerateIngress returns the ingress for the Mattermost app.
func GenerateIngressV1Beta(mattermost *mmv1beta.Mattermost, name, ingressName string, ingressAnnotations map[string]string) *v1beta1.Ingress {
	ingress := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       mattermost.Namespace,
			Labels:          mattermost.MattermostLabels(name),
			OwnerReferences: MattermostOwnerReference(mattermost),
			Annotations:     ingressAnnotations,
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
func GenerateDeploymentV1Beta(mattermost *mmv1beta.Mattermost, db DatabaseConfig, fileStore *FileStoreInfo, deploymentName, ingressName, serviceAccountName, containerImage string) *appsv1.Deployment {
	// DB
	envVarDB := db.EnvVars(mattermost)
	initContainers := db.InitContainers(mattermost)

	// File Store
	envVarFileStore := fileStoreEnvVars(fileStore)
	initContainers = append(initContainers, fileStore.config.InitContainers(mattermost)...)

	// TODO: DB setup job is temporarily disabled as `mattermost version` command
	// does not account for the custom configuration
	// Add init container to wait for DB setup job to complete
	//initContainers = append(initContainers, waitForSetupJobContainer())

	// ES section vars
	envVarES := []corev1.EnvVar{}
	if mattermost.Spec.ElasticSearch.Host != "" {
		envVarES = elasticSearchEnvVars(
			mattermost.Spec.ElasticSearch.Host,
			mattermost.Spec.ElasticSearch.UserName,
			mattermost.Spec.ElasticSearch.Password,
		)
	}

	// General settings
	siteURL := fmt.Sprintf("https://%s", ingressName)
	envVarGeneral := generalMattermostEnvVars(siteURL)

	// Determine max file size
	bodySize := strconv.Itoa(defaultMaxFileSize * sizeMB)
	if !mattermost.Spec.UseServiceLoadBalancer {
		bodySize = determineMaxBodySize(mattermost.Spec.IngressAnnotations, bodySize)
	}
	envVarGeneral = append(envVarGeneral, corev1.EnvVar{
		Name:  "MM_FILESETTINGS_MAXFILESIZE",
		Value: bodySize,
	})

	// Prepare volumes
	volumes := mattermost.Spec.Volumes
	volumeMounts := mattermost.Spec.VolumeMounts
	podAnnotations := map[string]string{}

	// Mattermost License
	if len(mattermost.Spec.LicenseSecret) != 0 {
		env, vMount, volume, annotations := mattermostLicenceConfig(mattermost.Spec.LicenseSecret)
		envVarGeneral = append(envVarGeneral, env)
		volumeMounts = append(volumeMounts, vMount)
		volumes = append(volumes, volume)
		podAnnotations = annotations
	}

	// Concat EnvVars
	envVars := []corev1.EnvVar{}
	envVars = append(envVars, envVarDB...)
	envVars = append(envVars, envVarFileStore...)
	envVars = append(envVars, envVarES...)
	envVars = append(envVars, envVarGeneral...)

	// Merge our custom env vars in.
	envVars = mergeEnvVars(envVars, mattermost.Spec.MattermostEnv)

	maxUnavailable := intstr.FromInt(defaultMaxUnavailable)
	maxSurge := intstr.FromInt(defaultMaxSurge)

	liveness, readiness := setProbes(mattermost.Spec.Probes.LivenessProbe, mattermost.Spec.Probes.ReadinessProbe)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            deploymentName,
			Namespace:       mattermost.Namespace,
			Labels:          mattermost.MattermostLabels(deploymentName),
			OwnerReferences: MattermostOwnerReference(mattermost),
		},
		Spec: appsv1.DeploymentSpec{
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxUnavailable: &maxUnavailable,
					MaxSurge:       &maxSurge,
				},
			},
			RevisionHistoryLimit: pkgUtils.NewInt32(defaultRevHistoryLimit),
			Replicas:             mattermost.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: mmv1beta.MattermostSelectorLabels(deploymentName),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      mattermost.MattermostLabels(deploymentName),
					Annotations: podAnnotations,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: serviceAccountName,
					InitContainers:     initContainers,
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
							VolumeMounts:   volumeMounts,
							Resources:      mattermost.Spec.Scheduling.Resources,
						},
					},
					Volumes:      volumes,
					Affinity:     mattermost.Spec.Scheduling.Affinity,
					NodeSelector: mattermost.Spec.Scheduling.NodeSelector,
				},
			},
		},
	}
}

// GenerateSecret returns the secret for Mattermost
func GenerateSecretV1Beta(mattermost *mmv1beta.Mattermost, secretName string, labels map[string]string, values map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Labels:          labels,
			Name:            secretName,
			Namespace:       mattermost.Namespace,
			OwnerReferences: MattermostOwnerReference(mattermost),
		},
		Data: values,
	}
}

// GenerateServiceAccount returns the Service Account for Mattermost
func GenerateServiceAccountV1Beta(mattermost *mmv1beta.Mattermost, saName string) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:            saName,
			Namespace:       mattermost.Namespace,
			OwnerReferences: MattermostOwnerReference(mattermost),
		},
	}
}

// GenerateRole returns the Role for Mattermost
func GenerateRoleV1Beta(mattermost *mmv1beta.Mattermost, roleName string) *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:            roleName,
			Namespace:       mattermost.Namespace,
			OwnerReferences: MattermostOwnerReference(mattermost),
		},
		Rules: mattermostRolePermissions(),
	}
}

func mattermostRolePermissions() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		{
			Verbs:         []string{"get", "list", "watch"},
			APIGroups:     []string{"batch"},
			Resources:     []string{"jobs"},
			ResourceNames: []string{SetupJobName},
		},
	}
}

// GenerateRoleBinding returns the RoleBinding for Mattermost
func GenerateRoleBindingV1Beta(mattermost *mmv1beta.Mattermost, roleName, saName string) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:            roleName,
			Namespace:       mattermost.Namespace,
			OwnerReferences: MattermostOwnerReference(mattermost),
		},
		Subjects: []rbacv1.Subject{
			{Kind: "ServiceAccount", Name: saName, Namespace: mattermost.Namespace},
		},
		RoleRef: rbacv1.RoleRef{Kind: "Role", Name: roleName},
	}
}

func MattermostOwnerReference(mattermost *mmv1beta.Mattermost) []metav1.OwnerReference {
	return []metav1.OwnerReference{
		*metav1.NewControllerRef(mattermost, schema.GroupVersionKind{
			Group:   mmv1beta.GroupVersion.Group,
			Version: mmv1beta.GroupVersion.Version,
			Kind:    "Mattermost",
		}),
	}
}

// newService returns semi-finished service with common parts filled.
// Returned service is expected to be completed by the caller.
func newServiceV1Beta(mattermost *mmv1beta.Mattermost, annotations map[string]string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels:          mattermost.MattermostLabels(mattermost.Name),
			Name:            mattermost.Name,
			Namespace:       mattermost.Namespace,
			OwnerReferences: MattermostOwnerReference(mattermost),
			Annotations:     annotations,
		},
		Spec: corev1.ServiceSpec{
			Selector: mmv1beta.MattermostSelectorLabels(mattermost.Name),
		},
	}
}

func mattermostLicenceConfig(secret string) (corev1.EnvVar, corev1.VolumeMount, corev1.Volume, map[string]string) {
	envVar := corev1.EnvVar{
		Name:  "MM_SERVICESETTINGS_LICENSEFILELOCATION",
		Value: "/mattermost-license/license",
	}
	volumeMount := corev1.VolumeMount{
		MountPath: "/mattermost-license",
		Name:      "mattermost-license",
		ReadOnly:  true,
	}
	volume := corev1.Volume{
		Name: "mattermost-license",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: secret,
			},
		},
	}
	annotations := map[string]string{
		"prometheus.io/scrape": "true",
		"prometheus.io/path":   "/metrics",
		"prometheus.io/port":   "8067",
	}
	return envVar, volumeMount, volume, annotations
}
