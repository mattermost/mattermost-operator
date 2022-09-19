package mattermost

import (
	"strconv"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	pkgUtils "github.com/mattermost/mattermost-operator/pkg/utils"

	rbacv1 "k8s.io/api/rbac/v1"

	mattermostv1alpha1 "github.com/mattermost/mattermost-operator/apis/mattermost/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	ingressClassAnnotation = "kubernetes.io/ingress.class"
)

type DatabaseConfig interface {
	EnvVars(mattermost *mmv1beta.Mattermost) []corev1.EnvVar
	InitContainers(mattermost *mmv1beta.Mattermost) []corev1.Container
}

type FileStoreConfig interface {
	InitContainers(mattermost *mmv1beta.Mattermost) []corev1.Container
}

// GenerateServiceV1Beta returns the service for the Mattermost app.
func GenerateServiceV1Beta(mattermost *mmv1beta.Mattermost) *corev1.Service {
	baseAnnotations := map[string]string{
		"service.alpha.kubernetes.io/tolerate-unready-endpoints": "true",
	}

	if mattermost.Spec.AWSLoadBalancerController.Enable {
		// Create a NodePort service because the ALB requires it
		service := newServiceV1Beta(mattermost, mergeStringMaps(baseAnnotations, mattermost.Spec.ServiceAnnotations))
		return configureMattermostServiceNodePort(service)
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
			Name:        "http",
			Port:        80,
			AppProtocol: pkgUtils.NewString("http"),
			TargetPort:  intstr.FromString("app"),
		},
		{
			Name:        "https",
			Port:        443,
			AppProtocol: pkgUtils.NewString("https"),
			TargetPort:  intstr.FromString("app"),
		},
	}
	service.Spec.Type = corev1.ServiceTypeLoadBalancer

	return service
}

func configureMattermostService(service *corev1.Service) *corev1.Service {
	service.Spec.Ports = []corev1.ServicePort{
		{
			Port:        8065,
			Name:        "app",
			AppProtocol: pkgUtils.NewString("http"),
			TargetPort:  intstr.FromString("app"),
		},
		{
			Port:        8067,
			Name:        "metrics",
			AppProtocol: pkgUtils.NewString("http"),
			TargetPort:  intstr.FromString("metrics"),
		},
	}
	service.Spec.ClusterIP = corev1.ClusterIPNone
	service.Spec.Type = corev1.ServiceTypeClusterIP

	return service
}

func configureMattermostServiceNodePort(service *corev1.Service) *corev1.Service {
	service.Spec.Ports = []corev1.ServicePort{
		{
			Port:        8065,
			Name:        "app",
			AppProtocol: pkgUtils.NewString("http"),
			TargetPort:  intstr.FromString("app"),
		},
		{
			Port:        8067,
			Name:        "metrics",
			AppProtocol: pkgUtils.NewString("http"),
			TargetPort:  intstr.FromString("metrics"),
		},
	}
	service.Spec.Type = corev1.ServiceTypeNodePort

	return service
}

// GenerateIngressV1Beta returns the ingress for the Mattermost app.
func GenerateIngressV1Beta(mattermost *mmv1beta.Mattermost) *networkingv1.Ingress {
	ingressAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/proxy-body-size": "1000M",
	}
	// This is somewhat tricky as you cannot set both ingress.class annotation
	// and spec.IngressClassName when creating Ingress.
	// At the same time older Nginx do not recognize spec.IngressClassName,
	// so we cannot transition to using only new field.
	// Both can exist if one is added on update therefore we leave the option of
	// specifying ingress.class annotation in IngressAnnotations.
	if mattermost.GetIngressClass() == nil {
		// TODO: for Operator v2 we should change the default behavior to do not to set this annotation.
		ingressAnnotations[ingressClassAnnotation] = "nginx"
	}

	for k, v := range mattermost.GetIngresAnnotations() {
		ingressAnnotations[k] = v
	}

	hosts := mattermost.GetIngressHostNames()

	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:            mattermost.Name,
			Namespace:       mattermost.Namespace,
			Labels:          mattermost.MattermostLabels(mattermost.Name),
			OwnerReferences: MattermostOwnerReference(mattermost),
			Annotations:     ingressAnnotations,
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: mattermost.GetIngressClass(),
			Rules:            makeIngressRules(hosts, mattermost),
		},
	}

	if mattermost.GetIngressTLSSecret() != "" {
		ingress.Spec.TLS = []networkingv1.IngressTLS{
			{
				// TODO: for now we use the same secret for all hosts.
				// We can easily extend this in the future by adding another filed to IngressHost.
				Hosts:      hosts,
				SecretName: mattermost.GetIngressTLSSecret(),
			},
		}
	}

	return ingress
}

// GenerateIngressALBIngressV1Beta returns the AWS ALB ingress for the Mattermost app.
func GenerateALBIngressV1Beta(mattermost *mmv1beta.Mattermost) *networkingv1.Ingress {
	ingressAnnotations := map[string]string{}

	if mattermost.Spec.AWSLoadBalancerController.InternetFacing {
		ingressAnnotations["alb.ingress.kubernetes.io/scheme"] = "internet-facing"
	} else {
		ingressAnnotations["alb.ingress.kubernetes.io/scheme"] = "internal"
	}

	if mattermost.Spec.AWSLoadBalancerController.CertificateARN != "" {
		ingressAnnotations["alb.ingress.kubernetes.io/certificate-arn"] = mattermost.Spec.AWSLoadBalancerController.CertificateARN
		ingressAnnotations["alb.ingress.kubernetes.io/ssl-redirect"] = "443"
		ingressAnnotations["alb.ingress.kubernetes.io/listen-ports"] = `'[{"HTTP": 80}, {"HTTPS":443}]'`
	} else {
		ingressAnnotations["alb.ingress.kubernetes.io/listen-ports"] = `'[{"HTTP": 8065}]'`
	}

	for k, v := range mattermost.GetIngresAnnotations() {
		ingressAnnotations[k] = v
	}

	hosts := mattermost.GetIngressHostNames()

	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:            mattermost.Name,
			Namespace:       mattermost.Namespace,
			Labels:          mattermost.MattermostLabels(mattermost.Name),
			OwnerReferences: MattermostOwnerReference(mattermost),
			Annotations:     ingressAnnotations,
		},
		Spec: networkingv1.IngressSpec{
			Rules:            makeIngressRules(hosts, mattermost),
			IngressClassName: pkgUtils.NewString("alb"),
		},
	}

	return ingress
}

func makeIngressRules(hosts []string, mattermost *mmv1beta.Mattermost) []networkingv1.IngressRule {
	rules := make([]networkingv1.IngressRule, 0, len(hosts))
	for _, host := range hosts {
		rule := networkingv1.IngressRule{
			Host: host,
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
		}
		rules = append(rules, rule)
	}

	return rules
}

// GenerateDeploymentV1Beta returns the deployment for Mattermost app.
func GenerateDeploymentV1Beta(mattermost *mmv1beta.Mattermost, db DatabaseConfig, fileStore *FileStoreInfo, deploymentName, ingressHost, serviceAccountName, containerImage string) *appsv1.Deployment {
	// DB
	envVarDB := db.EnvVars(mattermost)
	initContainers := db.InitContainers(mattermost)

	// File Store
	envVarFileStore := fileStoreEnvVars(fileStore)
	initContainers = append(initContainers, fileStore.config.InitContainers(mattermost)...)

	// Extensions
	if mattermost.Spec.PodExtensions.InitContainers != nil {
		initContainers = append(initContainers, mattermost.Spec.PodExtensions.InitContainers...)
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

	// General settings
	envVarGeneral := generalMattermostEnvVars(siteURLFromHost(ingressHost))

	// Determine max file size
	bodySize := strconv.Itoa(defaultMaxFileSize * sizeMB)
	if !mattermost.Spec.UseServiceLoadBalancer {
		bodySize = determineMaxBodySize(mattermost.GetIngresAnnotations(), bodySize)
	}
	envVarGeneral = append(envVarGeneral, corev1.EnvVar{
		Name:  "MM_FILESETTINGS_MAXFILESIZE",
		Value: bodySize,
	})

	// Prepare volumes
	volumes := mattermost.Spec.Volumes
	volumeMounts := mattermost.Spec.VolumeMounts
	podAnnotations := map[string]string{}

	// Set user specified annotations
	if mattermost.Spec.PodTemplate != nil && mattermost.Spec.PodTemplate.ExtraAnnotations != nil {
		podAnnotations = mattermost.Spec.PodTemplate.ExtraAnnotations
	}

	// Mattermost License
	if len(mattermost.Spec.LicenseSecret) != 0 {
		env, vMount, volume, annotations := mattermostLicenceConfig(mattermost.Spec.LicenseSecret)
		envVarGeneral = append(envVarGeneral, env)
		volumeMounts = append(volumeMounts, vMount)
		volumes = append(volumes, volume)
		// Add prometheus annotations, overwriting user specified if needed
		for k, v := range annotations {
			podAnnotations[k] = v
		}
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

	var containerSecurityContext *corev1.SecurityContext
	var podSecurityContext *corev1.PodSecurityContext
	if mattermost.Spec.PodTemplate != nil {
		containerSecurityContext = mattermost.Spec.PodTemplate.ContainerSecurityContext
		podSecurityContext = mattermost.Spec.PodTemplate.SecurityContext
	}

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
					Labels:      mattermost.MattermostPodLabels(deploymentName),
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
							ReadinessProbe:  readiness,
							LivenessProbe:   liveness,
							VolumeMounts:    volumeMounts,
							Resources:       mattermost.Spec.Scheduling.Resources,
							SecurityContext: containerSecurityContext,
						},
					},
					ImagePullSecrets: mattermost.Spec.ImagePullSecrets,
					Volumes:          volumes,
					DNSConfig:        mattermost.Spec.DNSConfig,
					DNSPolicy:        mattermost.Spec.DNSPolicy,
					Affinity:         mattermost.Spec.Scheduling.Affinity,
					NodeSelector:     mattermost.Spec.Scheduling.NodeSelector,
					Tolerations:      mattermost.Spec.Scheduling.Tolerations,
					SecurityContext:  podSecurityContext,
				},
			},
		},
	}
}

// GenerateSecretV1Beta returns the secret for Mattermost
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

// GenerateServiceAccountV1Beta returns the Service Account for Mattermost
func GenerateServiceAccountV1Beta(mattermost *mmv1beta.Mattermost, saName string) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:            saName,
			Namespace:       mattermost.Namespace,
			OwnerReferences: MattermostOwnerReference(mattermost),
		},
	}
}

// GenerateRoleV1Beta returns the Role for Mattermost
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

// GenerateRoleBindingV1Beta returns the RoleBinding for Mattermost
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
