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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
	// No LLMProviders — avoids HTTP calls to in-cluster LiteLLM URL during model registration.
	// API-level behaviour (model registration, virtual key creation) is covered in litellm_test.go.
	agent.Spec.LLMGateway = &mmv1beta.LLMGatewayConfig{
		OperatorManaged: &mmv1beta.OperatorManagedLLMGateway{
			Image: mmv1beta.AgentLiteLLMDefaultImage,
		},
	}
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

	masterKeySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmv1beta.AgentLiteLLMMasterKeySecretName,
			Namespace: "default",
		},
		Data: map[string][]byte{"masterKey": []byte("sk-test-master-key")},
	}

	dbCredentialsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmv1beta.AgentLiteLLMDBCredentialsSecret,
			Namespace: "default",
		},
		Data: map[string][]byte{"connectionString": []byte("postgres://user:pass@host/db")},
	}

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
		WithRuntimeObjects(agent, mm, masterKeySecret, dbCredentialsSecret, botTokenSecret, litellmKeySecret).
		Build()

	r := &AgentReconciler{
		Client:    c,
		Log:       logger,
		Scheme:    s,
		Resources: resources.NewResourceHelper(c, s),
	}

	// Pre-create the LiteLLM Deployment with ReadyReplicas=1 so checkLiteLLMReady passes.
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

	// First reconcile: adds finalizer and returns; the Update triggers the next reconcile via the watch.
	_, err = r.Reconcile(context.Background(), req)
	require.NoError(t, err)

	afterAdd := &mmv1beta.Agent{}
	require.NoError(t, c.Get(context.TODO(), types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, afterAdd))
	assert.True(t, controllerutil.ContainsFinalizer(afterAdd, agentFinalizer), "finalizer should be added on first reconcile")

	// Second reconcile: agent Deployment not ready yet → requeue with healthCheckRequeueDelay.
	res, err := r.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, 6*time.Second, res.RequeueAfter)

	// Verify agent is still reconciling (not ready yet).
	agentAfterFirstReconcile := &mmv1beta.Agent{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, agentAfterFirstReconcile)
	require.NoError(t, err)
	assert.Equal(t, mmv1beta.Reconciling, agentAfterFirstReconcile.Status.State)

	// Verify LiteLLM ConfigMap was created (shared resource, no OwnerReference).
	litellmCM := &corev1.ConfigMap{}
	err = c.Get(context.TODO(), types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMConfigMapName,
		Namespace: agent.Namespace,
	}, litellmCM)
	require.NoError(t, err, "LiteLLM ConfigMap should be created by reconcile")

	// Verify LiteLLM Service was created.
	litellmSvc := &corev1.Service{}
	err = c.Get(context.TODO(), types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMServiceName,
		Namespace: agent.Namespace,
	}, litellmSvc)
	require.NoError(t, err, "LiteLLM Service should be created by reconcile")
	assert.Equal(t, "mm-agent-litellm", litellmSvc.Labels["app"])

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

// newFinalizerTestAgent builds an Agent CR with the supplied name and an optional
// LLMGateway override for use in finalizer-focused tests.
func newFinalizerTestAgent(name string, gateway *mmv1beta.LLMGatewayConfig) *mmv1beta.Agent {
	a := newTestAgent()
	a.Name = name
	a.UID = types.UID(name + "-uid")
	a.Spec.LLMGateway = gateway
	return a
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

func TestFinalizer_AddedForOperatorManaged(t *testing.T) {
	logger := testLogger()
	logf.SetLogger(logger)

	agent := newFinalizerTestAgent("finalizer-agent", &mmv1beta.LLMGatewayConfig{
		OperatorManaged: &mmv1beta.OperatorManagedLLMGateway{
			Image: mmv1beta.AgentLiteLLMDefaultImage,
		},
	})

	s := setupScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&mmv1beta.Agent{}).
		WithRuntimeObjects(agent, newReadyMattermost()).
		Build()

	r := &AgentReconciler{Client: c, Log: logger, Scheme: s, Resources: resources.NewResourceHelper(c, s)}

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}}
	_, err := r.Reconcile(context.Background(), req)
	require.NoError(t, err)

	updated := &mmv1beta.Agent{}
	require.NoError(t, c.Get(context.TODO(), req.NamespacedName, updated))
	assert.Contains(t, updated.Finalizers, agentFinalizer)
}

func TestFinalizer_NotAddedForExternal(t *testing.T) {
	logger := testLogger()
	logf.SetLogger(logger)

	agent := newFinalizerTestAgent("external-agent", &mmv1beta.LLMGatewayConfig{
		External: &mmv1beta.ExternalLLMGateway{
			URL:              "http://litellm.external.svc.cluster.local:4000",
			VirtualKeySecret: "my-external-key-secret",
		},
	})

	botSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agent.BotTokenSecretName(),
			Namespace: agent.Namespace,
		},
		Data: map[string][]byte{"token": []byte("bot-token")},
	}
	virtualKeySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-external-key-secret",
			Namespace: agent.Namespace,
		},
		Data: map[string][]byte{"key": []byte("sk-external-key")},
	}

	s := setupScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&mmv1beta.Agent{}, &appsv1.Deployment{}).
		WithRuntimeObjects(agent, newReadyMattermost(), botSecret, virtualKeySecret).
		Build()

	r := &AgentReconciler{Client: c, Log: logger, Scheme: s, Resources: resources.NewResourceHelper(c, s)}

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}}
	_, err := r.Reconcile(context.Background(), req)
	require.NoError(t, err)

	updated := &mmv1beta.Agent{}
	require.NoError(t, c.Get(context.TODO(), req.NamespacedName, updated))
	assert.NotContains(t, updated.Finalizers, agentFinalizer)
}

func TestFinalizer_CleanupTearsDownLiteLLMWhenLast(t *testing.T) {
	logger := testLogger()
	logf.SetLogger(logger)

	agent := newFinalizerTestAgent("last-agent", &mmv1beta.LLMGatewayConfig{
		OperatorManaged: &mmv1beta.OperatorManagedLLMGateway{
			Image: mmv1beta.AgentLiteLLMDefaultImage,
		},
	})

	botSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agent.BotTokenSecretName(),
			Namespace: agent.Namespace,
		},
		Data: map[string][]byte{"token": []byte("bot-token")},
	}

	masterKey := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmv1beta.AgentLiteLLMMasterKeySecretName,
			Namespace: agent.Namespace,
		},
		Data: map[string][]byte{"masterKey": []byte("sk-test-master-key")},
	}
	dbCreds := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmv1beta.AgentLiteLLMDBCredentialsSecret,
			Namespace: agent.Namespace,
		},
		Data: map[string][]byte{"connectionString": []byte("postgres://user:pass@host/db")},
	}

	s := setupScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&mmv1beta.Agent{}, &appsv1.Deployment{}).
		WithRuntimeObjects(agent, newReadyMattermost(), botSecret, masterKey, dbCreds).
		Build()

	r := &AgentReconciler{Client: c, Log: logger, Scheme: s, Resources: resources.NewResourceHelper(c, s)}

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}}

	// Reconcile twice: add finalizer, then create LiteLLM resources.
	_, err := r.Reconcile(context.Background(), req)
	require.NoError(t, err)
	_, err = r.Reconcile(context.Background(), req)
	require.NoError(t, err)

	// Verify LiteLLM Deployment exists before deletion.
	litellmDeploy := &appsv1.Deployment{}
	err = c.Get(context.TODO(), types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMDeploymentName,
		Namespace: agent.Namespace,
	}, litellmDeploy)
	require.NoError(t, err, "LiteLLM Deployment should exist before deletion")

	// Mark agent for deletion. Because the finalizer is set, Delete is non-blocking and
	// the fake client populates DeletionTimestamp instead of removing the object.
	updated := &mmv1beta.Agent{}
	require.NoError(t, c.Get(context.TODO(), req.NamespacedName, updated))
	require.NoError(t, c.Delete(context.TODO(), updated))

	// Reconcile to run cleanup.
	_, err = r.Reconcile(context.Background(), req)
	require.NoError(t, err)

	// LiteLLM Deployment should be gone.
	err = c.Get(context.TODO(), types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMDeploymentName,
		Namespace: agent.Namespace,
	}, litellmDeploy)
	assert.True(t, k8sErrors.IsNotFound(err), "LiteLLM Deployment should be removed; got err=%v", err)

	// Master-key Secret should be retained (DB outlives teardown).
	retainedMasterKey := &corev1.Secret{}
	err = c.Get(context.TODO(), types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMMasterKeySecretName,
		Namespace: agent.Namespace,
	}, retainedMasterKey)
	require.NoError(t, err, "LiteLLM master-key Secret should be retained")
	assert.Equal(t, []byte("sk-test-master-key"), retainedMasterKey.Data["masterKey"])

	// Agent should have its finalizer removed (and may already be gone).
	finalAgent := &mmv1beta.Agent{}
	err = c.Get(context.TODO(), req.NamespacedName, finalAgent)
	if err == nil {
		assert.NotContains(t, finalAgent.Finalizers, agentFinalizer)
	} else {
		assert.True(t, k8sErrors.IsNotFound(err))
	}
}

func TestFinalizer_CleanupLeavesLiteLLMWhenSiblingsExist(t *testing.T) {
	logger := testLogger()
	logf.SetLogger(logger)

	agentA := newFinalizerTestAgent("agent-a", &mmv1beta.LLMGatewayConfig{
		OperatorManaged: &mmv1beta.OperatorManagedLLMGateway{
			Image: mmv1beta.AgentLiteLLMDefaultImage,
		},
	})
	agentB := newFinalizerTestAgent("agent-b", &mmv1beta.LLMGatewayConfig{
		OperatorManaged: &mmv1beta.OperatorManagedLLMGateway{
			Image: mmv1beta.AgentLiteLLMDefaultImage,
		},
	})

	botSecretA := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: agentA.BotTokenSecretName(), Namespace: agentA.Namespace},
		Data:       map[string][]byte{"token": []byte("bot-token-a")},
	}
	botSecretB := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: agentB.BotTokenSecretName(), Namespace: agentB.Namespace},
		Data:       map[string][]byte{"token": []byte("bot-token-b")},
	}
	masterKey := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: mmv1beta.AgentLiteLLMMasterKeySecretName, Namespace: agentA.Namespace},
		Data:       map[string][]byte{"masterKey": []byte("sk-test-master-key")},
	}
	dbCreds := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: mmv1beta.AgentLiteLLMDBCredentialsSecret, Namespace: agentA.Namespace},
		Data:       map[string][]byte{"connectionString": []byte("postgres://user:pass@host/db")},
	}

	s := setupScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&mmv1beta.Agent{}, &appsv1.Deployment{}).
		WithRuntimeObjects(agentA, agentB, newReadyMattermost(), botSecretA, botSecretB, masterKey, dbCreds).
		Build()

	r := &AgentReconciler{Client: c, Log: logger, Scheme: s, Resources: resources.NewResourceHelper(c, s)}

	reqA := reconcile.Request{NamespacedName: types.NamespacedName{Name: agentA.Name, Namespace: agentA.Namespace}}
	reqB := reconcile.Request{NamespacedName: types.NamespacedName{Name: agentB.Name, Namespace: agentB.Namespace}}

	// Reconcile both to add finalizers and provision LiteLLM.
	_, err := r.Reconcile(context.Background(), reqA)
	require.NoError(t, err)
	_, err = r.Reconcile(context.Background(), reqA)
	require.NoError(t, err)
	_, err = r.Reconcile(context.Background(), reqB)
	require.NoError(t, err)
	_, err = r.Reconcile(context.Background(), reqB)
	require.NoError(t, err)

	// Mark agentA for deletion. Finalizer keeps the object alive while DeletionTimestamp is set.
	updatedA := &mmv1beta.Agent{}
	require.NoError(t, c.Get(context.TODO(), reqA.NamespacedName, updatedA))
	require.NoError(t, c.Delete(context.TODO(), updatedA))

	// Reconcile agentA — cleanup should NOT delete LiteLLM because agentB still exists.
	_, err = r.Reconcile(context.Background(), reqA)
	require.NoError(t, err)

	// LiteLLM Deployment should still exist.
	litellmDeploy := &appsv1.Deployment{}
	err = c.Get(context.TODO(), types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMDeploymentName,
		Namespace: agentA.Namespace,
	}, litellmDeploy)
	require.NoError(t, err, "LiteLLM Deployment should remain because agent-b is a sibling")

	// agent-b should still have its finalizer.
	finalB := &mmv1beta.Agent{}
	require.NoError(t, c.Get(context.TODO(), reqB.NamespacedName, finalB))
	assert.Contains(t, finalB.Finalizers, agentFinalizer)
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
		OperatorManaged: &mmv1beta.OperatorManagedLLMGateway{
			Image: mmv1beta.AgentLiteLLMDefaultImage,
		},
	}
	_ = agent.SetDefaults()

	botTokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agent.BotTokenSecretName(),
			Namespace: agent.Namespace,
		},
		Data: map[string][]byte{"token": []byte("bot-secret-token")},
	}
	dbCredentialsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmv1beta.AgentLiteLLMDBCredentialsSecret,
			Namespace: agent.Namespace,
		},
		Data: map[string][]byte{"connectionString": []byte("postgres://user:pass@host/db")},
	}
	masterKeySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmv1beta.AgentLiteLLMMasterKeySecretName,
			Namespace: agent.Namespace,
		},
		Data: map[string][]byte{"masterKey": []byte("sk-test-master-key")},
	}

	s := setupScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&mmv1beta.Agent{}, &appsv1.Deployment{}, &mmv1beta.Mattermost{}).
		WithRuntimeObjects(agent, newReadyMattermost(), botTokenSecret, dbCredentialsSecret, masterKeySecret).
		Build()

	r := &AgentReconciler{
		Client:    c,
		Log:       logger,
		Scheme:    s,
		Resources: resources.NewResourceHelper(c, s),
	}

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
	require.NoError(t, c.Create(context.TODO(), litellmDeploy))
	litellmDeploy.Status.ReadyReplicas = 1
	require.NoError(t, c.Status().Update(context.TODO(), litellmDeploy))

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}}

	// First reconcile adds the finalizer.
	_, err := r.Reconcile(context.Background(), req)
	require.NoError(t, err)

	// Second reconcile: LiteLLM ready, but virtual-key secret missing.
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
