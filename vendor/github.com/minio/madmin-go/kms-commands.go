//
// MinIO Object Storage (c) 2022 MinIO, Inc.
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
	"net/http"
	"net/url"
)

// KMSStatus contains various informations about
// the KMS connected to a MinIO server - like
// the KMS endpoints and the default key ID.
type KMSStatus struct {
	Name         string               `json:"name"`           // Name or type of the KMS
	DefaultKeyID string               `json:"default-key-id"` // The key ID used when no explicit key is specified
	Endpoints    map[string]ItemState `json:"endpoints"`      // List of KMS endpoints and their status (online/offline)
}

// KMSKeyInfo contains key metadata
type KMSKeyInfo struct {
	CreatedAt string `json:"createdAt"`
	CreatedBy string `json:"createdBy"`
	Name      string `json:"name"`
}

// KMSPolicyInfo contains policy metadata
type KMSPolicyInfo struct {
	CreatedAt string `json:"created_at"`
	CreatedBy string `json:"created_by"`
	Name      string `json:"name"`
}

// KMSIdentityInfo contains policy metadata
type KMSIdentityInfo struct {
	CreatedAt string `json:"createdAt"`
	CreatedBy string `json:"createdBy"`
	Identity  string `json:"identity"`
	Policy    string `json:"policy"`
	Error     string `json:"error"`
}

// KMSDescribePolicy contains policy metadata
type KMSDescribePolicy struct {
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
	CreatedBy string `json:"created_by"`
}

// KMSPolicy represents a KMS policy
type KMSPolicy struct {
	Allow []string `json:"allow"`
	Deny  []string `json:"deny"`
}

// KMSDescribeIdentity contains identity metadata
type KMSDescribeIdentity struct {
	Policy    string `json:"policy"`
	Identity  string `json:"identity"`
	IsAdmin   bool   `json:"isAdmin"`
	CreatedAt string `json:"createdAt"`
	CreatedBy string `json:"createdBy"`
}

// KMSDescribeSelfIdentity describes the identity issuing the request
type KMSDescribeSelfIdentity struct {
	Policy     *KMSPolicy `json:"policy"`
	PolicyName string     `json:"policyName"`
	Identity   string     `json:"identity"`
	IsAdmin    bool       `json:"isAdmin"`
	CreatedAt  string     `json:"createdAt"`
	CreatedBy  string     `json:"createdBy"`
}

type KMSMetrics struct {
	RequestOK     int64 `json:"kes_http_request_success"`
	RequestErr    int64 `json:"kes_http_request_error"`
	RequestFail   int64 `json:"kes_http_request_failure"`
	RequestActive int64 `json:"kes_http_request_active"`

	AuditEvents int64 `json:"kes_log_audit_events"`
	ErrorEvents int64 `json:"kes_log_error_events"`

	LatencyHistogram map[int64]int64 `json:"kes_http_response_time"`

	UpTime     int64 `json:"kes_system_up_time"`
	CPUs       int64 `json:"kes_system_num_cpu"`
	UsableCPUs int64 `json:"kes_system_num_cpu_used"`

	Threads     int64 `json:"kes_system_num_threads"`
	HeapAlloc   int64 `json:"kes_system_mem_heap_used"`
	HeapObjects int64 `json:"kes_system_mem_heap_objects"`
	StackAlloc  int64 `json:"kes_system_mem_stack_used"`
}

type KMSAPI struct {
	Method  string
	Path    string
	MaxBody int64
	Timeout int64
}

type KMSVersion struct {
	Version string `json:"version"`
}

// KMSStatus returns status information about the KMS connected
// to the MinIO server, if configured.
func (adm *AdminClient) KMSStatus(ctx context.Context) (KMSStatus, error) {
	// GET /minio/kms/v1/status
	resp, err := adm.doKMSRequest(ctx, "/status", http.MethodGet, nil, map[string]string{})
	if err != nil {
		return KMSStatus{}, err
	}
	defer closeResponse(resp)
	if resp.StatusCode != http.StatusOK {
		return KMSStatus{}, httpRespToErrorResponse(resp)
	}
	var status KMSStatus
	if err = json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return KMSStatus{}, err
	}
	return status, nil
}

// KMSMetrics returns metrics about the KMS connected
// to the MinIO server, if configured.
func (adm *AdminClient) KMSMetrics(ctx context.Context) (*KMSMetrics, error) {
	// GET /minio/kms/v1/metrics
	resp, err := adm.doKMSRequest(ctx, "/metrics", http.MethodGet, nil, map[string]string{})
	if err != nil {
		return nil, err
	}
	defer closeResponse(resp)
	if resp.StatusCode != http.StatusOK {
		return nil, httpRespToErrorResponse(resp)
	}
	var metrics KMSMetrics
	if err = json.NewDecoder(resp.Body).Decode(&metrics); err != nil {
		return nil, err
	}
	return &metrics, nil
}

// KMSAPIs returns a list of supported API endpoints in the KMS connected
// to the MinIO server, if configured.
func (adm *AdminClient) KMSAPIs(ctx context.Context) ([]KMSAPI, error) {
	// GET /minio/kms/v1/apis
	resp, err := adm.doKMSRequest(ctx, "/apis", http.MethodGet, nil, map[string]string{})
	if err != nil {
		return nil, err
	}
	defer closeResponse(resp)
	if resp.StatusCode != http.StatusOK {
		return nil, httpRespToErrorResponse(resp)
	}
	var apis []KMSAPI
	if err = json.NewDecoder(resp.Body).Decode(&apis); err != nil {
		return nil, err
	}
	return apis, nil
}

// KMSVersion returns a list of supported API endpoints in the KMS connected
// to the MinIO server, if configured.
func (adm *AdminClient) KMSVersion(ctx context.Context) (*KMSVersion, error) {
	// GET /minio/kms/v1/version
	resp, err := adm.doKMSRequest(ctx, "/version", http.MethodGet, nil, map[string]string{})
	if err != nil {
		return nil, err
	}
	defer closeResponse(resp)
	if resp.StatusCode != http.StatusOK {
		return nil, httpRespToErrorResponse(resp)
	}
	var version KMSVersion
	if err = json.NewDecoder(resp.Body).Decode(&version); err != nil {
		return nil, err
	}
	return &version, nil
}

// CreateKey tries to create a new master key with the given keyID
// at the KMS connected to a MinIO server.
func (adm *AdminClient) CreateKey(ctx context.Context, keyID string) error {
	// POST /minio/kms/v1/key/create?key-id=<keyID>
	resp, err := adm.doKMSRequest(ctx, "/key/create", http.MethodPost, nil, map[string]string{"key-id": keyID})
	if err != nil {
		return err
	}
	defer closeResponse(resp)
	if resp.StatusCode != http.StatusOK {
		return httpRespToErrorResponse(resp)
	}
	return nil
}

// DeleteKey tries to delete a key with the given keyID
// at the KMS connected to a MinIO server.
func (adm *AdminClient) DeleteKey(ctx context.Context, keyID string) error {
	// DELETE /minio/kms/v1/key/delete?key-id=<keyID>
	resp, err := adm.doKMSRequest(ctx, "/key/delete", http.MethodDelete, nil, map[string]string{"key-id": keyID})
	if err != nil {
		return err
	}
	defer closeResponse(resp)
	if resp.StatusCode != http.StatusOK {
		return httpRespToErrorResponse(resp)
	}
	return nil
}

// ImportKey tries to import a cryptographic key
// at the KMS connected to a MinIO server.
func (adm *AdminClient) ImportKey(ctx context.Context, keyID string, content []byte) error {
	// POST /minio/kms/v1/key/import?key-id=<keyID>
	resp, err := adm.doKMSRequest(ctx, "/key/import", http.MethodPost, content, map[string]string{"key-id": keyID})
	if err != nil {
		return err
	}
	defer closeResponse(resp)
	if resp.StatusCode != http.StatusOK {
		return httpRespToErrorResponse(resp)
	}
	return nil
}

// ListKeys tries to get all key names that match the specified pattern
func (adm *AdminClient) ListKeys(ctx context.Context, pattern string) ([]KMSKeyInfo, error) {
	// GET /minio/kms/v1/key/list?pattern=<pattern>
	resp, err := adm.doKMSRequest(ctx, "/key/list", http.MethodGet, nil, map[string]string{"pattern": pattern})
	if err != nil {
		return nil, err
	}
	defer closeResponse(resp)
	if resp.StatusCode != http.StatusOK {
		return nil, httpRespToErrorResponse(resp)
	}
	var results []KMSKeyInfo
	if err = json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, err
	}
	return results, nil
}

// GetKeyStatus requests status information about the key referenced by keyID
// from the KMS connected to a MinIO by performing a Admin-API request.
// It basically hits the `/minio/admin/v3/kms/key/status` API endpoint.
func (adm *AdminClient) GetKeyStatus(ctx context.Context, keyID string) (*KMSKeyStatus, error) {
	// GET /minio/kms/v1/key/status?key-id=<keyID>
	resp, err := adm.doKMSRequest(ctx, "/key/status", http.MethodGet, nil, map[string]string{"key-id": keyID})
	if err != nil {
		return nil, err
	}
	defer closeResponse(resp)
	if resp.StatusCode != http.StatusOK {
		return nil, httpRespToErrorResponse(resp)
	}
	var keyInfo KMSKeyStatus
	if err = json.NewDecoder(resp.Body).Decode(&keyInfo); err != nil {
		return nil, err
	}
	return &keyInfo, nil
}

// KMSKeyStatus contains some status information about a KMS master key.
// The MinIO server tries to access the KMS and perform encryption and
// decryption operations. If the MinIO server can access the KMS and
// all master key operations succeed it returns a status containing only
// the master key ID but no error.
type KMSKeyStatus struct {
	KeyID         string `json:"key-id"`
	EncryptionErr string `json:"encryption-error,omitempty"` // An empty error == success
	DecryptionErr string `json:"decryption-error,omitempty"` // An empty error == success
}

// SetKMSPolicy tries to create or update a policy
// at the KMS connected to a MinIO server.
func (adm *AdminClient) SetKMSPolicy(ctx context.Context, policy string, content []byte) error {
	// POST /minio/kms/v1/policy/set?policy=<policy>
	resp, err := adm.doKMSRequest(ctx, "/policy/set", http.MethodPost, content, map[string]string{"policy": policy})
	if err != nil {
		return err
	}
	defer closeResponse(resp)
	if resp.StatusCode != http.StatusOK {
		return httpRespToErrorResponse(resp)
	}
	return nil
}

// AssignPolicy tries to assign a policy to an identity
// at the KMS connected to a MinIO server.
func (adm *AdminClient) AssignPolicy(ctx context.Context, policy string, content []byte) error {
	// POST /minio/kms/v1/policy/assign?policy=<policy>
	resp, err := adm.doKMSRequest(ctx, "/policy/assign", http.MethodPost, content, map[string]string{"policy": policy})
	if err != nil {
		return err
	}
	defer closeResponse(resp)
	if resp.StatusCode != http.StatusOK {
		return httpRespToErrorResponse(resp)
	}
	return nil
}

// DescribePolicy tries to describe a KMS policy
func (adm *AdminClient) DescribePolicy(ctx context.Context, policy string) (*KMSDescribePolicy, error) {
	// GET /minio/kms/v1/policy/describe?policy=<policy>
	resp, err := adm.doKMSRequest(ctx, "/policy/describe", http.MethodGet, nil, map[string]string{"policy": policy})
	if err != nil {
		return nil, err
	}
	defer closeResponse(resp)
	if resp.StatusCode != http.StatusOK {
		return nil, httpRespToErrorResponse(resp)
	}
	var dp KMSDescribePolicy
	if err = json.NewDecoder(resp.Body).Decode(&dp); err != nil {
		return nil, err
	}
	return &dp, nil
}

// GetPolicy tries to get a KMS policy
func (adm *AdminClient) GetPolicy(ctx context.Context, policy string) (*KMSPolicy, error) {
	// GET /minio/kms/v1/policy/get?policy=<policy>
	resp, err := adm.doKMSRequest(ctx, "/policy/get", http.MethodGet, nil, map[string]string{"policy": policy})
	if err != nil {
		return nil, err
	}
	defer closeResponse(resp)
	if resp.StatusCode != http.StatusOK {
		return nil, httpRespToErrorResponse(resp)
	}
	var p KMSPolicy
	if err = json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

// ListPolicies tries to get all policies that match the specified pattern
func (adm *AdminClient) ListPolicies(ctx context.Context, pattern string) ([]KMSPolicyInfo, error) {
	// GET /minio/kms/v1/policy/list?pattern=<pattern>
	resp, err := adm.doKMSRequest(ctx, "/policy/list", http.MethodGet, nil, map[string]string{"pattern": pattern})
	if err != nil {
		return nil, err
	}
	defer closeResponse(resp)
	if resp.StatusCode != http.StatusOK {
		return nil, httpRespToErrorResponse(resp)
	}
	var results []KMSPolicyInfo
	if err = json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, err
	}
	return results, nil
}

// DeletePolicy tries to delete a policy
// at the KMS connected to a MinIO server.
func (adm *AdminClient) DeletePolicy(ctx context.Context, policy string) error {
	// DELETE /minio/kms/v1/policy/delete?policy=<policy>
	resp, err := adm.doKMSRequest(ctx, "/policy/delete", http.MethodDelete, nil, map[string]string{"policy": policy})
	if err != nil {
		return err
	}
	defer closeResponse(resp)
	if resp.StatusCode != http.StatusOK {
		return httpRespToErrorResponse(resp)
	}
	return nil
}

// DescribeIdentity tries to describe a KMS identity
func (adm *AdminClient) DescribeIdentity(ctx context.Context, identity string) (*KMSDescribeIdentity, error) {
	// GET /minio/kms/v1/identity/describe?identity=<identity>
	resp, err := adm.doKMSRequest(ctx, "/identity/describe", http.MethodGet, nil, map[string]string{"identity": identity})
	if err != nil {
		return nil, err
	}
	defer closeResponse(resp)
	if resp.StatusCode != http.StatusOK {
		return nil, httpRespToErrorResponse(resp)
	}
	var i KMSDescribeIdentity
	if err = json.NewDecoder(resp.Body).Decode(&i); err != nil {
		return nil, err
	}
	return &i, nil
}

// DescribeSelfIdentity tries to describe the identity issuing the request.
func (adm *AdminClient) DescribeSelfIdentity(ctx context.Context) (*KMSDescribeSelfIdentity, error) {
	// GET /minio/kms/v1/identity/describe-self
	resp, err := adm.doKMSRequest(ctx, "/identity/describe-self", http.MethodGet, nil, map[string]string{})
	if err != nil {
		return nil, err
	}
	defer closeResponse(resp)
	if resp.StatusCode != http.StatusOK {
		return nil, httpRespToErrorResponse(resp)
	}
	var si KMSDescribeSelfIdentity
	if err = json.NewDecoder(resp.Body).Decode(&si); err != nil {
		return nil, err
	}
	return &si, nil
}

// ListIdentities tries to get all identities that match the specified pattern
func (adm *AdminClient) ListIdentities(ctx context.Context, pattern string) ([]KMSIdentityInfo, error) {
	// GET /minio/kms/v1/identity/list?pattern=<pattern>
	if pattern == "" { // list identities does not default to *
		pattern = "*"
	}
	resp, err := adm.doKMSRequest(ctx, "/identity/list", http.MethodGet, nil, map[string]string{"pattern": pattern})
	if err != nil {
		return nil, err
	}
	defer closeResponse(resp)
	if resp.StatusCode != http.StatusOK {
		return nil, httpRespToErrorResponse(resp)
	}
	var results []KMSIdentityInfo
	if err = json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, err
	}
	return results, nil
}

// DeleteIdentity tries to delete a identity
// at the KMS connected to a MinIO server.
func (adm *AdminClient) DeleteIdentity(ctx context.Context, identity string) error {
	// DELETE /minio/kms/v1/identity/delete?identity=<identity>
	resp, err := adm.doKMSRequest(ctx, "/identity/delete", http.MethodDelete, nil, map[string]string{"identity": identity})
	if err != nil {
		return err
	}
	defer closeResponse(resp)
	if resp.StatusCode != http.StatusOK {
		return httpRespToErrorResponse(resp)
	}
	return nil
}

func (adm *AdminClient) doKMSRequest(ctx context.Context, path, method string, content []byte, values map[string]string) (*http.Response, error) {
	qv := url.Values{}
	for key, value := range values {
		qv.Set(key, value)
	}
	reqData := requestData{
		relPath:     kmsAPIPrefix + path,
		queryValues: qv,
		isKMS:       true,
		content:     content,
	}
	return adm.executeMethod(ctx, method, reqData)
}
