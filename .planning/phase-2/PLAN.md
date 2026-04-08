# Phase 2: PVC Support — Prescriptive Plan

> **Milestone:** M7 — PVC, Init Container Removal, Allow Egress
> **Target repo:** `~/workspace/worktrees/mattermost-operator-the-trail`
> **Phase:** 2 (PVC Support)
> **Depends on:** Phase 1 (clean deployment without init container simplifies volume management)
> **Goal:** Add optional persistent storage to agent pods via a new `Storage` CRD field, PVC creation in the reconciler, and Volume/VolumeMount injection in the Deployment.

---

## Context: What Already Exists

### CRD Types
- `apis/mattermost/v1beta1/agent_types.go` — `AgentSpec` struct (line 22). Currently has no storage field.
- `apis/mattermost/v1beta1/agent_utils.go` — `SetDefaults()` (line 35), constants (lines 11-24), helpers like `BotTokenSecretName()` (line 87).

### PVC Patterns in This Repo
- `pkg/resources/create_resources.go` line 182: `CreatePvcIfNotExists(owner, pvc, reqLogger)` — follows the same create-if-not-exists pattern as other resources.
- `pkg/mattermost/file_store.go` line 43: `ExternalVolumeFileStore.Volumes()` — returns a PVC-backed Volume + VolumeMount. **This is the exact pattern to follow** for PVC volume injection:
  ```go
  volumes := []corev1.Volume{
      {
          Name: FileStoreDefaultVolumeName,
          VolumeSource: corev1.VolumeSource{
              PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
                  ClaimName: fs.VolumeClaimName,
              },
          },
      },
  }
  volumeMounts := []corev1.VolumeMount{
      {
          Name:      FileStoreDefaultVolumeName,
          MountPath: mmv1beta.DefaultLocalFilePath,
      },
  }
  ```

### Reconciler Patterns
- `controllers/mattermost/agent/agent.go` — check functions follow a consistent pattern:
  1. Generate desired resource
  2. Call `CreateXxxIfNotExists`
  3. Get current resource
  4. Call `r.Resources.Update(current, desired, reqLogger)`
- `controllers/mattermost/agent/controller.go` line 43: `SetupWithManager` chains `.Owns()` calls for Deployment, Service, ServiceAccount, Secret, ConfigMap, NetworkPolicy.
- `controllers/mattermost/agent/controller.go` lines 128-153: Reconcile loop calls check functions in order: ServiceAccount → HookSecret → Service → Deployment → NetworkPolicy.

### Owner References
- `pkg/mattermost/agent.go` line 17: `AgentOwnerReference(agent)` — used by all generated resources for cascade deletion.

### RBAC
- `config/rbac/role.yaml` line 15: `persistentvolumeclaims` already in the resource list with `['*']` verbs. **No RBAC changes needed.**

---

## Task 2.1: Add AgentStorageConfig Type and Storage Field to CRD

**File:** `apis/mattermost/v1beta1/agent_types.go`

### 2.1a: Add import for resource.Quantity

The `resource` package is already imported in `agent_utils.go` but NOT in `agent_types.go`. Add it:

```go
// OLD (lines 6-9):
import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NEW:
import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)
```

### 2.1b: Add AgentStorageConfig type

Insert **before** the `AgentSpec` struct (before line 22), after the imports and the IMPORTANT comment block:

```go
// AgentStorageConfig defines optional persistent storage for the agent pod.
type AgentStorageConfig struct {
	// Size is the requested PVC storage size (e.g., "1Gi", "500Mi").
	Size resource.Quantity `json:"size"`

	// StorageClassName is the name of the StorageClass to use for the PVC.
	// If omitted, the cluster default StorageClass is used.
	// +optional
	StorageClassName *string `json:"storageClassName,omitempty"`

	// MountPath is the path inside the container where the volume is mounted.
	// Defaults to "/data".
	// +optional
	MountPath string `json:"mountPath,omitempty"`
}
```

### 2.1c: Add Storage field to AgentSpec

Insert after the `LLMGateway` field (after line 59, before the closing brace of AgentSpec):

```go
	// Storage configures optional persistent storage for the agent pod.
	// When set, the operator creates a PVC and mounts it into the agent container.
	// +optional
	Storage *AgentStorageConfig `json:"storage,omitempty"`
```

### Full AgentSpec after change (key fields only):

```go
type AgentSpec struct {
	Image        string                      `json:"image"`
	Hooks        []string                    `json:"hooks,omitempty"`
	Resources    corev1.ResourceRequirements `json:"resources,omitempty"`
	EgressPolicy string                      `json:"egressPolicy,omitempty"`
	EgressAllowList []string                 `json:"egressAllowList,omitempty"`
	MattermostRef corev1.LocalObjectReference `json:"mattermostRef"`
	Env          []corev1.EnvVar             `json:"env,omitempty"`
	LLMGateway   *LLMGatewayConfig           `json:"llmGateway,omitempty"`
	Storage      *AgentStorageConfig          `json:"storage,omitempty"`  // NEW
}
```

---

## Task 2.2: Add Defaults and Helpers for Storage

**File:** `apis/mattermost/v1beta1/agent_utils.go`

### 2.2a: Add constant

Insert after the existing constants block (after line 23, inside the `const` block):

```go
// OLD (partial, line 11-24):
const (
	AgentEgressPolicyDeny             = "deny"
	AgentEgressPolicyAllowList        = "allowList"
	// ... other constants ...
	AgentLiteLLMDBCredentialsSecret   = "litellm-db-credentials"
)

// NEW — add this line inside the const block:
	AgentStorageDefaultMountPath      = "/data"
```

### 2.2b: Add Storage defaults in SetDefaults()

Insert after the LLMGateway defaults block (after line 58, before `return nil`):

```go
// OLD (lines 54-61):
	if a.Spec.LLMGateway != nil && a.Spec.LLMGateway.OperatorManaged != nil {
		if a.Spec.LLMGateway.OperatorManaged.Image == "" {
			a.Spec.LLMGateway.OperatorManaged.Image = AgentLiteLLMDefaultImage
		}
	}

	return nil

// NEW:
	if a.Spec.LLMGateway != nil && a.Spec.LLMGateway.OperatorManaged != nil {
		if a.Spec.LLMGateway.OperatorManaged.Image == "" {
			a.Spec.LLMGateway.OperatorManaged.Image = AgentLiteLLMDefaultImage
		}
	}

	if a.Spec.Storage != nil && a.Spec.Storage.MountPath == "" {
		a.Spec.Storage.MountPath = AgentStorageDefaultMountPath
	}

	return nil
```

### 2.2c: Add StoragePVCName helper

Insert after `HookSecretName()` (after line 99):

```go
// StoragePVCName returns the name of the PVC for the agent's persistent storage.
func (a *Agent) StoragePVCName() string {
	return a.Name + "-storage"
}
```

---

## Task 2.3: Add checkAgentPVC Reconciler Step

**File:** `controllers/mattermost/agent/agent.go`

Insert after the `checkAgentNetworkPolicy` function (after line 116). This follows the same pattern as other check functions but with two PVC-specific considerations:

1. **Early return if no storage** — skip entirely when `agent.Spec.Storage == nil`.
2. **PVC spec is mostly immutable** — K8s does not allow changing `storageClassName` or `accessModes` after creation. Only `resources.requests` can be expanded if the StorageClass supports it. The `Update` call handles this via objectMatcher.

```go
func (r *AgentReconciler) checkAgentPVC(ctx context.Context, agent *mmv1beta.Agent, reqLogger logr.Logger) error {
	if agent.Spec.Storage == nil {
		return nil
	}

	desired := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:            agent.StoragePVCName(),
			Namespace:       agent.Namespace,
			Labels:          mmv1beta.AgentLabels(agent.Name),
			OwnerReferences: mattermostApp.AgentOwnerReference(agent),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: agent.Spec.Storage.Size,
				},
			},
		},
	}

	if agent.Spec.Storage.StorageClassName != nil {
		desired.Spec.StorageClassName = agent.Spec.Storage.StorageClassName
	}

	err := r.Resources.CreatePvcIfNotExists(agent, desired, reqLogger)
	if err != nil {
		return errors.Wrap(err, "failed to create agent storage PVC")
	}

	current := &corev1.PersistentVolumeClaim{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, current)
	if err != nil {
		return errors.Wrap(err, "failed to get agent storage PVC")
	}

	return r.Resources.Update(current, desired, reqLogger)
}
```

### Required imports

Check the import block in `controllers/mattermost/agent/agent.go` (lines 1-19). The following are needed:
- `metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"` — **NOT currently imported.** Add it.
- `corev1` — already imported (line 16).
- `errors` from `github.com/pkg/errors` — already imported (line 13).
- `types` — already imported (line 18).
- `mattermostApp` — already imported (line 11).
- `mmv1beta` — already imported (line 9).

Add `metav1` to the import block:

```go
// OLD (lines 6-19):
import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/go-logr/logr"
	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	mattermostApp "github.com/mattermost/mattermost-operator/pkg/mattermost"
	"github.com/mattermost/mattermost-operator/pkg/resources"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
)

// NEW — add metav1:
import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/go-logr/logr"
	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	mattermostApp "github.com/mattermost/mattermost-operator/pkg/mattermost"
	"github.com/mattermost/mattermost-operator/pkg/resources"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)
```

> **Note:** The unused import `"github.com/mattermost/mattermost-operator/pkg/resources"` (line 12) should be verified — it's used in the `AgentReconciler` struct definition in `controller.go`, not in `agent.go`. If the compiler flags it, it can be removed from `agent.go` since `r.Resources` is accessed via the struct field, not a direct import.

---

## Task 2.4: Add checkAgentPVC Call to Reconciler

**File:** `controllers/mattermost/agent/controller.go`
**Lines:** 147-153

Insert the PVC check **before** `checkAgentDeployment` (before line 149). The PVC must exist before the Deployment references it as a volume.

```go
// OLD (lines 147-153):
	// Deployment
	err = r.checkAgentDeployment(ctx, agent, reqLogger)
	if err != nil {
		r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
		return reconcile.Result{}, err
	}

// NEW:
	// PVC (must exist before Deployment references it)
	err = r.checkAgentPVC(ctx, agent, reqLogger)
	if err != nil {
		r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
		return reconcile.Result{}, err
	}

	// Deployment
	err = r.checkAgentDeployment(ctx, agent, reqLogger)
	if err != nil {
		r.updateStatusReconcilingAndLogError(ctx, agent, status, reqLogger, err)
		return reconcile.Result{}, err
	}
```

---

## Task 2.5: Add Owns PVC to Controller Setup

**File:** `controllers/mattermost/agent/controller.go`
**Lines:** 43-53

Add `.Owns(&corev1.PersistentVolumeClaim{})` to the builder chain so the controller reconciles when a PVC owned by an Agent changes:

```go
// OLD (lines 43-53):
func (r *AgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mmv1beta.Agent{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&networkingv1.NetworkPolicy{}).
		Complete(r)
}

// NEW:
func (r *AgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mmv1beta.Agent{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&networkingv1.NetworkPolicy{}).
		Complete(r)
}
```

---

## Task 2.6: Inject PVC Volume + VolumeMount in GenerateAgentDeployment

**File:** `pkg/mattermost/agent.go`

After Phase 1, the Deployment has a single `bot-token` volume and mount defined inline. Refactor to use slices that can be conditionally extended.

### Change

Replace the inline Volumes and VolumeMounts in the Deployment struct with slice variables defined before the return. The exact location depends on Phase 1 output, but the pattern is:

**Insert before the `return &appsv1.Deployment{` statement** (currently line 135 after Phase 1 changes):

```go
	// Build volume and mount lists — start with bot-token, conditionally add storage.
	volumes := []corev1.Volume{
		{
			Name: "bot-token",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: agent.BotTokenSecretName(),
				},
			},
		},
	}
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "bot-token",
			MountPath: "/secrets/mmctl-token",
			ReadOnly:  true,
		},
	}

	if agent.Spec.Storage != nil {
		volumes = append(volumes, corev1.Volume{
			Name: "agent-storage",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: agent.StoragePVCName(),
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "agent-storage",
			MountPath: agent.Spec.Storage.MountPath,
		})
	}
```

**Then update the Deployment struct** to reference the slice variables instead of inline definitions:

```go
					Containers: []corev1.Container{
						{
							Name:         mmv1beta.AgentContainerName,
							Image:        agent.Spec.Image,
							Env:          envVars,
							Ports:        []corev1.ContainerPort{{ContainerPort: mmv1beta.AgentHTTPPort, Name: "http"}},
							Resources:    agent.Spec.Resources,
							VolumeMounts: volumeMounts,  // was inline
						},
					},
					Volumes: volumes,  // was inline
```

---

## Task 2.7: Run `make generate manifests`

After all type changes, run:

```bash
cd ~/workspace/worktrees/mattermost-operator-the-trail
make generate manifests
```

This regenerates:
- `config/crd/bases/installation.mattermost.com_agents.yaml` — will include the new `storage` field with `size`, `storageClassName`, `mountPath` subfields.
- `apis/mattermost/v1beta1/zz_generated.deepcopy.go` — will include `DeepCopyInto` for `AgentStorageConfig`.

**Verify:** `grep -A 10 'storage:' config/crd/bases/installation.mattermost.com_agents.yaml` should show the new fields.

---

## Task 2.8: Unit Tests for PVC Support

### 2.8a: Test defaults for Storage

**File:** Add to an existing test file in the `v1beta1` package, or to `pkg/mattermost/agent_test.go`.

Check if there's an existing test file:

```bash
ls apis/mattermost/v1beta1/*test*
```

Add these tests wherever agent defaults are tested:

```go
func TestSetDefaults_StorageMountPath(t *testing.T) {
	agent := &mmv1beta.Agent{
		Spec: mmv1beta.AgentSpec{
			Image:         "test:latest",
			MattermostRef: corev1.LocalObjectReference{Name: "mm"},
			Storage: &mmv1beta.AgentStorageConfig{
				Size: resource.MustParse("1Gi"),
			},
		},
	}
	err := agent.SetDefaults()
	require.NoError(t, err)
	assert.Equal(t, mmv1beta.AgentStorageDefaultMountPath, agent.Spec.Storage.MountPath)
}

func TestSetDefaults_StorageMountPathPreserved(t *testing.T) {
	agent := &mmv1beta.Agent{
		Spec: mmv1beta.AgentSpec{
			Image:         "test:latest",
			MattermostRef: corev1.LocalObjectReference{Name: "mm"},
			Storage: &mmv1beta.AgentStorageConfig{
				Size:      resource.MustParse("1Gi"),
				MountPath: "/custom/path",
			},
		},
	}
	err := agent.SetDefaults()
	require.NoError(t, err)
	assert.Equal(t, "/custom/path", agent.Spec.Storage.MountPath)
}

func TestStoragePVCName(t *testing.T) {
	agent := &mmv1beta.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "my-agent"},
	}
	assert.Equal(t, "my-agent-storage", agent.StoragePVCName())
}
```

### 2.8b: Test GenerateAgentDeployment with Storage

**File:** `pkg/mattermost/agent_test.go`

Add after the existing `TestGenerateAgentDeployment_CustomEnvVars` test:

```go
func TestGenerateAgentDeployment_WithStorage(t *testing.T) {
	agent := testAgent("my-agent", "test-ns")
	storageClass := "fast-ssd"
	agent.Spec.Storage = &mmv1beta.AgentStorageConfig{
		Size:             resource.MustParse("5Gi"),
		StorageClassName: &storageClass,
		MountPath:        "/workspace",
	}

	dep := GenerateAgentDeployment(agent)

	// Volumes: bot-token + agent-storage
	volumes := dep.Spec.Template.Spec.Volumes
	assert.Len(t, volumes, 2)
	assert.Equal(t, "bot-token", volumes[0].Name)
	assert.Equal(t, "agent-storage", volumes[1].Name)
	assert.Equal(t, agent.StoragePVCName(), volumes[1].PersistentVolumeClaim.ClaimName)

	// Volume mounts: bot-token + agent-storage
	mounts := dep.Spec.Template.Spec.Containers[0].VolumeMounts
	assert.Len(t, mounts, 2)
	assert.Equal(t, "bot-token", mounts[0].Name)
	assert.Equal(t, "agent-storage", mounts[1].Name)
	assert.Equal(t, "/workspace", mounts[1].MountPath)
}

func TestGenerateAgentDeployment_WithoutStorage(t *testing.T) {
	agent := testAgent("my-agent", "test-ns")
	// Storage is nil by default in testAgent

	dep := GenerateAgentDeployment(agent)

	// Only bot-token volume
	volumes := dep.Spec.Template.Spec.Volumes
	assert.Len(t, volumes, 1)
	assert.Equal(t, "bot-token", volumes[0].Name)

	// Only bot-token mount
	mounts := dep.Spec.Template.Spec.Containers[0].VolumeMounts
	assert.Len(t, mounts, 1)
	assert.Equal(t, "bot-token", mounts[0].Name)
}
```

### 2.8c: Test checkAgentPVC in reconciler

**File:** `controllers/mattermost/agent/agent_test.go`

Add after the existing `TestCheckAgentNetworkPolicy_DenyWithLiteLLM` test:

```go
func TestCheckAgentPVC_Creates(t *testing.T) {
	agent := newTestAgent()
	agent.Spec.Storage = &mmv1beta.AgentStorageConfig{
		Size:      resource.MustParse("1Gi"),
		MountPath: "/data",
	}
	_ = agent.SetDefaults()

	reconciler, _ := setupReconciler(t, agent)
	logger := testLogger()

	err := reconciler.checkAgentPVC(context.TODO(), agent, logger)
	require.NoError(t, err)

	// Verify PVC was created.
	pvc := &corev1.PersistentVolumeClaim{}
	err = reconciler.Client.Get(context.TODO(), types.NamespacedName{
		Name:      agent.StoragePVCName(),
		Namespace: agent.Namespace,
	}, pvc)
	require.NoError(t, err)

	assert.Equal(t, agent.StoragePVCName(), pvc.Name)
	assert.Equal(t, agent.Namespace, pvc.Namespace)
	assert.Equal(t, []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}, pvc.Spec.AccessModes)

	storageReq := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
	assert.Equal(t, resource.MustParse("1Gi"), storageReq)
}

func TestCheckAgentPVC_Skips(t *testing.T) {
	agent := newTestAgent()
	// Storage is nil — PVC should not be created.
	_ = agent.SetDefaults()

	reconciler, _ := setupReconciler(t, agent)
	logger := testLogger()

	err := reconciler.checkAgentPVC(context.TODO(), agent, logger)
	require.NoError(t, err)

	// Verify no PVC exists.
	pvc := &corev1.PersistentVolumeClaim{}
	err = reconciler.Client.Get(context.TODO(), types.NamespacedName{
		Name:      agent.Name + "-storage",
		Namespace: agent.Namespace,
	}, pvc)
	require.Error(t, err, "PVC should not exist when Storage is nil")
}

func TestCheckAgentPVC_OwnerReference(t *testing.T) {
	agent := newTestAgent()
	agent.Spec.Storage = &mmv1beta.AgentStorageConfig{
		Size:      resource.MustParse("2Gi"),
		MountPath: "/data",
	}
	_ = agent.SetDefaults()

	reconciler, _ := setupReconciler(t, agent)
	logger := testLogger()

	err := reconciler.checkAgentPVC(context.TODO(), agent, logger)
	require.NoError(t, err)

	pvc := &corev1.PersistentVolumeClaim{}
	err = reconciler.Client.Get(context.TODO(), types.NamespacedName{
		Name:      agent.StoragePVCName(),
		Namespace: agent.Namespace,
	}, pvc)
	require.NoError(t, err)

	// Verify OwnerReference points to the Agent CR.
	require.Len(t, pvc.OwnerReferences, 1)
	assert.Equal(t, "Agent", pvc.OwnerReferences[0].Kind)
	assert.Equal(t, agent.Name, pvc.OwnerReferences[0].Name)
}

func TestCheckAgentPVC_WithStorageClass(t *testing.T) {
	agent := newTestAgent()
	storageClass := "gp3-encrypted"
	agent.Spec.Storage = &mmv1beta.AgentStorageConfig{
		Size:             resource.MustParse("10Gi"),
		StorageClassName: &storageClass,
		MountPath:        "/workspace",
	}
	_ = agent.SetDefaults()

	reconciler, _ := setupReconciler(t, agent)
	logger := testLogger()

	err := reconciler.checkAgentPVC(context.TODO(), agent, logger)
	require.NoError(t, err)

	pvc := &corev1.PersistentVolumeClaim{}
	err = reconciler.Client.Get(context.TODO(), types.NamespacedName{
		Name:      agent.StoragePVCName(),
		Namespace: agent.Namespace,
	}, pvc)
	require.NoError(t, err)

	require.NotNil(t, pvc.Spec.StorageClassName)
	assert.Equal(t, "gp3-encrypted", *pvc.Spec.StorageClassName)
}

func TestCheckAgentPVC_Idempotent(t *testing.T) {
	agent := newTestAgent()
	agent.Spec.Storage = &mmv1beta.AgentStorageConfig{
		Size:      resource.MustParse("1Gi"),
		MountPath: "/data",
	}
	_ = agent.SetDefaults()

	reconciler, _ := setupReconciler(t, agent)
	logger := testLogger()

	// First call creates.
	err := reconciler.checkAgentPVC(context.TODO(), agent, logger)
	require.NoError(t, err)

	// Second call is idempotent.
	err = reconciler.checkAgentPVC(context.TODO(), agent, logger)
	require.NoError(t, err)
}
```

**Required imports for agent_test.go** — add `resource` if not already present:

```go
import (
	// ... existing imports ...
	"k8s.io/apimachinery/pkg/api/resource"
)
```

---

## File Change Summary

| File | Task | Action | Summary |
|------|------|--------|---------|
| `apis/mattermost/v1beta1/agent_types.go` | 2.1 | Modify | Add `resource` import, `AgentStorageConfig` type, `Storage` field to `AgentSpec` |
| `apis/mattermost/v1beta1/agent_utils.go` | 2.2 | Modify | Add `AgentStorageDefaultMountPath` constant, Storage defaults in `SetDefaults()`, `StoragePVCName()` helper |
| `controllers/mattermost/agent/agent.go` | 2.3 | Modify | Add `metav1` import, `checkAgentPVC()` function |
| `controllers/mattermost/agent/controller.go` | 2.4, 2.5 | Modify | Add `checkAgentPVC` call before Deployment, add `.Owns(&corev1.PersistentVolumeClaim{})` |
| `pkg/mattermost/agent.go` | 2.6 | Modify | Refactor to slice-based volumes/mounts, conditionally add PVC volume+mount |
| `config/crd/` (generated) | 2.7 | Generate | Regenerated CRD manifests with `storage` field |
| `apis/mattermost/v1beta1/zz_generated.deepcopy.go` | 2.7 | Generate | Regenerated deep copy for `AgentStorageConfig` |
| `pkg/mattermost/agent_test.go` | 2.8b | Modify | Add `TestGenerateAgentDeployment_WithStorage`, `_WithoutStorage` |
| `controllers/mattermost/agent/agent_test.go` | 2.8c | Modify | Add `TestCheckAgentPVC_Creates`, `_Skips`, `_OwnerReference`, `_WithStorageClass`, `_Idempotent` |

---

## Build & Verify

```bash
# Regenerate CRD and deep copy
make generate manifests

# Verify CRD has storage field
grep -A 10 'storage:' config/crd/bases/installation.mattermost.com_agents.yaml

# Run affected unit tests
go test ./apis/mattermost/v1beta1/... -v -count=1
go test ./pkg/mattermost/... -v -count=1 -run TestGenerateAgentDeployment
go test ./controllers/mattermost/agent/... -v -count=1 -run TestCheckAgentPVC

# Run full test suite
go test ./... -count=1

# Verify build
go build ./...
```

---

## Edge Cases & Gotchas

1. **PVC spec is immutable.** Once a PVC is created, K8s does not allow changing `accessModes` or `storageClassName`. Only `resources.requests` can be expanded (if the StorageClass has `allowVolumeExpansion: true`). The `r.Resources.Update()` call uses objectMatcher, which will only send a patch if there's a diff — so this is safe. But if a user changes `storageClassName` on a running Agent, the update will fail at the K8s API level. This is expected K8s behavior, not a bug.

2. **PVC lifecycle.** The OwnerReference on the PVC means K8s garbage collection will delete the PVC when the Agent CR is deleted. This is intentional — if the user wants to preserve data, they should back it up before deleting the Agent.

3. **`resource.Quantity` import.** The `agent_types.go` file needs `"k8s.io/apimachinery/pkg/api/resource"` imported because `AgentStorageConfig.Size` uses `resource.Quantity`. The import already exists in `agent_utils.go` but each file needs its own imports in Go.

4. **Deep copy generation.** Adding a new type (`AgentStorageConfig`) with pointer fields (`*string` for StorageClassName) requires `make generate` to regenerate `zz_generated.deepcopy.go`. Without this, the operator will panic on deep copy operations.

5. **Reconcile ordering.** `checkAgentPVC` MUST run before `checkAgentDeployment`. If the Deployment references a PVC that doesn't exist yet, the pod will be stuck in `Pending` with an `Unschedulable` event. The reconciler will eventually create the PVC on the next loop, but this creates unnecessary churn.

6. **Existing test fixtures.** The `newTestAgent()` helper in `agent_test.go` (line 25) and `testAgent()` in `agent_test.go` (line 15 in pkg/mattermost) do NOT set Storage. This means all existing tests continue to work without changes — the no-storage path is the default.

---

## Definition of Done

- [ ] `AgentStorageConfig` type exists in CRD with `size`, `storageClassName`, `mountPath`
- [ ] `SetDefaults` sets mountPath to `/data` when omitted
- [ ] `StoragePVCName()` returns `<agent-name>-storage`
- [ ] Reconciler creates PVC before Deployment when Storage is set
- [ ] Reconciler skips PVC when Storage is nil
- [ ] PVC has OwnerReference on Agent CR (cascade delete)
- [ ] PVC has correct labels, access mode (ReadWriteOnce), and requested size
- [ ] PVC supports optional StorageClassName
- [ ] Deployment has PVC Volume (`agent-storage`) + VolumeMount when Storage is set
- [ ] Deployment has only `bot-token` volume when Storage is nil
- [ ] Controller watches PVCs via `Owns(&corev1.PersistentVolumeClaim{})`
- [ ] CRD manifests regenerated with `storage` field
- [ ] Deep copy generated for `AgentStorageConfig`
- [ ] All tests pass
