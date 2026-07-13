// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package agent

import (
	"context"
	"testing"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newLiteLLMDeployment(namespace string, readyReplicas int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmv1beta.AgentLiteLLMDeploymentName,
			Namespace: namespace,
		},
		Status: appsv1.DeploymentStatus{ReadyReplicas: readyReplicas, Replicas: 1},
	}
}

func TestCheckLiteLLMReady_Ready(t *testing.T) {
	agent := newTestAgent()

	reconciler, _ := setupReconciler(t, agent, newLiteLLMDeployment(agent.Namespace, 1))

	ready, err := reconciler.checkLiteLLMReady(context.TODO(), agent, testLogger())
	require.NoError(t, err)
	assert.True(t, ready)
}

func TestCheckLiteLLMReady_NoReadyReplicas(t *testing.T) {
	agent := newTestAgent()

	reconciler, _ := setupReconciler(t, agent, newLiteLLMDeployment(agent.Namespace, 0))

	ready, err := reconciler.checkLiteLLMReady(context.TODO(), agent, testLogger())
	require.NoError(t, err)
	assert.False(t, ready)
}

func TestCheckLiteLLMReady_DeploymentMissing(t *testing.T) {
	agent := newTestAgent()

	reconciler, _ := setupReconciler(t, agent)

	ready, err := reconciler.checkLiteLLMReady(context.TODO(), agent, testLogger())
	require.NoError(t, err, "missing deployment is not-ready, not an error")
	assert.False(t, ready)
}
