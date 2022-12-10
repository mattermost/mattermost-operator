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
	"io/ioutil"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// RebalPoolProgress contains metrics like number of objects, versions, etc rebalanced so far.
type RebalPoolProgress struct {
	NumObjects  uint64        `json:"objects"`
	NumVersions uint64        `json:"versions"`
	Bytes       uint64        `json:"bytes"`
	Bucket      string        `json:"bucket"`
	Object      string        `json:"object"`
	Elapsed     time.Duration `json:"elapsed"`
	ETA         time.Duration `json:"eta"`
}

// RebalancePoolStatus contains metrics of a rebalance operation on a given pool
type RebalancePoolStatus struct {
	ID       int               `json:"id"`                 // Pool index (zero-based)
	Status   string            `json:"status"`             // Active if rebalance is running, empty otherwise
	Used     float64           `json:"used"`               // Percentage used space
	Progress RebalPoolProgress `json:"progress,omitempty"` // is empty when rebalance is not running
}

// RebalanceStatus contains metrics and progress related information on all pools
type RebalanceStatus struct {
	ID        uuid.UUID             // identifies the ongoing rebalance operation by a uuid
	StoppedAt time.Time             `json:"stoppedAt,omitempty"`
	Pools     []RebalancePoolStatus `json:"pools"` // contains all pools, including inactive
}

// RebalanceStart starts a rebalance operation if one isn't in progress already
func (adm *AdminClient) RebalanceStart(ctx context.Context) (id uuid.UUID, err error) {
	// Execute POST on /minio/admin/v3/rebalance/start to start a rebalance operation.
	var resp *http.Response
	resp, err = adm.executeMethod(ctx,
		http.MethodPost,
		requestData{relPath: adminAPIPrefix + "/rebalance/start"})
	defer closeResponse(resp)
	if err != nil {
		return id, err
	}

	if resp.StatusCode != http.StatusOK {
		return id, httpRespToErrorResponse(resp)
	}

	var rebalInfo struct {
		ID uuid.UUID `json:"id"`
	}
	respBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return id, err
	}

	err = json.Unmarshal(respBytes, &rebalInfo)
	if err != nil {
		return id, err
	}

	return rebalInfo.ID, nil
}

func (adm *AdminClient) RebalanceStatus(ctx context.Context) (r RebalanceStatus, err error) {
	// Execute GET on /minio/admin/v3/rebalance/status to get status of an ongoing rebalance operation.
	resp, err := adm.executeMethod(ctx,
		http.MethodGet,
		requestData{relPath: adminAPIPrefix + "/rebalance/status"})
	defer closeResponse(resp)
	if err != nil {
		return r, err
	}

	if resp.StatusCode != http.StatusOK {
		return r, httpRespToErrorResponse(resp)
	}

	respBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return r, err
	}

	err = json.Unmarshal(respBytes, &r)
	if err != nil {
		return r, err
	}

	return r, nil
}

func (adm *AdminClient) RebalanceStop(ctx context.Context) error {
	// Execute POST on /minio/admin/v3/rebalance/stop to stop an ongoing rebalance operation.
	resp, err := adm.executeMethod(ctx,
		http.MethodPost,
		requestData{relPath: adminAPIPrefix + "/rebalance/stop"})
	defer closeResponse(resp)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return httpRespToErrorResponse(resp)
	}

	return nil
}
