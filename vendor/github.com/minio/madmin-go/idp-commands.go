//
// MinIO Object Storage (c) 2015-2022 MinIO, Inc.
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
	"fmt"
	"net/http"
	"net/url"

	"github.com/minio/minio-go/v7/pkg/set"
)

// SetIDPConfig - set idp config to server.
func (adm *AdminClient) SetIDPConfig(ctx context.Context, cfgType, cfgName, cfgData string) (restart bool, err error) {
	encBytes, err := EncryptData(adm.getSecretKey(), []byte(cfgData))
	if err != nil {
		return false, err
	}

	queryParams := make(url.Values, 2)
	queryParams.Set("type", cfgType)
	queryParams.Set("name", cfgName)

	h := make(http.Header, 1)
	h.Add("Content-Type", "application/octet-stream")
	reqData := requestData{
		customHeaders: h,
		relPath:       adminAPIPrefix + "/idp-config",
		queryValues:   queryParams,
		content:       encBytes,
	}

	resp, err := adm.executeMethod(ctx, http.MethodPut, reqData)
	defer closeResponse(resp)
	if err != nil {
		return false, err
	}

	if resp.StatusCode != http.StatusOK {
		return false, httpRespToErrorResponse(resp)
	}

	return resp.Header.Get(ConfigAppliedHeader) != ConfigAppliedTrue, nil
}

// IDPCfgInfo represents a single configuration or related parameter
type IDPCfgInfo struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	IsCfg bool   `json:"isCfg"`
	IsEnv bool   `json:"isEnv"` // relevant only when isCfg=true
}

// IDPConfig contains IDP configuration information returned by server.
type IDPConfig struct {
	Type string       `json:"type"`
	Name string       `json:"name,omitempty"`
	Info []IDPCfgInfo `json:"info"`
}

// Constants for IDP configuration types.
const (
	OpenidIDPCfg string = "openid"
	LDAPIDPCfg   string = "ldap"
)

// ValidIDPConfigTypes - set of valid IDP configs.
var ValidIDPConfigTypes = set.CreateStringSet(OpenidIDPCfg, LDAPIDPCfg)

// GetIDPConfig - fetch IDP config from server.
func (adm *AdminClient) GetIDPConfig(ctx context.Context, cfgType, cfgName string) (c IDPConfig, err error) {
	if !ValidIDPConfigTypes.Contains(cfgType) {
		return c, fmt.Errorf("Invalid config type: %s", cfgType)
	}

	if cfgName == "" {
		cfgName = Default
	}

	queryParams := make(url.Values, 2)
	queryParams.Set("type", cfgType)
	queryParams.Set("name", cfgName)

	reqData := requestData{
		relPath:     adminAPIPrefix + "/idp-config",
		queryValues: queryParams,
	}

	resp, err := adm.executeMethod(ctx, http.MethodGet, reqData)
	defer closeResponse(resp)
	if err != nil {
		return c, err
	}

	if resp.StatusCode != http.StatusOK {
		return c, httpRespToErrorResponse(resp)
	}

	content, err := DecryptData(adm.getSecretKey(), resp.Body)
	if err != nil {
		return c, err
	}

	err = json.Unmarshal(content, &c)
	return c, err
}

// IDPListItem - represents an item in the List IDPs call.
type IDPListItem struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	RoleARN string `json:"roleARN,omitempty"`
}

// ListIDPConfig - list IDP configuration on the server.
func (adm *AdminClient) ListIDPConfig(ctx context.Context, cfgType string) ([]IDPListItem, error) {
	if !ValidIDPConfigTypes.Contains(cfgType) {
		return nil, fmt.Errorf("Invalid config type: %s", cfgType)
	}

	queryParams := make(url.Values, 1)
	queryParams.Set("type", cfgType)

	reqData := requestData{
		relPath:     adminAPIPrefix + "/idp-config",
		queryValues: queryParams,
	}

	resp, err := adm.executeMethod(ctx, http.MethodGet, reqData)
	defer closeResponse(resp)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, httpRespToErrorResponse(resp)
	}

	content, err := DecryptData(adm.getSecretKey(), resp.Body)
	if err != nil {
		return nil, err
	}

	var lst []IDPListItem
	err = json.Unmarshal(content, &lst)
	return lst, err
}

// DeleteIDPConfig - delete an IDP configuration on the server.
func (adm *AdminClient) DeleteIDPConfig(ctx context.Context, cfgType, cfgName string) (restart bool, err error) {
	queryParams := make(url.Values, 2)
	queryParams.Set("type", cfgType)
	queryParams.Set("name", cfgName)

	reqData := requestData{
		relPath:     adminAPIPrefix + "/idp-config",
		queryValues: queryParams,
	}

	resp, err := adm.executeMethod(ctx, http.MethodDelete, reqData)
	defer closeResponse(resp)
	if err != nil {
		return false, err
	}

	if resp.StatusCode != http.StatusOK {
		return false, httpRespToErrorResponse(resp)
	}

	return resp.Header.Get(ConfigAppliedHeader) != ConfigAppliedTrue, nil
}
