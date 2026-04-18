# Implementation Plan: The Trail — M8 Bug Bash (Operator)

> Fix imagePullPolicy for agent and LiteLLM deployments — auto-detect `:dev`/`:latest` tags and set `Always`.

## Metadata
- **Spec:** `planning/projects/mattermost-cloud-agents/ideas/002-the-trail/spec.md`
- **Target repo:** https://github.com/mattermost/mattermost-operator
- **Worktree:** `~/workspace/worktrees/mattermost-operator-the-trail`
- **Generated:** 2026-04-08
- **Status:** draft

## Architecture Overview

The operator reconciles Agent CRs into K8s resources (Deployment, Service, NetworkPolicy, PVC). `GenerateAgentDeployment()` in `pkg/mattermost/agent.go` builds the pod spec but currently sets no `ImagePullPolicy` on either the agent or LiteLLM containers. K8s defaults non-`:latest` tags to `IfNotPresent`, causing stale `:dev` images on k3d nodes. The `MattermostSpec` type already has an `ImagePullPolicy` field (line 91 of `mattermost_types.go`) as precedent.

## Phases

### Phase 1: ImagePullPolicy Auto-Detection (Bug 6)

**Goal:** Set `imagePullPolicy: Always` for `:dev`/`:latest`/no-tag images, `IfNotPresent` for all others. Apply to both agent and LiteLLM containers.
**Depends on:** none

#### Tasks

- [ ] **1.1 Add `imageTagNeedsAlwaysPull` helper**
  - **Files:** `pkg/mattermost/agent.go`
  - **Action:** Modify
  - **Details:** Add before `GenerateAgentDeployment`:
    ```go
    func imageTagNeedsAlwaysPull(image string) bool {
        if idx := strings.LastIndex(image, ":"); idx >= 0 {
            tag := image[idx+1:]
            return tag == "dev" || tag == "latest"
        }
        return true // no tag = K8s treats as :latest
    }
    ```
    Both `agent.go` and `litellm.go` are in `package mattermost` — callable from either file.

- [ ] **1.2 Apply ImagePullPolicy in `GenerateAgentDeployment`**
  - **Files:** `pkg/mattermost/agent.go`
  - **Action:** Modify
  - **Details:** Before the container struct (line ~187), compute policy:
    ```go
    pullPolicy := corev1.PullIfNotPresent
    if imageTagNeedsAlwaysPull(agent.Spec.Image) {
        pullPolicy = corev1.PullAlways
    }
    ```
    Add `ImagePullPolicy: pullPolicy` to the `corev1.Container` struct after `Image: agent.Spec.Image` (line ~189).

- [ ] **1.3 Apply ImagePullPolicy in `GenerateLiteLLMDeployment`**
  - **Files:** `pkg/mattermost/litellm.go`
  - **Action:** Modify
  - **Details:** The `image` parameter is passed into the function. Apply the same pattern:
    ```go
    liteLLMPullPolicy := corev1.PullIfNotPresent
    if imageTagNeedsAlwaysPull(image) {
        liteLLMPullPolicy = corev1.PullAlways
    }
    ```
    Add `ImagePullPolicy: liteLLMPullPolicy` to the LiteLLM container spec (lines ~117-148). Note: LiteLLM pinned to `v1.82.0-stable` will correctly get `IfNotPresent`.

- [ ] **1.4 Add unit tests**
  - **Files:** `pkg/mattermost/agent_test.go`
  - **Action:** Modify
  - **Details:** Add tests following the `TestGenerateAgentDeployment_WithStorage` pattern:
    - `TestImageTagNeedsAlwaysPull`: table-driven — `myimage:dev` → true, `myimage:latest` → true, `myimage` → true, `registry:5000/path/img:dev` → true, `myimage:v1.2.3` → false, `myimage:stable` → false
    - `TestGenerateAgentDeployment_ImagePullPolicy`: agent with `:dev` image → assert `PullAlways`; agent with `:v1.0.0` → assert `PullIfNotPresent`
    - Update existing `TestGenerateAgentDeployment` (uses `testAgent` with `:latest`) to also assert `ImagePullPolicy == PullAlways`

#### Definition of Done
- [ ] `imageTagNeedsAlwaysPull` returns correct policy for dev/latest/versioned/no-tag
- [ ] Agent Deployment has `imagePullPolicy: Always` for `:dev`/`:latest`
- [ ] Agent Deployment has `imagePullPolicy: IfNotPresent` for versioned tags
- [ ] LiteLLM Deployment has correct `imagePullPolicy` based on its image tag
- [ ] All new and existing tests pass: `go test ./pkg/mattermost/...`

---

## File Change Map

| File | Phase(s) | Action | Summary |
|------|----------|--------|---------|
| `pkg/mattermost/agent.go` | 1 | Modify | Add `imageTagNeedsAlwaysPull` helper; set `ImagePullPolicy` on agent container |
| `pkg/mattermost/litellm.go` | 1 | Modify | Set `ImagePullPolicy` on LiteLLM container |
| `pkg/mattermost/agent_test.go` | 1 | Modify | Add helper + deployment imagePullPolicy tests; update existing test |

## Testing Strategy

- **Unit tests:** `go test ./pkg/mattermost/...`
- **Integration:** In k3d dev cluster — delete and recreate an agent pod with `:dev` image, verify K8s pulls fresh image. Confirm LiteLLM with versioned tag uses `IfNotPresent`.

## Definition of Done (Overall)

- [ ] Agent and LiteLLM deployments have correct `imagePullPolicy` based on image tag
- [ ] All tests pass
- [ ] No CRD changes needed (auto-detection only)
