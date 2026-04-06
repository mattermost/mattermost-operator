package agent

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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func newTestAgentWithLLMGateway() *mmv1beta.Agent {
	agent := newTestAgent()
	agent.Spec.LLMGateway = &mmv1beta.LLMGatewayConfig{
		OperatorManaged: &mmv1beta.OperatorManagedLLMGateway{
			Image: "ghcr.io/berriai/litellm:main-latest",
			LLMProviders: []mmv1beta.LLMProvider{
				{
					Name:   "anthropic",
					Secret: "anthropic-api-key",
					Models: []string{"claude-3-5-sonnet-20241022"},
				},
			},
		},
	}
	return agent
}

func testLogger() logr.Logger {
	logSink := blubr.InitLogger(logrus.NewEntry(logrus.New()))
	return logr.New(logSink.WithName("test"))
}

// TestCheckLiteLLMDeployment_CreatesResources verifies that ConfigMap and Deployment
// are created when they do not exist.
func TestCheckLiteLLMDeployment_CreatesResources(t *testing.T) {
	agent := newTestAgentWithLLMGateway()
	_ = agent.SetDefaults()

	reconciler, _ := setupReconciler(t, agent)
	logger := testLogger()

	err := reconciler.checkLiteLLMDeployment(context.TODO(), agent, logger)
	require.NoError(t, err)

	// ConfigMap should exist.
	cm := &corev1.ConfigMap{}
	err = reconciler.Client.Get(context.TODO(), types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMConfigMapName,
		Namespace: agent.Namespace,
	}, cm)
	require.NoError(t, err)
	assert.Contains(t, cm.Data["config.yaml"], "store_model_in_db")

	// Deployment should exist.
	deploy := &appsv1.Deployment{}
	err = reconciler.Client.Get(context.TODO(), types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMDeploymentName,
		Namespace: agent.Namespace,
	}, deploy)
	require.NoError(t, err)
	assert.Equal(t, agent.Spec.LLMGateway.OperatorManaged.Image, deploy.Spec.Template.Spec.Containers[0].Image)
}

// TestCheckLiteLLMDeployment_Idempotent verifies that a second call does not error.
func TestCheckLiteLLMDeployment_Idempotent(t *testing.T) {
	agent := newTestAgentWithLLMGateway()
	_ = agent.SetDefaults()

	reconciler, _ := setupReconciler(t, agent)
	logger := testLogger()

	require.NoError(t, reconciler.checkLiteLLMDeployment(context.TODO(), agent, logger))
	require.NoError(t, reconciler.checkLiteLLMDeployment(context.TODO(), agent, logger))
}

// TestCheckLiteLLMService_Creates verifies that the Service is created.
func TestCheckLiteLLMService_Creates(t *testing.T) {
	agent := newTestAgentWithLLMGateway()
	_ = agent.SetDefaults()

	reconciler, _ := setupReconciler(t, agent)
	logger := testLogger()

	err := reconciler.checkLiteLLMService(context.TODO(), agent, logger)
	require.NoError(t, err)

	svc := &corev1.Service{}
	err = reconciler.Client.Get(context.TODO(), types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMServiceName,
		Namespace: agent.Namespace,
	}, svc)
	require.NoError(t, err)
	assert.Equal(t, int32(mmv1beta.AgentLiteLLMPort), svc.Spec.Ports[0].Port)
}

// TestCheckLiteLLMReady_NotReady returns false (not an error) when ReadyReplicas < 1.
func TestCheckLiteLLMReady_NotReady(t *testing.T) {
	agent := newTestAgentWithLLMGateway()

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmv1beta.AgentLiteLLMDeploymentName,
			Namespace: agent.Namespace,
		},
	}
	// ReadyReplicas defaults to 0.

	reconciler, _ := setupReconciler(t, agent, deploy)
	logger := testLogger()

	ready, err := reconciler.checkLiteLLMReady(context.TODO(), agent, logger)
	require.NoError(t, err)
	assert.False(t, ready)
}

// TestCheckLiteLLMReady_Ready returns true when ReadyReplicas >= 1.
func TestCheckLiteLLMReady_Ready(t *testing.T) {
	agent := newTestAgentWithLLMGateway()

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmv1beta.AgentLiteLLMDeploymentName,
			Namespace: agent.Namespace,
		},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas: 1,
		},
	}

	reconciler, _ := setupReconciler(t, agent, deploy)
	logger := testLogger()

	ready, err := reconciler.checkLiteLLMReady(context.TODO(), agent, logger)
	require.NoError(t, err)
	assert.True(t, ready)
}

// TestReconcileLiteLLMModels_CallsAPI verifies POST /model/new is called for each model
// that does not already exist.
func TestReconcileLiteLLMModels_CallsAPI(t *testing.T) {
	agent := newTestAgentWithLLMGateway()

	var modelsCalled []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/model/info":
			// Return empty list — no models registered yet.
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(liteLLMModelInfoResponse{Data: []liteLLMModelInfo{}})
		case r.Method == "POST" && r.URL.Path == "/model/new":
			var req liteLLMModelRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			modelsCalled = append(modelsCalled, req.LiteLLMParams.Model)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"model_id": "id1"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	reconciler, _ := setupReconciler(t, agent)
	logger := testLogger()

	err := reconciler.reconcileLiteLLMModels(context.TODO(), agent, srv.URL, "master-key", logger)
	require.NoError(t, err)
	assert.Contains(t, modelsCalled, "anthropic/claude-3-5-sonnet-20241022")
}

// TestReconcileLiteLLMModels_SkipsExisting verifies that models already registered
// in LiteLLM are not re-registered, preventing duplicates.
func TestReconcileLiteLLMModels_SkipsExisting(t *testing.T) {
	agent := newTestAgentWithLLMGateway()

	postCallCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/model/info":
			// Return both the prefixed and bare model names as already existing.
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(liteLLMModelInfoResponse{
				Data: []liteLLMModelInfo{
					{ModelName: "anthropic/claude-3-5-sonnet-20241022"},
					{ModelName: "claude-3-5-sonnet-20241022"},
				},
			})
		case r.Method == "POST" && r.URL.Path == "/model/new":
			postCallCount++
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"model_id": "id1"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	reconciler, _ := setupReconciler(t, agent)
	logger := testLogger()

	err := reconciler.reconcileLiteLLMModels(context.TODO(), agent, srv.URL, "master-key", logger)
	require.NoError(t, err)
	assert.Equal(t, 0, postCallCount, "expected no POST calls when all models already exist")
}

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
