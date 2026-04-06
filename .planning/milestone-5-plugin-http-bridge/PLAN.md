# Implementation Plan: M5 — Operator HTTP Support

> Update agent Service port from gRPC (50051) to HTTP (8080), register agent endpoints in LiteLLM as models, wire virtual key creation into reconcile loop, and generate/inject hook secret.

## Metadata
- **Spec:** `~/workspace/planning/projects/mattermost-cloud-agents/ideas/002-the-trail/spec.md`
- **Target repo:** https://github.com/mattermost/mattermost-operator (worktree: `~/workspace/worktrees/mattermost-operator-the-trail`)
- **Generated:** 2026-04-05
- **Status:** draft

## Architecture Overview

The operator already manages Agent CRDs with K8s Service, Deployment, NetworkPolicy, and LiteLLM integration. M5 changes are incremental:
1. Service port 50051 (gRPC) → 8080 (HTTP)
2. Register agent pod endpoint in LiteLLM as a model (using existing `registerModel` client method)
3. Wire `generateKey` into reconcile loop (method exists, not called from controller)
4. Generate and inject a shared hook secret for plugin-to-agent auth

Key files: `controllers/mattermost/agent/controller.go` (reconcile loop), `controllers/mattermost/agent/litellm.go` (LiteLLM reconciliation), `controllers/mattermost/agent/litellm_client.go` (HTTP client), `pkg/mattermost/agent.go` (resource generators), `apis/mattermost/v1beta1/agent_types.go` (CRD types).

## Phases

### Phase 1: HTTP Port + LiteLLM Agent Registration

**Goal:** Update agent Service to HTTP port 8080. Register agent pod endpoint as a model in LiteLLM so the plugin can route chat requests through LiteLLM to the agent.

**Depends on:** none

#### Tasks

- [ ] **1.1 Update agent Service port**
  - **Files:** `pkg/mattermost/agent.go`, `apis/mattermost/v1beta1/agent_types.go`
  - **Action:** Modify
  - **Details:** In `GenerateAgentService()` (agent.go:40-60), change:
    - Port: 50051 → 8080
    - Port name: `"grpc"` → `"http"`
    In `agent_types.go`, update `AgentGRPCPort` constant (or rename to `AgentHTTPPort`) from 50051 to 8080. Update `status.Endpoint` format in `checkAgentHealth()` (`controllers/mattermost/agent/agent.go:93`) to use the new port.

- [ ] **1.2 Register agent endpoint in LiteLLM as a model**
  - **Files:** `controllers/mattermost/agent/litellm.go`
  - **Action:** Modify
  - **Details:** Add new function `reconcileAgentModel(agent, litellmClient)` that registers the agent pod's K8s Service endpoint as a model in LiteLLM. Uses existing `registerModel()` client method with:
    - `modelName`: agent name (e.g., `"langgraph-demo"`)
    - `litellmModel`: `"openai/" + agent.Name` (the `openai/` prefix tells LiteLLM to use OpenAI-compatible routing)
    - `apiBase`: `fmt.Sprintf("http://%s.%s.svc.cluster.local:%d/v1", agent.Name, agent.Namespace, AgentHTTPPort)` — this is the K8s Service DNS, so LiteLLM load-balances natively across replicas
    Call this from the reconcile loop in `controller.go` after `checkAgentHealth` succeeds (agent is ready). The existing `registerModel` does `POST /model/new` with idempotency (lists first, skips existing).
    Note: the existing `registerModel` signature is `registerModel(modelName, litellmModel, apiKeyEnvRef string)`. The `apiKeyEnvRef` is for LLM API keys (e.g., `"os.environ/ANTHROPIC_API_KEY"`). For agent endpoints, pass `"fake-key"` (LiteLLM requires non-empty but agent pods don't validate it via this path — they use the virtual key). Alternatively, extend `registerModel` to accept an `apiBase` parameter for custom endpoints.

- [ ] **1.3 Extend `registerModel` for custom `api_base`**
  - **Files:** `controllers/mattermost/agent/litellm_client.go`
  - **Action:** Modify
  - **Details:** The current `registerModel` sends `litellm_params: {model, api_key}`. For agent pod routing, it also needs `api_base`. Add a new method or extend the existing one:
    ```go
    func (c *liteLLMClient) registerAgentModel(modelName, apiBase string) error
    // POST /model/new with body:
    // { "model_name": modelName, "litellm_params": { "model": "openai/" + modelName, "api_base": apiBase, "api_key": "agent-internal" } }
    ```
    Keep existing `registerModel` for LLM providers. Add tests using existing `httptest.Server` pattern from `litellm_client_test.go`.

- [ ] **1.4 Update tests**
  - **Files:** `controllers/mattermost/agent/agent_test.go`, `controllers/mattermost/agent/litellm_client_test.go`, `controllers/mattermost/agent/controller_test.go`
  - **Action:** Modify
  - **Details:** Update `checkAgentService` tests for port 8080 and name "http". Add test for `registerAgentModel`. Update full reconcile test (`TestFullReconcile`) to verify agent model registration step.

#### Definition of Done
- [ ] Agent Service created with port 8080, name "http"
- [ ] `status.Endpoint` reflects port 8080
- [ ] Agent pod endpoint registered in LiteLLM as `openai/<agent-name>` model
- [ ] LiteLLM can route `POST /v1/chat/completions` to agent pod
- [ ] All tests pass

---

### Phase 2: Virtual Key + Hook Secret

**Goal:** Complete the virtual key creation flow (existing stub) and add hook secret generation/injection.

**Depends on:** Phase 1

#### Tasks

- [ ] **2.1 Wire virtual key creation into reconcile loop**
  - **Files:** `controllers/mattermost/agent/litellm.go`, `controllers/mattermost/agent/controller.go`
  - **Action:** Modify
  - **Details:** The `generateKey` method exists on `liteLLMClient` (litellm_client.go:234) and is tested, but never called from the controller. Add `reconcileLiteLLMVirtualKey(agent, litellmClient)`:
    1. Check if key Secret already exists (`agent.LiteLLMKeySecretName()` = `"agent-<name>-litellm-key"`)
    2. If not, call `generateKey` with `key_alias: agent.Name`, `models: [agent.Name]` (grants access to the agent's own model in LiteLLM)
    3. Create the Secret via `GenerateAgentLiteLLMKeySecret(agent, keyValue)` (exists in `pkg/mattermost/litellm.go:198`)
    4. Store via `r.Resources.CreateSecretIfNotExists`
    Wire into controller reconcile loop after `reconcileAgentModel` (Phase 1) and after `reconcileLiteLLMModels`.
    The Secret is already referenced by the Deployment generator (`agent.go:99`) for `OPENAI_API_KEY`/`ANTHROPIC_API_KEY` env vars — no additional wiring needed for the pod to receive the key.

- [ ] **2.2 Add hook secret generation**
  - **Files:** `apis/mattermost/v1beta1/agent_utils.go`, `pkg/mattermost/agent.go`, `controllers/mattermost/agent/controller.go`
  - **Action:** Modify
  - **Details:**
    - `agent_utils.go`: Add `HookSecretName()` method: `return "agent-" + a.Name + "-hook-secret"`. Follow pattern of `BotTokenSecretName()` (line 87) and `LiteLLMKeySecretName()` (line 92).
    - `pkg/mattermost/agent.go`: Add `GenerateAgentHookSecret(agent, secretValue)` following pattern of `GenerateAgentBotTokenSecret`. Secret data key: `"hookSecret"`. OwnerReference: Agent CR.
    - `controller.go`: Add `checkHookSecret` step in reconcile loop (after `checkAgentServiceAccount`, before `checkAgentDeployment`). Generate a random 32-byte hex string if Secret doesn't exist. Create via `r.Resources.CreateSecretIfNotExists`.
    - Update agent Deployment generator in `pkg/mattermost/agent.go` to mount the hook secret as env var `HOOK_SECRET` (add to the container's `EnvFrom` or `Env` list).

- [ ] **2.3 Update tests**
  - **Files:** `controllers/mattermost/agent/litellm_test.go`, `controllers/mattermost/agent/agent_test.go`, `controllers/mattermost/agent/controller_test.go`, `pkg/mattermost/litellm_test.go`
  - **Action:** Modify
  - **Details:** Add test for `reconcileLiteLLMVirtualKey` (happy path, Secret already exists). Add test for `checkHookSecret` (creation, idempotency). Update `TestFullReconcile` to include both new steps. Add test for `GenerateAgentHookSecret` in `pkg/mattermost/litellm_test.go`.

#### Definition of Done
- [ ] Virtual key created and stored in K8s Secret during reconciliation
- [ ] Agent pod receives virtual key via env vars (existing plumbing)
- [ ] Hook secret generated and stored in K8s Secret
- [ ] Hook secret injected into agent pod as `HOOK_SECRET` env var
- [ ] All tests pass (`make unittest`)

## File Change Map

| File | Phase | Action | Summary |
|------|-------|--------|---------|
| `pkg/mattermost/agent.go` | 1, 2 | Modify | Service port 8080, hook secret generator |
| `apis/mattermost/v1beta1/agent_types.go` | 1 | Modify | Port constant |
| `apis/mattermost/v1beta1/agent_utils.go` | 2 | Modify | HookSecretName() helper |
| `controllers/mattermost/agent/agent.go` | 1 | Modify | status.Endpoint port |
| `controllers/mattermost/agent/controller.go` | 1, 2 | Modify | Add reconcile steps |
| `controllers/mattermost/agent/litellm.go` | 1, 2 | Modify | reconcileAgentModel, reconcileLiteLLMVirtualKey |
| `controllers/mattermost/agent/litellm_client.go` | 1 | Modify | registerAgentModel method |
| `controllers/mattermost/agent/agent_test.go` | 1, 2 | Modify | Service port + hook secret tests |
| `controllers/mattermost/agent/litellm_client_test.go` | 1 | Modify | registerAgentModel test |
| `controllers/mattermost/agent/litellm_test.go` | 2 | Modify | Virtual key reconciliation test |
| `controllers/mattermost/agent/controller_test.go` | 1, 2 | Modify | Full reconcile flow test |
| `pkg/mattermost/litellm_test.go` | 2 | Modify | Hook secret generator test |

## Testing Strategy

- **Unit:** `go test ./controllers/mattermost/agent/... -v` — all controller and client tests
- **Unit:** `go test ./pkg/mattermost/... -v` — resource generators
- **Full:** `make unittest` — all tests
- **Integration:** Deploy to k3d, verify agent Service port 8080, LiteLLM model registration, hook secret in pod env

## Definition of Done (Overall)

- [ ] Agent K8s Service uses port 8080
- [ ] Agent endpoint registered in LiteLLM as model
- [ ] Virtual key created and injected into agent pod
- [ ] Hook secret generated and injected into agent pod
- [ ] All tests pass (`make unittest`)
