package mattermost

import (
	"fmt"
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

// MattermostServerPort is the HTTP port exposed by the Mattermost server pod.
const MattermostServerPort = 8065

// mmServerURL returns the in-cluster URL for the Mattermost server referenced by the agent.
func mmServerURL(agent *mmv1beta.Agent) string {
	return fmt.Sprintf("http://%s.%s.svc.cluster.local:%d",
		agent.Spec.MattermostRef.Name, agent.Namespace, MattermostServerPort)
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
							ImagePullPolicy: corev1.PullIfNotPresent,
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
// Always allows ingress from MM server pods.
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

	egressRules := []networkingv1.NetworkPolicyEgressRule{
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
			Labels:          mmv1beta.AgentLabels(agent.Name),
			OwnerReferences: AgentOwnerReference(agent),
		},
		Data: map[string][]byte{"hookSecret": []byte(secretValue)},
	}
}

// GenerateAgentPVC returns the PersistentVolumeClaim for an Agent's storage.
func GenerateAgentPVC(agent *mmv1beta.Agent) *corev1.PersistentVolumeClaim {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:            agent.StoragePVCName(),
			Namespace:       agent.Namespace,
			Labels:          mmv1beta.AgentLabels(agent.Name),
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
