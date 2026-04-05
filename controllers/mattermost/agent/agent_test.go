package agent

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	"github.com/mattermost/mattermost-operator/pkg/resources"
	"github.com/sirupsen/logrus"

	blubr "github.com/mattermost/blubr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newTestAgent() *mmv1beta.Agent {
	return &mmv1beta.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
			UID:       types.UID("test-uid"),
		},
		Spec: mmv1beta.AgentSpec{
			Image:         "mattermost/agent:latest",
			Hooks:         []string{"MessageHasBeenPosted"},
			MattermostRef: corev1.LocalObjectReference{Name: "mm-test"},
			EgressPolicy:  mmv1beta.AgentEgressPolicyDeny,
		},
	}
}

func setupScheme(t *testing.T) *runtime.Scheme {
	s := scheme.Scheme
	err := mmv1beta.AddToScheme(s)
	require.NoError(t, err)
	return s
}

func setupReconciler(t *testing.T, objs ...runtime.Object) (*AgentReconciler, *runtime.Scheme) {
	s := setupScheme(t)

	clientObjs := make([]runtime.Object, len(objs))
	copy(clientObjs, objs)

	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&mmv1beta.Agent{}, &appsv1.Deployment{}, &mmv1beta.Mattermost{}).
		WithRuntimeObjects(clientObjs...).
		Build()

	logSink := blubr.InitLogger(logrus.NewEntry(logrus.New()))
	logSink = logSink.WithName("test.agent")
	logger := logr.New(logSink)

	r := &AgentReconciler{
		Client:    c,
		Log:       logger,
		Scheme:    s,
		Resources: resources.NewResourceHelper(c, s),
	}

	return r, s
}

func TestCheckAgentService(t *testing.T) {
	agent := newTestAgent()
	_ = agent.SetDefaults()

	reconciler, _ := setupReconciler(t, agent)

	logSink := blubr.InitLogger(logrus.NewEntry(logrus.New()))
	logger := logr.New(logSink.WithName("test"))

	// First call creates the service.
	err := reconciler.checkAgentService(context.TODO(), agent, logger)
	require.NoError(t, err)

	svc := &corev1.Service{}
	err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, svc)
	require.NoError(t, err)
	assert.Equal(t, "grpc", svc.Spec.Ports[0].Name)

	// Second call is idempotent.
	err = reconciler.checkAgentService(context.TODO(), agent, logger)
	require.NoError(t, err)
}

func TestCheckAgentDeployment(t *testing.T) {
	agent := newTestAgent()
	_ = agent.SetDefaults()

	// The deployment requires the bot token secret to exist for the volume.
	botSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agent.BotTokenSecretName(),
			Namespace: agent.Namespace,
		},
		Data: map[string][]byte{"token": []byte("test-token")},
	}

	reconciler, _ := setupReconciler(t, agent, botSecret)

	logSink := blubr.InitLogger(logrus.NewEntry(logrus.New()))
	logger := logr.New(logSink.WithName("test"))

	err := reconciler.checkAgentDeployment(context.TODO(), agent, logger)
	require.NoError(t, err)

	deployment := &appsv1.Deployment{}
	err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, deployment)
	require.NoError(t, err)

	// Verify env vars.
	container := deployment.Spec.Template.Spec.Containers[0]
	assert.Equal(t, mmv1beta.AgentContainerName, container.Name)

	var hasMmServerURL, hasAgentHooks bool
	for _, env := range container.Env {
		if env.Name == "MM_SERVER_URL" {
			hasMmServerURL = true
		}
		if env.Name == "AGENT_HOOKS" {
			hasAgentHooks = true
			assert.Equal(t, "MessageHasBeenPosted", env.Value)
		}
	}
	assert.True(t, hasMmServerURL, "expected MM_SERVER_URL env var")
	assert.True(t, hasAgentHooks, "expected AGENT_HOOKS env var")

	// Verify volumes.
	assert.Len(t, deployment.Spec.Template.Spec.Volumes, 2)
	assert.Equal(t, "bot-token", deployment.Spec.Template.Spec.Volumes[0].Name)
	assert.Equal(t, "mmctl-config", deployment.Spec.Template.Spec.Volumes[1].Name)

	// Verify init container.
	require.Len(t, deployment.Spec.Template.Spec.InitContainers, 1)
	assert.Equal(t, "mmctl-auth", deployment.Spec.Template.Spec.InitContainers[0].Name)
}

func TestCheckAgentNetworkPolicy_Deny(t *testing.T) {
	agent := newTestAgent()
	agent.Spec.EgressPolicy = mmv1beta.AgentEgressPolicyDeny
	_ = agent.SetDefaults()

	reconciler, _ := setupReconciler(t, agent)

	logSink := blubr.InitLogger(logrus.NewEntry(logrus.New()))
	logger := logr.New(logSink.WithName("test"))

	err := reconciler.checkAgentNetworkPolicy(context.TODO(), agent, logger)
	require.NoError(t, err)

	np := &networkingv1.NetworkPolicy{}
	err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, np)
	require.NoError(t, err)

	// Deny policy: MM egress + DNS = 2 egress rules.
	assert.Len(t, np.Spec.Egress, 2)
}

func TestCheckAgentNetworkPolicy_AllowList(t *testing.T) {
	agent := newTestAgent()
	agent.Spec.EgressPolicy = mmv1beta.AgentEgressPolicyAllowList
	_ = agent.SetDefaults()

	reconciler, _ := setupReconciler(t, agent)

	logSink := blubr.InitLogger(logrus.NewEntry(logrus.New()))
	logger := logr.New(logSink.WithName("test"))

	err := reconciler.checkAgentNetworkPolicy(context.TODO(), agent, logger)
	require.NoError(t, err)

	np := &networkingv1.NetworkPolicy{}
	err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, np)
	require.NoError(t, err)

	// AllowList policy: MM egress (8065) + DNS (53) + HTTPS (443) + HTTP (80) = 4 egress rules.
	assert.Len(t, np.Spec.Egress, 4)
}

func TestCheckAgentDeployment_WithLLMGateway(t *testing.T) {
	agent := newTestAgent()
	agent.Spec.LLMGateway = &mmv1beta.LLMGatewayConfig{
		OperatorManaged: &mmv1beta.OperatorManagedLLMGateway{
			Image: mmv1beta.AgentLiteLLMDefaultImage,
			LLMProviders: []mmv1beta.LLMProvider{
				{
					Name:   "anthropic",
					Secret: "anthropic-key",
					Models: []string{"claude-3-5-sonnet-20241022"},
				},
			},
		},
	}
	_ = agent.SetDefaults()

	botSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agent.BotTokenSecretName(),
			Namespace: agent.Namespace,
		},
		Data: map[string][]byte{"token": []byte("test-token")},
	}

	reconciler, _ := setupReconciler(t, agent, botSecret)

	logSink := blubr.InitLogger(logrus.NewEntry(logrus.New()))
	logger := logr.New(logSink.WithName("test"))

	err := reconciler.checkAgentDeployment(context.TODO(), agent, logger)
	require.NoError(t, err)

	deployment := &appsv1.Deployment{}
	err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, deployment)
	require.NoError(t, err)

	container := deployment.Spec.Template.Spec.Containers[0]

	// Build a map of env vars for easy lookup.
	envMap := make(map[string]corev1.EnvVar, len(container.Env))
	for _, e := range container.Env {
		envMap[e.Name] = e
	}

	// All 6 LiteLLM env vars must be present.
	expectedLiteLLMBaseURL := "http://litellm." + agent.Namespace + ".svc.cluster.local:4000"
	assert.Equal(t, expectedLiteLLMBaseURL, envMap["LITELLM_BASE_URL"].Value, "LITELLM_BASE_URL")
	assert.Equal(t, expectedLiteLLMBaseURL+"/mcp", envMap["LITELLM_MCP_URL"].Value, "LITELLM_MCP_URL")
	assert.Equal(t, expectedLiteLLMBaseURL+"/v1", envMap["OPENAI_BASE_URL"].Value, "OPENAI_BASE_URL")
	assert.Equal(t, expectedLiteLLMBaseURL, envMap["ANTHROPIC_BASE_URL"].Value, "ANTHROPIC_BASE_URL")

	// OPENAI_API_KEY and ANTHROPIC_API_KEY must be secretKeyRefs (not plain values).
	expectedKeySecretName := agent.LiteLLMKeySecretName()

	openAIKey, ok := envMap["OPENAI_API_KEY"]
	require.True(t, ok, "OPENAI_API_KEY must be present")
	require.NotNil(t, openAIKey.ValueFrom, "OPENAI_API_KEY must use ValueFrom")
	require.NotNil(t, openAIKey.ValueFrom.SecretKeyRef, "OPENAI_API_KEY must use SecretKeyRef")
	assert.Equal(t, expectedKeySecretName, openAIKey.ValueFrom.SecretKeyRef.Name)
	assert.Equal(t, "apiKey", openAIKey.ValueFrom.SecretKeyRef.Key)

	anthropicKey, ok := envMap["ANTHROPIC_API_KEY"]
	require.True(t, ok, "ANTHROPIC_API_KEY must be present")
	require.NotNil(t, anthropicKey.ValueFrom, "ANTHROPIC_API_KEY must use ValueFrom")
	require.NotNil(t, anthropicKey.ValueFrom.SecretKeyRef, "ANTHROPIC_API_KEY must use SecretKeyRef")
	assert.Equal(t, expectedKeySecretName, anthropicKey.ValueFrom.SecretKeyRef.Name)
	assert.Equal(t, "apiKey", anthropicKey.ValueFrom.SecretKeyRef.Key)

	// Standard env vars must still be present (backwards compat check).
	assert.Contains(t, envMap, "MM_SERVER_URL")
	assert.Contains(t, envMap, "MM_BOT_TOKEN")
	assert.Contains(t, envMap, "AGENT_HOOKS")
}

func TestCheckAgentNetworkPolicy_DenyWithLiteLLM(t *testing.T) {
	agent := newTestAgent()
	agent.Spec.EgressPolicy = mmv1beta.AgentEgressPolicyDeny
	agent.Spec.LLMGateway = &mmv1beta.LLMGatewayConfig{
		OperatorManaged: &mmv1beta.OperatorManagedLLMGateway{
			Image: mmv1beta.AgentLiteLLMDefaultImage,
		},
	}
	_ = agent.SetDefaults()

	reconciler, _ := setupReconciler(t, agent)

	logSink := blubr.InitLogger(logrus.NewEntry(logrus.New()))
	logger := logr.New(logSink.WithName("test"))

	err := reconciler.checkAgentNetworkPolicy(context.TODO(), agent, logger)
	require.NoError(t, err)

	np := &networkingv1.NetworkPolicy{}
	err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, np)
	require.NoError(t, err)

	// Deny + LiteLLM: 3 egress rules (MM + LiteLLM + DNS).
	assert.Len(t, np.Spec.Egress, 3, "deny+litellm should have 3 egress rules: MM, LiteLLM, DNS")

	// Rule 0: MM server (port 8065, has PodSelector).
	require.Len(t, np.Spec.Egress[0].Ports, 1)
	assert.Equal(t, int32(8065), np.Spec.Egress[0].Ports[0].Port.IntVal, "rule 0 should be MM port 8065")
	require.NotEmpty(t, np.Spec.Egress[0].To, "rule 0 should have a To selector (MM)")

	// Rule 1: LiteLLM (port 4000, has PodSelector with app: litellm).
	require.Len(t, np.Spec.Egress[1].Ports, 1)
	assert.Equal(t, int32(4000), np.Spec.Egress[1].Ports[0].Port.IntVal, "rule 1 should be LiteLLM port 4000")
	require.NotEmpty(t, np.Spec.Egress[1].To)
	require.NotNil(t, np.Spec.Egress[1].To[0].PodSelector)
	assert.Equal(t, "litellm", np.Spec.Egress[1].To[0].PodSelector.MatchLabels["app"])

	// Rule 2: DNS (port 53, no To selector — allows all destinations).
	require.Len(t, np.Spec.Egress[2].Ports, 2, "DNS rule should have TCP+UDP")
	assert.Empty(t, np.Spec.Egress[2].To, "DNS rule should have no destination selector")
}
