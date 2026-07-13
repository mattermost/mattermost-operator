package agent

import (
	"context"
	"testing"
	"time"

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
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconcileAgent_MattermostNotStable(t *testing.T) {
	logSink := blubr.InitLogger(logrus.NewEntry(logrus.New()))
	logSink = logSink.WithName("test.opr")
	logger := logr.New(logSink)
	logf.SetLogger(logger)

	agent := newTestAgent()

	mm := &mmv1beta.Mattermost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mm-test",
			Namespace: "default",
			UID:       types.UID("mm-uid"),
		},
		Spec: mmv1beta.MattermostSpec{
			Image:   "mattermost/mattermost-enterprise-edition",
			Version: "9.0.0",
		},
		Status: mmv1beta.MattermostStatus{
			State: mmv1beta.Reconciling,
		},
	}

	s := setupScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&mmv1beta.Agent{}, &appsv1.Deployment{}, &mmv1beta.Mattermost{}).
		WithRuntimeObjects(agent, mm).
		Build()

	r := &AgentReconciler{
		Client:    c,
		Log:       logger,
		Scheme:    s,
		Resources: resources.NewResourceHelper(c, s),
	}

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}}
	res, err := r.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, 15*time.Second, res.RequeueAfter)

	// No resources should have been created.
	svc := &corev1.Service{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, svc)
	require.Error(t, err, "service should not exist")

	deploy := &appsv1.Deployment{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, deploy)
	require.Error(t, err, "deployment should not exist")
}

func TestReconcileAgent_FullReconcile(t *testing.T) {
	logSink := blubr.InitLogger(logrus.NewEntry(logrus.New()))
	logSink = logSink.WithName("test.opr")
	logger := logr.New(logSink)
	logf.SetLogger(logger)

	agent := newTestAgent()

	mm := &mmv1beta.Mattermost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mm-test",
			Namespace: "default",
			UID:       types.UID("mm-uid"),
		},
		Spec: mmv1beta.MattermostSpec{
			Image:   "mattermost/mattermost-enterprise-edition",
			Version: "9.0.0",
		},
		Status: mmv1beta.MattermostStatus{
			State: mmv1beta.Stable,
		},
	}

	s := setupScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&mmv1beta.Agent{}, &appsv1.Deployment{}, &mmv1beta.Mattermost{}).
		WithRuntimeObjects(agent, mm).
		Build()

	r := &AgentReconciler{
		Client:    c,
		Log:       logger,
		Scheme:    s,
		Resources: resources.NewResourceHelper(c, s),
	}

	// Pre-create the bot token secret (the plugin creates this before the Agent CR).
	botTokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agent.BotTokenSecretName(),
			Namespace: agent.Namespace,
		},
		Data: map[string][]byte{"token": []byte("bot-secret-token")},
	}
	err := c.Create(context.TODO(), botTokenSecret)
	require.NoError(t, err)

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}}

	// First reconcile: deployment not ready yet, should requeue.
	res, err := r.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, 6*time.Second, res.RequeueAfter)

	// Verify all resources were created.
	sa := &corev1.ServiceAccount{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, sa)
	require.NoError(t, err)
	assert.Equal(t, mmv1beta.AgentLabels(agent.Name), sa.Labels)
	require.Len(t, sa.OwnerReferences, 1)
	assert.Equal(t, agent.Name, sa.OwnerReferences[0].Name)
	assert.Equal(t, "Agent", sa.OwnerReferences[0].Kind)

	svc := &corev1.Service{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, svc)
	require.NoError(t, err)

	deploy := &appsv1.Deployment{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, deploy)
	require.NoError(t, err)

	np := &networkingv1.NetworkPolicy{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, np)
	require.NoError(t, err)

	// Simulate deployment becoming ready.
	deploy.Status.ReadyReplicas = 1
	deploy.Status.Replicas = 1
	err = c.Status().Update(context.TODO(), deploy)
	require.NoError(t, err)

	// Second reconcile: should reach Stable.
	res, err = r.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)

	// Verify agent status is Stable.
	updatedAgent := &mmv1beta.Agent{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, updatedAgent)
	require.NoError(t, err)
	assert.Equal(t, mmv1beta.Stable, updatedAgent.Status.State)
	assert.Contains(t, updatedAgent.Status.Endpoint, agent.Name)
	assert.Contains(t, updatedAgent.Status.Endpoint, ":8080")
	assert.Equal(t, int32(1), updatedAgent.Status.ReadyReplicas)

	// Verify hook secret was created.
	hookSecret := &corev1.Secret{}
	err = c.Get(context.TODO(), types.NamespacedName{
		Name:      agent.HookSecretName(),
		Namespace: agent.Namespace,
	}, hookSecret)
	require.NoError(t, err, "hook secret should be created during reconcile")
	assert.Contains(t, hookSecret.Data, "hookSecret")
	assert.Len(t, string(hookSecret.Data["hookSecret"]), 64, "hook secret should be 64-char hex")
}

func TestReconcileAgent_ImageUpdate(t *testing.T) {
	logSink := blubr.InitLogger(logrus.NewEntry(logrus.New()))
	logSink = logSink.WithName("test.opr")
	logger := logr.New(logSink)
	logf.SetLogger(logger)

	agent := newTestAgent()
	_ = agent.SetDefaults()

	mm := &mmv1beta.Mattermost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mm-test",
			Namespace: "default",
			UID:       types.UID("mm-uid"),
		},
		Status: mmv1beta.MattermostStatus{
			State: mmv1beta.Stable,
		},
	}

	botTokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agent.BotTokenSecretName(),
			Namespace: agent.Namespace,
		},
		Data: map[string][]byte{"token": []byte("bot-token")},
	}

	s := setupScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&mmv1beta.Agent{}, &appsv1.Deployment{}, &mmv1beta.Mattermost{}).
		WithRuntimeObjects(agent, mm, botTokenSecret).
		Build()

	r := &AgentReconciler{
		Client:    c,
		Log:       logger,
		Scheme:    s,
		Resources: resources.NewResourceHelper(c, s),
	}

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}}

	// First reconcile to create all resources.
	_, err := r.Reconcile(context.Background(), req)
	require.NoError(t, err)

	// Verify initial image.
	deploy := &appsv1.Deployment{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, deploy)
	require.NoError(t, err)
	assert.Equal(t, "mattermost/agent:latest", deploy.Spec.Template.Spec.Containers[0].Image)

	// Update the agent image.
	updatedAgent := &mmv1beta.Agent{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, updatedAgent)
	require.NoError(t, err)
	updatedAgent.Spec.Image = "mattermost/agent:v2"
	err = c.Update(context.TODO(), updatedAgent)
	require.NoError(t, err)

	// Reconcile again.
	_, err = r.Reconcile(context.Background(), req)
	require.NoError(t, err)

	// Verify deployment was updated.
	err = c.Get(context.TODO(), types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, deploy)
	require.NoError(t, err)
	assert.Equal(t, "mattermost/agent:v2", deploy.Spec.Template.Spec.Containers[0].Image)
}

func TestReconcileAgent_WithLLMGateway(t *testing.T) {
	logSink := blubr.InitLogger(logrus.NewEntry(logrus.New()))
	logSink = logSink.WithName("test.opr")
	logger := logr.New(logSink)
	logf.SetLogger(logger)

	agent := newTestAgent()
	agent.Spec.LLMGateway = &mmv1beta.LLMGatewayConfig{
		OperatorManaged: &mmv1beta.OperatorManagedGateway{},
	}
	_ = agent.SetDefaults()

	mm := newReadyMattermostWithGateway()

	botTokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agent.BotTokenSecretName(),
			Namespace: agent.Namespace,
		},
		Data: map[string][]byte{"token": []byte("bot-secret-token")},
	}

	litellmKeySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agent.LiteLLMKeySecretName(),
			Namespace: agent.Namespace,
		},
		Data: map[string][]byte{"key": []byte("sk-test-virtual-key")},
	}

	s := setupScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&mmv1beta.Agent{}, &appsv1.Deployment{}, &mmv1beta.Mattermost{}).
		WithRuntimeObjects(agent, mm, botTokenSecret, litellmKeySecret).
		Build()

	r := &AgentReconciler{
		Client:    c,
		Log:       logger,
		Scheme:    s,
		Resources: resources.NewResourceHelper(c, s),
	}

	// Pre-create a ready LiteLLM Deployment (managed by the Mattermost
	// controller) so checkLiteLLMReady passes.
	litellmDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmv1beta.AgentLiteLLMDeploymentName,
			Namespace: agent.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "mm-agent-litellm"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "mm-agent-litellm"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "litellm", Image: mmv1beta.AgentLiteLLMDefaultImage}}},
			},
		},
		Status: appsv1.DeploymentStatus{ReadyReplicas: 1, Replicas: 1},
	}
	err := c.Create(context.TODO(), litellmDeploy)
	require.NoError(t, err)
	litellmDeploy.Status.ReadyReplicas = 1
	err = c.Status().Update(context.TODO(), litellmDeploy)
	require.NoError(t, err)

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}}

	// First reconcile: agent Deployment not ready yet → requeue with healthCheckRequeueDelay.
	res, err := r.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, 6*time.Second, res.RequeueAfter)

	// Verify agent is still reconciling (not ready yet).
	agentAfterFirstReconcile := &mmv1beta.Agent{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, agentAfterFirstReconcile)
	require.NoError(t, err)
	assert.Equal(t, mmv1beta.Reconciling, agentAfterFirstReconcile.Status.State)

	// Verify agent Deployment was created with LiteLLM env vars.
	agentDeploy := &appsv1.Deployment{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, agentDeploy)
	require.NoError(t, err)

	container := agentDeploy.Spec.Template.Spec.Containers[0]
	envMap := make(map[string]corev1.EnvVar, len(container.Env))
	for _, e := range container.Env {
		envMap[e.Name] = e
	}
	assert.Contains(t, envMap, "LITELLM_BASE_URL", "agent Deployment must have LITELLM_BASE_URL")
	assert.Contains(t, envMap, "OPENAI_API_KEY", "agent Deployment must have OPENAI_API_KEY")
	assert.Contains(t, envMap, "ANTHROPIC_API_KEY", "agent Deployment must have ANTHROPIC_API_KEY")

	// Raw API keys must NOT be plain values — they must be secretKeyRefs.
	require.NotNil(t, envMap["ANTHROPIC_API_KEY"].ValueFrom, "ANTHROPIC_API_KEY must use ValueFrom")
	assert.Equal(t, agent.LiteLLMKeySecretName(), envMap["ANTHROPIC_API_KEY"].ValueFrom.SecretKeyRef.Name)

	// Verify NetworkPolicy has 3 egress rules (MM + LiteLLM + DNS).
	np := &networkingv1.NetworkPolicy{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, np)
	require.NoError(t, err)
	assert.Len(t, np.Spec.Egress, 3, "deny+litellm policy must have 3 egress rules")

	// Verify hook secret was created (created before Deployment, no LLM dependency).
	hookSecret := &corev1.Secret{}
	err = c.Get(context.TODO(), types.NamespacedName{
		Name:      agent.HookSecretName(),
		Namespace: agent.Namespace,
	}, hookSecret)
	require.NoError(t, err, "hook secret should be created during reconcile")
	assert.Contains(t, hookSecret.Data, "hookSecret")
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

// newReadyMattermostWithGateway returns a stable Mattermost with the
// operator-managed LiteLLM gateway configured.
func newReadyMattermostWithGateway() *mmv1beta.Mattermost {
	mm := newReadyMattermost()
	mm.Spec.Agents = &mmv1beta.MattermostAgents{
		Enabled:    true,
		LLMGateway: &mmv1beta.AgentsLLMGateway{},
	}
	return mm
}

func TestReconcileAgent_OperatorManagedGatesOnLiteLLMReadiness(t *testing.T) {
	logger := testLogger()
	logf.SetLogger(logger)

	agent := newTestAgent()
	agent.Spec.LLMGateway = &mmv1beta.LLMGatewayConfig{
		OperatorManaged: &mmv1beta.OperatorManagedGateway{},
	}
	_ = agent.SetDefaults()

	// LiteLLM Deployment exists but has no ready replicas.
	litellmDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmv1beta.AgentLiteLLMDeploymentName,
			Namespace: agent.Namespace,
		},
		Status: appsv1.DeploymentStatus{ReadyReplicas: 0},
	}

	s := setupScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&mmv1beta.Agent{}, &appsv1.Deployment{}, &mmv1beta.Mattermost{}).
		WithRuntimeObjects(agent, newReadyMattermostWithGateway(), litellmDeploy).
		Build()

	r := &AgentReconciler{Client: c, Log: logger, Scheme: s, Resources: resources.NewResourceHelper(c, s)}

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}}
	res, err := r.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, 15*time.Second, res.RequeueAfter)

	// No agent resources until the gateway is ready.
	deploy := &appsv1.Deployment{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, deploy)
	require.True(t, k8sErrors.IsNotFound(err), "agent Deployment must not be created before LiteLLM is ready")
}

func TestReconcileAgent_OperatorManagedWithoutInstallationGateway(t *testing.T) {
	logger := testLogger()
	logf.SetLogger(logger)

	agent := newTestAgent()
	agent.Spec.LLMGateway = &mmv1beta.LLMGatewayConfig{
		OperatorManaged: &mmv1beta.OperatorManagedGateway{},
	}
	_ = agent.SetDefaults()

	// Mattermost is stable but does not configure spec.agents.llmGateway.
	mm := newReadyMattermost()

	s := setupScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&mmv1beta.Agent{}, &appsv1.Deployment{}, &mmv1beta.Mattermost{}).
		WithRuntimeObjects(agent, mm).
		Build()

	r := &AgentReconciler{Client: c, Log: logger, Scheme: s, Resources: resources.NewResourceHelper(c, s)}

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}}
	res, err := r.Reconcile(context.Background(), req)
	require.NoError(t, err, "config issue must not error-loop")
	assert.Equal(t, 60*time.Second, res.RequeueAfter)

	updated := &mmv1beta.Agent{}
	require.NoError(t, c.Get(context.TODO(), req.NamespacedName, updated))
	assert.Equal(t, mmv1beta.Reconciling, updated.Status.State)
	assert.Contains(t, updated.Status.Error, "spec.agents.llmGateway")
}

func TestReconcileAgent_MissingBotTokenSecret(t *testing.T) {
	logger := testLogger()
	logf.SetLogger(logger)

	agent := newTestAgent()
	mm := newReadyMattermost()

	s := setupScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&mmv1beta.Agent{}, &appsv1.Deployment{}, &mmv1beta.Mattermost{}).
		WithRuntimeObjects(agent, mm).
		Build()

	r := &AgentReconciler{
		Client:    c,
		Log:       logger,
		Scheme:    s,
		Resources: resources.NewResourceHelper(c, s),
	}

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}}
	res, err := r.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, 60*time.Second, res.RequeueAfter)

	deploy := &appsv1.Deployment{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, deploy)
	require.True(t, k8sErrors.IsNotFound(err), "agent Deployment must not be created without bot token secret")

	updated := &mmv1beta.Agent{}
	require.NoError(t, c.Get(context.TODO(), req.NamespacedName, updated))
	assert.Equal(t, mmv1beta.Reconciling, updated.Status.State)
	assert.Contains(t, updated.Status.Error, agent.BotTokenSecretName())
	assert.Contains(t, updated.Status.Error, "provisioned externally")
}

func TestReconcileAgent_MissingLiteLLMKeySecret_OperatorManaged(t *testing.T) {
	logger := testLogger()
	logf.SetLogger(logger)

	agent := newTestAgent()
	agent.Spec.LLMGateway = &mmv1beta.LLMGatewayConfig{
		OperatorManaged: &mmv1beta.OperatorManagedGateway{},
	}
	_ = agent.SetDefaults()

	botTokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agent.BotTokenSecretName(),
			Namespace: agent.Namespace,
		},
		Data: map[string][]byte{"token": []byte("bot-secret-token")},
	}

	// Pre-create a ready LiteLLM Deployment (managed by the Mattermost controller).
	litellmDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmv1beta.AgentLiteLLMDeploymentName,
			Namespace: agent.Namespace,
		},
		Status: appsv1.DeploymentStatus{ReadyReplicas: 1, Replicas: 1},
	}

	s := setupScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&mmv1beta.Agent{}, &appsv1.Deployment{}, &mmv1beta.Mattermost{}).
		WithRuntimeObjects(agent, newReadyMattermostWithGateway(), botTokenSecret, litellmDeploy).
		Build()

	r := &AgentReconciler{
		Client:    c,
		Log:       logger,
		Scheme:    s,
		Resources: resources.NewResourceHelper(c, s),
	}

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}}

	// LiteLLM ready, but virtual-key secret missing.
	res, err := r.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, 60*time.Second, res.RequeueAfter)

	deploy := &appsv1.Deployment{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, deploy)
	require.True(t, k8sErrors.IsNotFound(err), "agent Deployment must not be created without LiteLLM key secret")

	updated := &mmv1beta.Agent{}
	require.NoError(t, c.Get(context.TODO(), req.NamespacedName, updated))
	assert.Equal(t, mmv1beta.Reconciling, updated.Status.State)
	assert.Contains(t, updated.Status.Error, agent.LiteLLMKeySecretName())
	assert.Contains(t, updated.Status.Error, "provisioned externally")
}

func TestReconcileAgent_MissingVirtualKeySecret_External(t *testing.T) {
	logger := testLogger()
	logf.SetLogger(logger)

	agent := newTestAgent()
	agent.Spec.LLMGateway = &mmv1beta.LLMGatewayConfig{
		External: &mmv1beta.ExternalLLMGateway{
			URL:              "http://litellm.external.svc.cluster.local:4000",
			VirtualKeySecret: "my-external-key-secret",
		},
	}

	botTokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agent.BotTokenSecretName(),
			Namespace: agent.Namespace,
		},
		Data: map[string][]byte{"token": []byte("bot-token")},
	}

	s := setupScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&mmv1beta.Agent{}, &appsv1.Deployment{}, &mmv1beta.Mattermost{}).
		WithRuntimeObjects(agent, newReadyMattermost(), botTokenSecret).
		Build()

	r := &AgentReconciler{
		Client:    c,
		Log:       logger,
		Scheme:    s,
		Resources: resources.NewResourceHelper(c, s),
	}

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}}
	res, err := r.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, 60*time.Second, res.RequeueAfter)

	deploy := &appsv1.Deployment{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, deploy)
	require.True(t, k8sErrors.IsNotFound(err), "agent Deployment must not be created without external virtual-key secret")

	updated := &mmv1beta.Agent{}
	require.NoError(t, c.Get(context.TODO(), req.NamespacedName, updated))
	assert.Equal(t, mmv1beta.Reconciling, updated.Status.State)
	assert.Contains(t, updated.Status.Error, "my-external-key-secret")
	assert.Contains(t, updated.Status.Error, "provisioned externally")
}

func TestReconcileAgent_AllowEgressPolicy(t *testing.T) {
	logger := testLogger()
	logf.SetLogger(logger)

	agent := newTestAgent()
	agent.Spec.EgressPolicy = mmv1beta.AgentEgressPolicyAllow

	botTokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agent.BotTokenSecretName(),
			Namespace: agent.Namespace,
		},
		Data: map[string][]byte{"token": []byte("bot-secret-token")},
	}

	s := setupScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&mmv1beta.Agent{}, &appsv1.Deployment{}, &mmv1beta.Mattermost{}).
		WithRuntimeObjects(agent, newReadyMattermost(), botTokenSecret).
		Build()

	r := &AgentReconciler{
		Client:    c,
		Log:       logger,
		Scheme:    s,
		Resources: resources.NewResourceHelper(c, s),
	}

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}}

	res, err := r.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, 6*time.Second, res.RequeueAfter)

	deploy := &appsv1.Deployment{}
	require.NoError(t, c.Get(context.TODO(), types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, deploy))
	deploy.Status.ReadyReplicas = 1
	deploy.Status.Replicas = 1
	require.NoError(t, c.Status().Update(context.TODO(), deploy))

	res, err = r.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)

	updated := &mmv1beta.Agent{}
	require.NoError(t, c.Get(context.TODO(), req.NamespacedName, updated))
	assert.Equal(t, mmv1beta.Stable, updated.Status.State)

	np := &networkingv1.NetworkPolicy{}
	require.NoError(t, c.Get(context.TODO(), types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, np))
	require.Len(t, np.Spec.Egress, 1, "allow policy must have a single catch-all egress rule")
	assert.Empty(t, np.Spec.Egress[0].To)
	assert.Empty(t, np.Spec.Egress[0].Ports)
}
