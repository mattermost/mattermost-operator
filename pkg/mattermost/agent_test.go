package mattermost

import (
	"testing"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testAgent(name, ns string) *mmv1beta.Agent {
	return &mmv1beta.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: mmv1beta.AgentSpec{
			Image: "mattermost/test-agent:latest",
			Hooks: []string{"MessageHasBeenPosted", "UserHasJoinedChannel"},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("512Mi"),
				},
			},
			EgressPolicy: mmv1beta.AgentEgressPolicyDeny,
			MattermostRef: corev1.LocalObjectReference{
				Name: "mm-prod",
			},
		},
	}
}

func TestAgentOwnerReference(t *testing.T) {
	agent := testAgent("my-agent", "default")
	refs := AgentOwnerReference(agent)

	assert.Len(t, refs, 1)
	ref := refs[0]
	assert.Equal(t, "Agent", ref.Kind)
	assert.Equal(t, "installation.mattermost.com/v1beta1", ref.APIVersion)
	assert.True(t, *ref.Controller)
	assert.Equal(t, agent.Name, ref.Name)
}

func TestGenerateAgentServiceAccount(t *testing.T) {
	agent := testAgent("my-agent", "default")
	sa := GenerateAgentServiceAccount(agent)

	assert.Equal(t, "my-agent", sa.Name)
	assert.Equal(t, "default", sa.Namespace)
	assert.Equal(t, mmv1beta.AgentLabels("my-agent"), sa.Labels)
	assert.Len(t, sa.OwnerReferences, 1)
	assert.Equal(t, "Agent", sa.OwnerReferences[0].Kind)
}

func TestGenerateAgentService(t *testing.T) {
	agent := testAgent("my-agent", "default")
	svc := GenerateAgentService(agent)

	assert.Equal(t, "my-agent", svc.Name)
	assert.Equal(t, "default", svc.Namespace)
	assert.Equal(t, corev1.ServiceTypeClusterIP, svc.Spec.Type)
	assert.Len(t, svc.OwnerReferences, 1)

	assert.Len(t, svc.Spec.Ports, 1)
	assert.Equal(t, int32(50051), svc.Spec.Ports[0].Port)
	assert.Equal(t, "grpc", svc.Spec.Ports[0].Name)

	assert.Equal(t, mmv1beta.AgentSelectorLabels("my-agent"), svc.Spec.Selector)
}

func TestGenerateAgentDeployment(t *testing.T) {
	agent := testAgent("my-agent", "test-ns")
	dep := GenerateAgentDeployment(agent)

	assert.Equal(t, "my-agent", dep.Name)
	assert.Equal(t, "test-ns", dep.Namespace)
	assert.Len(t, dep.OwnerReferences, 1)

	// Replicas
	assert.Equal(t, int32(1), *dep.Spec.Replicas)

	// Selector
	assert.Equal(t, mmv1beta.AgentSelectorLabels("my-agent"), dep.Spec.Selector.MatchLabels)

	// Service account
	assert.Equal(t, "my-agent", dep.Spec.Template.Spec.ServiceAccountName)

	// Main container
	containers := dep.Spec.Template.Spec.Containers
	assert.Len(t, containers, 1)
	c := containers[0]
	assert.Equal(t, mmv1beta.AgentContainerName, c.Name)
	assert.Equal(t, "mattermost/test-agent:latest", c.Image)
	assert.Equal(t, agent.Spec.Resources, c.Resources)

	// Ports
	assert.Len(t, c.Ports, 1)
	assert.Equal(t, int32(50051), c.Ports[0].ContainerPort)

	// Env vars
	envMap := make(map[string]string)
	for _, e := range c.Env {
		envMap[e.Name] = e.Value
	}
	assert.Equal(t, "http://mm-prod.test-ns.svc.cluster.local:8065", envMap["MM_SERVER_URL"])
	assert.Equal(t, "MessageHasBeenPosted,UserHasJoinedChannel", envMap["AGENT_HOOKS"])

	// Volume mounts on main container
	assert.Len(t, c.VolumeMounts, 2)
	var hasBotToken, hasMmctlConfig bool
	for _, vm := range c.VolumeMounts {
		if vm.Name == "bot-token" {
			hasBotToken = true
			assert.Equal(t, "/secrets/mmctl-token", vm.MountPath)
			assert.True(t, vm.ReadOnly)
		}
		if vm.Name == "mmctl-config" {
			hasMmctlConfig = true
			assert.Equal(t, "/tmp/.config/mmctl", vm.MountPath)
		}
	}
	assert.True(t, hasBotToken, "bot-token volume mount expected")
	assert.True(t, hasMmctlConfig, "mmctl-config volume mount expected")

	// Init container
	initContainers := dep.Spec.Template.Spec.InitContainers
	assert.Len(t, initContainers, 1)
	init := initContainers[0]
	assert.Equal(t, "mmctl-auth", init.Name)
	assert.Contains(t, init.Command, "mmctl")
	assert.NotContains(t, init.Command, "--insecure-skip-verify")

	// Volumes
	volumes := dep.Spec.Template.Spec.Volumes
	assert.Len(t, volumes, 2)
	var hasBotTokenVol, hasMmctlConfigVol bool
	for _, v := range volumes {
		if v.Name == "bot-token" {
			hasBotTokenVol = true
			assert.Equal(t, agent.BotTokenSecretName(), v.Secret.SecretName)
		}
		if v.Name == "mmctl-config" {
			hasMmctlConfigVol = true
			assert.NotNil(t, v.EmptyDir)
		}
	}
	assert.True(t, hasBotTokenVol, "bot-token volume expected")
	assert.True(t, hasMmctlConfigVol, "mmctl-config emptyDir volume expected")
}

func TestGenerateAgentDeployment_CustomEnvVars(t *testing.T) {
	agent := testAgent("my-agent", "default")
	agent.Spec.Env = []corev1.EnvVar{
		{Name: "CUSTOM_VAR", Value: "custom-value"},
		{Name: "MM_SERVER_URL", Value: "should-not-override"},
	}

	dep := GenerateAgentDeployment(agent)
	c := dep.Spec.Template.Spec.Containers[0]

	envMap := make(map[string]string)
	for _, e := range c.Env {
		envMap[e.Name] = e.Value
	}

	// Custom env var is present
	assert.Equal(t, "custom-value", envMap["CUSTOM_VAR"])

	// mergeEnvVars overwrites base with user-specified values
	// (this is the documented behavior of mergeEnvVars — user env wins)
	assert.Equal(t, "should-not-override", envMap["MM_SERVER_URL"])
}

func TestGenerateAgentNetworkPolicy_Deny(t *testing.T) {
	agent := testAgent("my-agent", "default")
	agent.Spec.EgressPolicy = mmv1beta.AgentEgressPolicyDeny

	np := GenerateAgentNetworkPolicy(agent)

	assert.Equal(t, "my-agent", np.Name)
	assert.Equal(t, "default", np.Namespace)
	assert.Len(t, np.OwnerReferences, 1)

	// Policy types
	assert.Contains(t, np.Spec.PolicyTypes, networkingv1.PolicyTypeIngress)
	assert.Contains(t, np.Spec.PolicyTypes, networkingv1.PolicyTypeEgress)

	// Ingress: from MM pods on port 50051
	assert.Len(t, np.Spec.Ingress, 1)
	ingress := np.Spec.Ingress[0]
	assert.Len(t, ingress.From, 1)
	assert.Equal(t, "mm-prod", ingress.From[0].PodSelector.MatchLabels[mmv1beta.ClusterLabel])
	assert.Len(t, ingress.Ports, 1)
	assert.Equal(t, int32(50051), ingress.Ports[0].Port.IntVal)

	// Egress: 2 rules (MM + DNS)
	assert.Len(t, np.Spec.Egress, 2)

	// First egress rule: MM pods on 8065
	mmEgress := np.Spec.Egress[0]
	assert.Len(t, mmEgress.To, 1)
	assert.Equal(t, "mm-prod", mmEgress.To[0].PodSelector.MatchLabels[mmv1beta.ClusterLabel])
	assert.Len(t, mmEgress.Ports, 1)
	assert.Equal(t, int32(8065), mmEgress.Ports[0].Port.IntVal)

	// Second egress rule: DNS (UDP + TCP 53)
	dnsEgress := np.Spec.Egress[1]
	assert.Len(t, dnsEgress.Ports, 2)
}

func TestGenerateAgentNetworkPolicy_AllowList(t *testing.T) {
	agent := testAgent("my-agent", "default")
	agent.Spec.EgressPolicy = mmv1beta.AgentEgressPolicyAllowList
	agent.Spec.EgressAllowList = []string{"api.openai.com"}

	np := GenerateAgentNetworkPolicy(agent)

	// 4 egress rules: MM server (8065) + DNS (53) + HTTPS (443) + HTTP (80)
	assert.Len(t, np.Spec.Egress, 4)

	// First egress rule: MM pods on 8065
	mmEgress := np.Spec.Egress[0]
	assert.Len(t, mmEgress.To, 1)
	assert.Equal(t, "mm-prod", mmEgress.To[0].PodSelector.MatchLabels[mmv1beta.ClusterLabel])
	assert.Len(t, mmEgress.Ports, 1)
	assert.Equal(t, int32(8065), mmEgress.Ports[0].Port.IntVal)

	// Second egress rule: DNS (UDP + TCP 53)
	dnsEgress := np.Spec.Egress[1]
	assert.Len(t, dnsEgress.Ports, 2)
	assert.Equal(t, int32(53), dnsEgress.Ports[0].Port.IntVal)
	assert.Equal(t, int32(53), dnsEgress.Ports[1].Port.IntVal)

	// Third egress rule: HTTPS (port 443) to any destination
	httpsEgress := np.Spec.Egress[2]
	assert.Empty(t, httpsEgress.To, "no To selector means any destination")
	assert.Len(t, httpsEgress.Ports, 1)
	assert.Equal(t, int32(443), httpsEgress.Ports[0].Port.IntVal)
	assert.Equal(t, corev1.ProtocolTCP, *httpsEgress.Ports[0].Protocol)

	// Fourth egress rule: HTTP (port 80) to any destination
	httpEgress := np.Spec.Egress[3]
	assert.Empty(t, httpEgress.To, "no To selector means any destination")
	assert.Len(t, httpEgress.Ports, 1)
	assert.Equal(t, int32(80), httpEgress.Ports[0].Port.IntVal)
	assert.Equal(t, corev1.ProtocolTCP, *httpEgress.Ports[0].Protocol)
}

func TestGenerateAgentBotTokenSecret(t *testing.T) {
	agent := testAgent("my-agent", "default")
	secret := GenerateAgentBotTokenSecret(agent, "bot-token-value-123")

	assert.Equal(t, agent.BotTokenSecretName(), secret.Name)
	assert.Equal(t, "default", secret.Namespace)
	assert.Equal(t, mmv1beta.AgentLabels("my-agent"), secret.Labels)
	assert.Len(t, secret.OwnerReferences, 1)
	assert.Equal(t, "Agent", secret.OwnerReferences[0].Kind)
	assert.Equal(t, []byte("bot-token-value-123"), secret.Data["token"])
}
