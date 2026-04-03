# Implementation Plan: M3 — Agent Secret Protection (Secretless Pod Architecture)

> Deploy a central LiteLLM gateway managed by the mattermost-operator. Agent pods receive virtual keys instead of raw API keys. MCP server credentials are injected by the gateway, never exposed to agents.

## Metadata
- **Spec:** `~/workspace/planning/projects/mattermost-cloud-agents/ideas/002-the-trail/spec.md`
- **Target repos:** mattermost-operator (this repo), the-trail (`~/workspace/the-trail`)
- **Generated:** 2026-03-23
- **Status:** draft
- **Spikes:** `~/workspace/planning/projects/mattermost-cloud-agents/ideas/002-the-trail/spikes/`

## Architecture Overview

The operator extends the Agent CRD with `llmGateway` and `mcpServers` fields. When `operatorManaged` is set, the operator deploys a shared LiteLLM instance (Deployment + Service + ConfigMap) in the agent's namespace, registers LLM models and MCP servers via the LiteLLM management API, creates per-agent virtual keys, and injects the virtual key + base URL env vars into agent pods. NetworkPolicy is updated so deny-mode agents can only reach LiteLLM + MM server + DNS.

**Key patterns from existing code:**
- Bot provisioning in `agent.go` is the template for LiteLLM API calls (idempotency via K8s Secret check, raw `http.DefaultClient`, testability split with `WithURL` variant)
- Database `External`/`OperatorManaged` in `mattermost_types.go` is the template for the `LLMGatewayConfig` struct
- `mergeEnvVars` in `helpers.go` handles env var injection (LiteLLM vars go in `baseEnv`, user `spec.env` can override)
- Resource generators in `pkg/mattermost/agent.go` are the template for `GenerateLiteLLM*` functions
- All errors use `errors.Wrap(err, "message")` from `github.com/pkg/errors`

**Spike-validated API details (use these exact field names):**
- Image: `ghcr.io/berriai/litellm-database:main-v1.82.0-stable`
- Register model: `POST /model/new` — safe to call multiple times (upserts)
- Register MCP server: `POST /v1/mcp/server` — NOT idempotent, creates duplicates. Must list-then-create.
- List MCP servers: `GET /v1/mcp/server` (singular, not `/servers`)
- Create virtual key: `POST /key/generate` — rejects duplicate `key_alias` with 400
- Update virtual key: `POST /key/update` with token hash
- MCP credentials: nested `{"credentials": {"auth_value": "..."}}`
- MCP access: `{"object_permission": {"mcp_access_groups": [...]}}` on keys (NOT `access_group_ids`)
- Server names: only `[A-Za-z0-9_.]` (no hyphens — sanitize with underscores)
- Tool names namespaced: `<server_name>-<tool_name>`
- Config file for `general_settings` only — all models via API (DB overrides config after restart)

## Phases

### Phase 1: CRD Types + Resource Generation

**Goal:** All new Go types compiled, resource generators for LiteLLM K8s resources, env var injection into agent pods, NetworkPolicy update. No API calls yet — purely K8s resource generation.
**Depends on:** none

#### Tasks

- [ ] **1.1 Add CRD type definitions**
  - **Files:** `apis/mattermost/v1beta1/agent_types.go`
  - **Action:** Extend
  - **Details:** Add two fields to `AgentSpec` after `Env` (line 57): `LLMGateway *LLMGatewayConfig` and `MCPServers []AgentMCPServer`. Define 7 new types at end of file (before `func init()`): `LLMGatewayConfig` (External/OperatorManaged pointers), `ExternalLLMGateway` (URL, VirtualKeySecret), `OperatorManagedLLMGateway` (Image, Resources, LLMProviders, Database), `LLMProvider` (Name, Secret, Models), `LiteLLMDatabase` (External pointer using existing `ExternalDatabase` type from `mattermost_types.go`, UseMattermostDB *bool), `AgentMCPServer` (Name, URL, AuthType, CredentialSecret, SharedCredentialRef, MCPAccessGroup, AllowedTools, DisallowedTools). All fields use `json:"...,omitempty"` tags and `+optional` markers where appropriate. After editing, run `make generate manifests` to regenerate deepcopy and CRD YAML.

- [ ] **1.2 Add naming helpers and defaults**
  - **Files:** `apis/mattermost/v1beta1/agent_utils.go`
  - **Action:** Extend
  - **Details:** Add constants: `AgentLiteLLMDefaultImage = "ghcr.io/berriai/litellm-database:main-v1.82.0-stable"`, `AgentLiteLLMPort = int32(4000)`, `AgentLiteLLMDeploymentName = "litellm"`, `AgentLiteLLMServiceName = "litellm"`, `AgentLiteLLMConfigMapName = "litellm-config"`, `AgentLiteLLMMasterKeySecretName = "litellm-master-key"`, `AgentLiteLLMDBCredentialsSecret = "litellm-db-credentials"`. Add method `func (a *Agent) LiteLLMKeySecretName() string` returning `"agent-" + a.Name + "-litellm-key"` (following `BotTokenSecretName()` at line 64). Extend `SetDefaults()` (line 20) to set default image when `OperatorManaged` is non-nil and `Image` is empty.

- [ ] **1.3 Create LiteLLM K8s resource generators**
  - **Files:** `pkg/mattermost/litellm.go` (new)
  - **Action:** Create
  - **Details:** New file in package `mattermost`. Functions: `GenerateLiteLLMConfigMap(namespace string) *corev1.ConfigMap` (general_settings only, labels `app: litellm`), `GenerateLiteLLMDeployment(namespace, image string, providerEnvVars []corev1.EnvVar) *appsv1.Deployment` (1 replica, env: DATABASE_URL from secretKeyRef, LITELLM_MASTER_KEY from secretKeyRef, STORE_MODEL_IN_DB=True, plus provider env vars; volume: config from ConfigMap; probes: liveness `/health/liveliness`, readiness `/health/readiness` with initialDelaySeconds 15; resources: 500m/512Mi req, 2/2Gi lim), `GenerateLiteLLMService(namespace string) *corev1.Service` (port 4000, selector `app: litellm`), `GenerateAgentLiteLLMKeySecret(agent *Agent, keyValue string) *corev1.Secret` (following `GenerateAgentBotTokenSecret` pattern, key `"apiKey"`), `LiteLLMServiceURL(namespace string) string` (returns `http://litellm.<ns>.svc.cluster.local:4000`). Also add private helper `secretEnvSource(secretName, key string) *corev1.EnvVarSource`.

- [ ] **1.4 Modify agent pod env var injection**
  - **Files:** `pkg/mattermost/agent.go`
  - **Action:** Modify
  - **Details:** In `GenerateAgentDeployment` after `baseEnv` construction (around line 91, before `mergeEnvVars`), add conditional: when `agent.Spec.LLMGateway != nil`, append to `baseEnv`: `LITELLM_BASE_URL`, `LITELLM_MCP_URL` (+ `/mcp`), `OPENAI_BASE_URL` (+ `/v1`), `OPENAI_API_KEY` (secretKeyRef from `agent.LiteLLMKeySecretName()`, key `"apiKey"`), `ANTHROPIC_BASE_URL`, `ANTHROPIC_API_KEY` (same secretKeyRef). Use `secretEnvSource` helper from litellm.go. The `mergeEnvVars` call ensures user `spec.env` can override these.

- [ ] **1.5 Modify NetworkPolicy for LiteLLM egress**
  - **Files:** `pkg/mattermost/agent.go`
  - **Action:** Modify
  - **Details:** In `GenerateAgentNetworkPolicy` (line 192), when `agent.Spec.LLMGateway != nil` and egress policy is deny, add an egress rule: `podSelector: {matchLabels: {app: litellm}}`, port 4000 TCP. Insert after the MM server egress rule and before the DNS rule. Follow the exact pattern of the existing MM server rule at lines 199-210.

- [ ] **1.6 Add CreateConfigMapIfNotExists to ResourceHelper**
  - **Files:** `pkg/resources/create_resources.go`
  - **Action:** Extend
  - **Details:** Add `func (r *ResourceHelper) CreateConfigMapIfNotExists(owner v1.Object, cm *corev1.ConfigMap, reqLogger logr.Logger) error` following the pattern of `CreateSecretIfNotExists` (line 208). Note: for ownerless resources (LiteLLM ConfigMap shared across agents), the caller will use `r.client.Create(context.TODO(), cm)` directly instead of `r.Create(owner, cm, reqLogger)` which calls `controllerutil.SetControllerReference`.

#### Definition of Done
- [ ] `make generate manifests` succeeds with no errors
- [ ] `make build` compiles successfully
- [ ] New types appear in `config/crd/bases/installation.mattermost.com_agents.yaml`
- [ ] Existing tests still pass (`make unittest`)

---

### Phase 2: LiteLLM HTTP Client

**Goal:** Pure HTTP client for LiteLLM management API. Fully unit-testable with `httptest.NewServer`, no K8s dependencies.
**Depends on:** Phase 1 (types for function signatures)

#### Tasks

- [ ] **2.1 Implement LiteLLM API client**
  - **Files:** `controllers/mattermost/agent/litellm_client.go` (new)
  - **Action:** Create
  - **Details:** Package `agent`. Define `liteLLMClient` struct with `baseURL` and `masterKey` fields. Define request/response structs matching spike-validated API shapes: `liteLLMModel`, `liteLLMModelParams`, `liteLLMMCPServer` (with nested `liteLLMCredentials` for `credentials.auth_value`), `liteLLMMCPServerResponse`, `liteLLMKeyRequest` (with `liteLLMObjectPermission` for `object_permission.mcp_access_groups`), `liteLLMKeyResponse`, `liteLLMKeyUpdateRequest`. Implement functions: `newLiteLLMClient(baseURL, masterKey)`, `registerModel`, `listMCPServers` (GET `/v1/mcp/server`), `registerMCPServer` (POST), `updateMCPServer` (PUT `/{serverID}`), `deleteMCPServer` (DELETE `/{serverID}`), `generateKey` (POST `/key/generate`), `updateKey` (POST `/key/update`), `deleteKey` (DELETE `/key/delete`). All use `http.DefaultClient.Do(req)` with `Authorization: Bearer <masterKey>` header, following the exact pattern of `listBots`/`createBot` in `agent.go` (lines 95-173). Add exported `SanitizeMCPServerName(name string) string` that replaces hyphens with underscores.

- [ ] **2.2 Write client unit tests**
  - **Files:** `controllers/mattermost/agent/litellm_client_test.go` (new)
  - **Action:** Create
  - **Details:** Use `httptest.NewServer` pattern from `agent_test.go`. Tests: `TestRegisterModel_Success` (assert POST body), `TestRegisterModel_Error` (non-200 returns error), `TestListMCPServers_Empty`, `TestListMCPServers_WithEntries`, `TestRegisterMCPServer_Success` (assert nested `credentials.auth_value` in POST body), `TestGenerateKey_Success` (assert `object_permission.mcp_access_groups`), `TestGenerateKey_DuplicateAlias` (400 response handled as "already exists", not error), `TestUpdateKey_Success`, `TestDeleteKey_Success`, `TestSanitizeMCPServerName` (table-driven: `"jira-agent"` → `"jira_agent"`, clean names unchanged).

#### Definition of Done
- [ ] All 10 client tests pass
- [ ] `make unittest` passes (no regression)
- [ ] Client functions match spike-validated API field names exactly

---

### Phase 3: Reconciler Integration

**Goal:** Wire LiteLLM lifecycle into the Agent reconciler. Deploy LiteLLM, register models, create virtual keys.
**Depends on:** Phase 1, Phase 2

#### Tasks

- [ ] **3.1 Implement LiteLLM reconciler functions**
  - **Files:** `controllers/mattermost/agent/litellm.go` (new)
  - **Action:** Create
  - **Details:** Package `agent`. Functions: `checkLiteLLMDeployment(ctx, agent, reqLogger)` — generate ConfigMap + Deployment, create if not exists (use `r.client.Create` directly for ownerless resources), get current, update if changed. `checkLiteLLMService(ctx, agent, reqLogger)` — same pattern. `checkLiteLLMReady(ctx, agent, reqLogger) (bool, error)` — get LiteLLM Deployment, return `(false, nil)` if `ReadyReplicas < 1` for requeue. `getLiteLLMMasterKey(ctx, namespace) (string, error)` — read Secret `litellm-master-key`, return `masterKey` value. `reconcileLiteLLMModels(ctx, agent, litellmURL, masterKey, reqLogger)` — for each provider/model, call `client.registerModel()` with model name `provider.Name + "/" + model` and api_key `"os.environ/" + strings.ToUpper(provider.Name) + "_API_KEY"`. `reconcileLiteLLMVirtualKey(ctx, agent, litellmURL, masterKey, mcpAccessGroups, reqLogger)` — idempotency: check K8s Secret `agent.LiteLLMKeySecretName()` exists -> skip. Otherwise call `client.generateKey()` with `key_alias: "agent-" + agent.Name + "-key"` and `object_permission.mcp_access_groups`. On 400 duplicate, log and continue. Store key in K8s Secret via `GenerateAgentLiteLLMKeySecret`. Provider env vars for LiteLLM Deployment: iterate `LLMProviders`, build `[]corev1.EnvVar` with `Name: strings.ToUpper(p.Name) + "_API_KEY"`, `ValueFrom: secretKeyRef(p.Secret, "apiKey")`.

- [ ] **3.2 Wire reconciler steps into controller loop**
  - **Files:** `controllers/mattermost/agent/controller.go`
  - **Action:** Modify
  - **Details:** In `Reconcile()`, after `checkAgentBot` (line 116) and before `checkAgentServiceAccount` (line 118), insert conditional block: `if agent.Spec.LLMGateway != nil && agent.Spec.LLMGateway.OperatorManaged != nil { ... }`. Inside: call `checkLiteLLMDeployment`, `checkLiteLLMService`, `checkLiteLLMReady` (requeue with `mattermostNotReadyDelay` if not ready), `getLiteLLMMasterKey`, `reconcileLiteLLMModels`, compute `mcpAccessGroups` from `agent.Spec.MCPServers`, `reconcileLiteLLMVirtualKey`. Each step follows the existing error handling pattern. Also add `Owns(&corev1.ConfigMap{})` to `SetupWithManager` (line 50).

- [ ] **3.3 Write reconciler unit tests**
  - **Files:** `controllers/mattermost/agent/litellm_test.go` (new)
  - **Action:** Create
  - **Details:** Use `setupReconciler` + `newTestAgent` fixtures from `agent_test.go`. Use `httptest.NewServer` for LiteLLM API mocks. Tests: `TestCheckLiteLLMDeployment_CreatesResources`, `TestCheckLiteLLMDeployment_Idempotent`, `TestCheckLiteLLMService_Creates`, `TestCheckLiteLLMReady_NotReady`, `TestCheckLiteLLMReady_Ready`, `TestReconcileLiteLLMModels_CallsAPI`, `TestReconcileLiteLLMVirtualKey_CreatesSecret`, `TestReconcileLiteLLMVirtualKey_Idempotent`.

#### Definition of Done
- [ ] All reconciler tests pass
- [ ] Full reconcile loop works with LLMGateway set (LiteLLM resources created, virtual key Secret created)
- [ ] Full reconcile loop works WITHOUT LLMGateway set (backwards-compatible)
- [ ] `make unittest` passes

---

### Phase 4: MCP Server Registration

**Goal:** Operator registers MCP server entries in LiteLLM per agent, resolves credentials from K8s Secrets, handles shared vs per-agent entries.
**Depends on:** Phase 3

#### Tasks

- [ ] **4.1 Implement MCP server reconciler**
  - **Files:** `controllers/mattermost/agent/litellm.go` (append)
  - **Action:** Extend
  - **Details:** Add `reconcileLiteLLMMCPServers(ctx, agent, litellmURL, masterKey, reqLogger) ([]string, error)` — returns computed `mcpAccessGroups` list. Logic: if no MCPServers, return empty. Call `client.listMCPServers()` and build map by server_name. For each `AgentMCPServer`: sanitize name, compute access group (default `agent.Name + "_" + sanitizedName` for per-agent, `"shared_" + sanitizedName` for shared), resolve credential from K8s Secret, check if server exists in map — skip if yes, register if no. Collect access groups. Update controller.go to call this before `reconcileLiteLLMVirtualKey` and pass returned groups.

- [ ] **4.2 Write MCP reconciler tests**
  - **Files:** `controllers/mattermost/agent/litellm_test.go` (append)
  - **Action:** Extend
  - **Details:** Tests: `TestReconcileLiteLLMMCPServers_CreatesWhenMissing`, `TestReconcileLiteLLMMCPServers_SkipsExisting`, `TestReconcileLiteLLMMCPServers_CredentialResolution` (pre-create K8s Secret), `TestReconcileLiteLLMMCPServers_SharedAndPerAgent` (verify access group naming).

#### Definition of Done
- [ ] MCP server registration creates entries only when missing
- [ ] Credentials resolved correctly from K8s Secrets
- [ ] Shared and per-agent access groups computed correctly
- [ ] Virtual key receives all computed access groups
- [ ] `make unittest` passes

---

### Phase 5: Deploy Manifests + Dev Environment

**Goal:** Updated k3d dev environment with LiteLLM in the stack.
**Depends on:** Phase 1 (CRD must exist for valid YAML)

#### Tasks

- [ ] **5.1 Create LiteLLM dev secrets script**
  - **Files:** `~/workspace/the-trail/deploy/dev/litellm-secrets.sh` (new)
  - **Action:** Create
  - **Details:** Creates two K8s Secrets in `mattermost` namespace: `litellm-master-key` (key `masterKey`, auto-generated `sk-` + random hex) and `litellm-db-credentials` (key `connectionString`, PostgreSQL DSN). Uses `kubectl create secret ... --dry-run=client -o yaml | kubectl apply -f -` for idempotency. Make executable.

- [ ] **5.2 Create LiteLLM dev ConfigMap**
  - **Files:** `~/workspace/the-trail/deploy/dev/litellm-config.yaml` (new)
  - **Action:** Create
  - **Details:** K8s ConfigMap `litellm-config` with `general_settings:` only (no model_list).

- [ ] **5.3 Update Agent CR manifest**
  - **Files:** `~/workspace/the-trail/deploy/dev/agent.yaml`
  - **Action:** Modify
  - **Details:** Replace `spec.env` ANTHROPIC_API_KEY entry with `spec.llmGateway.operatorManaged` block. Set `llmProviders`, change `egressPolicy` to `deny`.

- [ ] **5.4 Update agent secrets script**
  - **Files:** `~/workspace/the-trail/deploy/dev/agent-secrets.sh`
  - **Action:** Modify
  - **Details:** Change anthropic-key secret to use key `apiKey` instead of `ANTHROPIC_API_KEY`.

#### Definition of Done
- [ ] `kubectl apply` succeeds for all new manifests
- [ ] Agent CR validates against the generated CRD schema

---

### Phase 6: Integration Tests + Validation

**Goal:** All tests green, no regressions, new behaviors validated.
**Depends on:** All previous phases

#### Tasks

- [ ] **6.1 Add LiteLLM-aware agent tests**
  - **Files:** `controllers/mattermost/agent/agent_test.go`
  - **Action:** Extend
  - **Details:** Add `TestCheckAgentDeployment_WithLLMGateway` (assert all 6 LiteLLM env vars present, secretKeyRef correct). Add `TestCheckAgentNetworkPolicy_DenyWithLiteLLM` (assert 3 egress rules). Verify existing tests pass unchanged.

- [ ] **6.2 Run full test suite and CRD generation**
  - **Files:** all modified files
  - **Action:** Validate
  - **Details:** Run `make generate manifests` then `make unittest` then `make build`. All must pass.

#### Definition of Done
- [ ] `make generate manifests` succeeds
- [ ] `make unittest` passes (0 failures)
- [ ] `make build` succeeds
- [ ] CRD YAML contains `llmGateway` and `mcpServers` fields

---

## File Change Map

| File | Phase(s) | Action | Summary |
|------|----------|--------|---------|
| `apis/mattermost/v1beta1/agent_types.go` | 1 | Extend | Add `LLMGateway`, `MCPServers` to AgentSpec + 7 new types |
| `apis/mattermost/v1beta1/agent_utils.go` | 1 | Extend | Constants, `LiteLLMKeySecretName()`, extend `SetDefaults()` |
| `apis/mattermost/v1beta1/zz_generated.deepcopy.go` | 1 | Regenerate | Auto-generated by `make generate` |
| `config/crd/bases/installation.mattermost.com_agents.yaml` | 1 | Regenerate | Auto-generated by `make manifests` |
| `pkg/mattermost/litellm.go` | 1 | Create | LiteLLM resource generators |
| `pkg/mattermost/agent.go` | 1 | Modify | Inject LiteLLM env vars + NetworkPolicy egress rule |
| `pkg/resources/create_resources.go` | 1 | Extend | `CreateConfigMapIfNotExists` |
| `controllers/mattermost/agent/litellm_client.go` | 2 | Create | HTTP client for LiteLLM management API |
| `controllers/mattermost/agent/litellm_client_test.go` | 2 | Create | Client unit tests (10 tests) |
| `controllers/mattermost/agent/litellm.go` | 3, 4 | Create | Reconciler functions |
| `controllers/mattermost/agent/litellm_test.go` | 3, 4 | Create | Reconciler tests (12 tests) |
| `controllers/mattermost/agent/controller.go` | 3 | Modify | Insert LiteLLM steps into Reconcile loop |
| `controllers/mattermost/agent/agent_test.go` | 6 | Extend | 2 new test variants |
| `~/workspace/the-trail/deploy/dev/litellm-secrets.sh` | 5 | Create | Dev secrets script |
| `~/workspace/the-trail/deploy/dev/litellm-config.yaml` | 5 | Create | LiteLLM ConfigMap |
| `~/workspace/the-trail/deploy/dev/agent.yaml` | 5 | Modify | Switch to spec.llmGateway |
| `~/workspace/the-trail/deploy/dev/agent-secrets.sh` | 5 | Modify | Change secret key name |

## Testing Strategy

**Unit tests (per phase):** Run `make unittest` after each phase.

**Test patterns:**
- K8s resources: `fake.NewClientBuilder()` with scheme — no real cluster needed
- LiteLLM API: `httptest.NewServer` mocking endpoints
- Reconciler: combine both

**Manual e2e (post-implementation):** Deploy to k3d, verify agent pod gets virtual key env vars, verify LLM calls work through LiteLLM, verify `ANTHROPIC_API_KEY` is NOT in agent pod env.

## Definition of Done (Overall)

- [ ] Agent CRD has `llmGateway` and `mcpServers` fields
- [ ] Operator deploys LiteLLM when `operatorManaged` is set
- [ ] Operator registers models via LiteLLM API
- [ ] Operator creates per-agent virtual keys stored in K8s Secrets
- [ ] Operator registers MCP server entries with per-agent credentials
- [ ] Agent pods receive OPENAI_BASE_URL, ANTHROPIC_BASE_URL, LITELLM_MCP_URL + virtual key
- [ ] Agent pods do NOT receive raw ANTHROPIC_API_KEY when llmGateway is set
- [ ] NetworkPolicy deny mode routes through LiteLLM
- [ ] Backwards compatible: agents without llmGateway work as before
- [ ] All unit tests pass (`make unittest`)
- [ ] CRD YAML is regenerated and correct (`make generate manifests`)
