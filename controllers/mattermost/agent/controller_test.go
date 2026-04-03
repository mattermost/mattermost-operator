package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

	adminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "admin-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{"token": []byte("admin-token")},
	}

	// Mock the MM API server for bot provisioning.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v4/bots":
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode([]mmBot{})
		case r.Method == "POST" && r.URL.Path == "/api/v4/bots":
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(mmBot{UserID: "bot123", Username: agent.Name})
		case r.Method == "POST" && r.URL.Path == "/api/v4/users/bot123/tokens":
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(mmToken{ID: "tok1", Token: "bot-secret-token"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	s := setupScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&mmv1beta.Agent{}, &appsv1.Deployment{}, &mmv1beta.Mattermost{}).
		WithRuntimeObjects(agent, mm, adminSecret).
		Build()

	r := &AgentReconciler{
		Client:    c,
		Log:       logger,
		Scheme:    s,
		Resources: resources.NewResourceHelper(c, s),
	}

	// Override the bot provisioning to use our test server.
	// We do this by pre-creating the bot token secret (since checkAgentBot
	// uses the secret as an idempotency check, we can't easily override the URL
	// in the full reconcile loop). Instead, let's create the bot token secret
	// to skip the API calls in the real reconcile, since that path is tested
	// separately in agent_test.go.
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

	adminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "admin-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{"token": []byte("admin-token")},
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
		WithRuntimeObjects(agent, mm, adminSecret, botTokenSecret).
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

	adminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "admin-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{"token": []byte("admin-token")},
	}

	masterKeySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmv1beta.AgentLiteLLMMasterKeySecretName,
			Namespace: "default",
		},
		Data: map[string][]byte{"masterKey": []byte("sk-test-master-key")},
	}

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
		WithRuntimeObjects(agent, mm, adminSecret, masterKeySecret, botTokenSecret).
		Build()

	r := &AgentReconciler{
		Client:    c,
		Log:       logger,
		Scheme:    s,
		Resources: resources.NewResourceHelper(c, s),
	}

	// Pre-create the LiteLLM virtual key Secret so reconcileLiteLLMVirtualKey is a no-op.
	litellmKeySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agent.LiteLLMKeySecretName(),
			Namespace: agent.Namespace,
		},
		Data: map[string][]byte{"apiKey": []byte("sk-virtual-key-pre-created")},
	}
	err := c.Create(context.TODO(), litellmKeySecret)
	require.NoError(t, err)

	// Pre-create the LiteLLM Deployment with ReadyReplicas=1 so checkLiteLLMReady passes.
	litellmDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmv1beta.AgentLiteLLMDeploymentName,
			Namespace: agent.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "litellm"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "litellm"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "litellm", Image: mmv1beta.AgentLiteLLMDefaultImage}}},
			},
		},
		Status: appsv1.DeploymentStatus{ReadyReplicas: 1, Replicas: 1},
	}
	err = c.Create(context.TODO(), litellmDeploy)
	require.NoError(t, err)
	litellmDeploy.Status.ReadyReplicas = 1
	err = c.Status().Update(context.TODO(), litellmDeploy)
	require.NoError(t, err)

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}}

	// First reconcile: agent Deployment not ready yet → requeue with healthCheckRequeueDelay.
	res, err := r.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, 6*time.Second, res.RequeueAfter)

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
	assert.Equal(t, "litellm", litellmSvc.Labels["app"])

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
}
