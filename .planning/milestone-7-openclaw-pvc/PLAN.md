# Implementation Plan: The Trail — M7 Operator Changes

> PVC support, init container removal, and configurable `allow` egress policy for Trail agent pods.

## Metadata
- **Spec:** `projects/mattermost-cloud-agents/ideas/002-the-trail/spec.md` in planning repo
- **Target repo:** https://github.com/mattermost/mattermost-operator
- **Worktree:** `~/workspace/worktrees/mattermost-operator-the-trail`
- **Generated:** 2026-04-07
- **Status:** draft

## Architecture Overview

The operator manages Trail agent pods via an `Agent` CRD. The reconciler in `controllers/mattermost/agent/` creates ServiceAccount, Hook Secret, Service, Deployment, and NetworkPolicy resources. Deployment generation lives in `pkg/mattermost/agent.go`. CRD types are in `apis/mattermost/v1beta1/agent_types.go` with defaults and helpers in `agent_utils.go`.

Currently, `GenerateAgentDeployment` unconditionally adds an `mmctl-auth` init container and `mmctl-config` EmptyDir volume, plus a `HOME=/tmp` env var override. These are redundant for non-Python agents and break the framework-agnostic contract. The CRD has `egressPolicy` with `deny`/`allowList` values but no `allow` mode. There is no PVC support for agent pods.

Existing patterns to follow:
- PVC creation: `pkg/resources/create_resources.go` has `CreatePvcIfNotExists` helper
- PVC volumes: `pkg/mattermost/file_store.go` (`ExternalVolumeFileStore`) shows PVC-backed Volume + VolumeMount
- OwnerReference: `AgentOwnerReference()` in `pkg/mattermost/agent.go` for cascade deletion
- Controller watches: `SetupWithManager` in `controllers/mattermost/agent/controller.go` chains `.Owns()` calls
- RBAC: `config/rbac/role.yaml` already has `persistentvolumeclaims` with `['*']` verbs

## Phases

### Phase 1: Remove mmctl Init Container + HOME Override
**Goal:** Remove the redundant `mmctl-auth` init container, `mmctl-config` EmptyDir volume, and `HOME=/tmp` env var override from `GenerateAgentDeployment`. This unblocks non-Python agents (OpenClaw).
**Depends on:** none

#### Tasks
- [ ] **1.1 Remove init container from GenerateAgentDeployment**
  - **Files:** `pkg/mattermost/agent.go`
  - **Action:** Modify
  - **Details:** Delete the `InitContainers` block (lines ~153-179) that creates the `mmctl-auth` init container. Set `InitContainers: nil` (or omit entirely) in the PodSpec.

- [ ] **1.2 Remove mmctl-config EmptyDir volume**
  - **Files:** `pkg/mattermost/agent.go`
  - **Action:** Modify
  - **Details:** Remove the `mmctl-config` entry from `Volumes` and its corresponding `VolumeMount` from the main container. Keep only the `bot-token` Secret volume.

- [ ] **1.3 Remove HOME=/tmp env var override**
  - **Files:** `pkg/mattermost/agent.go`
  - **Action:** Modify
  - **Details:** Remove the `corev1.EnvVar{Name: "HOME", Value: "/tmp"}` that is appended to the main container's Env (line ~185). Each agent image owns its own HOME.

- [ ] **1.4 Update unit tests**
  - **Files:** `pkg/mattermost/agent_test.go`, `controllers/mattermost/agent/agent_test.go`
  - **Action:** Modify
  - **Details:** Update `TestGenerateAgentDeployment` to assert: no init containers, only 1 volume (`bot-token`), only 1 volume mount (`bot-token`), no `HOME` env var in the container env list, no `mmctl-config` volume mount. Update `TestCheckAgentDeployment` similarly (currently asserts 2 volumes and 1 init container).

#### Definition of Done
- [ ] `GenerateAgentDeployment` produces a Deployment with zero init containers
- [ ] Only `bot-token` volume remains; `mmctl-config` EmptyDir is gone
- [ ] No `HOME=/tmp` env var in the main container
- [ ] All existing tests pass with updated assertions
- [ ] `make generate manifests` succeeds

### Phase 2: PVC Support
**Goal:** Add optional persistent storage to agent pods via a new `Storage` CRD field, PVC creation in the reconciler, and Volume/VolumeMount injection in the Deployment.
**Depends on:** Phase 1 (clean deployment without init container simplifies volume management)

#### Tasks
- [ ] **2.1 Add AgentStorageConfig type and Storage field to CRD**
  - **Files:** `apis/mattermost/v1beta1/agent_types.go`
  - **Action:** Modify
  - **Details:** Add new type:
    ```go
    type AgentStorageConfig struct {
        Size             resource.Quantity `json:"size"`
        StorageClassName *string           `json:"storageClassName,omitempty"`
        MountPath        string            `json:"mountPath,omitempty"`
    }
    ```
    Add field to `AgentSpec`: `Storage *AgentStorageConfig \`json:"storage,omitempty"\``
    Import `"k8s.io/apimachinery/pkg/api/resource"` (already imported in agent_utils.go, but needs to be in agent_types.go).

- [ ] **2.2 Add defaults for storage fields**
  - **Files:** `apis/mattermost/v1beta1/agent_utils.go`
  - **Action:** Modify
  - **Details:** In `SetDefaults()`, after existing defaults, add:
    ```go
    if a.Spec.Storage != nil && a.Spec.Storage.MountPath == "" {
        a.Spec.Storage.MountPath = "/data"
    }
    ```
    Add constant `AgentStorageDefaultMountPath = "/data"`.
    Add helper: `func (a *Agent) StoragePVCName() string { return a.Name + "-storage" }`.

- [ ] **2.3 Add checkAgentPVC reconciler step**
  - **Files:** `controllers/mattermost/agent/agent.go`
  - **Action:** Modify
  - **Details:** Add new function `checkAgentPVC` that:
    - Returns nil immediately if `agent.Spec.Storage == nil`
    - Builds a `*corev1.PersistentVolumeClaim` with name `agent.StoragePVCName()`, namespace, labels via `AgentLabels`, OwnerReferences via `AgentOwnerReference`, `ReadWriteOnce` access mode, requested size from `agent.Spec.Storage.Size`, optional `StorageClassName`
    - Calls `r.Resources.CreatePvcIfNotExists(agent, pvc, reqLogger)`
    - Gets the current PVC and calls `r.Resources.Update(current, desired, reqLogger)`

- [ ] **2.4 Add checkAgentPVC call to reconciler**
  - **Files:** `controllers/mattermost/agent/controller.go`
  - **Action:** Modify
  - **Details:** Add `r.checkAgentPVC(ctx, agent, reqLogger)` call BEFORE `r.checkAgentDeployment` (the PVC must exist before the Deployment references it). Follow the same error-handling pattern as other check functions.

- [ ] **2.5 Add Owns PVC to controller setup**
  - **Files:** `controllers/mattermost/agent/controller.go`
  - **Action:** Modify
  - **Details:** In `SetupWithManager`, add `.Owns(&corev1.PersistentVolumeClaim{})` to the controller builder chain.

- [ ] **2.6 Inject Volume + VolumeMount in GenerateAgentDeployment**
  - **Files:** `pkg/mattermost/agent.go`
  - **Action:** Modify
  - **Details:** After building the base deployment, conditionally append a PVC-backed Volume and VolumeMount when `agent.Spec.Storage != nil`:
    ```go
    if agent.Spec.Storage != nil {
        volumes = append(volumes, corev1.Volume{
            Name: "agent-storage",
            VolumeSource: corev1.VolumeSource{
                PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
                    ClaimName: agent.StoragePVCName(),
                },
            },
        })
        volumeMounts = append(volumeMounts, corev1.VolumeMount{
            Name:      "agent-storage",
            MountPath: agent.Spec.Storage.MountPath,
        })
    }
    ```
    Refactor the Deployment builder to use a `volumes` and `volumeMounts` slice that is populated before constructing the pod spec.

- [ ] **2.7 Run make generate manifests**
  - **Files:** CRD manifests under `config/crd/`
  - **Action:** Generate
  - **Details:** Run `make generate manifests` to regenerate CRD YAML from the updated types. Verify the `storage` field appears in the generated CRD.

- [ ] **2.8 Unit tests for PVC support**
  - **Files:** `pkg/mattermost/agent_test.go`, `controllers/mattermost/agent/agent_test.go`
  - **Action:** Modify
  - **Details:**
    - `TestGenerateAgentDeployment_WithStorage`: agent with Storage set → verify PVC Volume + VolumeMount present, correct claim name, correct mount path
    - `TestGenerateAgentDeployment_WithoutStorage`: agent without Storage → verify no `agent-storage` volume
    - `TestCheckAgentPVC_Creates`: reconciler creates PVC when Storage is set
    - `TestCheckAgentPVC_Skips`: reconciler skips when Storage is nil
    - `TestCheckAgentPVC_OwnerReference`: PVC has OwnerReference pointing to Agent CR
    - Verify PVC defaults (`/data` mount path when omitted)

#### Definition of Done
- [ ] `AgentStorageConfig` type exists in CRD with `size`, `storageClassName`, `mountPath`
- [ ] `SetDefaults` sets mountPath to `/data` when omitted
- [ ] Reconciler creates PVC before Deployment when Storage is set
- [ ] PVC has OwnerReference on Agent CR (cascade delete)
- [ ] Deployment has PVC Volume + VolumeMount when Storage is set
- [ ] Controller watches PVCs via `Owns`
- [ ] CRD manifests regenerated
- [ ] All tests pass

### Phase 3: Configurable Egress Policy — `allow` Mode
**Goal:** Add `allow` as a third egress policy mode that permits all outbound traffic from the agent pod.
**Depends on:** none (can be done in parallel with Phase 2)

#### Tasks
- [ ] **3.1 Add allow constant to agent_utils.go**
  - **Files:** `apis/mattermost/v1beta1/agent_utils.go`
  - **Action:** Modify
  - **Details:** Add `AgentEgressPolicyAllow = "allow"` constant alongside existing `deny` and `allowList`.

- [ ] **3.2 Update GenerateAgentNetworkPolicy for allow mode**
  - **Files:** `pkg/mattermost/agent.go`
  - **Action:** Modify
  - **Details:** In `GenerateAgentNetworkPolicy`, after building the base egress rules, add a check:
    ```go
    if agent.Spec.EgressPolicy == mmv1beta.AgentEgressPolicyAllow {
        // Allow all egress: single rule with no To selector and no Ports restriction
        egressRules = []networkingv1.NetworkPolicyEgressRule{
            {}, // empty rule = allow all egress
        }
    }
    ```
    This replaces all egress rules with a single permissive rule. Ingress restrictions are preserved (only MM + LiteLLM can reach the pod).

- [ ] **3.3 Unit tests for allow mode**
  - **Files:** `pkg/mattermost/agent_test.go`, `controllers/mattermost/agent/agent_test.go`
  - **Action:** Modify
  - **Details:**
    - `TestGenerateAgentNetworkPolicy_Allow`: agent with `egressPolicy: allow` → verify 1 egress rule with empty To/Ports (allow all)
    - `TestGenerateAgentNetworkPolicy_AllowWithLiteLLM`: agent with `allow` + LLMGateway → verify egress is still fully open, ingress still restricts to MM + LiteLLM
    - `TestCheckAgentNetworkPolicy_Allow`: reconciler creates correct NetworkPolicy for allow mode

#### Definition of Done
- [ ] `AgentEgressPolicyAllow` constant exists
- [ ] `allow` egress mode generates a NetworkPolicy that allows all outbound traffic
- [ ] Ingress restrictions are preserved in `allow` mode
- [ ] Existing `deny` and `allowList` tests still pass
- [ ] New tests validate `allow` mode behavior

## File Change Map

| File | Phase(s) | Action | Summary |
|------|----------|--------|---------|
| `apis/mattermost/v1beta1/agent_types.go` | 2 | Modify | Add `AgentStorageConfig` type, `Storage` field to `AgentSpec` |
| `apis/mattermost/v1beta1/agent_utils.go` | 2, 3 | Modify | Add storage defaults, `StoragePVCName()`, `AgentEgressPolicyAllow` constant |
| `pkg/mattermost/agent.go` | 1, 2, 3 | Modify | Remove init container/mmctl-config/HOME, add PVC volume/mount, add allow egress |
| `pkg/mattermost/agent_test.go` | 1, 2, 3 | Modify | Update deployment tests, add storage tests, add allow egress tests |
| `controllers/mattermost/agent/agent.go` | 2 | Modify | Add `checkAgentPVC` function |
| `controllers/mattermost/agent/controller.go` | 2 | Modify | Add PVC check call, add `Owns(&corev1.PersistentVolumeClaim{})` |
| `controllers/mattermost/agent/agent_test.go` | 1, 2, 3 | Modify | Update assertions for no init container, add PVC tests, add allow policy test |
| `controllers/mattermost/agent/controller_test.go` | 1 | Modify | Update full reconcile test assertions (no init container, 1 volume) |
| `config/crd/` (generated) | 2 | Generate | Regenerated CRD manifests with `storage` field |

## Testing Strategy

1. **Unit tests**: All changes have corresponding unit tests using the existing `fake.NewClientBuilder` pattern
2. **Build verification**: `make generate manifests` must succeed after CRD changes
3. **Existing test suite**: `go test ./...` must pass — no regressions in deny/allowList egress, LiteLLM integration, or deployment generation
4. **Manual cluster test**: After plugin phase, create an agent via Agent Builder with storage and allow egress, verify PVC created, verify NetworkPolicy allows all egress

## Definition of Done (Overall)
- [ ] mmctl init container removed from all agent deployments
- [ ] `HOME=/tmp` override removed
- [ ] PVC support functional: CRD field, reconciler, deployment injection, cascade delete
- [ ] `allow` egress policy generates permissive NetworkPolicy
- [ ] All existing tests pass with updated assertions
- [ ] CRD manifests regenerated
- [ ] `go test ./...` passes
