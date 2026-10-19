package mattermost

import (
	"net/url"
	"strconv"
	"strings"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// MattermostServerPort is the HTTP port exposed by the Mattermost server pod.
const MattermostServerPort = int32(8065)

// AgentOwnerReference returns the owner reference for an Agent resource.
func AgentOwnerReference(agent *mmv1beta.Agent) []metav1.OwnerReference {
	return []metav1.OwnerReference{
		*metav1.NewControllerRef(agent, schema.GroupVersionKind{
			Group:   mmv1beta.GroupVersion.Group,
			Version: mmv1beta.GroupVersion.Version,
			Kind:    "Agent",
		}),
	}
}

// GenerateAgentServiceAccount returns the ServiceAccount for an Agent.
func GenerateAgentServiceAccount(agent *mmv1beta.Agent) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:            agent.Name,
			Namespace:       agent.Namespace,
			Labels:          mmv1beta.AgentLabels(agent),
			OwnerReferences: AgentOwnerReference(agent),
		},
	}
}

// AgentServiceName returns the generated Service name for an Agent.
func AgentServiceName(agent *mmv1beta.Agent) string {
	return agent.Name
}

// AgentDeploymentName returns the generated Deployment name for an Agent.
func AgentDeploymentName(agent *mmv1beta.Agent) string {
	return agent.Name
}

// AgentServiceURL returns the in-cluster HTTP endpoint for an Agent.
func AgentServiceURL(agent *mmv1beta.Agent) string {
	return mmv1beta.ClusterServiceURL(AgentServiceName(agent), agent.Namespace, mmv1beta.AgentHTTPPort)
}

// GenerateAgentService returns the HTTP Service for an Agent.
func GenerateAgentService(agent *mmv1beta.Agent) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            AgentServiceName(agent),
			Namespace:       agent.Namespace,
			Labels:          mmv1beta.AgentLabels(agent),
			OwnerReferences: AgentOwnerReference(agent),
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: mmv1beta.AgentSelectorLabels(agent),
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       mmv1beta.AgentHTTPPort,
					TargetPort: intstr.FromInt32(mmv1beta.AgentHTTPPort),
				},
			},
		},
	}
}

// mmServerURL returns the in-cluster URL for the Mattermost server referenced by the agent.
func mmServerURL(agent *mmv1beta.Agent) string {
	return mmv1beta.ClusterServiceURL(agent.Spec.MattermostRef.Name, agent.Namespace, MattermostServerPort)
}

// imageTagNeedsAlwaysPull returns true if the image tag is "dev", "latest",
// or absent (K8s treats no-tag as :latest). Used to auto-set ImagePullPolicy.
func imageTagNeedsAlwaysPull(image string) bool {
	idx := strings.LastIndex(image, ":")
	if idx > strings.LastIndex(image, "/") {
		tag := image[idx+1:]
		return tag == "dev" || tag == "latest"
	}
	return true // no tag = K8s treats as :latest
}

func appendLiteLLMEnvVars(env []corev1.EnvVar, baseURL, keySecretName string) []corev1.EnvVar {
	keyEnvSource := EnvSourceFromSecret(keySecretName, mmv1beta.SecretKeyAPIKey)
	return append(env,
		corev1.EnvVar{Name: "LITELLM_BASE_URL", Value: baseURL},
		corev1.EnvVar{Name: "LITELLM_MCP_URL", Value: baseURL + "/mcp"},
		corev1.EnvVar{Name: "OPENAI_BASE_URL", Value: baseURL + "/v1"},
		corev1.EnvVar{Name: "OPENAI_API_KEY", ValueFrom: keyEnvSource},
		// The Anthropic SDK already prepends /v1/ to its API paths.
		corev1.EnvVar{Name: "ANTHROPIC_BASE_URL", Value: baseURL},
		corev1.EnvVar{Name: "ANTHROPIC_API_KEY", ValueFrom: keyEnvSource},
	)
}

// GenerateAgentDeployment returns the Deployment for an Agent.
func GenerateAgentDeployment(agent *mmv1beta.Agent) *appsv1.Deployment {
	replicas := int32(1)

	baseEnv := []corev1.EnvVar{
		{
			Name:  "MM_SERVER_URL",
			Value: mmServerURL(agent),
		},
		{
			Name:  "AGENT_HOOKS",
			Value: strings.Join(agent.Spec.Hooks, ","),
		},
		{
			Name: "MM_BOT_TOKEN",
			ValueFrom: EnvSourceFromSecret(
				agent.BotTokenSecretName(),
				mmv1beta.SecretKeyBotToken,
			),
		},
		{
			Name: "HOOK_SECRET",
			ValueFrom: EnvSourceFromSecret(
				agent.HookSecretName(),
				mmv1beta.SecretKeyHookSecret,
			),
		},
	}

	if baseURL, keySecretName, ok := agent.GatewayEndpoint(); ok {
		baseEnv = appendLiteLLMEnvVars(baseEnv, baseURL, keySecretName)
	}

	envVars := mergeEnvVars(baseEnv, agent.Spec.Env)

	volumes := []corev1.Volume{
		{
			Name: "bot-token",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: agent.BotTokenSecretName(),
				},
			},
		},
	}
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "bot-token",
			MountPath: "/secrets/mmctl-token",
			ReadOnly:  true,
		},
	}

	if agent.Spec.Storage != nil {
		volumes = append(volumes, corev1.Volume{
			Name: "agent-storage",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: agent.StoragePVCName(),
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "agent-storage",
			MountPath: agent.Spec.Storage.MountPath,
		})
	}

	pullPolicy := corev1.PullIfNotPresent
	if imageTagNeedsAlwaysPull(agent.Spec.Image) {
		pullPolicy = corev1.PullAlways
	}

	runAsNonRoot := true
	allowPrivilegeEscalation := false

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            AgentDeploymentName(agent),
			Namespace:       agent.Namespace,
			Labels:          mmv1beta.AgentLabels(agent),
			OwnerReferences: AgentOwnerReference(agent),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: mmv1beta.AgentSelectorLabels(agent),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: mmv1beta.AgentLabels(agent),
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: agent.Name,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &runAsNonRoot,
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{
						{
							Name:            mmv1beta.AgentContainerName,
							Image:           agent.Spec.Image,
							ImagePullPolicy: pullPolicy,
							Env:             envVars,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: mmv1beta.AgentHTTPPort,
									Name:          "http",
								},
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									TCPSocket: &corev1.TCPSocketAction{
										Port: intstr.FromInt32(mmv1beta.AgentHTTPPort),
									},
								},
								InitialDelaySeconds: 15,
								PeriodSeconds:       5,
								FailureThreshold:    6,
							},
							Resources:    agent.Spec.Resources,
							VolumeMounts: volumeMounts,
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: &allowPrivilegeEscalation,
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
							},
						},
					},
					Volumes: volumes,
				},
			},
		},
	}
}

// agentIngressRules returns the ingress rules for the Agent NetworkPolicy.
// It allows Mattermost server pods and an operator-managed LiteLLM gateway.
func agentIngressRules(agent *mmv1beta.Agent) []networkingv1.NetworkPolicyIngressRule {
	protocol := corev1.ProtocolTCP
	agentPort := intstr.FromInt32(mmv1beta.AgentHTTPPort)
	ingressFrom := []networkingv1.NetworkPolicyPeer{
		{
			PodSelector: &metav1.LabelSelector{
				MatchLabels: mmv1beta.MattermostSelectorLabels(agent.Spec.MattermostRef.Name),
			},
		},
	}

	if agent.HasOperatorManagedGateway() {
		ingressFrom = append(ingressFrom, networkingv1.NetworkPolicyPeer{
			PodSelector: &metav1.LabelSelector{
				MatchLabels: LiteLLMSelectorLabels(),
			},
		})
	}

	return []networkingv1.NetworkPolicyIngressRule{
		{
			From: ingressFrom,
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: &protocol,
					Port:     &agentPort,
				},
			},
		},
	}
}

// agentBaseEgressRules returns the restricted egress rules for an Agent.
func agentBaseEgressRules(agent *mmv1beta.Agent) []networkingv1.NetworkPolicyEgressRule {
	protocol := corev1.ProtocolTCP
	protocolUDP := corev1.ProtocolUDP
	mmPort := intstr.FromInt32(MattermostServerPort)
	dnsPort := intstr.FromInt32(53)

	egressRules := []networkingv1.NetworkPolicyEgressRule{
		{
			To: []networkingv1.NetworkPolicyPeer{
				{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: mmv1beta.MattermostSelectorLabels(agent.Spec.MattermostRef.Name),
					},
				},
			},
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: &protocol,
					Port:     &mmPort,
				},
			},
		},
	}

	if agent.HasOperatorManagedGateway() {
		liteLLMPort := intstr.FromInt32(mmv1beta.AgentLiteLLMPort)
		egressRules = append(egressRules, networkingv1.NetworkPolicyEgressRule{
			To: []networkingv1.NetworkPolicyPeer{
				{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: LiteLLMSelectorLabels(),
					},
				},
			},
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: &protocol,
					Port:     &liteLLMPort,
				},
			},
		})
	}

	if agent.HasExternalGateway() {
		if port, ok := externalGatewayPort(agent.Spec.LLMGateway.External.URL); ok {
			externalPort := intstr.FromInt32(port)
			egressRules = append(egressRules, networkingv1.NetworkPolicyEgressRule{
				Ports: []networkingv1.NetworkPolicyPort{
					{
						Protocol: &protocol,
						Port:     &externalPort,
					},
				},
			})
		}
	}

	egressRules = append(egressRules, networkingv1.NetworkPolicyEgressRule{
		Ports: []networkingv1.NetworkPolicyPort{
			{
				Protocol: &protocolUDP,
				Port:     &dnsPort,
			},
			{
				Protocol: &protocol,
				Port:     &dnsPort,
			},
		},
	})

	return egressRules
}

// externalGatewayPort derives the egress port for an external gateway URL.
// CRD validation guarantees an absolute http:// or https:// URL, so port
// derivation cannot fail: the explicit port wins, otherwise the scheme
// default (80/443) applies. The parse-failure fallback is kept harmless for
// objects persisted before that validation existed.
func externalGatewayPort(rawURL string) (int32, bool) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return 0, false
	}

	if parsedURL.Port() != "" {
		port, err := strconv.ParseInt(parsedURL.Port(), 10, 32)
		if err != nil || port < 1 || port > 65535 {
			return 0, false
		}
		return int32(port), true
	}

	if strings.EqualFold(parsedURL.Scheme, "http") {
		return 80, true
	}
	return 443, true
}

// webEgressRule allows HTTP and HTTPS to any destination.
func webEgressRule() networkingv1.NetworkPolicyEgressRule {
	protocol := corev1.ProtocolTCP
	httpsPort := intstr.FromInt32(443)
	httpPort := intstr.FromInt32(80)
	return networkingv1.NetworkPolicyEgressRule{
		Ports: []networkingv1.NetworkPolicyPort{
			{Protocol: &protocol, Port: &httpsPort},
			{Protocol: &protocol, Port: &httpPort},
		},
	}
}

// GenerateAgentNetworkPolicy returns the NetworkPolicy for an Agent.
func GenerateAgentNetworkPolicy(agent *mmv1beta.Agent) *networkingv1.NetworkPolicy {
	var egressRules []networkingv1.NetworkPolicyEgressRule
	switch agent.Spec.EgressPolicy {
	case mmv1beta.AgentEgressPolicyAllow:
		egressRules = []networkingv1.NetworkPolicyEgressRule{{}}
	case mmv1beta.AgentEgressPolicyAllowWeb:
		egressRules = append(agentBaseEgressRules(agent), webEgressRule())
	case mmv1beta.AgentEgressPolicyDeny:
		egressRules = agentBaseEgressRules(agent)
	default:
		egressRules = agentBaseEgressRules(agent)
	}

	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:            agent.Name,
			Namespace:       agent.Namespace,
			Labels:          mmv1beta.AgentLabels(agent),
			OwnerReferences: AgentOwnerReference(agent),
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: mmv1beta.AgentSelectorLabels(agent),
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: agentIngressRules(agent),
			Egress:  egressRules,
		},
	}
}

// GenerateAgentHookSecret returns the Secret storing the agent's hook secret.
func GenerateAgentHookSecret(agent *mmv1beta.Agent, secretValue string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            agent.HookSecretName(),
			Namespace:       agent.Namespace,
			Labels:          mmv1beta.AgentLabels(agent),
			OwnerReferences: AgentOwnerReference(agent),
		},
		Data: map[string][]byte{mmv1beta.SecretKeyHookSecret: []byte(secretValue)},
	}
}

// GenerateAgentPVC returns the PersistentVolumeClaim for an Agent's storage.
func GenerateAgentPVC(agent *mmv1beta.Agent) *corev1.PersistentVolumeClaim {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:            agent.StoragePVCName(),
			Namespace:       agent.Namespace,
			Labels:          mmv1beta.AgentLabels(agent),
			OwnerReferences: AgentOwnerReference(agent),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: agent.Spec.Storage.Size,
				},
			},
		},
	}

	if agent.Spec.Storage.StorageClassName != nil {
		pvc.Spec.StorageClassName = agent.Spec.Storage.StorageClassName
	}

	return pvc
}
