# Phase 1: Operator RBAC + CRD Changes — Prescriptive Plan

> **Milestone:** M4 — Plugin-Driven Trail Agent Orchestration
> **Target repo:** `~/workspace/worktrees/mattermost-operator-the-trail`
> **Phase:** 1 (Operator Simplification + RBAC Extension)
> **Depends on:** nothing (first phase)
> **Goal:** Extend MM server pod RBAC Role for Agent CRUD + Secret write. Add status fields (phase, message, readyReplicas) to Agent CRD. Remove `AdminCredentialsSecret`. Add lifecycle phase tracking in reconciler. All tests pass.

---

## Context: What Already Exists

All tasks in this phase **modify** existing files. Do not recreate files from scratch.

- `apis/mattermost/v1beta1/agent_types.go` — `AgentSpec` with `AdminCredentialsSecret` field (line 53), `AgentStatus` with State, Endpoint, BotUserID, BotUsername, ObservedGeneration, Error (lines 73-99).
- `apis/mattermost/v1beta1/agent_utils.go` — constants block (lines 11-24), `SetDefaults()`, label helpers, `BotTokenSecretName()`, `LiteLLMKeySecretName()`.
- `pkg/mattermost/mattermost_v1beta.go` — `mattermostRolePermissions()` returns 2 `PolicyRule` entries (lines 555-569), `GenerateRoleV1Beta()` calls it (line 544).
- `controllers/mattermost/agent/controller.go` — `Reconcile()` loop with admin secret read (lines 101-111), `checkAgentBot` call (lines 113-118), `reconcileLiteLLMVirtualKey` call (line 156).
- `controllers/mattermost/agent/agent.go` — `checkAgentBot()` (line 35), `checkAgentBotWithURL()` (line 41), bot HTTP helpers, `checkAgentHealth()` (line 245).
- `controllers/mattermost/agent/litellm.go` — `reconcileLiteLLMVirtualKey()` (line 157).
- `controllers/mattermost/agent/utils.go` — `updateStatusReconciling()`, `updateStatusReconcilingAndLogError()`, `updateStatus()`.

Module path: `github.com/mattermost/mattermost-operator`

---

## Task 1.1: Extend MM Server Pod RBAC Role

**File:** `pkg/mattermost/mattermost_v1beta.go`
**Lines:** 555-569 (`mattermostRolePermissions()`)

### What to change

Replace the current 2-rule return value with a 4-rule return value. The first rule (batch/jobs) is unchanged. The second rule (agents get/list/watch) is expanded to full CRUD. Two new rules are added: agents/status get, and secrets get/create/update.

### Exact edit

Replace the entire function body of `mattermostRolePermissions()` (lines 555-569):

**Old (lines 555-569):**
```go
func mattermostRolePermissions() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		{
			Verbs:         []string{"get", "list", "watch"},
			APIGroups:     []string{"batch"},
			Resources:     []string{"jobs"},
			ResourceNames: []string{SetupJobName},
		},
		{
			Verbs:     []string{"get", "list", "watch"},
			APIGroups: []string{"installation.mattermost.com"},
			Resources: []string{"agents"},
		},
	}
}
```

**New:**
```go
func mattermostRolePermissions() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		{
			Verbs:         []string{"get", "list", "watch"},
			APIGroups:     []string{"batch"},
			Resources:     []string{"jobs"},
			ResourceNames: []string{SetupJobName},
		},
		{
			Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
			APIGroups: []string{"installation.mattermost.com"},
			Resources: []string{"agents"},
		},
		{
			Verbs:     []string{"get"},
			APIGroups: []string{"installation.mattermost.com"},
			Resources: []string{"agents/status"},
		},
		{
			Verbs:     []string{"get", "create", "update"},
			APIGroups: []string{""},
			Resources: []string{"secrets"},
		},
	}
}
```

### Tests to update

**File:** `pkg/mattermost/mattermost_v1beta_test.go`
**Lines:** 1104-1132 (`TestGenerateRBACResources_V1Beta`)

The test currently asserts `require.Equal(t, 2, len(role.Rules))` on line 1123. Change to 4 rules and add verb assertions:

**Old (line 1123):**
```go
	require.Equal(t, 2, len(role.Rules))
```

**New (replace line 1123 with):**
```go
	require.Equal(t, 4, len(role.Rules))

	// Rule 0: batch/jobs — get,list,watch (unchanged).
	assert.Equal(t, []string{"get", "list", "watch"}, role.Rules[0].Verbs)
	assert.Equal(t, []string{"batch"}, role.Rules[0].APIGroups)
	assert.Equal(t, []string{"jobs"}, role.Rules[0].Resources)

	// Rule 1: agents — full CRUD.
	assert.Equal(t, []string{"get", "list", "watch", "create", "update", "patch", "delete"}, role.Rules[1].Verbs)
	assert.Equal(t, []string{"installation.mattermost.com"}, role.Rules[1].APIGroups)
	assert.Equal(t, []string{"agents"}, role.Rules[1].Resources)

	// Rule 2: agents/status — get only.
	assert.Equal(t, []string{"get"}, role.Rules[2].Verbs)
	assert.Equal(t, []string{"installation.mattermost.com"}, role.Rules[2].APIGroups)
	assert.Equal(t, []string{"agents/status"}, role.Rules[2].Resources)

	// Rule 3: secrets — get, create, update.
	assert.Equal(t, []string{"get", "create", "update"}, role.Rules[3].Verbs)
	assert.Equal(t, []string{""}, role.Rules[3].APIGroups)
	assert.Equal(t, []string{"secrets"}, role.Rules[3].Resources)
```

Also ensure `assert` is imported in this test file. It currently uses `require` but not `assert`. Add to the import block:
```go
	"github.com/stretchr/testify/assert"
```

**File:** `controllers/mattermost/mattermost/mattermost_test.go`
**Lines:** 175-197 (`"role"` subtest)

The existing test creates the role, modifies it (sets `Rules = nil`), re-reconciles, and asserts rules are restored. No changes needed — the `assert.Equal(t, original.Rules, found.Rules)` on line 196 will pass with the new 4-rule set because `original` captures whatever `mattermostRolePermissions()` returns.

### Validation

```bash
cd ~/workspace/worktrees/mattermost-operator-the-trail
go test ./pkg/mattermost/ -run TestGenerateRBACResources_V1Beta -v
go test ./controllers/mattermost/mattermost/ -run TestCheckMattermost -v
```

---

## Task 1.2: Add Agent CR Status Fields

### Part A: Add fields to AgentStatus

**File:** `apis/mattermost/v1beta1/agent_types.go`
**Lines:** 72-99 (`AgentStatus` struct)

Insert three new fields after the `Error` field (after line 98, before the closing `}` of `AgentStatus`):

**Old (lines 96-99):**
```go
	// Error records the last observed error in the reconciliation of this Agent.
	// +optional
	Error string `json:"error,omitempty"`
}
```

**New:**
```go
	// Error records the last observed error in the reconciliation of this Agent.
	// +optional
	Error string `json:"error,omitempty"`

	// Phase is the lifecycle phase of the agent.
	// One of: Provisioning, Deploying, Ready, Error.
	// +optional
	Phase string `json:"phase,omitempty"`

	// Message is a human-readable status message providing additional detail.
	// +optional
	Message string `json:"message,omitempty"`

	// ReadyReplicas is the number of ready replicas for the agent deployment.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`
}
```

### Part B: Add phase constants

**File:** `apis/mattermost/v1beta1/agent_utils.go`
**Lines:** 11-24 (existing const block)

Add a new const block **after** the existing one (after line 24):

```go
// Agent lifecycle phases (written to AgentStatus.Phase).
const (
	AgentPhaseProvisioning = "Provisioning"
	AgentPhaseDeploying    = "Deploying"
	AgentPhaseReady        = "Ready"
	AgentPhaseError        = "Error"
)
```

### After editing

```bash
cd ~/workspace/worktrees/mattermost-operator-the-trail
make generate manifests
```

This regenerates `zz_generated.deepcopy.go` (adds the new fields to `DeepCopyInto`) and updates CRD YAML manifests.

### Validation

Verify the CRD YAML contains the new status fields:
```bash
grep -A2 'phase\|message\|readyReplicas' config/crd/bases/installation.mattermost.com_agents.yaml
```

Expected: `phase`, `message`, and `readyReplicas` appear under `status.properties`.

---

## Task 1.3: Remove AdminCredentialsSecret

### Part A: Remove from AgentSpec

**File:** `apis/mattermost/v1beta1/agent_types.go`
**Lines:** 50-53

Delete these 4 lines entirely:
```go
	// AdminCredentialsSecret is the name of the Kubernetes Secret containing
	// a Mattermost admin access token used to provision the bot account.
	// The Secret must have a key "token" with the admin access token value.
	AdminCredentialsSecret string `json:"adminCredentialsSecret"`
```

### Part B: Remove admin secret read + bot provisioning from reconciler

**File:** `controllers/mattermost/agent/controller.go`

**Delete lines 101-118** (the admin secret read block and `checkAgentBot` call):
```go
	// Read admin token.
	adminSecret := &corev1.Secret{}
	err = r.Client.Get(ctx, types.NamespacedName{
		Name:      agent.Spec.AdminCredentialsSecret,
		Namespace: agent.Namespace,
	}, adminSecret)
	if err != nil {
		r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
		return reconcile.Result{}, err
	}
	adminToken := string(adminSecret.Data["token"])

	// Bot provisioning.
	err = r.checkAgentBot(ctx, agent, adminToken, reqLogger)
	if err != nil {
		r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
		return reconcile.Result{}, err
	}
```

Also remove the `reconcileLiteLLMVirtualKey` call. **Delete lines 156-159** (inside the `if agent.Spec.LLMGateway... OperatorManaged` block):
```go
		if err = r.reconcileLiteLLMVirtualKey(ctx, agent, litellmURL, masterKey, mcpAccessGroups, reqLogger); err != nil {
			r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
			return reconcile.Result{}, err
		}
```

After this removal, the `mcpAccessGroups` variable returned by `reconcileLiteLLMMCPServers` is unused. Change the call on line 151 to discard it:

**Old:**
```go
		mcpAccessGroups, err := r.reconcileLiteLLMMCPServers(ctx, agent, litellmURL, masterKey, reqLogger)
```

**New:**
```go
		_, err = r.reconcileLiteLLMMCPServers(ctx, agent, litellmURL, masterKey, reqLogger)
```

The resulting reconcile loop (after edits) should look like:
```
1. Fetch Agent CR
2. Set initial state
3. Set defaults
4. Fetch Mattermost CR, check stability
5. LiteLLM gateway (operator-managed):
   a. checkLiteLLMDeployment
   b. checkLiteLLMService
   c. checkLiteLLMReady
   d. getLiteLLMMasterKey
   e. reconcileLiteLLMModels
   f. reconcileLiteLLMMCPServers (return value discarded)
6. checkAgentServiceAccount
7. checkAgentService
8. checkAgentDeployment
9. checkAgentNetworkPolicy
10. checkAgentHealth
11. updateStatus
```

### Part C: Remove unused imports from controller.go

After removing the admin secret and bot provisioning code, check if any imports become unused. The `adminToken` variable and `adminSecret` variable are removed. The `corev1` import is still used elsewhere, so no import removal needed.

### Part D: Update test fixtures (compile errors)

These fixtures reference `AdminCredentialsSecret` and will fail to compile after Part A.

**File:** `controllers/mattermost/agent/agent_test.go` — `newTestAgent()` (line 39)

Delete:
```go
			AdminCredentialsSecret: "admin-secret",
```

**File:** `pkg/mattermost/agent_test.go` — `testAgent()` (line 37)

Delete:
```go
			AdminCredentialsSecret: "admin-secret",
```

**File:** `pkg/mattermost/litellm_test.go` — `TestGenerateAgentLiteLLMKeySecret` (line 178)

Delete:
```go
			AdminCredentialsSecret: "admin-secret",
```

**File:** `controllers/mattermost/agent/controller_test.go`

Multiple tests create `adminSecret` objects and pass them to the fake client. Remove these:

1. **`TestReconcileAgent_FullReconcile` (line 81):**
   - Delete the `adminSecret` variable declaration (lines 104-110).
   - Remove `adminSecret` from `WithRuntimeObjects(agent, mm, adminSecret)` → `WithRuntimeObjects(agent, mm)`.

2. **`TestReconcileAgent_ImageUpdate` (line 203):**
   - Delete the `adminSecret` variable declaration (lines 223-229).
   - Remove `adminSecret` from `WithRuntimeObjects(agent, mm, adminSecret, botTokenSecret)` → `WithRuntimeObjects(agent, mm, botTokenSecret)`.

3. **`TestReconcileAgent_WithLLMGateway` (line 283):**
   - Delete the `adminSecret` variable declaration (lines 310-316).
   - Remove `adminSecret` from `WithRuntimeObjects(agent, mm, adminSecret, masterKeySecret, botTokenSecret)` → `WithRuntimeObjects(agent, mm, masterKeySecret, botTokenSecret)`.

**File:** `config/samples/installation.mattermost.com_v1beta1_agent.yaml` (line 24)

Delete:
```yaml
  adminCredentialsSecret: my-mattermost-admin-token
```

### Part E: Remove unused bot provisioning code

After removing the `checkAgentBot` call from the reconciler, the following functions in `agent.go` are dead code. **Delete them entirely:**

**File:** `controllers/mattermost/agent/agent.go`

Delete these functions and their associated types:
- `type mmBot struct` (lines 23-26)
- `type mmToken struct` (lines 28-31)
- `func (r *AgentReconciler) checkAgentBot(...)` (lines 35-38)
- `func (r *AgentReconciler) checkAgentBotWithURL(...)` (lines 41-94)
- `func (r *AgentReconciler) listBots(...)` (lines 96-119)
- `func (r *AgentReconciler) createBot(...)` (lines 121-146)
- `func (r *AgentReconciler) createBotToken(...)` (lines 148-173)

After deletion, remove unused imports from `agent.go`. The following imports are only used by bot provisioning code:
- `"encoding/json"`
- `"io"`
- `"net/http"`
- `"strings"`

Check after deletion — `fmt` is still used by `checkAgentHealth`.

### Part F: Remove unused bot provisioning tests

**File:** `controllers/mattermost/agent/agent_test.go`

Delete these test functions (they test the removed `checkAgentBotWithURL`):
- `TestCheckAgentBot_CreatesNewBot` (lines 78-120)
- `TestCheckAgentBot_BotAlreadyExists` (lines 122-149)

After deletion, remove unused imports:
- `"encoding/json"`
- `"net/http"`
- `"net/http/httptest"`

Check remaining imports — `context`, `testing`, `mmv1beta`, `resources`, `logrus`, `blubr`, `assert`, `require`, `appsv1`, `corev1`, `networkingv1`, `metav1`, `runtime`, `types`, `scheme`, `fake` are still used by the remaining tests.

### Part G: Remove unused virtual key code

**File:** `controllers/mattermost/agent/litellm.go`

Delete the `reconcileLiteLLMVirtualKey` function (lines 153-208). It is no longer called.

After this deletion, check if `errKeyAliasExists` (used only by this function) is still referenced. If not, find its definition in `litellm_client.go` and remove it too.

**File:** `controllers/mattermost/agent/litellm_test.go`

Delete these test functions (they test the removed `reconcileLiteLLMVirtualKey`):
- `TestReconcileLiteLLMVirtualKey_CreatesSecret` (lines 177-203)
- `TestReconcileLiteLLMVirtualKey_Idempotent` (lines 207-231)

Also check if `liteLLMKeyResponse` type (used by these tests) is still referenced elsewhere; if not, remove it from `litellm_client.go`.

### After editing

```bash
cd ~/workspace/worktrees/mattermost-operator-the-trail
make generate manifests
```

### Validation

```bash
# Must compile
make build

# CRD YAML must NOT contain adminCredentialsSecret
grep -c adminCredentialsSecret config/crd/bases/installation.mattermost.com_agents.yaml
# Expected: 0

# All tests pass
make unittest
```

---

## Task 1.4: Verify Dual-Mode (Read-Only)

**No code changes.** This task verifies that the remaining skip-if-exists paths work correctly after Task 1.3 removes the bot/key provisioning code.

### What the plugin will do (context for verification)

In Phase 2 (plugin changes), the plugin creates these Secrets **before** the Agent CR:
- `agent-<name>-token` — bot token
- `agent-<name>-litellm-key` — LiteLLM virtual key

The operator does **not** create these Secrets anymore (Task 1.3 removed that code). The operator only **mounts** them into the agent Deployment.

### Verification points

1. **Bot token Secret mount is still present.**

   **File:** `pkg/mattermost/agent.go` — `GenerateAgentDeployment()`

   The deployment generator mounts `agent.BotTokenSecretName()` as volume `bot-token`. This code is unchanged and will continue to work. The Secret must exist when the pod starts; if it doesn't, the pod will stay in `CreateContainerConfigError` and the operator will requeue via `checkAgentHealth`.

2. **LiteLLM key Secret mount is still present.**

   **File:** `pkg/mattermost/agent.go` — `GenerateAgentDeployment()`

   When `LLMGateway.OperatorManaged` or `LLMGateway.External` is set, the deployment generator adds `OPENAI_API_KEY` and `ANTHROPIC_API_KEY` env vars referencing `agent.LiteLLMKeySecretName()` via `SecretKeyRef`. Same behavior — pod won't start until the Secret exists.

3. **No explicit "requeue if Secret missing" logic is needed in the operator.**

   K8s handles this natively: a Deployment referencing a non-existent Secret will keep pods in `Pending`/`CreateContainerConfigError`. The operator's `checkAgentHealth` (which checks `deployment.Status.ReadyReplicas < 1`) will requeue with `healthCheckRequeueDelay` (6s). This is functionally equivalent to an explicit Secret-existence check with requeue.

### Document in PR description

Include in the PR description:
> **Dual-mode provisioning:** The operator no longer creates bot token or LiteLLM key Secrets. These are now created by the plugin before the Agent CR. The operator mounts them via volume mounts and secretKeyRefs. If the Secrets don't exist when the pod starts, K8s keeps the pod in CreateContainerConfigError and the operator requeues via checkAgentHealth (6s delay).

---

## Task 1.5: Status Phase Tracking in Reconciler

**Depends on:** Task 1.2 (phase constants and status fields must exist)

### Part A: Set initial phase in reconcile loop

**File:** `controllers/mattermost/agent/controller.go`

In the `Reconcile()` function, after the initial state check block (which sets State to Reconciling), also set Phase to Provisioning if empty.

**Old (approximately lines 72-77 after Task 1.3 edits):**
```go
	// Set initial state to Reconciling.
	if len(agent.Status.State) == 0 {
		err = r.updateStatusReconciling(ctx, agent, status, reqLogger)
		if err != nil {
			return reconcile.Result{}, err
		}
	}
```

**New:**
```go
	// Set initial state to Reconciling.
	if len(agent.Status.State) == 0 {
		status.Phase = mmv1beta.AgentPhaseProvisioning
		err = r.updateStatusReconciling(ctx, agent, status, reqLogger)
		if err != nil {
			return reconcile.Result{}, err
		}
	}
```

### Part B: Set phase to Deploying after LiteLLM is ready

**File:** `controllers/mattermost/agent/controller.go`

After the LiteLLM block completes (before `checkAgentServiceAccount`), add:

```go
	// LiteLLM is ready (or not configured); transition to Deploying.
	if status.Phase == mmv1beta.AgentPhaseProvisioning {
		status.Phase = mmv1beta.AgentPhaseDeploying
	}
```

Insert this right **before** the `// ServiceAccount` comment. If the agent has no LiteLLM gateway, it transitions from Provisioning to Deploying immediately after the Mattermost CR readiness check.

### Part C: Set Error phase on errors

**File:** `controllers/mattermost/agent/utils.go`

In `updateStatusReconcilingAndLogError`, also set Phase to Error:

**Old (lines 18-26):**
```go
func (r *AgentReconciler) updateStatusReconcilingAndLogError(ctx context.Context, agent *mmv1beta.Agent, status mmv1beta.AgentStatus, reqLogger logr.Logger, statusErr error) {
	if statusErr != nil {
		status.Error = statusErr.Error()
	}
	err := r.updateStatusReconciling(ctx, agent, status, reqLogger)
	if err != nil {
		reqLogger.Error(err, "Failed to set agent state to reconciling")
	}
}
```

**New:**
```go
func (r *AgentReconciler) updateStatusReconcilingAndLogError(ctx context.Context, agent *mmv1beta.Agent, status mmv1beta.AgentStatus, reqLogger logr.Logger, statusErr error) {
	if statusErr != nil {
		status.Error = statusErr.Error()
	}
	status.Phase = mmv1beta.AgentPhaseError
	err := r.updateStatusReconciling(ctx, agent, status, reqLogger)
	if err != nil {
		reqLogger.Error(err, "Failed to set agent state to reconciling")
	}
}
```

### Part D: Set Ready/Deploying phase in checkAgentHealth

**File:** `controllers/mattermost/agent/agent.go` — `checkAgentHealth()` (line 245 before edits; line number will shift after Task 1.3 deletions)

**Old:**
```go
func (r *AgentReconciler) checkAgentHealth(ctx context.Context, agent *mmv1beta.Agent, currentStatus mmv1beta.AgentStatus, reqLogger logr.Logger) (mmv1beta.AgentStatus, error) {
	status := mmv1beta.AgentStatus{
		State:              mmv1beta.Reconciling,
		ObservedGeneration: agent.Generation,
		Endpoint:           fmt.Sprintf("%s.%s.svc.cluster.local:%d", agent.Name, agent.Namespace, mmv1beta.AgentGRPCPort),
	}

	deployment := &appsv1.Deployment{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, deployment)
	if err != nil {
		return status, errors.Wrap(err, "failed to get agent deployment for health check")
	}

	if deployment.Status.ReadyReplicas < 1 {
		return status, fmt.Errorf("agent deployment has %d ready replicas, need at least 1", deployment.Status.ReadyReplicas)
	}

	status.State = mmv1beta.Stable
	return status, nil
}
```

**New:**
```go
func (r *AgentReconciler) checkAgentHealth(ctx context.Context, agent *mmv1beta.Agent, currentStatus mmv1beta.AgentStatus, reqLogger logr.Logger) (mmv1beta.AgentStatus, error) {
	status := mmv1beta.AgentStatus{
		State:              mmv1beta.Reconciling,
		Phase:              mmv1beta.AgentPhaseDeploying,
		ObservedGeneration: agent.Generation,
		Endpoint:           fmt.Sprintf("%s.%s.svc.cluster.local:%d", agent.Name, agent.Namespace, mmv1beta.AgentGRPCPort),
	}

	deployment := &appsv1.Deployment{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, deployment)
	if err != nil {
		return status, errors.Wrap(err, "failed to get agent deployment for health check")
	}

	if deployment.Status.ReadyReplicas < 1 {
		return status, fmt.Errorf("agent deployment has %d ready replicas, need at least 1", deployment.Status.ReadyReplicas)
	}

	status.State = mmv1beta.Stable
	status.Phase = mmv1beta.AgentPhaseReady
	status.ReadyReplicas = deployment.Status.ReadyReplicas
	return status, nil
}
```

### Tests to update

**File:** `controllers/mattermost/agent/controller_test.go`

**`TestReconcileAgent_FullReconcile`:**

After the "second reconcile" (when deployment is ready and status is Stable), add assertions for the new phase fields. After line 199 (`assert.Contains(t, updatedAgent.Status.Endpoint, agent.Name)`), add:

```go
	assert.Equal(t, mmv1beta.AgentPhaseReady, updatedAgent.Status.Phase)
	assert.Equal(t, int32(1), updatedAgent.Status.ReadyReplicas)
```

**`TestReconcileAgent_WithLLMGateway`:**

Similarly, after the first reconcile (deployment not ready, requeued), verify the agent is in Deploying phase. After line 387 (`assert.Equal(t, 6*time.Second, res.RequeueAfter)`), add:

```go
	// Verify agent is in Deploying phase (not ready yet).
	agentAfterFirstReconcile := &mmv1beta.Agent{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, agentAfterFirstReconcile)
	require.NoError(t, err)
	assert.Equal(t, mmv1beta.AgentPhaseDeploying, agentAfterFirstReconcile.Status.Phase)
```

### Validation

```bash
cd ~/workspace/worktrees/mattermost-operator-the-trail
go test ./controllers/mattermost/agent/ -run TestReconcileAgent -v
```

---

## Execution Order

```
Tasks 1.1, 1.2, 1.3 — can run in parallel (independent edits to different code)
Task 1.4 — read-only verification (anytime)
Task 1.5 — after Task 1.2 completes (needs phase constants)

Final validation:
  make generate manifests
  make build
  make unittest
```

## Definition of Done

- [x] `make generate manifests` succeeds
- [x] `make build` compiles with zero errors
- [x] `make unittest` passes (0 failures)
- [x] CRD YAML contains `phase`, `message`, `readyReplicas` in status
- [x] CRD YAML does NOT contain `adminCredentialsSecret`
- [x] Role YAML includes agents CRUD + agents/status get + secrets get/create/update (4 rules total)
- [x] `checkAgentBot` and `reconcileLiteLLMVirtualKey` are fully removed (no dead code)
- [x] Phase tracking: agent reaches `Ready` phase with correct `readyReplicas` in tests

---

## Implementation Summary (2026-04-04)

All 5 tasks completed successfully:

### Task 1.1: RBAC Role Extended
- `mattermostRolePermissions()` now returns 4 rules: batch/jobs (unchanged), agents full CRUD, agents/status get, secrets get/create/update
- Updated both v1alpha1 and v1beta1 RBAC tests to assert 4 rules with verb-level checks

### Task 1.2: Agent CR Status Fields Added
- Added `Phase`, `Message`, `ReadyReplicas` to `AgentStatus` struct
- Added phase constants: `AgentPhaseProvisioning`, `AgentPhaseDeploying`, `AgentPhaseReady`, `AgentPhaseError`
- Ran `make generate manifests` to update deepcopy and CRD YAML

### Task 1.3: AdminCredentialsSecret Removed
- Removed `AdminCredentialsSecret` field from `AgentSpec`
- Removed admin secret read + `checkAgentBot` call from reconciler
- Removed `reconcileLiteLLMVirtualKey` call from reconciler (discarded `mcpAccessGroups` return value)
- Deleted all bot provisioning code: `mmBot`, `mmToken`, `checkAgentBot`, `checkAgentBotWithURL`, `listBots`, `createBot`, `createBotToken`
- Deleted `reconcileLiteLLMVirtualKey` function from litellm.go
- Deleted bot provisioning tests: `TestCheckAgentBot_CreatesNewBot`, `TestCheckAgentBot_BotAlreadyExists`
- Deleted virtual key tests: `TestReconcileLiteLLMVirtualKey_CreatesSecret`, `TestReconcileLiteLLMVirtualKey_Idempotent`
- Updated all test fixtures to remove `AdminCredentialsSecret` and `adminSecret` objects
- Cleaned up unused imports across all modified files
- Removed `adminCredentialsSecret` from sample YAML

### Task 1.4: Dual-Mode Verified (Read-Only)
- Bot token Secret mount still present in `GenerateAgentDeployment()` (volume mount)
- LiteLLM key Secret still referenced via `SecretKeyRef` env vars
- K8s handles missing Secrets natively (pod stays in CreateContainerConfigError)
- Operator requeues via `checkAgentHealth` with 6s delay

### Task 1.5: Phase Tracking Wired
- Initial reconcile sets `Phase = Provisioning`
- After LiteLLM block: transitions to `Deploying`
- `checkAgentHealth`: non-ready returns `Deploying`, ready returns `Ready` + `ReadyReplicas`
- `updateStatusReconcilingAndLogError`: sets `Phase = Error`
- Tests assert `Phase == Ready` and `ReadyReplicas == 1` in FullReconcile, `Phase == Deploying` in WithLLMGateway
