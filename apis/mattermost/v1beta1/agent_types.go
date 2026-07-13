// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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

// AgentEgressPolicy controls outbound network access from an agent pod.
type AgentEgressPolicy string

// AgentMattermostRef references a Mattermost CR by name.
type AgentMattermostRef struct {
	// Name of the Mattermost CR in the same namespace.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

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

// AgentSpec defines the desired state of Agent
// +k8s:openapi-gen=true
type AgentSpec struct {
	// MattermostRef is a reference to the Mattermost CR in the same namespace
	// that this agent is associated with.
	MattermostRef AgentMattermostRef `json:"mattermostRef"`

	// Image defines the agent container image.
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`

	// Hooks lists the Mattermost plugin hook names this agent subscribes to.
	// Example: ["MessageHasBeenPosted", "UserHasJoinedChannel"]
	// +optional
	Hooks []string `json:"hooks,omitempty"`

	// Resources defines the CPU/memory requests and limits for the agent pod.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// EgressPolicy controls outbound network access from the agent pod.
	//   - "deny" (default): only Mattermost server, DNS, and LiteLLM gateway
	//   - "allowWeb": additionally permits outbound TCP 80/443 to any destination (port-based; domain-level filtering is future work)
	//   - "allow": permits all outbound traffic
	// +kubebuilder:validation:Enum=deny;allowWeb;allow
	// +kubebuilder:default=deny
	// +optional
	EgressPolicy AgentEgressPolicy `json:"egressPolicy,omitempty"`

	// Env defines optional environment variables to inject into the agent pod.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// LLMGateway configures the LLM gateway for this agent.
	// When OperatorManaged is set, the agent uses the LiteLLM gateway managed
	// by the referenced Mattermost installation. When External is set, the
	// agent uses an existing LiteLLM instance.
	// +optional
	LLMGateway *LLMGatewayConfig `json:"llmGateway,omitempty"`

	// Storage configures optional persistent storage for the agent pod.
	// When set, the operator creates a PVC and mounts it into the agent container.
	// +optional
	Storage *AgentStorageConfig `json:"storage,omitempty"`
}

// AgentStatus defines the observed state of Agent
type AgentStatus struct {
	// State is the current running state of the agent.
	// +optional
	State RunningState `json:"state,omitempty"`

	// Endpoint is the in-cluster HTTP service endpoint for this agent.
	// Format: "http://<name>.<namespace>.svc.cluster.local:8080"
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// ObservedGeneration is the last observed Generation of the Agent resource
	// that was acted on.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Error records the last observed error in the reconciliation of this Agent.
	// +optional
	Error string `json:"error,omitempty"`

	// ReadyReplicas is the number of ready replicas for the agent deployment.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`
}

// LLMGatewayConfig defines how the agent connects to an LLM gateway.
// +kubebuilder:validation:XValidation:rule="has(self.external) != has(self.operatorManaged)",message="exactly one of external or operatorManaged must be set"
type LLMGatewayConfig struct {
	// External configures the agent to use an existing LiteLLM instance.
	// +optional
	External *ExternalLLMGateway `json:"external,omitempty"`

	// OperatorManaged opts this agent into the LiteLLM gateway of the
	// referenced Mattermost installation (spec.agents.llmGateway on the
	// Mattermost CR). The Mattermost agents plugin must create the Secret
	// named "agent-<name>-litellm-key" (key "apiKey") with this agent's
	// virtual key before the agent pod can start.
	// +optional
	OperatorManaged *OperatorManagedGateway `json:"operatorManaged,omitempty"`
}

// ExternalLLMGateway configures the agent to use an externally managed LiteLLM instance.
type ExternalLLMGateway struct {
	// URL is the base URL of the external LiteLLM instance.
	// The agent NetworkPolicy permits TCP egress to this URL's explicit port,
	// or to port 443 for HTTPS and port 80 for HTTP.
	// Example: "http://litellm.my-namespace.svc.cluster.local:4000"
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="isURL(self)",message="must be a valid URL"
	URL string `json:"url"`

	// VirtualKeySecret is the name of the K8s Secret containing the virtual key
	// for this agent. The Secret must have a key "apiKey".
	// +kubebuilder:validation:MinLength=1
	VirtualKeySecret string `json:"virtualKeySecret"`
}

// OperatorManagedGateway is an empty marker (the `emptyDir: {}` idiom) that
// opts an agent into the installation-level LiteLLM gateway. The gateway
// itself is configured and deployed via spec.agents.llmGateway on the
// referenced Mattermost CR.
type OperatorManagedGateway struct{}

// +genclient

// Agent is the Schema for the agents API
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
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

func init() {
	SchemeBuilder.Register(&Agent{}, &AgentList{})
}
