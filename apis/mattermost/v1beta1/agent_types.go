// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

////////////////////////////////////////////////////////////////////////////////
//                                 IMPORTANT!                                 //
////////////////////////////////////////////////////////////////////////////////
// Run "make generate manifests" in the root of this repository to regenerate //
// code after modifying this file.                                            //
// Add custom validation using kubebuilder tags:                              //
// https://book.kubebuilder.io/reference/generating-crd.html                  //
////////////////////////////////////////////////////////////////////////////////

// AgentSpec defines the desired state of Agent
// +k8s:openapi-gen=true
type AgentSpec struct {
	// Image defines the agent container image.
	Image string `json:"image"`

	// Hooks lists the Mattermost plugin hook names this agent subscribes to.
	// Example: ["MessageHasBeenPosted", "UserHasJoinedChannel"]
	// +optional
	Hooks []string `json:"hooks,omitempty"`

	// Resources defines the CPU/memory requests and limits for the agent pod.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// EgressPolicy controls outbound network access from the agent pod.
	// Accepted values are "deny" (default, blocks all egress except Mattermost)
	// and "allowList" (permits additional domains listed in EgressAllowList).
	// +optional
	EgressPolicy string `json:"egressPolicy,omitempty"`

	// EgressAllowList lists additional external domains to permit egress to.
	// Only evaluated when EgressPolicy is "allowList".
	// +optional
	EgressAllowList []string `json:"egressAllowList,omitempty"`

	// MattermostRef is a reference to the Mattermost CR in the same namespace
	// that this agent is associated with.
	MattermostRef corev1.LocalObjectReference `json:"mattermostRef"`

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

// AgentStatus defines the observed state of Agent
type AgentStatus struct {
	// State is the current running state of the agent.
	// +optional
	State RunningState `json:"state,omitempty"`

	// Endpoint is the HTTP service endpoint for this agent.
	// Format: "<name>.<namespace>.svc.cluster.local:8080"
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// BotUserID is the Mattermost user ID of the provisioned bot account.
	// +optional
	BotUserID string `json:"botUserID,omitempty"`

	// BotUsername is the Mattermost username of the provisioned bot account.
	// +optional
	BotUsername string `json:"botUsername,omitempty"`

	// ObservedGeneration is the last observed Generation of the Agent resource
	// that was acted on.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Error records the last observed error in the reconciliation of this Agent.
	// +optional
	Error string `json:"error,omitempty"`

	// Phase is the lifecycle phase of the agent.
	// One of: Provisioning, Deploying, Ready, Error.
	// +optional
	Phase string `json:"phase,omitempty"`

	// Message is a human-readable status message providing additional detail.
	// +optional
	Message string `json:"message,omitempty"`

	// ReadyReplicas is the number of ready replicas for the agent deployment.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`
}

// +genclient

// Agent is the Schema for the agents API
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName="agent"
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:priority=0,name="State",type=string,JSONPath=".status.state",description="State of Agent"
// +kubebuilder:printcolumn:priority=0,name="Image",type=string,JSONPath=".spec.image",description="Image of Agent"
// +kubebuilder:printcolumn:priority=0,name="Endpoint",type=string,JSONPath=".status.endpoint",description="HTTP Endpoint"
type Agent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentSpec   `json:"spec,omitempty"`
	Status AgentStatus `json:"status,omitempty"`
}

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
