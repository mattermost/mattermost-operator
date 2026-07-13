package mattermost

import (
	"testing"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// testMattermostWithGateway returns a Mattermost CR with the LiteLLM gateway
// enabled and defaults applied.
func testMattermostWithGateway(t *testing.T, name, ns string) *mmv1beta.Mattermost {
	mm := &mmv1beta.Mattermost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: mmv1beta.MattermostSpec{
			IngressName: "test.example.com",
			Agents: &mmv1beta.MattermostAgents{
				Enabled:    true,
				LLMGateway: &mmv1beta.AgentsLLMGateway{},
			},
		},
	}
	require.NoError(t, mm.SetDefaults())
	return mm
}

func TestGenerateLiteLLMMasterKeySecret(t *testing.T) {
	secret := GenerateLiteLLMMasterKeySecret("my-namespace", "sk-test-key")

	assert.Equal(t, mmv1beta.AgentLiteLLMMasterKeySecretName, secret.Name)
	assert.Equal(t, "my-namespace", secret.Namespace)
	assert.Equal(t, LiteLLMLabels(), secret.Labels)
	assert.Equal(t, []byte("sk-test-key"), secret.Data["masterKey"])
	assert.Empty(t, secret.OwnerReferences)
}

func TestGenerateLiteLLMDeployment(t *testing.T) {
	mm := testMattermostWithGateway(t, "mm-test", "my-namespace")
	dep := GenerateLiteLLMDeployment(mm)

	assert.Equal(t, mmv1beta.AgentLiteLLMDeploymentName, dep.Name)
	assert.Equal(t, "my-namespace", dep.Namespace)
	assert.Equal(t, LiteLLMLabels(), dep.Labels)

	require.NotNil(t, dep.Spec.Replicas)
	assert.Equal(t, int32(1), *dep.Spec.Replicas)

	assert.Equal(t, LiteLLMLabels(), dep.Spec.Selector.MatchLabels)

	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	c := dep.Spec.Template.Spec.Containers[0]

	assert.Equal(t, "litellm", c.Name)
	assert.Equal(t, mmv1beta.AgentLiteLLMDefaultImage, c.Image)
	assert.Empty(t, c.Args)

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

	// Default resources applied by SetDefaults.
	assert.Equal(t, resource.MustParse("500m"), c.Resources.Requests[corev1.ResourceCPU])
	assert.Equal(t, resource.MustParse("512Mi"), c.Resources.Requests[corev1.ResourceMemory])
	assert.Equal(t, resource.MustParse("2"), c.Resources.Limits[corev1.ResourceCPU])
	assert.Equal(t, resource.MustParse("2Gi"), c.Resources.Limits[corev1.ResourceMemory])

	require.NotNil(t, c.LivenessProbe)
	require.NotNil(t, c.LivenessProbe.HTTPGet)
	assert.Equal(t, "/health/liveliness", c.LivenessProbe.HTTPGet.Path)
	assert.Equal(t, mmv1beta.AgentLiteLLMPort, c.LivenessProbe.HTTPGet.Port.IntVal)

	require.NotNil(t, c.ReadinessProbe)
	require.NotNil(t, c.ReadinessProbe.HTTPGet)
	assert.Equal(t, "/health/readiness", c.ReadinessProbe.HTTPGet.Path)
	assert.Equal(t, mmv1beta.AgentLiteLLMPort, c.ReadinessProbe.HTTPGet.Port.IntVal)

	// The config file/ConfigMap was removed; STORE_MODEL_IN_DB covers it.
	assert.Empty(t, c.VolumeMounts)
	assert.Empty(t, dep.Spec.Template.Spec.Volumes)
}

func TestGenerateLiteLLMDeployment_CustomImageAndResources(t *testing.T) {
	mm := testMattermostWithGateway(t, "mm-test", "my-namespace")
	mm.Spec.Agents.LLMGateway.Image = "ghcr.io/berriai/litellm-database:v1.99.9"
	mm.Spec.Agents.LLMGateway.Resources = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("250m"),
			corev1.ResourceMemory: resource.MustParse("256Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("1Gi"),
		},
	}

	dep := GenerateLiteLLMDeployment(mm)

	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	c := dep.Spec.Template.Spec.Containers[0]

	assert.Equal(t, "ghcr.io/berriai/litellm-database:v1.99.9", c.Image)
	assert.Equal(t, corev1.PullIfNotPresent, c.ImagePullPolicy)
	assert.Equal(t, resource.MustParse("250m"), c.Resources.Requests[corev1.ResourceCPU])
	assert.Equal(t, resource.MustParse("256Mi"), c.Resources.Requests[corev1.ResourceMemory])
	assert.Equal(t, resource.MustParse("1"), c.Resources.Limits[corev1.ResourceCPU])
	assert.Equal(t, resource.MustParse("1Gi"), c.Resources.Limits[corev1.ResourceMemory])
}

func TestGenerateLiteLLMDeployment_MutableTagAlwaysPulls(t *testing.T) {
	mm := testMattermostWithGateway(t, "mm-test", "my-namespace")
	mm.Spec.Agents.LLMGateway.Image = "ghcr.io/berriai/litellm-database:latest"

	dep := GenerateLiteLLMDeployment(mm)

	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	assert.Equal(t, corev1.PullAlways, dep.Spec.Template.Spec.Containers[0].ImagePullPolicy)
}

func TestGenerateLiteLLMService(t *testing.T) {
	mm := testMattermostWithGateway(t, "mm-test", "my-namespace")
	svc := GenerateLiteLLMService(mm)

	assert.Equal(t, mmv1beta.AgentLiteLLMServiceName, svc.Name)
	assert.Equal(t, "my-namespace", svc.Namespace)
	assert.Equal(t, LiteLLMLabels(), svc.Labels)

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
		OperatorManaged: &mmv1beta.OperatorManagedGateway{},
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
		OperatorManaged: &mmv1beta.OperatorManagedGateway{},
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
		OperatorManaged: &mmv1beta.OperatorManagedGateway{},
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
