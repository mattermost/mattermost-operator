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
	assert.Equal(t, int32(8080), svc.Spec.Ports[0].Port)
	assert.Equal(t, "http", svc.Spec.Ports[0].Name)

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

	// ImagePullPolicy — testAgent uses :latest, should be PullAlways
	assert.Equal(t, corev1.PullAlways, c.ImagePullPolicy, "latest tag should get PullAlways")

	// Ports
	assert.Len(t, c.Ports, 1)
	assert.Equal(t, int32(8080), c.Ports[0].ContainerPort)

	// Env vars
	envMap := make(map[string]string)
	for _, e := range c.Env {
		envMap[e.Name] = e.Value
	}
	assert.Equal(t, "http://mm-prod.test-ns.svc.cluster.local:8065", envMap["MM_SERVER_URL"])
	assert.Equal(t, "MessageHasBeenPosted,UserHasJoinedChannel", envMap["AGENT_HOOKS"])

	// HOOK_SECRET from secret
	var hookSecretEnv *corev1.EnvVar
	for i, e := range c.Env {
		if e.Name == "HOOK_SECRET" {
			hookSecretEnv = &c.Env[i]
		}
	}
	require.NotNil(t, hookSecretEnv, "HOOK_SECRET env var must be present")
	require.NotNil(t, hookSecretEnv.ValueFrom)
	require.NotNil(t, hookSecretEnv.ValueFrom.SecretKeyRef)
	assert.Equal(t, agent.HookSecretName(), hookSecretEnv.ValueFrom.SecretKeyRef.Name)
	assert.Equal(t, "hookSecret", hookSecretEnv.ValueFrom.SecretKeyRef.Key)

	// Volume mounts on main container — only bot-token remains
	assert.Len(t, c.VolumeMounts, 1)
	assert.Equal(t, "bot-token", c.VolumeMounts[0].Name)
	assert.Equal(t, "/secrets/mmctl-token", c.VolumeMounts[0].MountPath)
	assert.True(t, c.VolumeMounts[0].ReadOnly)

	// No init containers
	assert.Empty(t, dep.Spec.Template.Spec.InitContainers, "init containers must be removed")

	// No HOME env var
	for _, e := range c.Env {
		assert.NotEqual(t, "HOME", e.Name, "HOME env var must not be present")
	}

	// Volumes — only bot-token remains
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

func TestGenerateAgentDeployment_WithStorage(t *testing.T) {
	agent := testAgent("my-agent", "test-ns")
	storageClass := "fast-ssd"
	agent.Spec.Storage = &mmv1beta.AgentStorageConfig{
		Size:             resource.MustParse("5Gi"),
		StorageClassName: &storageClass,
		MountPath:        "/workspace",
	}

	dep := GenerateAgentDeployment(agent)

	// Volumes: bot-token + agent-storage
	volumes := dep.Spec.Template.Spec.Volumes
	assert.Len(t, volumes, 2)
	assert.Equal(t, "bot-token", volumes[0].Name)
	assert.Equal(t, "agent-storage", volumes[1].Name)
	assert.Equal(t, agent.StoragePVCName(), volumes[1].PersistentVolumeClaim.ClaimName)

	// Volume mounts: bot-token + agent-storage
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

	// Only bot-token volume
	volumes := dep.Spec.Template.Spec.Volumes
	assert.Len(t, volumes, 1)
	assert.Equal(t, "bot-token", volumes[0].Name)

	// Only bot-token mount
	mounts := dep.Spec.Template.Spec.Containers[0].VolumeMounts
	assert.Len(t, mounts, 1)
	assert.Equal(t, "bot-token", mounts[0].Name)
}

func TestSetDefaults_StorageMountPath(t *testing.T) {
	agent := &mmv1beta.Agent{
		Spec: mmv1beta.AgentSpec{
			Image:         "test:latest",
			MattermostRef: corev1.LocalObjectReference{Name: "mm"},
			Storage: &mmv1beta.AgentStorageConfig{
				Size: resource.MustParse("1Gi"),
			},
		},
	}
	err := agent.SetDefaults()
	require.NoError(t, err)
	assert.Equal(t, mmv1beta.AgentStorageDefaultMountPath, agent.Spec.Storage.MountPath)
}

func TestSetDefaults_StorageMountPathPreserved(t *testing.T) {
	agent := &mmv1beta.Agent{
		Spec: mmv1beta.AgentSpec{
			Image:         "test:latest",
			MattermostRef: corev1.LocalObjectReference{Name: "mm"},
			Storage: &mmv1beta.AgentStorageConfig{
				Size:      resource.MustParse("1Gi"),
				MountPath: "/custom/path",
			},
		},
	}
	err := agent.SetDefaults()
	require.NoError(t, err)
	assert.Equal(t, "/custom/path", agent.Spec.Storage.MountPath)
}

func TestStoragePVCName(t *testing.T) {
	agent := &mmv1beta.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "my-agent"},
	}
	assert.Equal(t, "my-agent-storage", agent.StoragePVCName())
}

func TestGenerateAgentHookSecret(t *testing.T) {
	agent := testAgent("my-agent", "default")
	secret := GenerateAgentHookSecret(agent, "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890")

	assert.Equal(t, agent.HookSecretName(), secret.Name)
	assert.Equal(t, "default", secret.Namespace)
	assert.Equal(t, mmv1beta.AgentLabels("my-agent"), secret.Labels)
	assert.Len(t, secret.OwnerReferences, 1)
	assert.Equal(t, "Agent", secret.OwnerReferences[0].Kind)
	assert.Equal(t, []byte("abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"), secret.Data["hookSecret"])

	// Must NOT have other keys
	assert.NotContains(t, secret.Data, "token")
	assert.NotContains(t, secret.Data, "apiKey")
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

	// Ingress: from MM pods on port 8080
	assert.Len(t, np.Spec.Ingress, 1)
	ingress := np.Spec.Ingress[0]
	assert.Len(t, ingress.From, 1)
	assert.Equal(t, "mm-prod", ingress.From[0].PodSelector.MatchLabels[mmv1beta.ClusterLabel])
	assert.Len(t, ingress.Ports, 1)
	assert.Equal(t, int32(8080), ingress.Ports[0].Port.IntVal)

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

func TestGenerateAgentNetworkPolicy_Allow(t *testing.T) {
	agent := testAgent("my-agent", "default")
	agent.Spec.EgressPolicy = mmv1beta.AgentEgressPolicyAllow

	np := GenerateAgentNetworkPolicy(agent)

	assert.Equal(t, "my-agent", np.Name)
	assert.Equal(t, "default", np.Namespace)
	assert.Len(t, np.OwnerReferences, 1)

	// Policy types include both Ingress and Egress.
	assert.Contains(t, np.Spec.PolicyTypes, networkingv1.PolicyTypeIngress)
	assert.Contains(t, np.Spec.PolicyTypes, networkingv1.PolicyTypeEgress)

	// Ingress: from MM pods on port 8080 (unchanged from deny mode).
	assert.Len(t, np.Spec.Ingress, 1)
	ingress := np.Spec.Ingress[0]
	assert.Len(t, ingress.From, 1)
	assert.Equal(t, "mm-prod", ingress.From[0].PodSelector.MatchLabels[mmv1beta.ClusterLabel])

	// Egress: 1 rule — empty (allow all).
	require.Len(t, np.Spec.Egress, 1)
	assert.Empty(t, np.Spec.Egress[0].To, "allow-all rule has no To selector")
	assert.Empty(t, np.Spec.Egress[0].Ports, "allow-all rule has no Ports restriction")
}

func TestGenerateAgentNetworkPolicy_AllowWithLiteLLM(t *testing.T) {
	agent := testAgent("my-agent", "default")
	agent.Spec.EgressPolicy = mmv1beta.AgentEgressPolicyAllow
	agent.Spec.LLMGateway = &mmv1beta.LLMGatewayConfig{
		OperatorManaged: &mmv1beta.OperatorManagedLLMGateway{
			Image: mmv1beta.AgentLiteLLMDefaultImage,
		},
	}

	np := GenerateAgentNetworkPolicy(agent)

	// Egress: still 1 rule — allow-all overrides LiteLLM-specific rules.
	require.Len(t, np.Spec.Egress, 1)
	assert.Empty(t, np.Spec.Egress[0].To, "allow-all rule has no To selector")
	assert.Empty(t, np.Spec.Egress[0].Ports, "allow-all rule has no Ports restriction")

	// Ingress: should have BOTH MM and LiteLLM peers (allow mode doesn't affect ingress).
	require.Len(t, np.Spec.Ingress, 1)
	assert.Len(t, np.Spec.Ingress[0].From, 2, "ingress should allow both MM and LiteLLM pods")
	assert.Equal(t, "mm-prod", np.Spec.Ingress[0].From[0].PodSelector.MatchLabels[mmv1beta.ClusterLabel])
	assert.Equal(t, "litellm", np.Spec.Ingress[0].From[1].PodSelector.MatchLabels["app"])
}

func TestImageTagNeedsAlwaysPull(t *testing.T) {
	tests := []struct {
		image    string
		expected bool
	}{
		{"myimage:dev", true},
		{"myimage:latest", true},
		{"myimage", true},                       // no tag → treat as :latest
		{"registry:5000/path/img:dev", true},     // registry with port + :dev tag
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
		// testAgent already uses :latest
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
