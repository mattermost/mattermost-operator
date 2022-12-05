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
// Run "make generate manifests" in the root of this repository to regenerate //
// code after modifying this file.                                            //
// Add custom validation using kubebuilder tags:                              //
// https://book.kubebuilder.io/reference/generating-crd.html                  //
////////////////////////////////////////////////////////////////////////////////

// MattermostSpec defines the desired state of Mattermost
// +k8s:openapi-gen=true
type MattermostSpec struct {
	// Size defines the size of the Mattermost. This is typically specified in
	// number of users. This will override replica and resource requests/limits
	// appropriately for the provided number of users. This is a write-only
	// field - its value is erased after setting appropriate values of resources.
	// Accepted values are: 100users, 1000users, 5000users, 10000users,
	// and 250000users. If replicas and resource requests/limits are not
	// specified, and Size is not provided the configuration for 5000users will
	// be applied. Setting 'Replicas', 'Scheduling.Resources', 'FileStore.Replicas',
	// 'FileStore.Resource', 'Database.Replicas', or 'Database.Resources' will
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
	LicenseSecret string `json:"licenseSecret,omitempty"`
	// IngressName defines the host to be used when creating the ingress rules.
	// Deprecated: Use Spec.Ingress.Host instead.
	// +optional
	IngressName string `json:"ingressName"`
	// IngressAnnotations defines annotations passed to the Ingress associated with Mattermost.
	// Deprecated: Use Spec.Ingress.Annotations.
	// +optional
	IngressAnnotations map[string]string `json:"ingressAnnotations,omitempty"`
	// UseIngressTLS specifies whether TLS secret should be configured for Ingress.
	// Deprecated: Use Spec.Ingress.TLSSecret.
	// +optional
	UseIngressTLS bool `json:"useIngressTLS,omitempty"`
	// +optional
	UseServiceLoadBalancer bool `json:"useServiceLoadBalancer,omitempty"`
	// +optional
	ServiceAnnotations map[string]string `json:"serviceAnnotations,omitempty"`
	// +optional
	ResourceLabels map[string]string `json:"resourceLabels,omitempty"`

	// TODO: Before adding Ingress section Operator would always create the Ingress.
	// Therefore to preserve it as a default behavior this field needs to be optional
	// otherwise Ingress.Enabled will default to false.
	// After we drop old Ingress fields in new CR version this no longer needs to be a pointer
	// and we can default to not creating Ingress. We can also then remove the accessor methods.

	// Ingress defines configuration for Ingress resource created by the Operator.
	// +optional
	Ingress *Ingress `json:"ingress,omitempty"`

	// +optional
	AWSLoadBalancerController *AWSLoadBalancerController `json:"awsLoadBalancerController,omitempty"`
	// Volumes allows for mounting volumes from various sources into the
	// Mattermost application pods.
	// +optional
	Volumes []v1.Volume `json:"volumes,omitempty"`
	// Defines additional volumeMounts to add to Mattermost application pods.
	// +optional
	VolumeMounts []v1.VolumeMount `json:"volumeMounts,omitempty"`
	// Specify Mattermost deployment pull policy.
	// +optional
	ImagePullPolicy v1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// Specify Mattermost image pull secrets.
	// +optional
	ImagePullSecrets []v1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// Custom DNS configuration to use for the Mattermost Installation pods.
	// +optional
	DNSConfig *v1.PodDNSConfig `json:"dnsConfig,omitempty"`
	// Custom DNS policy to use for the Mattermost Installation pods.
	// +optional
	DNSPolicy v1.DNSPolicy `json:"dnsPolicy,omitempty"`

	// External Services
	Database      Database      `json:"database,omitempty"`
	FileStore     FileStore     `json:"fileStore,omitempty"`
	ElasticSearch ElasticSearch `json:"elasticSearch,omitempty"`

	// Advanced settings - it is recommended to leave the default configuration
	// for below settings, unless a very specific use case arises.

	// Scheduling defines the configuration related to scheduling of the Mattermost pods
	// as well as resource constraints. These settings generally don't need to be changed.
	// +optional
	Scheduling Scheduling `json:"scheduling,omitempty"`
	// Probes defines configuration of liveness and readiness probe for Mattermost pods.
	// These settings generally don't need to be changed.
	// +optional
	Probes Probes `json:"probes,omitempty"`

	// PodTemplate defines configuration for the template for Mattermost pods.
	// +optional
	PodTemplate *PodTemplate `json:"podTemplate,omitempty"`

	// UpdateJob defines configuration for the template for the update job.
	// +optional
	UpdateJob *UpdateJob `json:"updateJob,omitempty"`

	// PodExtensions specify custom extensions for Mattermost pods.
	// This can be used for custom readiness checks etc.
	// These settings generally don't need to be changed.
	// +optional
	PodExtensions PodExtensions `json:"podExtensions,omitempty"`

	// ResourcePatch specifies JSON patches that can be applied to resources created by Mattermost Operator.
	//
	// WARNING: ResourcePatch is highly experimental and subject to change.
	// Some patches may be impossible to perform or may impact the stability of Mattermost server.
	//
	// Use at your own risk when no other options are available.
	ResourcePatch *ResourcePatch `json:"resourcePatch,omitempty"`
}

// ResourcePatch allows defined custom  patches to resources.
type ResourcePatch struct {
	Service    *Patch `json:"service,omitempty"`
	Deployment *Patch `json:"deployment,omitempty"`
}

type Patch struct {
	Disable bool   `json:"disable,omitempty"`
	Patch   string `json:"patch,omitempty"`
}

// TODO:
// For future extendability we are creating new struct for additional hosts instead of using simple []string.
// Moving forward we might want to drop `Host` field and support only `Hosts` but we cannot break
// compatibility for now.

// Ingress defines configuration for Ingress resource created by the Operator.
type Ingress struct {
	// Enabled determines whether the Operator should create Ingress resource or not.
	// Disabling ingress on existing installation will cause Operator to remove it.
	Enabled bool `json:"enabled"`
	// Host defines the Ingress host to be used when creating the ingress rules.
	// +optional
	Host string `json:"host,omitempty"`

	// Hosts allows specifying additional domain names for Mattermost to use.
	// +optional
	Hosts []IngressHost `json:"hosts,omitempty"`

	// Annotations defines annotations passed to the Ingress associated with Mattermost.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
	// TLSSecret specifies secret used for configuring TLS for Ingress.
	// If empty TLS will not be configured.
	// +optional
	TLSSecret string `json:"tlsSecret,omitempty"`
	// IngressClass will be set on Ingress resource to associate it with specified IngressClass resource.
	// +optional
	IngressClass *string `json:"ingressClass,omitempty"`
}

type AWSLoadBalancerController struct {
	// An AWS ALB Ingress will be created instead of nginx
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Certificate arn for the ALB, required if SSL enabled
	// +optional
	CertificateARN string `json:"certificateARN,omitempty"`

	// Whether the Ingress will be internetfacing, default is false
	// +optional
	InternetFacing bool `json:"internetFacing,omitempty"`

	// Hosts allows specifying additional domain names for Mattermost to use.
	// +optional
	Hosts []IngressHost `json:"hosts,omitempty"`

	// IngressClassName for your ingress
	// +optional
	IngressClassName string `json:"ingressClassName,omitempty"`
}

// IngressHost specifies additional hosts configuration.
type IngressHost struct {
	HostName string `json:"hostName,omitempty"`
}

// Scheduling defines the configuration related to scheduling of the Mattermost pods
// as well as resource constraints.
type Scheduling struct {
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
	// Defines tolerations for the Mattermost app server pods
	// More info: https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/
	// +optional
	Tolerations []v1.Toleration `json:"tolerations,omitempty"`
}

// Probes defines configuration of liveness and readiness probe for Mattermost pods.
type Probes struct {
	// Defines the probe to check if the application is up and running.
	// +optional
	LivenessProbe v1.Probe `json:"livenessProbe,omitempty"`
	// Defines the probe to check if the application is ready to accept traffic.
	// +optional
	ReadinessProbe v1.Probe `json:"readinessProbe,omitempty"`
}

// PodTemplate defines configuration for the template for Mattermost pods.
type PodTemplate struct {
	// Defines the security context for the Mattermost app server pods.
	// +optional
	SecurityContext *v1.PodSecurityContext `json:"securityContext,omitempty"`
	// Defines the security context for the Mattermost app server container.
	// +optional
	ContainerSecurityContext *v1.SecurityContext `json:"containerSecurityContext,omitempty"`
	// Defines annotations to add to the Mattermost app server pods.
	// Overrides of default prometheus annotations are ignored.
	// +optional
	ExtraAnnotations map[string]string `json:"extraAnnotations,omitempty"`
	// Defines labels to add to the Mattermost app server pods.
	// Overrides what is set in ResourceLabels, does not override default labels (app and cluster labels).
	// +optional
	ExtraLabels map[string]string `json:"extraLabels,omitempty"`
}

// UpdateJob defines configuration for the template for the update job pod.
type UpdateJob struct {
	// Determines whether to disable the Operator's creation of the update job.
	// +optional
	Disabled bool `json:"disabled,omitempty"`
	// Defines annotations to add to the update job pod.
	// +optional
	ExtraAnnotations map[string]string `json:"extraAnnotations,omitempty"`
	// Defines labels to add to the update job pod.
	// Overrides what is set in ResourceLabels, does not override default label (app label).
	// +optional
	ExtraLabels map[string]string `json:"extraLabels,omitempty"`
}

// PodExtensions specify customized extensions for a pod.
type PodExtensions struct {
	// Additional InitContainers injected to pods.
	// The setting does not override InitContainers defined by the Operator.
	InitContainers []v1.Container `json:"initContainers,omitempty"`
}

// Database defines the database configuration for Mattermost.
type Database struct {
	// Defines the configuration of and external database.
	// +optional
	External *ExternalDatabase `json:"external,omitempty"`
	// Defines the configuration of database managed by Kubernetes operator.
	// +optional
	OperatorManaged *OperatorManagedDatabase `json:"operatorManaged,omitempty"`

	// DisableReadinessCheck instructs Operator to not add init container responsible for checking DB access.
	// Can be used to define custom init containers specified in `spec.PodExtensions.InitContainers`.
	// +optional
	DisableReadinessCheck bool `json:"disableReadinessCheck,omitempty"`
}

// ExternalDatabase defines the configuration of the external database that should be used by Mattermost.
type ExternalDatabase struct {
	// Secret contains data necessary to connect to the external database.
	// The Kubernetes Secret should contain:
	//   - Key: DB_CONNECTION_STRING | Value: Full database connection string.
	// It can also contain optional fields, such as:
	//   - Key: MM_SQLSETTINGS_DATASOURCEREPLICAS | Value: Connection string to read replicas of the database.
	//   - Key: DB_CONNECTION_CHECK_URL | Value: The URL used for checking that the database is accessible.
	//     Omitting this value in the secret will cause Operator to skip adding init container for database check.
	Secret string `json:"secret,omitempty"`
}

// OperatorManagedDatabase defines the configuration of a database managed by Kubernetes Operator.
type OperatorManagedDatabase struct {
	// Defines the type of database to use for an Operator-Managed database.
	Type string `json:"type,omitempty"`
	// Defines the storage size for the database. ie 50Gi
	// +optional
	// +kubebuilder:validation:Pattern=^([+-]?[0-9.]+)([eEinumkKMGTP]*[-+]?[0-9]*)$
	StorageSize string `json:"storageSize,omitempty"`
	// Defines the number of database replicas.
	// For redundancy use at least 2 replicas.
	// Setting this will override the number of replicas set by 'Size'.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`
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

// FileStore defines the file store configuration for Mattermost.
type FileStore struct {
	// Defines the configuration of an external file store.
	// +optional
	External *ExternalFileStore `json:"external,omitempty"`
	// Defines the configuration of file store managed by Kubernetes operator.
	// +optional
	OperatorManaged *OperatorManagedMinio `json:"operatorManaged,omitempty"`
	// Defines the configuration of PVC backed storage (local). This is NOT recommended for production environments.
	// +optional
	Local *LocalFileStore `json:"local,omitempty"`
}

// ExternalFileStore defines the configuration of the external file store that should be used by Mattermost.
type ExternalFileStore struct {
	// Set to use an external MinIO deployment or S3.
	URL string `json:"url,omitempty"`
	// Set to the bucket name of your external MinIO or S3.
	Bucket string `json:"bucket,omitempty"`
	// Optionally enter the name of already existing secret.
	// Secret should have two values: "accesskey" and "secretkey".
	Secret string `json:"secret,omitempty"`
}

// OperatorManagedMinio defines the configuration of a Minio file store managed by Kubernetes Operator.
type OperatorManagedMinio struct {
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
	Replicas *int32 `json:"replicas,omitempty"`
	// Defines the resource requests and limits for the Minio pods.
	// +optional
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
}

// LocalFileStore defines the configuration of the local file store that should be used by Mattermost (PVC configuration).
type LocalFileStore struct {
	// Set to use local (PVC) storage, require explicit enabled to prevent accidental misconfiguration.
	Enabled bool `json:"enabled"`
	// Defines the storage size for the PVC. (default 50Gi)
	// +optional
	// +kubebuilder:validation:Pattern=^([+-]?[0-9.]+)([eEinumkKMGTP]*[-+]?[0-9]*)$
	StorageSize string `json:"storageSize,omitempty"`
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
	// Ready is the state when the Mattermost instance is ready to start serving
	// traffic but not fully stable.
	Ready RunningState = "ready"
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
	// The last observed Generation of the Mattermost resource that was acted on.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// The last observed error in the deployment of this Mattermost instance
	// +optional
	Error string `json:"error,omitempty"`
	// Status of specified resource patches.
	ResourcePatch *ResourcePatchStatus `json:"resourcePatch,omitempty"`
}

// ResourcePatchStatus defines status of ResourcePatch
type ResourcePatchStatus struct {
	ServicePatch    *PatchStatus `json:"servicePatch,omitempty"`
	DeploymentPatch *PatchStatus `json:"deploymentPatch,omitempty"`
}

// PatchStatus represents status of particular patch.
type PatchStatus struct {
	Applied bool   `json:"applied,omitempty"`
	Error   string `json:"error,omitempty"`
}

// +genclient

// Mattermost is the Schema for the mattermosts API
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName="mm"
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
