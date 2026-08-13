package mattermost

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	pkgUtils "github.com/mattermost/mattermost-operator/pkg/utils"

	rbacv1 "k8s.io/api/rbac/v1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	ingressClassAnnotation = "kubernetes.io/ingress.class"

	// SetupJobName is the name of the (currently disabled) database setup job.
	SetupJobName = "mattermost-db-setup"
	// WaitForDBSetupContainerName is the init container that waits on that job.
	WaitForDBSetupContainerName = "init-wait-for-db-setup"
)

var defaultIngressPathType = networkingv1.PathTypeImplementationSpecific

// sanitizeIngressAnnotations filters out annotations that could allow arbitrary
// nginx/ingress controller config injection via snippet directives.
// Dropped keys are logged via logger when provided.
func sanitizeIngressAnnotations(annotations map[string]string, logger logr.Logger) map[string]string {
	if annotations == nil {
		return nil
	}
	safe := make(map[string]string, len(annotations))
	for k, v := range annotations {
		lower := strings.ToLower(k)
		if strings.Contains(lower, "snippet") {
			logger.Info("dropped unsafe ingress annotation", "key", k, "reason", "contains snippet")
			continue
		}
		if strings.ContainsAny(v, "\r\n") {
			logger.Info("dropped unsafe ingress annotation", "key", k, "reason", "value contains newline")
			continue
		}
		safe[k] = v
	}
	return safe
}

type DatabaseConfig interface {
	EnvVars(mattermost *mmv1beta.Mattermost) []corev1.EnvVar
	InitContainers(mattermost *mmv1beta.Mattermost) []corev1.Container
}

type FileStoreConfig interface {
	EnvVars(mattermost *mmv1beta.Mattermost) []corev1.EnvVar
	InitContainers(mattermost *mmv1beta.Mattermost) []corev1.Container
	Volumes(mattermost *mmv1beta.Mattermost) ([]corev1.Volume, []corev1.VolumeMount)
}

// GenerateServiceV1Beta returns the service for the Mattermost app.
func GenerateServiceV1Beta(mattermost *mmv1beta.Mattermost) *corev1.Service {
	annotations := mergeStringMaps(nil, mattermost.Spec.ServiceAnnotations)

	if mattermost.AWSLoadBalancerEnabled() {
		// Create a NodePort service because the ALB requires it
		service := newServiceV1Beta(mattermost, annotations)
		return configureMattermostServiceNodePort(service)
	}

	if mattermost.Spec.UseServiceLoadBalancer {
		// Create a LoadBalancer service with additional annotations provided in
		// the Mattermost Spec. The LoadBalancer is directly accessible from
		// outside the cluster thus exposes ports 80 and 443.
		service := newServiceV1Beta(mattermost, annotations)
		return configureMattermostLoadBalancerService(service)
	}

	// Create a headless service which is not directly accessible from outside
	// the cluster and thus exposes a custom port.
	service := newServiceV1Beta(mattermost, annotations)
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
	service = configureMattermostServicePorts(service)
	service.Spec.ClusterIP = corev1.ClusterIPNone
	service.Spec.Type = corev1.ServiceTypeClusterIP

	return service
}

func configureMattermostServiceNodePort(service *corev1.Service) *corev1.Service {
	service = configureMattermostServicePorts(service)
	service.Spec.Type = corev1.ServiceTypeNodePort

	return service
}

func configureMattermostServicePorts(service *corev1.Service) *corev1.Service {
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

	return service
}

// GenerateIngressV1Beta returns the ingress for the Mattermost app.
func GenerateIngressV1Beta(mattermost *mmv1beta.Mattermost, logger logr.Logger) *networkingv1.Ingress {
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

	for k, v := range sanitizeIngressAnnotations(mattermost.GetIngresAnnotations(), logger) {
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

func GenerateALBIngressClassV1Beta(mattermost *mmv1beta.Mattermost) *networkingv1.IngressClass {
	ingressClass := &networkingv1.IngressClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:            mattermost.Name,
			Namespace:       mattermost.Namespace,
			Labels:          mattermost.MattermostLabels(mattermost.Name),
			OwnerReferences: MattermostOwnerReference(mattermost),
		},
		Spec: networkingv1.IngressClassSpec{
			Controller: "ingress.k8s.aws/alb",
		},
	}

	return ingressClass
}

// GenerateIngressALBIngressV1Beta returns the AWS ALB ingress for the Mattermost app.
func GenerateALBIngressV1Beta(mattermost *mmv1beta.Mattermost, logger logr.Logger) *networkingv1.Ingress {
	ingressAnnotations := map[string]string{}

	if mattermost.Spec.AWSLoadBalancerController.InternetFacing {
		ingressAnnotations["alb.ingress.kubernetes.io/scheme"] = "internet-facing"
	} else {
		ingressAnnotations["alb.ingress.kubernetes.io/scheme"] = "internal"
	}

	if mattermost.Spec.AWSLoadBalancerController.CertificateARN != "" {
		ingressAnnotations["alb.ingress.kubernetes.io/certificate-arn"] = mattermost.Spec.AWSLoadBalancerController.CertificateARN
		ingressAnnotations["alb.ingress.kubernetes.io/ssl-redirect"] = "443"
		ingressAnnotations["alb.ingress.kubernetes.io/listen-ports"] = `[{"HTTP": 80}, {"HTTPS":443}]`
	} else {
		ingressAnnotations["alb.ingress.kubernetes.io/listen-ports"] = `[{"HTTP": 8065}]`
	}

	for k, v := range sanitizeIngressAnnotations(mattermost.GetAWSLoadBalancerIngressAnnotations(), logger) {
		ingressAnnotations[k] = v
	}

	hosts := mattermost.GetAWSLoadBalancerHostNames()

	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:            mattermost.Name,
			Namespace:       mattermost.Namespace,
			Labels:          mattermost.MattermostLabels(mattermost.Name),
			OwnerReferences: MattermostOwnerReference(mattermost),
			Annotations:     ingressAnnotations,
		},
		Spec: networkingv1.IngressSpec{
			Rules: makeIngressRules(hosts, mattermost),
		},
	}

	if mattermost.Spec.AWSLoadBalancerController.IngressClassName != "" {
		ingress.Spec.IngressClassName = pkgUtils.NewString(mattermost.Spec.AWSLoadBalancerController.IngressClassName)
	} else {
		ingress.Spec.IngressClassName = pkgUtils.NewString(mattermost.Name)
	}

	return ingress
}

var httpRouteGVK = schema.GroupVersionKind{
	Group:   "gateway.networking.k8s.io",
	Version: "v1",
	Kind:    "HTTPRoute",
}

// GenerateHTTPRouteV1Beta returns the HTTPRoute (Gateway API) for the Mattermost app.
//
// NOTE: Why unstructured instead of the typed sigs.k8s.io/gateway-api SDK:
//
// Adding sigs.k8s.io/gateway-api as a direct dependency causes a transitive
// dependency conflict that breaks the build. Any gateway-api release new enough
// to support HTTPRoute v1 (stable) pulls in a newer kube-openapi snapshot that
// requires sigs.k8s.io/structured-merge-diff/v6, while k8s.io/apimachinery at
// the version pinned by this module (v0.33.x) was compiled against v4.  The two
// major versions of structured-merge-diff expose incompatible types, so the
// compiler rejects the combined module graph.
//
// Upgrading controller-runtime or the entire k8s.io/* dependency set to resolve
// the conflict is a larger, separate effort. Using *unstructured.Unstructured is
// the standard pattern for operators that manage CRDs owned by another operator:
// it keeps the dependency graph clean, is version-agnostic, and is fully
// supported by controller-runtime's client and the banzaicloud object-matcher.
func GenerateHTTPRouteV1Beta(mattermost *mmv1beta.Mattermost, logger logr.Logger) *unstructured.Unstructured {
	requestTimeout := "3600s"
	if mattermost.Spec.HTTPRoute.RequestTimeout != "" {
		requestTimeout = mattermost.Spec.HTTPRoute.RequestTimeout
	}
	backendTimeout := "3600s"
	if mattermost.Spec.HTTPRoute.BackendRequestTimeout != "" {
		backendTimeout = mattermost.Spec.HTTPRoute.BackendRequestTimeout
	}

	gwGroup := "gateway.networking.k8s.io"
	if mattermost.Spec.HTTPRoute.GatewayRef.Group != "" {
		gwGroup = mattermost.Spec.HTTPRoute.GatewayRef.Group
	}

	gwNamespace := mattermost.Namespace
	if mattermost.Spec.HTTPRoute.GatewayRef.Namespace != "" {
		gwNamespace = mattermost.Spec.HTTPRoute.GatewayRef.Namespace
	}

	parentRef := map[string]interface{}{
		"group":     gwGroup,
		"kind":      "Gateway",
		"name":      mattermost.Spec.HTTPRoute.GatewayRef.Name,
		"namespace": gwNamespace,
	}
	if mattermost.Spec.HTTPRoute.GatewayRef.SectionName != "" {
		parentRef["sectionName"] = mattermost.Spec.HTTPRoute.GatewayRef.SectionName
	}

	hostNames := mattermost.GetHTTPRouteHostNames()
	hostnames := make([]interface{}, 0, len(hostNames))
	for _, h := range hostNames {
		hostnames = append(hostnames, h)
	}

	weight := int64(100)
	rule := map[string]interface{}{
		"matches": []interface{}{
			map[string]interface{}{
				"path": map[string]interface{}{
					"type":  "PathPrefix",
					"value": "/",
				},
			},
		},
		"backendRefs": []interface{}{
			map[string]interface{}{
				"group":  "",
				"kind":   "Service",
				"name":   mattermost.Name,
				"port":   int64(8065),
				"weight": weight,
			},
		},
		"timeouts": map[string]interface{}{
			"request":        requestTimeout,
			"backendRequest": backendTimeout,
		},
	}

	annotations := sanitizeIngressAnnotations(mattermost.GetHTTPRouteAnnotations(), logger)

	labels := mattermost.MattermostLabels(mattermost.Name)
	labelsIface := make(map[string]interface{}, len(labels))
	for k, v := range labels {
		labelsIface[k] = v
	}
	annotationsIface := make(map[string]interface{}, len(annotations))
	for k, v := range annotations {
		annotationsIface[k] = v
	}

	ownerRefs := MattermostOwnerReference(mattermost)
	ownerRefsIface := make([]interface{}, 0, len(ownerRefs))
	for _, ref := range ownerRefs {
		ownerRefsIface = append(ownerRefsIface, map[string]interface{}{
			"apiVersion":         ref.APIVersion,
			"kind":               ref.Kind,
			"name":               ref.Name,
			"uid":                string(ref.UID),
			"controller":         ref.Controller != nil && *ref.Controller,
			"blockOwnerDeletion": ref.BlockOwnerDeletion != nil && *ref.BlockOwnerDeletion,
		})
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "HTTPRoute",
			"metadata": map[string]interface{}{
				"name":            mattermost.Name,
				"namespace":       mattermost.Namespace,
				"labels":          labelsIface,
				"annotations":     annotationsIface,
				"ownerReferences": ownerRefsIface,
			},
			"spec": map[string]interface{}{
				"hostnames":  hostnames,
				"parentRefs": []interface{}{parentRef},
				"rules":      []interface{}{rule},
			},
		},
	}
	obj.SetGroupVersionKind(httpRouteGVK)

	return obj
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
func GenerateDeploymentV1Beta(mattermost *mmv1beta.Mattermost, db DatabaseConfig, fileStore FileStoreConfig, deploymentName, ingressHost, serviceAccountName, containerImage string) *appsv1.Deployment {
	// DB
	envVarDB := db.EnvVars(mattermost)
	initContainers := db.InitContainers(mattermost)

	// Base volumes
	volumes := mattermost.Spec.Volumes
	volumeMounts := mattermost.Spec.VolumeMounts

	// File Store
	envVarFileStore := fileStore.EnvVars(mattermost)
	fsVolumes, fsVmounts := fileStore.Volumes(mattermost)
	volumes = append(volumes, fsVolumes...)
	volumeMounts = append(volumeMounts, fsVmounts...)
	initContainers = append(initContainers, fileStore.InitContainers(mattermost)...)
	containerPorts := []corev1.ContainerPort{
		{
			ContainerPort: 8065,
			Name:          "app",
		},
		{
			ContainerPort: 8067,
			Name:          "metrics",
		},
	}

	// Extensions
	if mattermost.Spec.PodExtensions.InitContainers != nil {
		initContainers = append(initContainers, mattermost.Spec.PodExtensions.InitContainers...)
	}
	if mattermost.Spec.PodExtensions.ContainerPorts != nil {
		containerPorts = append(containerPorts, mattermost.Spec.PodExtensions.ContainerPorts...)
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

	// Apply optional job server settings
	if mattermost.Spec.JobServer != nil && mattermost.Spec.JobServer.DedicatedJobServer {
		envVarGeneral = append(envVarGeneral, corev1.EnvVar{
			Name:  "MM_JOBSETTINGS_RUNJOBS",
			Value: "false",
		})
	}

	// Prepare annotations
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

	// Deployment template
	revisionHistoryLimit := pkgUtils.NewInt32(defaultRevHistoryLimit)
	if mattermost.Spec.DeploymentTemplate != nil && mattermost.Spec.DeploymentTemplate.RevisionHistoryLimit != nil {
		revisionHistoryLimit = mattermost.Spec.DeploymentTemplate.RevisionHistoryLimit
	}

	// Default deployment strategy
	deploymentStrategy := appsv1.DeploymentStrategy{
		Type: appsv1.RollingUpdateDeploymentStrategyType,
		RollingUpdate: &appsv1.RollingUpdateDeployment{
			MaxUnavailable: &maxUnavailable,
			MaxSurge:       &maxSurge,
		},
	}

	// Override if the deployment type is not RollingUpdate and the strategy
	// type is valid.
	if mattermost.Spec.DeploymentTemplate != nil && mattermost.Spec.DeploymentTemplate.DeploymentStrategyType != "" {
		switch mattermost.Spec.DeploymentTemplate.DeploymentStrategyType {
		case appsv1.RecreateDeploymentStrategyType:
			deploymentStrategy = appsv1.DeploymentStrategy{
				Type: appsv1.RecreateDeploymentStrategyType,
			}
		}
	}

	// Custom container command
	command := []string{"mattermost"}
	if mattermost.Spec.PodTemplate != nil && mattermost.Spec.PodTemplate.Command != nil {
		command = mattermost.Spec.PodTemplate.Command
	}

	containers := []corev1.Container{
		{
			Name:                     mmv1beta.MattermostAppContainerName,
			Image:                    containerImage,
			ImagePullPolicy:          mattermost.Spec.ImagePullPolicy,
			TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
			Command:                  command,
			Env:                      envVars,
			Ports:                    containerPorts,
			ReadinessProbe:           readiness,
			LivenessProbe:            liveness,
			VolumeMounts:             volumeMounts,
			Resources:                mattermost.Spec.Scheduling.Resources,
			SecurityContext:          containerSecurityContext,
		},
	}

	// Final container extensions
	if mattermost.Spec.PodExtensions.SidecarContainers != nil {
		containers = append(containers, mattermost.Spec.PodExtensions.SidecarContainers...)
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            deploymentName,
			Namespace:       mattermost.Namespace,
			Labels:          mattermost.MattermostLabels(deploymentName),
			OwnerReferences: MattermostOwnerReference(mattermost),
		},
		Spec: appsv1.DeploymentSpec{
			Strategy:             deploymentStrategy,
			RevisionHistoryLimit: revisionHistoryLimit,
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
					Containers:         containers,
					ImagePullSecrets:   mattermost.Spec.ImagePullSecrets,
					Volumes:            volumes,
					DNSConfig:          mattermost.Spec.DNSConfig,
					DNSPolicy:          mattermost.Spec.DNSPolicy,
					Affinity:           mattermost.Spec.Scheduling.Affinity,
					NodeSelector:       mattermost.Spec.Scheduling.NodeSelector,
					Tolerations:        mattermost.Spec.Scheduling.Tolerations,
					SecurityContext:    podSecurityContext,
				},
			},
		},
	}
}

// GenerateJobServerDeploymentV1Beta returns the deployment for Mattermost app dedicated job server.
func GenerateJobServerDeploymentV1Beta(mattermost *mmv1beta.Mattermost, db DatabaseConfig, fileStore FileStoreConfig, deploymentName, ingressHost, serviceAccountName, containerImage string) *appsv1.Deployment {
	deployment := GenerateDeploymentV1Beta(
		mattermost,
		db,
		fileStore,
		mattermost.Name,
		mattermost.GetIngressHost(),
		mattermost.Name,
		mattermost.GetImageName(),
	)

	// Apply metadata overrides for dedicated job server configuration.
	deployment.Name = fmt.Sprintf("%s-jobserver", deploymentName)
	replicas := int32(1)
	deployment.Spec.Replicas = &replicas
	deployment.Spec.Template.ObjectMeta.Labels = mattermost.MattermostJobServerPodLabels(deploymentName)
	deployment.Spec.Selector.MatchLabels = mattermost.MattermostJobServerPodLabels(deploymentName)

	// Apply dedicated job server container configuration to existing
	// Mattermost container configuration.
	mattermostContainer := deployment.Spec.Template.Spec.Containers[0]
	mattermostContainer.Name = mmv1beta.MattermostJobServerContainerName
	mattermostContainer.Command = []string{"mattermost", "jobserver"}
	mattermostContainer.Ports = nil
	mattermostContainer.LivenessProbe = nil
	mattermostContainer.ReadinessProbe = nil

	deployment.Spec.Template.Spec.Containers = []corev1.Container{mattermostContainer}

	return deployment
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
			Selector:                 mmv1beta.MattermostSelectorLabels(mattermost.Name),
			PublishNotReadyAddresses: true,
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
