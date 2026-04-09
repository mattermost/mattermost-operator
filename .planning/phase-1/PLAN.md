# Phase 1: ImagePullPolicy Auto-Detection (Bug 6)

> **Status:** ready
> **Milestone:** M8 — Bug Bash
> **Target repo:** `~/workspace/worktrees/mattermost-operator-the-trail`
> **Files changed:** 3 (`agent.go`, `litellm.go`, `agent_test.go`)

---

## Context: Current State

`GenerateAgentDeployment` in `pkg/mattermost/agent.go` (line 68) builds a Deployment with no `ImagePullPolicy` on the agent container (line 190). `GenerateLiteLLMDeployment` in `pkg/mattermost/litellm.go` (line 59) similarly omits `ImagePullPolicy` on the LiteLLM container (line 120). K8s defaults non-`:latest` tags to `IfNotPresent`, causing stale `:dev` images on k3d nodes.

**Key facts from codebase research:**
- `strings` is already imported in `agent.go` (line 4) — no import changes needed
- Both files are in `package mattermost` — a helper in `agent.go` is callable from `litellm.go`
- `testAgent` helper (line 15 of `agent_test.go`) uses image `"mattermost/test-agent:latest"`
- The existing `TestGenerateAgentDeployment` (line 81) does NOT assert `ImagePullPolicy` — needs update
- `MattermostSpec` already has an `ImagePullPolicy` field (line 91 of `mattermost_types.go`) as precedent

---

## Task 1.1: Add `imageTagNeedsAlwaysPull` helper

**File:** `pkg/mattermost/agent.go`
**Action:** Insert new function between `mmServerURL` (ends line 65) and `GenerateAgentDeployment` (starts line 68).
**Import changes:** None — `strings` is already imported at line 4.

**Insert at line 67** (add a blank line after line 65, then the function):

```go
// imageTagNeedsAlwaysPull returns true if the image tag is "dev", "latest",
// or absent (K8s treats no-tag as :latest). Used to auto-set ImagePullPolicy.
func imageTagNeedsAlwaysPull(image string) bool {
	if idx := strings.LastIndex(image, ":"); idx >= 0 {
		tag := image[idx+1:]
		return tag == "dev" || tag == "latest"
	}
	return true // no tag = K8s treats as :latest
}
```

After insertion, `GenerateAgentDeployment` shifts to approximately line 77.

---

## Task 1.2: Apply ImagePullPolicy in `GenerateAgentDeployment`

**File:** `pkg/mattermost/agent.go`
**Action:** Two modifications inside `GenerateAgentDeployment`.

### Step A — Compute pull policy

Insert 4 lines immediately before the `return &appsv1.Deployment{` statement (currently line 169):

```go
	pullPolicy := corev1.PullIfNotPresent
	if imageTagNeedsAlwaysPull(agent.Spec.Image) {
		pullPolicy = corev1.PullAlways
	}
```

### Step B — Add `ImagePullPolicy` field to container struct

In the `corev1.Container` literal, add `ImagePullPolicy: pullPolicy` after the `Image` field.

**Current code** (lines 188-191):
```go
						{
							Name:  mmv1beta.AgentContainerName,
							Image: agent.Spec.Image,
							Env:   envVars,
```

**Replace with:**
```go
						{
							Name:            mmv1beta.AgentContainerName,
							Image:           agent.Spec.Image,
							ImagePullPolicy: pullPolicy,
							Env:             envVars,
```

---

## Task 1.3: Apply ImagePullPolicy in `GenerateLiteLLMDeployment`

**File:** `pkg/mattermost/litellm.go`
**Action:** Two modifications inside `GenerateLiteLLMDeployment`.
**Import changes:** None — `imageTagNeedsAlwaysPull` is in the same package, `strings` is not needed in this file.

### Step A — Compute pull policy

Insert 4 lines immediately before the `return &appsv1.Deployment{` statement (currently line 101):

```go
	liteLLMPullPolicy := corev1.PullIfNotPresent
	if imageTagNeedsAlwaysPull(image) {
		liteLLMPullPolicy = corev1.PullAlways
	}
```

### Step B — Add `ImagePullPolicy` field to container struct

In the `corev1.Container` literal, add `ImagePullPolicy: liteLLMPullPolicy` after the `Image` field.

**Current code** (lines 119-121):
```go
							Name:  "litellm",
							Image: image,
							Args:  []string{"--config", "/app/config/config.yaml"},
```

**Replace with:**
```go
							Name:            "litellm",
							Image:           image,
							ImagePullPolicy: liteLLMPullPolicy,
							Args:            []string{"--config", "/app/config/config.yaml"},
```

**Note:** LiteLLM is currently pinned to `ghcr.io/berriai/litellm:v1.82.0-stable` — this will correctly resolve to `PullIfNotPresent`.

---

## Task 1.4: Add and update unit tests

**File:** `pkg/mattermost/agent_test.go`

### 1.4a: Update existing `TestGenerateAgentDeployment`

**Insert after line 104** (after `assert.Equal(t, agent.Spec.Resources, c.Resources)`):

```go
	// ImagePullPolicy — testAgent uses :latest, should be PullAlways
	assert.Equal(t, corev1.PullAlways, c.ImagePullPolicy, "latest tag should get PullAlways")
```

### 1.4b: Add `TestImageTagNeedsAlwaysPull` — table-driven

**Append after the last test function** (after `TestGenerateAgentNetworkPolicy_AllowWithLiteLLM` ending at line 394):

```go
func TestImageTagNeedsAlwaysPull(t *testing.T) {
	tests := []struct {
		image    string
		expected bool
	}{
		{"myimage:dev", true},
		{"myimage:latest", true},
		{"myimage", true},                       // no tag → treat as :latest
		{"registry:5000/path/img:dev", true},     // registry with port + :dev tag
		{"myimage:v1.2.3", false},
		{"myimage:stable", false},
		{"ghcr.io/org/litellm:v1.82.0-stable", false},
	}
	for _, tt := range tests {
		t.Run(tt.image, func(t *testing.T) {
			assert.Equal(t, tt.expected, imageTagNeedsAlwaysPull(tt.image))
		})
	}
}
```

### 1.4c: Add `TestGenerateAgentDeployment_ImagePullPolicy`

**Append after `TestImageTagNeedsAlwaysPull`:**

```go
func TestGenerateAgentDeployment_ImagePullPolicy(t *testing.T) {
	t.Run("dev tag gets PullAlways", func(t *testing.T) {
		agent := testAgent("my-agent", "default")
		agent.Spec.Image = "mattermost/test-agent:dev"
		dep := GenerateAgentDeployment(agent)
		assert.Equal(t, corev1.PullAlways, dep.Spec.Template.Spec.Containers[0].ImagePullPolicy)
	})

	t.Run("latest tag gets PullAlways", func(t *testing.T) {
		agent := testAgent("my-agent", "default")
		// testAgent already uses :latest
		dep := GenerateAgentDeployment(agent)
		assert.Equal(t, corev1.PullAlways, dep.Spec.Template.Spec.Containers[0].ImagePullPolicy)
	})

	t.Run("no tag gets PullAlways", func(t *testing.T) {
		agent := testAgent("my-agent", "default")
		agent.Spec.Image = "mattermost/test-agent"
		dep := GenerateAgentDeployment(agent)
		assert.Equal(t, corev1.PullAlways, dep.Spec.Template.Spec.Containers[0].ImagePullPolicy)
	})

	t.Run("versioned tag gets PullIfNotPresent", func(t *testing.T) {
		agent := testAgent("my-agent", "default")
		agent.Spec.Image = "mattermost/test-agent:v1.0.0"
		dep := GenerateAgentDeployment(agent)
		assert.Equal(t, corev1.PullIfNotPresent, dep.Spec.Template.Spec.Containers[0].ImagePullPolicy)
	})
}
```

---

## Verification

```bash
cd ~/workspace/worktrees/mattermost-operator-the-trail

# Run targeted tests
go test ./pkg/mattermost/... -v -run "TestImageTagNeedsAlwaysPull|TestGenerateAgentDeployment"

# Run full package tests to catch regressions
go test ./pkg/mattermost/... -count=1
```

**Expected results:**
- `TestImageTagNeedsAlwaysPull` — 7 subtests pass
- `TestGenerateAgentDeployment` — passes with new `ImagePullPolicy` assertion
- `TestGenerateAgentDeployment_ImagePullPolicy` — 4 subtests pass
- All existing tests unchanged and passing

---

## Edge Cases & Gotchas

1. **`registry:5000/path/img:dev`** — `strings.LastIndex(image, ":")` correctly finds the `:dev` tag (not the `:5000` port separator), because LastIndex returns the rightmost `:`.

2. **No CRD changes needed.** This is auto-detection logic only — no new fields on AgentSpec.

3. **Existing tests that don't need changes:**
   - `TestGenerateAgentDeployment_CustomEnvVars` (line 152) — doesn't assert ImagePullPolicy
   - `TestGenerateAgentDeployment_WithStorage` (line 175) — doesn't assert ImagePullPolicy
   - `TestGenerateAgentDeployment_WithoutStorage` (line 201) — doesn't assert ImagePullPolicy

4. **`litellm.go` has no tests for `GenerateLiteLLMDeployment`.** Adding LiteLLM deployment tests is out of scope for this bug fix. The `TestGenerateAgentDeployment_ImagePullPolicy` tests validate the helper logic; LiteLLM just calls the same helper.

---

## Commit

After all tests pass, create a single local commit. Do **not** push.

```
fix(agent): auto-detect imagePullPolicy for agent and LiteLLM deployments

Set imagePullPolicy to Always for :dev, :latest, and untagged images.
Versioned tags default to IfNotPresent. Fixes stale image pulls on k3d
nodes during development.
```

---

## Definition of Done

- [x] `imageTagNeedsAlwaysPull` returns correct policy for dev/latest/versioned/no-tag images
- [x] Agent Deployment container has `ImagePullPolicy: Always` for `:dev`/`:latest`/no-tag
- [x] Agent Deployment container has `ImagePullPolicy: IfNotPresent` for versioned tags
- [x] LiteLLM Deployment container has correct `ImagePullPolicy` based on its image tag
- [x] All new and existing tests pass: `go test ./pkg/mattermost/...`

---

## Implementation Summary

**Implemented on:** 2026-04-08

### Changes Made

1. **`pkg/mattermost/agent.go`** — Added `imageTagNeedsAlwaysPull(image string) bool` helper between `mmServerURL` and `GenerateAgentDeployment`. Computes `pullPolicy` before the Deployment return statement and sets `ImagePullPolicy: pullPolicy` on the agent container.

2. **`pkg/mattermost/litellm.go`** — Computes `liteLLMPullPolicy` before the Deployment return statement using the same `imageTagNeedsAlwaysPull` helper (same package). Sets `ImagePullPolicy: liteLLMPullPolicy` on the LiteLLM container.

3. **`pkg/mattermost/agent_test.go`** — Added `ImagePullPolicy` assertion to existing `TestGenerateAgentDeployment`. Added `TestImageTagNeedsAlwaysPull` (7 table-driven subtests) and `TestGenerateAgentDeployment_ImagePullPolicy` (4 subtests covering dev/latest/no-tag/versioned).

### Test Results

All tests pass: `go test ./pkg/mattermost/... -count=1` — 0 failures, no regressions.
