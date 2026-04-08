# Phase 3: Configurable Egress Policy — `allow` Mode — Prescriptive Plan

> **Milestone:** M7 — PVC, Init Container Removal, Allow Egress
> **Target repo:** `~/workspace/worktrees/mattermost-operator-the-trail`
> **Phase:** 3 (Allow Egress)
> **Depends on:** nothing (can be done in parallel with Phase 2)
> **Goal:** Add `allow` as a third egress policy mode that permits all outbound traffic from the agent pod, while preserving ingress restrictions.

---

## Context: What Already Exists

### Egress Policy Constants
- `apis/mattermost/v1beta1/agent_utils.go` lines 12-13:
  ```go
  AgentEgressPolicyDeny      = "deny"
  AgentEgressPolicyAllowList = "allowList"
  ```

### NetworkPolicy Generation
- `pkg/mattermost/agent.go` line 267: `GenerateAgentNetworkPolicy(agent)` builds a `NetworkPolicy` with:
  - **Ingress** (lines 234-264 via `agentIngressRules()`): Always allows MM pods on port 8080. Also allows LiteLLM pods when LLMGateway is configured.
  - **Egress** (lines 275-357):
    - **Base rules** (lines 275-307): MM pods on port 8065 + DNS (UDP+TCP 53). Always present.
    - **LiteLLM rule** (lines 311-329): LiteLLM pods on port 4000. Added when `agent.Spec.LLMGateway != nil`.
    - **AllowList rules** (lines 334-357): HTTPS (443) + HTTP (80) to any destination. Added when `egressPolicy == "allowList"`.
  - **PolicyTypes** (lines 370-373): Both `Ingress` and `Egress` are always declared.

### Existing Tests
- `pkg/mattermost/agent_test.go`:
  - `TestGenerateAgentNetworkPolicy_Deny` (line 213): Asserts 2 egress rules (MM + DNS).
  - `TestGenerateAgentNetworkPolicy_AllowList` (line 250): Asserts 4 egress rules (MM + DNS + HTTPS + HTTP).
- `controllers/mattermost/agent/agent_test.go`:
  - `TestCheckAgentNetworkPolicy_Deny` (line 154): Asserts 2 egress rules.
  - `TestCheckAgentNetworkPolicy_AllowList` (line 175): Asserts 4 egress rules.
  - `TestCheckAgentNetworkPolicy_DenyWithLiteLLM` (line 316): Asserts 3 egress rules (MM + LiteLLM + DNS), ingress has 2 `From` peers.

### K8s NetworkPolicy Semantics for "Allow All Egress"
A `NetworkPolicyEgressRule` with no `To` and no `Ports` fields (empty struct `{}`) means "allow all egress traffic." This is the standard K8s pattern:
```yaml
egress:
  - {}   # empty rule = allow all
```

In Go: `networkingv1.NetworkPolicyEgressRule{}` — an empty struct literal.

---

## Task 3.1: Add `allow` Constant to agent_utils.go

**File:** `apis/mattermost/v1beta1/agent_utils.go`
**Lines:** 12-13

### Change

Add the new constant after the existing egress policy constants:

```go
// OLD (lines 12-13):
	AgentEgressPolicyDeny             = "deny"
	AgentEgressPolicyAllowList        = "allowList"

// NEW:
	AgentEgressPolicyDeny             = "deny"
	AgentEgressPolicyAllowList        = "allowList"
	AgentEgressPolicyAllow            = "allow"
```

---

## Task 3.2: Update GenerateAgentNetworkPolicy for `allow` Mode

**File:** `pkg/mattermost/agent.go`
**Lines:** 334-357 (after the allowList block)

### Change

Insert an `allow` mode check **after** the `allowList` block and **before** the `return` statement (line 359). When `allow` is set, replace the entire egress rules list with a single empty rule that permits all traffic.

```go
// OLD (lines 334-377):
	// If egressPolicy is allowList, add specific egress rules for HTTPS, HTTP,
	// and other required outbound traffic. ...
	if agent.Spec.EgressPolicy == mmv1beta.AgentEgressPolicyAllowList {
		// ... existing allowList logic ...
	}

	return &networkingv1.NetworkPolicy{
		// ...
		Spec: networkingv1.NetworkPolicySpec{
			// ...
			Egress: egressRules,
		},
	}

// NEW:
	// If egressPolicy is allowList, add specific egress rules for HTTPS, HTTP,
	// and other required outbound traffic. ...
	if agent.Spec.EgressPolicy == mmv1beta.AgentEgressPolicyAllowList {
		// ... existing allowList logic (unchanged) ...
	}

	// If egressPolicy is allow, permit all outbound traffic.
	// This replaces all egress rules (including MM, DNS, LiteLLM-specific rules)
	// with a single empty rule that allows everything.
	if agent.Spec.EgressPolicy == mmv1beta.AgentEgressPolicyAllow {
		egressRules = []networkingv1.NetworkPolicyEgressRule{{}}
	}

	return &networkingv1.NetworkPolicy{
		// ... unchanged ...
		Spec: networkingv1.NetworkPolicySpec{
			// ... unchanged ...
			Egress: egressRules,
		},
	}
```

### Exact insertion point

Insert these 5 lines immediately before the `return &networkingv1.NetworkPolicy{` statement:

```go
	// If egressPolicy is allow, permit all outbound traffic.
	if agent.Spec.EgressPolicy == mmv1beta.AgentEgressPolicyAllow {
		egressRules = []networkingv1.NetworkPolicyEgressRule{{}}
	}
```

### Design decisions

- **Replaces ALL egress rules.** With `allow`, the agent can reach anything — MM, DNS, LiteLLM, and external services are all covered by the blanket allow. Individual rules are redundant.
- **Ingress is preserved.** The `allow` mode only affects egress. Ingress rules (MM + optionally LiteLLM) are unchanged — the agent pod still only accepts inbound traffic from authorized sources.
- **Placement after allowList.** The `allow` check runs last, so it cleanly overrides any rules that were built up by the earlier logic (MM, DNS, LiteLLM, allowList ports). This is simpler than adding early-return branching.

---

## Task 3.3: Unit Tests for `allow` Mode

### 3.3a: Test GenerateAgentNetworkPolicy with allow

**File:** `pkg/mattermost/agent_test.go`

Add after the existing `TestGenerateAgentNetworkPolicy_AllowList` test (after line 286):

```go
func TestGenerateAgentNetworkPolicy_Allow(t *testing.T) {
	agent := testAgent("my-agent", "default")
	agent.Spec.EgressPolicy = mmv1beta.AgentEgressPolicyAllow

	np := GenerateAgentNetworkPolicy(agent)

	assert.Equal(t, "my-agent", np.Name)
	assert.Equal(t, "default", np.Namespace)
	assert.Len(t, np.OwnerReferences, 1)

	// Policy types include both Ingress and Egress.
	assert.Contains(t, np.Spec.PolicyTypes, networkingv1.PolicyTypeIngress)
	assert.Contains(t, np.Spec.PolicyTypes, networkingv1.PolicyTypeEgress)

	// Ingress: from MM pods on port 8080 (unchanged from deny mode).
	assert.Len(t, np.Spec.Ingress, 1)
	ingress := np.Spec.Ingress[0]
	assert.Len(t, ingress.From, 1)
	assert.Equal(t, "mm-prod", ingress.From[0].PodSelector.MatchLabels[mmv1beta.ClusterLabel])

	// Egress: 1 rule — empty (allow all).
	require.Len(t, np.Spec.Egress, 1)
	assert.Empty(t, np.Spec.Egress[0].To, "allow-all rule has no To selector")
	assert.Empty(t, np.Spec.Egress[0].Ports, "allow-all rule has no Ports restriction")
}

func TestGenerateAgentNetworkPolicy_AllowWithLiteLLM(t *testing.T) {
	agent := testAgent("my-agent", "default")
	agent.Spec.EgressPolicy = mmv1beta.AgentEgressPolicyAllow
	agent.Spec.LLMGateway = &mmv1beta.LLMGatewayConfig{
		OperatorManaged: &mmv1beta.OperatorManagedLLMGateway{
			Image: mmv1beta.AgentLiteLLMDefaultImage,
		},
	}

	np := GenerateAgentNetworkPolicy(agent)

	// Egress: still 1 rule — allow-all overrides LiteLLM-specific rules.
	require.Len(t, np.Spec.Egress, 1)
	assert.Empty(t, np.Spec.Egress[0].To, "allow-all rule has no To selector")
	assert.Empty(t, np.Spec.Egress[0].Ports, "allow-all rule has no Ports restriction")

	// Ingress: should have BOTH MM and LiteLLM peers (allow mode doesn't affect ingress).
	require.Len(t, np.Spec.Ingress, 1)
	assert.Len(t, np.Spec.Ingress[0].From, 2, "ingress should allow both MM and LiteLLM pods")
	assert.Equal(t, "mm-prod", np.Spec.Ingress[0].From[0].PodSelector.MatchLabels[mmv1beta.ClusterLabel])
	assert.Equal(t, "litellm", np.Spec.Ingress[0].From[1].PodSelector.MatchLabels["app"])
}
```

### 3.3b: Test reconciler creates correct NetworkPolicy for allow mode

**File:** `controllers/mattermost/agent/agent_test.go`

Add after the existing `TestCheckAgentNetworkPolicy_AllowList` test (after line 194):

```go
func TestCheckAgentNetworkPolicy_Allow(t *testing.T) {
	agent := newTestAgent()
	agent.Spec.EgressPolicy = mmv1beta.AgentEgressPolicyAllow
	_ = agent.SetDefaults()

	reconciler, _ := setupReconciler(t, agent)

	logSink := blubr.InitLogger(logrus.NewEntry(logrus.New()))
	logger := logr.New(logSink.WithName("test"))

	err := reconciler.checkAgentNetworkPolicy(context.TODO(), agent, logger)
	require.NoError(t, err)

	np := &networkingv1.NetworkPolicy{}
	err = reconciler.Client.Get(context.TODO(), types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, np)
	require.NoError(t, err)

	// Allow policy: 1 egress rule (empty = allow all).
	require.Len(t, np.Spec.Egress, 1)
	assert.Empty(t, np.Spec.Egress[0].To, "allow-all rule has no To selector")
	assert.Empty(t, np.Spec.Egress[0].Ports, "allow-all rule has no Ports restriction")

	// Ingress still restricts to MM pods.
	require.Len(t, np.Spec.Ingress, 1)
	assert.Len(t, np.Spec.Ingress[0].From, 1)
}

func TestCheckAgentNetworkPolicy_AllowWithLiteLLM(t *testing.T) {
	agent := newTestAgent()
	agent.Spec.EgressPolicy = mmv1beta.AgentEgressPolicyAllow
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

	// Egress: 1 rule (allow all), even with LiteLLM configured.
	require.Len(t, np.Spec.Egress, 1)
	assert.Empty(t, np.Spec.Egress[0].To)
	assert.Empty(t, np.Spec.Egress[0].Ports)

	// Ingress: 2 peers (MM + LiteLLM) — allow mode does not affect ingress.
	require.Len(t, np.Spec.Ingress, 1)
	assert.Len(t, np.Spec.Ingress[0].From, 2, "ingress should allow both MM and LiteLLM pods")
}
```

---

## File Change Summary

| File | Task | Action | Summary |
|------|------|--------|---------|
| `apis/mattermost/v1beta1/agent_utils.go` | 3.1 | Modify | Add `AgentEgressPolicyAllow = "allow"` constant |
| `pkg/mattermost/agent.go` | 3.2 | Modify | Add `allow` egress mode check before return in `GenerateAgentNetworkPolicy` |
| `pkg/mattermost/agent_test.go` | 3.3a | Modify | Add `TestGenerateAgentNetworkPolicy_Allow`, `_AllowWithLiteLLM` |
| `controllers/mattermost/agent/agent_test.go` | 3.3b | Modify | Add `TestCheckAgentNetworkPolicy_Allow`, `_AllowWithLiteLLM` |

---

## Build & Verify

```bash
# Run NetworkPolicy-specific tests
go test ./pkg/mattermost/... -v -count=1 -run TestGenerateAgentNetworkPolicy
go test ./controllers/mattermost/agent/... -v -count=1 -run TestCheckAgentNetworkPolicy

# Run full test suite to verify no regressions
go test ./... -count=1

# Verify build
go build ./...
```

---

## Edge Cases & Gotchas

1. **`allow` + `egressAllowList` combination.** If a user sets `egressPolicy: allow` AND provides `egressAllowList` entries, the allowList is ignored because the `allow` check runs last and replaces all egress rules. This is intentional — `allow` is a superset of `allowList`. No validation error is needed; the allowList is simply irrelevant.

2. **`allow` + LiteLLM.** When LiteLLM is configured with `allow` egress, the LiteLLM-specific egress rule is built (lines 311-329) but then immediately replaced by the blanket allow. This is harmless — the agent can reach LiteLLM either way. Ingress from LiteLLM is still added to the ingress rules, which is correct (LiteLLM needs to reach the agent pod).

3. **No CRD schema changes.** The `egressPolicy` field is already a plain `string` in the CRD (line 39 of `agent_types.go`). Adding a new valid value (`allow`) does not require CRD regeneration or schema changes. The constant is only used in Go code.

4. **No default change.** The `SetDefaults()` function (line 36 of `agent_utils.go`) defaults `egressPolicy` to `deny`. The `allow` value must be explicitly set by the user. This is the correct security posture.

5. **Existing deny and allowList tests are untouched.** The new `allow` check is additive — it only fires when `egressPolicy == "allow"`, so the `deny` and `allowList` code paths are completely unaffected.

---

## Definition of Done

- [ ] `AgentEgressPolicyAllow` constant exists with value `"allow"`
- [ ] `allow` egress mode generates a NetworkPolicy with 1 egress rule: empty (allow all)
- [ ] Ingress restrictions are preserved in `allow` mode (MM pods only, or MM + LiteLLM)
- [ ] `allow` mode correctly overrides LiteLLM-specific egress rules
- [ ] Existing `deny` tests still pass (2 egress rules)
- [ ] Existing `allowList` tests still pass (4 egress rules)
- [ ] Existing `deny + LiteLLM` tests still pass (3 egress rules)
- [ ] New tests validate `allow` mode behavior with and without LiteLLM
- [ ] `go build ./...` succeeds
