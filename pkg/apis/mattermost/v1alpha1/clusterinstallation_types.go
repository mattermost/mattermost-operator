package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterInstallationSpec defines the desired state of ClusterInstallation
// +k8s:openapi-gen=true
type ClusterInstallationSpec struct {
	// Image defines the ClusterInstallation Docker image.
	Image string `json:"image,omitempty"`
	// Version defines the ClusterInstallation Docker image version.
	Version string `json:"version,omitempty"`
	// Replicas defines the number of Mattermost instances in a ClusterInstallation resource
	Replicas int32 `json:"replicas,omitempty"`
	// IngressName defines the name to be used when creating the ingress rules
	IngressName string `json:"ingressName"`
	// NodeSelector is a selector which must be true for the pod to fit on a node.
	// Selector which must match a node's labels for the pod to be scheduled on that node.
	// More info: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// If specified, affinity will define the pod's scheduling constraints
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// MinioStorageSize defines the storage size for minio
	MinioStorageSize string `json:"minioStorageSize,omitempty"`

	DatabaseType DatabaseType `json:"databaseType,omitempty"`
}

// DatabaseType defines the Database configuration for a ClusterInstallation
type DatabaseType struct {
	Type string `json:"type,omitempty"`
	// If the user want to use an external DB.
	// This can be inside the same k8s cluster or outside like AWS RDS.
	ExternalDatabaseSecret string `json:"externalDatabaseSecret,omitempty"`
}

// ClusterInstallationStatus defines the observed state of ClusterInstallation
// +k8s:openapi-gen=true
type ClusterInstallationStatus struct {
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book.kubebuilder.io/beyond_basics/generating_crd.html

	// Represents whether any actions on the underlying managed objects are
	// being performed. Only delete actions will be performed.
	Paused bool `json:"paused"`
	// Total number of non-terminated pods targeted by this Mattermost deployment
	// (their labels match the selector).
	Replicas int32 `json:"replicas"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterInstallation is the Schema for the clusterinstallations API
// +k8s:openapi-gen=true
type ClusterInstallation struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object’s metadata. More info:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md#metadata
	// +k8s:openapi-gen=false
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Specification of the desired behavior of the Mattermost cluster. More info:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md#spec-and-status
	Spec ClusterInstallationSpec `json:"spec"`
	// Most recent observed status of the Mattermost cluster. Read-only. Not
	// included when requesting from the apiserver, only from the Mattermost
	// Operator API itself. More info:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md#spec-and-status
	Status *ClusterInstallationStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterInstallationList contains a list of ClusterInstallation
type ClusterInstallationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterInstallation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterInstallation{}, &ClusterInstallationList{})
}
