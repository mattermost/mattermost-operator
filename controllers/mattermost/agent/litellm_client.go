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

// liteLLMModelInfo represents a single entry returned by GET /model/info.
type liteLLMModelInfo struct {
	ModelName string `json:"model_name"`
}

// liteLLMModelInfoResponse is the envelope returned by GET /model/info.
type liteLLMModelInfoResponse struct {
	Data []liteLLMModelInfo `json:"data"`
}

// listModels returns all registered model names via GET /model/info.
func (c *liteLLMClient) listModels() ([]liteLLMModelInfo, error) {
	body, status, err := c.do("GET", "/model/info", nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("list models returned status %d: %s", status, string(body))
	}

	var resp liteLLMModelInfoResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to decode model info response: %w", err)
	}
	return resp.Data, nil
}

// registerModel registers an LLM model via POST /model/new.
// NOT idempotent — callers must check listModels first.
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
