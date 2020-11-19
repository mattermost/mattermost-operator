// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package v1beta1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

////////////////////////////////////////////////////////////////////////////////
//                                 IMPORTANT!                                 //
////////////////////////////////////////////////////////////////////////////////
// This is a work in progress and should not be used in production. Use the   //
// v1alpha1 specification instead.                                            //
////////////////////////////////////////////////////////////////////////////////
// Run "make generate manifests" in the root of this repository to regenerate //
// code after modifying this file.                                            //
// Add custom validation using kubebuilder tags:                              //
// https://book.kubebuilder.io/reference/generating-crd.html                  //
////////////////////////////////////////////////////////////////////////////////

// MattermostSpec defines the desired state of Mattermost
type MattermostSpec struct {
	// Size defines the size of the Mattermost. This is typically specified in
	// number of users. This will override replica and resource requests/limits
	// appropriately for the provided number of users. This is a write-only
	// field - its value is erased after setting appropriate values of resources.
	// Accepted values are: 100users, 1000users, 5000users, 10000users,
	// and 250000users. If replicas and resource requests/limits are not
	// specified, and Size is not provided the configuration for 5000users will
	// be applied. Setting 'Replicas', 'Advanced.Resources', 'Filestore.Replicas',
	// 'Filestore.Resource', 'Database.Replicas', or 'Database.Resources' will
	// override the values set by Size. Setting new Size will override previous
	// values regardless if set by Size or manually.
	// +optional
	Size string `json:"size,omitempty"`

	// Image defines the Mattermost Docker image.
	Image string `json:"image,omitempty"`
	// Version defines the Mattermost Docker image version.
	Version string `json:"version,omitempty"`
	// Replicas defines the number of replicas to use for the Mattermost app
	// servers.
	Replicas *int32 `json:"replicas,omitempty"`
	// Optional environment variables to set in the Mattermost application pods.
	// +optional
	MattermostEnv []v1.EnvVar `json:"mattermostEnv,omitempty"`
	// LicenseSecret is the name of the secret containing a Mattermost license.
	// +optional
	LicenseSecret string `json:"mattermostLicenseSecret,omitempty"`
	// IngressName defines the name to be used when creating the ingress rules
	IngressName string `json:"ingressName"`
	// +optional
	IngressAnnotations map[string]string `json:"ingressAnnotations,omitempty"`
	// +optional
	UseIngressTLS bool `json:"useIngressTLS,omitempty"`
	// +optional
	UseServiceLoadBalancer bool `json:"useServiceLoadBalancer,omitempty"`
	// +optional
	ServiceAnnotations map[string]string `json:"serviceAnnotations,omitempty"`
	// +optional
	ResourceLabels map[string]string `json:"resourceLabels,omitempty"`
	// Volumes allows for mounting volumes from various sources into the
	// Mattermost application pods.
	// +optional
	Volumes []v1.Volume `json:"volumes,omitempty"`
	// Defines additional volumeMounts to add to mattermost application pods.
	// +optional
	VolumeMounts []v1.VolumeMount `json:"volumeMounts,omitempty"`

	// Advanced contains additional advanced settings for Mattermost. These
	// settings generally don't need to be changed.
	Advanced Advanced `json:"advanced,omitempty"`

	// External Services
	Database      Database      `json:"database,omitempty"`
	Filestore     Filestore     `json:"filestore,omitempty"`
	ElasticSearch ElasticSearch `json:"elasticSearch,omitempty"`
}

// Advanced defines the advanced configuration for Mattermost. These settings
// should be correct for most types of deployments.
type Advanced struct {
	// Defines the resource requests and limits for the Mattermost app server pods.
	// +optional
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
	// NodeSelector is a selector which must be true for the pod to fit on a node.
	// Selector which must match a node's labels for the pod to be scheduled on that node.
	// More info: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// If specified, affinity will define the pod's scheduling constraints
	// +optional
	Affinity *v1.Affinity `json:"affinity,omitempty"`
	// Defines the probe to check if the application is up and running.
	// +optional
	LivenessProbe v1.Probe `json:"livenessProbe,omitempty"`
	// Defines the probe to check if the application is ready to accept traffic.
	// +optional
	ReadinessProbe v1.Probe `json:"readinessProbe,omitempty"`
}

// Database defines the database configuration for Mattermost.
type Database struct {
	// Defines the type of database to use for an Operator-Managed database. This
	// value is ignored when using a User-Managed database.
	Type string `json:"type,omitempty"`
	// Optionally enter the name of an already-existing Secret for connecting to
	// the database. This secret should be configured as follows:
	//
	// User-Managed Database
	//   - Key: DB_CONNECTION_STRING | Value: <FULL_DATABASE_CONNECTION_STRING>
	// Operator-Managed Database
	//   - Key: ROOT_PASSWORD | Value: <ROOT_DATABASE_PASSWORD>
	//   - Key: USER | Value: <USER_NAME>
	//   - Key: PASSWORD | Value: <USER_PASSWORD>
	//   - Key: DATABASE Value: <DATABASE_NAME>
	//
	// Notes:
	//   If you define all secret values for both User-Managed and
	//   Operator-Managed database types, the User-Managed connection string will
	//   take precedence and the Operator-Managed values will be ignored. If the
	//   secret is left blank, the default behavior is to use an Operator-Managed
	//   database with strong randomly-generated database credentials.
	// +optional
	Secret string `json:"secret,omitempty"`
	// Defines the storage size for the database. ie 50Gi
	// +optional
	// +kubebuilder:validation:Pattern=^([+-]?[0-9.]+)([eEinumkKMGTP]*[-+]?[0-9]*)$
	StorageSize string `json:"storageSize,omitempty"`
	// Defines the number of database replicas.
	// For redundancy use at least 2 replicas.
	// Setting this will override the number of replicas set by 'Size'.
	// +optional
	Replicas int32 `json:"replicas,omitempty"`
	// Defines the resource requests and limits for the database pods.
	// +optional
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
	// Defines the AWS S3 bucket where the Database Backup is stored.
	// The operator will download the file to restore the data.
	// +optional
	InitBucketURL string `json:"initBucketURL,omitempty"`
	// Defines the interval for backups in cron expression format.
	// +optional
	BackupSchedule string `json:"backupSchedule,omitempty"`
	// Defines the object storage url for uploading backups.
	// +optional
	BackupURL string `json:"backupURL,omitempty"`
	// Defines the backup retention policy.
	// +optional
	BackupRemoteDeletePolicy string `json:"backupRemoteDeletePolicy,omitempty"`
	// Defines the secret to be used for uploading/restoring backup.
	// +optional
	BackupSecretName string `json:"backupSecretName,omitempty"`
	// Defines the secret to be used when performing a database restore.
	// +optional
	BackupRestoreSecretName string `json:"backupRestoreSecretName,omitempty"`
}

// Filestore defines the filestore configuration for Mattermost.
type Filestore struct {
	// Defines the storage size for Minio. ie 50Gi
	// +optional
	// +kubebuilder:validation:Pattern=^([+-]?[0-9.]+)([eEinumkKMGTP]*[-+]?[0-9]*)$
	StorageSize string `json:"storageSize,omitempty"`
	// Defines the number of Minio replicas.
	// Supply 1 to run Minio in standalone mode with no redundancy.
	// Supply 4 or more to run Minio in distributed mode.
	// Note that it is not possible to upgrade Minio from standalone to distributed mode.
	// Setting this will override the number of replicas set by 'Size'.
	// More info: https://docs.min.io/docs/distributed-minio-quickstart-guide.html
	// +optional
	Replicas int32 `json:"replicas,omitempty"`
	// Defines the resource requests and limits for the Minio pods.
	// +optional
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
	// Set to use an external MinIO deployment or S3. Must also set 'Secret' and 'ExternalBucket'.
	// +optional
	ExternalURL string `json:"externalURL,omitempty"`
	// Set to the bucket name of your external MinIO or S3.
	// +optional
	ExternalBucket string `json:"externalBucket,omitempty"`
	// Optionally enter the name of already existing secret.
	// Secret should have two values: "accesskey" and "secretkey".
	// Required when "ExternalURL" is set.
	// +optional
	Secret string `json:"secret,omitempty"`
}

// ElasticSearch defines the ElasticSearch configuration for Mattermost.
type ElasticSearch struct {
	Host string `json:"host,omitempty"`
	// +optional
	UserName string `json:"username,omitempty"`
	// +optional
	Password string `json:"password,omitempty"`
}

// RunningState is the state of the Mattermost instance
type RunningState string

// Running States:
// Two types of instance running states are implemented: reconciling and stable.
// If any changes are being made on the mattermost instance, the state will be
// set to reconciling. If the reconcile loop reaches the end without requeuing
// then the state will be set to stable.
const (
	// Reconciling is the state when the Mattermost instance is being updated
	Reconciling RunningState = "reconciling"
	// Stable is the state when the Mattermost instance is fully running
	Stable RunningState = "stable"
)

// MattermostStatus defines the observed state of Mattermost
type MattermostStatus struct {
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

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Mattermost is the Schema for the mattermosts API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:priority=0,name="State",type=string,JSONPath=".status.state",description="State of Mattermost"
// +kubebuilder:printcolumn:priority=0,name="Image",type=string,JSONPath=".status.image",description="Image of Mattermost"
// +kubebuilder:printcolumn:priority=0,name="Version",type=string,JSONPath=".status.version",description="Version of Mattermost"
// +kubebuilder:printcolumn:priority=0,name="Endpoint",type=string,JSONPath=".status.endpoint",description="Endpoint"
type Mattermost struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MattermostSpec   `json:"spec,omitempty"`
	Status MattermostStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MattermostList contains a list of Mattermost
type MattermostList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Mattermost `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Mattermost{}, &MattermostList{})
}
