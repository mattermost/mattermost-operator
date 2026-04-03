# Phase 3: Reconciler Integration — Prescriptive Plan

> **Milestone:** M3 — Agent Secret Protection (LiteLLM Gateway)
> **Target repo:** `~/workspace/worktrees/mattermost-operator-the-trail`
> **Phase:** 3 of 6
> **Depends on:** Phase 1 (types), Phase 2 (HTTP client)
> **Goal:** Wire LiteLLM lifecycle into the Agent reconciler. Deploy LiteLLM K8s resources, register models, create per-agent virtual keys.

---

## Context: What Already Exists

All files below are **already implemented** from prior work. Do NOT recreate them — only the new files listed in this plan need to be created/modified.

- `controllers/mattermost/agent/controller.go` — `AgentReconciler`, `SetupWithManager`, `Reconcile` loop. The insertion point for LiteLLM steps is **after `checkAgentBot` (line 116) and before `checkAgentServiceAccount` (line 119)**.
- `controllers/mattermost/agent/agent.go` — `checkAgentBot`, `checkAgentServiceAccount`, `checkAgentService`, `checkAgentDeployment`, `checkAgentNetworkPolicy`, `checkAgentHealth`
- `controllers/mattermost/agent/agent_test.go` — `newTestAgent()`, `setupScheme()`, `setupReconciler()` fixtures; existing tests
- `controllers/mattermost/agent/controller_test.go` — `TestReconcileAgent_*` tests
- `controllers/mattermost/agent/litellm_client.go` — `liteLLMClient`, all request/response types, `errKeyAliasExists`, `SanitizeMCPServerName` (Phase 2)
- `pkg/mattermost/litellm.go` — `GenerateLiteLLMConfigMap`, `GenerateLiteLLMDeployment`, `GenerateLiteLLMService`, `GenerateAgentLiteLLMKeySecret`, `LiteLLMServiceURL` (Phase 1)
- `pkg/resources/create_resources.go` — `ResourceHelper` with `CreateConfigMapIfNotExists`, `CreateSecretIfNotExists`, etc. (Phase 1)

**Module path:** `github.com/mattermost/mattermost-operator`

**Key constraint:** LiteLLM's ConfigMap, Deployment, and Service are **shared** resources — they are not owned by any single Agent. The caller must use `r.Client.Create(ctx, obj)` directly (after setting the last-applied annotation), NOT `r.Resources.Create(agent, obj, logger)` (which sets OwnerReference to the agent). Secrets for virtual keys ARE per-agent and use `r.Resources.CreateSecretIfNotExists(agent, ...)`.

---

## Task 3.1: Create `controllers/mattermost/agent/litellm.go`

**File:** `controllers/mattermost/agent/litellm.go`
**Action:** Create (new file)

This file contains all LiteLLM reconciler functions called from `controller.go`. It lives in package `agent` alongside `agent.go`.

```go
// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package agent

import (
	"context"
	"errors"
	"strings"

	"github.com/go-logr/logr"
	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	mattermostApp "github.com/mattermost/mattermost-operator/pkg/mattermost"
	pkgerrors "github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	objectMatcher "github.com/banzaicloud/k8s-objectmatcher/patch"
)

// ─── K8s resource reconciliation ──────────────────────────────────────────────

// checkLiteLLMDeployment ensures the LiteLLM Deployment and ConfigMap exist and are up to date.
// These are shared resources (no OwnerReference) — they are created directly via r.Client.
func (r *AgentReconciler) checkLiteLLMDeployment(ctx context.Context, agent *mmv1beta.Agent, reqLogger logr.Logger) error {
	om := agent.Spec.LLMGateway.OperatorManaged

	// Build provider env vars: one env var per provider, sourced from their K8s Secret.
	providerEnvVars := buildProviderEnvVars(om.LLMProviders)

	// ── ConfigMap ──────────────────────────────────────────────────────────────
	desiredCM := mattermostApp.GenerateLiteLLMConfigMap(agent.Namespace)

	foundCM := &corev1.ConfigMap{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: desiredCM.Name, Namespace: desiredCM.Namespace}, foundCM)
	if err != nil && k8sErrors.IsNotFound(err) {
		reqLogger.Info("Creating LiteLLM ConfigMap", "name", desiredCM.Name)
		if annotErr := defaultAnnotator.SetLastAppliedAnnotation(desiredCM); annotErr != nil {
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
		if annotErr := defaultAnnotator.SetLastAppliedAnnotation(desiredDeploy); annotErr != nil {
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

// checkLiteLLMService ensures the LiteLLM Service exists.
// This is a shared resource (no OwnerReference) — created directly via r.Client.
func (r *AgentReconciler) checkLiteLLMService(ctx context.Context, agent *mmv1beta.Agent, reqLogger logr.Logger) error {
	desiredSvc := mattermostApp.GenerateLiteLLMService(agent.Namespace)

	foundSvc := &corev1.Service{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: desiredSvc.Name, Namespace: desiredSvc.Namespace}, foundSvc)
	if err != nil && k8sErrors.IsNotFound(err) {
		reqLogger.Info("Creating LiteLLM Service", "name", desiredSvc.Name)
		if annotErr := defaultAnnotator.SetLastAppliedAnnotation(desiredSvc); annotErr != nil {
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

// checkLiteLLMReady returns (true, nil) when the LiteLLM Deployment has at least one ready replica.
// Returns (false, nil) when not yet ready — the caller should requeue.
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

// ─── Master key ────────────────────────────────────────────────────────────────

// getLiteLLMMasterKey reads the LiteLLM master key from the well-known Secret.
// The Secret must exist before the operator can call the LiteLLM management API.
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
		return "", pkgerrors.New("litellm master key secret exists but 'masterKey' value is empty")
	}
	return key, nil
}

// ─── Model registration ────────────────────────────────────────────────────────

// reconcileLiteLLMModels registers all configured LLM provider models via the LiteLLM management API.
// POST /model/new is safe to call multiple times (upserts), so no check-before-create is needed.
func (r *AgentReconciler) reconcileLiteLLMModels(ctx context.Context, agent *mmv1beta.Agent, litellmURL, masterKey string, reqLogger logr.Logger) error {
	client := newLiteLLMClient(litellmURL, masterKey)
	om := agent.Spec.LLMGateway.OperatorManaged

	for _, provider := range om.LLMProviders {
		// The API key reference uses os.environ/<VAR> syntax so LiteLLM reads it
		// from the Deployment's env vars at runtime (sourced from the provider Secret).
		apiKeyEnvRef := "os.environ/" + strings.ToUpper(provider.Name) + "_API_KEY"

		for _, model := range provider.Models {
			// LiteLLM model name format: "<provider>/<model>"
			// e.g. "anthropic/claude-3-5-sonnet-20241022"
			litellmModelName := provider.Name + "/" + model

			reqLogger.Info("Registering LiteLLM model", "model", litellmModelName)
			if err := client.registerModel(litellmModelName, litellmModelName, apiKeyEnvRef); err != nil {
				return pkgerrors.Wrapf(err, "failed to register model %s", litellmModelName)
			}
		}
	}

	return nil
}

// ─── Virtual key ───────────────────────────────────────────────────────────────

// reconcileLiteLLMVirtualKey creates a per-agent virtual key in LiteLLM and stores it in a K8s Secret.
// Idempotency: if the K8s Secret already exists, the function returns immediately without calling the API.
// If the LiteLLM API returns "key alias already exists" (errKeyAliasExists), the error is logged and
// the function continues — this handles the case where the Secret was deleted but the key still exists.
func (r *AgentReconciler) reconcileLiteLLMVirtualKey(ctx context.Context, agent *mmv1beta.Agent, litellmURL, masterKey string, mcpAccessGroups []string, reqLogger logr.Logger) error {
	// Idempotency check: if the Secret already exists, the key is already provisioned.
	existingSecret := &corev1.Secret{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      agent.LiteLLMKeySecretName(),
		Namespace: agent.Namespace,
	}, existingSecret)
	if err == nil {
		reqLogger.Info("LiteLLM key secret already exists, skipping key generation")
		return nil
	}
	if !k8sErrors.IsNotFound(err) {
		return pkgerrors.Wrap(err, "failed to check litellm key secret")
	}

	reqLogger.Info("Generating LiteLLM virtual key", "agent", agent.Name)

	// Build model list from all providers.
	var models []string
	for _, p := range agent.Spec.LLMGateway.OperatorManaged.LLMProviders {
		for _, m := range p.Models {
			models = append(models, p.Name+"/"+m)
		}
	}

	c := newLiteLLMClient(litellmURL, masterKey)
	keyResp, err := c.generateKey(liteLLMKeyRequest{
		KeyAlias: "agent-" + agent.Name + "-key",
		Models:   models,
		Metadata: map[string]string{
			"agent_name":  agent.Name,
			"managed_by":  "mattermost-operator",
			"namespace":   agent.Namespace,
		},
		ObjectPermission: liteLLMObjectPermission{
			MCPAccessGroups: mcpAccessGroups,
		},
	})
	if err != nil {
		if errors.Is(err, errKeyAliasExists) {
			// Key already exists in LiteLLM but Secret is missing.
			// Log and continue — the next reconcile will retry, or the user
			// can manually recreate the Secret. This is a recoverable state.
			reqLogger.Info("LiteLLM virtual key alias already exists (Secret was deleted?), skipping", "agent", agent.Name)
			return nil
		}
		return pkgerrors.Wrap(err, "failed to generate litellm virtual key")
	}

	// Store the virtual key in a K8s Secret owned by this Agent.
	desired := mattermostApp.GenerateAgentLiteLLMKeySecret(agent, keyResp.Key)
	if createErr := r.Resources.CreateSecretIfNotExists(agent, desired, reqLogger); createErr != nil {
		return pkgerrors.Wrap(createErr, "failed to create litellm key secret")
	}

	reqLogger.Info("LiteLLM virtual key provisioned", "agent", agent.Name, "keyAlias", keyResp.KeyAlias)
	return nil
}

// ─── Helpers ───────────────────────────────────────────────────────────────────

// buildProviderEnvVars constructs the env var list for the LiteLLM Deployment.
// Each provider gets one env var: <PROVIDER_NAME>_API_KEY sourced from its K8s Secret.
// LiteLLM reads these via "os.environ/<VAR>" references in model registration.
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

// defaultAnnotator is re-exported from pkg/resources via the package-level variable.
// It is used here to set last-applied annotations on shared (ownerless) resources
// before calling r.Client.Create directly.
// NOTE: This references the package-level var defined in pkg/resources/create_resources.go.
// Since litellm.go is in package agent (not package resources), we must use the annotator
// from the resources package. Import it via a helper or duplicate the var here.
// Use the approach below: declare a package-level annotator in this file.
```

**Important note on `defaultAnnotator`:** The `pkg/resources` package declares `defaultAnnotator` as a package-level variable but it is unexported. The `r.Resources.Update()` method already uses it internally. For the `Create` calls on shared resources, we need to set the annotation ourselves. Add a package-level variable in `litellm.go`:

```go
// litellmAnnotator mirrors the annotator used by ResourceHelper so that
// shared LiteLLM resources (no OwnerReference) get the last-applied annotation
// needed by objectMatcher for future Update() calls.
var litellmAnnotator = objectMatcher.NewAnnotator("mattermost.com/last-applied")
```

Replace all `defaultAnnotator.SetLastAppliedAnnotation(...)` calls in the functions above with `litellmAnnotator.SetLastAppliedAnnotation(...)`.

### Complete corrected file

```go
// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package agent

import (
	"context"
	"errors"
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
// Safe to call every reconcile — LiteLLM upserts on repeated calls.
func (r *AgentReconciler) reconcileLiteLLMModels(ctx context.Context, agent *mmv1beta.Agent, litellmURL, masterKey string, reqLogger logr.Logger) error {
	c := newLiteLLMClient(litellmURL, masterKey)
	for _, provider := range agent.Spec.LLMGateway.OperatorManaged.LLMProviders {
		// os.environ/<VAR> tells LiteLLM to read the key from the pod's env at runtime.
		apiKeyEnvRef := "os.environ/" + strings.ToUpper(provider.Name) + "_API_KEY"
		for _, model := range provider.Models {
			// LiteLLM model name: "<provider>/<model>", e.g. "anthropic/claude-3-5-sonnet-20241022"
			litellmModel := provider.Name + "/" + model
			reqLogger.Info("Registering LiteLLM model", "model", litellmModel)
			if err := c.registerModel(litellmModel, litellmModel, apiKeyEnvRef); err != nil {
				return pkgerrors.Wrapf(err, "failed to register model %s", litellmModel)
			}
		}
	}
	return nil
}

// reconcileLiteLLMVirtualKey creates a per-agent virtual key and stores it in a K8s Secret.
// Idempotency: if the Secret already exists, the function returns immediately (no API call).
// If the LiteLLM API returns errKeyAliasExists, the error is logged and swallowed — the
// Secret was likely deleted after the key was created; the user must recreate it.
func (r *AgentReconciler) reconcileLiteLLMVirtualKey(ctx context.Context, agent *mmv1beta.Agent, litellmURL, masterKey string, mcpAccessGroups []string, reqLogger logr.Logger) error {
	// Idempotency: Secret existence means the key is already provisioned.
	existingSecret := &corev1.Secret{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      agent.LiteLLMKeySecretName(),
		Namespace: agent.Namespace,
	}, existingSecret)
	if err == nil {
		reqLogger.Info("LiteLLM key secret already exists, skipping", "agent", agent.Name)
		return nil
	}
	if !k8sErrors.IsNotFound(err) {
		return pkgerrors.Wrap(err, "failed to check litellm key secret")
	}

	// Build model list for the key's allowed models.
	var models []string
	for _, p := range agent.Spec.LLMGateway.OperatorManaged.LLMProviders {
		for _, m := range p.Models {
			models = append(models, p.Name+"/"+m)
		}
	}

	c := newLiteLLMClient(litellmURL, masterKey)
	keyResp, err := c.generateKey(liteLLMKeyRequest{
		KeyAlias: "agent-" + agent.Name + "-key",
		Models:   models,
		Metadata: map[string]string{
			"agent_name": agent.Name,
			"managed_by": "mattermost-operator",
			"namespace":  agent.Namespace,
		},
		ObjectPermission: liteLLMObjectPermission{
			MCPAccessGroups: mcpAccessGroups,
		},
	})
	if err != nil {
		if errors.Is(err, errKeyAliasExists) {
			reqLogger.Info("LiteLLM key alias already exists (Secret missing?), skipping", "agent", agent.Name)
			return nil
		}
		return pkgerrors.Wrap(err, "failed to generate litellm virtual key")
	}

	desired := mattermostApp.GenerateAgentLiteLLMKeySecret(agent, keyResp.Key)
	if err := r.Resources.CreateSecretIfNotExists(agent, desired, reqLogger); err != nil {
		return pkgerrors.Wrap(err, "failed to create litellm key secret")
	}

	reqLogger.Info("LiteLLM virtual key provisioned", "agent", agent.Name, "keyAlias", keyResp.KeyAlias)
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
```

---

## Task 3.2: Modify `controllers/mattermost/agent/controller.go`

**File:** `controllers/mattermost/agent/controller.go`
**Action:** Two edits — add `Owns(&corev1.ConfigMap{})` and insert the LiteLLM reconcile block.

### Edit A: Add ConfigMap to `SetupWithManager`

Current `SetupWithManager` (lines 43–52):

```go
func (r *AgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mmv1beta.Agent{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&corev1.Secret{}).
		Owns(&networkingv1.NetworkPolicy{}).
		Complete(r)
}
```

Add `Owns(&corev1.ConfigMap{})` after `Owns(&corev1.Secret{})`:

```go
func (r *AgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mmv1beta.Agent{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&networkingv1.NetworkPolicy{}).
		Complete(r)
}
```

### Edit B: Insert LiteLLM reconcile block in `Reconcile()`

**Exact insertion point:** After `checkAgentBot` block (lines 111–116) and before `checkAgentServiceAccount` (line 118).

Current code at the insertion point:

```go
	// Bot provisioning.
	err = r.checkAgentBot(ctx, agent, adminToken, reqLogger)
	if err != nil {
		r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
		return reconcile.Result{}, err
	}

	// ServiceAccount
	err = r.checkAgentServiceAccount(ctx, agent, reqLogger)
```

Change to:

```go
	// Bot provisioning.
	err = r.checkAgentBot(ctx, agent, adminToken, reqLogger)
	if err != nil {
		r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
		return reconcile.Result{}, err
	}

	// LiteLLM gateway (operator-managed only).
	if agent.Spec.LLMGateway != nil && agent.Spec.LLMGateway.OperatorManaged != nil {
		if err = r.checkLiteLLMDeployment(ctx, agent, reqLogger); err != nil {
			r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
			return reconcile.Result{}, err
		}

		if err = r.checkLiteLLMService(ctx, agent, reqLogger); err != nil {
			r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
			return reconcile.Result{}, err
		}

		ready, err := r.checkLiteLLMReady(ctx, agent, reqLogger)
		if err != nil {
			r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
			return reconcile.Result{}, err
		}
		if !ready {
			return reconcile.Result{RequeueAfter: mattermostNotReadyDelay}, nil
		}

		masterKey, err := r.getLiteLLMMasterKey(ctx, agent.Namespace)
		if err != nil {
			r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
			return reconcile.Result{}, err
		}

		litellmURL := mattermostApp.LiteLLMServiceURL(agent.Namespace)

		if err = r.reconcileLiteLLMModels(ctx, agent, litellmURL, masterKey, reqLogger); err != nil {
			r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
			return reconcile.Result{}, err
		}

		// MCP servers are reconciled in Phase 4. For now, pass empty access groups.
		var mcpAccessGroups []string
		if err = r.reconcileLiteLLMVirtualKey(ctx, agent, litellmURL, masterKey, mcpAccessGroups, reqLogger); err != nil {
			r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
			return reconcile.Result{}, err
		}
	}

	// ServiceAccount
	err = r.checkAgentServiceAccount(ctx, agent, reqLogger)
```

**New import needed in `controller.go`:** Add `mattermostApp "github.com/mattermost/mattermost-operator/pkg/mattermost"` to the import block if not already present.

Check the current imports in `controller.go` — `mattermostApp` is not imported there (it is in `agent.go`). The `LiteLLMServiceURL` call requires it. Add to the import block:

```go
	mattermostApp "github.com/mattermost/mattermost-operator/pkg/mattermost"
```

---

## Task 3.3: Create `controllers/mattermost/agent/litellm_test.go`

**File:** `controllers/mattermost/agent/litellm_test.go`
**Action:** Create (new file)

Uses the same `newTestAgent()`, `setupReconciler()`, and `setupScheme()` fixtures from `agent_test.go` (same package). Uses `httptest.NewServer` for the LiteLLM API mock.

```go
// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	"github.com/mattermost/blubr"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// newTestAgentWithLLMGateway returns a test Agent with OperatorManaged LLMGateway configured.
func newTestAgentWithLLMGateway() *mmv1beta.Agent {
	a := newTestAgent()
	a.Spec.LLMGateway = &mmv1beta.LLMGatewayConfig{
		OperatorManaged: &mmv1beta.OperatorManagedLLMGateway{
			Image: mmv1beta.AgentLiteLLMDefaultImage,
			LLMProviders: []mmv1beta.LLMProvider{
				{
					Name:   "anthropic",
					Secret: "anthropic-key",
					Models: []string{"claude-3-5-sonnet-20241022"},
				},
			},
		},
	}
	return a
}

func testLogger() logr.Logger {
	logSink := blubr.InitLogger(logrus.NewEntry(logrus.New()))
	return logr.New(logSink.WithName("test"))
}

// ─── checkLiteLLMDeployment ────────────────────────────────────────────────────

func TestCheckLiteLLMDeployment_CreatesResources(t *testing.T) {
	agent := newTestAgentWithLLMGateway()
	_ = agent.SetDefaults()

	reconciler, _ := setupReconciler(t, agent)
	logger := testLogger()

	err := reconciler.checkLiteLLMDeployment(context.TODO(), agent, logger)
	require.NoError(t, err)

	// Verify ConfigMap was created.
	cm := &corev1.ConfigMap{}
	err = reconciler.Client.Get(context.TODO(), types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMConfigMapName,
		Namespace: agent.Namespace,
	}, cm)
	require.NoError(t, err)
	assert.Contains(t, cm.Data["config.yaml"], "general_settings")

	// Verify Deployment was created.
	deploy := &appsv1.Deployment{}
	err = reconciler.Client.Get(context.TODO(), types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMDeploymentName,
		Namespace: agent.Namespace,
	}, deploy)
	require.NoError(t, err)
	assert.Equal(t, mmv1beta.AgentLiteLLMDefaultImage, deploy.Spec.Template.Spec.Containers[0].Image)

	// Verify provider env var was injected into Deployment.
	var hasAnthropicKey bool
	for _, env := range deploy.Spec.Template.Spec.Containers[0].Env {
		if env.Name == "ANTHROPIC_API_KEY" {
			hasAnthropicKey = true
			assert.Equal(t, "anthropic-key", env.ValueFrom.SecretKeyRef.Name)
			assert.Equal(t, "apiKey", env.ValueFrom.SecretKeyRef.Key)
		}
	}
	assert.True(t, hasAnthropicKey, "expected ANTHROPIC_API_KEY env var in LiteLLM Deployment")
}

func TestCheckLiteLLMDeployment_Idempotent(t *testing.T) {
	agent := newTestAgentWithLLMGateway()
	_ = agent.SetDefaults()

	reconciler, _ := setupReconciler(t, agent)
	logger := testLogger()

	// First call creates resources.
	err := reconciler.checkLiteLLMDeployment(context.TODO(), agent, logger)
	require.NoError(t, err)

	// Second call should not error (idempotent).
	err = reconciler.checkLiteLLMDeployment(context.TODO(), agent, logger)
	require.NoError(t, err)
}

// ─── checkLiteLLMService ──────────────────────────────────────────────────────

func TestCheckLiteLLMService_Creates(t *testing.T) {
	agent := newTestAgentWithLLMGateway()
	_ = agent.SetDefaults()

	reconciler, _ := setupReconciler(t, agent)
	logger := testLogger()

	err := reconciler.checkLiteLLMService(context.TODO(), agent, logger)
	require.NoError(t, err)

	svc := &corev1.Service{}
	err = reconciler.Client.Get(context.TODO(), types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMServiceName,
		Namespace: agent.Namespace,
	}, svc)
	require.NoError(t, err)
	assert.Equal(t, mmv1beta.AgentLiteLLMPort, svc.Spec.Ports[0].Port)
}

// ─── checkLiteLLMReady ────────────────────────────────────────────────────────

func TestCheckLiteLLMReady_NotReady(t *testing.T) {
	agent := newTestAgentWithLLMGateway()

	// Pre-create the LiteLLM Deployment with ReadyReplicas=0.
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmv1beta.AgentLiteLLMDeploymentName,
			Namespace: agent.Namespace,
		},
		Status: appsv1.DeploymentStatus{ReadyReplicas: 0},
	}

	reconciler, _ := setupReconciler(t, agent, deploy)
	logger := testLogger()

	ready, err := reconciler.checkLiteLLMReady(context.TODO(), agent, logger)
	require.NoError(t, err)
	assert.False(t, ready)
}

func TestCheckLiteLLMReady_Ready(t *testing.T) {
	agent := newTestAgentWithLLMGateway()

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmv1beta.AgentLiteLLMDeploymentName,
			Namespace: agent.Namespace,
		},
		Status: appsv1.DeploymentStatus{ReadyReplicas: 1},
	}

	reconciler, _ := setupReconciler(t, agent, deploy)
	logger := testLogger()

	ready, err := reconciler.checkLiteLLMReady(context.TODO(), agent, logger)
	require.NoError(t, err)
	assert.True(t, ready)
}

// ─── reconcileLiteLLMModels ───────────────────────────────────────────────────

func TestReconcileLiteLLMModels_CallsAPI(t *testing.T) {
	agent := newTestAgentWithLLMGateway()
	_ = agent.SetDefaults()

	var registeredModels []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "POST", r.Method)
		require.Equal(t, "/model/new", r.URL.Path)

		var body liteLLMModelRequest
		err := json.NewDecoder(r.Body).Decode(&body)
		require.NoError(t, err)
		registeredModels = append(registeredModels, body.ModelName)

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"model_id": "m1"})
	}))
	defer srv.Close()

	reconciler, _ := setupReconciler(t, agent)
	logger := testLogger()

	err := reconciler.reconcileLiteLLMModels(context.TODO(), agent, srv.URL, "master-key", logger)
	require.NoError(t, err)

	// One model: "anthropic/claude-3-5-sonnet-20241022"
	require.Len(t, registeredModels, 1)
	assert.Equal(t, "anthropic/claude-3-5-sonnet-20241022", registeredModels[0])
}

// ─── reconcileLiteLLMVirtualKey ───────────────────────────────────────────────

func TestReconcileLiteLLMVirtualKey_CreatesSecret(t *testing.T) {
	agent := newTestAgentWithLLMGateway()
	_ = agent.SetDefaults()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "POST", r.Method)
		require.Equal(t, "/key/generate", r.URL.Path)

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(liteLLMKeyResponse{
			Key:      "sk-virtual-key-abc",
			KeyAlias: "agent-test-agent-key",
			Token:    "tok-hash",
		})
	}))
	defer srv.Close()

	reconciler, _ := setupReconciler(t, agent)
	logger := testLogger()

	err := reconciler.reconcileLiteLLMVirtualKey(context.TODO(), agent, srv.URL, "master-key", []string{}, logger)
	require.NoError(t, err)

	// Verify the Secret was created with the virtual key value.
	secret := &corev1.Secret{}
	err = reconciler.Client.Get(context.TODO(), types.NamespacedName{
		Name:      agent.LiteLLMKeySecretName(),
		Namespace: agent.Namespace,
	}, secret)
	require.NoError(t, err)
	assert.Equal(t, "sk-virtual-key-abc", string(secret.Data["apiKey"]))
}

func TestReconcileLiteLLMVirtualKey_Idempotent(t *testing.T) {
	agent := newTestAgentWithLLMGateway()
	_ = agent.SetDefaults()

	// Pre-create the key Secret.
	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agent.LiteLLMKeySecretName(),
			Namespace: agent.Namespace,
		},
		Data: map[string][]byte{"apiKey": []byte("existing-key")},
	}

	apiCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalled = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	reconciler, _ := setupReconciler(t, agent, existingSecret)
	logger := testLogger()

	err := reconciler.reconcileLiteLLMVirtualKey(context.TODO(), agent, srv.URL, "master-key", []string{}, logger)
	require.NoError(t, err)
	assert.False(t, apiCalled, "API should not be called when Secret already exists")
}
```

---

## Task 3.4: Run tests

```bash
cd /Users/nickmisasi/workspace/worktrees/mattermost-operator-the-trail

# Run new litellm reconciler tests only
go test ./controllers/mattermost/agent/... -run TestCheckLiteLLM -v
go test ./controllers/mattermost/agent/... -run TestReconcileLiteLLM -v

# Run full controller package (must not break existing tests)
make unittest
```

Expected: all 8 new tests pass, all existing tests pass.

---

## Definition of Done

- [ ] `controllers/mattermost/agent/litellm.go` compiles — all 7 functions implemented
- [ ] `controllers/mattermost/agent/litellm_test.go` compiles — all 8 tests pass
- [ ] `controller.go` has `Owns(&corev1.ConfigMap{})` in `SetupWithManager`
- [ ] `controller.go` has LiteLLM reconcile block inserted after `checkAgentBot`
- [ ] `make unittest` passes (0 failures — no regression in existing tests)
- [ ] `make build` compiles successfully

---

## Precise Change Map

| File | Action | Lines / Location | Summary |
|------|--------|-----------------|---------|
| `controllers/mattermost/agent/litellm.go` | Create | New file | 7 functions: `checkLiteLLMDeployment`, `checkLiteLLMService`, `checkLiteLLMReady`, `getLiteLLMMasterKey`, `reconcileLiteLLMModels`, `reconcileLiteLLMVirtualKey`, `buildProviderEnvVars` |
| `controllers/mattermost/agent/litellm_test.go` | Create | New file | 8 tests using `httptest.NewServer` and fake K8s client |
| `controllers/mattermost/agent/controller.go` | Modify | Line 50: add `Owns(&corev1.ConfigMap{})` | Watch ConfigMap objects |
| `controllers/mattermost/agent/controller.go` | Modify | After line 116 (post-checkAgentBot): insert LiteLLM block | 40-line conditional block for LiteLLM reconciliation |
| `controllers/mattermost/agent/controller.go` | Modify | Import block | Add `mattermostApp "github.com/mattermost/mattermost-operator/pkg/mattermost"` |

---

## Design Notes

**`litellmAnnotator` vs `defaultAnnotator`:** The `pkg/resources` package uses its own `defaultAnnotator` (unexported). When we call `r.Resources.Update(current, desired, ...)` on shared LiteLLM resources, the update comparison uses the `mattermost.com/last-applied` annotation set by that package's annotator. Since we use the same annotation key (`mattermost.com/last-applied`) in `litellmAnnotator`, the Create → Update cycle is consistent. The annotation key string `"mattermost.com/last-applied"` must match `lastAppliedConfig` in `create_resources.go` (line 24: `const lastAppliedConfig = "mattermost.com/last-applied"`).

**Phase 4 stub:** The `reconcileLiteLLMVirtualKey` call passes `var mcpAccessGroups []string` (empty slice). Phase 4 replaces this with the actual MCP access groups returned by `reconcileLiteLLMMCPServers`. The empty slice is valid — a key with no MCP access groups can still make LLM calls; it just has no MCP tool access.

**Requeue delay for LiteLLM not ready:** Uses `mattermostNotReadyDelay` (15 seconds) — the same constant used for Mattermost not ready. This is appropriate since both are "wait for a dependency to become ready" situations.

**No `context.Context` threading through `liteLLMClient`:** The HTTP client calls do not accept context (matching the existing `checkAgentBot` pattern). LiteLLM API calls during reconcile are fast and short-lived enough that timeouts are not a current concern.

---

## Implementation Summary

**Completed:** 2026-03-24

### Files Created
- `controllers/mattermost/agent/litellm.go` — 7 functions: `checkLiteLLMDeployment` (ConfigMap + Deployment), `checkLiteLLMService`, `checkLiteLLMReady`, `getLiteLLMMasterKey`, `reconcileLiteLLMModels`, `reconcileLiteLLMVirtualKey`, `buildProviderEnvVars`. Package-level `litellmAnnotator` uses the same annotation key as `pkg/resources` for consistent update diffing.
- `controllers/mattermost/agent/litellm_test.go` — 8 tests: all pass. Uses `httptest.NewServer` and fake K8s client via `setupReconciler`. Covers create, idempotency, readiness state, model registration, and virtual key provisioning.

### Files Modified
- `controllers/mattermost/agent/controller.go`:
  - Added `mattermostApp "github.com/mattermost/mattermost-operator/pkg/mattermost"` import
  - Added `Owns(&corev1.ConfigMap{})` to `SetupWithManager`
  - Inserted 40-line LiteLLM reconcile block after `checkAgentBot`, before `checkAgentServiceAccount`. The block gates on `agent.Spec.LLMGateway != nil && agent.Spec.LLMGateway.OperatorManaged != nil`. Passes `nil` for `mcpAccessGroups` (Phase 4 stub).

### Test Results
- All 8 new Phase 3 tests pass.
- Pre-existing `TestGenerateRBACResources_V1Beta` failure is unrelated to LiteLLM work (pre-existing).
- No regressions in `controllers/mattermost/agent/` package.
