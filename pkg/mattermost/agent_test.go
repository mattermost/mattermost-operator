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

	assert.Empty(t, dep.Spec.Template.Spec.InitContainers, "init containers must be removed")

	for _, e := range c.Env {
		assert.NotEqual(t, "HOME", e.Name, "HOME env var must not be present")
	}

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

func TestGenerateAgentNetworkPolicy_Deny(t *testing.T) {
	agent := testAgent("my-agent", "default")
	agent.Spec.EgressPolicy = mmv1beta.AgentEgressPolicyDeny

	np := GenerateAgentNetworkPolicy(agent)

	assert.Equal(t, "my-agent", np.Name)
	assert.Equal(t, "default", np.Namespace)
	assert.Len(t, np.OwnerReferences, 1)

	assert.Contains(t, np.Spec.PolicyTypes, networkingv1.PolicyTypeIngress)
	assert.Contains(t, np.Spec.PolicyTypes, networkingv1.PolicyTypeEgress)

	assert.Len(t, np.Spec.Ingress, 1)
	ingress := np.Spec.Ingress[0]
	assert.Len(t, ingress.From, 1)
	assert.Equal(t, "mm-prod", ingress.From[0].PodSelector.MatchLabels[mmv1beta.ClusterLabel])
	assert.Equal(t, mmv1beta.MattermostAppContainerName, ingress.From[0].PodSelector.MatchLabels["app"])
	assert.Len(t, ingress.Ports, 1)
	assert.Equal(t, int32(8080), ingress.Ports[0].Port.IntVal)

	assert.Len(t, np.Spec.Egress, 2)

	mmEgress := np.Spec.Egress[0]
	assert.Len(t, mmEgress.To, 1)
	assert.Equal(t, "mm-prod", mmEgress.To[0].PodSelector.MatchLabels[mmv1beta.ClusterLabel])
	assert.Equal(t, mmv1beta.MattermostAppContainerName, mmEgress.To[0].PodSelector.MatchLabels["app"])
	assert.Len(t, mmEgress.Ports, 1)
	assert.Equal(t, int32(8065), mmEgress.Ports[0].Port.IntVal)

	dnsEgress := np.Spec.Egress[1]
	assert.Len(t, dnsEgress.Ports, 2)
}

func TestGenerateAgentNetworkPolicy_AllowWeb(t *testing.T) {
	agent := testAgent("my-agent", "default")
	agent.Spec.EgressPolicy = mmv1beta.AgentEgressPolicyAllowWeb

	np := GenerateAgentNetworkPolicy(agent)

	assert.Len(t, np.Spec.Egress, 3)

	mmEgress := np.Spec.Egress[0]
	assert.Len(t, mmEgress.To, 1)
	assert.Equal(t, "mm-prod", mmEgress.To[0].PodSelector.MatchLabels[mmv1beta.ClusterLabel])
	assert.Len(t, mmEgress.Ports, 1)
	assert.Equal(t, int32(8065), mmEgress.Ports[0].Port.IntVal)

	dnsEgress := np.Spec.Egress[1]
	assert.Len(t, dnsEgress.Ports, 2)
	assert.Equal(t, int32(53), dnsEgress.Ports[0].Port.IntVal)
	assert.Equal(t, int32(53), dnsEgress.Ports[1].Port.IntVal)

	webEgress := np.Spec.Egress[2]
	assert.Empty(t, webEgress.To, "no To selector means any destination")
	require.Len(t, webEgress.Ports, 2)
	assert.Equal(t, int32(443), webEgress.Ports[0].Port.IntVal)
	assert.Equal(t, corev1.ProtocolTCP, *webEgress.Ports[0].Protocol)
	assert.Equal(t, int32(80), webEgress.Ports[1].Port.IntVal)
	assert.Equal(t, corev1.ProtocolTCP, *webEgress.Ports[1].Protocol)
}

func TestGenerateAgentNetworkPolicy_Allow(t *testing.T) {
	agent := testAgent("my-agent", "default")
	agent.Spec.EgressPolicy = mmv1beta.AgentEgressPolicyAllow

	np := GenerateAgentNetworkPolicy(agent)

	assert.Equal(t, "my-agent", np.Name)
	assert.Equal(t, "default", np.Namespace)
	assert.Len(t, np.OwnerReferences, 1)

	assert.Contains(t, np.Spec.PolicyTypes, networkingv1.PolicyTypeIngress)
	assert.Contains(t, np.Spec.PolicyTypes, networkingv1.PolicyTypeEgress)

	assert.Len(t, np.Spec.Ingress, 1)
	ingress := np.Spec.Ingress[0]
	assert.Len(t, ingress.From, 1)
	assert.Equal(t, "mm-prod", ingress.From[0].PodSelector.MatchLabels[mmv1beta.ClusterLabel])
	assert.Equal(t, mmv1beta.MattermostAppContainerName, ingress.From[0].PodSelector.MatchLabels["app"])

	require.Len(t, np.Spec.Egress, 1)
	assert.Empty(t, np.Spec.Egress[0].To, "allow-all rule has no To selector")
	assert.Empty(t, np.Spec.Egress[0].Ports, "allow-all rule has no Ports restriction")
}

func TestGenerateAgentNetworkPolicy_AllowWithLiteLLM(t *testing.T) {
	agent := testAgent("my-agent", "default")
	agent.Spec.EgressPolicy = mmv1beta.AgentEgressPolicyAllow
	agent.Spec.LLMGateway = &mmv1beta.LLMGatewayConfig{
		OperatorManaged: &mmv1beta.OperatorManagedGateway{},
	}

	np := GenerateAgentNetworkPolicy(agent)

	require.Len(t, np.Spec.Egress, 1)
	assert.Empty(t, np.Spec.Egress[0].To, "allow-all rule has no To selector")
	assert.Empty(t, np.Spec.Egress[0].Ports, "allow-all rule has no Ports restriction")

	require.Len(t, np.Spec.Ingress, 1)
	assert.Len(t, np.Spec.Ingress[0].From, 2, "ingress should allow both MM and LiteLLM pods")
	assert.Equal(t, "mm-prod", np.Spec.Ingress[0].From[0].PodSelector.MatchLabels[mmv1beta.ClusterLabel])
	assert.Equal(t, mmv1beta.AgentLiteLLMDeploymentName, np.Spec.Ingress[0].From[1].PodSelector.MatchLabels["app"])
}

func TestGenerateAgentNetworkPolicy_EgressPolicy_TableDriven(t *testing.T) {
	tests := []struct {
		name             string
		egressPolicy     mmv1beta.AgentEgressPolicy
		expectedEgresses int
	}{
		{name: "empty", egressPolicy: "", expectedEgresses: 2},
		{name: "deny", egressPolicy: mmv1beta.AgentEgressPolicyDeny, expectedEgresses: 2},
		{name: "allowWeb", egressPolicy: mmv1beta.AgentEgressPolicyAllowWeb, expectedEgresses: 3},
		{name: "allow", egressPolicy: mmv1beta.AgentEgressPolicyAllow, expectedEgresses: 1},
		{name: "unknown", egressPolicy: mmv1beta.AgentEgressPolicy("unknown"), expectedEgresses: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := testAgent("my-agent", "default")
			agent.Spec.EgressPolicy = tt.egressPolicy

			np := GenerateAgentNetworkPolicy(agent)

			assert.Equal(t, []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			}, np.Spec.PolicyTypes)
			assert.Len(t, np.Spec.Egress, tt.expectedEgresses)
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
