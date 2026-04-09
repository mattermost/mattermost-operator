# Phase O1: Remove LiteLLM Business Logic from Reconciler

> **Status:** ready for implementation
> **Target repo:** `mattermost-operator` (worktree: `~/workspace/worktrees/mattermost-operator-the-trail/`)
> **Depends on:** Plugin Phases P1–P4 must be complete and deployed
> **Files changed:** 3 modified, 3 deleted

## Summary

Strip all LiteLLM management API calls (model registration, MCP server registration, virtual key creation) from the operator's agent reconciler. Delete the LiteLLM HTTP client entirely. The operator retains only infrastructure: deploy LiteLLM Deployment/Service/ConfigMap, readiness checks, agent pod deployment, env var injection, NetworkPolicy.

---

## Task O1.1: Strip LiteLLM Reconcile Calls from `controller.go`

### File: `controllers/mattermost/agent/controller.go`

Two blocks to remove and one import to clean up.

#### Remove pre-deploy business logic block (lines 122–137)

**Before (lines 121–138):**
```go
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
		_, err = r.reconcileLiteLLMMCPServers(ctx, agent, litellmURL, masterKey, reqLogger)
		if err != nil {
			r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
			return reconcile.Result{}, err
		}
	}
```

**After (lines 121–122):**
```go
		if !ready {
			return reconcile.Result{RequeueAfter: mattermostNotReadyDelay}, nil
		}
	}
```

Delete lines 122–137 (the `masterKey`, `litellmURL`, `reconcileLiteLLMModels`, and `reconcileLiteLLMMCPServers` block). The closing `}` on line 138 stays — it closes the `if agent.Spec.LLMGateway != nil` block.

#### Remove post-health business logic block (lines 191–217)

**Before (lines 190–217):**
```go

	// Register agent pod endpoint as a model in LiteLLM and create virtual key
	// (after health check confirms agent is running).
	if agent.Spec.LLMGateway != nil && agent.Spec.LLMGateway.OperatorManaged != nil {
		masterKey, err := r.getLiteLLMMasterKey(ctx, agent.Namespace)
		if err != nil {
			r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
			return reconcile.Result{}, err
		}
		litellmURL := mattermostApp.LiteLLMServiceURL(agent.Namespace)
		if err := r.reconcileAgentModel(ctx, agent, litellmURL, masterKey, reqLogger); err != nil {
			r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
			return reconcile.Result{}, err
		}

		// Collect MCP access groups from the earlier reconcileLiteLLMMCPServers call.
		// Re-derive them here rather than threading state through the reconcile loop.
		mcpAccessGroups, err := r.reconcileLiteLLMMCPServers(ctx, agent, litellmURL, masterKey, reqLogger)
		if err != nil {
			r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
			return reconcile.Result{}, err
		}

		if err := r.reconcileLiteLLMVirtualKey(ctx, agent, litellmURL, masterKey, mcpAccessGroups, reqLogger); err != nil {
			r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
			return reconcile.Result{}, err
		}
	}
```

**After:** Delete the entire block (lines 191–217, including the comment on line 191). The next line should be `err = r.updateStatus(...)` (currently line 219).

#### Remove unused import

**Before (line 9):**
```go
	mattermostApp "github.com/mattermost/mattermost-operator/pkg/mattermost"
```

**After:** Delete line 9.

After removing the two blocks, `mattermostApp` is no longer referenced anywhere in `controller.go`. The `checkLiteLLM*` methods are defined in `litellm.go` (same package) and call `mattermostApp` from there — the import in `controller.go` is only for the `LiteLLMServiceURL` calls being removed.

#### Verify no other unused imports

After the changes, check that all remaining imports in `controller.go` are still used:
- `"context"` — used (line 56)
- `"time"` — used (lines 23-24)
- `logr` — used (line 57)
- `mmv1beta` — used (lines 8, 46, 61, 73, 88, etc.)
- `resources` — used (line 32)
- `appsv1`, `corev1`, `networkingv1` — used in `SetupWithManager` (lines 47-52)
- `k8sErrors` — used (line 63)
- `"k8s.io/apimachinery/pkg/runtime"` — used (line 31)
- `types` — used (lines 89-91)
- `ctrl` — used (lines 35, 44, 56)
- `client` — used (line 18, 29)
- `reconcile` — used (line 64)

All remain used. Only `mattermostApp` is removed.

---

## Task O1.2: Delete Business Logic Functions from `litellm.go`

### File: `controllers/mattermost/agent/litellm.go`

#### Fix `checkLiteLLMDeployment` — remove `buildProviderEnvVars` call

**Before (lines 30–55):**
```go
func (r *AgentReconciler) checkLiteLLMDeployment(ctx context.Context, agent *mmv1beta.Agent, reqLogger logr.Logger) error {
	om := agent.Spec.LLMGateway.OperatorManaged
	providerEnvVars := buildProviderEnvVars(om.LLMProviders)

	// ── ConfigMap ──────────────────────────────────────────────────────────────
	desiredCM := mattermostApp.GenerateLiteLLMConfigMap(agent.Namespace)
	...
	// ── Deployment ─────────────────────────────────────────────────────────────
	desiredDeploy := mattermostApp.GenerateLiteLLMDeployment(agent.Namespace, om.Image, providerEnvVars)
```

**After:**
```go
func (r *AgentReconciler) checkLiteLLMDeployment(ctx context.Context, agent *mmv1beta.Agent, reqLogger logr.Logger) error {
	om := agent.Spec.LLMGateway.OperatorManaged

	// ── ConfigMap ──────────────────────────────────────────────────────────────
	desiredCM := mattermostApp.GenerateLiteLLMConfigMap(agent.Namespace)
	...
	// ── Deployment ─────────────────────────────────────────────────────────────
	desiredDeploy := mattermostApp.GenerateLiteLLMDeployment(agent.Namespace, om.Image, nil)
```

Two changes:
1. **Delete line 32:** `providerEnvVars := buildProviderEnvVars(om.LLMProviders)` — this called the function being deleted
2. **Change line 55:** Replace `providerEnvVars` with `nil` in the `GenerateLiteLLMDeployment` call

Provider API keys are no longer injected as env vars into the LiteLLM container. The plugin now passes API keys inline via the LiteLLM management API (`litellm_params.api_key` in `POST /model/new`). Passing `nil` means the LiteLLM deployment gets no provider env vars — which is correct because there are no `LLMProviders` in the spec anymore (O2 removes the field, but O1 stops using it).

#### Delete six functions

Delete these functions entirely (they span lines 117–373):

| Function | Lines | Why deleted |
|----------|-------|-------------|
| `getLiteLLMMasterKey` | 117–132 | Plugin reads master key via its own K8s client |
| `reconcileLiteLLMModels` | 134–186 | Plugin registers models via `syncServicesToLiteLLM` |
| `reconcileAgentModel` | 188–219 | Plugin registers agent-as-model in `CreateTrailAgent` |
| `reconcileLiteLLMVirtualKey` | 221–273 | Plugin manages keys in `CreateTrailAgent` |
| `buildProviderEnvVars` | 275–291 | No longer needed — provider API keys injected inline |
| `reconcileLiteLLMMCPServers` | 293–373 | Plugin registers MCP servers via `syncMCPServersToLiteLLM` |

**After deletion, `litellm.go` retains only:**
```
Line 1:   package declaration + copyright
Lines 6-20: imports (cleaned up)
Line 22-26: litellmAnnotator var
Line 28-75: checkLiteLLMDeployment (modified as above)
Line 77-97: checkLiteLLMService (unchanged)
Line 99-115: checkLiteLLMReady (unchanged)
```

#### Clean up imports in `litellm.go`

**Before (lines 6–20):**
```go
import (
	"context"
	"fmt"
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
```

**After:**
```go
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
```

Remove:
- `"fmt"` — only used in `reconcileAgentModel` (deleted)
- `"strings"` — only used in `reconcileLiteLLMModels` and `buildProviderEnvVars` (both deleted)

All other imports remain used by the three kept functions:
- `objectMatcher` — `litellmAnnotator` (line 26), used in `checkLiteLLMDeployment` and `checkLiteLLMService`
- `logr` — function signatures
- `mmv1beta` — `agent.Spec`, constants like `AgentLiteLLMDeploymentName`
- `mattermostApp` — `GenerateLiteLLMConfigMap`, `GenerateLiteLLMDeployment`, `GenerateLiteLLMService`
- `pkgerrors` — error wrapping
- `appsv1` — `&appsv1.Deployment{}` in `checkLiteLLMDeployment` and `checkLiteLLMReady`
- `corev1` — `&corev1.ConfigMap{}` in `checkLiteLLMDeployment`
- `k8sErrors` — `k8sErrors.IsNotFound` checks
- `types` — `types.NamespacedName`

---

## Task O1.3: Delete `litellm_client.go` and Test Files

### Files to delete

| File | Lines | Contents |
|------|-------|----------|
| `controllers/mattermost/agent/litellm_client.go` | 328 | `liteLLMClient`, all request/response types, HTTP methods, `SanitizeMCPServerName` |
| `controllers/mattermost/agent/litellm_client_test.go` | (exists) | Unit tests for the client |
| `controllers/mattermost/agent/litellm_test.go` | (exists) | Tests for reconcile functions |

**Delete all three files entirely.**

All types and functions in `litellm_client.go` were only referenced by the reconcile functions deleted in O1.2. Verify with:

```bash
cd ~/workspace/worktrees/mattermost-operator-the-trail
# After making changes, confirm no dangling references:
grep -r "liteLLMClient\|newLiteLLMClient\|liteLLMModelRequest\|liteLLMMCPServerRequest\|liteLLMKeyRequest\|liteLLMObjectPermission\|errKeyAliasExists\|SanitizeMCPServerName" controllers/mattermost/agent/ --include="*.go" | grep -v "_test.go"
```

This should return zero matches after the deletions.

---

## Complete Before/After: `litellm.go`

The file goes from ~373 lines to ~115 lines. Here is the complete final state:

```go
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
	desiredDeploy := mattermostApp.GenerateLiteLLMDeployment(agent.Namespace, om.Image, nil)
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
```

---

## Reconcile Flow: Before and After

### Before (controller.go `Reconcile`)

```
1. Fetch Agent CR
2. Set initial state
3. Apply defaults
4. Check Mattermost readiness
5. If LLMGateway.OperatorManaged:
   a. checkLiteLLMDeployment        ← KEEP
   b. checkLiteLLMService           ← KEEP
   c. checkLiteLLMReady             ← KEEP
   d. getLiteLLMMasterKey            ← REMOVE
   e. reconcileLiteLLMModels        ← REMOVE
   f. reconcileLiteLLMMCPServers    ← REMOVE
6. checkAgentServiceAccount
7. checkHookSecret
8. checkAgentService
9. checkAgentDeployment
10. checkAgentNetworkPolicy
11. checkAgentHealth
12. If LLMGateway.OperatorManaged:
   a. getLiteLLMMasterKey            ← REMOVE
   b. reconcileAgentModel           ← REMOVE
   c. reconcileLiteLLMMCPServers    ← REMOVE
   d. reconcileLiteLLMVirtualKey    ← REMOVE
13. updateStatus
```

### After

```
1. Fetch Agent CR
2. Set initial state
3. Apply defaults
4. Check Mattermost readiness
5. If LLMGateway.OperatorManaged:
   a. checkLiteLLMDeployment
   b. checkLiteLLMService
   c. checkLiteLLMReady
6. checkAgentServiceAccount
7. checkHookSecret
8. checkAgentService
9. checkAgentDeployment
10. checkAgentNetworkPolicy
11. checkAgentHealth
12. updateStatus
```

The operator's job is now purely infrastructure: deploy LiteLLM, deploy agent pods, and ensure health. All LiteLLM management API interactions are handled by the plugin.

---

## Verification

### Compile check
```bash
cd ~/workspace/worktrees/mattermost-operator-the-trail
go build ./...
go vet ./...
```

### Confirm no dangling references
```bash
# Should return no matches after changes:
grep -rn "reconcileLiteLLMModels\|reconcileAgentModel\|reconcileLiteLLMVirtualKey\|reconcileLiteLLMMCPServers\|buildProviderEnvVars\|getLiteLLMMasterKey\|newLiteLLMClient\|liteLLMClient\b\|errKeyAliasExists\|SanitizeMCPServerName" \
  controllers/mattermost/agent/ --include="*.go"
```

### Run remaining tests
```bash
go test ./controllers/mattermost/agent/...
go test ./pkg/mattermost/...
```

**Expected:** Some tests in `agent_test.go` and `pkg/mattermost/litellm_test.go` may fail because they reference `LLMProviders` in test fixtures. Those failures are addressed in Phase O3 (Task #23) — not in O1. If the implementing agent encounters compile errors in test files due to deleted types, note them for O3 but don't fix them here. Use `go build ./...` (not `go test`) as the O1 gate.

### k3d verification
After deploying the updated operator:
1. Verify LiteLLM Deployment/Service/ConfigMap still created for agents with `LLMGateway.OperatorManaged`
2. Verify agent pods still deploy and become healthy
3. Verify env vars still injected from Secrets (`AGENT_MODEL`, `OPENAI_API_KEY`, etc.)
4. Verify the operator makes NO LiteLLM API calls (check operator logs — no "Registering model", "Registering MCP server", "Generated LiteLLM virtual key" log lines)

---

## Risks

| Risk | Mitigation |
|------|------------|
| Tests fail due to deleted types | Expected — O3 fixes tests. Use `go build ./...` as O1 gate, not `go test`. |
| `GenerateLiteLLMDeployment` called with `nil` providerEnvVars | The function handles `nil` correctly — `append(baseEnv, nil...)` is a no-op in Go. Verified in `pkg/mattermost/litellm.go` line 79. |
| Existing dev agents have `LLMProviders`/`MCPServers` in their CRs | The fields still exist in the CRD (removed in O2). They are simply ignored now — the operator no longer reads them for business logic. `checkLiteLLMDeployment` accesses `om.Image` but no longer reads `om.LLMProviders`. |
| `SanitizeMCPServerName` is exported — external consumers? | Only called from `reconcileLiteLLMMCPServers` (deleted) and tests (deleted). Plugin has its own copy (from P1). No external packages import the operator's agent controller package. |

---

## Implementation Summary (completed 2026-04-07)

### O1.1: controller.go
- Removed `mattermostApp` import (no longer referenced)
- Removed pre-deploy business logic block: `getLiteLLMMasterKey`, `LiteLLMServiceURL`, `reconcileLiteLLMModels`, `reconcileLiteLLMMCPServers`
- Removed post-health business logic block: `getLiteLLMMasterKey`, `LiteLLMServiceURL`, `reconcileAgentModel`, `reconcileLiteLLMMCPServers`, `reconcileLiteLLMVirtualKey`

### O1.2: litellm.go
- Removed `"fmt"` and `"strings"` imports
- Removed `buildProviderEnvVars` call; pass `nil` to `GenerateLiteLLMDeployment`
- Deleted 6 functions: `getLiteLLMMasterKey`, `reconcileLiteLLMModels`, `reconcileAgentModel`, `reconcileLiteLLMVirtualKey`, `buildProviderEnvVars`, `reconcileLiteLLMMCPServers`
- File reduced from ~373 lines to ~113 lines (3 infra functions remain)

### O1.3: Deleted files
- `litellm_client.go` (328 lines — HTTP client + types)
- `litellm_client_test.go` (unit tests)
- `litellm_test.go` (reconcile function tests)

### Deviations from plan
- None. Implemented exactly as specified.

### Verification
- `go build ./...` — passed, zero compile errors
- Dangling reference grep — zero matches for any deleted function/type names
- No commits created (per instructions)
