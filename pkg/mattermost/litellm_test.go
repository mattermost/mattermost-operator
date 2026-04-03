package mattermost

import (
	"testing"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestGenerateLiteLLMConfigMap verifies namespace, labels, and data content.
func TestGenerateLiteLLMConfigMap(t *testing.T) {
	cm := GenerateLiteLLMConfigMap("my-namespace")

	assert.Equal(t, mmv1beta.AgentLiteLLMConfigMapName, cm.Name)
	assert.Equal(t, "my-namespace", cm.Namespace)
	assert.Equal(t, liteLLMLabels(), cm.Labels)

	// Data must contain config.yaml with store_model_in_db
	require.Contains(t, cm.Data, "config.yaml")
	assert.Contains(t, cm.Data["config.yaml"], "store_model_in_db: true")
	assert.Contains(t, cm.Data["config.yaml"], "general_settings")

	// No owner references — shared resource
	assert.Empty(t, cm.OwnerReferences)
}

// TestGenerateLiteLLMDeployment verifies image, env vars (DATABASE_URL, MASTER_KEY,
// STORE_MODEL_IN_DB), probes, resources, and volumes.
func TestGenerateLiteLLMDeployment(t *testing.T) {
	dep := GenerateLiteLLMDeployment("my-namespace", mmv1beta.AgentLiteLLMDefaultImage, nil)

	assert.Equal(t, mmv1beta.AgentLiteLLMDeploymentName, dep.Name)
	assert.Equal(t, "my-namespace", dep.Namespace)
	assert.Equal(t, liteLLMLabels(), dep.Labels)

	// No owner references — shared resource
	assert.Empty(t, dep.OwnerReferences)

	// Replicas
	require.NotNil(t, dep.Spec.Replicas)
	assert.Equal(t, int32(1), *dep.Spec.Replicas)

	// Selector
	assert.Equal(t, liteLLMLabels(), dep.Spec.Selector.MatchLabels)

	// Single container
	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	c := dep.Spec.Template.Spec.Containers[0]

	assert.Equal(t, "litellm", c.Name)
	assert.Equal(t, mmv1beta.AgentLiteLLMDefaultImage, c.Image)
	assert.Equal(t, []string{"--config", "/app/config/config.yaml"}, c.Args)

	// Required env vars
	envMap := make(map[string]*corev1.EnvVar)
	for i := range c.Env {
		envMap[c.Env[i].Name] = &c.Env[i]
	}

	// DATABASE_URL from secret
	require.Contains(t, envMap, "DATABASE_URL")
	require.NotNil(t, envMap["DATABASE_URL"].ValueFrom)
	require.NotNil(t, envMap["DATABASE_URL"].ValueFrom.SecretKeyRef)
	assert.Equal(t, mmv1beta.AgentLiteLLMDBCredentialsSecret, envMap["DATABASE_URL"].ValueFrom.SecretKeyRef.Name)
	assert.Equal(t, "connectionString", envMap["DATABASE_URL"].ValueFrom.SecretKeyRef.Key)

	// LITELLM_MASTER_KEY from secret
	require.Contains(t, envMap, "LITELLM_MASTER_KEY")
	require.NotNil(t, envMap["LITELLM_MASTER_KEY"].ValueFrom)
	require.NotNil(t, envMap["LITELLM_MASTER_KEY"].ValueFrom.SecretKeyRef)
	assert.Equal(t, mmv1beta.AgentLiteLLMMasterKeySecretName, envMap["LITELLM_MASTER_KEY"].ValueFrom.SecretKeyRef.Name)
	assert.Equal(t, "masterKey", envMap["LITELLM_MASTER_KEY"].ValueFrom.SecretKeyRef.Key)

	// STORE_MODEL_IN_DB literal value
	require.Contains(t, envMap, "STORE_MODEL_IN_DB")
	assert.Equal(t, "True", envMap["STORE_MODEL_IN_DB"].Value)

	// Port
	require.Len(t, c.Ports, 1)
	assert.Equal(t, mmv1beta.AgentLiteLLMPort, c.Ports[0].ContainerPort)
	assert.Equal(t, "http", c.Ports[0].Name)

	// Resources
	assert.NotEmpty(t, c.Resources.Requests)
	assert.NotEmpty(t, c.Resources.Limits)

	// Probes
	require.NotNil(t, c.LivenessProbe)
	require.NotNil(t, c.LivenessProbe.HTTPGet)
	assert.Equal(t, "/health/liveliness", c.LivenessProbe.HTTPGet.Path)
	assert.Equal(t, mmv1beta.AgentLiteLLMPort, c.LivenessProbe.HTTPGet.Port.IntVal)

	require.NotNil(t, c.ReadinessProbe)
	require.NotNil(t, c.ReadinessProbe.HTTPGet)
	assert.Equal(t, "/health/readiness", c.ReadinessProbe.HTTPGet.Path)
	assert.Equal(t, mmv1beta.AgentLiteLLMPort, c.ReadinessProbe.HTTPGet.Port.IntVal)

	// Volume mount on container
	require.Len(t, c.VolumeMounts, 1)
	assert.Equal(t, "litellm-config", c.VolumeMounts[0].Name)
	assert.Equal(t, "/app/config", c.VolumeMounts[0].MountPath)
	assert.True(t, c.VolumeMounts[0].ReadOnly)

	// Volume
	require.Len(t, dep.Spec.Template.Spec.Volumes, 1)
	vol := dep.Spec.Template.Spec.Volumes[0]
	assert.Equal(t, "litellm-config", vol.Name)
	require.NotNil(t, vol.ConfigMap)
	assert.Equal(t, mmv1beta.AgentLiteLLMConfigMapName, vol.ConfigMap.Name)
}

// TestGenerateLiteLLMDeployment_WithProviderEnvVars verifies provider env vars are injected.
func TestGenerateLiteLLMDeployment_WithProviderEnvVars(t *testing.T) {
	providerEnvVars := []corev1.EnvVar{
		{Name: "ANTHROPIC_API_KEY", Value: "sk-ant-test"},
		{Name: "OPENAI_API_KEY", Value: "sk-oai-test"},
	}

	dep := GenerateLiteLLMDeployment("my-namespace", mmv1beta.AgentLiteLLMDefaultImage, providerEnvVars)
	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	c := dep.Spec.Template.Spec.Containers[0]

	envMap := make(map[string]string)
	for _, e := range c.Env {
		envMap[e.Name] = e.Value
	}

	assert.Equal(t, "sk-ant-test", envMap["ANTHROPIC_API_KEY"])
	assert.Equal(t, "sk-oai-test", envMap["OPENAI_API_KEY"])

	// Base env vars still present
	hasDBURL := false
	for _, e := range c.Env {
		if e.Name == "DATABASE_URL" {
			hasDBURL = true
		}
	}
	assert.True(t, hasDBURL, "DATABASE_URL must be present alongside provider env vars")
}

// TestGenerateLiteLLMService verifies port, selector, and labels.
func TestGenerateLiteLLMService(t *testing.T) {
	svc := GenerateLiteLLMService("my-namespace")

	assert.Equal(t, mmv1beta.AgentLiteLLMServiceName, svc.Name)
	assert.Equal(t, "my-namespace", svc.Namespace)
	assert.Equal(t, liteLLMLabels(), svc.Labels)

	// No owner references — shared resource
	assert.Empty(t, svc.OwnerReferences)

	assert.Equal(t, corev1.ServiceTypeClusterIP, svc.Spec.Type)
	assert.Equal(t, liteLLMLabels(), svc.Spec.Selector)

	require.Len(t, svc.Spec.Ports, 1)
	port := svc.Spec.Ports[0]
	assert.Equal(t, "http", port.Name)
	assert.Equal(t, mmv1beta.AgentLiteLLMPort, port.Port)
	assert.Equal(t, mmv1beta.AgentLiteLLMPort, port.TargetPort.IntVal)
}

// TestGenerateAgentLiteLLMKeySecret verifies secret data key "apiKey".
func TestGenerateAgentLiteLLMKeySecret(t *testing.T) {
	agent := &mmv1beta.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-agent",
			Namespace: "my-namespace",
		},
		Spec: mmv1beta.AgentSpec{
			Image: "mattermost/test-agent:latest",
			MattermostRef: corev1.LocalObjectReference{
				Name: "mm-prod",
			},
			AdminCredentialsSecret: "admin-secret",
		},
	}

	secret := GenerateAgentLiteLLMKeySecret(agent, "sk-virtual-key-123")

	assert.Equal(t, agent.LiteLLMKeySecretName(), secret.Name)
	assert.Equal(t, "my-namespace", secret.Namespace)
	assert.Equal(t, mmv1beta.AgentLabels("my-agent"), secret.Labels)

	// Must have owner reference pointing to the Agent
	require.Len(t, secret.OwnerReferences, 1)
	assert.Equal(t, "Agent", secret.OwnerReferences[0].Kind)
	assert.Equal(t, agent.Name, secret.OwnerReferences[0].Name)

	// Data key must be "apiKey"
	require.Contains(t, secret.Data, "apiKey")
	assert.Equal(t, []byte("sk-virtual-key-123"), secret.Data["apiKey"])

	// Must NOT have a "token" key (that's for bot token secrets)
	assert.NotContains(t, secret.Data, "token")
}

// TestLiteLLMServiceURL verifies URL format.
func TestLiteLLMServiceURL(t *testing.T) {
	url := LiteLLMServiceURL("my-namespace")
	expected := "http://litellm.my-namespace.svc.cluster.local:4000"
	assert.Equal(t, expected, url)
}

// TestGenerateAgentDeployment_WithLLMGateway verifies all 6 LiteLLM env vars are injected
// when LLMGateway.OperatorManaged is set.
func TestGenerateAgentDeployment_WithLLMGateway(t *testing.T) {
	agent := testAgent("my-agent", "my-namespace")
	agent.Spec.LLMGateway = &mmv1beta.LLMGatewayConfig{
		OperatorManaged: &mmv1beta.OperatorManagedLLMGateway{
			Image: mmv1beta.AgentLiteLLMDefaultImage,
		},
	}

	dep := GenerateAgentDeployment(agent)
	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	c := dep.Spec.Template.Spec.Containers[0]

	envMap := make(map[string]*corev1.EnvVar)
	for i := range c.Env {
		envMap[c.Env[i].Name] = &c.Env[i]
	}

	expectedBaseURL := LiteLLMServiceURL("my-namespace")
	expectedKeySecretName := agent.LiteLLMKeySecretName()

	// LITELLM_BASE_URL
	require.Contains(t, envMap, "LITELLM_BASE_URL")
	assert.Equal(t, expectedBaseURL, envMap["LITELLM_BASE_URL"].Value)

	// LITELLM_MCP_URL
	require.Contains(t, envMap, "LITELLM_MCP_URL")
	assert.Equal(t, expectedBaseURL+"/mcp", envMap["LITELLM_MCP_URL"].Value)

	// OPENAI_BASE_URL
	require.Contains(t, envMap, "OPENAI_BASE_URL")
	assert.Equal(t, expectedBaseURL+"/v1", envMap["OPENAI_BASE_URL"].Value)

	// OPENAI_API_KEY from secret
	require.Contains(t, envMap, "OPENAI_API_KEY")
	require.NotNil(t, envMap["OPENAI_API_KEY"].ValueFrom)
	require.NotNil(t, envMap["OPENAI_API_KEY"].ValueFrom.SecretKeyRef)
	assert.Equal(t, expectedKeySecretName, envMap["OPENAI_API_KEY"].ValueFrom.SecretKeyRef.Name)
	assert.Equal(t, "apiKey", envMap["OPENAI_API_KEY"].ValueFrom.SecretKeyRef.Key)

	// ANTHROPIC_BASE_URL
	require.Contains(t, envMap, "ANTHROPIC_BASE_URL")
	assert.Equal(t, expectedBaseURL+"/v1", envMap["ANTHROPIC_BASE_URL"].Value)

	// ANTHROPIC_API_KEY from secret
	require.Contains(t, envMap, "ANTHROPIC_API_KEY")
	require.NotNil(t, envMap["ANTHROPIC_API_KEY"].ValueFrom)
	require.NotNil(t, envMap["ANTHROPIC_API_KEY"].ValueFrom.SecretKeyRef)
	assert.Equal(t, expectedKeySecretName, envMap["ANTHROPIC_API_KEY"].ValueFrom.SecretKeyRef.Name)
	assert.Equal(t, "apiKey", envMap["ANTHROPIC_API_KEY"].ValueFrom.SecretKeyRef.Key)
}

// TestGenerateAgentDeployment_WithLLMGateway_External verifies env vars use the external URL
// and external virtual key secret when LLMGateway.External is set.
func TestGenerateAgentDeployment_WithLLMGateway_External(t *testing.T) {
	agent := testAgent("my-agent", "my-namespace")
	agent.Spec.LLMGateway = &mmv1beta.LLMGatewayConfig{
		External: &mmv1beta.ExternalLLMGateway{
			URL:              "http://litellm.external.svc.cluster.local:4000",
			VirtualKeySecret: "my-external-key-secret",
		},
	}

	dep := GenerateAgentDeployment(agent)
	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	c := dep.Spec.Template.Spec.Containers[0]

	envMap := make(map[string]*corev1.EnvVar)
	for i := range c.Env {
		envMap[c.Env[i].Name] = &c.Env[i]
	}

	require.Contains(t, envMap, "LITELLM_BASE_URL")
	assert.Equal(t, "http://litellm.external.svc.cluster.local:4000", envMap["LITELLM_BASE_URL"].Value)

	require.Contains(t, envMap, "OPENAI_API_KEY")
	require.NotNil(t, envMap["OPENAI_API_KEY"].ValueFrom)
	assert.Equal(t, "my-external-key-secret", envMap["OPENAI_API_KEY"].ValueFrom.SecretKeyRef.Name)
}

// TestGenerateAgentDeployment_WithoutLLMGateway verifies no LiteLLM env vars are present
// when LLMGateway is nil (backwards compatibility).
func TestGenerateAgentDeployment_WithoutLLMGateway(t *testing.T) {
	agent := testAgent("my-agent", "my-namespace")
	// LLMGateway is nil by default from testAgent

	dep := GenerateAgentDeployment(agent)
	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	c := dep.Spec.Template.Spec.Containers[0]

	liteLLMEnvVarNames := []string{
		"LITELLM_BASE_URL",
		"LITELLM_MCP_URL",
		"OPENAI_BASE_URL",
		"OPENAI_API_KEY",
		"ANTHROPIC_BASE_URL",
		"ANTHROPIC_API_KEY",
	}

	envNames := make(map[string]bool)
	for _, e := range c.Env {
		envNames[e.Name] = true
	}

	for _, name := range liteLLMEnvVarNames {
		assert.False(t, envNames[name], "expected no %s env var when LLMGateway is nil", name)
	}
}

// TestGenerateAgentNetworkPolicy_DenyWithLiteLLM verifies 3 egress rules when
// LLMGateway is set and EgressPolicy is "deny".
func TestGenerateAgentNetworkPolicy_DenyWithLiteLLM(t *testing.T) {
	agent := testAgent("my-agent", "my-namespace")
	agent.Spec.EgressPolicy = mmv1beta.AgentEgressPolicyDeny
	agent.Spec.LLMGateway = &mmv1beta.LLMGatewayConfig{
		OperatorManaged: &mmv1beta.OperatorManagedLLMGateway{
			Image: mmv1beta.AgentLiteLLMDefaultImage,
		},
	}

	np := GenerateAgentNetworkPolicy(agent)

	// 3 egress rules: MM server (8065) + LiteLLM (4000) + DNS (53)
	assert.Len(t, np.Spec.Egress, 3)

	// Rule 0: MM pods on 8065
	mmEgress := np.Spec.Egress[0]
	require.Len(t, mmEgress.To, 1)
	assert.Equal(t, agent.Spec.MattermostRef.Name, mmEgress.To[0].PodSelector.MatchLabels[mmv1beta.ClusterLabel])
	require.Len(t, mmEgress.Ports, 1)
	assert.Equal(t, int32(8065), mmEgress.Ports[0].Port.IntVal)

	// Rule 1: LiteLLM pods on 4000
	liteLLMEgress := np.Spec.Egress[1]
	require.Len(t, liteLLMEgress.To, 1)
	assert.Equal(t, liteLLMLabels(), liteLLMEgress.To[0].PodSelector.MatchLabels)
	require.Len(t, liteLLMEgress.Ports, 1)
	assert.Equal(t, mmv1beta.AgentLiteLLMPort, liteLLMEgress.Ports[0].Port.IntVal)
	assert.Equal(t, corev1.ProtocolTCP, *liteLLMEgress.Ports[0].Protocol)

	// Rule 2: DNS
	dnsEgress := np.Spec.Egress[2]
	assert.Len(t, dnsEgress.Ports, 2)
	assert.Equal(t, int32(53), dnsEgress.Ports[0].Port.IntVal)
}

// TestGenerateAgentNetworkPolicy_DenyWithoutLiteLLM verifies 2 egress rules when
// LLMGateway is nil (backwards compatibility).
func TestGenerateAgentNetworkPolicy_DenyWithoutLiteLLM(t *testing.T) {
	agent := testAgent("my-agent", "my-namespace")
	agent.Spec.EgressPolicy = mmv1beta.AgentEgressPolicyDeny
	// LLMGateway is nil — no LiteLLM rule added

	np := GenerateAgentNetworkPolicy(agent)

	// 2 egress rules: MM server (8065) + DNS (53)
	assert.Len(t, np.Spec.Egress, 2)

	// No LiteLLM rule
	for _, rule := range np.Spec.Egress {
		for _, p := range rule.Ports {
			assert.NotEqual(t, mmv1beta.AgentLiteLLMPort, p.Port.IntVal,
				"expected no LiteLLM egress rule when LLMGateway is nil")
		}
	}
}

// TestGenerateAgentNetworkPolicy_AllowListWithLiteLLM verifies 5 egress rules when
// LLMGateway is set and EgressPolicy is "allowList".
func TestGenerateAgentNetworkPolicy_AllowListWithLiteLLM(t *testing.T) {
	agent := testAgent("my-agent", "my-namespace")
	agent.Spec.EgressPolicy = mmv1beta.AgentEgressPolicyAllowList
	agent.Spec.LLMGateway = &mmv1beta.LLMGatewayConfig{
		OperatorManaged: &mmv1beta.OperatorManagedLLMGateway{
			Image: mmv1beta.AgentLiteLLMDefaultImage,
		},
	}

	np := GenerateAgentNetworkPolicy(agent)

	// 5 rules: MM (8065) + LiteLLM (4000) + DNS (53) + HTTPS (443) + HTTP (80)
	assert.Len(t, np.Spec.Egress, 5)
}

// TestGenerateAgentNetworkPolicy_LiteLLMEgressHasCorrectPodSelector ensures the LiteLLM
// egress rule targets pods by the litellm app label, not the agent's own labels.
func TestGenerateAgentNetworkPolicy_LiteLLMEgressHasCorrectPodSelector(t *testing.T) {
	agent := testAgent("my-agent", "my-namespace")
	agent.Spec.LLMGateway = &mmv1beta.LLMGatewayConfig{
		External: &mmv1beta.ExternalLLMGateway{
			URL:              "http://litellm:4000",
			VirtualKeySecret: "key-secret",
		},
	}

	np := GenerateAgentNetworkPolicy(agent)

	// Find the LiteLLM egress rule (port 4000)
	var liteLLMRule *networkingv1.NetworkPolicyEgressRule
	for i := range np.Spec.Egress {
		for _, p := range np.Spec.Egress[i].Ports {
			if p.Port.IntVal == mmv1beta.AgentLiteLLMPort {
				liteLLMRule = &np.Spec.Egress[i]
			}
		}
	}
	require.NotNil(t, liteLLMRule, "expected a LiteLLM egress rule")

	require.Len(t, liteLLMRule.To, 1)
	assert.Equal(t, liteLLMLabels(), liteLLMRule.To[0].PodSelector.MatchLabels)
}
