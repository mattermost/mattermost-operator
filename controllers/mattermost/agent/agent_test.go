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
	"k8s.io/apimachinery/pkg/api/resource"
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
			MattermostRef: mmv1beta.AgentMattermostRef{Name: "mm-test"},
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

func testLogger() logr.Logger {
	logSink := blubr.InitLogger(logrus.NewEntry(logrus.New()))
	return logr.New(logSink.WithName("test"))
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
	assert.Equal(t, "http", svc.Spec.Ports[0].Name)

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

	// Verify volumes — only bot-token remains.
	assert.Len(t, deployment.Spec.Template.Spec.Volumes, 1)
	assert.Equal(t, "bot-token", deployment.Spec.Template.Spec.Volumes[0].Name)

	// Verify no init containers.
	assert.Empty(t, deployment.Spec.Template.Spec.InitContainers, "init containers must be removed")

	// Verify no HOME env var.
	for _, e := range container.Env {
		assert.NotEqual(t, "HOME", e.Name, "HOME env var must not be present")
	}
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

func TestCheckAgentNetworkPolicy_AllowWeb(t *testing.T) {
	agent := newTestAgent()
	agent.Spec.EgressPolicy = mmv1beta.AgentEgressPolicyAllowWeb
	_ = agent.SetDefaults()

	reconciler, _ := setupReconciler(t, agent)

	logSink := blubr.InitLogger(logrus.NewEntry(logrus.New()))
	logger := logr.New(logSink.WithName("test"))

	err := reconciler.checkAgentNetworkPolicy(context.TODO(), agent, logger)
	require.NoError(t, err)

	np := &networkingv1.NetworkPolicy{}
	err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, np)
	require.NoError(t, err)

	// AllowWeb policy: MM egress (8065) + DNS (53) + HTTPS (443) + HTTP (80) = 4 egress rules.
	assert.Len(t, np.Spec.Egress, 4)
}

func TestCheckHookSecret_CreatesSecret(t *testing.T) {
	agent := newTestAgent()
	_ = agent.SetDefaults()

	reconciler, _ := setupReconciler(t, agent)
	logger := testLogger()

	err := reconciler.checkHookSecret(context.TODO(), agent, logger)
	require.NoError(t, err)

	// Verify Secret was created.
	secret := &corev1.Secret{}
	err = reconciler.Client.Get(context.TODO(), types.NamespacedName{
		Name:      agent.HookSecretName(),
		Namespace: agent.Namespace,
	}, secret)
	require.NoError(t, err)
	assert.Contains(t, secret.Data, "hookSecret")

	// Verify the hook secret is a 64-character hex string (32 bytes encoded).
	hookSecret := string(secret.Data["hookSecret"])
	assert.Len(t, hookSecret, 64)
}

func TestCheckHookSecret_Idempotent(t *testing.T) {
	agent := newTestAgent()
	_ = agent.SetDefaults()

	// Pre-create the hook secret.
	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agent.HookSecretName(),
			Namespace: agent.Namespace,
		},
		Data: map[string][]byte{"hookSecret": []byte("pre-existing-value")},
	}

	reconciler, _ := setupReconciler(t, agent, existingSecret)
	logger := testLogger()

	err := reconciler.checkHookSecret(context.TODO(), agent, logger)
	require.NoError(t, err)

	// Verify original value is preserved.
	secret := &corev1.Secret{}
	err = reconciler.Client.Get(context.TODO(), types.NamespacedName{
		Name:      agent.HookSecretName(),
		Namespace: agent.Namespace,
	}, secret)
	require.NoError(t, err)
	assert.Equal(t, []byte("pre-existing-value"), secret.Data["hookSecret"])
}

func TestCheckAgentHealth_CarriesForwardPriorStatus(t *testing.T) {
	agent := newTestAgent()
	_ = agent.SetDefaults()

	// Pre-create a ready Deployment so the health check succeeds.
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agent.Name,
			Namespace: agent.Namespace,
		},
		Status: appsv1.DeploymentStatus{ReadyReplicas: 1, Replicas: 1},
	}

	reconciler, _ := setupReconciler(t, agent, deployment)
	logger := testLogger()

	prior := mmv1beta.AgentStatus{
		ObservedGeneration: 7,
	}

	status, err := reconciler.checkAgentHealth(context.TODO(), agent, prior, logger)
	require.NoError(t, err)

	// New fields are set correctly; ObservedGeneration is overwritten from the CR.
	assert.Equal(t, mmv1beta.Stable, status.State)
	assert.Equal(t, agent.Generation, status.ObservedGeneration)
	assert.Empty(t, status.Error)
	assert.Contains(t, status.Endpoint, "http://")
}

func TestCheckAgentPVC_Creates(t *testing.T) {
	agent := newTestAgent()
	agent.Spec.Storage = &mmv1beta.AgentStorageConfig{
		Size:      resource.MustParse("1Gi"),
		MountPath: "/data",
	}
	_ = agent.SetDefaults()

	reconciler, _ := setupReconciler(t, agent)
	logger := testLogger()

	err := reconciler.checkAgentPVC(context.TODO(), agent, logger)
	require.NoError(t, err)

	// Verify PVC was created.
	pvc := &corev1.PersistentVolumeClaim{}
	err = reconciler.Client.Get(context.TODO(), types.NamespacedName{
		Name:      agent.StoragePVCName(),
		Namespace: agent.Namespace,
	}, pvc)
	require.NoError(t, err)

	assert.Equal(t, agent.StoragePVCName(), pvc.Name)
	assert.Equal(t, agent.Namespace, pvc.Namespace)
	assert.Equal(t, []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}, pvc.Spec.AccessModes)

	storageReq := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
	assert.Equal(t, resource.MustParse("1Gi"), storageReq)
}

func TestCheckAgentPVC_Skips(t *testing.T) {
	agent := newTestAgent()
	// Storage is nil — PVC should not be created.
	_ = agent.SetDefaults()

	reconciler, _ := setupReconciler(t, agent)
	logger := testLogger()

	err := reconciler.checkAgentPVC(context.TODO(), agent, logger)
	require.NoError(t, err)

	// Verify no PVC exists.
	pvc := &corev1.PersistentVolumeClaim{}
	err = reconciler.Client.Get(context.TODO(), types.NamespacedName{
		Name:      agent.Name + "-storage",
		Namespace: agent.Namespace,
	}, pvc)
	require.Error(t, err, "PVC should not exist when Storage is nil")
}

func TestCheckAgentPVC_OwnerReference(t *testing.T) {
	agent := newTestAgent()
	agent.Spec.Storage = &mmv1beta.AgentStorageConfig{
		Size:      resource.MustParse("2Gi"),
		MountPath: "/data",
	}
	_ = agent.SetDefaults()

	reconciler, _ := setupReconciler(t, agent)
	logger := testLogger()

	err := reconciler.checkAgentPVC(context.TODO(), agent, logger)
	require.NoError(t, err)

	pvc := &corev1.PersistentVolumeClaim{}
	err = reconciler.Client.Get(context.TODO(), types.NamespacedName{
		Name:      agent.StoragePVCName(),
		Namespace: agent.Namespace,
	}, pvc)
	require.NoError(t, err)

	// Verify OwnerReference points to the Agent CR.
	require.Len(t, pvc.OwnerReferences, 1)
	assert.Equal(t, "Agent", pvc.OwnerReferences[0].Kind)
	assert.Equal(t, agent.Name, pvc.OwnerReferences[0].Name)
}

func TestCheckAgentPVC_WithStorageClass(t *testing.T) {
	agent := newTestAgent()
	storageClass := "gp3-encrypted"
	agent.Spec.Storage = &mmv1beta.AgentStorageConfig{
		Size:             resource.MustParse("10Gi"),
		StorageClassName: &storageClass,
		MountPath:        "/workspace",
	}
	_ = agent.SetDefaults()

	reconciler, _ := setupReconciler(t, agent)
	logger := testLogger()

	err := reconciler.checkAgentPVC(context.TODO(), agent, logger)
	require.NoError(t, err)

	pvc := &corev1.PersistentVolumeClaim{}
	err = reconciler.Client.Get(context.TODO(), types.NamespacedName{
		Name:      agent.StoragePVCName(),
		Namespace: agent.Namespace,
	}, pvc)
	require.NoError(t, err)

	require.NotNil(t, pvc.Spec.StorageClassName)
	assert.Equal(t, "gp3-encrypted", *pvc.Spec.StorageClassName)
}

func TestCheckAgentPVC_Idempotent(t *testing.T) {
	agent := newTestAgent()
	agent.Spec.Storage = &mmv1beta.AgentStorageConfig{
		Size:      resource.MustParse("1Gi"),
		MountPath: "/data",
	}
	_ = agent.SetDefaults()

	reconciler, _ := setupReconciler(t, agent)
	logger := testLogger()

	// First call creates.
	err := reconciler.checkAgentPVC(context.TODO(), agent, logger)
	require.NoError(t, err)

	// Second call is idempotent.
	err = reconciler.checkAgentPVC(context.TODO(), agent, logger)
	require.NoError(t, err)
}

func TestCheckAgentNetworkPolicy_Allow(t *testing.T) {
	agent := newTestAgent()
	agent.Spec.EgressPolicy = mmv1beta.AgentEgressPolicyAllow
	_ = agent.SetDefaults()

	reconciler, _ := setupReconciler(t, agent)

	logSink := blubr.InitLogger(logrus.NewEntry(logrus.New()))
	logger := logr.New(logSink.WithName("test"))

	err := reconciler.checkAgentNetworkPolicy(context.TODO(), agent, logger)
	require.NoError(t, err)

	np := &networkingv1.NetworkPolicy{}
	err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, np)
	require.NoError(t, err)

	// Allow policy: 1 egress rule (empty = allow all).
	require.Len(t, np.Spec.Egress, 1)
	assert.Empty(t, np.Spec.Egress[0].To, "allow-all rule has no To selector")
	assert.Empty(t, np.Spec.Egress[0].Ports, "allow-all rule has no Ports restriction")

	// Ingress still restricts to MM pods.
	require.Len(t, np.Spec.Ingress, 1)
	assert.Len(t, np.Spec.Ingress[0].From, 1)
}
