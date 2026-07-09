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

// AgentMattermostRef references a Mattermost CR by name.
type AgentMattermostRef struct {
	// Name of the Mattermost CR in the same namespace.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// AgentSpec defines the desired state of Agent
// +k8s:openapi-gen=true
type AgentSpec struct {
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

	// MattermostRef is a reference to the Mattermost CR in the same namespace
	// that this agent is associated with.
	MattermostRef AgentMattermostRef `json:"mattermostRef"`

	// Env defines optional environment variables to inject into the agent pod.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`
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

func init() {
	SchemeBuilder.Register(&Agent{}, &AgentList{})
}
