// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	SchemeBuilder.Register(&MattermostRestoreDB{}, &MattermostRestoreDBList{})
}

// MattermostRestoreDBSpec defines the desired state of MattermostRestoreDB
// +k8s:openapi-gen=true
type MattermostRestoreDBSpec struct {
	// MattermostClusterName defines the ClusterInstallation name.
	MattermostClusterName string `json:"mattermostClusterName,omitempty"`
	// RestoreSecret defines the secret that holds the credentials to
	// MySQL Operator be able to download the DB backup file
	RestoreSecret string `json:"restoreSecret,omitempty"`
	// InitBucketURL defines where the DB backup file is located.
	InitBucketURL string `json:"initBucketURL,omitempty"`
	// MattermostDBUser defines the user to access the database.
	// Need to set if the user is different from `mmuser`.
	// +optional
	MattermostDBUser string `json:"mattermostDBUser,omitempty"`
	// MattermostDBPassword defines the user password to access the database.
	// Need to set if the user is different from the one created by the operator.
	// +optional
	MattermostDBPassword string `json:"mattermostDBPassword,omitempty"`
	// MattermostDBName defines the database name.
	// Need to set if different from `mattermost`.
	// +optional
	MattermostDBName string `json:"mattermostDBName,omitempty"`
}

// RestoreState is the state of the Mattermost Restore Database
type RestoreState string

// Restore States:
// Two types of restore states are implemented: restoring and finished.
const (
	// Restoring is the state when the Mattermost DB is being restored.
	Restoring RestoreState = "restoring"
	// Finished is the state when the Mattermost DB restore process is complete.
	Finished RestoreState = "finished"
	// Failed is the state when the Mattermost DB restore process is failed due a non existing cluster installation.
	Failed RestoreState = "failed"
)

// MattermostRestoreDBStatus defines the observed state of MattermostRestoreDB
// +k8s:openapi-gen=true
type MattermostRestoreDBStatus struct {
	// Represents the state of the Mattermost restore Database.
	// +optional
	State RestoreState `json:"state,omitempty"`
	// The original number of database replicas. will be used to restore after applying the db restore process.
	// +optional
	OriginalDBReplicas int32 `json:"originalDBReplicas,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MattermostRestoreDB is the Schema for the mattermostrestoredbs API
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:priority=0,name="State",type=string,JSONPath=".status.state",description="State of Mattermost DB Restore"
// +kubebuilder:printcolumn:priority=0,name="Original DB Replicas",type=string,JSONPath=".status.originalDBReplicas",description="Original DB Replicas"
type MattermostRestoreDB struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MattermostRestoreDBSpec   `json:"spec,omitempty"`
	Status MattermostRestoreDBStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MattermostRestoreDBList contains a list of MattermostRestoreDB
type MattermostRestoreDBList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MattermostRestoreDB `json:"items"`
}
