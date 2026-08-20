package mattermost

import (
	"context"
	"testing"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	mattermostApp "github.com/mattermost/mattermost-operator/pkg/mattermost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func newLiteLLMTestMattermost(t *testing.T) *mmv1beta.Mattermost {
	mm := &mmv1beta.Mattermost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
			UID:       types.UID("test"),
		},
		Spec: mmv1beta.MattermostSpec{
			Image:       "mattermost/mattermost-enterprise-edition",
			Version:     "10.0.0",
			IngressName: "foo.mattermost.dev",
			Agents: &mmv1beta.MattermostAgents{
				Enabled:    true,
				LLMGateway: &mmv1beta.AgentsLLMGateway{},
			},
		},
	}
	require.NoError(t, mm.SetDefaults())
	return mm
}

func newLiteLLMDBCredentialsSecret(namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmv1beta.AgentLiteLLMDBCredentialsSecret,
			Namespace: namespace,
		},
		Data: map[string][]byte{mmv1beta.SecretKeyConnectionString: []byte("postgres://user:pass@host/db")},
	}
}

func TestCheckLiteLLM(t *testing.T) {
	logger, fakeClient, reconciler := setupTestDeps(t)

	mm := newLiteLLMTestMattermost(t)
	mm.Spec.Agents.LLMGateway.Image = "ghcr.io/berriai/litellm-database:v1.99.9"
	mm.Spec.Agents.LLMGateway.Resources = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("250m"),
			corev1.ResourceMemory: resource.MustParse("256Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("1Gi"),
		},
	}

	require.NoError(t, fakeClient.Create(context.TODO(), newLiteLLMDBCredentialsSecret(mm.Namespace)))

	err := reconciler.checkLiteLLM(context.TODO(), mm, logger)
	require.NoError(t, err)

	t.Run("deployment owned by mm CR with image and resources from spec", func(t *testing.T) {
		deployment := &appsv1.Deployment{}
		err = fakeClient.Get(context.TODO(), types.NamespacedName{
			Name:      mmv1beta.AgentLiteLLMDeploymentName,
			Namespace: mm.Namespace,
		}, deployment)
		require.NoError(t, err)

		require.Len(t, deployment.OwnerReferences, 1)
		assert.Equal(t, "Mattermost", deployment.OwnerReferences[0].Kind)
		assert.Equal(t, mm.Name, deployment.OwnerReferences[0].Name)

		require.Len(t, deployment.Spec.Template.Spec.Containers, 1)
		container := deployment.Spec.Template.Spec.Containers[0]
		assert.Equal(t, "ghcr.io/berriai/litellm-database:v1.99.9", container.Image)
		assert.Equal(t, resource.MustParse("250m"), container.Resources.Requests[corev1.ResourceCPU])
		assert.Equal(t, resource.MustParse("256Mi"), container.Resources.Requests[corev1.ResourceMemory])
		assert.Equal(t, resource.MustParse("1"), container.Resources.Limits[corev1.ResourceCPU])
		assert.Equal(t, resource.MustParse("1Gi"), container.Resources.Limits[corev1.ResourceMemory])
	})

	t.Run("service owned by mm CR", func(t *testing.T) {
		service := &corev1.Service{}
		err = fakeClient.Get(context.TODO(), types.NamespacedName{
			Name:      mmv1beta.AgentLiteLLMServiceName,
			Namespace: mm.Namespace,
		}, service)
		require.NoError(t, err)

		require.Len(t, service.OwnerReferences, 1)
		assert.Equal(t, "Mattermost", service.OwnerReferences[0].Kind)
		assert.Equal(t, mm.Name, service.OwnerReferences[0].Name)
	})

	t.Run("master key created unowned and preserved on subsequent reconciles", func(t *testing.T) {
		secret := &corev1.Secret{}
		err = fakeClient.Get(context.TODO(), types.NamespacedName{
			Name:      mmv1beta.AgentLiteLLMMasterKeySecretName,
			Namespace: mm.Namespace,
		}, secret)
		require.NoError(t, err)

		assert.Empty(t, secret.OwnerReferences, "master-key Secret must not be owned")
		originalKey := string(secret.Data[mmv1beta.SecretKeyMasterKey])
		assert.NotEmpty(t, originalKey)

		err = reconciler.checkLiteLLM(context.TODO(), mm, logger)
		require.NoError(t, err)

		err = fakeClient.Get(context.TODO(), types.NamespacedName{
			Name:      mmv1beta.AgentLiteLLMMasterKeySecretName,
			Namespace: mm.Namespace,
		}, secret)
		require.NoError(t, err)
		assert.Equal(t, originalKey, string(secret.Data[mmv1beta.SecretKeyMasterKey]), "master key must be preserved")
	})
}

func TestCheckLiteLLM_OwnerReferencesSurviveUpdate(t *testing.T) {
	logger, fakeClient, reconciler := setupTestDeps(t)

	mm := newLiteLLMTestMattermost(t)
	require.NoError(t, fakeClient.Create(context.TODO(), newLiteLLMDBCredentialsSecret(mm.Namespace)))
	require.NoError(t, reconciler.checkLiteLLM(context.TODO(), mm, logger))

	// Change the spec so the second reconcile issues a real Update.
	mm.Spec.Agents.LLMGateway.Image = "ghcr.io/berriai/litellm-database:v2.0.0"
	require.NoError(t, reconciler.checkLiteLLM(context.TODO(), mm, logger))

	deployment := &appsv1.Deployment{}
	require.NoError(t, fakeClient.Get(context.TODO(), types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMDeploymentName,
		Namespace: mm.Namespace,
	}, deployment))
	assert.Equal(t, "ghcr.io/berriai/litellm-database:v2.0.0", deployment.Spec.Template.Spec.Containers[0].Image)
	require.Len(t, deployment.OwnerReferences, 1, "owner references must survive updates")
	assert.Equal(t, "Mattermost", deployment.OwnerReferences[0].Kind)
	assert.Equal(t, mm.Name, deployment.OwnerReferences[0].Name)

	service := &corev1.Service{}
	require.NoError(t, fakeClient.Get(context.TODO(), types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMServiceName,
		Namespace: mm.Namespace,
	}, service))
	require.Len(t, service.OwnerReferences, 1, "owner references must survive updates")
	assert.Equal(t, mm.Name, service.OwnerReferences[0].Name)
}

func TestCheckLiteLLM_DBCredentialsRotationRollsPods(t *testing.T) {
	logger, fakeClient, reconciler := setupTestDeps(t)

	mm := newLiteLLMTestMattermost(t)
	secret := newLiteLLMDBCredentialsSecret(mm.Namespace)
	require.NoError(t, fakeClient.Create(context.TODO(), secret))
	require.NoError(t, reconciler.checkLiteLLM(context.TODO(), mm, logger))

	deploymentKey := types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMDeploymentName,
		Namespace: mm.Namespace,
	}
	deployment := &appsv1.Deployment{}
	require.NoError(t, fakeClient.Get(context.TODO(), deploymentKey, deployment))
	originalHash := deployment.Spec.Template.Annotations[mattermostApp.LiteLLMDBCredentialsHashAnnotation]
	require.NotEmpty(t, originalHash)

	secret.Data[mmv1beta.SecretKeyConnectionString] = []byte("postgres://user:rotated@host/db")
	require.NoError(t, fakeClient.Update(context.TODO(), secret))
	require.NoError(t, reconciler.checkLiteLLM(context.TODO(), mm, logger))

	require.NoError(t, fakeClient.Get(context.TODO(), deploymentKey, deployment))
	rotatedHash := deployment.Spec.Template.Annotations[mattermostApp.LiteLLMDBCredentialsHashAnnotation]
	assert.NotEmpty(t, rotatedHash)
	assert.NotEqual(t, originalHash, rotatedHash, "rotating db credentials must change the pod template annotation")
}

func TestCheckLiteLLM_MissingDBCredentials(t *testing.T) {
	logger, fakeClient, reconciler := setupTestDeps(t)

	mm := newLiteLLMTestMattermost(t)

	err := reconciler.checkLiteLLM(context.TODO(), mm, logger)
	require.Error(t, err)
	assert.Contains(t, err.Error(), mmv1beta.AgentLiteLLMDBCredentialsSecret)

	deployment := &appsv1.Deployment{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMDeploymentName,
		Namespace: mm.Namespace,
	}, deployment)
	assert.True(t, k8sErrors.IsNotFound(err), "deployment must not be created without db credentials")
}

func TestCheckLiteLLM_InvalidConnectionString(t *testing.T) {
	tests := []struct {
		name string
		data map[string][]byte
	}{
		{name: "key missing", data: map[string][]byte{"other": []byte("value")}},
		{name: "key empty", data: map[string][]byte{mmv1beta.SecretKeyConnectionString: nil}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, fakeClient, reconciler := setupTestDeps(t)
			mm := newLiteLLMTestMattermost(t)
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      mmv1beta.AgentLiteLLMDBCredentialsSecret,
					Namespace: mm.Namespace,
				},
				Data: tt.data,
			}
			require.NoError(t, fakeClient.Create(context.TODO(), secret))

			err := reconciler.checkLiteLLM(context.TODO(), mm, logger)
			require.Error(t, err)
			assert.Contains(t, err.Error(), mmv1beta.SecretKeyConnectionString)
		})
	}
}

func TestCheckLiteLLM_RecreatesDeletedService(t *testing.T) {
	logger, fakeClient, reconciler := setupTestDeps(t)
	mm := newLiteLLMTestMattermost(t)
	require.NoError(t, fakeClient.Create(
		context.TODO(), newLiteLLMDBCredentialsSecret(mm.Namespace),
	))
	require.NoError(t, reconciler.checkLiteLLM(context.TODO(), mm, logger))

	service := &corev1.Service{}
	key := types.NamespacedName{
		Name: mmv1beta.AgentLiteLLMServiceName, Namespace: mm.Namespace,
	}
	require.NoError(t, fakeClient.Get(context.TODO(), key, service))
	require.NoError(t, fakeClient.Delete(context.TODO(), service))

	require.NoError(t, reconciler.checkLiteLLM(context.TODO(), mm, logger))
	require.NoError(t, fakeClient.Get(context.TODO(), key, &corev1.Service{}))
}

func TestCheckLiteLLM_DisableTransition(t *testing.T) {
	logger, fakeClient, reconciler := setupTestDeps(t)

	mm := newLiteLLMTestMattermost(t)
	require.NoError(t, fakeClient.Create(context.TODO(), newLiteLLMDBCredentialsSecret(mm.Namespace)))

	err := reconciler.checkLiteLLM(context.TODO(), mm, logger)
	require.NoError(t, err)

	// With the gateway still enabled the cleanup is a no-op.
	require.NoError(t, reconciler.cleanupLiteLLMIfDisabled(context.TODO(), mm, logger))
	require.NoError(t, fakeClient.Get(context.TODO(), types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMDeploymentName,
		Namespace: mm.Namespace,
	}, &appsv1.Deployment{}))

	// Disable the gateway.
	mm.Spec.Agents.LLMGateway = nil

	require.NoError(t, reconciler.checkLiteLLM(context.TODO(), mm, logger), "create/update path must be a no-op when disabled")
	require.NoError(t, reconciler.cleanupLiteLLMIfDisabled(context.TODO(), mm, logger))

	deployment := &appsv1.Deployment{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMDeploymentName,
		Namespace: mm.Namespace,
	}, deployment)
	assert.True(t, k8sErrors.IsNotFound(err), "deployment must be deleted when gateway is disabled")

	service := &corev1.Service{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMServiceName,
		Namespace: mm.Namespace,
	}, service)
	assert.True(t, k8sErrors.IsNotFound(err), "service must be deleted when gateway is disabled")

	masterKey := &corev1.Secret{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMMasterKeySecretName,
		Namespace: mm.Namespace,
	}, masterKey)
	require.NoError(t, err, "master-key Secret must be retained when gateway is disabled")
	assert.NotEmpty(t, masterKey.Data[mmv1beta.SecretKeyMasterKey])

	// Disabled with nothing deployed is a no-op.
	err = reconciler.cleanupLiteLLMIfDisabled(context.TODO(), mm, logger)
	require.NoError(t, err)
}

func TestCheckLiteLLM_DoesNotStealOtherInstallationsGateway(t *testing.T) {
	logger, fakeClient, reconciler := setupTestDeps(t)

	// Installation A owns the gateway in the namespace.
	mmA := newLiteLLMTestMattermost(t)
	mmA.Name = "installation-a"
	mmA.UID = types.UID("uid-a")
	require.NoError(t, fakeClient.Create(context.TODO(), newLiteLLMDBCredentialsSecret(mmA.Namespace)))
	require.NoError(t, reconciler.checkLiteLLM(context.TODO(), mmA, logger))

	// Installation B also enables the gateway; it must not take over A's objects.
	mmB := newLiteLLMTestMattermost(t)
	mmB.Name = "installation-b"
	mmB.UID = types.UID("uid-b")
	mmB.Spec.Agents.LLMGateway.Image = "ghcr.io/berriai/litellm-database:v9.9.9"

	err := reconciler.checkLiteLLM(context.TODO(), mmB, logger)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not managed by this Mattermost installation")

	deployment := &appsv1.Deployment{}
	require.NoError(t, fakeClient.Get(context.TODO(), types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMDeploymentName,
		Namespace: mmA.Namespace,
	}, deployment))
	require.Len(t, deployment.OwnerReferences, 1)
	assert.Equal(t, mmA.Name, deployment.OwnerReferences[0].Name, "gateway must remain owned by installation A")
	assert.NotEqual(t, "ghcr.io/berriai/litellm-database:v9.9.9", deployment.Spec.Template.Spec.Containers[0].Image)
}

func TestCleanupLiteLLMIfDisabled_DoesNotDeleteOtherInstallationsGateway(t *testing.T) {
	logger, fakeClient, reconciler := setupTestDeps(t)

	// Installation A owns the gateway in the namespace.
	mmA := newLiteLLMTestMattermost(t)
	mmA.Name = "installation-a"
	mmA.UID = types.UID("uid-a")
	require.NoError(t, fakeClient.Create(context.TODO(), newLiteLLMDBCredentialsSecret(mmA.Namespace)))
	require.NoError(t, reconciler.checkLiteLLM(context.TODO(), mmA, logger))

	// Installation B in the same namespace has the gateway disabled.
	mmB := newLiteLLMTestMattermost(t)
	mmB.Name = "installation-b"
	mmB.UID = types.UID("uid-b")
	mmB.Spec.Agents = nil

	require.NoError(t, reconciler.cleanupLiteLLMIfDisabled(context.TODO(), mmB, logger))

	deployment := &appsv1.Deployment{}
	require.NoError(t, fakeClient.Get(context.TODO(), types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMDeploymentName,
		Namespace: mmA.Namespace,
	}, deployment), "installation B must not delete installation A's gateway deployment")

	service := &corev1.Service{}
	require.NoError(t, fakeClient.Get(context.TODO(), types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMServiceName,
		Namespace: mmA.Namespace,
	}, service), "installation B must not delete installation A's gateway service")

	// The owning installation can still clean its own gateway up.
	mmA.Spec.Agents.LLMGateway = nil
	require.NoError(t, reconciler.cleanupLiteLLMIfDisabled(context.TODO(), mmA, logger))

	err := fakeClient.Get(context.TODO(), types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMDeploymentName,
		Namespace: mmA.Namespace,
	}, &appsv1.Deployment{})
	assert.True(t, k8sErrors.IsNotFound(err))
}
