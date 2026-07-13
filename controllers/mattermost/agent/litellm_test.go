// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package agent

import (
	"context"
	"testing"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	"github.com/pkg/errors"
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
		Status: appsv1.DeploymentStatus{
			ReadyReplicas:   readyReplicas,
			UpdatedReplicas: readyReplicas,
			Replicas:        1,
		},
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

func TestCheckLiteLLMReady_MidRolloutNotReady(t *testing.T) {
	agent := newTestAgent()

	// The Deployment controller has not observed the latest generation yet;
	// the ready replica may belong to the old ReplicaSet.
	deploy := newLiteLLMDeployment(agent.Namespace, 1)
	deploy.Generation = 2
	deploy.Status.ObservedGeneration = 1

	reconciler, _ := setupReconciler(t, agent, deploy)

	ready, err := reconciler.checkLiteLLMReady(context.TODO(), agent, testLogger())
	require.NoError(t, err)
	assert.False(t, ready, "a deployment mid-rollout is not ready")
}

func TestCheckLiteLLMReady_DeploymentMissing(t *testing.T) {
	agent := newTestAgent()

	reconciler, _ := setupReconciler(t, agent)

	ready, err := reconciler.checkLiteLLMReady(context.TODO(), agent, testLogger())
	require.NoError(t, err, "missing deployment is not-ready, not an error")
	assert.False(t, ready)
}

func TestCheckLiteLLMGateway_PassthroughWithoutOperatorManagedGateway(t *testing.T) {
	agent := newTestAgent()

	reconciler, _ := setupReconciler(t, agent)

	ready, err := reconciler.checkLiteLLMGateway(context.TODO(), agent, newReadyMattermost(), testLogger())
	require.NoError(t, err)
	assert.True(t, ready, "agents without the operator-managed gateway are not gated")
}

func TestCheckLiteLLMGateway_UnconfiguredInstallationIsConfigIssue(t *testing.T) {
	agent := newTestAgent()
	agent.Spec.LLMGateway = &mmv1beta.LLMGatewayConfig{
		OperatorManaged: &mmv1beta.OperatorManagedGateway{},
	}

	reconciler, _ := setupReconciler(t, agent)

	ready, err := reconciler.checkLiteLLMGateway(context.TODO(), agent, newReadyMattermost(), testLogger())
	require.Error(t, err)
	assert.False(t, ready)
	assert.True(t, errors.Is(err, errConfigIssue))
	assert.Contains(t, err.Error(), "spec.agents.llmGateway")
}
