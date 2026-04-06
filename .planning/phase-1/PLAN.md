# Phase 1: HTTP Port + LiteLLM Agent Registration — Prescriptive Plan

> **Milestone:** M5 — Operator HTTP Support
> **Target repo:** `~/workspace/worktrees/mattermost-operator-the-trail`
> **Phase:** 1 (HTTP Port + LiteLLM Agent Registration)
> **Depends on:** nothing (first phase)
> **Goal:** Update agent Service port from gRPC 50051 to HTTP 8080, rename all related constants/variables/comments. Register agent pod endpoint in LiteLLM as an OpenAI-compatible model so the plugin can route chat requests through LiteLLM to the agent. All tests pass.

---

## Context: What Already Exists

All tasks in this phase **modify** existing files. Do not recreate files from scratch.

- `apis/mattermost/v1beta1/agent_types.go` — `AgentSpec`, `AgentStatus` with `Endpoint` field (line 76, comment says "gRPC service endpoint" and "50051"), kubebuilder printcolumn (line 118, description says "gRPC Endpoint").
- `apis/mattermost/v1beta1/agent_utils.go` — constants block (lines 11-24) with `AgentGRPCPort = int32(50051)`.
- `pkg/mattermost/agent.go` — `GenerateAgentService()` (lines 40-60) creates Service with port name `"grpc"` and `mmv1beta.AgentGRPCPort`. `GenerateAgentDeployment()` (lines 68-218) sets container port to `mmv1beta.AgentGRPCPort` with name `"grpc"` (line 178-182). `GenerateAgentNetworkPolicy()` (lines 221-350) uses `grpcPort` variable from `mmv1beta.AgentGRPCPort` (line 224) for ingress rule.
- `controllers/mattermost/agent/agent.go` — `checkAgentHealth()` (lines 88-110) formats `status.Endpoint` using `mmv1beta.AgentGRPCPort` (line 93).
- `controllers/mattermost/agent/controller.go` — `Reconcile()` loop (lines 56-191). LiteLLM block at lines 103-138. Health check at line 174. Status update at line 184.
- `controllers/mattermost/agent/litellm.go` — `reconcileLiteLLMModels()` (lines 137-185) uses `registerModel()`. `reconcileLiteLLMMCPServers()` (lines 215-285).
- `controllers/mattermost/agent/litellm_client.go` — `liteLLMClient` with `registerModel()` (line 149), `listModels()` (line 131), `generateKey()` (line 234). Request types: `liteLLMModelParams` (lines 31-34) has `Model` and `APIKey` fields only (no `api_base`). `liteLLMModelRequest` (lines 36-39).
- `controllers/mattermost/agent/litellm_client_test.go` — Tests for `registerModel`, `listModels`, `generateKey`, etc. Uses `httptest.NewServer` pattern.
- `controllers/mattermost/agent/agent_test.go` — `TestCheckAgentService` (line 74) asserts `svc.Spec.Ports[0].Name` == `"grpc"` (line 90).
- `controllers/mattermost/agent/controller_test.go` — `TestReconcileAgent_FullReconcile` (line 78), `TestReconcileAgent_WithLLMGateway` (line 243).

Module path: `github.com/mattermost/mattermost-operator`

---

## Task 1.1: Rename Port Constant and Update Value

**File:** `apis/mattermost/v1beta1/agent_utils.go`
**Line:** 15

### Change

```go
// OLD (line 15):
AgentGRPCPort                     = int32(50051)

// NEW:
AgentHTTPPort                     = int32(8080)
```

This is a rename + value change. Every file that references `AgentGRPCPort` must be updated to `AgentHTTPPort`. The full list of references (excluding `.planning/` files):

| File | Line | Old | New |
|------|------|-----|-----|
| `apis/mattermost/v1beta1/agent_utils.go` | 15 | `AgentGRPCPort = int32(50051)` | `AgentHTTPPort = int32(8080)` |
| `pkg/mattermost/agent.go` | 54 | `mmv1beta.AgentGRPCPort` | `mmv1beta.AgentHTTPPort` |
| `pkg/mattermost/agent.go` | 55 | `mmv1beta.AgentGRPCPort` | `mmv1beta.AgentHTTPPort` |
| `pkg/mattermost/agent.go` | 180 | `mmv1beta.AgentGRPCPort` | `mmv1beta.AgentHTTPPort` |
| `pkg/mattermost/agent.go` | 224 | `mmv1beta.AgentGRPCPort` | `mmv1beta.AgentHTTPPort` |
| `controllers/mattermost/agent/agent.go` | 93 | `mmv1beta.AgentGRPCPort` | `mmv1beta.AgentHTTPPort` |

### Verification

After this change, `grep -r AgentGRPCPort` in the repo (excluding `.planning/`) must return zero results.

---

## Task 1.2: Update Service Port Name and Comments

**File:** `pkg/mattermost/agent.go`

### 1.2a: GenerateAgentService — function comment + port name

```go
// OLD (line 39):
// GenerateAgentService returns the gRPC Service for an Agent.

// NEW:
// GenerateAgentService returns the HTTP Service for an Agent.
```

```go
// OLD (lines 52-56):
Ports: []corev1.ServicePort{
    {
        Name:       "grpc",
        Port:       mmv1beta.AgentGRPCPort,
        TargetPort: intstr.FromInt32(mmv1beta.AgentGRPCPort),
    },
},

// NEW:
Ports: []corev1.ServicePort{
    {
        Name:       "http",
        Port:       mmv1beta.AgentHTTPPort,
        TargetPort: intstr.FromInt32(mmv1beta.AgentHTTPPort),
    },
},
```

### 1.2b: GenerateAgentDeployment — container port

```go
// OLD (lines 178-182):
Ports: []corev1.ContainerPort{
    {
        ContainerPort: mmv1beta.AgentGRPCPort,
        Name:          "grpc",
    },
},

// NEW:
Ports: []corev1.ContainerPort{
    {
        ContainerPort: mmv1beta.AgentHTTPPort,
        Name:          "http",
    },
},
```

### 1.2c: GenerateAgentNetworkPolicy — variable rename + value

```go
// OLD (line 224):
grpcPort := intstr.FromInt32(mmv1beta.AgentGRPCPort)

// NEW:
agentPort := intstr.FromInt32(mmv1beta.AgentHTTPPort)
```

Also update the reference on line 340 (ingress rule):

```go
// OLD (line 340):
Port:     &grpcPort,

// NEW:
Port:     &agentPort,
```

---

## Task 1.3: Update AgentStatus Endpoint Comment and Kubebuilder Tag

**File:** `apis/mattermost/v1beta1/agent_types.go`

### 1.3a: Endpoint field comment

```go
// OLD (lines 74-76):
// Endpoint is the gRPC service endpoint for this agent.
// Format: "agent-<name>.<namespace>.svc.cluster.local:50051"

// NEW:
// Endpoint is the HTTP service endpoint for this agent.
// Format: "<name>.<namespace>.svc.cluster.local:8080"
```

### 1.3b: Kubebuilder printcolumn description

```go
// OLD (line 118):
// +kubebuilder:printcolumn:priority=0,name="Endpoint",type=string,JSONPath=".status.endpoint",description="gRPC Endpoint"

// NEW:
// +kubebuilder:printcolumn:priority=0,name="Endpoint",type=string,JSONPath=".status.endpoint",description="HTTP Endpoint"
```

> **Note:** After changing kubebuilder tags, run `make generate manifests` to regenerate CRD manifests. This is a post-implementation step, not a code change.

---

## Task 1.4: Update checkAgentHealth Endpoint Format

**File:** `controllers/mattermost/agent/agent.go`
**Line:** 93

```go
// OLD (line 93):
Endpoint:           fmt.Sprintf("%s.%s.svc.cluster.local:%d", agent.Name, agent.Namespace, mmv1beta.AgentGRPCPort),

// NEW:
Endpoint:           fmt.Sprintf("%s.%s.svc.cluster.local:%d", agent.Name, agent.Namespace, mmv1beta.AgentHTTPPort),
```

---

## Task 1.5: Add `registerAgentModel` Method to liteLLMClient

**File:** `controllers/mattermost/agent/litellm_client.go`

### 1.5a: Add new request type

Insert after the existing `liteLLMModelRequest` struct (after line 39):

```go
// liteLLMAgentModelParams contains the LiteLLM params for registering an agent
// pod as an OpenAI-compatible model endpoint.
type liteLLMAgentModelParams struct {
	Model  string `json:"model"`
	APIBase string `json:"api_base"`
	APIKey string `json:"api_key"`
}

// liteLLMAgentModelRequest is the POST /model/new body for agent model registration.
type liteLLMAgentModelRequest struct {
	ModelName     string                  `json:"model_name"`
	LiteLLMParams liteLLMAgentModelParams `json:"litellm_params"`
}
```

### 1.5b: Add `registerAgentModel` method

Insert after the existing `registerModel` method (after line 166):

```go
// registerAgentModel registers an agent pod endpoint as an OpenAI-compatible model
// in LiteLLM via POST /model/new. NOT idempotent — callers must check listModels first.
//
// LiteLLM uses the "openai/" prefix to route via the OpenAI-compatible protocol.
// api_base points to the agent's K8s Service, and api_key is a placeholder
// (agent pods authenticate via virtual keys, not this value).
func (c *liteLLMClient) registerAgentModel(modelName, apiBase string) error {
	req := liteLLMAgentModelRequest{
		ModelName: modelName,
		LiteLLMParams: liteLLMAgentModelParams{
			Model:  "openai/" + modelName,
			APIBase: apiBase,
			APIKey: "agent-internal",
		},
	}

	body, status, err := c.do("POST", "/model/new", req)
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return fmt.Errorf("register agent model returned status %d: %s", status, string(body))
	}
	return nil
}
```

### Key design decisions

- **Separate method, not extending `registerModel`:** The existing `registerModel` uses `apiKeyEnvRef` (e.g., `os.environ/ANTHROPIC_API_KEY`) for LLM providers. Agent model registration uses `api_base` (a URL) and a placeholder `api_key`. Keeping them separate avoids muddying the existing method and its callers.
- **`api_key: "agent-internal"`:** LiteLLM requires a non-empty `api_key` field. Agent pods don't validate this value — authentication is via virtual keys issued to the plugin. The literal string `"agent-internal"` signals this is not a real credential.
- **`model: "openai/" + modelName`:** The `openai/` prefix tells LiteLLM to use OpenAI-compatible routing (POST `/v1/chat/completions` format), which is what agent pods implement.

---

## Task 1.6: Add `reconcileAgentModel` to litellm.go

**File:** `controllers/mattermost/agent/litellm.go`

Insert after the `reconcileLiteLLMModels` function (after line 185, before the `buildProviderEnvVars` function):

```go
// reconcileAgentModel registers the agent pod's K8s Service endpoint as a model
// in LiteLLM. This allows the plugin to route chat/completion requests to the
// agent through LiteLLM using the agent's name as the model identifier.
//
// Idempotency: POST /model/new is NOT idempotent — this function lists existing
// models first and only creates if missing.
func (r *AgentReconciler) reconcileAgentModel(ctx context.Context, agent *mmv1beta.Agent, litellmURL, masterKey string, reqLogger logr.Logger) error {
	c := newLiteLLMClient(litellmURL, masterKey)

	existing, err := c.listModels()
	if err != nil {
		return pkgerrors.Wrap(err, "failed to list existing models for agent model registration")
	}
	existingByName := make(map[string]struct{}, len(existing))
	for _, m := range existing {
		existingByName[m.ModelName] = struct{}{}
	}

	if _, found := existingByName[agent.Name]; found {
		reqLogger.Info("Agent model already registered, skipping", "model", agent.Name)
		return nil
	}

	apiBase := fmt.Sprintf("http://%s.%s.svc.cluster.local:%d/v1", agent.Name, agent.Namespace, mmv1beta.AgentHTTPPort)

	reqLogger.Info("Registering agent model in LiteLLM", "model", agent.Name, "apiBase", apiBase)
	if err := c.registerAgentModel(agent.Name, apiBase); err != nil {
		return pkgerrors.Wrapf(err, "failed to register agent model %s", agent.Name)
	}

	return nil
}
```

### Required import addition

The function uses `fmt` which is already imported in `litellm.go`. It also uses `mmv1beta.AgentHTTPPort` — check that `mmv1beta` is already imported (it is, line 8). No new imports needed.

---

## Task 1.7: Wire `reconcileAgentModel` into the Reconcile Loop

**File:** `controllers/mattermost/agent/controller.go`

### Placement

Insert **after** the health check succeeds (after line 182, the closing brace of the health check error block) and **before** the final `r.updateStatus` call (line 184).

### Exact change

```go
// OLD (lines 174-190):
	// Health check
	status, err = r.checkAgentHealth(ctx, agent, status, reqLogger)
	if err != nil {
		statusErr := r.updateStatus(ctx, agent, status, reqLogger)
		if statusErr != nil {
			reqLogger.Error(statusErr, "Error updating status")
		}
		reqLogger.Info("Agent not healthy", "msg", err.Error())
		return reconcile.Result{RequeueAfter: healthCheckRequeueDelay}, nil
	}

	err = r.updateStatus(ctx, agent, status, reqLogger)

// NEW:
	// Health check
	status, err = r.checkAgentHealth(ctx, agent, status, reqLogger)
	if err != nil {
		statusErr := r.updateStatus(ctx, agent, status, reqLogger)
		if statusErr != nil {
			reqLogger.Error(statusErr, "Error updating status")
		}
		reqLogger.Info("Agent not healthy", "msg", err.Error())
		return reconcile.Result{RequeueAfter: healthCheckRequeueDelay}, nil
	}

	// Register agent pod endpoint as a model in LiteLLM (after health check confirms agent is running).
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
	}

	err = r.updateStatus(ctx, agent, status, reqLogger)
```

### Why after health check, not in the earlier LiteLLM block?

The earlier LiteLLM block (lines 103-138) runs *before* the agent Deployment exists — it sets up LiteLLM infrastructure (ConfigMap, Deployment, Service) and registers LLM provider models. Agent model registration must happen *after* the agent pod is healthy, because:
1. The model's `api_base` points to the agent's K8s Service, which needs to be serving traffic.
2. Registering a model that can't respond would cause LiteLLM routing errors.

### Import check

`controller.go` already imports `mattermostApp "github.com/mattermost/mattermost-operator/pkg/mattermost"` (line 9). No new imports needed.

---

## Task 1.8: Update Tests

### 1.8a: Update `TestCheckAgentService` port assertion

**File:** `controllers/mattermost/agent/agent_test.go`
**Line:** 90

```go
// OLD:
assert.Equal(t, "grpc", svc.Spec.Ports[0].Name)

// NEW:
assert.Equal(t, "http", svc.Spec.Ports[0].Name)
```

### 1.8b: Add `TestRegisterAgentModel_Success`

**File:** `controllers/mattermost/agent/litellm_client_test.go`

Insert after `TestRegisterModel_Error` (after line 54):

```go
// ─── registerAgentModel ──────────────────────────────────────────────────

func TestRegisterAgentModel_Success(t *testing.T) {
	var capturedBody liteLLMAgentModelRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "POST", r.Method)
		require.Equal(t, "/model/new", r.URL.Path)
		require.Equal(t, "Bearer test-master-key", r.Header.Get("Authorization"))

		err := json.NewDecoder(r.Body).Decode(&capturedBody)
		require.NoError(t, err)

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"model_id": "m1"})
	}))
	defer srv.Close()

	c := newLiteLLMClient(srv.URL, "test-master-key")
	err := c.registerAgentModel("my-agent", "http://my-agent.default.svc.cluster.local:8080/v1")
	require.NoError(t, err)

	assert.Equal(t, "my-agent", capturedBody.ModelName)
	assert.Equal(t, "openai/my-agent", capturedBody.LiteLLMParams.Model)
	assert.Equal(t, "http://my-agent.default.svc.cluster.local:8080/v1", capturedBody.LiteLLMParams.APIBase)
	assert.Equal(t, "agent-internal", capturedBody.LiteLLMParams.APIKey)
}

func TestRegisterAgentModel_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	c := newLiteLLMClient(srv.URL, "key")
	err := c.registerAgentModel("my-agent", "http://my-agent.default.svc.cluster.local:8080/v1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}
```

### 1.8c: Update `TestReconcileAgent_FullReconcile` endpoint assertion

**File:** `controllers/mattermost/agent/controller_test.go`
**Line:** 166

The existing assertion `assert.Contains(t, updatedAgent.Status.Endpoint, agent.Name)` already passes — it just checks that the agent name is in the endpoint string. The port change (50051 → 8080) doesn't break this assertion.

**Add** a more specific assertion after line 166 to verify the port:

```go
// OLD (line 166):
assert.Contains(t, updatedAgent.Status.Endpoint, agent.Name)

// NEW:
assert.Contains(t, updatedAgent.Status.Endpoint, agent.Name)
assert.Contains(t, updatedAgent.Status.Endpoint, ":8080")
```

### 1.8d: Update `TestReconcileAgent_WithLLMGateway` for agent model registration

**File:** `controllers/mattermost/agent/controller_test.go`

The `TestReconcileAgent_WithLLMGateway` test (line 243) runs the full reconcile with operator-managed LLM gateway but **no LLMProviders** to avoid HTTP calls to an in-cluster LiteLLM URL. The new `reconcileAgentModel` will fire after health check succeeds and will attempt to call `listModels()` against the in-cluster LiteLLM URL, which doesn't resolve in tests.

**However**, in the current test, the first reconcile returns with `RequeueAfter: 6*time.Second` because the agent deployment has 0 ready replicas. The second reconcile (after making replicas ready) would trigger the agent model registration — but **this test doesn't do a second reconcile**. So the existing test won't break.

**No changes needed to `TestReconcileAgent_WithLLMGateway`** for Phase 1. The agent model registration path is tested via the `litellm_client_test.go` unit tests. A controller-level integration test for this path would require an `httptest.Server` to mock LiteLLM — defer to Phase 2 or add in a follow-up if desired.

**If the team wants a controller-level test**, extend `TestReconcileAgent_WithLLMGateway` with a second reconcile that simulates ready replicas. This will require either:
- Injecting a `liteLLMClient` interface (larger refactor, not Phase 1 scope), or
- Accepting that the `listModels` call will fail against the fake in-cluster URL and verifying the error is handled gracefully.

---

## Complete Reconcile Loop After Phase 1

For reference, the reconcile loop after all Phase 1 changes:

```
1.  Fetch Agent CR
2.  Set initial state to Reconciling (if new)
3.  Apply defaults
4.  Check Mattermost CR readiness
5.  LiteLLM gateway (if operator-managed):
    a. checkLiteLLMDeployment (ConfigMap + Deployment)
    b. checkLiteLLMService
    c. checkLiteLLMReady
    d. getLiteLLMMasterKey
    e. reconcileLiteLLMModels (LLM provider models)
    f. reconcileLiteLLMMCPServers
6.  Transition to Deploying phase
7.  checkAgentServiceAccount
8.  checkAgentService                    ← port 8080, name "http"
9.  checkAgentDeployment                 ← container port 8080
10. checkAgentNetworkPolicy              ← ingress port 8080
11. checkAgentHealth                     ← endpoint uses port 8080
12. reconcileAgentModel (if operator-managed)  ← NEW: register agent as LiteLLM model
13. updateStatus
```

---

## LiteLLM API Reference

For the implementing agent's context, here is the exact API contract for agent model registration:

### POST /model/new

**Request:**
```json
{
  "model_name": "my-agent",
  "litellm_params": {
    "model": "openai/my-agent",
    "api_base": "http://my-agent.default.svc.cluster.local:8080/v1",
    "api_key": "agent-internal"
  }
}
```

**Response (200):**
```json
{
  "model_id": "abc123",
  "model_name": "my-agent"
}
```

**Semantics:**
- `model_name` is the name the plugin uses when calling `POST /v1/chat/completions` with `"model": "my-agent"`.
- `litellm_params.model` with `openai/` prefix tells LiteLLM to use OpenAI-compatible routing.
- `litellm_params.api_base` is the K8s Service DNS URL. LiteLLM forwards requests to `{api_base}/chat/completions`.
- `litellm_params.api_key` is required non-empty but not validated by the agent pod (auth is via virtual key on the LiteLLM side).

### GET /model/info (for idempotency check)

**Response (200):**
```json
{
  "data": [
    {"model_name": "anthropic/claude-3-5-sonnet-20241022"},
    {"model_name": "my-agent"}
  ]
}
```

---

## File Change Summary

| File | Task | Action | Summary |
|------|------|--------|---------|
| `apis/mattermost/v1beta1/agent_utils.go` | 1.1 | Modify | Rename `AgentGRPCPort` → `AgentHTTPPort`, value 50051 → 8080 |
| `apis/mattermost/v1beta1/agent_types.go` | 1.3 | Modify | Update Endpoint comment + kubebuilder printcolumn description |
| `pkg/mattermost/agent.go` | 1.1, 1.2 | Modify | All `AgentGRPCPort` → `AgentHTTPPort`, port name `"grpc"` → `"http"`, variable `grpcPort` → `agentPort`, comment update |
| `controllers/mattermost/agent/agent.go` | 1.4 | Modify | `AgentGRPCPort` → `AgentHTTPPort` in endpoint format |
| `controllers/mattermost/agent/litellm_client.go` | 1.5 | Modify | Add `liteLLMAgentModelParams`, `liteLLMAgentModelRequest`, `registerAgentModel()` |
| `controllers/mattermost/agent/litellm.go` | 1.6 | Modify | Add `reconcileAgentModel()` |
| `controllers/mattermost/agent/controller.go` | 1.7 | Modify | Wire `reconcileAgentModel` after health check |
| `controllers/mattermost/agent/agent_test.go` | 1.8a | Modify | Port name assertion `"grpc"` → `"http"` |
| `controllers/mattermost/agent/litellm_client_test.go` | 1.8b | Modify | Add `TestRegisterAgentModel_Success`, `TestRegisterAgentModel_Error` |
| `controllers/mattermost/agent/controller_test.go` | 1.8c | Modify | Add `:8080` endpoint assertion |

---

## Testing Strategy

```bash
# Unit tests — controller and client
go test ./controllers/mattermost/agent/... -v

# Unit tests — resource generators
go test ./pkg/mattermost/... -v

# Full suite
make unittest
```

### Post-code generation

After all code changes, run:
```bash
make generate manifests
```

This regenerates CRD YAML from the updated kubebuilder tags. Commit the generated files alongside the code changes.

---

## Definition of Done

- [ ] `AgentGRPCPort` renamed to `AgentHTTPPort` with value `8080` — zero references to old name remain
- [ ] Agent Service created with port 8080, name `"http"`
- [ ] Agent container port is 8080, name `"http"`
- [ ] NetworkPolicy ingress allows port 8080
- [ ] `status.Endpoint` reflects port 8080
- [ ] `registerAgentModel` method on `liteLLMClient` sends correct POST /model/new body
- [ ] `reconcileAgentModel` called from reconcile loop after health check (operator-managed only)
- [ ] Idempotency: agent model not re-registered if already present
- [ ] All tests pass (`make unittest`)
- [ ] CRD manifests regenerated (`make generate manifests`)

---

## Implementation Summary

**Completed:** 2026-04-05

All Phase 1 tasks implemented and verified:

### Changes Made

| File | Changes |
|------|---------|
| `apis/mattermost/v1beta1/agent_utils.go` | `AgentGRPCPort` → `AgentHTTPPort` (8080) |
| `apis/mattermost/v1beta1/agent_types.go` | Endpoint comment: gRPC → HTTP, kubebuilder printcolumn description updated |
| `pkg/mattermost/agent.go` | Service port name `"grpc"` → `"http"`, all `AgentGRPCPort` → `AgentHTTPPort`, `grpcPort` var → `agentPort`, function comment updated |
| `controllers/mattermost/agent/agent.go` | Endpoint format uses `AgentHTTPPort` |
| `controllers/mattermost/agent/litellm_client.go` | Added `liteLLMAgentModelParams`, `liteLLMAgentModelRequest` types and `registerAgentModel()` method |
| `controllers/mattermost/agent/litellm.go` | Added `reconcileAgentModel()` with idempotency (checks `listModels` first), added `"fmt"` import |
| `controllers/mattermost/agent/controller.go` | Wired `reconcileAgentModel` after health check in reconcile loop (operator-managed only) |
| `controllers/mattermost/agent/agent_test.go` | Port name assertion `"grpc"` → `"http"` |
| `controllers/mattermost/agent/litellm_client_test.go` | Added `TestRegisterAgentModel_Success` and `TestRegisterAgentModel_Error` |
| `controllers/mattermost/agent/controller_test.go` | Added `:8080` endpoint assertion |
| `pkg/mattermost/agent_test.go` | Updated port assertions: 50051 → 8080, `"grpc"` → `"http"` |

### Test Results

```
go test ./controllers/mattermost/agent/... -v -count=1  → PASS (all 32 tests)
go test ./pkg/mattermost/... -v -count=1                → PASS (all tests)
```

### Remaining

- `make generate manifests` — should be run to regenerate CRD YAML from updated kubebuilder tags (requires controller-gen tooling)
