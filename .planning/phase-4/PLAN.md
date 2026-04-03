# Phase 4: MCP Server Registration — Prescriptive Plan

> **Milestone:** M3 — Agent Secret Protection (LiteLLM Gateway)
> **Target repo:** `~/workspace/worktrees/mattermost-operator-the-trail`
> **Phase:** 4 of 6
> **Depends on:** Phase 3 (reconciler with `litellm.go`, `checkLiteLLMDeployment`, `reconcileLiteLLMVirtualKey`)
> **Goal:** Operator registers MCP server entries in LiteLLM per agent, resolves credentials from K8s Secrets, and passes the resulting MCP access group list to virtual key creation.

---

## Context: What Already Exists After Phase 3

These files are fully implemented. Do NOT recreate them — Phase 4 only appends to `litellm.go` and `litellm_test.go`, and makes one edit to `controller.go`.

- `controllers/mattermost/agent/litellm_client.go` — `liteLLMClient`, `listMCPServers()`, `registerMCPServer()`, `updateMCPServer()`, `SanitizeMCPServerName()`, `errKeyAliasExists`
- `controllers/mattermost/agent/litellm.go` — `checkLiteLLMDeployment`, `checkLiteLLMService`, `checkLiteLLMReady`, `getLiteLLMMasterKey`, `reconcileLiteLLMModels`, `reconcileLiteLLMVirtualKey`, `buildProviderEnvVars`, `litellmAnnotator`
- `controllers/mattermost/agent/controller.go` — LiteLLM reconcile block inserted after `checkAgentBot`, calling all Phase 3 functions. Currently calls `reconcileLiteLLMVirtualKey` with an empty `mcpAccessGroups` slice.
- `controllers/mattermost/agent/litellm_test.go` — 8 tests for Phase 3 functions (created by Phase 3 implementation)
- `apis/mattermost/v1beta1/agent_types.go` — `AgentMCPServer` with fields: `Name`, `URL`, `CredentialSecret`, `MCPAccessGroup`, `AllowedTools`, `DisallowedTools`

**Module path:** `github.com/mattermost/mattermost-operator`

**Key constraint from spikes:**
- `POST /v1/mcp/server` creates duplicates — MUST list-then-create
- Server names: only `[A-Za-z0-9_.]` — no hyphens; use `SanitizeMCPServerName()`
- Credentials: `{"credentials": {"auth_value": "..."}}` (nested, not top-level)
- Key MCP access: `{"object_permission": {"mcp_access_groups": [...]}}` on `POST /key/generate`
- Access group naming: per-agent = `<agentName>_<sanitizedName>`, shared = derived from `MCPAccessGroup` field if set

---

## Task 4.1: Append `reconcileLiteLLMMCPServers` to `controllers/mattermost/agent/litellm.go`

**File:** `controllers/mattermost/agent/litellm.go`
**Action:** Append (do NOT overwrite — the file already has Phase 3 content)

Append the following at the end of `litellm.go`, after `buildProviderEnvVars`.

First, add `"context"` is already imported. Verify the import block already has:
- `"context"` ✓
- `"errors"` ✓ (stdlib, for `errors.Is`)
- `"strings"` ✓
- `objectMatcher` ✓
- `"github.com/go-logr/logr"` ✓
- `mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"` ✓
- `mattermostApp "github.com/mattermost/mattermost-operator/pkg/mattermost"` ✓
- `pkgerrors "github.com/pkg/errors"` ✓
- `appsv1 "k8s.io/api/apps/v1"` ✓
- `corev1 "k8s.io/api/core/v1"` ✓
- `k8sErrors "k8s.io/apimachinery/pkg/api/errors"` ✓
- `"k8s.io/apimachinery/pkg/types"` ✓

No new imports are needed — all are already present.

**Append this code block:**

```go
// ─── MCP server registration ───────────────────────────────────────────────────

// reconcileLiteLLMMCPServers registers all MCP servers configured in agent.Spec.MCPServers
// into the LiteLLM instance, resolving credentials from K8s Secrets.
//
// Returns the list of MCP access group names that should be granted to this agent's
// virtual key. The caller passes this list to reconcileLiteLLMVirtualKey.
//
// Idempotency: POST /v1/mcp/server is NOT idempotent — it creates duplicates.
// This function lists existing servers first and only creates missing ones.
func (r *AgentReconciler) reconcileLiteLLMMCPServers(ctx context.Context, agent *mmv1beta.Agent, litellmURL, masterKey string, reqLogger logr.Logger) ([]string, error) {
	if len(agent.Spec.MCPServers) == 0 {
		return nil, nil
	}

	c := newLiteLLMClient(litellmURL, masterKey)

	// Build a set of existing server names to check before creating.
	existing, err := c.listMCPServers()
	if err != nil {
		return nil, pkgerrors.Wrap(err, "failed to list existing mcp servers")
	}
	existingByName := make(map[string]struct{}, len(existing))
	for _, srv := range existing {
		existingByName[srv.ServerName] = struct{}{}
	}

	var accessGroups []string

	for _, mcpServer := range agent.Spec.MCPServers {
		sanitizedName := SanitizeMCPServerName(mcpServer.Name)

		// Determine the MCP access group for this server.
		// If the spec provides an explicit MCPAccessGroup, use it as-is.
		// Otherwise, derive a per-agent name: "<agentName>_<sanitizedServerName>".
		accessGroup := mcpServer.MCPAccessGroup
		if accessGroup == "" {
			accessGroup = agent.Name + "_" + sanitizedName
		}
		accessGroups = append(accessGroups, accessGroup)

		// Skip registration if the server is already present in LiteLLM.
		if _, found := existingByName[sanitizedName]; found {
			reqLogger.Info("MCP server already registered, skipping", "server", sanitizedName)
			continue
		}

		// Resolve credential from K8s Secret (optional — some MCP servers are unauthenticated).
		credentialValue := ""
		if mcpServer.CredentialSecret != "" {
			credSecret := &corev1.Secret{}
			err := r.Client.Get(ctx, types.NamespacedName{
				Name:      mcpServer.CredentialSecret,
				Namespace: agent.Namespace,
			}, credSecret)
			if err != nil {
				return nil, pkgerrors.Wrapf(err, "failed to get credential secret %q for mcp server %q", mcpServer.CredentialSecret, mcpServer.Name)
			}
			credentialValue = string(credSecret.Data["apiKey"])
		}

		// Build the registration request.
		req := liteLLMMCPServerRequest{
			ServerName:      sanitizedName,
			URL:             mcpServer.URL,
			Transport:       "http",
			MCPAccessGroups: []string{accessGroup},
		}
		if credentialValue != "" {
			req.AuthType = "bearer_token"
			req.Credentials = liteLLMCredentials{AuthValue: credentialValue}
		}

		reqLogger.Info("Registering MCP server", "server", sanitizedName, "accessGroup", accessGroup)
		if _, err := c.registerMCPServer(req); err != nil {
			return nil, pkgerrors.Wrapf(err, "failed to register mcp server %q", sanitizedName)
		}
	}

	return accessGroups, nil
}
```

---

## Task 4.2: Edit `controller.go` — wire `reconcileLiteLLMMCPServers` into the reconcile block

**File:** `controllers/mattermost/agent/controller.go`
**Action:** Edit the LiteLLM reconcile block inserted by Phase 3

The current Phase 3 LiteLLM block (inserted after `checkAgentBot`, before `checkAgentServiceAccount`) looks like this:

```go
// LiteLLM gateway lifecycle (operator-managed only).
if agent.Spec.LLMGateway != nil && agent.Spec.LLMGateway.OperatorManaged != nil {
    if err = r.checkLiteLLMDeployment(ctx, agent, reqLogger); err != nil {
        r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
        return reconcile.Result{}, err
    }
    if err = r.checkLiteLLMService(ctx, agent, reqLogger); err != nil {
        r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
        return reconcile.Result{}, err
    }
    ready, err := r.checkLiteLLMReady(ctx, agent, reqLogger)
    if err != nil {
        r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
        return reconcile.Result{}, err
    }
    if !ready {
        return reconcile.Result{RequeueAfter: mattermostNotReadyDelay}, nil
    }
    masterKey, err := r.getLiteLLMMasterKey(ctx, agent.Namespace)
    if err != nil {
        r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
        return reconcile.Result{}, err
    }
    litellmURL := mattermostApp.LiteLLMServiceURL(agent.Namespace)
    if err = r.reconcileLiteLLMModels(ctx, agent, litellmURL, masterKey, reqLogger); err != nil {
        r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
        return reconcile.Result{}, err
    }
    if err = r.reconcileLiteLLMVirtualKey(ctx, agent, litellmURL, masterKey, nil, reqLogger); err != nil {
        r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
        return reconcile.Result{}, err
    }
}
```

**Replace** the last two lines of the inner block (the `reconcileLiteLLMVirtualKey` call and its error handling) with the MCP registration call followed by the updated virtual key call:

**Find this exact code** (the end of the `if agent.Spec.LLMGateway != nil` block):
```go
    if err = r.reconcileLiteLLMVirtualKey(ctx, agent, litellmURL, masterKey, nil, reqLogger); err != nil {
        r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
        return reconcile.Result{}, err
    }
```

**Replace with:**
```go
    mcpAccessGroups, err := r.reconcileLiteLLMMCPServers(ctx, agent, litellmURL, masterKey, reqLogger)
    if err != nil {
        r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
        return reconcile.Result{}, err
    }
    if err = r.reconcileLiteLLMVirtualKey(ctx, agent, litellmURL, masterKey, mcpAccessGroups, reqLogger); err != nil {
        r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
        return reconcile.Result{}, err
    }
```

**No new imports are needed** — `mattermostApp` is already imported in `controller.go` after Phase 3.

---

## Task 4.3: Append MCP reconciler tests to `controllers/mattermost/agent/litellm_test.go`

**File:** `controllers/mattermost/agent/litellm_test.go`
**Action:** Append (do NOT overwrite — the file already has 8 Phase 3 tests)

The test file uses these fixtures already defined in `agent_test.go`:
- `newTestAgent()` — returns a basic Agent with no LLMGateway
- `setupReconciler(t, objs...)` — returns `(*AgentReconciler, *runtime.Scheme)` with fake client
- `blubr.InitLogger` / `logr.New` pattern for logger construction

Phase 4 tests need an agent with MCPServers configured. Add a new fixture and 4 tests.

**Append this entire block to the end of `litellm_test.go`:**

```go
// ─── MCP server registration tests ──────────────────────────────────────────

// newTestAgentWithMCPServers returns an Agent configured with both LLMGateway
// and MCPServers for Phase 4 tests.
func newTestAgentWithMCPServers() *mmv1beta.Agent {
	agent := newTestAgentWithLLMGateway()
	agent.Spec.MCPServers = []mmv1beta.AgentMCPServer{
		{
			Name:             "jira-agent-alpha",
			URL:              "http://jira-mcp.tools.svc.cluster.local:8080/mcp",
			CredentialSecret: "jira-mcp-secret",
		},
		{
			Name: "github-shared",
			URL:  "http://github-mcp.tools.svc.cluster.local:8080/mcp",
			// No CredentialSecret — unauthenticated MCP server.
		},
	}
	return agent
}

func TestReconcileLiteLLMMCPServers_NoServers(t *testing.T) {
	agent := newTestAgentWithLLMGateway()
	// agent has no MCPServers configured.

	reconciler, _ := setupReconciler(t, agent)
	logSink := blubr.InitLogger(logrus.NewEntry(logrus.New()))
	logger := logr.New(logSink.WithName("test"))

	// Build a mock LiteLLM server — it should NOT be called.
	apiCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalled = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	groups, err := reconciler.reconcileLiteLLMMCPServers(context.TODO(), agent, srv.URL, "test-master-key", logger)
	require.NoError(t, err)
	assert.Nil(t, groups)
	assert.False(t, apiCalled, "expected no API calls when MCPServers is empty")
}

func TestReconcileLiteLLMMCPServers_CreatesWhenMissing(t *testing.T) {
	agent := newTestAgentWithMCPServers()

	// Pre-create the credential Secret for the first MCP server.
	jiraSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "jira-mcp-secret",
			Namespace: agent.Namespace,
		},
		Data: map[string][]byte{"apiKey": []byte("jira-token-abc")},
	}

	reconciler, _ := setupReconciler(t, agent, jiraSecret)
	logSink := blubr.InitLogger(logrus.NewEntry(logrus.New()))
	logger := logr.New(logSink.WithName("test"))

	var registeredServers []liteLLMMCPServerRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/v1/mcp/server":
			// Return empty list — no servers registered yet.
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode([]liteLLMMCPServerResponse{})
		case r.Method == "POST" && r.URL.Path == "/v1/mcp/server":
			var req liteLLMMCPServerRequest
			err := json.NewDecoder(r.Body).Decode(&req)
			require.NoError(t, err)
			registeredServers = append(registeredServers, req)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(liteLLMMCPServerResponse{
				ServerID:   "srv-" + req.ServerName,
				ServerName: req.ServerName,
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	groups, err := reconciler.reconcileLiteLLMMCPServers(context.TODO(), agent, srv.URL, "test-master-key", logger)
	require.NoError(t, err)

	// Two servers should have been registered.
	require.Len(t, registeredServers, 2)

	// Verify first server (jira — has credential).
	jira := registeredServers[0]
	assert.Equal(t, "jira_agent_alpha", jira.ServerName) // hyphen sanitized
	assert.Equal(t, "http://jira-mcp.tools.svc.cluster.local:8080/mcp", jira.URL)
	assert.Equal(t, "bearer_token", jira.AuthType)
	assert.Equal(t, "jira-token-abc", jira.Credentials.AuthValue)

	// Verify second server (github — no credential).
	github := registeredServers[1]
	assert.Equal(t, "github_shared", github.ServerName)
	assert.Equal(t, "", github.AuthType) // no auth
	assert.Equal(t, "", github.Credentials.AuthValue)

	// Verify returned access groups.
	require.Len(t, groups, 2)
	// Default access group: "<agentName>_<sanitizedServerName>"
	assert.Equal(t, agent.Name+"_jira_agent_alpha", groups[0])
	assert.Equal(t, agent.Name+"_github_shared", groups[1])
}

func TestReconcileLiteLLMMCPServers_SkipsExisting(t *testing.T) {
	agent := newTestAgentWithMCPServers()

	// Pre-create credential secret.
	jiraSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "jira-mcp-secret",
			Namespace: agent.Namespace,
		},
		Data: map[string][]byte{"apiKey": []byte("jira-token-abc")},
	}

	reconciler, _ := setupReconciler(t, agent, jiraSecret)
	logSink := blubr.InitLogger(logrus.NewEntry(logrus.New()))
	logger := logr.New(logSink.WithName("test"))

	postCallCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/v1/mcp/server":
			// Return both servers as already existing.
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode([]liteLLMMCPServerResponse{
				{ServerID: "srv-1", ServerName: "jira_agent_alpha"},
				{ServerID: "srv-2", ServerName: "github_shared"},
			})
		case r.Method == "POST" && r.URL.Path == "/v1/mcp/server":
			postCallCount++
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(liteLLMMCPServerResponse{})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	groups, err := reconciler.reconcileLiteLLMMCPServers(context.TODO(), agent, srv.URL, "test-master-key", logger)
	require.NoError(t, err)
	assert.Equal(t, 0, postCallCount, "expected no POST calls when all servers already exist")

	// Access groups are still returned even when creation was skipped.
	require.Len(t, groups, 2)
	assert.Equal(t, agent.Name+"_jira_agent_alpha", groups[0])
	assert.Equal(t, agent.Name+"_github_shared", groups[1])
}

func TestReconcileLiteLLMMCPServers_ExplicitAccessGroup(t *testing.T) {
	agent := newTestAgentWithLLMGateway()
	agent.Spec.MCPServers = []mmv1beta.AgentMCPServer{
		{
			Name:           "jira-shared",
			URL:            "http://jira-mcp.tools.svc.cluster.local:8080/mcp",
			MCPAccessGroup: "shared_jira", // explicit access group
		},
	}

	reconciler, _ := setupReconciler(t, agent)
	logSink := blubr.InitLogger(logrus.NewEntry(logrus.New()))
	logger := logr.New(logSink.WithName("test"))

	var capturedReq liteLLMMCPServerRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/v1/mcp/server":
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode([]liteLLMMCPServerResponse{})
		case r.Method == "POST" && r.URL.Path == "/v1/mcp/server":
			err := json.NewDecoder(r.Body).Decode(&capturedReq)
			require.NoError(t, err)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(liteLLMMCPServerResponse{
				ServerID:   "srv-abc",
				ServerName: capturedReq.ServerName,
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	groups, err := reconciler.reconcileLiteLLMMCPServers(context.TODO(), agent, srv.URL, "test-master-key", logger)
	require.NoError(t, err)

	// Access group must be the explicit value, not the derived default.
	require.Len(t, groups, 1)
	assert.Equal(t, "shared_jira", groups[0])

	// Verify the group was also sent in the registration request.
	assert.Equal(t, []string{"shared_jira"}, capturedReq.MCPAccessGroups)
}

func TestReconcileLiteLLMMCPServers_CredentialResolution(t *testing.T) {
	agent := newTestAgentWithLLMGateway()
	agent.Spec.MCPServers = []mmv1beta.AgentMCPServer{
		{
			Name:             "secure-mcp",
			URL:              "http://secure-mcp.svc.cluster.local:8080/mcp",
			CredentialSecret: "secure-mcp-creds",
		},
	}

	// Do NOT pre-create the Secret — should return an error.
	reconciler, _ := setupReconciler(t, agent)
	logSink := blubr.InitLogger(logrus.NewEntry(logrus.New()))
	logger := logr.New(logSink.WithName("test"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/v1/mcp/server" {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode([]liteLLMMCPServerResponse{})
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	groups, err := reconciler.reconcileLiteLLMMCPServers(context.TODO(), agent, srv.URL, "test-master-key", logger)
	require.Error(t, err)
	assert.Nil(t, groups)
	assert.Contains(t, err.Error(), "secure-mcp-creds")
}
```

**Note on `newTestAgentWithLLMGateway()`:** This fixture was defined in `litellm_test.go` by Phase 3 implementation. It must already exist in the file. If it does not, add it before the Phase 4 tests:

```go
func newTestAgentWithLLMGateway() *mmv1beta.Agent {
	agent := newTestAgent()
	agent.Spec.LLMGateway = &mmv1beta.LLMGatewayConfig{
		OperatorManaged: &mmv1beta.OperatorManagedLLMGateway{
			Image: mmv1beta.AgentLiteLLMDefaultImage,
			LLMProviders: []mmv1beta.LLMProvider{
				{
					Name:   "anthropic",
					Secret: "anthropic-key",
					Models: []string{"claude-3-5-sonnet-20241022"},
				},
			},
		},
	}
	return agent
}
```

**Required imports for `litellm_test.go`** (these should already be present from Phase 3; verify before compiling):

```go
import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	blubr "github.com/mattermost/blubr"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
)
```

---

## Task 4.4: Run tests

```bash
cd ~/workspace/worktrees/mattermost-operator-the-trail
go test ./controllers/mattermost/agent/... -v -run TestReconcileLiteLLMMCPServers
```

Then run the full suite to verify no regressions:

```bash
make unittest
```

---

## Definition of Done

- [ ] `reconcileLiteLLMMCPServers` is appended to `litellm.go`
- [ ] `controller.go` calls `reconcileLiteLLMMCPServers` before `reconcileLiteLLMVirtualKey`, passing computed groups
- [ ] `reconcileLiteLLMVirtualKey` no longer receives `nil` for `mcpAccessGroups` — it receives the computed list
- [ ] MCP registration creates entries only when missing (list-then-create)
- [ ] Credentials resolved correctly from K8s Secrets via key `"apiKey"`
- [ ] Access group naming: `<agentName>_<sanitizedName>` by default, explicit `MCPAccessGroup` field when set
- [ ] Unauthenticated servers (no `CredentialSecret`) work correctly — no `AuthType` or `Credentials` in request
- [ ] Missing credential Secret returns a descriptive error containing the secret name
- [ ] All 5 new MCP tests pass
- [ ] `make unittest` passes (no regressions)

---

## Design Decisions

### Why list-then-create instead of a status annotation

The spike confirmed `POST /v1/mcp/server` creates duplicates on every call. The operator cannot use the K8s-side idempotency pattern (checking a Secret) for MCP servers because the LiteLLM DB is the source of truth. The cheapest correct approach: list all servers from LiteLLM on every reconcile and skip any already present. `GET /v1/mcp/server` returns all servers as a JSON array; building a `map[string]struct{}` makes the lookup O(1).

### Why no update path in Phase 4

The spike shows `PUT /v1/mcp/server/{serverID}` exists and works. However, the operator has no way to detect whether an existing server's URL or credentials have changed without storing state. Phase 4 implements create-if-missing only. A future milestone can add update support by comparing the desired URL against the existing entry.

### Access group naming scheme

- **Per-agent default:** `<agentName>_<sanitizedServerName>` — e.g., `myagent_jira_agent_alpha`. Unique per agent+server combination. Ensures different agents get different access groups for the same MCP server, maintaining isolation.
- **Explicit override:** `MCPAccessGroup` field on `AgentMCPServer` — when set, used verbatim. Enables shared access groups (e.g., `shared_jira` used by multiple agents that all need access to the same Jira MCP server).

### Credential key name: `"apiKey"`

Consistent with the `ExternalLLMGateway.VirtualKeySecret` pattern and `GenerateAgentLiteLLMKeySecret` — all credential Secrets use key `"apiKey"`.

### `reconcileLiteLLMVirtualKey` with `nil` vs empty slice

Phase 3 passed `nil` as `mcpAccessGroups` to `reconcileLiteLLMVirtualKey`. After Phase 4, it receives the computed slice (which may be `nil` if there are no MCPServers). Both `nil` and an empty slice serialize to `[]` (or are omitted if `omitempty` is set) in the JSON body — the behavior is identical. The `liteLLMObjectPermission.MCPAccessGroups` field has no `omitempty` tag, so `nil` and `[]string{}` both produce `"mcp_access_groups":null` vs `"mcp_access_groups":[]`. This is acceptable — LiteLLM treats both as no MCP access restriction.
