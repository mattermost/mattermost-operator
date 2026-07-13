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
			MattermostRef: mmv1beta.AgentMattermostRef{
				Name: "mm-prod",
			},
		},
	}
}

func envVarsByName(env []corev1.EnvVar) map[string]*corev1.EnvVar {
	envMap := make(map[string]*corev1.EnvVar)
	for i := range env {
		envMap[env[i].Name] = &env[i]
	}
	return envMap
}

func findEgressRuleByPort(t *testing.T, policy *networkingv1.NetworkPolicy, port int32) *networkingv1.NetworkPolicyEgressRule {
	t.Helper()

	var matches []*networkingv1.NetworkPolicyEgressRule
	for i := range policy.Spec.Egress {
		for _, policyPort := range policy.Spec.Egress[i].Ports {
			if policyPort.Port != nil && policyPort.Port.IntVal == port {
				matches = append(matches, &policy.Spec.Egress[i])
				break
			}
		}
	}
	require.Len(t, matches, 1, "expected exactly one egress rule containing port %d", port)
	return matches[0]
}

func egressRuleHasPort(rule *networkingv1.NetworkPolicyEgressRule, port int32) bool {
	for _, policyPort := range rule.Ports {
		if policyPort.Port != nil && policyPort.Port.IntVal == port {
			return true
		}
	}
	return false
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
	assert.Equal(t, mmv1beta.AgentLabels(agent), sa.Labels)
	assert.Equal(t, "my-agent", sa.Labels[mmv1beta.AgentNameLabel])
	assert.Equal(t, "mm-prod", sa.Labels[mmv1beta.ClusterLabel])
	assert.Equal(t, "my-agent", sa.Labels[mmv1beta.ClusterResourceLabel])
	assert.Equal(t, mmv1beta.AgentAppName, sa.Labels["app"])
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
	assert.Equal(t, int32(8080), svc.Spec.Ports[0].Port)
	assert.Equal(t, "http", svc.Spec.Ports[0].Name)

	assert.Equal(t, mmv1beta.AgentSelectorLabels(agent), svc.Spec.Selector)
	assert.NotContains(t, svc.Spec.Selector, mmv1beta.ClusterLabel)
}

func TestGenerateAgentDeployment(t *testing.T) {
	agent := testAgent("my-agent", "test-ns")
	dep := GenerateAgentDeployment(agent)

	assert.Equal(t, "my-agent", dep.Name)
	assert.Equal(t, "test-ns", dep.Namespace)
	assert.Len(t, dep.OwnerReferences, 1)

	assert.Equal(t, int32(1), *dep.Spec.Replicas)

	assert.Equal(t, mmv1beta.AgentSelectorLabels(agent), dep.Spec.Selector.MatchLabels)

	assert.Equal(t, "my-agent", dep.Spec.Template.Spec.ServiceAccountName)

	containers := dep.Spec.Template.Spec.Containers
	assert.Len(t, containers, 1)
	c := containers[0]
	assert.Equal(t, mmv1beta.AgentContainerName, c.Name)
	assert.Equal(t, "mattermost/test-agent:latest", c.Image)
	assert.Equal(t, agent.Spec.Resources, c.Resources)

	assert.Equal(t, corev1.PullAlways, c.ImagePullPolicy, "latest tag should get PullAlways")

	assert.Len(t, c.Ports, 1)
	assert.Equal(t, int32(8080), c.Ports[0].ContainerPort)
	require.NotNil(t, c.ReadinessProbe)
	require.NotNil(t, c.ReadinessProbe.TCPSocket)
	assert.Equal(t, mmv1beta.AgentHTTPPort, c.ReadinessProbe.TCPSocket.Port.IntVal)

	envMap := envVarsByName(c.Env)
	require.Contains(t, envMap, "MM_SERVER_URL")
	require.Contains(t, envMap, "AGENT_HOOKS")
	assert.Equal(t, "http://mm-prod.test-ns.svc.cluster.local:8065", envMap["MM_SERVER_URL"].Value)
	assert.Equal(t, "MessageHasBeenPosted,UserHasJoinedChannel", envMap["AGENT_HOOKS"].Value)

	hookSecretEnv := envMap["HOOK_SECRET"]
	require.NotNil(t, hookSecretEnv, "HOOK_SECRET env var must be present")
	require.NotNil(t, hookSecretEnv.ValueFrom)
	require.NotNil(t, hookSecretEnv.ValueFrom.SecretKeyRef)
	assert.Equal(t, agent.HookSecretName(), hookSecretEnv.ValueFrom.SecretKeyRef.Name)
	assert.Equal(t, mmv1beta.SecretKeyHookSecret, hookSecretEnv.ValueFrom.SecretKeyRef.Key)

	assert.Len(t, c.VolumeMounts, 1)
	assert.Equal(t, "bot-token", c.VolumeMounts[0].Name)
	assert.Equal(t, "/secrets/mmctl-token", c.VolumeMounts[0].MountPath)
	assert.True(t, c.VolumeMounts[0].ReadOnly)

	volumes := dep.Spec.Template.Spec.Volumes
	assert.Len(t, volumes, 1)
	assert.Equal(t, "bot-token", volumes[0].Name)
	assert.Equal(t, agent.BotTokenSecretName(), volumes[0].Secret.SecretName)
}

func TestGenerateAgentDeployment_CustomEnvVars(t *testing.T) {
	agent := testAgent("my-agent", "default")
	agent.Spec.Env = []corev1.EnvVar{
		{Name: "CUSTOM_VAR", Value: "custom-value"},
		{Name: "MM_SERVER_URL", Value: "should-not-override"},
	}

	dep := GenerateAgentDeployment(agent)
	c := dep.Spec.Template.Spec.Containers[0]

	envMap := envVarsByName(c.Env)

	require.Contains(t, envMap, "CUSTOM_VAR")
	require.Contains(t, envMap, "MM_SERVER_URL")
	assert.Equal(t, "custom-value", envMap["CUSTOM_VAR"].Value)
	assert.Equal(t, "should-not-override", envMap["MM_SERVER_URL"].Value)
}

func TestGenerateAgentDeployment_GatewayEnvMatrix(t *testing.T) {
	tests := []struct {
		name          string
		gateway       *mmv1beta.LLMGatewayConfig
		expectedURL   string
		expectedKey   string
		expectGateway bool
	}{
		{name: "none"},
		{
			name: "operator managed",
			gateway: &mmv1beta.LLMGatewayConfig{
				OperatorManaged: &mmv1beta.OperatorManagedGateway{},
			},
			expectedURL:   mmv1beta.LiteLLMServiceURL("my-namespace"),
			expectedKey:   "agent-my-agent-litellm-key",
			expectGateway: true,
		},
		{
			name: "external",
			gateway: &mmv1beta.LLMGatewayConfig{
				External: &mmv1beta.ExternalLLMGateway{
					URL:              "https://gateway.example.com:8443",
					VirtualKeySecret: "external-key-secret",
				},
			},
			expectedURL:   "https://gateway.example.com:8443",
			expectedKey:   "external-key-secret",
			expectGateway: true,
		},
	}

	gatewayEnvNames := []string{
		"LITELLM_BASE_URL",
		"LITELLM_MCP_URL",
		"OPENAI_BASE_URL",
		"OPENAI_API_KEY",
		"ANTHROPIC_BASE_URL",
		"ANTHROPIC_API_KEY",
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := testAgent("my-agent", "my-namespace")
			agent.Spec.LLMGateway = tt.gateway
			deployment := GenerateAgentDeployment(agent)
			require.Len(t, deployment.Spec.Template.Spec.Containers, 1)
			env := envVarsByName(deployment.Spec.Template.Spec.Containers[0].Env)

			if !tt.expectGateway {
				for _, name := range gatewayEnvNames {
					assert.NotContains(t, env, name)
				}
				return
			}

			for _, name := range gatewayEnvNames {
				require.Contains(t, env, name)
			}
			assert.Equal(t, tt.expectedURL, env["LITELLM_BASE_URL"].Value)
			assert.Equal(t, tt.expectedURL+"/mcp", env["LITELLM_MCP_URL"].Value)
			assert.Equal(t, tt.expectedURL+"/v1", env["OPENAI_BASE_URL"].Value)
			assert.Equal(t, tt.expectedURL, env["ANTHROPIC_BASE_URL"].Value)
			for _, name := range []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY"} {
				require.NotNil(t, env[name].ValueFrom)
				require.NotNil(t, env[name].ValueFrom.SecretKeyRef)
				assert.Equal(t, tt.expectedKey, env[name].ValueFrom.SecretKeyRef.Name)
				assert.Equal(t, mmv1beta.SecretKeyAPIKey, env[name].ValueFrom.SecretKeyRef.Key)
			}
		})
	}
}

func TestGenerateAgentDeployment_WithStorage(t *testing.T) {
	agent := testAgent("my-agent", "test-ns")
	storageClass := "fast-ssd"
	agent.Spec.Storage = &mmv1beta.AgentStorageConfig{
		Size:             resource.MustParse("5Gi"),
		StorageClassName: &storageClass,
		MountPath:        "/workspace",
	}

	dep := GenerateAgentDeployment(agent)

	volumes := dep.Spec.Template.Spec.Volumes
	assert.Len(t, volumes, 2)
	assert.Equal(t, "bot-token", volumes[0].Name)
	assert.Equal(t, "agent-storage", volumes[1].Name)
	assert.Equal(t, agent.StoragePVCName(), volumes[1].PersistentVolumeClaim.ClaimName)

	mounts := dep.Spec.Template.Spec.Containers[0].VolumeMounts
	assert.Len(t, mounts, 2)
	assert.Equal(t, "bot-token", mounts[0].Name)
	assert.Equal(t, "agent-storage", mounts[1].Name)
	assert.Equal(t, "/workspace", mounts[1].MountPath)
}

func TestGenerateAgentDeployment_WithoutStorage(t *testing.T) {
	agent := testAgent("my-agent", "test-ns")
	// Storage is nil by default in testAgent

	dep := GenerateAgentDeployment(agent)

	volumes := dep.Spec.Template.Spec.Volumes
	assert.Len(t, volumes, 1)
	assert.Equal(t, "bot-token", volumes[0].Name)

	mounts := dep.Spec.Template.Spec.Containers[0].VolumeMounts
	assert.Len(t, mounts, 1)
	assert.Equal(t, "bot-token", mounts[0].Name)
}

func TestSetDefaults_StorageMountPath(t *testing.T) {
	agent := &mmv1beta.Agent{
		Spec: mmv1beta.AgentSpec{
			Image:         "test:latest",
			MattermostRef: mmv1beta.AgentMattermostRef{Name: "mm"},
			Storage: &mmv1beta.AgentStorageConfig{
				Size: resource.MustParse("1Gi"),
			},
		},
	}
	agent.SetDefaults()
	assert.Equal(t, mmv1beta.AgentStorageDefaultMountPath, agent.Spec.Storage.MountPath)
}

func TestSetDefaults_StorageMountPathPreserved(t *testing.T) {
	agent := &mmv1beta.Agent{
		Spec: mmv1beta.AgentSpec{
			Image:         "test:latest",
			MattermostRef: mmv1beta.AgentMattermostRef{Name: "mm"},
			Storage: &mmv1beta.AgentStorageConfig{
				Size:      resource.MustParse("1Gi"),
				MountPath: "/custom/path",
			},
		},
	}
	agent.SetDefaults()
	assert.Equal(t, "/custom/path", agent.Spec.Storage.MountPath)
}

func TestSetDefaults_ResourcesAsPair(t *testing.T) {
	t.Run("defaults both when both are unset", func(t *testing.T) {
		agent := testAgent("my-agent", "default")
		agent.Spec.Resources = corev1.ResourceRequirements{}

		agent.SetDefaults()

		assert.Equal(t, resource.MustParse("100m"), agent.Spec.Resources.Requests[corev1.ResourceCPU])
		assert.Equal(t, resource.MustParse("500m"), agent.Spec.Resources.Limits[corev1.ResourceCPU])
	})

	t.Run("preserves limits-only configuration", func(t *testing.T) {
		agent := testAgent("my-agent", "default")
		agent.Spec.Resources = corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("50m"),
			},
		}

		agent.SetDefaults()

		assert.Nil(t, agent.Spec.Resources.Requests)
		assert.Equal(t, resource.MustParse("50m"), agent.Spec.Resources.Limits[corev1.ResourceCPU])
	})
}

func TestStoragePVCName(t *testing.T) {
	agent := &mmv1beta.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "my-agent"},
	}
	assert.Equal(t, "agent-my-agent-storage", agent.StoragePVCName())
}

func TestGenerateAgentHookSecret(t *testing.T) {
	agent := testAgent("my-agent", "default")
	secret := GenerateAgentHookSecret(agent, "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890")

	assert.Equal(t, agent.HookSecretName(), secret.Name)
	assert.Equal(t, "default", secret.Namespace)
	assert.Equal(t, mmv1beta.AgentLabels(agent), secret.Labels)
	assert.Len(t, secret.OwnerReferences, 1)
	assert.Equal(t, "Agent", secret.OwnerReferences[0].Kind)
	assert.Equal(t, []byte("abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"), secret.Data[mmv1beta.SecretKeyHookSecret])

	assert.NotContains(t, secret.Data, mmv1beta.SecretKeyBotToken)
	assert.NotContains(t, secret.Data, mmv1beta.SecretKeyAPIKey)
}

func TestGenerateAgentNetworkPolicy_EgressPolicyGatewayMatrix(t *testing.T) {
	policies := []struct {
		name            string
		value           mmv1beta.AgentEgressPolicy
		baseEgressCount int
		restricted      bool
		expectWebEgress bool
	}{
		{name: "empty", baseEgressCount: 2, restricted: true},
		{name: "deny", value: mmv1beta.AgentEgressPolicyDeny, baseEgressCount: 2, restricted: true},
		{name: "allow web", value: mmv1beta.AgentEgressPolicyAllowWeb, baseEgressCount: 3, restricted: true, expectWebEgress: true},
		{name: "allow", value: mmv1beta.AgentEgressPolicyAllow, baseEgressCount: 1},
		{name: "unknown", value: mmv1beta.AgentEgressPolicy("unknown"), baseEgressCount: 2, restricted: true},
	}
	gateways := []struct {
		name                 string
		config               *mmv1beta.LLMGatewayConfig
		restrictedEgressPort int32
		operatorManaged      bool
	}{
		{name: "none"},
		{
			name: "operator managed",
			config: &mmv1beta.LLMGatewayConfig{
				OperatorManaged: &mmv1beta.OperatorManagedGateway{},
			},
			restrictedEgressPort: mmv1beta.AgentLiteLLMPort,
			operatorManaged:      true,
		},
		{
			name: "external explicit port",
			config: &mmv1beta.LLMGatewayConfig{
				External: &mmv1beta.ExternalLLMGateway{
					URL:              "https://gateway.example.com:8443",
					VirtualKeySecret: "external-key-secret",
				},
			},
			restrictedEgressPort: 8443,
		},
	}

	for _, policy := range policies {
		for _, gateway := range gateways {
			t.Run(policy.name+"/"+gateway.name, func(t *testing.T) {
				agent := testAgent("my-agent", "default")
				agent.Spec.EgressPolicy = policy.value
				agent.Spec.LLMGateway = gateway.config

				networkPolicy := GenerateAgentNetworkPolicy(agent)

				assert.Equal(t, "my-agent", networkPolicy.Name)
				assert.Equal(t, "default", networkPolicy.Namespace)
				assert.Len(t, networkPolicy.OwnerReferences, 1)
				assert.ElementsMatch(t, []networkingv1.PolicyType{
					networkingv1.PolicyTypeIngress,
					networkingv1.PolicyTypeEgress,
				}, networkPolicy.Spec.PolicyTypes)

				expectedEgressCount := policy.baseEgressCount
				if policy.restricted && gateway.restrictedEgressPort != 0 {
					expectedEgressCount++
				}
				assert.Len(t, networkPolicy.Spec.Egress, expectedEgressCount)

				require.Len(t, networkPolicy.Spec.Ingress, 1)
				expectedIngressPeers := 1
				if gateway.operatorManaged {
					expectedIngressPeers++
				}
				assert.Len(t, networkPolicy.Spec.Ingress[0].From, expectedIngressPeers)

				if !policy.restricted {
					require.Len(t, networkPolicy.Spec.Egress, 1)
					assert.Empty(t, networkPolicy.Spec.Egress[0].To)
					assert.Empty(t, networkPolicy.Spec.Egress[0].Ports)
					return
				}

				mattermostEgress := findEgressRuleByPort(t, networkPolicy, MattermostServerPort)
				require.Len(t, mattermostEgress.To, 1)
				require.NotNil(t, mattermostEgress.To[0].PodSelector)
				assert.Equal(
					t,
					mmv1beta.MattermostSelectorLabels(agent.Spec.MattermostRef.Name),
					mattermostEgress.To[0].PodSelector.MatchLabels,
				)

				dnsEgress := findEgressRuleByPort(t, networkPolicy, 53)
				require.Len(t, dnsEgress.Ports, 2)
				assert.Empty(t, dnsEgress.To)
				protocols := make([]corev1.Protocol, 0, len(dnsEgress.Ports))
				for _, port := range dnsEgress.Ports {
					require.NotNil(t, port.Protocol)
					protocols = append(protocols, *port.Protocol)
				}
				assert.ElementsMatch(t, []corev1.Protocol{
					corev1.ProtocolTCP,
					corev1.ProtocolUDP,
				}, protocols)

				if gateway.restrictedEgressPort != 0 {
					gatewayEgress := findEgressRuleByPort(
						t, networkPolicy, gateway.restrictedEgressPort,
					)
					if gateway.operatorManaged {
						require.Len(t, gatewayEgress.To, 1)
						require.NotNil(t, gatewayEgress.To[0].PodSelector)
						assert.Equal(
							t,
							LiteLLMSelectorLabels(),
							gatewayEgress.To[0].PodSelector.MatchLabels,
						)
					} else {
						assert.Empty(t, gatewayEgress.To)
					}
				}

				if policy.expectWebEgress {
					webEgress := findEgressRuleByPort(t, networkPolicy, 443)
					assert.Empty(t, webEgress.To)
					assert.True(t, egressRuleHasPort(webEgress, 80))
				}
			})
		}
	}
}

func TestExternalGatewayPort(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		port     int32
		expected bool
	}{
		{name: "explicit", url: "https://gateway.example.com:8443", port: 8443, expected: true},
		{name: "https default", url: "https://gateway.example.com", port: 443, expected: true},
		{name: "http default", url: "http://gateway.example.com/path", port: 80, expected: true},
		{name: "unparseable", url: "http://[::1", expected: false},
		// CRD validation now rejects scheme-less URLs; already-persisted
		// objects fall back to the HTTPS default rather than silently
		// dropping the egress rule.
		{name: "legacy scheme-less", url: "litellm.example.com:4000", port: 443, expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port, ok := externalGatewayPort(tt.url)
			assert.Equal(t, tt.expected, ok)
			assert.Equal(t, tt.port, port)
		})
	}
}

func TestImageTagNeedsAlwaysPull(t *testing.T) {
	tests := []struct {
		image    string
		expected bool
	}{
		{"myimage:dev", true},
		{"myimage:latest", true},
		{"myimage", true},
		{"registry:5000/path/img", true},
		{"registry:5000/path/img:dev", true},
		{"myimage:v1.2.3", false},
		{"myimage:stable", false},
		{"ghcr.io/org/litellm:v1.82.0-stable", false},
	}
	for _, tt := range tests {
		t.Run(tt.image, func(t *testing.T) {
			assert.Equal(t, tt.expected, imageTagNeedsAlwaysPull(tt.image))
		})
	}
}

func TestGenerateAgentDeployment_ImagePullPolicy(t *testing.T) {
	t.Run("dev tag gets PullAlways", func(t *testing.T) {
		agent := testAgent("my-agent", "default")
		agent.Spec.Image = "mattermost/test-agent:dev"
		dep := GenerateAgentDeployment(agent)
		assert.Equal(t, corev1.PullAlways, dep.Spec.Template.Spec.Containers[0].ImagePullPolicy)
	})

	t.Run("latest tag gets PullAlways", func(t *testing.T) {
		agent := testAgent("my-agent", "default")
		dep := GenerateAgentDeployment(agent)
		assert.Equal(t, corev1.PullAlways, dep.Spec.Template.Spec.Containers[0].ImagePullPolicy)
	})

	t.Run("no tag gets PullAlways", func(t *testing.T) {
		agent := testAgent("my-agent", "default")
		agent.Spec.Image = "mattermost/test-agent"
		dep := GenerateAgentDeployment(agent)
		assert.Equal(t, corev1.PullAlways, dep.Spec.Template.Spec.Containers[0].ImagePullPolicy)
	})

	t.Run("versioned tag gets PullIfNotPresent", func(t *testing.T) {
		agent := testAgent("my-agent", "default")
		agent.Spec.Image = "mattermost/test-agent:v1.0.0"
		dep := GenerateAgentDeployment(agent)
		assert.Equal(t, corev1.PullIfNotPresent, dep.Spec.Template.Spec.Containers[0].ImagePullPolicy)
	})
}

func TestGenerateAgentPVC(t *testing.T) {
	agent := testAgent("my-agent", "default")
	storageClass := "fast-ssd"
	agent.Spec.Storage = &mmv1beta.AgentStorageConfig{
		Size:             resource.MustParse("5Gi"),
		StorageClassName: &storageClass,
		MountPath:        "/workspace",
	}

	pvc := GenerateAgentPVC(agent)

	assert.Equal(t, agent.StoragePVCName(), pvc.Name)
	assert.Equal(t, "default", pvc.Namespace)
	assert.Equal(t, mmv1beta.AgentLabels(agent), pvc.Labels)
	assert.Len(t, pvc.OwnerReferences, 1)
	assert.Equal(t, "Agent", pvc.OwnerReferences[0].Kind)
	assert.Equal(t, []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}, pvc.Spec.AccessModes)
	assert.Equal(t, resource.MustParse("5Gi"), pvc.Spec.Resources.Requests[corev1.ResourceStorage])
	require.NotNil(t, pvc.Spec.StorageClassName)
	assert.Equal(t, "fast-ssd", *pvc.Spec.StorageClassName)
}
