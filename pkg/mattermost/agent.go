package mattermost

import (
	"strings"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
)

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
			Labels:          mmv1beta.AgentLabels(agent.Name),
			OwnerReferences: AgentOwnerReference(agent),
		},
	}
}

// GenerateAgentService returns the HTTP Service for an Agent.
func GenerateAgentService(agent *mmv1beta.Agent) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            agent.Name,
			Namespace:       agent.Namespace,
			Labels:          mmv1beta.AgentLabels(agent.Name),
			OwnerReferences: AgentOwnerReference(agent),
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: mmv1beta.AgentSelectorLabels(agent.Name),
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
	return "http://" + agent.Spec.MattermostRef.Name + "." + agent.Namespace + ".svc.cluster.local:8065"
}

// imageTagNeedsAlwaysPull returns true if the image tag is "dev", "latest",
// or absent (K8s treats no-tag as :latest). Used to auto-set ImagePullPolicy.
func imageTagNeedsAlwaysPull(image string) bool {
	if idx := strings.LastIndex(image, ":"); idx >= 0 {
		tag := image[idx+1:]
		return tag == "dev" || tag == "latest"
	}
	return true // no tag = K8s treats as :latest
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
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: agent.BotTokenSecretName(),
					},
					Key: "token",
				},
			},
		},
		{
			Name: "HOOK_SECRET",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: agent.HookSecretName(),
					},
					Key: "hookSecret",
				},
			},
		},
	}

	// Inject LiteLLM gateway env vars when llmGateway is configured.
	// These go into baseEnv so user's spec.env can override them via mergeEnvVars.
	if agent.Spec.LLMGateway != nil {
		var baseURL string
		var keySecretName string

		if agent.Spec.LLMGateway.OperatorManaged != nil {
			baseURL = LiteLLMServiceURL(agent.Namespace)
			keySecretName = agent.LiteLLMKeySecretName()
		} else if agent.Spec.LLMGateway.External != nil {
			baseURL = agent.Spec.LLMGateway.External.URL
			keySecretName = agent.Spec.LLMGateway.External.VirtualKeySecret
		}

		if baseURL != "" && keySecretName != "" {
			keyEnvSource := secretEnvSource(keySecretName, "apiKey")
			baseEnv = append(baseEnv,
				corev1.EnvVar{Name: "LITELLM_BASE_URL", Value: baseURL},
				corev1.EnvVar{Name: "LITELLM_MCP_URL", Value: baseURL + "/mcp"},
				corev1.EnvVar{Name: "OPENAI_BASE_URL", Value: baseURL + "/v1"},
				corev1.EnvVar{Name: "OPENAI_API_KEY", ValueFrom: keyEnvSource},
				// The Anthropic SDK already prepends /v1/ to its API paths,
				// so ANTHROPIC_BASE_URL must NOT include /v1.
				corev1.EnvVar{Name: "ANTHROPIC_BASE_URL", Value: baseURL},
				corev1.EnvVar{Name: "ANTHROPIC_API_KEY", ValueFrom: keyEnvSource},
			)
		}
	}

	envVars := mergeEnvVars(baseEnv, agent.Spec.Env)

	// Build volume and mount lists — start with bot-token, conditionally add storage.
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

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            agent.Name,
			Namespace:       agent.Namespace,
			Labels:          mmv1beta.AgentLabels(agent.Name),
			OwnerReferences: AgentOwnerReference(agent),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: mmv1beta.AgentSelectorLabels(agent.Name),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: mmv1beta.AgentLabels(agent.Name),
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: agent.Name,
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
							Resources:    agent.Spec.Resources,
							VolumeMounts: volumeMounts,
						},
					},
					Volumes: volumes,
				},
			},
		},
	}
}

// agentIngressRules returns the ingress rules for the Agent NetworkPolicy.
// Always allows ingress from MM server pods. When LLMGateway is configured,
// also allows ingress from LiteLLM pods (which proxy chat requests to agents).
func agentIngressRules(agent *mmv1beta.Agent, protocol *corev1.Protocol, agentPort *intstr.IntOrString) []networkingv1.NetworkPolicyIngressRule {
	ingressFrom := []networkingv1.NetworkPolicyPeer{
		{
			PodSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					mmv1beta.ClusterLabel: agent.Spec.MattermostRef.Name,
				},
			},
		},
	}

	if agent.Spec.LLMGateway != nil {
		ingressFrom = append(ingressFrom, networkingv1.NetworkPolicyPeer{
			PodSelector: &metav1.LabelSelector{
				MatchLabels: liteLLMLabels(),
			},
		})
	}

	return []networkingv1.NetworkPolicyIngressRule{
		{
			From: ingressFrom,
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: protocol,
					Port:     agentPort,
				},
			},
		},
	}
}

// GenerateAgentNetworkPolicy returns the NetworkPolicy for an Agent.
func GenerateAgentNetworkPolicy(agent *mmv1beta.Agent) *networkingv1.NetworkPolicy {
	protocol := corev1.ProtocolTCP
	protocolUDP := corev1.ProtocolUDP
	agentPort := intstr.FromInt32(mmv1beta.AgentHTTPPort)
	mmPort := intstr.FromInt32(8065)
	dnsPort := intstr.FromInt32(53)
	liteLLMPort := intstr.FromInt32(mmv1beta.AgentLiteLLMPort)

	egressRules := []networkingv1.NetworkPolicyEgressRule{
		// Allow egress to Mattermost pods on port 8065
		{
			To: []networkingv1.NetworkPolicyPeer{
				{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							mmv1beta.ClusterLabel: agent.Spec.MattermostRef.Name,
						},
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
		// Allow DNS
		{
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
		},
	}

	// When llmGateway is configured, agents must be able to reach LiteLLM on port 4000.
	// Insert before DNS rule so deny-mode agents route LLM calls through the gateway.
	if agent.Spec.LLMGateway != nil {
		liteLLMEgressRule := networkingv1.NetworkPolicyEgressRule{
			To: []networkingv1.NetworkPolicyPeer{
				{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: liteLLMLabels(),
					},
				},
			},
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: &protocol,
					Port:     &liteLLMPort,
				},
			},
		}
		// Insert after MM rule (index 0), before DNS rule (index 1).
		egressRules = append(egressRules[:1], append([]networkingv1.NetworkPolicyEgressRule{liteLLMEgressRule}, egressRules[1:]...)...)
	}

	// If egressPolicy is allowList, add specific egress rules for HTTPS, HTTP,
	// and other required outbound traffic. This avoids a catch-all that would
	// let the agent reach internal services (e.g., PostgreSQL) it shouldn't access.
	if agent.Spec.EgressPolicy == mmv1beta.AgentEgressPolicyAllowList {
		httpsPort := intstr.FromInt32(443)
		httpPort := intstr.FromInt32(80)

		// Allow egress to any IP on port 443 (HTTPS) — external APIs
		egressRules = append(egressRules, networkingv1.NetworkPolicyEgressRule{
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: &protocol,
					Port:     &httpsPort,
				},
			},
		})

		// Allow egress to any IP on port 80 (HTTP)
		egressRules = append(egressRules, networkingv1.NetworkPolicyEgressRule{
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: &protocol,
					Port:     &httpPort,
				},
			},
		})
	}

	// If egressPolicy is allow, permit all outbound traffic.
	if agent.Spec.EgressPolicy == mmv1beta.AgentEgressPolicyAllow {
		egressRules = []networkingv1.NetworkPolicyEgressRule{{}}
	}

	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:            agent.Name,
			Namespace:       agent.Namespace,
			Labels:          mmv1beta.AgentLabels(agent.Name),
			OwnerReferences: AgentOwnerReference(agent),
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: mmv1beta.AgentSelectorLabels(agent.Name),
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: agentIngressRules(agent, &protocol, &agentPort),
			Egress: egressRules,
		},
	}
}

// GenerateAgentBotTokenSecret returns the Secret storing the agent's bot token.
func GenerateAgentBotTokenSecret(agent *mmv1beta.Agent, token string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            agent.BotTokenSecretName(),
			Namespace:       agent.Namespace,
			Labels:          mmv1beta.AgentLabels(agent.Name),
			OwnerReferences: AgentOwnerReference(agent),
		},
		Data: map[string][]byte{"token": []byte(token)},
	}
}

// GenerateAgentHookSecret returns the Secret storing the agent's hook secret.
func GenerateAgentHookSecret(agent *mmv1beta.Agent, secretValue string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            agent.HookSecretName(),
			Namespace:       agent.Namespace,
			Labels:          mmv1beta.AgentLabels(agent.Name),
			OwnerReferences: AgentOwnerReference(agent),
		},
		Data: map[string][]byte{"hookSecret": []byte(secretValue)},
	}
}
