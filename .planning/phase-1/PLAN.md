# Phase 1: Remove mmctl Init Container + HOME Override — Prescriptive Plan

> **Milestone:** M7 — PVC, Init Container Removal, Allow Egress
> **Target repo:** `~/workspace/worktrees/mattermost-operator-the-trail`
> **Phase:** 1 (Init Container Removal)
> **Depends on:** nothing (first phase)
> **Goal:** Remove the redundant `mmctl-auth` init container, `mmctl-config` EmptyDir volume, and `HOME=/tmp` env var override from `GenerateAgentDeployment`. This unblocks non-Python agents (OpenClaw) that don't use mmctl.

---

## Context: What Already Exists

`GenerateAgentDeployment` in `pkg/mattermost/agent.go` (line 68) currently produces a Deployment with:
- **InitContainers** (lines 153-179): A single `mmctl-auth` container that runs `mmctl auth login ...` using the bot-token volume and an mmctl-config EmptyDir.
- **Main container VolumeMounts** (lines 196-203): Two mounts — `bot-token` at `/secrets/mmctl-token` (read-only) and `mmctl-config` at `/tmp/.config/mmctl`.
- **Main container Env** (line 185): `append(envVars, corev1.EnvVar{Name: "HOME", Value: "/tmp"})` — HOME override so mmctl can find its config.
- **Volumes** (lines 209-224): Two volumes — `bot-token` (Secret) and `mmctl-config` (EmptyDir).

The init container and mmctl-config volume exist solely for mmctl authentication. With Trail moving to framework-agnostic agents that authenticate via `MM_BOT_TOKEN` env var (SecretKeyRef), these are dead code. The `HOME=/tmp` override also breaks agents that expect their image's default HOME.

The `bot-token` Secret volume + mount is **kept** — some agent images may still want to read the token from a file at `/secrets/mmctl-token`. The `MM_BOT_TOKEN` env var (lines 82-91) reads from the same Secret via SecretKeyRef, so it works regardless.

---

## Task 1.1: Remove Init Container from GenerateAgentDeployment

**File:** `pkg/mattermost/agent.go`
**Lines:** 151-179

### Change

Remove the entire `InitContainers` field from the PodSpec:

```go
// REMOVE these lines (151-179):
					InitContainers: []corev1.Container{
						{
							Name:    "mmctl-auth",
							Image:   agent.Spec.Image,
							Command: []string{"mmctl", "auth", "login", "$(MM_SERVER_URL)", "--access-token-file", "/secrets/mmctl-token/token", "--name", "local"},
							Env: []corev1.EnvVar{
								{
									Name:  "MM_SERVER_URL",
									Value: mmServerURL(agent),
								},
								{
									Name:  "HOME",
									Value: "/tmp",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "bot-token",
									MountPath: "/secrets/mmctl-token",
									ReadOnly:  true,
								},
								{
									Name:      "mmctl-config",
									MountPath: "/tmp/.config/mmctl",
								},
							},
						},
					},
```

After removal, the PodSpec goes directly from `ServiceAccountName` to `Containers`.

---

## Task 1.2: Remove mmctl-config EmptyDir Volume

**File:** `pkg/mattermost/agent.go`
**Lines:** 218-222

### Change

Remove the `mmctl-config` entry from the `Volumes` slice, keeping only `bot-token`:

```go
// OLD (lines 209-224):
					Volumes: []corev1.Volume{
						{
							Name: "bot-token",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: agent.BotTokenSecretName(),
								},
							},
						},
						{
							Name: "mmctl-config",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},

// NEW:
					Volumes: []corev1.Volume{
						{
							Name: "bot-token",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: agent.BotTokenSecretName(),
								},
							},
						},
					},
```

Also remove the `mmctl-config` VolumeMount from the main container (lines 200-203):

```go
// OLD (lines 196-203):
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "bot-token",
									MountPath: "/secrets/mmctl-token",
									ReadOnly:  true,
								},
								{
									Name:      "mmctl-config",
									MountPath: "/tmp/.config/mmctl",
								},
							},

// NEW:
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "bot-token",
									MountPath: "/secrets/mmctl-token",
									ReadOnly:  true,
								},
							},
```

---

## Task 1.3: Remove HOME=/tmp Env Var Override

**File:** `pkg/mattermost/agent.go`
**Line:** 185

### Change

```go
// OLD (line 185):
							Env: append(envVars, corev1.EnvVar{
								Name:  "HOME",
								Value: "/tmp",
							}),

// NEW:
							Env: envVars,
```

Each agent image now owns its own HOME directory.

---

## Task 1.4: Update Unit Tests

### 1.4a: Update `TestGenerateAgentDeployment`

**File:** `pkg/mattermost/agent_test.go`
**Lines:** 132-172

Replace the volume mount, init container, and volume assertion blocks:

```go
// OLD (lines 132-172):
	// Volume mounts on main container
	assert.Len(t, c.VolumeMounts, 2)
	var hasBotToken, hasMmctlConfig bool
	for _, vm := range c.VolumeMounts {
		if vm.Name == "bot-token" {
			hasBotToken = true
			assert.Equal(t, "/secrets/mmctl-token", vm.MountPath)
			assert.True(t, vm.ReadOnly)
		}
		if vm.Name == "mmctl-config" {
			hasMmctlConfig = true
			assert.Equal(t, "/tmp/.config/mmctl", vm.MountPath)
		}
	}
	assert.True(t, hasBotToken, "bot-token volume mount expected")
	assert.True(t, hasMmctlConfig, "mmctl-config volume mount expected")

	// Init container
	initContainers := dep.Spec.Template.Spec.InitContainers
	assert.Len(t, initContainers, 1)
	init := initContainers[0]
	assert.Equal(t, "mmctl-auth", init.Name)
	assert.Contains(t, init.Command, "mmctl")
	assert.NotContains(t, init.Command, "--insecure-skip-verify")

	// Volumes
	volumes := dep.Spec.Template.Spec.Volumes
	assert.Len(t, volumes, 2)
	var hasBotTokenVol, hasMmctlConfigVol bool
	for _, v := range volumes {
		if v.Name == "bot-token" {
			hasBotTokenVol = true
			assert.Equal(t, agent.BotTokenSecretName(), v.Secret.SecretName)
		}
		if v.Name == "mmctl-config" {
			hasMmctlConfigVol = true
			assert.NotNil(t, v.EmptyDir)
		}
	}
	assert.True(t, hasBotTokenVol, "bot-token volume expected")
	assert.True(t, hasMmctlConfigVol, "mmctl-config emptyDir volume expected")

// NEW:
	// Volume mounts on main container — only bot-token remains
	assert.Len(t, c.VolumeMounts, 1)
	assert.Equal(t, "bot-token", c.VolumeMounts[0].Name)
	assert.Equal(t, "/secrets/mmctl-token", c.VolumeMounts[0].MountPath)
	assert.True(t, c.VolumeMounts[0].ReadOnly)

	// No init containers
	assert.Empty(t, dep.Spec.Template.Spec.InitContainers, "init containers must be removed")

	// No HOME env var
	for _, e := range c.Env {
		assert.NotEqual(t, "HOME", e.Name, "HOME env var must not be present")
	}

	// Volumes — only bot-token remains
	volumes := dep.Spec.Template.Spec.Volumes
	assert.Len(t, volumes, 1)
	assert.Equal(t, "bot-token", volumes[0].Name)
	assert.Equal(t, agent.BotTokenSecretName(), volumes[0].Secret.SecretName)
```

### 1.4b: Update `TestCheckAgentDeployment`

**File:** `controllers/mattermost/agent/agent_test.go`
**Lines:** 144-151

Replace the volume and init container assertions:

```go
// OLD (lines 144-151):
	// Verify volumes.
	assert.Len(t, deployment.Spec.Template.Spec.Volumes, 2)
	assert.Equal(t, "bot-token", deployment.Spec.Template.Spec.Volumes[0].Name)
	assert.Equal(t, "mmctl-config", deployment.Spec.Template.Spec.Volumes[1].Name)

	// Verify init container.
	require.Len(t, deployment.Spec.Template.Spec.InitContainers, 1)
	assert.Equal(t, "mmctl-auth", deployment.Spec.Template.Spec.InitContainers[0].Name)

// NEW:
	// Verify volumes — only bot-token remains.
	assert.Len(t, deployment.Spec.Template.Spec.Volumes, 1)
	assert.Equal(t, "bot-token", deployment.Spec.Template.Spec.Volumes[0].Name)

	// Verify no init containers.
	assert.Empty(t, deployment.Spec.Template.Spec.InitContainers, "init containers must be removed")

	// Verify no HOME env var.
	for _, e := range container.Env {
		assert.NotEqual(t, "HOME", e.Name, "HOME env var must not be present")
	}
```

---

## Complete `GenerateAgentDeployment` After Phase 1

For the implementing agent's reference, the Deployment returned after all Phase 1 changes:

```go
func GenerateAgentDeployment(agent *mmv1beta.Agent) *appsv1.Deployment {
	replicas := int32(1)

	baseEnv := []corev1.EnvVar{
		// ... MM_SERVER_URL, AGENT_HOOKS, MM_BOT_TOKEN, HOOK_SECRET (unchanged)
	}

	// LiteLLM gateway env vars (unchanged)
	if agent.Spec.LLMGateway != nil { ... }

	envVars := mergeEnvVars(baseEnv, agent.Spec.Env)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{ /* unchanged */ },
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{ /* unchanged */ },
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{ /* unchanged */ },
				Spec: corev1.PodSpec{
					ServiceAccountName: agent.Name,
					// NO InitContainers
					Containers: []corev1.Container{
						{
							Name:      mmv1beta.AgentContainerName,
							Image:     agent.Spec.Image,
							Env:       envVars,  // NO HOME=/tmp appended
							Ports:     []corev1.ContainerPort{{ContainerPort: mmv1beta.AgentHTTPPort, Name: "http"}},
							Resources: agent.Spec.Resources,
							VolumeMounts: []corev1.VolumeMount{
								{Name: "bot-token", MountPath: "/secrets/mmctl-token", ReadOnly: true},
								// NO mmctl-config mount
							},
						},
					},
					Volumes: []corev1.Volume{
						{Name: "bot-token", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: agent.BotTokenSecretName()}}},
						// NO mmctl-config EmptyDir
					},
				},
			},
		},
	}
}
```

---

## File Change Summary

| File | Task | Action | Summary |
|------|------|--------|---------|
| `pkg/mattermost/agent.go` | 1.1, 1.2, 1.3 | Modify | Remove InitContainers block, mmctl-config Volume/VolumeMount, HOME=/tmp append |
| `pkg/mattermost/agent_test.go` | 1.4a | Modify | Assert 0 init containers, 1 volume, 1 volume mount, no HOME env |
| `controllers/mattermost/agent/agent_test.go` | 1.4b | Modify | Assert 1 volume (not 2), 0 init containers (not 1), no HOME env |

---

## Build & Verify

```bash
# Run affected unit tests
go test ./pkg/mattermost/... -v -count=1 -run TestGenerateAgentDeployment
go test ./controllers/mattermost/agent/... -v -count=1 -run TestCheckAgentDeployment

# Run full test suite
go test ./... -count=1

# Verify build
go build ./...
```

---

## Edge Cases & Gotchas

1. **bot-token mount kept intentionally.** The `MM_BOT_TOKEN` env var uses SecretKeyRef (reads from secret directly, not from mounted file). However, the volume mount at `/secrets/mmctl-token` is preserved because some agent images may read the token file directly. Removing it is a separate decision.

2. **No CRD changes.** This phase only touches Go code and tests — no `make generate manifests` needed.

3. **TestGenerateAgentDeployment_CustomEnvVars** (line 174 in `agent_test.go`) does NOT assert on volumes, init containers, or HOME, so it **passes without changes**.

4. **TestCheckAgentDeployment_WithLLMGateway** (line 196 in `agent_test.go`) does NOT assert on volumes or init containers — it only checks env vars. **Passes without changes**.

5. **TestReconcileAgent_FullReconcile** (line 78 in `controller_test.go`) does not assert on init containers or volume count. **Passes without changes**.

---

## Definition of Done

- [ ] `GenerateAgentDeployment` produces a Deployment with zero init containers
- [ ] Only `bot-token` volume remains; `mmctl-config` EmptyDir is gone
- [ ] Only `bot-token` volume mount on main container; `mmctl-config` mount is gone
- [ ] No `HOME=/tmp` env var in the main container
- [ ] All existing tests pass with updated assertions
- [ ] `go build ./...` succeeds
