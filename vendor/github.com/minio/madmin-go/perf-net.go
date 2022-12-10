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
	"time"
)

// NetperfNodeResult - stats from each server
type NetperfNodeResult struct {
	Endpoint string `json:"endpoint"`
	TX       uint64 `json:"tx"`
	RX       uint64 `json:"rx"`
	Error    string `json:"error,omitempty"`
}

// NetperfResult - aggregate results from all servers
type NetperfResult struct {
	NodeResults []NetperfNodeResult `json:"nodeResults"`
}

// Netperf - perform netperf on the MinIO servers
func (adm *AdminClient) Netperf(ctx context.Context, duration time.Duration) (result NetperfResult, err error) {
	queryVals := make(url.Values)
	queryVals.Set("duration", duration.String())

	resp, err := adm.executeMethod(ctx,
		http.MethodPost, requestData{
			relPath:     adminAPIPrefix + "/speedtest/net",
			queryValues: queryVals,
		})
	if err != nil {
		return result, err
	}
	if resp.StatusCode != http.StatusOK {
		return result, httpRespToErrorResponse(resp)
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	return result, err
}
