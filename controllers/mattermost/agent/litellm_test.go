// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package agent

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestCheckLiteLLMReady(t *testing.T) {
	tests := []struct {
		name       string
		deployment func(namespace string) client.Object
		wantReady  bool
	}{
		{
			name: "rollout complete",
			deployment: func(namespace string) client.Object {
				return newReadyLiteLLMDeployment(namespace)
			},
			wantReady: true,
		},
		{
			name: "no ready replicas",
			deployment: func(namespace string) client.Object {
				deployment := newReadyLiteLLMDeployment(namespace)
				deployment.Status.ReadyReplicas = 0
				return deployment
			},
		},
		{
			name: "generation not observed",
			deployment: func(namespace string) client.Object {
				deployment := newReadyLiteLLMDeployment(namespace)
				deployment.Generation = 2
				return deployment
			},
		},
		{name: "deployment missing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := newTestAgent()
			var objects []client.Object
			if tt.deployment != nil {
				objects = append(objects, tt.deployment(agent.Namespace))
			}
			reconciler := setupReconciler(t, objects...)

			ready, err := reconciler.checkLiteLLMReady(
				context.Background(), agent, reconciler.Log,
			)
			require.NoError(t, err)
			assert.Equal(t, tt.wantReady, ready)
		})
	}
}

func TestCheckLiteLLMGateway_PassthroughWithoutOperatorManagedGateway(t *testing.T) {
	agent := newTestAgent()
	reconciler := setupReconciler(t)

	ready, err := reconciler.checkLiteLLMGateway(
		context.Background(), agent, newReadyMattermost(), reconciler.Log,
	)
	require.NoError(t, err)
	assert.True(t, ready)
}

func TestCheckLiteLLMGateway_UnconfiguredInstallationIsConfigIssue(t *testing.T) {
	agent := newTestAgent()
	agent.Spec.LLMGateway = operatorManagedGatewayConfig()
	reconciler := setupReconciler(t)

	ready, err := reconciler.checkLiteLLMGateway(
		context.Background(), agent, newReadyMattermost(), reconciler.Log,
	)
	require.Error(t, err)
	assert.False(t, ready)
	assert.True(t, errors.Is(err, errConfigIssue))
	assert.Contains(t, err.Error(), "spec.agents.llmGateway")
}
