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
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ServiceRestart - restarts the MinIO cluster
func (adm *AdminClient) ServiceRestart(ctx context.Context) error {
	return adm.serviceCallAction(ctx, ServiceActionRestart)
}

// ServiceStop - stops the MinIO cluster
func (adm *AdminClient) ServiceStop(ctx context.Context) error {
	return adm.serviceCallAction(ctx, ServiceActionStop)
}

// ServiceFreeze - freezes all incoming S3 API calls on MinIO cluster
func (adm *AdminClient) ServiceFreeze(ctx context.Context) error {
	return adm.serviceCallAction(ctx, ServiceActionFreeze)
}

// ServiceUnfreeze - un-freezes all incoming S3 API calls on MinIO cluster
func (adm *AdminClient) ServiceUnfreeze(ctx context.Context) error {
	return adm.serviceCallAction(ctx, ServiceActionUnfreeze)
}

// ServiceAction - type to restrict service-action values
type ServiceAction string

const (
	// ServiceActionRestart represents restart action
	ServiceActionRestart ServiceAction = "restart"
	// ServiceActionStop represents stop action
	ServiceActionStop = "stop"
	// ServiceActionFreeze represents freeze action
	ServiceActionFreeze = "freeze"
	// ServiceActionUnfreeze represents unfreeze a previous freeze action
	ServiceActionUnfreeze = "unfreeze"
)

// serviceCallAction - call service restart/update/stop API.
func (adm *AdminClient) serviceCallAction(ctx context.Context, action ServiceAction) error {
	queryValues := url.Values{}
	queryValues.Set("action", string(action))

	// Request API to Restart server
	resp, err := adm.executeMethod(ctx,
		http.MethodPost, requestData{
			relPath:     adminAPIPrefix + "/service",
			queryValues: queryValues,
		},
	)
	defer closeResponse(resp)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return httpRespToErrorResponse(resp)
	}

	return nil
}

// ServiceTraceInfo holds http trace
type ServiceTraceInfo struct {
	Trace TraceInfo
	Err   error `json:"-"`
}

// ServiceTraceOpts holds tracing options
type ServiceTraceOpts struct {
	// Trace types:
	S3                bool
	Internal          bool
	Storage           bool
	OS                bool
	Scanner           bool
	Decommission      bool
	Healing           bool
	BatchReplication  bool
	Rebalance         bool
	ReplicationResync bool
	OnlyErrors        bool
	Threshold         time.Duration
}

// TraceTypes returns the enabled traces as a bitfield value.
func (t ServiceTraceOpts) TraceTypes() TraceType {
	var tt TraceType
	tt.SetIf(t.S3, TraceS3)
	tt.SetIf(t.Internal, TraceInternal)
	tt.SetIf(t.Storage, TraceStorage)
	tt.SetIf(t.OS, TraceOS)
	tt.SetIf(t.Scanner, TraceScanner)
	tt.SetIf(t.Decommission, TraceDecommission)
	tt.SetIf(t.Healing, TraceHealing)
	tt.SetIf(t.BatchReplication, TraceBatchReplication)
	tt.SetIf(t.Rebalance, TraceRebalance)
	tt.SetIf(t.ReplicationResync, TraceReplicationResync)

	return tt
}

// AddParams will add parameter to url values.
func (t ServiceTraceOpts) AddParams(u url.Values) {
	u.Set("err", strconv.FormatBool(t.OnlyErrors))
	u.Set("threshold", t.Threshold.String())

	u.Set("s3", strconv.FormatBool(t.S3))
	u.Set("internal", strconv.FormatBool(t.Internal))
	u.Set("storage", strconv.FormatBool(t.Storage))
	u.Set("os", strconv.FormatBool(t.OS))
	u.Set("scanner", strconv.FormatBool(t.Scanner))
	u.Set("decommission", strconv.FormatBool(t.Decommission))
	u.Set("healing", strconv.FormatBool(t.Healing))
	u.Set("batch-replication", strconv.FormatBool(t.BatchReplication))
	u.Set("rebalance", strconv.FormatBool(t.Rebalance))
	u.Set("replication-resync", strconv.FormatBool(t.ReplicationResync))
}

// ParseParams will parse parameters and set them to t.
func (t *ServiceTraceOpts) ParseParams(r *http.Request) (err error) {
	t.S3 = r.Form.Get("s3") == "true"
	t.OS = r.Form.Get("os") == "true"
	t.Scanner = r.Form.Get("scanner") == "true"
	t.Decommission = r.Form.Get("decommission") == "true"
	t.Healing = r.Form.Get("healing") == "true"
	t.BatchReplication = r.Form.Get("batch-replication") == "true"
	t.Rebalance = r.Form.Get("rebalance") == "true"
	t.Storage = r.Form.Get("storage") == "true"
	t.Internal = r.Form.Get("internal") == "true"
	t.OnlyErrors = r.Form.Get("err") == "true"
	t.ReplicationResync = r.Form.Get("replication-resync") == "true"

	if th := r.Form.Get("threshold"); th != "" {
		d, err := time.ParseDuration(th)
		if err != nil {
			return err
		}
		t.Threshold = d
	}
	return nil
}

// ServiceTrace - listen on http trace notifications.
func (adm AdminClient) ServiceTrace(ctx context.Context, opts ServiceTraceOpts) <-chan ServiceTraceInfo {
	traceInfoCh := make(chan ServiceTraceInfo)
	// Only success, start a routine to start reading line by line.
	go func(traceInfoCh chan<- ServiceTraceInfo) {
		defer close(traceInfoCh)
		for {
			urlValues := make(url.Values)
			opts.AddParams(urlValues)

			reqData := requestData{
				relPath:     adminAPIPrefix + "/trace",
				queryValues: urlValues,
			}
			// Execute GET to call trace handler
			resp, err := adm.executeMethod(ctx, http.MethodGet, reqData)
			if err != nil {
				traceInfoCh <- ServiceTraceInfo{Err: err}
				return
			}

			if resp.StatusCode != http.StatusOK {
				closeResponse(resp)
				traceInfoCh <- ServiceTraceInfo{Err: httpRespToErrorResponse(resp)}
				return
			}

			dec := json.NewDecoder(resp.Body)
			for {
				var info traceInfoLegacy
				if err = dec.Decode(&info); err != nil {
					closeResponse(resp)
					traceInfoCh <- ServiceTraceInfo{Err: err}
					break
				}
				// Convert if legacy...
				if info.TraceType == TraceType(0) {
					if strings.HasPrefix(info.FuncName, "s3.") {
						info.TraceType = TraceS3
					} else {
						info.TraceType = TraceInternal
					}
					info.HTTP = &TraceHTTPStats{}
					if info.ReqInfo != nil {
						info.Path = info.ReqInfo.Path
						info.HTTP.ReqInfo = *info.ReqInfo
					}
					if info.RespInfo != nil {
						info.HTTP.RespInfo = *info.RespInfo
					}
					if info.CallStats != nil {
						info.Duration = info.CallStats.Latency
						info.HTTP.CallStats = *info.CallStats
					}
				}
				if info.TraceType == TraceOS && info.OSStats != nil {
					info.Path = info.OSStats.Path
					info.Duration = info.OSStats.Duration
				}
				if info.TraceType == TraceStorage && info.StorageStats != nil {
					info.Path = info.StorageStats.Path
					info.Duration = info.StorageStats.Duration
				}
				select {
				case <-ctx.Done():
					closeResponse(resp)
					return
				case traceInfoCh <- ServiceTraceInfo{Trace: info.TraceInfo}:
				}
			}
		}
	}(traceInfoCh)

	// Returns the trace info channel, for caller to start reading from.
	return traceInfoCh
}
