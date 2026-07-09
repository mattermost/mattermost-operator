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

func TestReconcileAgent_MissingBotTokenSecret(t *testing.T) {
	logger := testLogger()
	logf.SetLogger(logger)

	agent := newTestAgent()

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
