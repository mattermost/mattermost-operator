// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package agent

import (
	"context"

	objectMatcher "github.com/banzaicloud/k8s-objectmatcher/patch"
	"github.com/go-logr/logr"
	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	mattermostApp "github.com/mattermost/mattermost-operator/pkg/mattermost"
	pkgerrors "github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

// litellmAnnotator sets the last-applied annotation used by objectMatcher on
// shared LiteLLM resources that have no OwnerReference (ConfigMap, Deployment, Service).
// The annotation key matches the one in pkg/resources/create_resources.go so that
// r.Resources.Update() can correctly diff shared resources created here.
var litellmAnnotator = objectMatcher.NewAnnotator("mattermost.com/last-applied")

// checkLiteLLMDeployment ensures the LiteLLM ConfigMap and Deployment exist and are up to date.
// These are shared resources — no OwnerReference is set, so r.Client.Create is used directly.
func (r *AgentReconciler) checkLiteLLMDeployment(ctx context.Context, agent *mmv1beta.Agent, reqLogger logr.Logger) error {
	om := agent.Spec.LLMGateway.OperatorManaged

	// ── ConfigMap ──────────────────────────────────────────────────────────────
	desiredCM := mattermostApp.GenerateLiteLLMConfigMap(agent.Namespace)
	foundCM := &corev1.ConfigMap{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: desiredCM.Name, Namespace: desiredCM.Namespace}, foundCM)
	if err != nil && k8sErrors.IsNotFound(err) {
		reqLogger.Info("Creating LiteLLM ConfigMap", "name", desiredCM.Name)
		if annotErr := litellmAnnotator.SetLastAppliedAnnotation(desiredCM); annotErr != nil {
			return pkgerrors.Wrap(annotErr, "failed to annotate litellm configmap")
		}
		if createErr := r.Client.Create(ctx, desiredCM); createErr != nil {
			return pkgerrors.Wrap(createErr, "failed to create litellm configmap")
		}
	} else if err != nil {
		return pkgerrors.Wrap(err, "failed to get litellm configmap")
	} else {
		if updateErr := r.Resources.Update(foundCM, desiredCM, reqLogger); updateErr != nil {
			return pkgerrors.Wrap(updateErr, "failed to update litellm configmap")
		}
	}

	// ── Deployment ─────────────────────────────────────────────────────────────
	desiredDeploy := mattermostApp.GenerateLiteLLMDeployment(agent.Namespace, om.Image)
	foundDeploy := &appsv1.Deployment{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: desiredDeploy.Name, Namespace: desiredDeploy.Namespace}, foundDeploy)
	if err != nil && k8sErrors.IsNotFound(err) {
		reqLogger.Info("Creating LiteLLM Deployment", "name", desiredDeploy.Name)
		if annotErr := litellmAnnotator.SetLastAppliedAnnotation(desiredDeploy); annotErr != nil {
			return pkgerrors.Wrap(annotErr, "failed to annotate litellm deployment")
		}
		if createErr := r.Client.Create(ctx, desiredDeploy); createErr != nil {
			return pkgerrors.Wrap(createErr, "failed to create litellm deployment")
		}
	} else if err != nil {
		return pkgerrors.Wrap(err, "failed to get litellm deployment")
	} else {
		if updateErr := r.Resources.Update(foundDeploy, desiredDeploy, reqLogger); updateErr != nil {
			return pkgerrors.Wrap(updateErr, "failed to update litellm deployment")
		}
	}

	return nil
}

// checkLiteLLMService ensures the LiteLLM Service exists and is up to date.
// Shared resource — no OwnerReference.
func (r *AgentReconciler) checkLiteLLMService(ctx context.Context, agent *mmv1beta.Agent, reqLogger logr.Logger) error {
	desiredSvc := mattermostApp.GenerateLiteLLMService(agent.Namespace)
	foundSvc := &corev1.Service{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: desiredSvc.Name, Namespace: desiredSvc.Namespace}, foundSvc)
	if err != nil && k8sErrors.IsNotFound(err) {
		reqLogger.Info("Creating LiteLLM Service", "name", desiredSvc.Name)
		if annotErr := litellmAnnotator.SetLastAppliedAnnotation(desiredSvc); annotErr != nil {
			return pkgerrors.Wrap(annotErr, "failed to annotate litellm service")
		}
		if createErr := r.Client.Create(ctx, desiredSvc); createErr != nil {
			return pkgerrors.Wrap(createErr, "failed to create litellm service")
		}
		return nil
	} else if err != nil {
		return pkgerrors.Wrap(err, "failed to get litellm service")
	}

	return r.Resources.Update(foundSvc, desiredSvc, reqLogger)
}

// checkLiteLLMReady returns (true, nil) when LiteLLM has at least one ready replica.
// Returns (false, nil) — not an error — when not yet ready; the caller requeues.
func (r *AgentReconciler) checkLiteLLMReady(ctx context.Context, agent *mmv1beta.Agent, reqLogger logr.Logger) (bool, error) {
	deploy := &appsv1.Deployment{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMDeploymentName,
		Namespace: agent.Namespace,
	}, deploy)
	if err != nil {
		return false, pkgerrors.Wrap(err, "failed to get litellm deployment for readiness check")
	}
	if deploy.Status.ReadyReplicas < 1 {
		reqLogger.Info("LiteLLM not ready yet, will requeue", "readyReplicas", deploy.Status.ReadyReplicas)
		return false, nil
	}
	return true, nil
}

