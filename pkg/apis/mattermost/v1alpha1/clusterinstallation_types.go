package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
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

// ClusterInstallationSpec defines the desired state of ClusterInstallation
// +k8s:openapi-gen=true
type ClusterInstallationSpec struct {
	// Image defines the ClusterInstallation Docker image.
	Image string `json:"image,omitempty"`
	// Version defines the ClusterInstallation Docker image version.
	Version string `json:"version,omitempty"`
	// Size defines the size of the ClusterInstallation. This is typically specified in number of users.
	// This will set replica and resource requests/limits appropriately for the provided number of users.
	// Accepted values are: 100users, 1000users, 5000users, 10000users, 250000users. Defaults to 5000users.
	// Setting 'Replicas', 'Resources', 'Minio.Replicas', 'Minio.Resource', 'Database.Replicas',
	// or 'Database.Resources' will override the values set by Size.
	Size string `json:"size,omitempty"`
	// Replicas defines the number of replicas to use for the Mattermost app servers.
	// Setting this will override the number of replicas set by 'Size'.
	// +optional
	Replicas int32 `json:"replicas,omitempty"`
	// Defines the resource requests and limits for the Mattermost app server pods.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
	// IngressName defines the name to be used when creating the ingress rules
	IngressName string `json:"ingressName"`
	// Secret that contains the mattermost license
	// +optional
	MattermostLicenseSecret string `json:"mattermostLicenseSecret,omitempty"`
	// NodeSelector is a selector which must be true for the pod to fit on a node.
	// Selector which must match a node's labels for the pod to be scheduled on that node.
	// More info: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// If specified, affinity will define the pod's scheduling constraints
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	Minio Minio `json:"minio,omitempty"`

	Database Database `json:"database,omitempty"`

	// +optional
	BlueGreen BlueGreen `json:"blueGreen,omitempty"`

	// +optional
	Canary Canary `json:"canary,omitempty"`

	// +optional
	ElasticSearch ElasticSearch `json:"elasticSearch,omitempty"`

	// +optional
	UseServiceLoadBalancer bool `json:"useServiceLoadBalancer,omitempty"`

	// +optional
	ServiceAnnotations map[string]string `json:"serviceAnnotations,omitempty"`

	// +optional
	IngressAnnotations map[string]string `json:"ingressAnnotations,omitempty"`
}

// Canary defines the configuration of Canary deployment for a ClusterInstallation
type Canary struct {
	// Enable defines if a canary build will be deployed.
	// +optional
	Enable bool `json:"enable,omitempty"`
	// Deployment defines the canary deployment.
	// +optional
	Deployment AppDeployment `json:"deployment,omitempty"`
}

// BlueGreen defines the configuration of BlueGreen deployment for a ClusterInstallation
type BlueGreen struct {
	// ProductionDeployment defines if the current production is blue or green.
	// +optional
	ProductionDeployment string `json:"productionDeployment,omitempty"`
	// Enable defines if BlueGreen deployment will be applied.
	// +optional
	Enable bool `json:"enable,omitempty"`
	// Blue defines the blue deployment.
	// +optional
	Blue AppDeployment `json:"blue,omitempty"`
	// Green defines the green deployment.
	// +optional
	Green AppDeployment `json:"green,omitempty"`
}

// AppDeployment defines the configuration of deployment for a ClusterInstallation.
type AppDeployment struct {
	// Name defines the name of the deployment
	// +optional
	Name string `json:"name,omitempty"`
	// IngressName defines the ingress name that will be used by the deployment.
	// This option is not used for Canary builds.
	// +optional
	IngressName string `json:"ingressName,omitempty"`
	// Image defines the base Docker image that will be used for the deployment.
	// Required when BlueGreen or Canary is enabled.
	// +optional
	Image string `json:"image,omitempty"`
	// Version defines the Docker image version that will be used for the deployment.
	// Required when BlueGreen or Canary is enabled.
	// +optional
	Version string `json:"version,omitempty"`
}

// Minio defines the configuration of Minio for a ClusterInstallation.
type Minio struct {
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
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
	// Optionally enter the name of already existing Secret
	// +optional
	Secret string `json:"secret,omitempty"`
}

// Database defines the database configuration for a ClusterInstallation.
type Database struct {
	Type string `json:"type,omitempty"`
	// If the user want to use an external DB.
	// This can be inside the same k8s cluster or outside like AWS RDS.
	// +optional
	ExternalSecret string `json:"externalSecret,omitempty"`
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
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
	// Defines the interval for backups in cron expression format.
	// +optional
	BackupSchedule string `json:"backupSchedule,omitempty"`
	// Defines the object storage url for uploading backups.
	// +optional
	BackupURL string `json:"backupURL,omitempty"`
	// Defines the backup retention policy.
	// +optional
	BackupRemoteDeletePolicy string `json:"backupRemoteDeletePolicy,omitempty"`
	// Defines the secret to be used for uploading backup.
	// +optional
	BackupSecretName string `json:"backupSecretName,omitempty"`
}

// ElasticSearch defines the ElasticSearch configuration for a ClusterInstallation.
type ElasticSearch struct {
	Host string `json:"host,omitempty"`
	// +optional
	UserName string `json:"username,omitempty"`

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

// ClusterInstallationStatus defines the observed state of ClusterInstallation
type ClusterInstallationStatus struct {
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

// ClusterInstallation is the Schema for the clusterinstallations API
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:priority=0,name="State",type=string,JSONPath=".status.state",description="State of Mattermost"
// +kubebuilder:printcolumn:priority=0,name="Image",type=string,JSONPath=".status.image",description="Image of Mattermost"
// +kubebuilder:printcolumn:priority=0,name="Version",type=string,JSONPath=".status.version",description="Version of Mattermost"
// +kubebuilder:printcolumn:priority=0,name="Endpoint",type=string,JSONPath=".status.endpoint",description="Endpoint"
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
	Status ClusterInstallationStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterInstallationList contains a list of ClusterInstallation
type ClusterInstallationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterInstallation `json:"items"`
}
