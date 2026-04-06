# Implementation Plan: M4 Phase 1 — Operator Simplification + RBAC Extension

> **Spec:** `~/workspace/planning/projects/mattermost-cloud-agents/ideas/002-the-trail/spec.md`
> **Worktree:** `~/workspace/worktrees/mattermost-operator-the-trail`
> **Generated:** 2026-04-03
> **Status:** ready

## Overview

Extend the MM server pod's RBAC Role to allow Agent CR CRUD + Secret write. Add status fields to the Agent CRD. Remove `AdminCredentialsSecret`. Keep dual-mode bot/key provisioning (operator skips if Secrets exist). Add lifecycle phase tracking to Agent CR status.

## Tasks

### Task 1: Extend MM Server Pod RBAC Role

**File:** `pkg/mattermost/mattermost_v1beta.go` — `mattermostRolePermissions()` (~line 555)

**Action:** Replace the second rule (`agents: get,list,watch`) with three rules:

```go
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
```

**Note:** `mattermostRolePermissions()` is shared between v1beta1 and v1alpha1 (ClusterInstallation). Both paths will get the new rules.

**Test:** `controllers/mattermost/mattermost/mattermost_test.go` — add assertions on rule count (4) and agent CRUD verbs in the `"role"` subtest.

**Depends on:** nothing

---

### Task 2: Add Agent CR Status Fields

**File:** `apis/mattermost/v1beta1/agent_types.go` — `AgentStatus` struct (~line 73)

**Action:** Add three fields after `Error string`:

```go
Phase         string `json:"phase,omitempty"`
Message       string `json:"message,omitempty"`
ReadyReplicas int32  `json:"readyReplicas,omitempty"`
```

**File:** `apis/mattermost/v1beta1/agent_utils.go` — add phase constants after existing const block:

```go
const (
    AgentPhaseProvisioning = "Provisioning"
    AgentPhaseDeploying    = "Deploying"
    AgentPhaseReady        = "Ready"
    AgentPhaseError        = "Error"
)
```

**After editing:** `make generate manifests`

**Depends on:** nothing

---

### Task 3: Remove AdminCredentialsSecret

**Part A — Remove from AgentSpec:**
- `apis/mattermost/v1beta1/agent_types.go` — delete `AdminCredentialsSecret` field (~lines 50-53)

**Part B — Remove from controller:**
- `controllers/mattermost/agent/controller.go` — remove the `adminSecret` read block (~lines 101-111)
- Update `checkAgentBot` call: pass empty string for `adminToken`. The idempotency check (skip if Secret exists, line 43-53 in `agent.go`) handles this — if Secret exists, `adminToken` is never used.

**Part C — Update test fixtures (compile errors):**
- `controllers/mattermost/agent/agent_test.go` line 39 — `newTestAgent()`: remove `AdminCredentialsSecret`
- `pkg/mattermost/agent_test.go` line 37 — `testAgent()`: remove `AdminCredentialsSecret`
- `pkg/mattermost/litellm_test.go` line 178 — inline fixture: remove `AdminCredentialsSecret`
- `config/samples/installation.mattermost.com_v1beta1_agent.yaml` line 24: remove `adminCredentialsSecret`

**After editing:** `make generate manifests`

**Depends on:** nothing (parallel with Tasks 1, 2)

---

### Task 4: Verify Dual-Mode (Read-Only)

No code changes. Verify these skip paths exist:
- `controllers/mattermost/agent/agent.go` lines 43-53 — `checkAgentBot` skips if bot token Secret exists
- `controllers/mattermost/agent/litellm.go` lines 159-167 — `reconcileLiteLLMVirtualKey` skips if LiteLLM key Secret exists

Both are already tested. Document in PR description.

**Depends on:** nothing

---

### Task 5: Status Phase Tracking in Reconciler

**File:** `controllers/mattermost/agent/controller.go`

Add phase transitions:
- After initial status check (~line 72): set `Phase = AgentPhaseProvisioning` if empty
- After LiteLLM ready check: set `Phase = AgentPhaseDeploying`
- In error helper (`utils.go` `updateStatusReconcilingAndLogError`): set `Phase = AgentPhaseError`

**File:** `controllers/mattermost/agent/agent.go` — `checkAgentHealth` (~line 246)
- Non-ready path: set `Phase = AgentPhaseDeploying`
- Ready path: set `Phase = AgentPhaseReady`, `ReadyReplicas = deployment.Status.ReadyReplicas`

**Tests:** Update `TestReconcileAgent_FullReconcile` in `controller_test.go` to assert `Phase == "Ready"` and `ReadyReplicas == 1`.

**Depends on:** Task 2 (status fields must exist)

---

## Execution Order

```
Tasks 1, 2, 3 — can run in parallel (independent)
Task 4 — read-only verification (anytime)
Task 5 — after Task 2 completes
Final: make generate manifests && make build && make unittest
```

## Definition of Done

- [ ] `make generate manifests` succeeds
- [ ] `make build` compiles
- [ ] `make unittest` passes (0 failures)
- [ ] CRD YAML contains `phase`, `message`, `readyReplicas` in status
- [ ] CRD YAML does NOT contain `adminCredentialsSecret`
- [ ] Role YAML includes agents CRUD + secrets get/create/update
