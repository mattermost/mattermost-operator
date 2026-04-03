# Phase 2: LiteLLM HTTP Client — Prescriptive Plan

> **Milestone:** M3 — Agent Secret Protection (LiteLLM Gateway)
> **Target repo:** `~/workspace/worktrees/mattermost-operator-the-trail`
> **Phase:** 2 of 6
> **Depends on:** Phase 1 (types for function signatures)
> **Goal:** Pure HTTP client for LiteLLM management API. Fully unit-testable with `httptest.NewServer`, no K8s dependencies.

---

## Context: Patterns to Follow

All client code lives in `controllers/mattermost/agent/` (package `agent`). Study these existing files before implementing:

- `controllers/mattermost/agent/agent.go` — the authoritative pattern:
  - `http.NewRequest` → set headers → `http.DefaultClient.Do` → check status → decode
  - Error format: `fmt.Errorf("list bots returned status %d: %s", resp.StatusCode, string(body))`
  - Body read on error: `body, _ := io.ReadAll(resp.Body)`
  - JSON decode: `json.NewDecoder(resp.Body).Decode(&target)`
- `controllers/mattermost/agent/agent_test.go` — the test pattern:
  - `httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { ... }))`
  - Switch on `r.Method + r.URL.Path`
  - `json.NewEncoder(w).Encode(responseStruct)`
  - Logger: `blubr.InitLogger(logrus.NewEntry(logrus.New()))`

All errors use `fmt.Errorf(...)` (not `errors.Wrap` from pkg/errors — that's used in the reconciler layer, not the raw HTTP client).

---

## Task 2.1: Create `controllers/mattermost/agent/litellm_client.go`

**File:** `controllers/mattermost/agent/litellm_client.go`
**Action:** Create (new file)

### API shapes (from spike validation)

**POST /model/new** — register an LLM model:
```json
{
  "model_name": "claude-sonnet",
  "litellm_params": {
    "model": "anthropic/claude-3-5-sonnet-20241022",
    "api_key": "os.environ/ANTHROPIC_API_KEY"
  }
}
```
Response: `200 OK` with `{"model_id": "...", "model_name": "..."}`. Safe to call multiple times (upserts).

**GET /v1/mcp/server** — list all registered MCP servers:
Response: JSON array of server objects, each with at least `server_id` (string) and `server_name` (string).
Note: endpoint is `/v1/mcp/server` (singular), NOT `/v1/mcp/servers`.

**POST /v1/mcp/server** — register an MCP server:
```json
{
  "server_name": "jira_agent_alpha",
  "url": "http://jira-mcp.tools.svc.cluster.local:8080/mcp",
  "transport": "http",
  "auth_type": "bearer_token",
  "credentials": {"auth_value": "secret-token-here"},
  "mcp_access_groups": ["agent_name_jira_agent_alpha"]
}
```
Response: `200 OK`. NOT idempotent — creates duplicates on repeated calls.

**PUT /v1/mcp/server/{server_id}** — update an existing MCP server.

**DELETE /v1/mcp/server/{server_id}** — delete an MCP server.

**POST /key/generate** — create a virtual key:
```json
{
  "key_alias": "agent-my-agent-key",
  "models": ["claude-sonnet"],
  "metadata": {"agent_name": "my-agent", "managed_by": "mattermost-operator"},
  "object_permission": {
    "mcp_access_groups": ["my_agent_jira_agent_alpha", "shared_github"]
  }
}
```
Response: `200 OK` with `{"key": "sk-...", "key_alias": "...", "token": "..."}`.
On duplicate `key_alias`: returns `400` with body containing `"Key with alias already exists"` — treat this as non-fatal (log and return `errKeyAliasExists` sentinel).

**POST /key/update** — update a virtual key in place:
```json
{
  "key": "<token-hash>",
  "object_permission": {
    "mcp_access_groups": ["my_agent_jira_agent_alpha"]
  }
}
```
Response: `200 OK`.

**DELETE /key/delete** — delete a virtual key:
```json
{"keys": ["sk-..."]}
```
Response: `200 OK`.

### File content

```go
// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// errKeyAliasExists is returned by generateKey when the key_alias is already taken.
// Callers should treat this as a non-fatal signal to skip creation.
var errKeyAliasExists = fmt.Errorf("key alias already exists")

// liteLLMClient is a minimal HTTP client for the LiteLLM management API.
type liteLLMClient struct {
	baseURL   string
	masterKey string
}

func newLiteLLMClient(baseURL, masterKey string) *liteLLMClient {
	return &liteLLMClient{baseURL: baseURL, masterKey: masterKey}
}

// ─── Request / Response types ──────────────────────────────────────────────

type liteLLMModelParams struct {
	Model  string `json:"model"`
	APIKey string `json:"api_key"`
}

type liteLLMModelRequest struct {
	ModelName     string             `json:"model_name"`
	LiteLLMParams liteLLMModelParams `json:"litellm_params"`
}

type liteLLMCredentials struct {
	AuthValue string `json:"auth_value"`
}

type liteLLMMCPServerRequest struct {
	ServerName      string             `json:"server_name"`
	URL             string             `json:"url"`
	Transport       string             `json:"transport"`
	AuthType        string             `json:"auth_type,omitempty"`
	Credentials     liteLLMCredentials `json:"credentials,omitempty"`
	MCPAccessGroups []string           `json:"mcp_access_groups,omitempty"`
}

type liteLLMMCPServerResponse struct {
	ServerID   string `json:"server_id"`
	ServerName string `json:"server_name"`
}

type liteLLMObjectPermission struct {
	MCPAccessGroups []string `json:"mcp_access_groups"`
}

type liteLLMKeyRequest struct {
	KeyAlias         string                  `json:"key_alias"`
	Models           []string                `json:"models,omitempty"`
	Metadata         map[string]string       `json:"metadata,omitempty"`
	ObjectPermission liteLLMObjectPermission `json:"object_permission,omitempty"`
}

type liteLLMKeyResponse struct {
	Key      string `json:"key"`
	KeyAlias string `json:"key_alias"`
	Token    string `json:"token"`
}

type liteLLMKeyUpdateRequest struct {
	Key              string                  `json:"key"`
	ObjectPermission liteLLMObjectPermission `json:"object_permission,omitempty"`
}

type liteLLMKeyDeleteRequest struct {
	Keys []string `json:"keys"`
}

// ─── HTTP helpers ──────────────────────────────────────────────────────────

// do executes an authenticated request and returns the response body bytes.
// On non-2xx status it returns a descriptive error including the body.
func (c *liteLLMClient) do(method, path string, body interface{}) ([]byte, int, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.masterKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	return respBody, resp.StatusCode, nil
}

// ─── Model registration ────────────────────────────────────────────────────

// registerModel registers an LLM model via POST /model/new.
// Safe to call multiple times — LiteLLM upserts.
func (c *liteLLMClient) registerModel(modelName, litellmModel, apiKeyEnvRef string) error {
	req := liteLLMModelRequest{
		ModelName: modelName,
		LiteLLMParams: liteLLMModelParams{
			Model:  litellmModel,
			APIKey: apiKeyEnvRef,
		},
	}

	body, status, err := c.do("POST", "/model/new", req)
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return fmt.Errorf("register model returned status %d: %s", status, string(body))
	}
	return nil
}

// ─── MCP server management ─────────────────────────────────────────────────

// listMCPServers returns all registered MCP servers via GET /v1/mcp/server.
func (c *liteLLMClient) listMCPServers() ([]liteLLMMCPServerResponse, error) {
	body, status, err := c.do("GET", "/v1/mcp/server", nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("list mcp servers returned status %d: %s", status, string(body))
	}

	var servers []liteLLMMCPServerResponse
	if err := json.Unmarshal(body, &servers); err != nil {
		return nil, fmt.Errorf("failed to decode mcp server list: %w", err)
	}
	return servers, nil
}

// registerMCPServer registers a new MCP server via POST /v1/mcp/server.
// NOT idempotent — callers must check listMCPServers first.
func (c *liteLLMClient) registerMCPServer(req liteLLMMCPServerRequest) (*liteLLMMCPServerResponse, error) {
	body, status, err := c.do("POST", "/v1/mcp/server", req)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return nil, fmt.Errorf("register mcp server returned status %d: %s", status, string(body))
	}

	var resp liteLLMMCPServerResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to decode mcp server response: %w", err)
	}
	return &resp, nil
}

// updateMCPServer updates an existing MCP server via PUT /v1/mcp/server/{serverID}.
func (c *liteLLMClient) updateMCPServer(serverID string, req liteLLMMCPServerRequest) error {
	body, status, err := c.do("PUT", "/v1/mcp/server/"+serverID, req)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("update mcp server returned status %d: %s", status, string(body))
	}
	return nil
}

// deleteMCPServer deletes an MCP server via DELETE /v1/mcp/server/{serverID}.
func (c *liteLLMClient) deleteMCPServer(serverID string) error {
	body, status, err := c.do("DELETE", "/v1/mcp/server/"+serverID, nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("delete mcp server returned status %d: %s", status, string(body))
	}
	return nil
}

// ─── Virtual key management ────────────────────────────────────────────────

// generateKey creates a virtual key via POST /key/generate.
// Returns errKeyAliasExists if the key_alias is already taken (HTTP 400 with alias-exists message).
// Callers should treat errKeyAliasExists as non-fatal.
func (c *liteLLMClient) generateKey(req liteLLMKeyRequest) (*liteLLMKeyResponse, error) {
	body, status, err := c.do("POST", "/key/generate", req)
	if err != nil {
		return nil, err
	}

	// LiteLLM returns 400 with "Key with alias already exists" for duplicate key_alias.
	if status == http.StatusBadRequest && strings.Contains(string(body), "already exists") {
		return nil, errKeyAliasExists
	}

	if status != http.StatusOK && status != http.StatusCreated {
		return nil, fmt.Errorf("generate key returned status %d: %s", status, string(body))
	}

	var resp liteLLMKeyResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to decode key response: %w", err)
	}
	return &resp, nil
}

// updateKey updates a virtual key in place via POST /key/update.
// key is the token hash returned in liteLLMKeyResponse.Token (or the key value itself).
func (c *liteLLMClient) updateKey(req liteLLMKeyUpdateRequest) error {
	body, status, err := c.do("POST", "/key/update", req)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("update key returned status %d: %s", status, string(body))
	}
	return nil
}

// deleteKey deletes a virtual key via DELETE /key/delete.
func (c *liteLLMClient) deleteKey(keyValue string) error {
	body, status, err := c.do("DELETE", "/key/delete", liteLLMKeyDeleteRequest{Keys: []string{keyValue}})
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("delete key returned status %d: %s", status, string(body))
	}
	return nil
}

// ─── Utility ───────────────────────────────────────────────────────────────

// SanitizeMCPServerName replaces characters invalid in LiteLLM MCP server names.
// LiteLLM only accepts [A-Za-z0-9_.] — hyphens must become underscores.
func SanitizeMCPServerName(name string) string {
	return strings.ReplaceAll(name, "-", "_")
}
```

---

## Task 2.2: Create `controllers/mattermost/agent/litellm_client_test.go`

**File:** `controllers/mattermost/agent/litellm_client_test.go`
**Action:** Create (new file)

### Import pattern (exactly matching `agent_test.go`)

```go
package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)
```

Note: No K8s imports needed — this is a pure HTTP client test file.

### File content

```go
// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── registerModel ─────────────────────────────────────────────────────────

func TestRegisterModel_Success(t *testing.T) {
	var capturedBody liteLLMModelRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "POST", r.Method)
		require.Equal(t, "/model/new", r.URL.Path)
		require.Equal(t, "Bearer test-master-key", r.Header.Get("Authorization"))

		err := json.NewDecoder(r.Body).Decode(&capturedBody)
		require.NoError(t, err)

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"model_id": "m1", "model_name": "claude-sonnet"})
	}))
	defer srv.Close()

	c := newLiteLLMClient(srv.URL, "test-master-key")
	err := c.registerModel("claude-sonnet", "anthropic/claude-3-5-sonnet-20241022", "os.environ/ANTHROPIC_API_KEY")
	require.NoError(t, err)

	assert.Equal(t, "claude-sonnet", capturedBody.ModelName)
	assert.Equal(t, "anthropic/claude-3-5-sonnet-20241022", capturedBody.LiteLLMParams.Model)
	assert.Equal(t, "os.environ/ANTHROPIC_API_KEY", capturedBody.LiteLLMParams.APIKey)
}

func TestRegisterModel_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	c := newLiteLLMClient(srv.URL, "key")
	err := c.registerModel("m", "anthropic/m", "os.environ/KEY")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

// ─── listMCPServers ─────────────────────────────────────────────────────────

func TestListMCPServers_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "GET", r.Method)
		require.Equal(t, "/v1/mcp/server", r.URL.Path)

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]liteLLMMCPServerResponse{})
	}))
	defer srv.Close()

	c := newLiteLLMClient(srv.URL, "key")
	servers, err := c.listMCPServers()
	require.NoError(t, err)
	assert.Empty(t, servers)
}

func TestListMCPServers_WithEntries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]liteLLMMCPServerResponse{
			{ServerID: "srv-1", ServerName: "jira_agent_alpha"},
			{ServerID: "srv-2", ServerName: "github_shared"},
		})
	}))
	defer srv.Close()

	c := newLiteLLMClient(srv.URL, "key")
	servers, err := c.listMCPServers()
	require.NoError(t, err)
	require.Len(t, servers, 2)
	assert.Equal(t, "srv-1", servers[0].ServerID)
	assert.Equal(t, "jira_agent_alpha", servers[0].ServerName)
}

// ─── registerMCPServer ─────────────────────────────────────────────────────

func TestRegisterMCPServer_Success(t *testing.T) {
	var capturedBody liteLLMMCPServerRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "POST", r.Method)
		require.Equal(t, "/v1/mcp/server", r.URL.Path)

		err := json.NewDecoder(r.Body).Decode(&capturedBody)
		require.NoError(t, err)

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(liteLLMMCPServerResponse{
			ServerID:   "srv-abc",
			ServerName: capturedBody.ServerName,
		})
	}))
	defer srv.Close()

	c := newLiteLLMClient(srv.URL, "key")
	req := liteLLMMCPServerRequest{
		ServerName:      "jira_agent_alpha",
		URL:             "http://jira-mcp.tools.svc.cluster.local:8080/mcp",
		Transport:       "http",
		AuthType:        "bearer_token",
		Credentials:     liteLLMCredentials{AuthValue: "secret-token"},
		MCPAccessGroups: []string{"my_agent_jira_agent_alpha"},
	}
	resp, err := c.registerMCPServer(req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "srv-abc", resp.ServerID)

	// Verify nested credentials.auth_value was serialized correctly.
	assert.Equal(t, "secret-token", capturedBody.Credentials.AuthValue)
	assert.Equal(t, "my_agent_jira_agent_alpha", capturedBody.MCPAccessGroups[0])
}

// ─── generateKey ───────────────────────────────────────────────────────────

func TestGenerateKey_Success(t *testing.T) {
	var capturedBody liteLLMKeyRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "POST", r.Method)
		require.Equal(t, "/key/generate", r.URL.Path)

		err := json.NewDecoder(r.Body).Decode(&capturedBody)
		require.NoError(t, err)

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(liteLLMKeyResponse{
			Key:      "sk-abc123",
			KeyAlias: capturedBody.KeyAlias,
			Token:    "tok-hash-xyz",
		})
	}))
	defer srv.Close()

	c := newLiteLLMClient(srv.URL, "key")
	req := liteLLMKeyRequest{
		KeyAlias: "agent-my-agent-key",
		Models:   []string{"claude-sonnet"},
		Metadata: map[string]string{"agent_name": "my-agent"},
		ObjectPermission: liteLLMObjectPermission{
			MCPAccessGroups: []string{"my_agent_jira_agent_alpha", "shared_github"},
		},
	}
	resp, err := c.generateKey(req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "sk-abc123", resp.Key)

	// Verify object_permission.mcp_access_groups was serialized correctly.
	assert.Equal(t, []string{"my_agent_jira_agent_alpha", "shared_github"},
		capturedBody.ObjectPermission.MCPAccessGroups)
}

func TestGenerateKey_DuplicateAlias(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "Key with alias already exists: agent-my-agent-key"}`))
	}))
	defer srv.Close()

	c := newLiteLLMClient(srv.URL, "key")
	resp, err := c.generateKey(liteLLMKeyRequest{KeyAlias: "agent-my-agent-key"})
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, errKeyAliasExists)
}

func TestGenerateKey_OtherError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer srv.Close()

	c := newLiteLLMClient(srv.URL, "key")
	resp, err := c.generateKey(liteLLMKeyRequest{KeyAlias: "x"})
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.NotErrorIs(t, err, errKeyAliasExists)
	assert.Contains(t, err.Error(), "500")
}

// ─── updateKey ─────────────────────────────────────────────────────────────

func TestUpdateKey_Success(t *testing.T) {
	var capturedBody liteLLMKeyUpdateRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "POST", r.Method)
		require.Equal(t, "/key/update", r.URL.Path)

		err := json.NewDecoder(r.Body).Decode(&capturedBody)
		require.NoError(t, err)

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	c := newLiteLLMClient(srv.URL, "key")
	err := c.updateKey(liteLLMKeyUpdateRequest{
		Key: "tok-hash-xyz",
		ObjectPermission: liteLLMObjectPermission{
			MCPAccessGroups: []string{"my_agent_github"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "tok-hash-xyz", capturedBody.Key)
	assert.Equal(t, []string{"my_agent_github"}, capturedBody.ObjectPermission.MCPAccessGroups)
}

// ─── deleteKey ─────────────────────────────────────────────────────────────

func TestDeleteKey_Success(t *testing.T) {
	var capturedBody liteLLMKeyDeleteRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "DELETE", r.Method)
		require.Equal(t, "/key/delete", r.URL.Path)

		err := json.NewDecoder(r.Body).Decode(&capturedBody)
		require.NoError(t, err)

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	c := newLiteLLMClient(srv.URL, "key")
	err := c.deleteKey("sk-abc123")
	require.NoError(t, err)
	require.Len(t, capturedBody.Keys, 1)
	assert.Equal(t, "sk-abc123", capturedBody.Keys[0])
}

// ─── SanitizeMCPServerName ──────────────────────────────────────────────────

func TestSanitizeMCPServerName(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"jira-agent-alpha", "jira_agent_alpha"},
		{"my-mcp-server", "my_mcp_server"},
		{"already_clean", "already_clean"},
		{"no.changes.needed", "no.changes.needed"},
		{"mixed-hyphens_and.dots", "mixed_hyphens_and.dots"},
		{"", ""},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			assert.Equal(t, tc.expected, SanitizeMCPServerName(tc.input))
		})
	}
}
```

---

## Task 2.3: Run tests

```bash
cd /Users/nickmisasi/workspace/worktrees/mattermost-operator-the-trail

# Run only the new client tests
go test ./controllers/mattermost/agent/... -run TestRegisterModel -v
go test ./controllers/mattermost/agent/... -run TestListMCPServers -v
go test ./controllers/mattermost/agent/... -run TestRegisterMCPServer -v
go test ./controllers/mattermost/agent/... -run TestGenerateKey -v
go test ./controllers/mattermost/agent/... -run TestUpdateKey -v
go test ./controllers/mattermost/agent/... -run TestDeleteKey -v
go test ./controllers/mattermost/agent/... -run TestSanitizeMCPServerName -v

# Run entire package (must not break existing tests)
make unittest
```

Expected: all 12 new tests pass, all existing tests still pass.

---

## Definition of Done

- [ ] `controllers/mattermost/agent/litellm_client.go` compiles with no errors
- [ ] `controllers/mattermost/agent/litellm_client_test.go` compiles with no errors
- [ ] All 12 client tests pass:
  - `TestRegisterModel_Success` — asserts POST body fields
  - `TestRegisterModel_Error` — non-200 returns error with status code
  - `TestListMCPServers_Empty` — empty array decoded correctly
  - `TestListMCPServers_WithEntries` — entries decoded with correct fields
  - `TestRegisterMCPServer_Success` — asserts `credentials.auth_value` nested field in POST body
  - `TestGenerateKey_Success` — asserts `object_permission.mcp_access_groups` in POST body
  - `TestGenerateKey_DuplicateAlias` — 400 + "already exists" returns `errKeyAliasExists`
  - `TestGenerateKey_OtherError` — non-400 error returns generic error (not errKeyAliasExists)
  - `TestUpdateKey_Success` — asserts POST to `/key/update` with correct body
  - `TestDeleteKey_Success` — asserts DELETE to `/key/delete` with keys array
  - `TestSanitizeMCPServerName` — 6 table-driven cases
- [ ] `make unittest` passes (no regression in existing tests)
- [ ] Client functions use exact API field names validated by spikes (not guessed)

---

## Precise Change Map

| File | Action | Summary |
|------|--------|---------|
| `controllers/mattermost/agent/litellm_client.go` | Create | `liteLLMClient` struct, 7 request/response types, 8 methods (`registerModel`, `listMCPServers`, `registerMCPServer`, `updateMCPServer`, `deleteMCPServer`, `generateKey`, `updateKey`, `deleteKey`), `SanitizeMCPServerName`, `errKeyAliasExists` sentinel |
| `controllers/mattermost/agent/litellm_client_test.go` | Create | 12 tests using `httptest.NewServer` — no K8s dependencies |

---

## Key Design Notes

**`do` helper centralizes HTTP mechanics.** All methods call `c.do(method, path, body)` which handles marshal → request → auth header → execute → read body → return `([]byte, int, error)`. Each method then checks the status code and decodes as needed. This avoids duplicating the 10-line HTTP boilerplate from `agent.go`'s `listBots`/`createBot` — those were written before this helper existed.

**`errKeyAliasExists` is a sentinel error, not a string check at call sites.** The reconciler in Phase 3 uses `errors.Is(err, errKeyAliasExists)` to distinguish "already exists, skip" from "real error, fail". The string check `strings.Contains(body, "already exists")` is isolated to `generateKey` only.

**`SanitizeMCPServerName` is exported.** It will be called by the reconciler in Phase 3/4 when constructing server names. Exporting it makes it testable and visible from `litellm.go` (Phase 3).

**No `context.Context` parameter on client methods.** The existing `agent.go` HTTP calls (`listBots`, `createBot`, `createBotToken`) do not use context either — they call `http.DefaultClient.Do(req)` without `http.NewRequestWithContext`. Match that pattern. If context support is needed later, it can be added in a follow-up.
