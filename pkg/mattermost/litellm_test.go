package mattermost

import (
	"testing"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
)

func TestGenerateLiteLLMConfigMap(t *testing.T) {
	cm := GenerateLiteLLMConfigMap("my-namespace")

	assert.Equal(t, mmv1beta.AgentLiteLLMConfigMapName, cm.Name)
	assert.Equal(t, "my-namespace", cm.Namespace)
	assert.Equal(t, LiteLLMLabels(), cm.Labels)

	require.Contains(t, cm.Data, "config.yaml")
	assert.Contains(t, cm.Data["config.yaml"], "store_model_in_db: true")
	assert.Contains(t, cm.Data["config.yaml"], "general_settings")

	assert.Empty(t, cm.OwnerReferences)
}

func TestGenerateLiteLLMDeployment(t *testing.T) {
	dep := GenerateLiteLLMDeployment("my-namespace", mmv1beta.AgentLiteLLMDefaultImage)

	assert.Equal(t, mmv1beta.AgentLiteLLMDeploymentName, dep.Name)
	assert.Equal(t, "my-namespace", dep.Namespace)
	assert.Equal(t, LiteLLMLabels(), dep.Labels)

	assert.Empty(t, dep.OwnerReferences)

	require.NotNil(t, dep.Spec.Replicas)
	assert.Equal(t, int32(1), *dep.Spec.Replicas)

	assert.Equal(t, LiteLLMLabels(), dep.Spec.Selector.MatchLabels)

	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	c := dep.Spec.Template.Spec.Containers[0]

	assert.Equal(t, "litellm", c.Name)
	assert.Equal(t, mmv1beta.AgentLiteLLMDefaultImage, c.Image)
	assert.Equal(t, []string{"--config", "/app/config/config.yaml"}, c.Args)

	envMap := envVarsByName(c.Env)

	require.Contains(t, envMap, "DATABASE_URL")
	require.NotNil(t, envMap["DATABASE_URL"].ValueFrom)
	require.NotNil(t, envMap["DATABASE_URL"].ValueFrom.SecretKeyRef)
	assert.Equal(t, mmv1beta.AgentLiteLLMDBCredentialsSecret, envMap["DATABASE_URL"].ValueFrom.SecretKeyRef.Name)
	assert.Equal(t, "connectionString", envMap["DATABASE_URL"].ValueFrom.SecretKeyRef.Key)

	require.Contains(t, envMap, "LITELLM_MASTER_KEY")
	require.NotNil(t, envMap["LITELLM_MASTER_KEY"].ValueFrom)
	require.NotNil(t, envMap["LITELLM_MASTER_KEY"].ValueFrom.SecretKeyRef)
	assert.Equal(t, mmv1beta.AgentLiteLLMMasterKeySecretName, envMap["LITELLM_MASTER_KEY"].ValueFrom.SecretKeyRef.Name)
	assert.Equal(t, "masterKey", envMap["LITELLM_MASTER_KEY"].ValueFrom.SecretKeyRef.Key)

	require.Contains(t, envMap, "STORE_MODEL_IN_DB")
	assert.Equal(t, "True", envMap["STORE_MODEL_IN_DB"].Value)

	require.Len(t, c.Ports, 1)
	assert.Equal(t, mmv1beta.AgentLiteLLMPort, c.Ports[0].ContainerPort)
	assert.Equal(t, "http", c.Ports[0].Name)

	assert.NotEmpty(t, c.Resources.Requests)
	assert.NotEmpty(t, c.Resources.Limits)

	require.NotNil(t, c.LivenessProbe)
	require.NotNil(t, c.LivenessProbe.HTTPGet)
	assert.Equal(t, "/health/liveliness", c.LivenessProbe.HTTPGet.Path)
	assert.Equal(t, mmv1beta.AgentLiteLLMPort, c.LivenessProbe.HTTPGet.Port.IntVal)

	require.NotNil(t, c.ReadinessProbe)
	require.NotNil(t, c.ReadinessProbe.HTTPGet)
	assert.Equal(t, "/health/readiness", c.ReadinessProbe.HTTPGet.Path)
	assert.Equal(t, mmv1beta.AgentLiteLLMPort, c.ReadinessProbe.HTTPGet.Port.IntVal)

	require.Len(t, c.VolumeMounts, 1)
	assert.Equal(t, "litellm-config", c.VolumeMounts[0].Name)
	assert.Equal(t, "/app/config", c.VolumeMounts[0].MountPath)
	assert.True(t, c.VolumeMounts[0].ReadOnly)

	require.Len(t, dep.Spec.Template.Spec.Volumes, 1)
	vol := dep.Spec.Template.Spec.Volumes[0]
	assert.Equal(t, "litellm-config", vol.Name)
	require.NotNil(t, vol.ConfigMap)
	assert.Equal(t, mmv1beta.AgentLiteLLMConfigMapName, vol.ConfigMap.Name)
}

func TestGenerateLiteLLMService(t *testing.T) {
	svc := GenerateLiteLLMService("my-namespace")

	assert.Equal(t, mmv1beta.AgentLiteLLMServiceName, svc.Name)
	assert.Equal(t, "my-namespace", svc.Namespace)
	assert.Equal(t, LiteLLMLabels(), svc.Labels)

	assert.Empty(t, svc.OwnerReferences)

	assert.Equal(t, corev1.ServiceTypeClusterIP, svc.Spec.Type)
	assert.Equal(t, LiteLLMLabels(), svc.Spec.Selector)

	require.Len(t, svc.Spec.Ports, 1)
	port := svc.Spec.Ports[0]
	assert.Equal(t, "http", port.Name)
	assert.Equal(t, mmv1beta.AgentLiteLLMPort, port.Port)
	assert.Equal(t, mmv1beta.AgentLiteLLMPort, port.TargetPort.IntVal)
}

func TestLiteLLMServiceURL(t *testing.T) {
	url := LiteLLMServiceURL("my-namespace")
	expected := "http://mm-agent-litellm.my-namespace.svc.cluster.local:4000"
	assert.Equal(t, expected, url)
}

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

	envMap := envVarsByName(c.Env)

	expectedBaseURL := LiteLLMServiceURL("my-namespace")
	expectedKeySecretName := agent.LiteLLMKeySecretName()

	require.Contains(t, envMap, "LITELLM_BASE_URL")
	assert.Equal(t, expectedBaseURL, envMap["LITELLM_BASE_URL"].Value)

	require.Contains(t, envMap, "LITELLM_MCP_URL")
	assert.Equal(t, expectedBaseURL+"/mcp", envMap["LITELLM_MCP_URL"].Value)

	require.Contains(t, envMap, "OPENAI_BASE_URL")
	assert.Equal(t, expectedBaseURL+"/v1", envMap["OPENAI_BASE_URL"].Value)

	require.Contains(t, envMap, "OPENAI_API_KEY")
	require.NotNil(t, envMap["OPENAI_API_KEY"].ValueFrom)
	require.NotNil(t, envMap["OPENAI_API_KEY"].ValueFrom.SecretKeyRef)
	assert.Equal(t, expectedKeySecretName, envMap["OPENAI_API_KEY"].ValueFrom.SecretKeyRef.Name)
	assert.Equal(t, "apiKey", envMap["OPENAI_API_KEY"].ValueFrom.SecretKeyRef.Key)

	require.Contains(t, envMap, "ANTHROPIC_BASE_URL")
	assert.Equal(t, expectedBaseURL, envMap["ANTHROPIC_BASE_URL"].Value)

	require.Contains(t, envMap, "ANTHROPIC_API_KEY")
	require.NotNil(t, envMap["ANTHROPIC_API_KEY"].ValueFrom)
	require.NotNil(t, envMap["ANTHROPIC_API_KEY"].ValueFrom.SecretKeyRef)
	assert.Equal(t, expectedKeySecretName, envMap["ANTHROPIC_API_KEY"].ValueFrom.SecretKeyRef.Name)
	assert.Equal(t, "apiKey", envMap["ANTHROPIC_API_KEY"].ValueFrom.SecretKeyRef.Key)
}

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

	envMap := envVarsByName(c.Env)

	require.Contains(t, envMap, "LITELLM_BASE_URL")
	assert.Equal(t, "http://litellm.external.svc.cluster.local:4000", envMap["LITELLM_BASE_URL"].Value)

	require.Contains(t, envMap, "OPENAI_API_KEY")
	require.NotNil(t, envMap["OPENAI_API_KEY"].ValueFrom)
	assert.Equal(t, "my-external-key-secret", envMap["OPENAI_API_KEY"].ValueFrom.SecretKeyRef.Name)
}

func TestGenerateAgentDeployment_WithoutLLMGateway(t *testing.T) {
	agent := testAgent("my-agent", "my-namespace")
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

	envMap := envVarsByName(c.Env)

	for _, name := range liteLLMEnvVarNames {
		assert.NotContains(t, envMap, name)
	}
}

func TestGenerateAgentNetworkPolicy_DenyWithLiteLLM(t *testing.T) {
	agent := testAgent("my-agent", "my-namespace")
	agent.Spec.EgressPolicy = mmv1beta.AgentEgressPolicyDeny
	agent.Spec.LLMGateway = &mmv1beta.LLMGatewayConfig{
		OperatorManaged: &mmv1beta.OperatorManagedLLMGateway{
			Image: mmv1beta.AgentLiteLLMDefaultImage,
		},
	}

	np := GenerateAgentNetworkPolicy(agent)

	require.Len(t, np.Spec.Ingress, 1)
	ingress := np.Spec.Ingress[0]
	require.Len(t, ingress.From, 2, "ingress should allow both MM and LiteLLM pods")
	assert.Equal(t, agent.Spec.MattermostRef.Name, ingress.From[0].PodSelector.MatchLabels[mmv1beta.ClusterLabel])
	assert.Equal(t, "mm-agent-litellm", ingress.From[1].PodSelector.MatchLabels["app"])
	require.Len(t, ingress.Ports, 1)
	assert.Equal(t, mmv1beta.AgentHTTPPort, ingress.Ports[0].Port.IntVal)

	assert.Len(t, np.Spec.Egress, 3)

	mmEgress := np.Spec.Egress[0]
	require.Len(t, mmEgress.To, 1)
	assert.Equal(t, agent.Spec.MattermostRef.Name, mmEgress.To[0].PodSelector.MatchLabels[mmv1beta.ClusterLabel])
	require.Len(t, mmEgress.Ports, 1)
	assert.Equal(t, int32(8065), mmEgress.Ports[0].Port.IntVal)

	liteLLMEgress := np.Spec.Egress[1]
	require.Len(t, liteLLMEgress.To, 1)
	assert.Equal(t, LiteLLMLabels(), liteLLMEgress.To[0].PodSelector.MatchLabels)
	require.Len(t, liteLLMEgress.Ports, 1)
	assert.Equal(t, mmv1beta.AgentLiteLLMPort, liteLLMEgress.Ports[0].Port.IntVal)
	assert.Equal(t, corev1.ProtocolTCP, *liteLLMEgress.Ports[0].Protocol)

	dnsEgress := np.Spec.Egress[2]
	assert.Len(t, dnsEgress.Ports, 2)
	assert.Equal(t, int32(53), dnsEgress.Ports[0].Port.IntVal)
}

func TestGenerateAgentNetworkPolicy_DenyWithoutLiteLLM(t *testing.T) {
	agent := testAgent("my-agent", "my-namespace")
	agent.Spec.EgressPolicy = mmv1beta.AgentEgressPolicyDeny

	np := GenerateAgentNetworkPolicy(agent)

	assert.Len(t, np.Spec.Egress, 2)

	for _, rule := range np.Spec.Egress {
		for _, p := range rule.Ports {
			assert.NotEqual(t, mmv1beta.AgentLiteLLMPort, p.Port.IntVal,
				"expected no LiteLLM egress rule when LLMGateway is nil")
		}
	}
}

func TestGenerateAgentNetworkPolicy_AllowWebWithLiteLLM(t *testing.T) {
	agent := testAgent("my-agent", "my-namespace")
	agent.Spec.EgressPolicy = mmv1beta.AgentEgressPolicyAllowWeb
	agent.Spec.LLMGateway = &mmv1beta.LLMGatewayConfig{
		OperatorManaged: &mmv1beta.OperatorManagedLLMGateway{
			Image: mmv1beta.AgentLiteLLMDefaultImage,
		},
	}

	np := GenerateAgentNetworkPolicy(agent)

	assert.Len(t, np.Spec.Egress, 5)
}

func TestGenerateAgentNetworkPolicy_LiteLLMEgressHasCorrectPodSelector(t *testing.T) {
	agent := testAgent("my-agent", "my-namespace")
	agent.Spec.LLMGateway = &mmv1beta.LLMGatewayConfig{
		External: &mmv1beta.ExternalLLMGateway{
			URL:              "http://litellm:4000",
			VirtualKeySecret: "key-secret",
		},
	}

	np := GenerateAgentNetworkPolicy(agent)

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
	assert.Equal(t, LiteLLMLabels(), liteLLMRule.To[0].PodSelector.MatchLabels)
}
