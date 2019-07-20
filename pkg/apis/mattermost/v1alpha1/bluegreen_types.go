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
	// IngressName defines the name to be used when creating the ingress rules
	IngressName string `json:"ingressName"`

	// +optional
	UseServiceLoadBalancer bool `json:"useServiceLoadBalancer,omitempty"`

	// +optional
	ServiceAnnotations map[string]string `json:"serviceAnnotations,omitempty"`

	// +optional
	IngressAnnotations map[string]string `json:"ingressAnnotations,omitempty"`
}

// BlueGreenStatus defines the observed state of BlueGreen
// +k8s:openapi-gen=true
type BlueGreenStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BlueGreen is the Schema for the bluegreens API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
type BlueGreen struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BlueGreenSpec   `json:"spec,omitempty"`
	Status BlueGreenStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BlueGreenList contains a list of BlueGreen
type BlueGreenList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BlueGreen `json:"items"`
}
