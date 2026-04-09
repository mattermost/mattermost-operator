# Phase O2+O3: CRD Cleanup + Test Updates

> Prescriptive implementation plan. Every file, struct, function, and test is specified exactly.

## Overview

Remove `LLMProviders` and `MCPServers` from the Agent CRD types. Clean up `GenerateLiteLLMDeployment` to no longer accept provider env vars. Fix all tests referencing removed types/fields.

**Depends on:** Phase O1 (remove LiteLLM business logic from reconciler). This plan assumes O1 is complete:
- `reconcileLiteLLMModels`, `reconcileLiteLLMMCPServers`, `buildProviderEnvVars`, `reconcileAgentModel`, `reconcileLiteLLMVirtualKey`, `getLiteLLMMasterKey` are deleted from `litellm.go`
- `litellm_client.go`, `litellm_client_test.go`, `litellm_test.go` are deleted
- Controller reconcile calls to deleted functions are removed
- `checkLiteLLMDeployment` passes `nil` (or empty slice) to `GenerateLiteLLMDeployment` for the `providerEnvVars` parameter (temporary O1 stub — O2 removes the parameter entirely)

---

## Phase O2: Remove CRD Fields

### Task O2.1: Remove CRD type fields from `apis/mattermost/v1beta1/agent_types.go`

**Action:** Modify existing file

#### O2.1a: Remove `MCPServers` field from `AgentSpec`

**Current** (line 64):
```go
	MCPServers      []AgentMCPServer            `json:"mcpServers,omitempty"`
```

**Action:** Delete this line entirely.

#### O2.1b: Remove `LLMProviders` field from `OperatorManagedLLMGateway`

**Current** (lines 161-175):
```go
type OperatorManagedLLMGateway struct {
	// Image is the LiteLLM proxy container image.
	// +optional
	Image string `json:"image,omitempty"`

	// Resources defines the compute resources for the LiteLLM pod.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// LLMProviders is the list of LLM providers to configure in LiteLLM.
	// +optional
	LLMProviders []LLMProvider `json:"llmProviders,omitempty"`
}
```

**New:**
```go
type OperatorManagedLLMGateway struct {
	// Image is the LiteLLM proxy container image.
	// +optional
	Image string `json:"image,omitempty"`

	// Resources defines the compute resources for the LiteLLM pod.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}
```

Delete the `LLMProviders` field and its comment (3 lines).

#### O2.1c: Delete `LLMProvider` struct

**Current** (lines 178-190):
```go
// LLMProvider defines a provider (e.g. Anthropic, OpenAI) and its models.
type LLMProvider struct {
	// Name is the provider identifier used by LiteLLM (e.g. "anthropic", "openai").
	Name string `json:"name"`

	// Secret is the name of a K8s Secret containing the provider API key.
	// The secret must have a key named "apiKey".
	Secret string `json:"secret"`

	// Models is the list of model identifiers supported by this provider.
	Models []string `json:"models"`
}
```

**Action:** Delete the entire struct and its comment (13 lines).

#### O2.1d: Delete `AgentMCPServer` struct

**Current** (lines 193-220):
```go
// AgentMCPServer defines an MCP server configuration for an agent.
type AgentMCPServer struct {
	// Name is the unique identifier for this MCP server.
	Name string `json:"name"`

	// URL is the HTTP endpoint of the MCP server.
	URL string `json:"url"`

	// CredentialSecret is the name of a K8s Secret containing the MCP server credential.
	// The secret must have a key named "apiKey".
	// +optional
	CredentialSecret string `json:"credentialSecret,omitempty"`

	// MCPAccessGroup is the LiteLLM access group for this server.
	// If empty, defaults to "<agentName>_<sanitizedServerName>".
	// +optional
	MCPAccessGroup string `json:"mcpAccessGroup,omitempty"`

	// AllowedTools restricts which tools the agent can call on this server.
	// +optional
	AllowedTools []string `json:"allowedTools,omitempty"`

	// DisallowedTools blocks specific tools on this server.
	// +optional
	DisallowedTools []string `json:"disallowedTools,omitempty"`
}
```

**Action:** Delete the entire struct and its comment (28 lines).

#### Summary of `agent_types.go` changes

| Location | Change |
|----------|--------|
| Line 64 | Delete `MCPServers` field |
| Lines 172-174 | Delete `LLMProviders` field + comment |
| Lines 178-190 | Delete `LLMProvider` struct |
| Lines 193-220 | Delete `AgentMCPServer` struct |

**Net:** ~45 lines removed.

---

### Task O2.2: Clean up `GenerateLiteLLMDeployment` in `pkg/mattermost/litellm.go`

**Action:** Modify existing file

#### O2.2a: Remove `providerEnvVars` parameter from function signature

**Current** (line 61):
```go
func GenerateLiteLLMDeployment(namespace, image string, providerEnvVars []corev1.EnvVar) *appsv1.Deployment {
```

**New:**
```go
func GenerateLiteLLMDeployment(namespace, image string) *appsv1.Deployment {
```

#### O2.2b: Remove `providerEnvVars` append

**Current** (line 79):
```go
	baseEnv = append(baseEnv, providerEnvVars...)
```

**Action:** Delete this line entirely. The LiteLLM Deployment now only has its own base env vars (DB credentials, master key ref, etc.). Provider API keys are no longer injected into the LiteLLM pod — the plugin passes them inline via the LiteLLM management API.

#### Summary of `litellm.go` changes

| Location | Change |
|----------|--------|
| Line 61 | Remove `providerEnvVars []corev1.EnvVar` parameter |
| Line 79 | Delete `baseEnv = append(baseEnv, providerEnvVars...)` |

---

### Task O2.3: Update `checkLiteLLMDeployment` in `controllers/mattermost/agent/litellm.go`

**Action:** Modify existing file

After O1, `checkLiteLLMDeployment` should look something like:

```go
func (r *AgentReconciler) checkLiteLLMDeployment(ctx context.Context, agent *mmv1beta.Agent, ...) error {
    om := agent.Spec.LLMGateway.OperatorManaged
    // O1 stub: providerEnvVars removed, passing nil
    desired := mattermost.GenerateLiteLLMDeployment(agent.Namespace, om.Image, nil)
    ...
}
```

**New** (remove the third argument):

```go
func (r *AgentReconciler) checkLiteLLMDeployment(ctx context.Context, agent *mmv1beta.Agent, ...) error {
    om := agent.Spec.LLMGateway.OperatorManaged
    desired := mattermost.GenerateLiteLLMDeployment(agent.Namespace, om.Image)
    ...
}
```

If O1 instead kept `buildProviderEnvVars` temporarily and still passes `buildProviderEnvVars(om.LLMProviders)`:

**Current (pre-O1 state)** (lines 30-55):
```go
func (r *AgentReconciler) checkLiteLLMDeployment(...) error {
    om := agent.Spec.LLMGateway.OperatorManaged
    providerEnvVars := buildProviderEnvVars(om.LLMProviders)
    ...
    desired := mattermost.GenerateLiteLLMDeployment(agent.Namespace, om.Image, providerEnvVars)
    ...
}
```

**New:**
```go
func (r *AgentReconciler) checkLiteLLMDeployment(...) error {
    om := agent.Spec.LLMGateway.OperatorManaged
    desired := mattermost.GenerateLiteLLMDeployment(agent.Namespace, om.Image)
    ...
}
```

Changes:
- Delete the `providerEnvVars := buildProviderEnvVars(om.LLMProviders)` line
- Update `GenerateLiteLLMDeployment` call to drop the third argument

If `buildProviderEnvVars` still exists at this point (O1 didn't delete it), verify it's now unused and delete it. If O1 already deleted it, no action needed.

---

### Task O2.4: Delete `buildProviderEnvVars` if still present

**Action:** Conditionally delete from `controllers/mattermost/agent/litellm.go`

If O1 deleted this function, skip. If O1 left it as a temporary stub, delete it now:

**Function to delete** (lines 276-291 in pre-O1 state):
```go
func buildProviderEnvVars(providers []mmv1beta.LLMProvider) []corev1.EnvVar {
    var envVars []corev1.EnvVar
    for _, provider := range providers {
        envVars = append(envVars, corev1.EnvVar{
            Name: strings.ToUpper(provider.Name) + "_API_KEY",
            ValueFrom: &corev1.EnvVarSource{
                SecretKeyRef: &corev1.SecretKeySelector{
                    LocalObjectReference: corev1.LocalObjectReference{Name: provider.Secret},
                    Key:                  "apiKey",
                },
            },
        })
    }
    return envVars
}
```

After deletion, clean up the `"strings"` import if no other usages remain.

---

### Task O2.5: Regenerate CRD manifests

**Action:** Run code generation

```bash
cd ~/workspace/worktrees/mattermost-operator-the-trail
make generate manifests
```

This regenerates:
- `config/crd/bases/installation.mattermost.com_agents.yaml` — CRD YAML schema

**Verify:** The regenerated CRD YAML should NOT contain:
- `llmProviders` field
- `mcpServers` field
- Any reference to `LLMProvider` or `AgentMCPServer` schema definitions

**Also verify:** The CRD YAML SHOULD still contain:
- `llmGateway` with `external` and `operatorManaged` variants
- `operatorManaged` with `image` and `resources` fields (no `llmProviders`)
- `env` array in `spec`
- `egressPolicy`, `egressAllowList`

---

## Phase O3: Update Tests

### Task O3.1: Update `controllers/mattermost/agent/agent_test.go`

**Action:** Modify existing file

#### O3.1a: Fix `TestCheckAgentDeployment_WithLLMGateway` (lines 191-263)

**Current test agent spec** (lines 196-204):
```go
agent.Spec.LLMGateway = &mmv1beta.LLMGatewayConfig{
    OperatorManaged: &mmv1beta.OperatorManagedLLMGateway{
        Image: mmv1beta.AgentLiteLLMDefaultImage,
        LLMProviders: []mmv1beta.LLMProvider{
            {
                Name:   "anthropic",
                Secret: "anthropic-key",
                Models: []string{"claude-sonnet-4-5-20250929"},
            },
        },
    },
}
```

**New:**
```go
agent.Spec.LLMGateway = &mmv1beta.LLMGatewayConfig{
    OperatorManaged: &mmv1beta.OperatorManagedLLMGateway{
        Image: mmv1beta.AgentLiteLLMDefaultImage,
    },
}
```

Remove the `LLMProviders` field entirely. The `OperatorManagedLLMGateway` struct no longer has this field.

**Check test assertions (lines 205-260):** The test asserts that the agent Deployment has LiteLLM-related env vars (`LITELLM_BASE_URL`, `OPENAI_BASE_URL`, `OPENAI_API_KEY`, etc.). These env vars come from `GenerateAgentDeployment` (which reads `spec.llmGateway.operatorManaged` to determine the LiteLLM URL and key secret), NOT from the provider env vars. These assertions should remain valid.

**However:** One assertion may check for `ANTHROPIC_API_KEY` sourced from the `anthropic-key` secret (this was a provider env var, not a gateway env var). Verify: does `GenerateAgentDeployment` inject per-provider API key env vars?

Based on the explorer's findings for `pkg/mattermost/agent.go`: `GenerateAgentDeployment` only injects gateway-level env vars (`LITELLM_BASE_URL`, `OPENAI_BASE_URL`, `OPENAI_API_KEY` from virtual key secret, `ANTHROPIC_BASE_URL`, `ANTHROPIC_API_KEY` from virtual key secret). It does NOT inject per-provider env vars from `LLMProviders`. So no provider-specific assertions need removal.

**If the test has an assertion for `ANTHROPIC_API_KEY` sourced from `anthropic-key` secret (provider secret):** Delete that assertion. The `ANTHROPIC_API_KEY` should now come from the virtual key secret only. **Implementer: verify the exact assertions in the test before deleting.**

#### O3.1b: Verify other tests are clean

Based on explorer findings:
- `TestCheckAgentNetworkPolicy_DenyWithLiteLLM` (lines 318-364): Uses `OperatorManagedLLMGateway{Image: ...}` with no `LLMProviders` — **no change needed**.
- `newTestAgent()` (lines 25-39): No `LLMProviders` or `MCPServers` — **no change needed**.
- All other test functions: **no change needed**.

---

### Task O3.2: Update `pkg/mattermost/litellm_test.go`

**Action:** Modify existing file

#### O3.2a: Fix `TestGenerateLiteLLMDeployment` call site (line 34)

**Current:**
```go
deployment := GenerateLiteLLMDeployment("my-namespace", mmv1beta.AgentLiteLLMDefaultImage, nil)
```

**New:**
```go
deployment := GenerateLiteLLMDeployment("my-namespace", mmv1beta.AgentLiteLLMDefaultImage)
```

Drop the third argument (`nil`).

#### O3.2b: Delete `TestGenerateLiteLLMDeployment_WithProviderEnvVars` (lines 117-143)

**Action:** Delete the entire test function.

This test verifies that provider env vars injected via `providerEnvVars` parameter appear in the LiteLLM Deployment's container env. Since the parameter is removed, this test is obsolete.

```go
// DELETE ENTIRELY:
func TestGenerateLiteLLMDeployment_WithProviderEnvVars(t *testing.T) {
    providerEnvVars := []corev1.EnvVar{
        {
            Name: "ANTHROPIC_API_KEY",
            ValueFrom: &corev1.EnvVarSource{
                SecretKeyRef: &corev1.SecretKeySelector{
                    LocalObjectReference: corev1.LocalObjectReference{Name: "anthropic-key"},
                    Key:                  "apiKey",
                },
            },
        },
    }
    deployment := GenerateLiteLLMDeployment("my-namespace", mmv1beta.AgentLiteLLMDefaultImage, providerEnvVars)

    container := deployment.Spec.Template.Spec.Containers[0]
    // ... assertions about ANTHROPIC_API_KEY in container.Env ...
}
```

#### O3.2c: Verify other tests in `litellm_test.go`

Based on explorer findings, these tests are clean:
- `TestGenerateLiteLLMConfigMap` — no `providerEnvVars` param — **no change**
- `TestGenerateLiteLLMService` — **no change**
- `TestGenerateAgentLiteLLMKeySecret` — **no change**
- `TestLiteLLMServiceURL` — **no change**
- All `TestGenerateAgentDeployment_*` tests — **no change** (they test `GenerateAgentDeployment`, not `GenerateLiteLLMDeployment`)
- Network policy tests — **no change**

---

### Task O3.3: Verify `pkg/mattermost/agent_test.go` is clean

Based on explorer findings: No references to `LLMProviders` or `MCPServers` in test agent specs. All test agents use `OperatorManagedLLMGateway{Image: ...}` only. **No changes needed.**

---

### Task O3.4: Verify no compilation errors from removed types

After O2.1 deletes `LLMProvider` and `AgentMCPServer` structs, any remaining references will cause compile errors. Run:

```bash
cd ~/workspace/worktrees/mattermost-operator-the-trail
go build ./...
```

If compilation fails, grep for remaining references:

```bash
grep -rn 'LLMProvider\|AgentMCPServer\|MCPServers\|LLMProviders\|buildProviderEnvVars' --include='*.go' .
```

Fix any remaining references. Based on the explorer's analysis, the only references outside the deleted code are:
- `agent_types.go` — removed in O2.1
- `checkLiteLLMDeployment` — updated in O2.3
- `TestCheckAgentDeployment_WithLLMGateway` — updated in O3.1
- `TestGenerateLiteLLMDeployment_WithProviderEnvVars` — deleted in O3.2

If O1 properly cleaned up `controller.go` and `litellm.go`, there should be no other references.

---

## Gotchas and Edge Cases

### 1. O1 completion state is uncertain
The plan assumes O1 has been fully implemented. If O1 left `buildProviderEnvVars` as a temporary stub (passing `nil` to `GenerateLiteLLMDeployment`), O2 still works — it just removes the stub. If O1 deleted `buildProviderEnvVars` entirely, the call site in `checkLiteLLMDeployment` was already updated. **Implementer: read the actual state of these files after O1 before applying O2 changes.**

### 2. `OperatorManagedLLMGateway` is NOT deleted — only simplified
The struct retains `Image` and `Resources` fields. The operator still needs this type to know which LiteLLM image to deploy and what resources to allocate. Only the `LLMProviders` field is removed.

### 3. `ExternalLLMGateway` is untouched
The `External` variant (`url` + `virtualKeySecret`) is the new path used by the plugin's `buildAgentCR`. It remains unchanged.

### 4. `SetDefaults()` in `agent_utils.go` may reference `LLMProviders`
Check lines 54-58 of `agent_utils.go` for any `LLMProviders` default logic. Based on explorer findings: no defaults for `LLMProviders` are set — `SetDefaults()` only sets the default LiteLLM image. **No change needed.**

### 5. `make generate manifests` requires controller-gen
The CRD regeneration command depends on `controller-gen` being installed. The operator repo's `Makefile` handles this. Run from the repo root.

### 6. Existing Agent CRs with `llmProviders`/`mcpServers` in the cluster
After the CRD is updated, existing Agent CRs that have these fields will have them silently ignored (Kubernetes preserves unknown fields with `x-kubernetes-preserve-unknown-fields` or drops them depending on schema validation settings). The operator should handle this gracefully — it no longer reads these fields. **No migration needed** — the fields simply become no-ops.

### 7. `SanitizeMCPServerName` function in `litellm_client.go`
This was exported and may be referenced elsewhere. Since `litellm_client.go` is deleted in O1, verify no other files import it. Based on explorer findings: `SanitizeMCPServerName` is only used within `litellm.go`'s `reconcileLiteLLMMCPServers` (deleted in O1). **No external references.**

### 8. `reconcileAgentModel` and `reconcileLiteLLMVirtualKey` deletion in O1
These functions were deleted by O1 (the plugin now handles agent-as-model registration and virtual key provisioning). If O1 missed either, they need to be deleted before O2 since they reference types being removed. **Implementer: verify they're gone.**

---

## Execution Order

1. **Task O2.1** (remove CRD type fields) — no dependencies within O2
2. **Task O2.2** (clean up `GenerateLiteLLMDeployment`) — no dependencies within O2
3. **Task O2.3** (update `checkLiteLLMDeployment` call) — depends on O2.2
4. **Task O2.4** (delete `buildProviderEnvVars` if present) — depends on O2.3
5. **Task O2.5** (regenerate CRD manifests) — depends on O2.1
6. **Task O3.1** (fix agent_test.go) — depends on O2.1 (type removal)
7. **Task O3.2** (fix litellm_test.go) — depends on O2.2 (signature change)
8. **Task O3.3** (verify agent_test.go is clean) — depends on O2.1
9. **Task O3.4** (verify no compilation errors) — depends on all above

**Practical recommendation:** 

O2.1 + O2.2 in parallel → O2.3 + O2.4 → O2.5 → O3.1 + O3.2 in parallel → O3.4 (build + test verification).

Or more simply: Apply all O2 changes → `go build ./...` to catch issues → Apply O3 test fixes → `make generate manifests` → `go test ./...` to verify.

---

## Verification

```bash
cd ~/workspace/worktrees/mattermost-operator-the-trail

# Build
go build ./...

# Vet
go vet ./...

# Regenerate CRD
make generate manifests

# Verify CRD changes
grep -c 'llmProviders\|mcpServers' config/crd/bases/installation.mattermost.com_agents.yaml
# Expected: 0

# Verify no remaining references to deleted types
grep -rn 'LLMProvider\b\|AgentMCPServer\|MCPServers\|buildProviderEnvVars' --include='*.go' .
# Expected: 0 matches (or only in vendor/)

# Run all tests
go test ./...
```

---

## Files Modified/Created

| File | Action | Lines Changed (approx) |
|------|--------|----------------------|
| `apis/mattermost/v1beta1/agent_types.go` | **Modify** | -45 lines (delete 2 structs + 2 fields) |
| `pkg/mattermost/litellm.go` | **Modify** | -2 lines (remove param + append) |
| `controllers/mattermost/agent/litellm.go` | **Modify** | -2 lines (remove providerEnvVars call + arg) |
| `controllers/mattermost/agent/litellm.go` | **Conditionally modify** | -16 lines (delete `buildProviderEnvVars` if O1 didn't) |
| `config/crd/bases/installation.mattermost.com_agents.yaml` | **Regenerate** | auto |
| `controllers/mattermost/agent/agent_test.go` | **Modify** | -7 lines (remove LLMProviders from test spec) |
| `pkg/mattermost/litellm_test.go` | **Modify** | -27 lines (delete test + fix call site) |

---

## Implementation Summary (completed 2026-04-07)

### O2.1: agent_types.go
- Deleted `MCPServers` field from `AgentSpec`
- Deleted `LLMProviders` field from `OperatorManagedLLMGateway`
- Deleted `LLMProvider` struct (13 lines)
- Deleted `AgentMCPServer` struct (28 lines)

### O2.2: pkg/mattermost/litellm.go
- Removed `providerEnvVars []corev1.EnvVar` parameter from `GenerateLiteLLMDeployment`
- Deleted `baseEnv = append(baseEnv, providerEnvVars...)` line

### O2.3: controllers/mattermost/agent/litellm.go
- Updated `GenerateLiteLLMDeployment` call to drop third argument (was `nil` from O1)

### O2.4: `buildProviderEnvVars` already deleted in O1 — no action needed.

### O2.5: CRD regeneration
- Ran `make generate manifests` — regenerated deepcopy and CRD YAML
- CRD has 0 references to `llmProviders`/`mcpServers`

### O3.1: controllers/mattermost/agent/agent_test.go
- Removed `LLMProviders` from `TestCheckAgentDeployment_WithLLMGateway` test fixture
- Added `testLogger()` helper (was defined in deleted `litellm_test.go` from O1)

### O3.2: pkg/mattermost/litellm_test.go
- Updated `GenerateLiteLLMDeployment` call to 2-arg form
- Deleted `TestGenerateLiteLLMDeployment_WithProviderEnvVars` test

### Deviations from plan
- Added `testLogger()` to `agent_test.go` — this helper was defined in `litellm_test.go` which was deleted in O1. The plan didn't account for relocating it.

### Verification
- `go build ./...` — passes
- `go vet ./...` — passes
- `go test ./controllers/mattermost/agent/... ./pkg/mattermost/...` — all pass
- CRD grep: 0 matches for `llmProviders`/`mcpServers`
- Code grep: 0 matches for `LLMProvider`/`AgentMCPServer`/`MCPServers`/`buildProviderEnvVars`
- No commits created (per instructions)
