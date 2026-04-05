// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package agent

import (
	"context"
	"strings"

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
	providerEnvVars := buildProviderEnvVars(om.LLMProviders)

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
	desiredDeploy := mattermostApp.GenerateLiteLLMDeployment(agent.Namespace, om.Image, providerEnvVars)
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

// getLiteLLMMasterKey reads the master key from the well-known Secret.
func (r *AgentReconciler) getLiteLLMMasterKey(ctx context.Context, namespace string) (string, error) {
	secret := &corev1.Secret{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMMasterKeySecretName,
		Namespace: namespace,
	}, secret)
	if err != nil {
		return "", pkgerrors.Wrap(err, "failed to get litellm master key secret")
	}
	key := string(secret.Data["masterKey"])
	if key == "" {
		return "", pkgerrors.New("litellm master key secret has empty 'masterKey' value")
	}
	return key, nil
}

// reconcileLiteLLMModels registers all provider models via POST /model/new.
//
// Idempotency: POST /model/new is NOT idempotent — it creates duplicates.
// This function lists existing models first and only creates missing ones.
func (r *AgentReconciler) reconcileLiteLLMModels(ctx context.Context, agent *mmv1beta.Agent, litellmURL, masterKey string, reqLogger logr.Logger) error {
	providers := agent.Spec.LLMGateway.OperatorManaged.LLMProviders
	if len(providers) == 0 {
		return nil
	}

	c := newLiteLLMClient(litellmURL, masterKey)

	// Build a set of existing model names to check before creating.
	existing, err := c.listModels()
	if err != nil {
		return pkgerrors.Wrap(err, "failed to list existing models")
	}
	existingByName := make(map[string]struct{}, len(existing))
	for _, m := range existing {
		existingByName[m.ModelName] = struct{}{}
	}

	for _, provider := range providers {
		// os.environ/<VAR> tells LiteLLM to read the key from the pod's env at runtime.
		apiKeyEnvRef := "os.environ/" + strings.ToUpper(provider.Name) + "_API_KEY"
		for _, model := range provider.Models {
			// Register with the prefixed name: "<provider>/<model>" for OpenAI-compat calls.
			litellmModel := provider.Name + "/" + model

			if _, found := existingByName[litellmModel]; found {
				reqLogger.Info("Model already registered, skipping", "model", litellmModel)
			} else {
				reqLogger.Info("Registering LiteLLM model", "model", litellmModel)
				if err := c.registerModel(litellmModel, litellmModel, apiKeyEnvRef); err != nil {
					return pkgerrors.Wrapf(err, "failed to register model %s", litellmModel)
				}
			}

			// Also register with the bare model name (no provider prefix).
			// LiteLLM's Anthropic passthrough endpoint (/v1/messages) looks up
			// models by the bare name, so agents using the Anthropic SDK need this.
			if _, found := existingByName[model]; found {
				reqLogger.Info("Model already registered (bare name), skipping", "model", model)
			} else {
				reqLogger.Info("Registering LiteLLM model (bare name)", "model", model)
				if err := c.registerModel(model, litellmModel, apiKeyEnvRef); err != nil {
					return pkgerrors.Wrapf(err, "failed to register model %s (bare name)", model)
				}
			}
		}
	}
	return nil
}

// buildProviderEnvVars constructs env vars for the LiteLLM Deployment.
// Each provider contributes one env var: <PROVIDER_NAME>_API_KEY from its K8s Secret.
func buildProviderEnvVars(providers []mmv1beta.LLMProvider) []corev1.EnvVar {
	vars := make([]corev1.EnvVar, 0, len(providers))
	for _, p := range providers {
		vars = append(vars, corev1.EnvVar{
			Name: strings.ToUpper(p.Name) + "_API_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: p.Secret},
					Key:                  "apiKey",
				},
			},
		})
	}
	return vars
}

// ─── MCP server registration ───────────────────────────────────────────────────

// reconcileLiteLLMMCPServers registers all MCP servers configured in agent.Spec.MCPServers
// into the LiteLLM instance, resolving credentials from K8s Secrets.
//
// Returns the list of MCP access group names that should be granted to this agent's
// virtual key. The caller passes this list to reconcileLiteLLMVirtualKey.
//
// Idempotency: POST /v1/mcp/server is NOT idempotent — it creates duplicates.
// This function lists existing servers first and only creates missing ones.
func (r *AgentReconciler) reconcileLiteLLMMCPServers(ctx context.Context, agent *mmv1beta.Agent, litellmURL, masterKey string, reqLogger logr.Logger) ([]string, error) {
	if len(agent.Spec.MCPServers) == 0 {
		return nil, nil
	}

	c := newLiteLLMClient(litellmURL, masterKey)

	// Build a set of existing server names to check before creating.
	existing, err := c.listMCPServers()
	if err != nil {
		return nil, pkgerrors.Wrap(err, "failed to list existing mcp servers")
	}
	existingByName := make(map[string]struct{}, len(existing))
	for _, srv := range existing {
		existingByName[srv.ServerName] = struct{}{}
	}

	var accessGroups []string

	for _, mcpServer := range agent.Spec.MCPServers {
		sanitizedName := SanitizeMCPServerName(mcpServer.Name)

		// Determine the MCP access group for this server.
		// If the spec provides an explicit MCPAccessGroup, use it as-is.
		// Otherwise, derive a per-agent name: "<agentName>_<sanitizedServerName>".
		accessGroup := mcpServer.MCPAccessGroup
		if accessGroup == "" {
			accessGroup = agent.Name + "_" + sanitizedName
		}
		accessGroups = append(accessGroups, accessGroup)

		// Skip registration if the server is already present in LiteLLM.
		if _, found := existingByName[sanitizedName]; found {
			reqLogger.Info("MCP server already registered, skipping", "server", sanitizedName)
			continue
		}

		// Resolve credential from K8s Secret (optional — some MCP servers are unauthenticated).
		credentialValue := ""
		if mcpServer.CredentialSecret != "" {
			credSecret := &corev1.Secret{}
			err := r.Client.Get(ctx, types.NamespacedName{
				Name:      mcpServer.CredentialSecret,
				Namespace: agent.Namespace,
			}, credSecret)
			if err != nil {
				return nil, pkgerrors.Wrapf(err, "failed to get credential secret %q for mcp server %q", mcpServer.CredentialSecret, mcpServer.Name)
			}
			credentialValue = string(credSecret.Data["apiKey"])
		}

		// Build the registration request.
		req := liteLLMMCPServerRequest{
			ServerName:      sanitizedName,
			URL:             mcpServer.URL,
			Transport:       "http",
			MCPAccessGroups: []string{accessGroup},
		}
		if credentialValue != "" {
			req.AuthType = "bearer_token"
			req.Credentials = liteLLMCredentials{AuthValue: credentialValue}
		}

		reqLogger.Info("Registering MCP server", "server", sanitizedName, "accessGroup", accessGroup)
		if _, err := c.registerMCPServer(req); err != nil {
			return nil, pkgerrors.Wrapf(err, "failed to register mcp server %q", sanitizedName)
		}
	}

	return accessGroups, nil
}
