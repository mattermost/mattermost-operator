# Implementation Plan: M6 — Operator Cleanup (Remove LiteLLM Business Logic)

> Remove all LiteLLM business logic from the operator reconciler — model registration, MCP server registration, virtual key creation, and agent-as-model registration. The operator retains only infrastructure concerns: deploy LiteLLM, deploy agent pods, inject env vars, network policy.

## Metadata
- **Spec:** `projects/mattermost-cloud-agents/ideas/002-the-trail/spec.md`
- **Target repo:** https://github.com/mattermost/mattermost-operator
- **Generated:** 2026-04-06
- **Status:** draft

## Architecture Overview

The operator's agent controller (`controllers/mattermost/agent/controller.go`) has a reconcile loop that currently handles both infrastructure and business logic. M6 strips the business logic (model/MCP/key management via LiteLLM API calls), leaving only infrastructure: deploy LiteLLM Deployment/Service/ConfigMap, deploy agent pods, health checks, env var injection, NetworkPolicy.

Key files:
- `controllers/mattermost/agent/controller.go` — main reconcile loop
- `controllers/mattermost/agent/litellm.go` — LiteLLM reconcile functions (partially removed)
- `controllers/mattermost/agent/litellm_client.go` — HTTP client (fully removed, ported to plugin)
- `apis/mattermost/v1beta1/agent_types.go` — CRD types (fields removed)
- `pkg/mattermost/litellm.go` — LiteLLM K8s resource generators (signature change)

## Phases

### Phase O1: Remove LiteLLM Business Logic from Reconciler

**Goal:** Strip model/MCP/key reconcile calls from the controller, delete the LiteLLM HTTP client.
**Depends on:** Plugin Phases 1-4 must be complete and deployed.

#### Tasks

- [ ] **O1.1 Strip LiteLLM reconcile calls from `controller.go`**
  - **Files:** `controllers/mattermost/agent/controller.go`
  - **Action:** Modify
  - **Details:** In `Reconcile` method: remove the pre-deploy block at lines 129-137 (`reconcileLiteLLMModels` + first `reconcileLiteLLMMCPServers` call). Remove the entire post-health block at lines 193-217 (`reconcileAgentModel`, second `reconcileLiteLLMMCPServers`, `reconcileLiteLLMVirtualKey`). Remove `getLiteLLMMasterKey` calls and `litellmURL`/`masterKey` variable declarations that become unused. Keep: `checkLiteLLMDeployment`, `checkLiteLLMService`, `checkLiteLLMReady` (lines 103-120).

- [ ] **O1.2 Delete business logic functions from `litellm.go`**
  - **Files:** `controllers/mattermost/agent/litellm.go`
  - **Action:** Modify
  - **Details:** Delete functions: `reconcileLiteLLMModels` (line 138), `reconcileAgentModel` (line 194), `reconcileLiteLLMVirtualKey` (line 228), `reconcileLiteLLMMCPServers` (line 303), `buildProviderEnvVars` (line 277), `getLiteLLMMasterKey` (line 118). Keep: `checkLiteLLMDeployment` (line 30), `checkLiteLLMService` (line 77), `checkLiteLLMReady` (line 100). Clean up unused imports.

- [ ] **O1.3 Delete `litellm_client.go` and its tests**
  - **Files:** `controllers/mattermost/agent/litellm_client.go`, `controllers/mattermost/agent/litellm_client_test.go`, `controllers/mattermost/agent/litellm_test.go`
  - **Action:** Delete
  - **Details:** The entire LiteLLM HTTP client is now superseded by the plugin's `LiteLLMClient`. Delete the client file, its unit tests, and the reconcile function tests. All model/MCP/key management logic now lives in the plugin.

#### Definition of Done
- [ ] `controller.go` compiles with no LiteLLM API calls
- [ ] Only infrastructure functions remain in `litellm.go`
- [ ] `litellm_client.go` deleted
- [ ] `go build ./controllers/...` succeeds
- [ ] `go vet ./...` passes

---

### Phase O2: Remove CRD Fields

**Goal:** Remove `llmProviders` and `mcpServers` from the Agent CRD. Clean up dependent code.
**Depends on:** Phase O1. Delete and recreate dev agents first.

#### Tasks

- [ ] **O2.1 Remove CRD type fields from `agent_types.go`**
  - **Files:** `apis/mattermost/v1beta1/agent_types.go`
  - **Action:** Modify
  - **Details:** Remove `LLMProviders []LLMProvider` field from `OperatorManagedLLMGateway` (line 173). Delete `LLMProvider` struct (lines 177-190). Delete `AgentMCPServer` struct (lines 192-220). Remove `MCPServers []AgentMCPServer` from `AgentSpec` (lines 63-65).

- [ ] **O2.2 Clean up `GenerateLiteLLMDeployment` signature**
  - **Files:** `pkg/mattermost/litellm.go`, `controllers/mattermost/agent/litellm.go`
  - **Action:** Modify
  - **Details:** In `pkg/mattermost/litellm.go`: remove `providerEnvVars []corev1.EnvVar` parameter from `GenerateLiteLLMDeployment` (line 61). Remove `baseEnv = append(baseEnv, providerEnvVars...)` (line 79). In `controllers/mattermost/agent/litellm.go` `checkLiteLLMDeployment`: update the call to `GenerateLiteLLMDeployment` to remove the `providerEnvVars` argument.

- [ ] **O2.3 Regenerate CRD manifests**
  - **Files:** `config/crd/bases/installation.mattermost.com_agents.yaml`
  - **Action:** Regenerate
  - **Details:** Run `make generate manifests` in the operator repo root. Verify the CRD YAML no longer contains `llmProviders` or `mcpServers` fields.

#### Definition of Done
- [ ] CRD types no longer contain `LLMProvider`, `AgentMCPServer`, `llmProviders`, `mcpServers`
- [ ] `GenerateLiteLLMDeployment` no longer takes provider env vars
- [ ] CRD manifest regenerated
- [ ] `go build ./...` succeeds

---

### Phase O3: Update Tests

**Goal:** Fix all tests that reference removed types/fields.
**Depends on:** Phase O2

#### Tasks

- [ ] **O3.1 Update operator tests for removed fields**
  - **Files:** `controllers/mattermost/agent/agent_test.go`, `pkg/mattermost/agent_test.go`, `pkg/mattermost/litellm_test.go`
  - **Action:** Modify
  - **Details:** Remove `LLMProviders` from test agent specs. Remove assertions about provider API key env vars in LiteLLM deployment tests. Update `GenerateLiteLLMDeployment` test calls to match new signature (no `providerEnvVars` param). Verify `OPENAI_API_KEY`/`ANTHROPIC_API_KEY` from virtual key Secret references remain (those come from `GenerateAgentDeployment`, not the removed provider env vars).

#### Definition of Done
- [ ] All operator tests pass (`go test ./...`)
- [ ] No references to removed types in test files

---

## File Change Map

| File | Phase(s) | Action | Summary |
|------|----------|--------|---------|
| `controllers/mattermost/agent/controller.go` | O1 | Modify | Remove LiteLLM business logic reconcile calls |
| `controllers/mattermost/agent/litellm.go` | O1, O2 | Modify | Delete reconcile functions, keep infra functions, update `checkLiteLLMDeployment` call |
| `controllers/mattermost/agent/litellm_client.go` | O1 | Delete | Entire file — ported to plugin |
| `controllers/mattermost/agent/litellm_client_test.go` | O1 | Delete | Tests for deleted client |
| `controllers/mattermost/agent/litellm_test.go` | O1 | Delete | Tests for deleted reconcile functions |
| `apis/mattermost/v1beta1/agent_types.go` | O2 | Modify | Remove `LLMProvider`, `AgentMCPServer`, `llmProviders`, `mcpServers` |
| `pkg/mattermost/litellm.go` | O2 | Modify | Remove `providerEnvVars` param from `GenerateLiteLLMDeployment` |
| `config/crd/bases/installation.mattermost.com_agents.yaml` | O2 | Regenerate | `make generate manifests` |
| `controllers/mattermost/agent/agent_test.go` | O3 | Modify | Remove references to `LLMProviders` |
| `pkg/mattermost/agent_test.go` | O3 | Modify | Update test assertions |
| `pkg/mattermost/litellm_test.go` | O3 | Modify | Update `GenerateLiteLLMDeployment` test calls |

## Testing Strategy

Run `go test ./...` from the operator repo root. Tests use controller-runtime's `envtest` for CRD validation and standard Go test patterns for unit tests. After CRD regeneration, apply the new CRD to the k3d dev cluster and verify agents can still be created.

## Definition of Done (Overall)

- [ ] Operator no longer makes any LiteLLM management API calls
- [ ] Agent CRD has no `llmProviders` or `mcpServers` fields
- [ ] LiteLLM infrastructure management (Deployment/Service/ConfigMap) still works
- [ ] Agent pod deployment and env var injection still works
- [ ] All tests pass
