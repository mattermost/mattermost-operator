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
)

// LogMask is a bit mask for log types.
type LogMask uint64

const (
	LogMaskMinIO LogMask = 1 << iota
	LogMaskApplication

	// LogMaskAll must be the last.
	LogMaskAll LogMask = (1 << iota) - 1
)

// Mask returns the LogMask as uint64
func (m LogMask) Mask() uint64 {
	return uint64(m)
}

// Contains returns whether all flags in other is present in t.
func (m LogMask) Contains(other LogMask) bool {
	return m&other == other
}

// LogKind specifies the kind of error log
type LogKind string

const (
	// LogKindMinio Minio errors
	LogKindMinio LogKind = "MINIO"
	// LogKindApplication Application errors
	LogKindApplication LogKind = "APPLICATION"
	// LogKindAll All errors
	LogKindAll LogKind = "ALL"
)

// LogMask returns the mask based on the kind.
func (l LogKind) LogMask() LogMask {
	switch l {
	case LogKindMinio:
		return LogMaskMinIO
	case LogKindApplication:
		return LogMaskApplication
	case LogKindAll:
		return LogMaskAll
	}
	return 0
}

func (l LogKind) String() string {
	return string(l)
}

// LogInfo holds console log messages
type LogInfo struct {
	logEntry
	ConsoleMsg string
	NodeName   string `json:"node"`
	Err        error  `json:"-"`
}

// GetLogs - listen on console log messages.
func (adm AdminClient) GetLogs(ctx context.Context, node string, lineCnt int, logKind string) <-chan LogInfo {
	logCh := make(chan LogInfo, 1)

	// Only success, start a routine to start reading line by line.
	go func(logCh chan<- LogInfo) {
		defer close(logCh)
		urlValues := make(url.Values)
		urlValues.Set("node", node)
		urlValues.Set("limit", strconv.Itoa(lineCnt))
		urlValues.Set("logType", logKind)
		for {
			reqData := requestData{
				relPath:     adminAPIPrefix + "/log",
				queryValues: urlValues,
			}
			// Execute GET to call log handler
			resp, err := adm.executeMethod(ctx, http.MethodGet, reqData)
			if err != nil {
				closeResponse(resp)
				return
			}

			if resp.StatusCode != http.StatusOK {
				logCh <- LogInfo{Err: httpRespToErrorResponse(resp)}
				return
			}
			dec := json.NewDecoder(resp.Body)
			for {
				var info LogInfo
				if err = dec.Decode(&info); err != nil {
					break
				}
				select {
				case <-ctx.Done():
					return
				case logCh <- info:
				}
			}

		}
	}(logCh)

	// Returns the log info channel, for caller to start reading from.
	return logCh
}

// Mask returns the mask based on the error level.
func (l LogInfo) Mask() uint64 {
	return l.LogKind.LogMask().Mask()
}
