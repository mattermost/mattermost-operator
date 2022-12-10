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

// ReplDiffOpts holds options for `mc replicate diff` command
type ReplDiffOpts struct {
	ARN     string
	Verbose bool
	Prefix  string
}

// TgtDiffInfo returns status of unreplicated objects
// for the target ARN
type TgtDiffInfo struct {
	ReplicationStatus       string `json:"rStatus,omitempty"`  // target replication status
	DeleteReplicationStatus string `json:"drStatus,omitempty"` // target delete replication status
}

// DiffInfo represents relevant replication status and last attempt to replicate
// for the replication targets configured for the bucket
type DiffInfo struct {
	Object                  string                 `json:"object"`
	VersionID               string                 `json:"versionId"`
	Targets                 map[string]TgtDiffInfo `json:"targets,omitempty"`
	Err                     error                  `json:"error,omitempty"`
	ReplicationStatus       string                 `json:"rStatus,omitempty"` // overall replication status
	DeleteReplicationStatus string                 `json:"dStatus,omitempty"` // overall replication status of version delete
	ReplicationTimestamp    time.Time              `json:"replTimestamp,omitempty"`
	LastModified            time.Time              `json:"lastModified,omitempty"`
	IsDeleteMarker          bool                   `json:"deletemarker"`
}

// BucketReplicationDiff - gets diff for non-replicated entries.
func (adm *AdminClient) BucketReplicationDiff(ctx context.Context, bucketName string, opts ReplDiffOpts) <-chan DiffInfo {
	diffCh := make(chan DiffInfo)

	// start a routine to start reading line by line.
	go func(diffCh chan<- DiffInfo) {
		defer close(diffCh)
		queryValues := url.Values{}
		queryValues.Set("bucket", bucketName)

		if opts.Verbose {
			queryValues.Set("verbose", "true")
		}
		if opts.ARN != "" {
			queryValues.Set("arn", opts.ARN)
		}
		if opts.Prefix != "" {
			queryValues.Set("prefix", opts.Prefix)
		}

		reqData := requestData{
			relPath:     adminAPIPrefix + "/replication/diff",
			queryValues: queryValues,
		}

		// Execute PUT on /minio/admin/v3/diff to set quota for a bucket.
		resp, err := adm.executeMethod(ctx, http.MethodPost, reqData)
		if err != nil {
			diffCh <- DiffInfo{Err: err}
			return
		}
		defer closeResponse(resp)

		if resp.StatusCode != http.StatusOK {
			diffCh <- DiffInfo{Err: httpRespToErrorResponse(resp)}
			return
		}

		dec := json.NewDecoder(resp.Body)
		for {
			var di DiffInfo
			if err = dec.Decode(&di); err != nil {
				break
			}
			select {
			case <-ctx.Done():
				return
			case diffCh <- di:
			}
		}
	}(diffCh)
	// Returns the diff channel, for caller to start reading from.
	return diffCh
}
