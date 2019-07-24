package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

////////////////////////////////////////////////////////////////////////////////
//                                 IMPORTANT!                                 //
////////////////////////////////////////////////////////////////////////////////
// Run "make generate" in the root of this repository to regenerate code      //
// after modifying this file.                                                 //
// Add custom validation using kubebuilder tags:                              //
// https://book.kubebuilder.io/beyond_basics/generating_crd.html              //
////////////////////////////////////////////////////////////////////////////////

// BlueGreenSpec defines the desired state of BlueGreen
// +k8s:openapi-gen=true
type BlueGreenSpec struct {
	// InstallationName defines the ClusterInstallation name that will be usef for the BlueGreen deployment
	InstallationName string `json:"installationName"`
	// InstallationNamespace defines the ClusterInstallation namespace that will be usef for the BlueGreen deployment
	Version string `json:"version,omitempty"`
	// IngressName defines the name to be used when creating the ingress rules
	IngressName string `json:"ingressName"`

	// +optional
	ServiceAnnotations map[string]string `json:"serviceAnnotations,omitempty"`

	// +optional
	IngressAnnotations map[string]string `json:"ingressAnnotations,omitempty"`
}

// BlueGreenStatus defines the observed state of BlueGreen
type BlueGreenStatus struct {
	// Represents the running state of the Mattermost instance
	// +optional
	State RunningState `json:"state,omitempty"`
	// The version currently running in the Mattermost instance
	// +optional
	Version string `json:"version,omitempty"`
	// The image running on the pods in the Mattermost instance
	// +optional
	Image string `json:"image,omitempty"`
	// The endpoint to access the Mattermost instance
	// +optional
	Endpoint string `json:"endpoint,omitempty"`
	// Total number of non-terminated pods targeted by this Mattermost deployment
	// +optional
	Replicas int32 `json:"replicas,omitempty"`
	// Total number of non-terminated pods targeted by this Mattermost deployment
	// that are running with the desired image.
	// +optional
	UpdatedReplicas int32 `json:"updatedReplicas,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BlueGreen is the Schema for the bluegreens API
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:priority=0,name="State",type=string,JSONPath=".status.state",description="State of Mattermost"
// +kubebuilder:printcolumn:priority=0,name="Image",type=string,JSONPath=".status.image",description="Image of Mattermost"
// +kubebuilder:printcolumn:priority=0,name="Version",type=string,JSONPath=".status.version",description="Version of Mattermost"
// +kubebuilder:printcolumn:priority=0,name="Endpoint",type=string,JSONPath=".status.endpoint",description="Endpoint"
type BlueGreen struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object’s metadata. More info:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md#metadata
	// +k8s:openapi-gen=false
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Specification of the desired behavior of the Mattermost cluster. More info:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md#spec-and-status
	Spec BlueGreenSpec `json:"spec"`
	// Most recent observed status of the Mattermost cluster. Read-only. Not
	// included when requesting from the apiserver, only from the Mattermost
	// Operator API itself. More info:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md#spec-and-status
	Status BlueGreenStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BlueGreenList contains a list of BlueGreen
type BlueGreenList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BlueGreen `json:"items"`
}
