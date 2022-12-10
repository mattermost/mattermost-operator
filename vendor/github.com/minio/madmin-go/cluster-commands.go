//
// MinIO Object Storage (c) 2021 MinIO, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package madmin

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/minio/minio-go/v7/pkg/replication"
)

// PeerSite - represents a cluster/site to be added to the set of replicated
// sites.
type PeerSite struct {
	Name      string `json:"name"`
	Endpoint  string `json:"endpoints"`
	AccessKey string `json:"accessKey"`
	SecretKey string `json:"secretKey"`
}

// Meaningful values for ReplicateAddStatus.Status
const (
	ReplicateAddStatusSuccess = "Requested sites were configured for replication successfully."
	ReplicateAddStatusPartial = "Some sites could not be configured for replication."
)

// ReplicateAddStatus - returns status of add request.
type ReplicateAddStatus struct {
	Success                 bool   `json:"success"`
	Status                  string `json:"status"`
	ErrDetail               string `json:"errorDetail,omitempty"`
	InitialSyncErrorMessage string `json:"initialSyncErrorMessage,omitempty"`
}

// SiteReplicationAdd - sends the SR add API call.
func (adm *AdminClient) SiteReplicationAdd(ctx context.Context, sites []PeerSite) (ReplicateAddStatus, error) {
	sitesBytes, err := json.Marshal(sites)
	if err != nil {
		return ReplicateAddStatus{}, nil
	}
	encBytes, err := EncryptData(adm.getSecretKey(), sitesBytes)
	if err != nil {
		return ReplicateAddStatus{}, err
	}

	reqData := requestData{
		relPath: adminAPIPrefix + "/site-replication/add",
		content: encBytes,
	}

	resp, err := adm.executeMethod(ctx, http.MethodPut, reqData)
	defer closeResponse(resp)
	if err != nil {
		return ReplicateAddStatus{}, err
	}

	if resp.StatusCode != http.StatusOK {
		return ReplicateAddStatus{}, httpRespToErrorResponse(resp)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return ReplicateAddStatus{}, err
	}

	var res ReplicateAddStatus
	if err = json.Unmarshal(b, &res); err != nil {
		return ReplicateAddStatus{}, err
	}

	return res, nil
}

// SiteReplicationInfo - contains cluster replication information.
type SiteReplicationInfo struct {
	Enabled                 bool       `json:"enabled"`
	Name                    string     `json:"name,omitempty"`
	Sites                   []PeerInfo `json:"sites,omitempty"`
	ServiceAccountAccessKey string     `json:"serviceAccountAccessKey,omitempty"`
}

// SiteReplicationInfo - returns cluster replication information.
func (adm *AdminClient) SiteReplicationInfo(ctx context.Context) (info SiteReplicationInfo, err error) {
	reqData := requestData{
		relPath: adminAPIPrefix + "/site-replication/info",
	}

	resp, err := adm.executeMethod(ctx, http.MethodGet, reqData)
	defer closeResponse(resp)
	if err != nil {
		return info, err
	}

	if resp.StatusCode != http.StatusOK {
		return info, httpRespToErrorResponse(resp)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return info, err
	}

	err = json.Unmarshal(b, &info)
	return info, err
}

// SRPeerJoinReq - arg body for SRPeerJoin
type SRPeerJoinReq struct {
	SvcAcctAccessKey string              `json:"svcAcctAccessKey"`
	SvcAcctSecretKey string              `json:"svcAcctSecretKey"`
	SvcAcctParent    string              `json:"svcAcctParent"`
	Peers            map[string]PeerInfo `json:"peers"`
}

// PeerInfo - contains some properties of a cluster peer.
type PeerInfo struct {
	Endpoint string `json:"endpoint"`
	Name     string `json:"name"`
	// Deployment ID is useful as it is immutable - though endpoint may
	// change.
	DeploymentID string `json:"deploymentID"`
}

// SRPeerJoin - used only by minio server to send SR join requests to peer
// servers.
func (adm *AdminClient) SRPeerJoin(ctx context.Context, r SRPeerJoinReq) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	encBuf, err := EncryptData(adm.getSecretKey(), b)
	if err != nil {
		return err
	}

	reqData := requestData{
		relPath: adminAPIPrefix + "/site-replication/peer/join",
		content: encBuf,
	}

	resp, err := adm.executeMethod(ctx, http.MethodPut, reqData)
	defer closeResponse(resp)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return httpRespToErrorResponse(resp)
	}

	return nil
}

// BktOp represents the bucket operation being requested.
type BktOp string

// BktOp value constants.
const (
	// make bucket and enable versioning
	MakeWithVersioningBktOp BktOp = "make-with-versioning"
	// add replication configuration
	ConfigureReplBktOp BktOp = "configure-replication"
	// delete bucket (forceDelete = off)
	DeleteBucketBktOp BktOp = "delete-bucket"
	// delete bucket (forceDelete = on)
	ForceDeleteBucketBktOp BktOp = "force-delete-bucket"
	// purge bucket
	PurgeDeletedBucketOp BktOp = "purge-deleted-bucket"
)

// SRPeerBucketOps - tells peers to create bucket and setup replication.
func (adm *AdminClient) SRPeerBucketOps(ctx context.Context, bucket string, op BktOp, opts map[string]string) error {
	v := url.Values{}
	v.Add("bucket", bucket)
	v.Add("operation", string(op))

	// For make-bucket, bucket options may be sent via `opts`
	if op == MakeWithVersioningBktOp || op == DeleteBucketBktOp {
		for k, val := range opts {
			v.Add(k, val)
		}
	}
	reqData := requestData{
		queryValues: v,
		relPath:     adminAPIPrefix + "/site-replication/peer/bucket-ops",
	}

	resp, err := adm.executeMethod(ctx, http.MethodPut, reqData)
	defer closeResponse(resp)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return httpRespToErrorResponse(resp)
	}

	return nil
}

// SRIAMItem.Type constants.
const (
	SRIAMItemPolicy        = "policy"
	SRIAMItemSvcAcc        = "service-account"
	SRIAMItemSTSAcc        = "sts-account"
	SRIAMItemPolicyMapping = "policy-mapping"
	SRIAMItemIAMUser       = "iam-user"
	SRIAMItemGroupInfo     = "group-info"
)

// SRSvcAccCreate - create operation
type SRSvcAccCreate struct {
	Parent        string                 `json:"parent"`
	AccessKey     string                 `json:"accessKey"`
	SecretKey     string                 `json:"secretKey"`
	Groups        []string               `json:"groups"`
	Claims        map[string]interface{} `json:"claims"`
	SessionPolicy json.RawMessage        `json:"sessionPolicy"`
	Status        string                 `json:"status"`
}

// SRSvcAccUpdate - update operation
type SRSvcAccUpdate struct {
	AccessKey     string          `json:"accessKey"`
	SecretKey     string          `json:"secretKey"`
	Status        string          `json:"status"`
	SessionPolicy json.RawMessage `json:"sessionPolicy"`
}

// SRSvcAccDelete - delete operation
type SRSvcAccDelete struct {
	AccessKey string `json:"accessKey"`
}

// SRSvcAccChange - sum-type to represent an svc account change.
type SRSvcAccChange struct {
	Create *SRSvcAccCreate `json:"crSvcAccCreate"`
	Update *SRSvcAccUpdate `json:"crSvcAccUpdate"`
	Delete *SRSvcAccDelete `json:"crSvcAccDelete"`
}

// SRPolicyMapping - represents mapping of a policy to a user or group.
type SRPolicyMapping struct {
	UserOrGroup string    `json:"userOrGroup"`
	UserType    int       `json:"userType"`
	IsGroup     bool      `json:"isGroup"`
	Policy      string    `json:"policy"`
	CreatedAt   time.Time `json:"createdAt,omitempty"`
	UpdatedAt   time.Time `json:"updatedAt,omitempty"`
}

// SRSTSCredential - represents an STS credential to be replicated.
type SRSTSCredential struct {
	AccessKey           string `json:"accessKey"`
	SecretKey           string `json:"secretKey"`
	SessionToken        string `json:"sessionToken"`
	ParentUser          string `json:"parentUser"`
	ParentPolicyMapping string `json:"parentPolicyMapping,omitempty"`
}

// SRIAMUser - represents a regular (IAM) user to be replicated. A nil UserReq
// implies that a user delete operation should be replicated on the peer cluster.
type SRIAMUser struct {
	AccessKey   string              `json:"accessKey"`
	IsDeleteReq bool                `json:"isDeleteReq"`
	UserReq     *AddOrUpdateUserReq `json:"userReq"`
}

// SRGroupInfo - represents a regular (IAM) user to be replicated.
type SRGroupInfo struct {
	UpdateReq GroupAddRemove `json:"updateReq"`
}

// SRIAMItem - represents an IAM object that will be copied to a peer.
type SRIAMItem struct {
	Type string `json:"type"`

	// Name and Policy below are used when Type == SRIAMItemPolicy
	Name   string          `json:"name"`
	Policy json.RawMessage `json:"policy"`

	// Used when Type == SRIAMItemPolicyMapping
	PolicyMapping *SRPolicyMapping `json:"policyMapping"`

	// Used when Type == SRIAMItemSvcAcc
	SvcAccChange *SRSvcAccChange `json:"serviceAccountChange"`

	// Used when Type = SRIAMItemSTSAcc
	STSCredential *SRSTSCredential `json:"stsCredential"`

	// Used when Type = SRIAMItemIAMUser
	IAMUser *SRIAMUser `json:"iamUser"`

	// Used when Type = SRIAMItemGroupInfo
	GroupInfo *SRGroupInfo `json:"groupInfo"`

	// UpdatedAt - timestamp of last update
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
}

// SRPeerReplicateIAMItem - copies an IAM object to a peer cluster.
func (adm *AdminClient) SRPeerReplicateIAMItem(ctx context.Context, item SRIAMItem) error {
	b, err := json.Marshal(item)
	if err != nil {
		return err
	}
	reqData := requestData{
		relPath: adminAPIPrefix + "/site-replication/peer/iam-item",
		content: b,
	}

	resp, err := adm.executeMethod(ctx, http.MethodPut, reqData)
	defer closeResponse(resp)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return httpRespToErrorResponse(resp)
	}

	return nil
}

// SRBucketMeta.Type constants
const (
	SRBucketMetaTypePolicy           = "policy"
	SRBucketMetaTypeTags             = "tags"
	SRBucketMetaTypeVersionConfig    = "version-config"
	SRBucketMetaTypeObjectLockConfig = "object-lock-config"
	SRBucketMetaTypeSSEConfig        = "sse-config"
	SRBucketMetaTypeQuotaConfig      = "quota-config"
)

// SRBucketMeta - represents a bucket metadata change that will be copied to a peer.
type SRBucketMeta struct {
	Type   string          `json:"type"`
	Bucket string          `json:"bucket"`
	Policy json.RawMessage `json:"policy,omitempty"`

	// Since Versioning config does not have a json representation, we use
	// xml byte presentation directly.
	Versioning *string `json:"versioningConfig,omitempty"`

	// Since tags does not have a json representation, we use its xml byte
	// representation directly.
	Tags *string `json:"tags,omitempty"`

	// Since object lock does not have a json representation, we use its xml
	// byte representation.
	ObjectLockConfig *string `json:"objectLockConfig,omitempty"`

	// Since SSE config does not have a json representation, we use its xml
	// byte respresentation.
	SSEConfig *string `json:"sseConfig,omitempty"`

	// Quota has a json representation use it as is.
	Quota json.RawMessage `json:"quota,omitempty"`

	// UpdatedAt - timestamp of last update
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
}

// SRPeerReplicateBucketMeta - copies a bucket metadata change to a peer cluster.
func (adm *AdminClient) SRPeerReplicateBucketMeta(ctx context.Context, item SRBucketMeta) error {
	b, err := json.Marshal(item)
	if err != nil {
		return err
	}
	reqData := requestData{
		relPath: adminAPIPrefix + "/site-replication/peer/bucket-meta",
		content: b,
	}

	resp, err := adm.executeMethod(ctx, http.MethodPut, reqData)
	defer closeResponse(resp)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return httpRespToErrorResponse(resp)
	}

	return nil
}

// SRBucketInfo - returns all the bucket metadata available for bucket
type SRBucketInfo struct {
	Bucket string          `json:"bucket"`
	Policy json.RawMessage `json:"policy,omitempty"`

	// Since Versioning config does not have a json representation, we use
	// xml byte presentation directly.
	Versioning *string `json:"versioningConfig,omitempty"`

	// Since tags does not have a json representation, we use its xml byte
	// representation directly.
	Tags *string `json:"tags,omitempty"`

	// Since object lock does not have a json representation, we use its xml
	// byte representation.
	ObjectLockConfig *string `json:"objectLockConfig,omitempty"`

	// Since SSE config does not have a json representation, we use its xml
	// byte respresentation.
	SSEConfig *string `json:"sseConfig,omitempty"`
	// replication config in json representation
	ReplicationConfig *string `json:"replicationConfig,omitempty"`
	// quota config in json representation
	QuotaConfig *string `json:"quotaConfig,omitempty"`

	// time stamps of bucket metadata updates
	PolicyUpdatedAt            time.Time `json:"policyTimestamp,omitempty"`
	TagConfigUpdatedAt         time.Time `json:"tagTimestamp,omitempty"`
	ObjectLockConfigUpdatedAt  time.Time `json:"olockTimestamp,omitempty"`
	SSEConfigUpdatedAt         time.Time `json:"sseTimestamp,omitempty"`
	VersioningConfigUpdatedAt  time.Time `json:"versioningTimestamp,omitempty"`
	ReplicationConfigUpdatedAt time.Time `json:"replicationConfigTimestamp,omitempty"`
	QuotaConfigUpdatedAt       time.Time `json:"quotaTimestamp,omitempty"`
	CreatedAt                  time.Time `json:"bucketTimestamp,omitempty"`
	DeletedAt                  time.Time `json:"bucketDeletedTimestamp,omitempty"`
	Location                   string    `json:"location,omitempty"`
}

// OpenIDProviderSettings contains info on a particular OIDC based provider.
type OpenIDProviderSettings struct {
	ClaimName            string
	ClaimUserinfoEnabled bool
	RolePolicy           string
	ClientID             string
	HashedClientSecret   string
}

// OpenIDSettings contains OpenID configuration info of a cluster.
type OpenIDSettings struct {
	// Enabled is true iff there is at least one OpenID provider configured.
	Enabled bool
	Region  string
	// Map of role ARN to provider info
	Roles map[string]OpenIDProviderSettings
	// Info on the claim based provider (all fields are empty if not
	// present)
	ClaimProvider OpenIDProviderSettings
}

// IDPSettings contains key IDentity Provider settings to validate that all
// peers have the same configuration.
type IDPSettings struct {
	LDAP   LDAPSettings
	OpenID OpenIDSettings
}

// LDAPSettings contains LDAP configuration info of a cluster.
type LDAPSettings struct {
	IsLDAPEnabled          bool
	LDAPUserDNSearchBase   string
	LDAPUserDNSearchFilter string
	LDAPGroupSearchBase    string
	LDAPGroupSearchFilter  string
}

// SRPeerGetIDPSettings - fetches IDP settings from the server.
func (adm *AdminClient) SRPeerGetIDPSettings(ctx context.Context) (info IDPSettings, err error) {
	reqData := requestData{
		relPath: adminAPIPrefix + "/site-replication/peer/idp-settings",
	}

	resp, err := adm.executeMethod(ctx, http.MethodGet, reqData)
	defer closeResponse(resp)
	if err != nil {
		return info, err
	}

	if resp.StatusCode != http.StatusOK {
		return info, httpRespToErrorResponse(resp)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return info, err
	}

	err = json.Unmarshal(b, &info)
	if err != nil {
		// If the server is older version, the IDPSettings was =
		// LDAPSettings, so we try that.
		err2 := json.Unmarshal(b, &info.LDAP)
		if err2 == nil {
			err = nil
		}
	}
	return info, err
}

// SRIAMPolicy - represents an IAM policy.
type SRIAMPolicy struct {
	Policy    json.RawMessage `json:"policy"`
	UpdatedAt time.Time       `json:"updatedAt,omitempty"`
}

// SRInfo gets replication metadata for a site
type SRInfo struct {
	Enabled        bool
	Name           string
	DeploymentID   string
	Buckets        map[string]SRBucketInfo       // map of bucket metadata info
	Policies       map[string]SRIAMPolicy        //  map of IAM policy name to content
	UserPolicies   map[string]SRPolicyMapping    // map of username -> user policy mapping
	UserInfoMap    map[string]UserInfo           // map of user name to UserInfo
	GroupDescMap   map[string]GroupDesc          // map of group name to GroupDesc
	GroupPolicies  map[string]SRPolicyMapping    // map of groupname -> group policy mapping
	ReplicationCfg map[string]replication.Config // map of bucket -> replication config
}

// SRMetaInfo - returns replication metadata info for a site.
func (adm *AdminClient) SRMetaInfo(ctx context.Context, opts SRStatusOptions) (info SRInfo, err error) {
	reqData := requestData{
		relPath:     adminAPIPrefix + "/site-replication/metainfo",
		queryValues: opts.getURLValues(),
	}

	resp, err := adm.executeMethod(ctx, http.MethodGet, reqData)
	defer closeResponse(resp)
	if err != nil {
		return info, err
	}

	if resp.StatusCode != http.StatusOK {
		return info, httpRespToErrorResponse(resp)
	}

	err = json.NewDecoder(resp.Body).Decode(&info)
	return info, err
}

// SRStatusInfo returns detailed status on site replication status
type SRStatusInfo struct {
	Enabled      bool
	MaxBuckets   int                      // maximum buckets seen across sites
	MaxUsers     int                      // maximum users seen across sites
	MaxGroups    int                      // maximum groups seen across sites
	MaxPolicies  int                      // maximum policies across sites
	Sites        map[string]PeerInfo      // deployment->sitename
	StatsSummary map[string]SRSiteSummary // map of deployment id -> site stat
	// BucketStats map of bucket to slice of deployment IDs with stats. This is populated only if there are
	// mismatches or if a specific bucket's stats are requested
	BucketStats map[string]map[string]SRBucketStatsSummary
	// PolicyStats map of policy to slice of deployment IDs with stats. This is populated only if there are
	// mismatches or if a specific bucket's stats are requested
	PolicyStats map[string]map[string]SRPolicyStatsSummary
	// UserStats map of user to slice of deployment IDs with stats. This is populated only if there are
	// mismatches or if a specific bucket's stats are requested
	UserStats map[string]map[string]SRUserStatsSummary
	// GroupStats map of group to slice of deployment IDs with stats. This is populated only if there are
	// mismatches or if a specific bucket's stats are requested
	GroupStats map[string]map[string]SRGroupStatsSummary
}

// SRPolicyStatsSummary has status of policy replication misses
type SRPolicyStatsSummary struct {
	DeploymentID   string
	PolicyMismatch bool
	HasPolicy      bool
}

// SRUserStatsSummary has status of user replication misses
type SRUserStatsSummary struct {
	DeploymentID     string
	PolicyMismatch   bool
	UserInfoMismatch bool
	HasUser          bool
	HasPolicyMapping bool
}

// SRGroupStatsSummary has status of group replication misses
type SRGroupStatsSummary struct {
	DeploymentID      string
	PolicyMismatch    bool
	HasGroup          bool
	GroupDescMismatch bool
	HasPolicyMapping  bool
}

// SRBucketStatsSummary has status of bucket metadata replication misses
type SRBucketStatsSummary struct {
	DeploymentID             string
	HasBucket                bool
	BucketMarkedDeleted      bool
	TagMismatch              bool
	VersioningConfigMismatch bool
	OLockConfigMismatch      bool
	PolicyMismatch           bool
	SSEConfigMismatch        bool
	ReplicationCfgMismatch   bool
	QuotaCfgMismatch         bool
	HasTagsSet               bool
	HasOLockConfigSet        bool
	HasPolicySet             bool
	HasSSECfgSet             bool
	HasReplicationCfg        bool
	HasQuotaCfgSet           bool
}

// SRSiteSummary holds the count of replicated items in site replication
type SRSiteSummary struct {
	ReplicatedBuckets             int // count of buckets replicated across sites
	ReplicatedTags                int // count of buckets with tags replicated across sites
	ReplicatedBucketPolicies      int // count of policies replicated across sites
	ReplicatedIAMPolicies         int // count of IAM policies replicated across sites
	ReplicatedUsers               int // count of users replicated across sites
	ReplicatedGroups              int // count of groups replicated across sites
	ReplicatedLockConfig          int // count of object lock config replicated across sites
	ReplicatedSSEConfig           int // count of SSE config replicated across sites
	ReplicatedVersioningConfig    int // count of versioning config replicated across sites
	ReplicatedQuotaConfig         int // count of bucket with quota config replicated across sites
	ReplicatedUserPolicyMappings  int // count of user policy mappings replicated across sites
	ReplicatedGroupPolicyMappings int // count of group policy mappings replicated across sites

	TotalBucketsCount            int // total buckets on this site
	TotalTagsCount               int // total count of buckets with tags on this site
	TotalBucketPoliciesCount     int // total count of buckets with bucket policies for this site
	TotalIAMPoliciesCount        int // total count of IAM policies for this site
	TotalLockConfigCount         int // total count of buckets with object lock config for this site
	TotalSSEConfigCount          int // total count of buckets with SSE config
	TotalVersioningConfigCount   int // total count of bucekts with versioning config
	TotalQuotaConfigCount        int // total count of buckets with quota config
	TotalUsersCount              int // total number of users seen on this site
	TotalGroupsCount             int // total number of groups seen on this site
	TotalUserPolicyMappingCount  int // total number of user policy mappings seen on this site
	TotalGroupPolicyMappingCount int // total number of group policy mappings seen on this site
}

// SREntityType specifies type of entity
type SREntityType int

const (
	// Unspecified entity
	Unspecified SREntityType = iota

	// SRBucketEntity Bucket entity type
	SRBucketEntity

	// SRPolicyEntity Policy entity type
	SRPolicyEntity

	// SRUserEntity User entity type
	SRUserEntity

	// SRGroupEntity Group entity type
	SRGroupEntity
)

// SRStatusOptions holds SR status options
type SRStatusOptions struct {
	Buckets     bool
	Policies    bool
	Users       bool
	Groups      bool
	Entity      SREntityType
	EntityValue string
	ShowDeleted bool
}

// IsEntitySet returns true if entity option is set
func (o *SRStatusOptions) IsEntitySet() bool {
	switch o.Entity {
	case SRBucketEntity, SRPolicyEntity, SRUserEntity, SRGroupEntity:
		return true
	default:
		return false
	}
}

// GetSREntityType returns the SREntityType for a key
func GetSREntityType(name string) SREntityType {
	switch name {
	case "bucket":
		return SRBucketEntity
	case "user":
		return SRUserEntity
	case "group":
		return SRGroupEntity
	case "policy":
		return SRPolicyEntity
	default:
		return Unspecified
	}
}

func (o *SRStatusOptions) getURLValues() url.Values {
	urlValues := make(url.Values)
	urlValues.Set("buckets", strconv.FormatBool(o.Buckets))
	urlValues.Set("policies", strconv.FormatBool(o.Policies))
	urlValues.Set("users", strconv.FormatBool(o.Users))
	urlValues.Set("groups", strconv.FormatBool(o.Groups))
	urlValues.Set("showDeleted", strconv.FormatBool(o.ShowDeleted))

	if o.IsEntitySet() {
		urlValues.Set("entityvalue", o.EntityValue)
		switch o.Entity {
		case SRBucketEntity:
			urlValues.Set("entity", "bucket")
		case SRPolicyEntity:
			urlValues.Set("entity", "policy")
		case SRUserEntity:
			urlValues.Set("entity", "user")
		case SRGroupEntity:
			urlValues.Set("entity", "group")
		}
	}
	return urlValues
}

// SRStatusInfo - returns site replication status
func (adm *AdminClient) SRStatusInfo(ctx context.Context, opts SRStatusOptions) (info SRStatusInfo, err error) {
	reqData := requestData{
		relPath:     adminAPIPrefix + "/site-replication/status",
		queryValues: opts.getURLValues(),
	}

	resp, err := adm.executeMethod(ctx, http.MethodGet, reqData)
	defer closeResponse(resp)
	if err != nil {
		return info, err
	}

	if resp.StatusCode != http.StatusOK {
		return info, httpRespToErrorResponse(resp)
	}

	err = json.NewDecoder(resp.Body).Decode(&info)
	return info, err
}

// ReplicateEditStatus - returns status of edit request.
type ReplicateEditStatus struct {
	Success   bool   `json:"success"`
	Status    string `json:"status"`
	ErrDetail string `json:"errorDetail,omitempty"`
}

// SiteReplicationEdit - sends the SR edit API call.
func (adm *AdminClient) SiteReplicationEdit(ctx context.Context, site PeerInfo) (ReplicateEditStatus, error) {
	sitesBytes, err := json.Marshal(site)
	if err != nil {
		return ReplicateEditStatus{}, nil
	}
	encBytes, err := EncryptData(adm.getSecretKey(), sitesBytes)
	if err != nil {
		return ReplicateEditStatus{}, err
	}

	reqData := requestData{
		relPath: adminAPIPrefix + "/site-replication/edit",
		content: encBytes,
	}

	resp, err := adm.executeMethod(ctx, http.MethodPut, reqData)
	defer closeResponse(resp)
	if err != nil {
		return ReplicateEditStatus{}, err
	}

	if resp.StatusCode != http.StatusOK {
		return ReplicateEditStatus{}, httpRespToErrorResponse(resp)
	}

	var res ReplicateEditStatus
	err = json.NewDecoder(resp.Body).Decode(&res)
	return res, err
}

// SRPeerEdit - used only by minio server to update peer endpoint
// for a server already in the site replication setup
func (adm *AdminClient) SRPeerEdit(ctx context.Context, pi PeerInfo) error {
	b, err := json.Marshal(pi)
	if err != nil {
		return err
	}

	reqData := requestData{
		relPath: adminAPIPrefix + "/site-replication/peer/edit",
		content: b,
	}

	resp, err := adm.executeMethod(ctx, http.MethodPut, reqData)
	defer closeResponse(resp)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return httpRespToErrorResponse(resp)
	}

	return nil
}

// SiteReplicationRemove - unlinks a site from site replication
func (adm *AdminClient) SiteReplicationRemove(ctx context.Context, removeReq SRRemoveReq) (st ReplicateRemoveStatus, err error) {
	rmvBytes, err := json.Marshal(removeReq)
	if err != nil {
		return st, nil
	}

	reqData := requestData{
		relPath: adminAPIPrefix + "/site-replication/remove",
		content: rmvBytes,
	}

	resp, err := adm.executeMethod(ctx, http.MethodPut, reqData)
	defer closeResponse(resp)
	if err != nil {
		return st, err
	}

	if resp.StatusCode != http.StatusOK {
		return st, httpRespToErrorResponse(resp)
	}
	var res ReplicateRemoveStatus
	err = json.NewDecoder(resp.Body).Decode(&res)
	return res, err
}

// SRPeerRemove - used only by minio server to unlink cluster replication
// for a server already in the site replication setup
func (adm *AdminClient) SRPeerRemove(ctx context.Context, removeReq SRRemoveReq) (st ReplicateRemoveStatus, err error) {
	reqBytes, err := json.Marshal(removeReq)
	if err != nil {
		return st, err
	}

	reqData := requestData{
		relPath: adminAPIPrefix + "/site-replication/peer/remove",
		content: reqBytes,
	}

	resp, err := adm.executeMethod(ctx, http.MethodPut, reqData)
	defer closeResponse(resp)
	if err != nil {
		return st, err
	}

	if resp.StatusCode != http.StatusOK {
		return st, httpRespToErrorResponse(resp)
	}
	return ReplicateRemoveStatus{}, nil
}

// ReplicateRemoveStatus - returns status of unlink request.
type ReplicateRemoveStatus struct {
	Status    string `json:"status"`
	ErrDetail string `json:"errorDetail,omitempty"`
}

// SRRemoveReq - arg body for SRRemoveReq
type SRRemoveReq struct {
	SiteNames []string `json:"sites"`
	RemoveAll bool     `json:"all"` // true if all sites are to be removed.
}

const (
	ReplicateRemoveStatusSuccess = "Requested site(s) were removed from cluster replication successfully."
	ReplicateRemoveStatusPartial = "Some site(s) could not be removed from cluster replication configuration."
)

type ResyncBucketStatus struct {
	Bucket    string `json:"bucket"`
	Status    string `json:"status"`
	ErrDetail string `json:"errorDetail,omitempty"`
}

// SRResyncOpStatus - returns status of resync start request.
type SRResyncOpStatus struct {
	OpType    string               `json:"op"` // one of "start" or "cancel"
	ResyncID  string               `json:"id"`
	Status    string               `json:"status"`
	Buckets   []ResyncBucketStatus `json:"buckets"`
	ErrDetail string               `json:"errorDetail,omitempty"`
}

// SiteResyncOp type of resync operation
type SiteResyncOp string

const (
	// SiteResyncStart starts a site resync operation
	SiteResyncStart SiteResyncOp = "start"
	// SiteResyncCancel cancels ongoing site resync
	SiteResyncCancel SiteResyncOp = "cancel"
)

// SiteReplicationResyncOp - perform a site replication resync operation
func (adm *AdminClient) SiteReplicationResyncOp(ctx context.Context, site PeerInfo, op SiteResyncOp) (SRResyncOpStatus, error) {
	reqBytes, err := json.Marshal(site)
	if err != nil {
		return SRResyncOpStatus{}, nil
	}

	v := url.Values{}
	v.Add("operation", string(op))

	reqData := requestData{
		relPath:     adminAPIPrefix + "/site-replication/resync/op",
		content:     reqBytes,
		queryValues: v,
	}

	resp, err := adm.executeMethod(ctx, http.MethodPut, reqData)
	defer closeResponse(resp)
	if err != nil {
		return SRResyncOpStatus{}, err
	}

	if resp.StatusCode != http.StatusOK {
		return SRResyncOpStatus{}, httpRespToErrorResponse(resp)
	}

	var res SRResyncOpStatus
	err = json.NewDecoder(resp.Body).Decode(&res)
	return res, err
}
