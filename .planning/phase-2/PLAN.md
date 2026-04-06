# Phase 2: Virtual Key + Hook Secret

> Prescriptive implementation plan for wiring LiteLLM virtual key creation into the reconcile loop and adding hook secret generation/injection.

## Prerequisites

- Phase 1 complete: agent Service uses port 8080, agent endpoint registered in LiteLLM as model
- `generateKey` method exists on `liteLLMClient` (litellm_client.go:274) and is tested
- `GenerateAgentLiteLLMKeySecret` exists in `pkg/mattermost/litellm.go:198` and is tested
- `LiteLLMKeySecretName()` exists in `apis/mattermost/v1beta1/agent_utils.go:92`
- Deployment generator already references `LiteLLMKeySecretName()` for `OPENAI_API_KEY`/`ANTHROPIC_API_KEY` env vars (pkg/mattermost/agent.go:101)

## Task 2.1: Wire Virtual Key Creation into Reconcile Loop

### 2.1a: Add `reconcileLiteLLMVirtualKey` to `controllers/mattermost/agent/litellm.go`

Add this function after `reconcileAgentModel` (after line 219):

```go
// reconcileLiteLLMVirtualKey ensures a LiteLLM virtual key exists for the agent
// and is stored in a K8s Secret. The key grants access to the agent's own model
// and any MCP access groups returned by reconcileLiteLLMMCPServers.
//
// Idempotency: if the Secret already exists, this is a no-op. If the key_alias
// already exists in LiteLLM (but we lost the Secret), we cannot recover the key
// value — log a warning. A future reconcile after manual cleanup will recreate it.
func (r *AgentReconciler) reconcileLiteLLMVirtualKey(ctx context.Context, agent *mmv1beta.Agent, litellmURL, masterKey string, mcpAccessGroups []string, reqLogger logr.Logger) error {
	// Check if the Secret already exists — if so, nothing to do.
	secretName := agent.LiteLLMKeySecretName()
	existingSecret := &corev1.Secret{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: secretName, Namespace: agent.Namespace}, existingSecret)
	if err == nil {
		reqLogger.Info("LiteLLM virtual key secret already exists, skipping", "secret", secretName)
		return nil
	}
	if !k8sErrors.IsNotFound(err) {
		return pkgerrors.Wrap(err, "failed to check for existing litellm key secret")
	}

	// Secret does not exist — generate a virtual key via the LiteLLM API.
	c := newLiteLLMClient(litellmURL, masterKey)

	keyReq := liteLLMKeyRequest{
		KeyAlias: agent.Name,
		Models:   []string{agent.Name},
		Metadata: map[string]string{"agent_name": agent.Name},
	}
	if len(mcpAccessGroups) > 0 {
		keyReq.ObjectPermission = liteLLMObjectPermission{
			MCPAccessGroups: mcpAccessGroups,
		}
	}

	keyResp, err := c.generateKey(keyReq)
	if err != nil {
		if err == errKeyAliasExists {
			reqLogger.Info("LiteLLM key alias already exists but Secret is missing — manual cleanup required",
				"keyAlias", agent.Name, "expectedSecret", secretName)
			return nil
		}
		return pkgerrors.Wrap(err, "failed to generate litellm virtual key")
	}

	// Store the key in a K8s Secret owned by the Agent CR.
	desired := mattermostApp.GenerateAgentLiteLLMKeySecret(agent, keyResp.Key)
	if err := r.Resources.CreateSecretIfNotExists(agent, desired, reqLogger); err != nil {
		return pkgerrors.Wrap(err, "failed to create litellm key secret")
	}

	reqLogger.Info("Created LiteLLM virtual key and secret", "secret", secretName)
	return nil
}
```

**Imports required** (already present in litellm.go): `context`, `"github.com/go-logr/logr"`, `mmv1beta`, `mattermostApp`, `pkgerrors`, `corev1`, `k8sErrors`, `"k8s.io/apimachinery/pkg/types"`.

### 2.1b: Wire into controller reconcile loop — `controllers/mattermost/agent/controller.go`

In the `Reconcile` method, inside the post-health-check LiteLLM block (lines 185-196), add the virtual key reconciliation **after** `reconcileAgentModel`. The block currently ends at line 196. Replace the block to also call `reconcileLiteLLMVirtualKey`:

**Current code (lines 184-196):**
```go
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
```

**New code:**
```go
	// Register agent pod endpoint as a model in LiteLLM and create virtual key
	// (after health check confirms agent is running).
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

		// Collect MCP access groups from the earlier reconcileLiteLLMMCPServers call.
		// Re-derive them here rather than threading state through the reconcile loop.
		mcpAccessGroups, err := r.reconcileLiteLLMMCPServers(ctx, agent, litellmURL, masterKey, reqLogger)
		if err != nil {
			r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
			return reconcile.Result{}, err
		}

		if err := r.reconcileLiteLLMVirtualKey(ctx, agent, litellmURL, masterKey, mcpAccessGroups, reqLogger); err != nil {
			r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
			return reconcile.Result{}, err
		}
	}
```

**Note on MCP access groups:** The `reconcileLiteLLMMCPServers` call is already made earlier in the reconcile loop (line 133-134). However, the return value (`mcpAccessGroups`) is discarded. We call it again here because:
1. It's idempotent (skips already-registered servers)
2. It returns the access groups we need for virtual key creation
3. This avoids threading state through the reconcile loop

**Alternative (cleaner):** Refactor to capture the `mcpAccessGroups` return value from the first call at line 133 and pass it through. This is a judgment call for the implementer — either approach works.

---

## Task 2.2: Add Hook Secret Generation

### 2.2a: Add `HookSecretName()` helper — `apis/mattermost/v1beta1/agent_utils.go`

Add after `LiteLLMKeySecretName()` (after line 94):

```go
// HookSecretName returns the name of the K8s Secret storing this agent's hook secret.
func (a *Agent) HookSecretName() string {
	return "agent-" + a.Name + "-hook-secret"
}
```

### 2.2b: Add `GenerateAgentHookSecret` — `pkg/mattermost/agent.go`

Add after `GenerateAgentBotTokenSecret` (after line 363):

```go
// GenerateAgentHookSecret returns the Secret storing the agent's hook secret.
func GenerateAgentHookSecret(agent *mmv1beta.Agent, secretValue string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            agent.HookSecretName(),
			Namespace:       agent.Namespace,
			Labels:          mmv1beta.AgentLabels(agent.Name),
			OwnerReferences: AgentOwnerReference(agent),
		},
		Data: map[string][]byte{"hookSecret": []byte(secretValue)},
	}
}
```

### 2.2c: Add `checkHookSecret` step — `controllers/mattermost/agent/agent.go`

Add after `checkAgentServiceAccount` (after line 33). This needs `crypto/rand` and `encoding/hex` imports.

**Add to imports:**
```go
import (
	"crypto/rand"
	"encoding/hex"
	// ... existing imports
)
```

**Add function:**
```go
func (r *AgentReconciler) checkHookSecret(ctx context.Context, agent *mmv1beta.Agent, reqLogger logr.Logger) error {
	secretName := agent.HookSecretName()
	existingSecret := &corev1.Secret{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: secretName, Namespace: agent.Namespace}, existingSecret)
	if err == nil {
		return nil // Secret already exists
	}
	if !k8sErrors.IsNotFound(err) {
		return errors.Wrap(err, "failed to check for existing hook secret")
	}

	// Generate a random 32-byte hex string.
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return errors.Wrap(err, "failed to generate random hook secret")
	}
	secretValue := hex.EncodeToString(b)

	desired := mattermostApp.GenerateAgentHookSecret(agent, secretValue)
	if err := r.Resources.CreateSecretIfNotExists(agent, desired, reqLogger); err != nil {
		return errors.Wrap(err, "failed to create hook secret")
	}

	reqLogger.Info("Created hook secret", "secret", secretName)
	return nil
}
```

**Add `k8sErrors` import** to `agent.go` if not already present:
```go
k8sErrors "k8s.io/apimachinery/pkg/api/errors"
```

### 2.2d: Wire `checkHookSecret` into reconcile loop — `controllers/mattermost/agent/controller.go`

Add the call **after `checkAgentServiceAccount`** and **before `checkAgentService`**. This ensures the hook secret exists before the Deployment is created (since the Deployment needs to reference it).

**Insert between lines 150 and 152 (after ServiceAccount, before Service):**

```go
	// Hook Secret
	err = r.checkHookSecret(ctx, agent, reqLogger)
	if err != nil {
		r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
		return reconcile.Result{}, err
	}
```

The reconcile loop order becomes:
1. Mattermost CR readiness check
2. LiteLLM gateway (Deployment, Service, ready check, models, MCP servers)
3. Phase transition → Deploying
4. **ServiceAccount**
5. **Hook Secret** ← NEW
6. **Service**
7. **Deployment**
8. **NetworkPolicy**
9. Health check
10. Agent model registration + virtual key ← NEW (virtual key part)
11. Status update

### 2.2e: Inject `HOOK_SECRET` env var into Deployment — `pkg/mattermost/agent.go`

In `GenerateAgentDeployment`, add the `HOOK_SECRET` env var to `baseEnv` after the `MM_BOT_TOKEN` entry (after line 91, before the LiteLLM gateway block):

**Insert after line 91 (the closing `}` of `MM_BOT_TOKEN`):**

```go
		{
			Name: "HOOK_SECRET",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: agent.HookSecretName(),
					},
					Key: "hookSecret",
				},
			},
		},
```

This goes into `baseEnv` so it's always present and user `spec.env` can override it via `mergeEnvVars`.

---

## Task 2.3: Tests

### 2.3a: Test `reconcileLiteLLMVirtualKey` — `controllers/mattermost/agent/litellm_test.go`

Add these tests at the end of the file:

```go
// ─── Virtual key reconciliation tests ──────────────────────────────────────

func TestReconcileLiteLLMVirtualKey_CreatesKeyAndSecret(t *testing.T) {
	agent := newTestAgentWithLLMGateway()

	var keyReqCaptured liteLLMKeyRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/key/generate":
			err := json.NewDecoder(r.Body).Decode(&keyReqCaptured)
			require.NoError(t, err)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(liteLLMKeyResponse{
				Key:      "sk-virtual-key-abc",
				KeyAlias: keyReqCaptured.KeyAlias,
				Token:    "tok-hash-123",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	reconciler, _ := setupReconciler(t, agent)
	logger := testLogger()

	err := reconciler.reconcileLiteLLMVirtualKey(
		context.TODO(), agent, srv.URL, "master-key",
		[]string{"test-agent_jira_agent_alpha"}, logger,
	)
	require.NoError(t, err)

	// Verify the key request.
	assert.Equal(t, agent.Name, keyReqCaptured.KeyAlias)
	assert.Equal(t, []string{agent.Name}, keyReqCaptured.Models)
	assert.Equal(t, []string{"test-agent_jira_agent_alpha"}, keyReqCaptured.ObjectPermission.MCPAccessGroups)

	// Verify Secret was created.
	secret := &corev1.Secret{}
	err = reconciler.Client.Get(context.TODO(), types.NamespacedName{
		Name:      agent.LiteLLMKeySecretName(),
		Namespace: agent.Namespace,
	}, secret)
	require.NoError(t, err)
	assert.Equal(t, []byte("sk-virtual-key-abc"), secret.Data["apiKey"])
}

func TestReconcileLiteLLMVirtualKey_SecretAlreadyExists(t *testing.T) {
	agent := newTestAgentWithLLMGateway()

	// Pre-create the Secret.
	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agent.LiteLLMKeySecretName(),
			Namespace: agent.Namespace,
		},
		Data: map[string][]byte{"apiKey": []byte("existing-key")},
	}

	// Mock server should NOT be called.
	apiCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalled = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	reconciler, _ := setupReconciler(t, agent, existingSecret)
	logger := testLogger()

	err := reconciler.reconcileLiteLLMVirtualKey(context.TODO(), agent, srv.URL, "master-key", nil, logger)
	require.NoError(t, err)
	assert.False(t, apiCalled, "expected no API calls when Secret already exists")
}

func TestReconcileLiteLLMVirtualKey_KeyAliasAlreadyExists(t *testing.T) {
	agent := newTestAgentWithLLMGateway()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate LiteLLM returning key_alias already exists.
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "Key with alias already exists: test-agent"}`))
	}))
	defer srv.Close()

	reconciler, _ := setupReconciler(t, agent)
	logger := testLogger()

	// Should not error — treated as non-fatal.
	err := reconciler.reconcileLiteLLMVirtualKey(context.TODO(), agent, srv.URL, "master-key", nil, logger)
	require.NoError(t, err)

	// Secret should NOT exist (we can't recover the key value).
	secret := &corev1.Secret{}
	err = reconciler.Client.Get(context.TODO(), types.NamespacedName{
		Name:      agent.LiteLLMKeySecretName(),
		Namespace: agent.Namespace,
	}, secret)
	require.Error(t, err, "secret should not exist when key alias exists but we can't recover the key")
}

func TestReconcileLiteLLMVirtualKey_NoMCPAccessGroups(t *testing.T) {
	agent := newTestAgentWithLLMGateway()

	var keyReqCaptured liteLLMKeyRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/key/generate" {
			json.NewDecoder(r.Body).Decode(&keyReqCaptured)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(liteLLMKeyResponse{Key: "sk-key", KeyAlias: agent.Name})
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	reconciler, _ := setupReconciler(t, agent)
	logger := testLogger()

	err := reconciler.reconcileLiteLLMVirtualKey(context.TODO(), agent, srv.URL, "master-key", nil, logger)
	require.NoError(t, err)

	// object_permission should be zero-value (no mcp_access_groups).
	assert.Nil(t, keyReqCaptured.ObjectPermission.MCPAccessGroups)
}
```

**Additional imports needed in litellm_test.go:** `"net/http/httptest"` is already imported.

### 2.3b: Test `checkHookSecret` — `controllers/mattermost/agent/agent_test.go`

Add at the end of the existing `agent_test.go` file (which already has `newTestAgent`, `setupReconciler`, etc.):

```go
func TestCheckHookSecret_CreatesSecret(t *testing.T) {
	agent := newTestAgent()
	_ = agent.SetDefaults()

	reconciler, _ := setupReconciler(t, agent)
	logger := testLogger()

	err := reconciler.checkHookSecret(context.TODO(), agent, logger)
	require.NoError(t, err)

	// Verify Secret was created.
	secret := &corev1.Secret{}
	err = reconciler.Client.Get(context.TODO(), types.NamespacedName{
		Name:      agent.HookSecretName(),
		Namespace: agent.Namespace,
	}, secret)
	require.NoError(t, err)
	assert.Contains(t, secret.Data, "hookSecret")

	// Verify the hook secret is a 64-character hex string (32 bytes encoded).
	hookSecret := string(secret.Data["hookSecret"])
	assert.Len(t, hookSecret, 64)
}

func TestCheckHookSecret_Idempotent(t *testing.T) {
	agent := newTestAgent()
	_ = agent.SetDefaults()

	// Pre-create the hook secret.
	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agent.HookSecretName(),
			Namespace: agent.Namespace,
		},
		Data: map[string][]byte{"hookSecret": []byte("pre-existing-value")},
	}

	reconciler, _ := setupReconciler(t, agent, existingSecret)
	logger := testLogger()

	err := reconciler.checkHookSecret(context.TODO(), agent, logger)
	require.NoError(t, err)

	// Verify original value is preserved.
	secret := &corev1.Secret{}
	err = reconciler.Client.Get(context.TODO(), types.NamespacedName{
		Name:      agent.HookSecretName(),
		Namespace: agent.Namespace,
	}, secret)
	require.NoError(t, err)
	assert.Equal(t, []byte("pre-existing-value"), secret.Data["hookSecret"])
}
```

**Additional imports needed in agent_test.go:** Add `"context"` (already present). `testLogger()` is defined in `litellm_test.go` in the same package — it's available.

### 2.3c: Test `GenerateAgentHookSecret` — `pkg/mattermost/agent_test.go`

Add at the end of the file:

```go
func TestGenerateAgentHookSecret(t *testing.T) {
	agent := testAgent("my-agent", "default")
	secret := GenerateAgentHookSecret(agent, "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890")

	assert.Equal(t, agent.HookSecretName(), secret.Name)
	assert.Equal(t, "default", secret.Namespace)
	assert.Equal(t, mmv1beta.AgentLabels("my-agent"), secret.Labels)
	assert.Len(t, secret.OwnerReferences, 1)
	assert.Equal(t, "Agent", secret.OwnerReferences[0].Kind)
	assert.Equal(t, []byte("abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"), secret.Data["hookSecret"])

	// Must NOT have other keys
	assert.NotContains(t, secret.Data, "token")
	assert.NotContains(t, secret.Data, "apiKey")
}
```

### 2.3d: Test `HOOK_SECRET` env var in Deployment — `pkg/mattermost/agent_test.go`

Add to `TestGenerateAgentDeployment` (around line 109, after existing env var checks):

```go
	// HOOK_SECRET from secret
	var hookSecretEnv *corev1.EnvVar
	for i, e := range c.Env {
		if e.Name == "HOOK_SECRET" {
			hookSecretEnv = &c.Env[i]
		}
	}
	require.NotNil(t, hookSecretEnv, "HOOK_SECRET env var must be present")
	require.NotNil(t, hookSecretEnv.ValueFrom)
	require.NotNil(t, hookSecretEnv.ValueFrom.SecretKeyRef)
	assert.Equal(t, agent.HookSecretName(), hookSecretEnv.ValueFrom.SecretKeyRef.Name)
	assert.Equal(t, "hookSecret", hookSecretEnv.ValueFrom.SecretKeyRef.Key)
```

### 2.3e: Update `TestReconcileAgent_FullReconcile` — `controllers/mattermost/agent/controller_test.go`

The full reconcile test (line 78) does not use LLMGateway, so the virtual key path is not exercised. The hook secret IS created for all agents. Add verification after the second reconcile (after line 166):

```go
	// Verify hook secret was created.
	hookSecret := &corev1.Secret{}
	err = c.Get(context.TODO(), types.NamespacedName{
		Name:      agent.HookSecretName(),
		Namespace: agent.Namespace,
	}, hookSecret)
	require.NoError(t, err, "hook secret should be created during reconcile")
	assert.Contains(t, hookSecret.Data, "hookSecret")
	assert.Len(t, string(hookSecret.Data["hookSecret"]), 64, "hook secret should be 64-char hex")
```

### 2.3f: Update `TestReconcileAgent_WithLLMGateway` — `controllers/mattermost/agent/controller_test.go`

This test verifies the LLMGateway path but doesn't simulate agent readiness (it only gets to the health-check requeue). The virtual key is created in the post-health-check block, so it won't be tested in this existing test. However, the hook secret should be verified. Add after line 376 (before the closing `}`):

```go
	// Verify hook secret was created (created before Deployment, no LLM dependency).
	hookSecret := &corev1.Secret{}
	err = c.Get(context.TODO(), types.NamespacedName{
		Name:      agent.HookSecretName(),
		Namespace: agent.Namespace,
	}, hookSecret)
	require.NoError(t, err, "hook secret should be created during reconcile")
	assert.Contains(t, hookSecret.Data, "hookSecret")
```

---

## File Change Summary

| File | Action | Summary |
|------|--------|---------|
| `controllers/mattermost/agent/litellm.go` | Add function | `reconcileLiteLLMVirtualKey` (~40 lines) |
| `controllers/mattermost/agent/controller.go` | Modify | Add virtual key call in post-health-check block, add hook secret call before Service |
| `controllers/mattermost/agent/agent.go` | Add function + imports | `checkHookSecret` (~25 lines), add `crypto/rand`, `encoding/hex`, `k8sErrors` imports |
| `apis/mattermost/v1beta1/agent_utils.go` | Add method | `HookSecretName()` (3 lines) |
| `pkg/mattermost/agent.go` | Add function + modify | `GenerateAgentHookSecret` (12 lines), add `HOOK_SECRET` env var to Deployment generator |
| `controllers/mattermost/agent/litellm_test.go` | Add tests | 4 tests for `reconcileLiteLLMVirtualKey` |
| `controllers/mattermost/agent/agent_test.go` | Add tests | 2 tests for `checkHookSecret` |
| `pkg/mattermost/agent_test.go` | Add + modify tests | `TestGenerateAgentHookSecret`, add HOOK_SECRET assertion to `TestGenerateAgentDeployment` |
| `controllers/mattermost/agent/controller_test.go` | Modify tests | Add hook secret verification to `TestReconcileAgent_FullReconcile` and `TestReconcileAgent_WithLLMGateway` |

## Reconcile Loop Order (Final)

```
1.  Fetch Agent CR
2.  Set initial state (Provisioning)
3.  Apply defaults
4.  Check Mattermost CR readiness
5.  [if LLMGateway] LiteLLM Deployment + ConfigMap
6.  [if LLMGateway] LiteLLM Service
7.  [if LLMGateway] LiteLLM readiness check
8.  [if LLMGateway] Get master key
9.  [if LLMGateway] Reconcile LLM provider models
10. [if LLMGateway] Reconcile MCP servers
11. Phase → Deploying
12. ServiceAccount
13. Hook Secret                          ← NEW
14. Service
15. Deployment
16. NetworkPolicy
17. Health check
18. [if LLMGateway] Get master key (post-health)
19. [if LLMGateway] Register agent model
20. [if LLMGateway] Reconcile MCP servers (re-derive access groups)
21. [if LLMGateway] Create virtual key   ← NEW
22. Update status
```

## Testing Strategy

```bash
# Unit tests — controller and client
go test ./controllers/mattermost/agent/... -v

# Unit tests — resource generators
go test ./pkg/mattermost/... -v

# Full suite
make unittest
```

## Definition of Done

- [x] Virtual key created via LiteLLM API and stored in K8s Secret during reconciliation
- [x] Agent pod receives virtual key via existing `OPENAI_API_KEY`/`ANTHROPIC_API_KEY` env vars (plumbing already exists)
- [x] Hook secret generated (random 32-byte hex) and stored in K8s Secret
- [x] Hook secret injected into agent pod as `HOOK_SECRET` env var
- [x] All tests pass (`make unittest`)

---

## Completion Summary

**Completed:** 2026-04-05

All tasks implemented per the prescriptive plan:

### Task 2.1: Virtual Key Creation
- Added `reconcileLiteLLMVirtualKey` to `litellm.go` — checks for existing Secret, calls `generateKey` API, stores result in K8s Secret
- Wired into controller reconcile loop after `reconcileAgentModel` in the post-health-check LiteLLM block
- Re-derives MCP access groups via idempotent `reconcileLiteLLMMCPServers` call (avoids threading state)

### Task 2.2: Hook Secret Generation
- Added `HookSecretName()` helper to `agent_utils.go`
- Added `GenerateAgentHookSecret` resource generator to `pkg/mattermost/agent.go`
- Added `checkHookSecret` to `controllers/mattermost/agent/agent.go` — generates random 32-byte hex, creates K8s Secret
- Wired into reconcile loop after ServiceAccount, before Service
- Added `HOOK_SECRET` env var to `GenerateAgentDeployment` baseEnv

### Task 2.3: Tests
- 4 tests for `reconcileLiteLLMVirtualKey` (create, idempotent, key-alias-exists, no-mcp-groups)
- 2 tests for `checkHookSecret` (create, idempotent)
- 1 test for `GenerateAgentHookSecret`
- HOOK_SECRET env var assertion added to `TestGenerateAgentDeployment`
- Hook secret verification added to `TestReconcileAgent_FullReconcile` and `TestReconcileAgent_WithLLMGateway`

### Test Results
```
go test ./controllers/mattermost/agent/... -v -count=1  → PASS (all 39 tests)
go test ./pkg/mattermost/... -v -count=1                → PASS (all tests)
```
