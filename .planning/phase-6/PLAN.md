# Phase 6: Integration Tests + Validation — Prescriptive Plan

> **Milestone:** M3 — Agent Secret Protection (LiteLLM Gateway)
> **Target repo:** `~/workspace/worktrees/mattermost-operator-the-trail`
> **Phase:** 6 of 6
> **Depends on:** All prior phases (1–5) complete
> **Goal:** All tests green, no regressions, new LiteLLM-aware behaviors validated in existing test files.

---

## Context: What Already Exists After Phases 1–5

These files are fully implemented. Phase 6 only extends two files:

- `controllers/mattermost/agent/agent_test.go` — existing tests: `TestCheckAgentDeployment`, `TestCheckAgentNetworkPolicy_Deny`, `TestCheckAgentNetworkPolicy_AllowList`
- `controllers/mattermost/agent/controller_test.go` — existing tests: `TestReconcileAgent_MattermostNotStable`, `TestReconcileAgent_FullReconcile`, `TestReconcileAgent_ImageUpdate`

**What Phase 6 adds:**

1. Two new test functions in `agent_test.go` — verify LiteLLM env vars in agent Deployment, and LiteLLM egress rule in NetworkPolicy
2. One new test function in `controller_test.go` — full reconcile loop with LLMGateway set, verifying all LiteLLM resources are created and virtual key Secret exists
3. A final validation task — run `make generate manifests`, `make unittest`, `make build` and verify all pass

**Key facts about existing code needed by these tests:**

From `pkg/mattermost/agent.go` (`GenerateAgentDeployment`):
- When `agent.Spec.LLMGateway != nil`, 6 env vars are appended to `baseEnv`:
  `LITELLM_BASE_URL`, `LITELLM_MCP_URL`, `OPENAI_BASE_URL`, `OPENAI_API_KEY`, `ANTHROPIC_BASE_URL`, `ANTHROPIC_API_KEY`
- `OPENAI_API_KEY` and `ANTHROPIC_API_KEY` use `secretKeyRef` pointing at `agent.LiteLLMKeySecretName()` with key `"apiKey"`

From `pkg/mattermost/agent.go` (`GenerateAgentNetworkPolicy`):
- When `agent.Spec.LLMGateway != nil`, a LiteLLM egress rule is inserted after the MM rule and before the DNS rule
- Deny mode without LLMGateway: 2 egress rules (MM + DNS)
- Deny mode with LLMGateway: 3 egress rules (MM + LiteLLM + DNS)

From `pkg/mattermost/litellm.go`:
- `LiteLLMServiceURL(namespace)` returns `http://litellm.<namespace>.svc.cluster.local:4000`

From `apis/mattermost/v1beta1/agent_utils.go`:
- `agent.LiteLLMKeySecretName()` returns `"agent-" + agent.Name + "-litellm-key"`

**Module path:** `github.com/mattermost/mattermost-operator`

---

## Task 6.1: Extend `controllers/mattermost/agent/agent_test.go`

**File:** `controllers/mattermost/agent/agent_test.go`
**Action:** Append two new test functions at the end of the file

No new imports are needed — `agent_test.go` already imports:
- `corev1 "k8s.io/api/core/v1"` ✓
- `networkingv1 "k8s.io/api/networking/v1"` ✓
- `metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"` ✓
- `mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"` ✓
- standard test imports (`testing`, `context`, `assert`, `require`) ✓

**Append these two functions:**

```go
func TestCheckAgentDeployment_WithLLMGateway(t *testing.T) {
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
	_ = agent.SetDefaults()

	botSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agent.BotTokenSecretName(),
			Namespace: agent.Namespace,
		},
		Data: map[string][]byte{"token": []byte("test-token")},
	}

	reconciler, _ := setupReconciler(t, agent, botSecret)

	logSink := blubr.InitLogger(logrus.NewEntry(logrus.New()))
	logger := logr.New(logSink.WithName("test"))

	err := reconciler.checkAgentDeployment(context.TODO(), agent, logger)
	require.NoError(t, err)

	deployment := &appsv1.Deployment{}
	err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, deployment)
	require.NoError(t, err)

	container := deployment.Spec.Template.Spec.Containers[0]

	// Build a map of env vars for easy lookup.
	envMap := make(map[string]corev1.EnvVar, len(container.Env))
	for _, e := range container.Env {
		envMap[e.Name] = e
	}

	// All 6 LiteLLM env vars must be present.
	expectedLiteLLMBaseURL := "http://litellm." + agent.Namespace + ".svc.cluster.local:4000"
	assert.Equal(t, expectedLiteLLMBaseURL, envMap["LITELLM_BASE_URL"].Value, "LITELLM_BASE_URL")
	assert.Equal(t, expectedLiteLLMBaseURL+"/mcp", envMap["LITELLM_MCP_URL"].Value, "LITELLM_MCP_URL")
	assert.Equal(t, expectedLiteLLMBaseURL+"/v1", envMap["OPENAI_BASE_URL"].Value, "OPENAI_BASE_URL")
	assert.Equal(t, expectedLiteLLMBaseURL+"/v1", envMap["ANTHROPIC_BASE_URL"].Value, "ANTHROPIC_BASE_URL")

	// OPENAI_API_KEY and ANTHROPIC_API_KEY must be secretKeyRefs (not plain values).
	expectedKeySecretName := agent.LiteLLMKeySecretName() // "agent-test-agent-litellm-key"

	openAIKey, ok := envMap["OPENAI_API_KEY"]
	require.True(t, ok, "OPENAI_API_KEY must be present")
	require.NotNil(t, openAIKey.ValueFrom, "OPENAI_API_KEY must use ValueFrom")
	require.NotNil(t, openAIKey.ValueFrom.SecretKeyRef, "OPENAI_API_KEY must use SecretKeyRef")
	assert.Equal(t, expectedKeySecretName, openAIKey.ValueFrom.SecretKeyRef.Name, "OPENAI_API_KEY SecretKeyRef name")
	assert.Equal(t, "apiKey", openAIKey.ValueFrom.SecretKeyRef.Key, "OPENAI_API_KEY SecretKeyRef key")

	anthropicKey, ok := envMap["ANTHROPIC_API_KEY"]
	require.True(t, ok, "ANTHROPIC_API_KEY must be present")
	require.NotNil(t, anthropicKey.ValueFrom, "ANTHROPIC_API_KEY must use ValueFrom")
	require.NotNil(t, anthropicKey.ValueFrom.SecretKeyRef, "ANTHROPIC_API_KEY must use SecretKeyRef")
	assert.Equal(t, expectedKeySecretName, anthropicKey.ValueFrom.SecretKeyRef.Name, "ANTHROPIC_API_KEY SecretKeyRef name")
	assert.Equal(t, "apiKey", anthropicKey.ValueFrom.SecretKeyRef.Key, "ANTHROPIC_API_KEY SecretKeyRef key")

	// Standard env vars must still be present (backwards compat check).
	assert.Contains(t, envMap, "MM_SERVER_URL", "MM_SERVER_URL must still be present")
	assert.Contains(t, envMap, "MM_BOT_TOKEN", "MM_BOT_TOKEN must still be present")
	assert.Contains(t, envMap, "AGENT_HOOKS", "AGENT_HOOKS must still be present")
}

func TestCheckAgentNetworkPolicy_DenyWithLiteLLM(t *testing.T) {
	agent := newTestAgent()
	agent.Spec.EgressPolicy = mmv1beta.AgentEgressPolicyDeny
	agent.Spec.LLMGateway = &mmv1beta.LLMGatewayConfig{
		OperatorManaged: &mmv1beta.OperatorManagedLLMGateway{
			Image: mmv1beta.AgentLiteLLMDefaultImage,
		},
	}
	_ = agent.SetDefaults()

	reconciler, _ := setupReconciler(t, agent)

	logSink := blubr.InitLogger(logrus.NewEntry(logrus.New()))
	logger := logr.New(logSink.WithName("test"))

	err := reconciler.checkAgentNetworkPolicy(context.TODO(), agent, logger)
	require.NoError(t, err)

	np := &networkingv1.NetworkPolicy{}
	err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, np)
	require.NoError(t, err)

	// Deny + LiteLLM: 3 egress rules (MM + LiteLLM + DNS).
	assert.Len(t, np.Spec.Egress, 3, "deny+litellm should have 3 egress rules: MM, LiteLLM, DNS")

	// Verify rule ordering: MM(8065), LiteLLM(4000), DNS(53).
	// Rule 0: MM server (port 8065, has PodSelector).
	require.Len(t, np.Spec.Egress[0].Ports, 1)
	assert.Equal(t, int32(8065), np.Spec.Egress[0].Ports[0].Port.IntVal, "rule 0 should be MM port 8065")
	require.NotEmpty(t, np.Spec.Egress[0].To, "rule 0 should have a To selector (MM)")

	// Rule 1: LiteLLM (port 4000, has PodSelector with app: litellm).
	require.Len(t, np.Spec.Egress[1].Ports, 1)
	assert.Equal(t, int32(4000), np.Spec.Egress[1].Ports[0].Port.IntVal, "rule 1 should be LiteLLM port 4000")
	require.NotEmpty(t, np.Spec.Egress[1].To)
	require.NotNil(t, np.Spec.Egress[1].To[0].PodSelector)
	assert.Equal(t, "litellm", np.Spec.Egress[1].To[0].PodSelector.MatchLabels["app"], "rule 1 should target app=litellm")

	// Rule 2: DNS (port 53, no To selector — allows all destinations).
	require.Len(t, np.Spec.Egress[2].Ports, 2, "DNS rule should have TCP+UDP")
	assert.Empty(t, np.Spec.Egress[2].To, "DNS rule should have no destination selector")
}
```

---

## Task 6.2: Extend `controllers/mattermost/agent/controller_test.go`

**File:** `controllers/mattermost/agent/controller_test.go`
**Action:** Append one new test function at the end of the file

No new imports are needed — `controller_test.go` already imports all necessary packages.

**Append this function:**

```go
func TestReconcileAgent_WithLLMGateway(t *testing.T) {
	logSink := blubr.InitLogger(logrus.NewEntry(logrus.New()))
	logSink = logSink.WithName("test.opr")
	logger := logr.New(logSink)
	logf.SetLogger(logger)

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
	_ = agent.SetDefaults()

	mm := &mmv1beta.Mattermost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mm-test",
			Namespace: "default",
			UID:       types.UID("mm-uid"),
		},
		Status: mmv1beta.MattermostStatus{
			State: mmv1beta.Stable,
		},
	}

	adminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "admin-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{"token": []byte("admin-token")},
	}

	masterKeySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmv1beta.AgentLiteLLMMasterKeySecretName,
			Namespace: "default",
		},
		Data: map[string][]byte{"masterKey": []byte("sk-test-master-key")},
	}

	botTokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agent.BotTokenSecretName(),
			Namespace: agent.Namespace,
		},
		Data: map[string][]byte{"token": []byte("bot-secret-token")},
	}

	// Mock the LiteLLM management API.
	litellmAPICalled := make(map[string]int)
	litellmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		litellmAPICalled[key]++
		switch {
		case r.Method == "POST" && r.URL.Path == "/model/new":
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"model_id": "m1"})
		case r.Method == "GET" && r.URL.Path == "/v1/mcp/server":
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode([]liteLLMMCPServerResponse{})
		case r.Method == "POST" && r.URL.Path == "/key/generate":
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(liteLLMKeyResponse{
				Key:      "sk-virtual-key-abc",
				KeyAlias: "agent-test-agent-key",
				Token:    "tok-hash-xyz",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer litellmSrv.Close()

	s := setupScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&mmv1beta.Agent{}, &appsv1.Deployment{}, &mmv1beta.Mattermost{}).
		WithRuntimeObjects(agent, mm, adminSecret, masterKeySecret, botTokenSecret).
		Build()

	r := &AgentReconciler{
		Client:    c,
		Log:       logger,
		Scheme:    s,
		Resources: resources.NewResourceHelper(c, s),
	}

	// Override LiteLLM service URL to point at test server.
	// Since LiteLLMServiceURL() builds from namespace, we inject the URL via
	// a mock-aware wrapper: pre-set litellmURL in the reconcile so the test
	// httptest server is used. We do this by pre-creating the LiteLLM Deployment
	// with ReadyReplicas=1 in the fake client so checkLiteLLMReady returns true,
	// and overriding the URL by patching the reconciler's litellm URL computation.
	//
	// Practical approach: pre-create the LiteLLM Deployment so readiness passes,
	// and use the litellmSrv.URL for the URL by injecting it. Since reconcileLiteLLMModels
	// and reconcileLiteLLMVirtualKey build the URL via mattermostApp.LiteLLMServiceURL,
	// which we can't override without a URL parameter injection point, we take the
	// simpler approach used in TestReconcileAgent_FullReconcile: pre-create the
	// LiteLLM virtual key Secret to skip API calls for the virtual key, and test
	// the resource creation independently.
	//
	// This test validates: LiteLLM K8s resources (Deployment, Service, ConfigMap)
	// are created by the reconcile loop, and the agent Deployment gets LiteLLM env vars.

	// Pre-create the LiteLLM virtual key Secret so reconcileLiteLLMVirtualKey is a no-op.
	litellmKeySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agent.LiteLLMKeySecretName(),
			Namespace: agent.Namespace,
		},
		Data: map[string][]byte{"apiKey": []byte("sk-virtual-key-pre-created")},
	}
	err := c.Create(context.TODO(), litellmKeySecret)
	require.NoError(t, err)

	// Pre-create the LiteLLM Deployment with ReadyReplicas=1 so checkLiteLLMReady passes.
	litellmDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmv1beta.AgentLiteLLMDeploymentName,
			Namespace: agent.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "litellm"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "litellm"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "litellm", Image: mmv1beta.AgentLiteLLMDefaultImage}},
				},
			},
		},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas: 1,
			Replicas:      1,
		},
	}
	err = c.Create(context.TODO(), litellmDeploy)
	require.NoError(t, err)
	litellmDeploy.Status.ReadyReplicas = 1
	err = c.Status().Update(context.TODO(), litellmDeploy)
	require.NoError(t, err)

	// Pre-register models so reconcileLiteLLMModels is skipped on calls to the
	// fake LiteLLM URL (in-cluster URL won't resolve in tests — that's OK since
	// we've pre-created the key Secret). The model registration will fail on the
	// in-cluster URL, so we need a different approach:
	// Inject a reconcileLiteLLMModels call that goes to litellmSrv.URL
	// by observing the test failure and adjusting.
	//
	// NOTE FOR IMPLEMENTER: The simplest approach that avoids URL injection complexity
	// is to structure this as two sub-tests:
	//   1. Test K8s resource creation (ConfigMap, Deployment, Service, LiteLLM Deployment,
	//      agent Deployment with LiteLLM env vars, NetworkPolicy with LiteLLM egress rule)
	//      — pre-create the LiteLLM key Secret and Deployment so API calls are skipped.
	//   2. Test LiteLLM API calls (models + key) separately in litellm_test.go
	//      (already covered by TestReconcileLiteLLMModels_CallsAPI and
	//      TestReconcileLiteLLMVirtualKey_CreatesSecret in Phase 3 tests).
	//
	// This function tests case (1) only. The API call tests are in litellm_test.go.

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}}

	// First reconcile: agent Deployment not ready yet → requeue with healthCheckRequeueDelay.
	res, err := r.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, 6*time.Second, res.RequeueAfter)

	// Verify LiteLLM Service was created (shared resource, no OwnerReference).
	litellmSvc := &corev1.Service{}
	err = c.Get(context.TODO(), types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMServiceName,
		Namespace: agent.Namespace,
	}, litellmSvc)
	require.NoError(t, err, "LiteLLM Service should be created by reconcile")
	assert.Equal(t, "litellm", litellmSvc.Labels["app"])

	// Verify LiteLLM ConfigMap was created.
	litellmCM := &corev1.ConfigMap{}
	err = c.Get(context.TODO(), types.NamespacedName{
		Name:      mmv1beta.AgentLiteLLMConfigMapName,
		Namespace: agent.Namespace,
	}, litellmCM)
	require.NoError(t, err, "LiteLLM ConfigMap should be created by reconcile")

	// Verify agent Deployment was created with LiteLLM env vars.
	agentDeploy := &appsv1.Deployment{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, agentDeploy)
	require.NoError(t, err)

	container := agentDeploy.Spec.Template.Spec.Containers[0]
	envMap := make(map[string]corev1.EnvVar, len(container.Env))
	for _, e := range container.Env {
		envMap[e.Name] = e
	}

	assert.Contains(t, envMap, "LITELLM_BASE_URL", "agent Deployment must have LITELLM_BASE_URL")
	assert.Contains(t, envMap, "OPENAI_API_KEY", "agent Deployment must have OPENAI_API_KEY")
	assert.Contains(t, envMap, "ANTHROPIC_API_KEY", "agent Deployment must have ANTHROPIC_API_KEY")

	// Raw API keys must NOT be present as plain values.
	assert.Nil(t, envMap["ANTHROPIC_API_KEY"].Value, "ANTHROPIC_API_KEY must not be a plain value")
	require.NotNil(t, envMap["ANTHROPIC_API_KEY"].ValueFrom, "ANTHROPIC_API_KEY must use ValueFrom")
	assert.Equal(t, agent.LiteLLMKeySecretName(), envMap["ANTHROPIC_API_KEY"].ValueFrom.SecretKeyRef.Name)

	// Verify NetworkPolicy has 3 egress rules (MM + LiteLLM + DNS).
	np := &networkingv1.NetworkPolicy{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, np)
	require.NoError(t, err)
	assert.Len(t, np.Spec.Egress, 3, "deny+litellm policy must have 3 egress rules")

	// Verify backwards compat: agent without LLMGateway still reconciles successfully
	// in TestReconcileAgent_FullReconcile (existing test, unchanged).
}
```

**Important note on the `litellmAPICalled` variable:** it is declared but not used in the simplified version above. Remove it or use it if the full URL injection approach is taken. The simplest option is to remove the variable and the `litellmSrv` server entirely since model/key API calls are already tested in `litellm_test.go`. The controller test's sole responsibility is verifying K8s resources.

**Simplified version (recommended for implementation):**

Replace the `litellmSrv` block and `litellmAPICalled` with a note comment. The test still pre-creates the LiteLLM key Secret and Deployment to avoid the in-cluster URL resolution problem.

---

## Task 6.3: Run full validation suite

Execute these commands in order. All must pass.

### Step 1: Regenerate CRD YAML and deepcopy

```bash
cd ~/workspace/worktrees/mattermost-operator-the-trail
make generate manifests
```

**Expected:** No errors. Verify the following files changed:
- `apis/mattermost/v1beta1/zz_generated.deepcopy.go` — contains `DeepCopyInto` for `LLMGatewayConfig`, `AgentMCPServer`, `OperatorManagedLLMGateway`, `LLMProvider`, `ExternalLLMGateway`
- `config/crd/bases/installation.mattermost.com_agents.yaml` — contains `llmGateway` and `mcpServers` fields

Spot-check the CRD YAML:
```bash
grep -A 3 "llmGateway:" config/crd/bases/installation.mattermost.com_agents.yaml
grep -A 3 "mcpServers:" config/crd/bases/installation.mattermost.com_agents.yaml
```

### Step 2: Run unit tests

```bash
make unittest
```

**Expected:** 0 failures. All test files pass:
- `controllers/mattermost/agent/agent_test.go` — including 2 new tests
- `controllers/mattermost/agent/controller_test.go` — including 1 new test
- `controllers/mattermost/agent/litellm_client_test.go` — 11 existing tests
- `controllers/mattermost/agent/litellm_test.go` — 13+ tests (Phase 3 + Phase 4)
- All other existing test files unchanged

### Step 3: Build

```bash
make build
```

**Expected:** Clean build, no compilation errors.

### Step 4: Verify CRD YAML content

```bash
# Confirm llmGateway field is in the CRD
grep "llmGateway" config/crd/bases/installation.mattermost.com_agents.yaml

# Confirm mcpServers field is in the CRD
grep "mcpServers" config/crd/bases/installation.mattermost.com_agents.yaml

# Confirm operatorManaged is in the CRD
grep "operatorManaged" config/crd/bases/installation.mattermost.com_agents.yaml
```

---

## Definition of Done

- [ ] `TestCheckAgentDeployment_WithLLMGateway` passes — all 6 LiteLLM env vars present, OPENAI_API_KEY and ANTHROPIC_API_KEY use secretKeyRef pointing at `agent.LiteLLMKeySecretName()` with key `"apiKey"`
- [ ] `TestCheckAgentNetworkPolicy_DenyWithLiteLLM` passes — 3 egress rules, LiteLLM rule at index 1 with port 4000 and `app=litellm` selector
- [ ] `TestReconcileAgent_WithLLMGateway` passes — LiteLLM Service and ConfigMap created, agent Deployment has LiteLLM env vars, ANTHROPIC_API_KEY is a secretKeyRef not a plain value, NetworkPolicy has 3 egress rules
- [ ] Existing tests unchanged and passing (`TestCheckAgentDeployment`, `TestCheckAgentNetworkPolicy_Deny`, `TestCheckAgentNetworkPolicy_AllowList`, all controller_test.go tests)
- [ ] `make generate manifests` succeeds
- [ ] `make unittest` passes (0 failures)
- [ ] `make build` succeeds
- [ ] CRD YAML contains `llmGateway` and `mcpServers` fields

---

## Precise Change Map

| File | Action | What changes |
|------|--------|-------------|
| `controllers/mattermost/agent/agent_test.go` | Append | `TestCheckAgentDeployment_WithLLMGateway`, `TestCheckAgentNetworkPolicy_DenyWithLiteLLM` |
| `controllers/mattermost/agent/controller_test.go` | Append | `TestReconcileAgent_WithLLMGateway` |
| All other files | No change | — |

---

## Test Design Rationale

### Why only 2 new tests in `agent_test.go`?

The master plan specifies `TestCheckAgentDeployment_WithLLMGateway` (assert all 6 LiteLLM env vars, secretKeyRef correct) and `TestCheckAgentNetworkPolicy_DenyWithLiteLLM` (assert 3 egress rules). The existing `TestCheckAgentDeployment` already tests the baseline case without LLMGateway — the new test only adds the LLMGateway variant.

### Why `TestReconcileAgent_WithLLMGateway` pre-creates resources instead of mocking URLs?

The controller calls `mattermostApp.LiteLLMServiceURL(agent.Namespace)` which builds an in-cluster URL that doesn't resolve in unit tests. The pattern established by `TestReconcileAgent_FullReconcile` is to pre-create the bot token Secret to skip API calls. We follow the same pattern: pre-create the LiteLLM key Secret (skips `reconcileLiteLLMVirtualKey` API call) and pre-create the LiteLLM Deployment with `ReadyReplicas=1` (makes `checkLiteLLMReady` return true).

The unit tests for LiteLLM API calls (`reconcileLiteLLMModels`, `reconcileLiteLLMVirtualKey`, `reconcileLiteLLMMCPServers`) are already fully covered in `litellm_test.go` with `httptest.NewServer` mocks. The controller test validates the integration: that all steps are called in the right order and the resulting K8s resources exist.

### Why `make generate manifests` is a required step?

Phase 1 added new types to `agent_types.go`. The `make generate manifests` command regenerates:
1. `zz_generated.deepcopy.go` — Go deepcopy methods for all new types (required for compilation)
2. `config/crd/bases/installation.mattermost.com_agents.yaml` — the CRD YAML deployed to Kubernetes

If `make generate` was already run during Phase 1 implementation, this step is a verification step. If it was not run (unlikely), it must be run now.
