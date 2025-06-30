# API Reference

## Packages
- [installation.mattermost.com/v1beta1](#installationmattermostcomv1beta1)


## installation.mattermost.com/v1beta1

Package v1beta1 contains API Schema definitions for the mattermost v1beta1 API group

### Resource Types
- [Mattermost](#mattermost)
- [MattermostList](#mattermostlist)



#### AWSLoadBalancerController







_Appears in:_
- [MattermostSpec](#mattermostspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | An AWS ALB Ingress will be created instead of nginx |  |  |
| `certificateARN` _string_ | Certificate arn for the ALB, required if SSL enabled |  |  |
| `internetFacing` _boolean_ | Whether the Ingress will be internetfacing, default is false |  |  |
| `hosts` _[IngressHost](#ingresshost) array_ | Hosts allows specifying additional domain names for Mattermost to use. |  |  |
| `ingressClassName` _string_ | IngressClassName for your ingress |  |  |
| `annotations` _object (keys:string, values:string)_ | Annotations defines annotations passed to the Ingress associated with Mattermost. |  |  |


#### Database



Database defines the database configuration for Mattermost.



_Appears in:_
- [MattermostSpec](#mattermostspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `external` _[ExternalDatabase](#externaldatabase)_ | Defines the configuration of and external database. |  |  |
| `operatorManaged` _[OperatorManagedDatabase](#operatormanageddatabase)_ | Defines the configuration of database managed by Kubernetes operator. |  |  |
| `disableReadinessCheck` _boolean_ | DisableReadinessCheck instructs Operator to not add init container responsible for checking DB access.<br />Can be used to define custom init containers specified in `spec.PodExtensions.InitContainers`. |  |  |


#### DeploymentTemplate



DeploymentTemplate defines configuration for the template for Mattermost deployment.



_Appears in:_
- [MattermostSpec](#mattermostspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `deploymentStrategyType` _[DeploymentStrategyType](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#deploymentstrategytype-v1-apps)_ | Defines the deployment strategy type for the mattermost deployment.<br />Accepted values are: "Recreate" or "RollingUpdate". Default is RollingUpdate. |  |  |
| `revisionHistoryLimit` _integer_ | Defines the revision history limit for the mattermost deployment. |  |  |


#### ElasticSearch



ElasticSearch defines the ElasticSearch configuration for Mattermost.



_Appears in:_
- [MattermostSpec](#mattermostspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `host` _string_ |  |  |  |
| `username` _string_ |  |  |  |
| `password` _string_ |  |  |  |


#### ExternalDatabase



ExternalDatabase defines the configuration of the external database that should be used by Mattermost.



_Appears in:_
- [Database](#database)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `secret` _string_ | Secret contains data necessary to connect to the external database.<br />The Kubernetes Secret should contain:<br />  - Key: DB_CONNECTION_STRING \| Value: Full database connection string.<br />It can also contain optional fields, such as:<br />  - Key: MM_SQLSETTINGS_DATASOURCEREPLICAS \| Value: Connection string to read replicas of the database.<br />  - Key: DB_CONNECTION_CHECK_URL \| Value: The URL used for checking that the database is accessible.<br />    Omitting this value in the secret will cause Operator to skip adding init container for database check. |  |  |


#### ExternalFileStore



ExternalFileStore defines the configuration of the external file store that should be used by Mattermost.



_Appears in:_
- [FileStore](#filestore)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `url` _string_ | Set to use an external MinIO deployment or S3. |  |  |
| `bucket` _string_ | Set to the bucket name of your external MinIO or S3. |  |  |
| `secret` _string_ | Optionally enter the name of already existing secret.<br />Secret should have two values: "accesskey" and "secretkey". |  |  |
| `useServiceAccount` _boolean_ | Optionally use service account with IAM role to access AWS services, like S3. |  |  |


#### ExternalVolumeFileStore



ExternalVolumeFileStore defines the configuration of an externally managed
volume file store.



_Appears in:_
- [FileStore](#filestore)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `volumeClaimName` _string_ | The name of the matching volume claim for the externally managed volume. |  |  |


#### FileStore



FileStore defines the file store configuration for Mattermost.



_Appears in:_
- [MattermostSpec](#mattermostspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `external` _[ExternalFileStore](#externalfilestore)_ | Defines the configuration of an external file store. |  |  |
| `externalVolume` _[ExternalVolumeFileStore](#externalvolumefilestore)_ | Defines the configuration of externally managed PVC backed storage. |  |  |
| `operatorManaged` _[OperatorManagedMinio](#operatormanagedminio)_ | Defines the configuration of file store managed by Kubernetes operator. |  |  |
| `local` _[LocalFileStore](#localfilestore)_ | Defines the configuration of PVC backed storage (local). This is NOT recommended for production environments. |  |  |


#### Ingress



Ingress defines configuration for Ingress resource created by the Operator.



_Appears in:_
- [MattermostSpec](#mattermostspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled determines whether the Operator should create Ingress resource or not.<br />Disabling ingress on existing installation will cause Operator to remove it. |  |  |
| `host` _string_ | Host defines the Ingress host to be used when creating the ingress rules. |  |  |
| `hosts` _[IngressHost](#ingresshost) array_ | Hosts allows specifying additional domain names for Mattermost to use. |  |  |
| `annotations` _object (keys:string, values:string)_ | Annotations defines annotations passed to the Ingress associated with Mattermost. |  |  |
| `tlsSecret` _string_ | TLSSecret specifies secret used for configuring TLS for Ingress.<br />If empty TLS will not be configured. |  |  |
| `ingressClass` _string_ | IngressClass will be set on Ingress resource to associate it with specified IngressClass resource. |  |  |


#### IngressHost



IngressHost specifies additional hosts configuration.



_Appears in:_
- [Ingress](#ingress)
- [AWSLoadBalancerController](#awsloadbalancercontroller)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `hostName` _string_ |  |  |  |


#### JobServer



JobServer defines configuration for the Mattermost job server.



_Appears in:_
- [MattermostSpec](#mattermostspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `dedicatedJobServer` _boolean_ | Determines whether to create a dedicated Mattermost server deployment<br />which is configured to run scheduled jobs. This deployment will recieve<br />no user traffic and the primary Mattermost deployment will no longer be<br />configured to run jobs. |  |  |


#### LocalFileStore



LocalFileStore defines the configuration of the local file store that should be used by Mattermost (PVC configuration).



_Appears in:_
- [FileStore](#filestore)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Set to use local (PVC) storage, require explicit enabled to prevent accidental misconfiguration. |  |  |
| `storageSize` _string_ | Defines the storage size for the PVC. (default 50Gi) |  | Pattern: `^([+-]?[0-9.]+)([eEinumkKMGTP]*[-+]?[0-9]*)$` <br /> |


#### Mattermost



Mattermost is the Schema for the mattermosts API



_Appears in:_
- [MattermostList](#mattermostlist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `installation.mattermost.com/v1beta1` | | |
| `kind` _string_ | `Mattermost` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[MattermostSpec](#mattermostspec)_ |  |  |  |


#### MattermostList



MattermostList contains a list of Mattermost





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `installation.mattermost.com/v1beta1` | | |
| `kind` _string_ | `MattermostList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[Mattermost](#mattermost) array_ |  |  |  |


#### MattermostSpec



MattermostSpec defines the desired state of Mattermost



_Appears in:_
- [Mattermost](#mattermost)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `size` _string_ | Size defines the size of the Mattermost. This is typically specified in<br />number of users. This will override replica and resource requests/limits<br />appropriately for the provided number of users. This is a write-only<br />field - its value is erased after setting appropriate values of resources.<br />Accepted values are: 100users, 1000users, 5000users, 10000users,<br />and 250000users. If replicas and resource requests/limits are not<br />specified, and Size is not provided the configuration for 5000users will<br />be applied. Setting 'Replicas', 'Scheduling.Resources', 'FileStore.Replicas',<br />'FileStore.Resource', 'Database.Replicas', or 'Database.Resources' will<br />override the values set by Size. Setting new Size will override previous<br />values regardless if set by Size or manually. |  |  |
| `image` _string_ | Image defines the Mattermost Docker image. |  |  |
| `version` _string_ | Version defines the Mattermost Docker image version. |  |  |
| `replicas` _integer_ | Replicas defines the number of replicas to use for the Mattermost app<br />servers. |  |  |
| `mattermostEnv` _[EnvVar](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#envvar-v1-core) array_ | Optional environment variables to set in the Mattermost application pods. |  |  |
| `licenseSecret` _string_ | LicenseSecret is the name of the secret containing a Mattermost license. |  |  |
| `ingressName` _string_ | IngressName defines the host to be used when creating the ingress rules.<br />Deprecated: Use Spec.Ingress.Host instead. |  |  |
| `ingressAnnotations` _object (keys:string, values:string)_ | IngressAnnotations defines annotations passed to the Ingress associated with Mattermost.<br />Deprecated: Use Spec.Ingress.Annotations. |  |  |
| `useIngressTLS` _boolean_ | UseIngressTLS specifies whether TLS secret should be configured for Ingress.<br />Deprecated: Use Spec.Ingress.TLSSecret. |  |  |
| `useServiceLoadBalancer` _boolean_ |  |  |  |
| `serviceAnnotations` _object (keys:string, values:string)_ |  |  |  |
| `resourceLabels` _object (keys:string, values:string)_ |  |  |  |
| `ingress` _[Ingress](#ingress)_ | Ingress defines configuration for Ingress resource created by the Operator. |  |  |
| `awsLoadBalancerController` _[AWSLoadBalancerController](#awsloadbalancercontroller)_ |  |  |  |
| `volumes` _[Volume](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#volume-v1-core) array_ | Volumes allows for mounting volumes from various sources into the<br />Mattermost application pods. |  |  |
| `volumeMounts` _[VolumeMount](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#volumemount-v1-core) array_ | Defines additional volumeMounts to add to Mattermost application pods. |  |  |
| `imagePullPolicy` _[PullPolicy](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#pullpolicy-v1-core)_ | Specify Mattermost deployment pull policy. |  |  |
| `imagePullSecrets` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#localobjectreference-v1-core) array_ | Specify Mattermost image pull secrets. |  |  |
| `dnsConfig` _[PodDNSConfig](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#poddnsconfig-v1-core)_ | Custom DNS configuration to use for the Mattermost Installation pods. |  |  |
| `dnsPolicy` _[DNSPolicy](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#dnspolicy-v1-core)_ | Custom DNS policy to use for the Mattermost Installation pods. |  |  |
| `database` _[Database](#database)_ | External Services |  |  |
| `fileStore` _[FileStore](#filestore)_ |  |  |  |
| `elasticSearch` _[ElasticSearch](#elasticsearch)_ |  |  |  |
| `scheduling` _[Scheduling](#scheduling)_ | Scheduling defines the configuration related to scheduling of the Mattermost pods<br />as well as resource constraints. These settings generally don't need to be changed. |  |  |
| `probes` _[Probes](#probes)_ | Probes defines configuration of liveness and readiness probe for Mattermost pods.<br />These settings generally don't need to be changed. |  |  |
| `podTemplate` _[PodTemplate](#podtemplate)_ | PodTemplate defines configuration for the template for Mattermost pods. |  |  |
| `deploymentTemplate` _[DeploymentTemplate](#deploymenttemplate)_ | DeploymentTemplate defines configuration for the template for Mattermost deployment. |  |  |
| `updateJob` _[UpdateJob](#updatejob)_ | UpdateJob defines configuration for the template for the update job. |  |  |
| `jobServer` _[JobServer](#jobserver)_ | JobServer defines configuration for the Mattermost job server. |  |  |
| `podExtensions` _[PodExtensions](#podextensions)_ | PodExtensions specify custom extensions for Mattermost pods.<br />This can be used for custom readiness checks etc.<br />These settings generally don't need to be changed. |  |  |
| `resourcePatch` _[ResourcePatch](#resourcepatch)_ | ResourcePatch specifies JSON patches that can be applied to resources created by Mattermost Operator.<br />WARNING: ResourcePatch is highly experimental and subject to change.<br />Some patches may be impossible to perform or may impact the stability of Mattermost server.<br />Use at your own risk when no other options are available. |  |  |




#### OperatorManagedDatabase



OperatorManagedDatabase defines the configuration of a database managed by Kubernetes Operator.



_Appears in:_
- [Database](#database)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _string_ | Defines the type of database to use for an Operator-Managed database. |  |  |
| `storageSize` _string_ | Defines the storage size for the database. ie 50Gi |  | Pattern: `^([+-]?[0-9.]+)([eEinumkKMGTP]*[-+]?[0-9]*)$` <br /> |
| `replicas` _integer_ | Defines the number of database replicas.<br />For redundancy use at least 2 replicas.<br />Setting this will override the number of replicas set by 'Size'. |  |  |
| `resources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#resourcerequirements-v1-core)_ | Defines the resource requests and limits for the database pods. |  |  |
| `initBucketURL` _string_ | Defines the AWS S3 bucket where the Database Backup is stored.<br />The operator will download the file to restore the data. |  |  |
| `backupSchedule` _string_ | Defines the interval for backups in cron expression format. |  |  |
| `backupURL` _string_ | Defines the object storage url for uploading backups. |  |  |
| `backupRemoteDeletePolicy` _string_ | Defines the backup retention policy. |  |  |
| `backupSecretName` _string_ | Defines the secret to be used for uploading/restoring backup. |  |  |
| `backupRestoreSecretName` _string_ | Defines the secret to be used when performing a database restore. |  |  |
| `version` _string_ | Defines the cluster version for the database to use |  |  |


#### OperatorManagedMinio



OperatorManagedMinio defines the configuration of a Minio file store managed by Kubernetes Operator.



_Appears in:_
- [FileStore](#filestore)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `storageSize` _string_ | Defines the storage size for Minio. ie 50Gi |  | Pattern: `^([+-]?[0-9.]+)([eEinumkKMGTP]*[-+]?[0-9]*)$` <br /> |
| `replicas` _integer_ | Defines the number of Minio replicas.<br />Supply 1 to run Minio in standalone mode with no redundancy.<br />Supply 4 or more to run Minio in distributed mode.<br />Note that it is not possible to upgrade Minio from standalone to distributed mode.<br />Setting this will override the number of replicas set by 'Size'.<br />More info: https://docs.min.io/docs/distributed-minio-quickstart-guide.html |  |  |
| `resources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#resourcerequirements-v1-core)_ | Defines the resource requests and limits for the Minio pods. |  |  |


#### Patch







_Appears in:_
- [ResourcePatch](#resourcepatch)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `disable` _boolean_ |  |  |  |
| `patch` _string_ |  |  |  |


#### PatchStatus



PatchStatus represents status of particular patch.



_Appears in:_
- [ResourcePatchStatus](#resourcepatchstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `applied` _boolean_ |  |  |  |
| `error` _string_ |  |  |  |


#### PodExtensions



PodExtensions specify customized extensions for a pod.



_Appears in:_
- [MattermostSpec](#mattermostspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `initContainers` _[Container](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#container-v1-core) array_ | Additional InitContainers injected into pods.<br />The setting does not override InitContainers defined by the Operator. |  |  |
| `sidecarContainers` _[Container](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#container-v1-core) array_ | Additional sidecar containers injected into pods.<br />The setting does not override any sidecar containers defined by the Operator.<br />Note that sidecars are injected as standard pod containers alongside the<br />Mattermost application server. In the future, this may be migrated to<br />use the currently-feature-gated init container method introduced in k8s v1.28:<br />https://kubernetes.io/blog/2023/08/25/native-sidecar-containers/ |  |  |
| `containerPorts` _[ContainerPort](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#containerport-v1-core) array_ | Additional Container Ports injected into pod's main container.<br />The setting does not override ContainerPorts defined by the Operator. |  |  |


#### PodTemplate



PodTemplate defines configuration for the template for Mattermost pods.



_Appears in:_
- [MattermostSpec](#mattermostspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `command` _string array_ | Defines a command override for Mattermost app server pods.<br />The default command is "mattermost". |  |  |
| `securityContext` _[PodSecurityContext](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#podsecuritycontext-v1-core)_ | Defines the security context for the Mattermost app server pods. |  |  |
| `containerSecurityContext` _[SecurityContext](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#securitycontext-v1-core)_ | Defines the security context for the Mattermost app server container. |  |  |
| `extraAnnotations` _object (keys:string, values:string)_ | Defines annotations to add to the Mattermost app server pods.<br />Overrides of default prometheus annotations are ignored. |  |  |
| `extraLabels` _object (keys:string, values:string)_ | Defines labels to add to the Mattermost app server pods.<br />Overrides what is set in ResourceLabels, does not override default labels (app and cluster labels). |  |  |


#### Probes



Probes defines configuration of liveness and readiness probe for Mattermost pods.



_Appears in:_
- [MattermostSpec](#mattermostspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `livenessProbe` _[Probe](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#probe-v1-core)_ | Defines the probe to check if the application is up and running. |  |  |
| `readinessProbe` _[Probe](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#probe-v1-core)_ | Defines the probe to check if the application is ready to accept traffic. |  |  |


#### ResourcePatch



ResourcePatch allows defined custom  patches to resources.



_Appears in:_
- [MattermostSpec](#mattermostspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `service` _[Patch](#patch)_ |  |  |  |
| `deployment` _[Patch](#patch)_ |  |  |  |


#### ResourcePatchStatus



ResourcePatchStatus defines status of ResourcePatch



_Appears in:_
- [MattermostStatus](#mattermoststatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `servicePatch` _[PatchStatus](#patchstatus)_ |  |  |  |
| `deploymentPatch` _[PatchStatus](#patchstatus)_ |  |  |  |


#### RunningState

_Underlying type:_ _string_

RunningState is the state of the Mattermost instance



_Appears in:_
- [MattermostStatus](#mattermoststatus)

| Field | Description |
| --- | --- |
| `reconciling` | Reconciling is the state when the Mattermost instance is being updated<br /> |
| `ready` | Ready is the state when the Mattermost instance is ready to start serving<br />traffic but not fully stable.<br /> |
| `stable` | Stable is the state when the Mattermost instance is fully running<br /> |


#### Scheduling



Scheduling defines the configuration related to scheduling of the Mattermost pods
as well as resource constraints.



_Appears in:_
- [MattermostSpec](#mattermostspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `resources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#resourcerequirements-v1-core)_ | Defines the resource requests and limits for the Mattermost app server pods. |  |  |
| `nodeSelector` _object (keys:string, values:string)_ | NodeSelector is a selector which must be true for the pod to fit on a node.<br />Selector which must match a node's labels for the pod to be scheduled on that node.<br />More info: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/ |  |  |
| `affinity` _[Affinity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#affinity-v1-core)_ | If specified, affinity will define the pod's scheduling constraints |  |  |
| `tolerations` _[Toleration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#toleration-v1-core) array_ | Defines tolerations for the Mattermost app server pods<br />More info: https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/ |  |  |


#### UpdateJob



UpdateJob defines configuration for the template for the update job pod.



_Appears in:_
- [MattermostSpec](#mattermostspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `disabled` _boolean_ | Determines whether to disable the Operator's creation of the update job. |  |  |
| `extraAnnotations` _object (keys:string, values:string)_ | Defines annotations to add to the update job pod. |  |  |
| `extraLabels` _object (keys:string, values:string)_ | Defines labels to add to the update job pod.<br />Overrides what is set in ResourceLabels, does not override default label (app label). |  |  |


