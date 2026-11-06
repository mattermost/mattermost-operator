package agent

import (
	"context"
	stderrors "errors"
	"testing"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func requestFor(agent *mmv1beta.Agent) reconcile.Request {
	return reconcile.Request{NamespacedName: client.ObjectKeyFromObject(agent)}
}

func markAgentDeploymentReady(t *testing.T, reconciler *AgentReconciler, agent *mmv1beta.Agent) {
	t.Helper()

	deployment := &appsv1.Deployment{}
	require.NoError(t, reconciler.Client.Get(
		context.Background(), client.ObjectKeyFromObject(agent), deployment,
	))
	deployment.Status.ObservedGeneration = deployment.Generation
	deployment.Status.ReadyReplicas = 1
	deployment.Status.Replicas = 1
	deployment.Status.UpdatedReplicas = 1
	require.NoError(t, reconciler.Client.Status().Update(context.Background(), deployment))
}

func envValue(container corev1.Container, name string) (string, bool) {
	for _, env := range container.Env {
		if env.Name == name {
			return env.Value, true
		}
	}
	return "", false
}

type secretReadErrorClient struct {
	client.Client
	secretName string
	err        error
}

func (c *secretReadErrorClient) Get(
	ctx context.Context,
	key client.ObjectKey,
	obj client.Object,
	opts ...client.GetOption,
) error {
	if _, ok := obj.(*corev1.Secret); ok && key.Name == c.secretName {
		return c.err
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

func TestReconcileAgent_MattermostNotStable(t *testing.T) {
	agent := newTestAgent()
	agent.Generation = 3
	agent.Status = mmv1beta.AgentStatus{
		State:              mmv1beta.Stable,
		Error:              "stale error from a previous reconcile",
		ObservedGeneration: 1,
	}
	mm := newReadyMattermost()
	mm.Status.State = mmv1beta.Reconciling
	reconciler := setupReconciler(t, agent, mm)

	result, err := reconciler.Reconcile(context.Background(), requestFor(agent))
	require.NoError(t, err)
	assert.Equal(t, dependencyRequeueDelay, result.RequeueAfter)

	deployment := &appsv1.Deployment{}
	err = reconciler.Client.Get(context.Background(), client.ObjectKeyFromObject(agent), deployment)
	assert.True(t, k8sErrors.IsNotFound(err))

	// The wait must not leave a stale status behind: state drops to
	// Reconciling, the old error is cleared and the generation advances.
	persisted := &mmv1beta.Agent{}
	require.NoError(t, reconciler.Client.Get(context.Background(), client.ObjectKeyFromObject(agent), persisted))
	assert.Equal(t, mmv1beta.Reconciling, persisted.Status.State)
	assert.Empty(t, persisted.Status.Error)
	assert.Equal(t, agent.Generation, persisted.Status.ObservedGeneration)
}

func TestReconcileAgent_FullLifecycle(t *testing.T) {
	agent := newTestAgent()
	reconciler := setupReconciler(
		t,
		agent,
		newReadyMattermost(),
		newBotTokenSecret(agent),
	)

	result, err := reconciler.Reconcile(context.Background(), requestFor(agent))
	require.NoError(t, err)
	assert.Equal(t, healthCheckRequeueDelay, result.RequeueAfter)

	key := client.ObjectKeyFromObject(agent)
	require.NoError(t, reconciler.Client.Get(context.Background(), key, &corev1.ServiceAccount{}))
	require.NoError(t, reconciler.Client.Get(context.Background(), key, &corev1.Service{}))
	require.NoError(t, reconciler.Client.Get(context.Background(), key, &appsv1.Deployment{}))
	require.NoError(t, reconciler.Client.Get(context.Background(), key, &networkingv1.NetworkPolicy{}))
	require.NoError(t, reconciler.Client.Get(context.Background(), types.NamespacedName{
		Name: agent.HookSecretName(), Namespace: agent.Namespace,
	}, &corev1.Secret{}))

	persisted := &mmv1beta.Agent{}
	require.NoError(t, reconciler.Client.Get(context.Background(), key, persisted))
	assert.NotNil(t, persisted.Spec.Resources.Requests)

	markAgentDeploymentReady(t, reconciler, agent)
	result, err = reconciler.Reconcile(context.Background(), requestFor(agent))
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result)

	require.NoError(t, reconciler.Client.Get(context.Background(), key, persisted))
	assert.Equal(t, mmv1beta.Stable, persisted.Status.State)
	assert.Equal(t, persisted.Generation, persisted.Status.ObservedGeneration)
	assert.Equal(t, int32(1), persisted.Status.ReadyReplicas)
	assert.Contains(t, persisted.Status.Endpoint, agent.Name)
}

func TestReconcileAgent_RepairsOwnedResourceDrift(t *testing.T) {
	agent := newTestAgent()
	reconciler := setupReconciler(
		t,
		agent,
		newReadyMattermost(),
		newBotTokenSecret(agent),
	)
	require.NoError(t, reconcileOnce(reconciler, agent))

	key := client.ObjectKeyFromObject(agent)
	deployment := &appsv1.Deployment{}
	require.NoError(t, reconciler.Client.Get(context.Background(), key, deployment))
	deployment.Spec.Template.Spec.Containers[0].Image = "example.invalid/drifted:v1"
	require.NoError(t, reconciler.Client.Update(context.Background(), deployment))

	service := &corev1.Service{}
	require.NoError(t, reconciler.Client.Get(context.Background(), key, service))
	service.Spec.Selector["app"] = "drifted"
	require.NoError(t, reconciler.Client.Update(context.Background(), service))

	require.NoError(t, reconcileOnce(reconciler, agent))

	require.NoError(t, reconciler.Client.Get(context.Background(), key, deployment))
	assert.Equal(t, agent.Spec.Image, deployment.Spec.Template.Spec.Containers[0].Image)
	require.NoError(t, reconciler.Client.Get(context.Background(), key, service))
	assert.Equal(t, mmv1beta.AgentAppName, service.Spec.Selector["app"])
}

func reconcileOnce(reconciler *AgentReconciler, agent *mmv1beta.Agent) error {
	_, err := reconciler.Reconcile(context.Background(), requestFor(agent))
	return err
}

func TestReconcileAgent_PVCRetainsOriginalSizeAndStorageClass(t *testing.T) {
	agent := newTestAgent()
	originalClass := "fast"
	agent.Spec.Storage = &mmv1beta.AgentStorageConfig{
		Size:             resource.MustParse("1Gi"),
		StorageClassName: &originalClass,
		MountPath:        "/data",
	}
	reconciler := setupReconciler(
		t,
		agent,
		newReadyMattermost(),
		newBotTokenSecret(agent),
	)

	require.NoError(t, reconcileOnce(reconciler, agent))

	persisted := &mmv1beta.Agent{}
	require.NoError(t, reconciler.Client.Get(
		context.Background(), client.ObjectKeyFromObject(agent), persisted,
	))
	changedClass := "slow"
	persisted.Spec.Storage.Size = resource.MustParse("2Gi")
	persisted.Spec.Storage.StorageClassName = &changedClass
	require.NoError(t, reconciler.Client.Update(context.Background(), persisted))
	require.NoError(t, reconcileOnce(reconciler, agent))

	pvc := &corev1.PersistentVolumeClaim{}
	require.NoError(t, reconciler.Client.Get(context.Background(), types.NamespacedName{
		Name: agent.StoragePVCName(), Namespace: agent.Namespace,
	}, pvc))
	assert.Equal(t, resource.MustParse("1Gi"), pvc.Spec.Resources.Requests[corev1.ResourceStorage])
	require.NotNil(t, pvc.Spec.StorageClassName)
	assert.Equal(t, originalClass, *pvc.Spec.StorageClassName)
}

func TestReconcileAgent_ExternalGatewayReachesStable(t *testing.T) {
	agent := newTestAgent()
	agent.Spec.LLMGateway = externalGatewayConfig()
	reconciler := setupReconciler(
		t,
		agent,
		newReadyMattermost(),
		newBotTokenSecret(agent),
		newVirtualKeySecret(agent),
	)

	result, err := reconciler.Reconcile(context.Background(), requestFor(agent))
	require.NoError(t, err)
	assert.Equal(t, healthCheckRequeueDelay, result.RequeueAfter)

	deployment := &appsv1.Deployment{}
	require.NoError(t, reconciler.Client.Get(
		context.Background(), client.ObjectKeyFromObject(agent), deployment,
	))
	require.Len(t, deployment.Spec.Template.Spec.Containers, 1)
	baseURL, found := envValue(deployment.Spec.Template.Spec.Containers[0], "LITELLM_BASE_URL")
	require.True(t, found)
	assert.Equal(t, externalGatewayURL, baseURL)

	markAgentDeploymentReady(t, reconciler, agent)
	result, err = reconciler.Reconcile(context.Background(), requestFor(agent))
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result)

	persisted := &mmv1beta.Agent{}
	require.NoError(t, reconciler.Client.Get(
		context.Background(), client.ObjectKeyFromObject(agent), persisted,
	))
	assert.Equal(t, mmv1beta.Stable, persisted.Status.State)
}

func TestReconcileAgent_OperatorManagedGatewayUnreadyThenReady(t *testing.T) {
	agent := newTestAgent()
	agent.Spec.LLMGateway = operatorManagedGatewayConfig()
	gatewayDeployment := newReadyLiteLLMDeployment(agent.Namespace)
	gatewayDeployment.Status.ReadyReplicas = 0
	reconciler := setupReconciler(
		t,
		agent,
		newReadyMattermostWithGateway(),
		newBotTokenSecret(agent),
		newVirtualKeySecret(agent),
		gatewayDeployment,
	)

	result, err := reconciler.Reconcile(context.Background(), requestFor(agent))
	require.NoError(t, err)
	assert.Equal(t, dependencyRequeueDelay, result.RequeueAfter)

	agentDeployment := &appsv1.Deployment{}
	err = reconciler.Client.Get(
		context.Background(), client.ObjectKeyFromObject(agent), agentDeployment,
	)
	assert.True(t, k8sErrors.IsNotFound(err))

	// Waiting on the gateway persists a fresh Reconciling status.
	persisted := &mmv1beta.Agent{}
	require.NoError(t, reconciler.Client.Get(context.Background(), client.ObjectKeyFromObject(agent), persisted))
	assert.Equal(t, mmv1beta.Reconciling, persisted.Status.State)
	assert.Empty(t, persisted.Status.Error)
	assert.Equal(t, persisted.Generation, persisted.Status.ObservedGeneration)

	gatewayDeployment = &appsv1.Deployment{}
	require.NoError(t, reconciler.Client.Get(context.Background(), types.NamespacedName{
		Name: mmv1beta.AgentLiteLLMDeploymentName, Namespace: agent.Namespace,
	}, gatewayDeployment))
	gatewayDeployment.Status.ObservedGeneration = gatewayDeployment.Generation
	gatewayDeployment.Status.ReadyReplicas = 1
	gatewayDeployment.Status.Replicas = 1
	gatewayDeployment.Status.UpdatedReplicas = 1
	require.NoError(t, reconciler.Client.Status().Update(context.Background(), gatewayDeployment))

	result, err = reconciler.Reconcile(context.Background(), requestFor(agent))
	require.NoError(t, err)
	assert.Equal(t, healthCheckRequeueDelay, result.RequeueAfter)
	require.NoError(t, reconciler.Client.Get(
		context.Background(), client.ObjectKeyFromObject(agent), agentDeployment,
	))
}

func TestReconcileAgent_OperatorManagedWithoutInstallationGateway(t *testing.T) {
	agent := newTestAgent()
	agent.Spec.LLMGateway = operatorManagedGatewayConfig()
	reconciler := setupReconciler(t, agent, newReadyMattermost())

	result, err := reconciler.Reconcile(context.Background(), requestFor(agent))
	require.NoError(t, err)
	assert.Equal(t, configIssueRequeueDelay, result.RequeueAfter)

	persisted := &mmv1beta.Agent{}
	require.NoError(t, reconciler.Client.Get(
		context.Background(), client.ObjectKeyFromObject(agent), persisted,
	))
	assert.Equal(t, mmv1beta.Reconciling, persisted.Status.State)
	assert.Contains(t, persisted.Status.Error, "spec.agents.llmGateway")
}

func TestReconcileAgent_RequiredSecretConfigIssues(t *testing.T) {
	tests := []struct {
		name             string
		gateway          *mmv1beta.LLMGatewayConfig
		installation     func() *mmv1beta.Mattermost
		precreated       func(*mmv1beta.Agent) []client.Object
		expectedErrorSub string
	}{
		{
			name:             "bot token secret missing",
			installation:     newReadyMattermost,
			precreated:       func(*mmv1beta.Agent) []client.Object { return nil },
			expectedErrorSub: "provisioned externally",
		},
		{
			name:         "bot token key missing",
			installation: newReadyMattermost,
			precreated: func(agent *mmv1beta.Agent) []client.Object {
				secret := newBotTokenSecret(agent)
				secret.Data = map[string][]byte{"unexpected": []byte("value")}
				return []client.Object{secret}
			},
			expectedErrorSub: mmv1beta.SecretKeyBotToken,
		},
		{
			name:         "operator-managed virtual key secret missing",
			gateway:      operatorManagedGatewayConfig(),
			installation: newReadyMattermostWithGateway,
			precreated: func(agent *mmv1beta.Agent) []client.Object {
				return []client.Object{
					newBotTokenSecret(agent),
					newReadyLiteLLMDeployment(agent.Namespace),
				}
			},
			expectedErrorSub: "litellm-key",
		},
		{
			name:         "external virtual key secret missing",
			gateway:      externalGatewayConfig(),
			installation: newReadyMattermost,
			precreated: func(agent *mmv1beta.Agent) []client.Object {
				return []client.Object{newBotTokenSecret(agent)}
			},
			expectedErrorSub: externalGatewayKeySecret,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := newTestAgent()
			agent.Spec.LLMGateway = tt.gateway
			objects := []client.Object{agent, tt.installation()}
			objects = append(objects, tt.precreated(agent)...)
			reconciler := setupReconciler(t, objects...)

			result, err := reconciler.Reconcile(context.Background(), requestFor(agent))
			require.NoError(t, err)
			assert.Equal(t, configIssueRequeueDelay, result.RequeueAfter)

			deployment := &appsv1.Deployment{}
			err = reconciler.Client.Get(
				context.Background(), client.ObjectKeyFromObject(agent), deployment,
			)
			assert.True(t, k8sErrors.IsNotFound(err))

			persisted := &mmv1beta.Agent{}
			require.NoError(t, reconciler.Client.Get(
				context.Background(), client.ObjectKeyFromObject(agent), persisted,
			))
			assert.Equal(t, mmv1beta.Reconciling, persisted.Status.State)
			assert.Contains(t, persisted.Status.Error, tt.expectedErrorSub)
		})
	}
}

func TestReconcileAgent_TransientSecretReadErrorPropagates(t *testing.T) {
	agent := newTestAgent()
	reconciler := setupReconciler(t, agent, newReadyMattermost())
	transientErr := k8sErrors.NewInternalError(stderrors.New("etcd timeout"))
	reconciler.Client = &secretReadErrorClient{
		Client:     reconciler.Client,
		secretName: agent.BotTokenSecretName(),
		err:        transientErr,
	}

	_, err := reconciler.Reconcile(context.Background(), requestFor(agent))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "etcd timeout")
}

func TestAgentsForMattermost(t *testing.T) {
	agentA := newTestAgent()
	agentA.Name = "agent-a"
	agentB := newTestAgent()
	agentB.Name = "agent-b"
	otherAgent := newTestAgent()
	otherAgent.Name = "agent-other"
	otherAgent.Spec.MattermostRef.Name = "mm-other"
	reconciler := setupReconciler(t, agentA, agentB, otherAgent)

	mm := newReadyMattermost()
	requests := reconciler.agentsForMattermost(context.Background(), mm)
	require.Len(t, requests, 2)
	assert.ElementsMatch(t, []string{"agent-a", "agent-b"}, []string{
		requests[0].Name,
		requests[1].Name,
	})

	unreferenced := newReadyMattermost()
	unreferenced.Name = "mm-unreferenced"
	assert.Empty(t, reconciler.agentsForMattermost(context.Background(), unreferenced))
}

func TestAgentsForLiteLLMDeployment(t *testing.T) {
	gatewayAgent := newTestAgent()
	gatewayAgent.Name = "agent-gateway"
	gatewayAgent.Spec.LLMGateway = operatorManagedGatewayConfig()
	plainAgent := newTestAgent()
	plainAgent.Name = "agent-plain"
	reconciler := setupReconciler(t, gatewayAgent, plainAgent)

	gatewayDeployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name: mmv1beta.AgentLiteLLMDeploymentName, Namespace: gatewayAgent.Namespace,
	}}
	requests := reconciler.agentsForLiteLLMDeployment(context.Background(), gatewayDeployment)
	require.Len(t, requests, 1)
	assert.Equal(t, gatewayAgent.Name, requests[0].Name)

	unrelated := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name: "unrelated", Namespace: gatewayAgent.Namespace,
	}}
	assert.Empty(t, reconciler.agentsForLiteLLMDeployment(context.Background(), unrelated))
}

func TestAgentsForSecret(t *testing.T) {
	plainAgent := newTestAgent()
	plainAgent.Name = "agent-plain"

	managedAgent := newTestAgent()
	managedAgent.Name = "agent-managed"
	managedAgent.Spec.LLMGateway = operatorManagedGatewayConfig()

	externalAgent := newTestAgent()
	externalAgent.Name = "agent-external"
	externalAgent.Spec.LLMGateway = externalGatewayConfig()

	otherNamespaceAgent := newTestAgent()
	otherNamespaceAgent.Name = "agent-elsewhere"
	otherNamespaceAgent.Namespace = "other"

	reconciler := setupReconciler(t, plainAgent, managedAgent, externalAgent, otherNamespaceAgent)

	secretInDefault := func(name string) *corev1.Secret {
		return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"}}
	}
	requestNames := func(requests []reconcile.Request) []string {
		names := make([]string, 0, len(requests))
		for _, request := range requests {
			names = append(names, request.Name)
		}
		return names
	}

	t.Run("bot token secret maps to its agent", func(t *testing.T) {
		requests := reconciler.agentsForSecret(context.Background(), secretInDefault(plainAgent.BotTokenSecretName()))
		assert.ElementsMatch(t, []string{plainAgent.Name}, requestNames(requests))
	})

	t.Run("operator-managed virtual key secret maps to its agent", func(t *testing.T) {
		requests := reconciler.agentsForSecret(context.Background(), secretInDefault(managedAgent.LiteLLMKeySecretName()))
		assert.ElementsMatch(t, []string{managedAgent.Name}, requestNames(requests))
	})

	t.Run("external virtual key secret maps to its agent", func(t *testing.T) {
		requests := reconciler.agentsForSecret(context.Background(), secretInDefault(externalGatewayKeySecret))
		assert.ElementsMatch(t, []string{externalAgent.Name}, requestNames(requests))
	})

	t.Run("unrelated secret maps to nothing", func(t *testing.T) {
		assert.Empty(t, reconciler.agentsForSecret(context.Background(), secretInDefault("unrelated-secret")))
	})

	t.Run("secret in another namespace does not map across namespaces", func(t *testing.T) {
		secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
			Name: otherNamespaceAgent.BotTokenSecretName(), Namespace: "other",
		}}
		requests := reconciler.agentsForSecret(context.Background(), secret)
		assert.ElementsMatch(t, []string{otherNamespaceAgent.Name}, requestNames(requests))
	})
}
