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
