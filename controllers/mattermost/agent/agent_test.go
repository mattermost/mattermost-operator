package agent

import (
	"context"
	"os"
	"testing"

	"github.com/go-logr/logr"
	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	"github.com/mattermost/mattermost-operator/pkg/resources"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	externalGatewayURL       = "https://gateway.example.com:8443"
	externalGatewayKeySecret = "external-gateway-key"
)

var packageTestLogger = logr.Discard()

func TestMain(m *testing.M) {
	logf.SetLogger(packageTestLogger)
	os.Exit(m.Run())
}

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
			MattermostRef: mmv1beta.AgentMattermostRef{Name: "mm-test"},
			EgressPolicy:  mmv1beta.AgentEgressPolicyDeny,
		},
	}
}

func operatorManagedGatewayConfig() *mmv1beta.LLMGatewayConfig {
	return &mmv1beta.LLMGatewayConfig{
		OperatorManaged: &mmv1beta.OperatorManagedGateway{},
	}
}

func externalGatewayConfig() *mmv1beta.LLMGatewayConfig {
	return &mmv1beta.LLMGatewayConfig{
		External: &mmv1beta.ExternalLLMGateway{
			URL:              externalGatewayURL,
			VirtualKeySecret: externalGatewayKeySecret,
		},
	}
}

func newBotTokenSecret(agent *mmv1beta.Agent) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agent.BotTokenSecretName(),
			Namespace: agent.Namespace,
		},
		Data: map[string][]byte{
			mmv1beta.SecretKeyBotToken: []byte("test-bot-token"),
		},
	}
}

func newVirtualKeySecret(agent *mmv1beta.Agent) *corev1.Secret {
	_, name, ok := agent.GatewayEndpoint()
	if !ok {
		panic("newVirtualKeySecret requires an agent with a gateway")
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: agent.Namespace},
		Data: map[string][]byte{
			mmv1beta.SecretKeyAPIKey: []byte("sk-test-virtual-key"),
		},
	}
}

func newRolloutCompleteDeployment(name, namespace string) *appsv1.Deployment {
	replicas := int32(1)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: appsv1.DeploymentSpec{Replicas: &replicas},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 1,
			ReadyReplicas:      1,
			Replicas:           1,
			UpdatedReplicas:    1,
		},
	}
}

func newReadyLiteLLMDeployment(namespace string) *appsv1.Deployment {
	return newRolloutCompleteDeployment(mmv1beta.AgentLiteLLMDeploymentName, namespace)
}

func newReadyMattermost() *mmv1beta.Mattermost {
	return &mmv1beta.Mattermost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mm-test",
			Namespace: "default",
			UID:       types.UID("mm-uid"),
		},
		Status: mmv1beta.MattermostStatus{State: mmv1beta.Stable},
	}
}

func newReadyMattermostWithGateway() *mmv1beta.Mattermost {
	mm := newReadyMattermost()
	mm.Spec.Agents = &mmv1beta.MattermostAgents{
		Enabled:    true,
		LLMGateway: &mmv1beta.AgentsLLMGateway{},
	}
	return mm
}

func setupReconciler(t *testing.T, objs ...client.Object) *AgentReconciler {
	t.Helper()

	s := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(s))
	require.NoError(t, mmv1beta.AddToScheme(s))

	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&mmv1beta.Agent{}, &appsv1.Deployment{}, &mmv1beta.Mattermost{}).
		WithIndex(&mmv1beta.Agent{}, mattermostRefNameIndex, indexAgentByMattermostRef).
		WithObjects(objs...).
		Build()

	return &AgentReconciler{
		Client:    c,
		Log:       packageTestLogger,
		Scheme:    s,
		Resources: resources.NewResourceHelper(c, s),
	}
}

func TestCheckAgentService_CreatesAndRepairsSelector(t *testing.T) {
	agent := newTestAgent()
	agent.SetDefaults()
	reconciler := setupReconciler(t, agent)

	require.NoError(t, reconciler.checkAgentService(context.Background(), agent, reconciler.Log))

	service := &corev1.Service{}
	key := types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}
	require.NoError(t, reconciler.Client.Get(context.Background(), key, service))
	require.Len(t, service.Spec.Ports, 1)
	assert.Equal(t, "http", service.Spec.Ports[0].Name)

	service.Spec.Selector["app"] = "drifted"
	require.NoError(t, reconciler.Client.Update(context.Background(), service))
	require.NoError(t, reconciler.checkAgentService(context.Background(), agent, reconciler.Log))
	require.NoError(t, reconciler.Client.Get(context.Background(), key, service))
	assert.Equal(t, mmv1beta.AgentAppName, service.Spec.Selector["app"])
}

func TestCheckAgentDeployment_CreatesAndRepairsImage(t *testing.T) {
	agent := newTestAgent()
	agent.SetDefaults()
	reconciler := setupReconciler(t, agent, newBotTokenSecret(agent))

	require.NoError(t, reconciler.checkAgentDeployment(context.Background(), agent, reconciler.Log))

	deployment := &appsv1.Deployment{}
	key := types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}
	require.NoError(t, reconciler.Client.Get(context.Background(), key, deployment))
	require.Len(t, deployment.Spec.Template.Spec.Containers, 1)
	assert.Equal(t, mmv1beta.AgentContainerName, deployment.Spec.Template.Spec.Containers[0].Name)

	deployment.Spec.Template.Spec.Containers[0].Image = "example.invalid/drifted:v1"
	require.NoError(t, reconciler.Client.Update(context.Background(), deployment))
	require.NoError(t, reconciler.checkAgentDeployment(context.Background(), agent, reconciler.Log))
	require.NoError(t, reconciler.Client.Get(context.Background(), key, deployment))
	assert.Equal(t, agent.Spec.Image, deployment.Spec.Template.Spec.Containers[0].Image)
}

func TestCheckAgentNetworkPolicy_CreatesAndIsIdempotent(t *testing.T) {
	agent := newTestAgent()
	agent.SetDefaults()
	reconciler := setupReconciler(t, agent)

	require.NoError(t, reconciler.checkAgentNetworkPolicy(context.Background(), agent, reconciler.Log))
	require.NoError(t, reconciler.checkAgentNetworkPolicy(context.Background(), agent, reconciler.Log))

	policy := &networkingv1.NetworkPolicy{}
	require.NoError(t, reconciler.Client.Get(context.Background(), client.ObjectKeyFromObject(agent), policy))
	assert.Equal(t, agent.Name, policy.Name)
}

func TestCheckHookSecret_CreatesOnce(t *testing.T) {
	agent := newTestAgent()
	agent.SetDefaults()
	reconciler := setupReconciler(t, agent)

	require.NoError(t, reconciler.checkHookSecret(context.Background(), agent, reconciler.Log))

	secret := &corev1.Secret{}
	key := types.NamespacedName{Name: agent.HookSecretName(), Namespace: agent.Namespace}
	require.NoError(t, reconciler.Client.Get(context.Background(), key, secret))
	original := secret.Data[mmv1beta.SecretKeyHookSecret]
	assert.Len(t, string(original), 64)

	require.NoError(t, reconciler.checkHookSecret(context.Background(), agent, reconciler.Log))
	require.NoError(t, reconciler.Client.Get(context.Background(), key, secret))
	assert.Equal(t, original, secret.Data[mmv1beta.SecretKeyHookSecret])
}

func TestCheckAgentHealth_PreservesObservedGeneration(t *testing.T) {
	agent := newTestAgent()
	agent.SetDefaults()
	deployment := newRolloutCompleteDeployment(agent.Name, agent.Namespace)
	reconciler := setupReconciler(t, agent, deployment)

	status, err := reconciler.checkAgentHealth(context.Background(), agent, mmv1beta.AgentStatus{
		ObservedGeneration: 7,
		Error:              "prior error",
	}, reconciler.Log)
	require.NoError(t, err)
	assert.Equal(t, mmv1beta.Stable, status.State)
	assert.Equal(t, int64(7), status.ObservedGeneration)
	assert.Equal(t, int32(1), status.ReadyReplicas)
	assert.Empty(t, status.Error)
	assert.Contains(t, status.Endpoint, agent.Name)
}

func TestCheckAgentHealth_MissingDeploymentResetsReadyReplicas(t *testing.T) {
	agent := newTestAgent()
	agent.SetDefaults()
	reconciler := setupReconciler(t, agent)

	status, err := reconciler.checkAgentHealth(
		context.Background(), agent, mmv1beta.AgentStatus{ReadyReplicas: 3}, reconciler.Log,
	)
	require.Error(t, err)
	assert.Equal(t, mmv1beta.Reconciling, status.State)
	assert.Equal(t, int32(0), status.ReadyReplicas,
		"a stale count must not survive a failed deployment lookup")
}

func TestCheckAgentHealth_MidRolloutNotStable(t *testing.T) {
	tests := []struct {
		name   string
		status appsv1.DeploymentStatus
	}{
		{
			name: "generation not observed",
			status: appsv1.DeploymentStatus{
				ObservedGeneration: 1, ReadyReplicas: 1, Replicas: 1, UpdatedReplicas: 1,
			},
		},
		{
			name: "no updated replicas",
			status: appsv1.DeploymentStatus{
				ObservedGeneration: 2, ReadyReplicas: 1, Replicas: 1,
			},
		},
		{
			name: "no ready replicas",
			status: appsv1.DeploymentStatus{
				ObservedGeneration: 2, Replicas: 1, UpdatedReplicas: 1,
			},
		},
		{
			name: "old replicas still terminating",
			status: appsv1.DeploymentStatus{
				ObservedGeneration: 2, ReadyReplicas: 1, Replicas: 2, UpdatedReplicas: 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := newTestAgent()
			agent.SetDefaults()
			deployment := newRolloutCompleteDeployment(agent.Name, agent.Namespace)
			deployment.Generation = 2
			deployment.Status = tt.status
			reconciler := setupReconciler(t, agent, deployment)

			status, err := reconciler.checkAgentHealth(
				context.Background(), agent, mmv1beta.AgentStatus{ReadyReplicas: 9}, reconciler.Log,
			)
			require.Error(t, err)
			assert.Equal(t, mmv1beta.Reconciling, status.State)
			assert.Equal(t, tt.status.ReadyReplicas, status.ReadyReplicas,
				"replica counts must be refreshed even on unhealthy paths")
		})
	}
}

func TestCheckExternallyProvisionedSecrets(t *testing.T) {
	tests := []struct {
		name        string
		gateway     *mmv1beta.LLMGatewayConfig
		objects     func(*mmv1beta.Agent) []client.Object
		errorSubstr string
	}{
		{
			name: "empty bot token",
			objects: func(agent *mmv1beta.Agent) []client.Object {
				secret := newBotTokenSecret(agent)
				secret.Data[mmv1beta.SecretKeyBotToken] = nil
				return []client.Object{secret}
			},
			errorSubstr: mmv1beta.SecretKeyBotToken,
		},
		{
			name:    "empty external virtual key",
			gateway: externalGatewayConfig(),
			objects: func(agent *mmv1beta.Agent) []client.Object {
				keySecret := newVirtualKeySecret(agent)
				keySecret.Data[mmv1beta.SecretKeyAPIKey] = nil
				return []client.Object{newBotTokenSecret(agent), keySecret}
			},
			errorSubstr: mmv1beta.SecretKeyAPIKey,
		},
		{
			name:    "valid external secrets",
			gateway: externalGatewayConfig(),
			objects: func(agent *mmv1beta.Agent) []client.Object {
				return []client.Object{newBotTokenSecret(agent), newVirtualKeySecret(agent)}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := newTestAgent()
			agent.Spec.LLMGateway = tt.gateway
			reconciler := setupReconciler(t, tt.objects(agent)...)

			err := reconciler.checkExternallyProvisionedSecrets(context.Background(), agent)
			if tt.errorSubstr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.True(t, errors.Is(err, errConfigIssue))
			assert.Contains(t, err.Error(), tt.errorSubstr)
		})
	}
}

func TestCheckAgentPVC_SkipsWithoutStorage(t *testing.T) {
	agent := newTestAgent()
	agent.SetDefaults()
	reconciler := setupReconciler(t, agent)

	require.NoError(t, reconciler.checkAgentPVC(context.Background(), agent, reconciler.Log))

	pvc := &corev1.PersistentVolumeClaim{}
	err := reconciler.Client.Get(context.Background(), types.NamespacedName{
		Name: agent.StoragePVCName(), Namespace: agent.Namespace,
	}, pvc)
	assert.True(t, k8sErrors.IsNotFound(err))
}
