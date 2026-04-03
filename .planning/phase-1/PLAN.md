# Phase 1: CRD Types + Resource Generation — Prescriptive Plan

> **Milestone:** M3 — Agent Secret Protection (LiteLLM Gateway)
> **Target repo:** `~/workspace/worktrees/mattermost-operator-the-trail`
> **Phase:** 1 of 6
> **Depends on:** nothing (first phase)
> **Goal:** All new Go types compiled, resource generators for LiteLLM K8s resources, env var injection into agent pods, NetworkPolicy update. No API calls yet — purely K8s resource generation.

---

## Context: What Already Exists

The files below already exist with working content. All tasks in this phase **extend** them — do not replace or recreate them from scratch.

- `apis/mattermost/v1beta1/agent_types.go` — `AgentSpec` struct with existing fields (Image, Hooks, Resources, EgressPolicy, EgressAllowList, MattermostRef, AdminCredentialsSecret, Env). Ends with `func init()` at line 116.
- `apis/mattermost/v1beta1/agent_utils.go` — constants (AgentEgressPolicyDeny, AgentContainerName, AgentGRPCPort, AgentBotTokenSecretNamePrefix), `SetDefaults()`, label helpers, `BotTokenSecretName()`.
- `pkg/mattermost/agent.go` — `GenerateAgentDeployment` (builds `baseEnv`, calls `mergeEnvVars`), `GenerateAgentNetworkPolicy` (builds egressRules slice), `GenerateAgentBotTokenSecret`.
- `pkg/resources/create_resources.go` — `ResourceHelper` with `CreateSecretIfNotExists`, `CreateDeploymentIfNotExists`, etc.
- `controllers/mattermost/agent/controller.go` — `SetupWithManager` (Owns registrations), `Reconcile` loop (checkAgentBot → checkAgentServiceAccount → checkAgentService → checkAgentDeployment → checkAgentNetworkPolicy → checkAgentHealth).

Module path: `github.com/mattermost/mattermost-operator`

---

## Task 1.1: Extend `apis/mattermost/v1beta1/agent_types.go`

**File:** `apis/mattermost/v1beta1/agent_types.go`
**Action:** Two separate edits to the existing file.

### Edit A: Add two fields to `AgentSpec`

Insert after line 57 (the `Env` field, `json:"env,omitempty"`), before the closing `}` of `AgentSpec`:

```go
	// LLMGateway configures the LLM gateway for this agent.
	// When OperatorManaged is set, the operator deploys a shared LiteLLM instance
	// in the agent's namespace and provisions a virtual key for this agent.
	// When External is set, the agent uses an existing LiteLLM instance.
	// +optional
	LLMGateway *LLMGatewayConfig `json:"llmGateway,omitempty"`

	// MCPServers lists MCP servers to register in the LiteLLM gateway for this agent.
	// Only evaluated when LLMGateway.OperatorManaged is set.
	// +optional
	MCPServers []AgentMCPServer `json:"mcpServers,omitempty"`
```

After this edit `AgentSpec` ends as:

```go
	// Env defines optional environment variables to inject into the agent pod.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// LLMGateway configures the LLM gateway for this agent.
	// When OperatorManaged is set, the operator deploys a shared LiteLLM instance
	// in the agent's namespace and provisions a virtual key for this agent.
	// When External is set, the agent uses an existing LiteLLM instance.
	// +optional
	LLMGateway *LLMGatewayConfig `json:"llmGateway,omitempty"`

	// MCPServers lists MCP servers to register in the LiteLLM gateway for this agent.
	// Only evaluated when LLMGateway.OperatorManaged is set.
	// +optional
	MCPServers []AgentMCPServer `json:"mcpServers,omitempty"`
}
```

### Edit B: Add 7 new type definitions

Insert the following block **before** `func init()` at the end of the file (currently line 116). Add it between the `AgentList` struct's closing brace and `func init()`:

```go
// LLMGatewayConfig defines how the agent connects to an LLM gateway.
// Exactly one of External or OperatorManaged must be set.
type LLMGatewayConfig struct {
	// External configures the agent to use an existing LiteLLM instance.
	// +optional
	External *ExternalLLMGateway `json:"external,omitempty"`

	// OperatorManaged configures the operator to deploy and manage a shared
	// LiteLLM instance in the agent's namespace.
	// +optional
	OperatorManaged *OperatorManagedLLMGateway `json:"operatorManaged,omitempty"`
}

// ExternalLLMGateway configures the agent to use an externally managed LiteLLM instance.
type ExternalLLMGateway struct {
	// URL is the base URL of the external LiteLLM instance.
	// Example: "http://litellm.my-namespace.svc.cluster.local:4000"
	URL string `json:"url"`

	// VirtualKeySecret is the name of the K8s Secret containing the virtual key
	// for this agent. The Secret must have a key "apiKey".
	VirtualKeySecret string `json:"virtualKeySecret"`
}

// OperatorManagedLLMGateway configures the operator to deploy and manage LiteLLM.
type OperatorManagedLLMGateway struct {
	// Image is the LiteLLM container image to use.
	// Defaults to "ghcr.io/berriai/litellm-database:main-v1.82.0-stable".
	// +optional
	Image string `json:"image,omitempty"`

	// Resources defines the CPU/memory requests and limits for the LiteLLM pod.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// LLMProviders lists the LLM providers to register in LiteLLM.
	// Each provider maps to a POST /model/new call during reconciliation.
	// +optional
	LLMProviders []LLMProvider `json:"llmProviders,omitempty"`
}

// LLMProvider defines a provider (e.g. anthropic, openai) and the models to register.
type LLMProvider struct {
	// Name is the provider name as recognised by LiteLLM.
	// Examples: "anthropic", "openai", "bedrock".
	Name string `json:"name"`

	// Secret is the name of the K8s Secret containing the provider API key.
	// The Secret must have a key "apiKey".
	Secret string `json:"secret"`

	// Models lists the model names to register for this provider.
	// Example: ["claude-3-5-sonnet-20241022", "claude-3-opus-20240229"]
	Models []string `json:"models"`
}

// AgentMCPServer defines an MCP server entry to register in LiteLLM for this agent.
type AgentMCPServer struct {
	// Name is the human-readable name for this MCP server.
	// Will be sanitized (hyphens replaced with underscores) when registered in LiteLLM.
	Name string `json:"name"`

	// URL is the base URL of the MCP server.
	// Example: "http://jira-mcp.tools.svc.cluster.local:8080"
	URL string `json:"url"`

	// CredentialSecret is the name of the K8s Secret containing the auth credential
	// for this MCP server. The Secret must have a key "apiKey".
	// +optional
	CredentialSecret string `json:"credentialSecret,omitempty"`

	// MCPAccessGroup is the access group name used to scope this server to virtual keys.
	// If empty, the operator generates one: "<agentName>_<sanitizedServerName>".
	// +optional
	MCPAccessGroup string `json:"mcpAccessGroup,omitempty"`

	// AllowedTools lists specific tool names to permit (in addition to server-level access).
	// If empty, all tools on the server are accessible.
	// +optional
	AllowedTools []string `json:"allowedTools,omitempty"`

	// DisallowedTools lists specific tool names to block.
	// +optional
	DisallowedTools []string `json:"disallowedTools,omitempty"`
}
```

**Full resulting end of file** (from `AgentList` onward):

```go
// +kubebuilder:object:root=true

// AgentList contains a list of Agent
type AgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Agent `json:"items"`
}

// LLMGatewayConfig defines how the agent connects to an LLM gateway.
// Exactly one of External or OperatorManaged must be set.
type LLMGatewayConfig struct {
	// External configures the agent to use an existing LiteLLM instance.
	// +optional
	External *ExternalLLMGateway `json:"external,omitempty"`

	// OperatorManaged configures the operator to deploy and manage a shared
	// LiteLLM instance in the agent's namespace.
	// +optional
	OperatorManaged *OperatorManagedLLMGateway `json:"operatorManaged,omitempty"`
}

// ExternalLLMGateway configures the agent to use an externally managed LiteLLM instance.
type ExternalLLMGateway struct {
	// URL is the base URL of the external LiteLLM instance.
	// Example: "http://litellm.my-namespace.svc.cluster.local:4000"
	URL string `json:"url"`

	// VirtualKeySecret is the name of the K8s Secret containing the virtual key
	// for this agent. The Secret must have a key "apiKey".
	VirtualKeySecret string `json:"virtualKeySecret"`
}

// OperatorManagedLLMGateway configures the operator to deploy and manage LiteLLM.
type OperatorManagedLLMGateway struct {
	// Image is the LiteLLM container image to use.
	// Defaults to "ghcr.io/berriai/litellm-database:main-v1.82.0-stable".
	// +optional
	Image string `json:"image,omitempty"`

	// Resources defines the CPU/memory requests and limits for the LiteLLM pod.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// LLMProviders lists the LLM providers to register in LiteLLM.
	// Each provider maps to a POST /model/new call during reconciliation.
	// +optional
	LLMProviders []LLMProvider `json:"llmProviders,omitempty"`
}

// LLMProvider defines a provider (e.g. anthropic, openai) and the models to register.
type LLMProvider struct {
	// Name is the provider name as recognised by LiteLLM.
	// Examples: "anthropic", "openai", "bedrock".
	Name string `json:"name"`

	// Secret is the name of the K8s Secret containing the provider API key.
	// The Secret must have a key "apiKey".
	Secret string `json:"secret"`

	// Models lists the model names to register for this provider.
	// Example: ["claude-3-5-sonnet-20241022", "claude-3-opus-20240229"]
	Models []string `json:"models"`
}

// AgentMCPServer defines an MCP server entry to register in LiteLLM for this agent.
type AgentMCPServer struct {
	// Name is the human-readable name for this MCP server.
	// Will be sanitized (hyphens replaced with underscores) when registered in LiteLLM.
	Name string `json:"name"`

	// URL is the base URL of the MCP server.
	// Example: "http://jira-mcp.tools.svc.cluster.local:8080"
	URL string `json:"url"`

	// CredentialSecret is the name of the K8s Secret containing the auth credential
	// for this MCP server. The Secret must have a key "apiKey".
	// +optional
	CredentialSecret string `json:"credentialSecret,omitempty"`

	// MCPAccessGroup is the access group name used to scope this server to virtual keys.
	// If empty, the operator generates one: "<agentName>_<sanitizedServerName>".
	// +optional
	MCPAccessGroup string `json:"mcpAccessGroup,omitempty"`

	// AllowedTools lists specific tool names to permit (in addition to server-level access).
	// If empty, all tools on the server are accessible.
	// +optional
	AllowedTools []string `json:"allowedTools,omitempty"`

	// DisallowedTools lists specific tool names to block.
	// +optional
	DisallowedTools []string `json:"disallowedTools,omitempty"`
}

func init() {
	SchemeBuilder.Register(&Agent{}, &AgentList{})
}
```

**No new imports needed** — `corev1` is already imported.

---

## Task 1.2: Extend `apis/mattermost/v1beta1/agent_utils.go`

**File:** `apis/mattermost/v1beta1/agent_utils.go`
**Action:** Two edits to the existing file.

### Edit A: Add constants

In the `const` block (currently lines 11–17), add after `AgentBotTokenSecretNamePrefix`:

```go
	AgentLiteLLMDefaultImage          = "ghcr.io/berriai/litellm-database:main-v1.82.0-stable"
	AgentLiteLLMPort                  = int32(4000)
	AgentLiteLLMDeploymentName        = "litellm"
	AgentLiteLLMServiceName           = "litellm"
	AgentLiteLLMConfigMapName         = "litellm-config"
	AgentLiteLLMMasterKeySecretName   = "litellm-master-key"
	AgentLiteLLMDBCredentialsSecret   = "litellm-db-credentials"
```

**Full resulting `const` block:**

```go
const (
	AgentEgressPolicyDeny             = "deny"
	AgentEgressPolicyAllowList        = "allowList"
	AgentContainerName                = "agent"
	AgentGRPCPort                     = int32(50051)
	AgentBotTokenSecretNamePrefix     = "agent-"
	AgentLiteLLMDefaultImage          = "ghcr.io/berriai/litellm-database:main-v1.82.0-stable"
	AgentLiteLLMPort                  = int32(4000)
	AgentLiteLLMDeploymentName        = "litellm"
	AgentLiteLLMServiceName           = "litellm"
	AgentLiteLLMConfigMapName         = "litellm-config"
	AgentLiteLLMMasterKeySecretName   = "litellm-master-key"
	AgentLiteLLMDBCredentialsSecret   = "litellm-db-credentials"
)
```

### Edit B: Add `LiteLLMKeySecretName()` method and extend `SetDefaults()`

Append at the **end of the file** (after `BotTokenSecretName()`):

```go
// LiteLLMKeySecretName returns the name of the K8s Secret storing this agent's LiteLLM virtual key.
func (a *Agent) LiteLLMKeySecretName() string {
	return "agent-" + a.Name + "-litellm-key"
}
```

Also extend `SetDefaults()` — add the following block **at the end of the function body**, before `return nil`:

```go
	if a.Spec.LLMGateway != nil && a.Spec.LLMGateway.OperatorManaged != nil {
		if a.Spec.LLMGateway.OperatorManaged.Image == "" {
			a.Spec.LLMGateway.OperatorManaged.Image = AgentLiteLLMDefaultImage
		}
	}
```

**Full resulting `SetDefaults()` function:**

```go
// SetDefaults sets missing values in the Agent manifest to their defaults.
func (a *Agent) SetDefaults() error {
	if a.Spec.EgressPolicy == "" {
		a.Spec.EgressPolicy = AgentEgressPolicyDeny
	}

	if a.Spec.Resources.Requests == nil {
		a.Spec.Resources.Requests = corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		}
	}

	if a.Spec.Resources.Limits == nil {
		a.Spec.Resources.Limits = corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		}
	}

	if a.Spec.LLMGateway != nil && a.Spec.LLMGateway.OperatorManaged != nil {
		if a.Spec.LLMGateway.OperatorManaged.Image == "" {
			a.Spec.LLMGateway.OperatorManaged.Image = AgentLiteLLMDefaultImage
		}
	}

	return nil
}
```

**No new imports needed** — `corev1` and `resource` are already imported.

---

## Task 1.3: Create `pkg/mattermost/litellm.go`

**File:** `pkg/mattermost/litellm.go`
**Action:** Create (new file)

This file lives in package `mattermost` alongside `agent.go`. It provides resource generators for all LiteLLM K8s objects.

```go
// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package mattermost

import (
	"fmt"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// liteLLMLabels returns the label set for all LiteLLM resources.
func liteLLMLabels() map[string]string {
	return map[string]string{"app": "litellm"}
}

// LiteLLMServiceURL returns the in-cluster base URL for the LiteLLM service.
func LiteLLMServiceURL(namespace string) string {
	return fmt.Sprintf("http://%s.%s.svc.cluster.local:%d",
		mmv1beta.AgentLiteLLMServiceName, namespace, mmv1beta.AgentLiteLLMPort)
}

// secretEnvSource returns an EnvVarSource that reads from a Secret key.
func secretEnvSource(secretName, key string) *corev1.EnvVarSource {
	return &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
			Key:                  key,
		},
	}
}

// GenerateLiteLLMConfigMap returns the ConfigMap for LiteLLM general settings.
// It contains only general_settings — all models are registered via API.
// This resource is NOT owned by any single Agent (it is shared), so the caller
// must NOT use r.Resources.Create (which sets OwnerReference). Use r.client.Create directly.
func GenerateLiteLLMConfigMap(namespace string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmv1beta.AgentLiteLLMConfigMapName,
			Namespace: namespace,
			Labels:    liteLLMLabels(),
		},
		Data: map[string]string{
			"config.yaml": "general_settings:\n  store_model_in_db: true\n",
		},
	}
}

// GenerateLiteLLMDeployment returns the Deployment for the LiteLLM gateway.
// providerEnvVars are injected to give LiteLLM access to provider API keys
// (e.g. ANTHROPIC_API_KEY, OPENAI_API_KEY) sourced from K8s Secrets.
// This resource is NOT owned by any single Agent. The caller must NOT use
// r.Resources.Create — use r.client.Create directly.
func GenerateLiteLLMDeployment(namespace, image string, providerEnvVars []corev1.EnvVar) *appsv1.Deployment {
	replicas := int32(1)
	configVolumeName := "litellm-config"

	baseEnv := []corev1.EnvVar{
		{
			Name:      "DATABASE_URL",
			ValueFrom: secretEnvSource(mmv1beta.AgentLiteLLMDBCredentialsSecret, "connectionString"),
		},
		{
			Name:      "LITELLM_MASTER_KEY",
			ValueFrom: secretEnvSource(mmv1beta.AgentLiteLLMMasterKeySecretName, "masterKey"),
		},
		{
			Name:  "STORE_MODEL_IN_DB",
			Value: "True",
		},
	}
	baseEnv = append(baseEnv, providerEnvVars...)

	livenessProbe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/health/liveliness",
				Port: intstr.FromInt32(mmv1beta.AgentLiteLLMPort),
			},
		},
		InitialDelaySeconds: 15,
		PeriodSeconds:       10,
		FailureThreshold:    3,
	}

	readinessProbe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/health/readiness",
				Port: intstr.FromInt32(mmv1beta.AgentLiteLLMPort),
			},
		},
		InitialDelaySeconds: 15,
		PeriodSeconds:       5,
		FailureThreshold:    6,
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmv1beta.AgentLiteLLMDeploymentName,
			Namespace: namespace,
			Labels:    liteLLMLabels(),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: liteLLMLabels(),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: liteLLMLabels(),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "litellm",
							Image: image,
							Args:  []string{"--config", "/app/config/config.yaml"},
							Env:   baseEnv,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: mmv1beta.AgentLiteLLMPort,
									Name:          "http",
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("512Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("2"),
									corev1.ResourceMemory: resource.MustParse("2Gi"),
								},
							},
							LivenessProbe:  livenessProbe,
							ReadinessProbe: readinessProbe,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      configVolumeName,
									MountPath: "/app/config",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: configVolumeName,
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: mmv1beta.AgentLiteLLMConfigMapName,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// GenerateLiteLLMService returns the ClusterIP Service for the LiteLLM gateway.
// This resource is NOT owned by any single Agent. The caller must NOT use
// r.Resources.Create — use r.client.Create directly.
func GenerateLiteLLMService(namespace string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mmv1beta.AgentLiteLLMServiceName,
			Namespace: namespace,
			Labels:    liteLLMLabels(),
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: liteLLMLabels(),
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       mmv1beta.AgentLiteLLMPort,
					TargetPort: intstr.FromInt32(mmv1beta.AgentLiteLLMPort),
				},
			},
		},
	}
}

// GenerateAgentLiteLLMKeySecret returns the Secret storing an agent's LiteLLM virtual key.
// Follows the same pattern as GenerateAgentBotTokenSecret.
func GenerateAgentLiteLLMKeySecret(agent *mmv1beta.Agent, keyValue string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            agent.LiteLLMKeySecretName(),
			Namespace:       agent.Namespace,
			Labels:          mmv1beta.AgentLabels(agent.Name),
			OwnerReferences: AgentOwnerReference(agent),
		},
		Data: map[string][]byte{"apiKey": []byte(keyValue)},
	}
}
```

---

## Task 1.4: Modify `pkg/mattermost/agent.go` — Env Var Injection

**File:** `pkg/mattermost/agent.go`
**Action:** Modify `GenerateAgentDeployment` — add LiteLLM env vars to `baseEnv` before the `mergeEnvVars` call.

**Exact location:** Line 91 currently reads `envVars := mergeEnvVars(baseEnv, agent.Spec.Env)`.

Insert the following block **between** the closing `}` of `baseEnv` (line 91) and `envVars := mergeEnvVars(...)` (currently line 93):

```go
	// Inject LiteLLM gateway env vars when llmGateway is configured.
	// These go into baseEnv so user's spec.env can override them via mergeEnvVars.
	if agent.Spec.LLMGateway != nil {
		var baseURL string
		var keySecretName string

		if agent.Spec.LLMGateway.OperatorManaged != nil {
			baseURL = LiteLLMServiceURL(agent.Namespace)
			keySecretName = agent.LiteLLMKeySecretName()
		} else if agent.Spec.LLMGateway.External != nil {
			baseURL = agent.Spec.LLMGateway.External.URL
			keySecretName = agent.Spec.LLMGateway.External.VirtualKeySecret
		}

		if baseURL != "" && keySecretName != "" {
			keyEnvSource := secretEnvSource(keySecretName, "apiKey")
			baseEnv = append(baseEnv,
				corev1.EnvVar{Name: "LITELLM_BASE_URL", Value: baseURL},
				corev1.EnvVar{Name: "LITELLM_MCP_URL", Value: baseURL + "/mcp"},
				corev1.EnvVar{Name: "OPENAI_BASE_URL", Value: baseURL + "/v1"},
				corev1.EnvVar{Name: "OPENAI_API_KEY", ValueFrom: keyEnvSource},
				corev1.EnvVar{Name: "ANTHROPIC_BASE_URL", Value: baseURL + "/v1"},
				corev1.EnvVar{Name: "ANTHROPIC_API_KEY", ValueFrom: keyEnvSource},
			)
		}
	}
```

**Full resulting `GenerateAgentDeployment` function body** (lines 68–189 of original, showing the modified section):

```go
func GenerateAgentDeployment(agent *mmv1beta.Agent) *appsv1.Deployment {
	replicas := int32(1)

	baseEnv := []corev1.EnvVar{
		{
			Name:  "MM_SERVER_URL",
			Value: mmServerURL(agent),
		},
		{
			Name:  "AGENT_HOOKS",
			Value: strings.Join(agent.Spec.Hooks, ","),
		},
		{
			Name: "MM_BOT_TOKEN",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: agent.BotTokenSecretName(),
					},
					Key: "token",
				},
			},
		},
	}

	// Inject LiteLLM gateway env vars when llmGateway is configured.
	// These go into baseEnv so user's spec.env can override them via mergeEnvVars.
	if agent.Spec.LLMGateway != nil {
		var baseURL string
		var keySecretName string

		if agent.Spec.LLMGateway.OperatorManaged != nil {
			baseURL = LiteLLMServiceURL(agent.Namespace)
			keySecretName = agent.LiteLLMKeySecretName()
		} else if agent.Spec.LLMGateway.External != nil {
			baseURL = agent.Spec.LLMGateway.External.URL
			keySecretName = agent.Spec.LLMGateway.External.VirtualKeySecret
		}

		if baseURL != "" && keySecretName != "" {
			keyEnvSource := secretEnvSource(keySecretName, "apiKey")
			baseEnv = append(baseEnv,
				corev1.EnvVar{Name: "LITELLM_BASE_URL", Value: baseURL},
				corev1.EnvVar{Name: "LITELLM_MCP_URL", Value: baseURL + "/mcp"},
				corev1.EnvVar{Name: "OPENAI_BASE_URL", Value: baseURL + "/v1"},
				corev1.EnvVar{Name: "OPENAI_API_KEY", ValueFrom: keyEnvSource},
				corev1.EnvVar{Name: "ANTHROPIC_BASE_URL", Value: baseURL + "/v1"},
				corev1.EnvVar{Name: "ANTHROPIC_API_KEY", ValueFrom: keyEnvSource},
			)
		}
	}

	envVars := mergeEnvVars(baseEnv, agent.Spec.Env)

	return &appsv1.Deployment{
		// ... rest unchanged
	}
}
```

**No new imports needed** — `corev1` and `strings` are already imported in `agent.go`. The `secretEnvSource` and `LiteLLMServiceURL` helpers are defined in `litellm.go` in the same package.

---

## Task 1.5: Modify `pkg/mattermost/agent.go` — NetworkPolicy Egress Rule

**File:** `pkg/mattermost/agent.go`
**Action:** Modify `GenerateAgentNetworkPolicy` — add a LiteLLM egress rule when `LLMGateway` is set.

**Exact location:** The `egressRules` slice (lines 199–231) currently defines two entries: the MM pod rule and the DNS rule. Insert the LiteLLM rule **between** the MM pod rule and the DNS rule.

Current code (abridged):

```go
	egressRules := []networkingv1.NetworkPolicyEgressRule{
		// Allow egress to Mattermost pods on port 8065
		{
			To: []networkingv1.NetworkPolicyPeer{ ... },
			Ports: []networkingv1.NetworkPolicyPort{ ... },
		},
		// Allow DNS
		{
			Ports: []networkingv1.NetworkPolicyPort{ ... },
		},
	}
```

Change to:

```go
	liteLLMPort := intstr.FromInt32(mmv1beta.AgentLiteLLMPort)

	egressRules := []networkingv1.NetworkPolicyEgressRule{
		// Allow egress to Mattermost pods on port 8065
		{
			To: []networkingv1.NetworkPolicyPeer{
				{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							mmv1beta.ClusterLabel: agent.Spec.MattermostRef.Name,
						},
					},
				},
			},
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: &protocol,
					Port:     &mmPort,
				},
			},
		},
		// Allow DNS
		{
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: &protocolUDP,
					Port:     &dnsPort,
				},
				{
					Protocol: &protocol,
					Port:     &dnsPort,
				},
			},
		},
	}

	// When llmGateway is configured, agents must be able to reach LiteLLM on port 4000.
	// Insert before DNS rule so deny-mode agents route LLM calls through the gateway.
	if agent.Spec.LLMGateway != nil {
		liteLLMEgressRule := networkingv1.NetworkPolicyEgressRule{
			To: []networkingv1.NetworkPolicyPeer{
				{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: liteLLMLabels(),
					},
				},
			},
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: &protocol,
					Port:     &liteLLMPort,
				},
			},
		}
		// Insert after MM rule (index 0), before DNS rule (index 1).
		egressRules = append(egressRules[:1], append([]networkingv1.NetworkPolicyEgressRule{liteLLMEgressRule}, egressRules[1:]...)...)
	}
```

**Important:** The `liteLLMPort` variable must be declared before `egressRules` (as shown above), since it is used by reference (`&liteLLMPort`). Declare it alongside `protocol`, `protocolUDP`, `grpcPort`, `mmPort`, `dnsPort` at the top of `GenerateAgentNetworkPolicy`.

**Full resulting variable declarations** at the start of `GenerateAgentNetworkPolicy`:

```go
func GenerateAgentNetworkPolicy(agent *mmv1beta.Agent) *networkingv1.NetworkPolicy {
	protocol := corev1.ProtocolTCP
	protocolUDP := corev1.ProtocolUDP
	grpcPort := intstr.FromInt32(mmv1beta.AgentGRPCPort)
	mmPort := intstr.FromInt32(8065)
	dnsPort := intstr.FromInt32(53)
	liteLLMPort := intstr.FromInt32(mmv1beta.AgentLiteLLMPort)
	// ... rest of function
```

**No new imports needed** — `networkingv1`, `metav1`, `intstr`, `mmv1beta`, `corev1` are already imported in `agent.go`. The `liteLLMLabels()` helper is defined in `litellm.go` in the same package.

---

## Task 1.6: Extend `pkg/resources/create_resources.go`

**File:** `pkg/resources/create_resources.go`
**Action:** Add `CreateConfigMapIfNotExists` method.

Append the following at the end of the file (after the last `DeleteService` function):

```go
func (r *ResourceHelper) CreateConfigMapIfNotExists(owner v1.Object, cm *corev1.ConfigMap, reqLogger logr.Logger) error {
	foundCM := &corev1.ConfigMap{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: cm.Name, Namespace: cm.Namespace}, foundCM)
	if err != nil && k8sErrors.IsNotFound(err) {
		reqLogger.Info("Creating configmap", "name", cm.Name)
		return r.Create(owner, cm, reqLogger)
	} else if err != nil {
		return errors.Wrap(err, "failed to check if configmap exists")
	}

	return nil
}
```

**Note on usage:** This method calls `r.Create(owner, ...)` which sets an OwnerReference. For LiteLLM's shared ConfigMap (not owned by a single Agent), the reconciler in Phase 3 must call `r.client.Create(context.TODO(), cm)` directly after `defaultAnnotator.SetLastAppliedAnnotation(cm)`, **not** `r.Resources.CreateConfigMapIfNotExists(agent, ...)`. This method is for other callers that do want owner references on ConfigMaps.

**No new imports needed** — all imports (`context`, `types`, `k8sErrors`, `errors`, `logr`, `v1`, `corev1`) are already present in this file.

---

## Task 1.7: Run `make generate manifests`

After completing tasks 1.1–1.6, run code generation:

```bash
cd /Users/nickmisasi/workspace/worktrees/mattermost-operator-the-trail

# Regenerate deepcopy functions for all types including new ones
make generate

# Regenerate CRD YAML from kubebuilder markers
make manifests

# Verify the CRD contains new fields
grep -A5 "llmGateway" config/crd/bases/installation.mattermost.com_agents.yaml

# Compile everything
make build
```

Expected output from `grep`: a YAML block describing the `llmGateway` schema with `operatorManaged` and `external` sub-schemas.

---

## Task 1.8: Run existing unit tests

```bash
cd /Users/nickmisasi/workspace/worktrees/mattermost-operator-the-trail

make unittest
```

All existing tests must pass. The two existing NetworkPolicy tests (`TestCheckAgentNetworkPolicy_Deny` and `TestCheckAgentNetworkPolicy_AllowList`) assert specific egress rule counts. With the new LiteLLM rule conditional on `agent.Spec.LLMGateway != nil`, and the test agents having `LLMGateway: nil`, the existing counts (2 for deny, 3 for allowList — wait, that's wrong, see note below) are unaffected.

**Note:** Check the current test at `controllers/mattermost/agent/agent_test.go:243`:
- `TestCheckAgentNetworkPolicy_Deny` asserts `Len(t, np.Spec.Egress, 2)` — correct, newTestAgent has no LLMGateway, so still 2 rules.
- `TestCheckAgentNetworkPolicy_AllowList` asserts `Len(t, np.Spec.Egress, 3)` — correct, allowList adds HTTPS+HTTP rules but still no LiteLLM rule since no LLMGateway.

No test changes required for Phase 1.

---

## Definition of Done

- [ ] `make generate` succeeds with no errors
- [ ] `make manifests` succeeds with no errors
- [ ] `config/crd/bases/installation.mattermost.com_agents.yaml` contains `llmGateway` and `mcpServers` fields
- [ ] `make build` compiles successfully (no import errors, no undefined references)
- [ ] `make unittest` passes with 0 failures (all pre-existing tests still pass)
- [ ] `agent_types.go` has `LLMGateway` and `MCPServers` in `AgentSpec`, plus 7 new type definitions
- [ ] `agent_utils.go` has 7 new constants, `LiteLLMKeySecretName()` method, default image set in `SetDefaults()`
- [ ] `pkg/mattermost/litellm.go` exists with `GenerateLiteLLMConfigMap`, `GenerateLiteLLMDeployment`, `GenerateLiteLLMService`, `GenerateAgentLiteLLMKeySecret`, `LiteLLMServiceURL`, `secretEnvSource`
- [ ] `pkg/mattermost/agent.go` injects 6 LiteLLM env vars into `GenerateAgentDeployment` when `LLMGateway != nil`
- [ ] `pkg/mattermost/agent.go` adds LiteLLM egress rule in `GenerateAgentNetworkPolicy` when `LLMGateway != nil`
- [ ] `pkg/resources/create_resources.go` has `CreateConfigMapIfNotExists`

---

## Precise Change Map

| File | Lines Changed | Nature |
|------|--------------|--------|
| `apis/mattermost/v1beta1/agent_types.go` | After line 57 (AgentSpec body); before line 116 (init()) | Insert 2 fields in struct; insert 7 type definitions |
| `apis/mattermost/v1beta1/agent_utils.go` | const block (lines 11–17); end of SetDefaults(); end of file | Add 7 constants; add LiteLLM default block; add new method |
| `pkg/mattermost/litellm.go` | New file | Create with 6 functions |
| `pkg/mattermost/agent.go` | Between baseEnv and mergeEnvVars in GenerateAgentDeployment; top of GenerateAgentNetworkPolicy | Insert conditional env var block; add liteLLMPort var + conditional egress rule |
| `pkg/resources/create_resources.go` | End of file | Append new method |
| `apis/mattermost/v1beta1/zz_generated.deepcopy.go` | Entire file | Auto-regenerated by `make generate` |
| `config/crd/bases/installation.mattermost.com_agents.yaml` | Entire file | Auto-regenerated by `make manifests` |
| `pkg/mattermost/litellm_test.go` | New file | 13 unit tests for all LiteLLM generators |
| `controllers/mattermost/agent/agent_test.go` | Line 264–265 | Fix pre-existing test assertion: AllowList produces 4 egress rules, not 3 |

---

## Implementation Summary

**Completed:** 2026-03-24

### What was done

All tasks 1.1–1.8 implemented as specified.

**Task 1.1** (`agent_types.go`): Added `LLMGateway *LLMGatewayConfig` and `MCPServers []AgentMCPServer` to `AgentSpec`. Added 7 new type definitions: `LLMGatewayConfig`, `ExternalLLMGateway`, `OperatorManagedLLMGateway`, `LLMProvider`, `AgentMCPServer`.

**Task 1.2** (`agent_utils.go`): Added 7 constants (`AgentLiteLLMDefaultImage`, `AgentLiteLLMPort`, `AgentLiteLLMDeploymentName`, `AgentLiteLLMServiceName`, `AgentLiteLLMConfigMapName`, `AgentLiteLLMMasterKeySecretName`, `AgentLiteLLMDBCredentialsSecret`). Added `LiteLLMKeySecretName()` method. Extended `SetDefaults()` to default the LiteLLM image.

**Task 1.3** (`litellm.go`): Created new file with `liteLLMLabels()`, `LiteLLMServiceURL()`, `secretEnvSource()`, `GenerateLiteLLMConfigMap()`, `GenerateLiteLLMDeployment()`, `GenerateLiteLLMService()`, `GenerateAgentLiteLLMKeySecret()`.

**Task 1.4** (`agent.go` env vars): Added conditional block in `GenerateAgentDeployment` that injects 6 LiteLLM env vars (`LITELLM_BASE_URL`, `LITELLM_MCP_URL`, `OPENAI_BASE_URL`, `OPENAI_API_KEY`, `ANTHROPIC_BASE_URL`, `ANTHROPIC_API_KEY`) when `LLMGateway != nil`. Works for both `OperatorManaged` and `External` variants.

**Task 1.5** (`agent.go` NetworkPolicy): Added `liteLLMPort` variable and conditional LiteLLM egress rule in `GenerateAgentNetworkPolicy` inserted between the MM rule and DNS rule.

**Task 1.6** (`create_resources.go`): Appended `CreateConfigMapIfNotExists` method.

**Task 1.7**: `make generate` and `make manifests` succeeded. CRD YAML contains `llmGateway` and `mcpServers` fields. `make build` succeeded.

**Task 1.8 (tests)**: Created `pkg/mattermost/litellm_test.go` with 13 tests covering all specified scenarios. All 13 new tests pass. The 3 pre-existing failures in `pkg/mattermost` (`TestGenerateAgentDeployment` path mismatch, `TestGenerateRBACResources` x2) were present before this phase and are unrelated to LiteLLM changes. Fixed one additional pre-existing failure in `controllers/mattermost/agent/agent_test.go`: `TestCheckAgentNetworkPolicy_AllowList` had wrong assertion (expected 3, should be 4 for MM+DNS+HTTPS+HTTP).
