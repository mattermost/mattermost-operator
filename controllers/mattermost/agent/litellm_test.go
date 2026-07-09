// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package agent

import (
	"context"
	"strings"
	"testing"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestCheckLiteLLMMasterKey_CreatesIfMissing(t *testing.T) {
	agent := newTestAgent()

	reconciler, _ := setupReconciler(t, agent)
	logger := testLogger()

	err := reconciler.checkLiteLLMMasterKey(context.TODO(), agent, logger)
	require.NoError(t, err)

	secret := &corev1.Secret{}
	err = reconciler.Client.Get(context.TODO(), types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMMasterKeySecretName,
		Namespace: agent.Namespace,
	}, secret)
	require.NoError(t, err)

	masterKey := string(secret.Data["masterKey"])
	assert.NotEmpty(t, masterKey)
	assert.True(t, strings.HasPrefix(masterKey, "sk-"), "master key should start with 'sk-', got %q", masterKey)
}

func TestCheckLiteLLMMasterKey_NoOpIfPresent(t *testing.T) {
	agent := newTestAgent()

	existing := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmv1beta.AgentLiteLLMMasterKeySecretName,
			Namespace: agent.Namespace,
		},
		Data: map[string][]byte{"masterKey": []byte("sk-prepopulated-value")},
	}

	reconciler, _ := setupReconciler(t, agent, existing)
	logger := testLogger()

	err := reconciler.checkLiteLLMMasterKey(context.TODO(), agent, logger)
	require.NoError(t, err)

	secret := &corev1.Secret{}
	err = reconciler.Client.Get(context.TODO(), types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMMasterKeySecretName,
		Namespace: agent.Namespace,
	}, secret)
	require.NoError(t, err)

	assert.Equal(t, []byte("sk-prepopulated-value"), secret.Data["masterKey"], "existing master key must be preserved")
}

func TestCheckLiteLLMDBCredentials_ErrorIfMissing(t *testing.T) {
	agent := newTestAgent()

	reconciler, _ := setupReconciler(t, agent)

	err := reconciler.checkLiteLLMDBCredentials(context.TODO(), agent)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mm-agent-litellm-db-credentials")
}

func TestCheckLiteLLMDBCredentials_OKIfPresent(t *testing.T) {
	agent := newTestAgent()

	dbCreds := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmv1beta.AgentLiteLLMDBCredentialsSecret,
			Namespace: agent.Namespace,
		},
		Data: map[string][]byte{"connectionString": []byte("postgres://user:pass@host/db")},
	}

	reconciler, _ := setupReconciler(t, agent, dbCreds)

	err := reconciler.checkLiteLLMDBCredentials(context.TODO(), agent)
	require.NoError(t, err)
}
