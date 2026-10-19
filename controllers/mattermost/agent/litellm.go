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
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

// checkLiteLLMGateway gates reconciliation on the LiteLLM gateway for agents
// that use the operator-managed gateway; other agents pass through. An
// installation that does not configure spec.agents.llmGateway is a config
// issue (errConfigIssue), not a transient failure.
func (r *AgentReconciler) checkLiteLLMGateway(ctx context.Context, agent *mmv1beta.Agent, mm *mmv1beta.Mattermost, reqLogger logr.Logger) (bool, error) {
	if !agent.HasOperatorManagedGateway() {
		return true, nil
	}

	if !mm.OperatorManagedLLMGatewayEnabled() {
		return false, errors.Wrapf(errConfigIssue, "agent uses llmGateway.operatorManaged but Mattermost installation %q does not configure spec.agents.llmGateway", mm.Name)
	}

	return r.checkLiteLLMReady(ctx, agent, reqLogger)
}

// checkLiteLLMReady reports whether the LiteLLM Deployment finished its
// rollout. A missing Deployment is not-ready: the Mattermost controller may
// not have created it yet.
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
		return false, errors.Wrap(err, "failed to get litellm deployment for readiness check")
	}

	if err := checkDeploymentRolloutComplete(deploy); err != nil {
		reqLogger.Info("LiteLLM not ready yet, will requeue", "reason", err.Error())
		return false, nil
	}
	return true, nil
}
