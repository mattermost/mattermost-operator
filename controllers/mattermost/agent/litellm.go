// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package agent

// The LiteLLM gateway (Deployment/Service/master-key Secret) is owned and
// reconciled by the Mattermost controller (spec.agents.llmGateway). Agents
// that opt in via spec.llmGateway.operatorManaged only gate on its readiness.

import (
	"context"

	"github.com/go-logr/logr"
	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	pkgerrors "github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

// checkLiteLLMReady reports whether LiteLLM has at least one ready replica.
// A missing Deployment is not-ready: the Mattermost controller may not have
// created it yet.
func (r *AgentReconciler) checkLiteLLMReady(ctx context.Context, agent *mmv1beta.Agent, reqLogger logr.Logger) (bool, error) {
	deploy := &appsv1.Deployment{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMDeploymentName,
		Namespace: agent.Namespace,
	}, deploy)
	if k8sErrors.IsNotFound(err) {
		reqLogger.Info("LiteLLM deployment not created yet, will requeue")
		return false, nil
	}
	if err != nil {
		return false, pkgerrors.Wrap(err, "failed to get litellm deployment for readiness check")
	}
	if deploy.Status.ReadyReplicas < 1 {
		reqLogger.Info("LiteLLM not ready yet, will requeue", "readyReplicas", deploy.Status.ReadyReplicas)
		return false, nil
	}
	return true, nil
}
