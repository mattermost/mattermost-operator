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
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// BatchJobType type to describe batch job types
type BatchJobType string

const (
	BatchJobReplicate BatchJobType = "replicate"
)

// SupportedJobTypes supported job types
var SupportedJobTypes = []BatchJobType{
	BatchJobReplicate,
	// add new job types
}

// BatchJobReplicateTemplate provides a sample template
// for batch replication
const BatchJobReplicateTemplate = `replicate:
  apiVersion: v1
  # source of the objects to be replicated
  source:
    type: TYPE # valid values are "minio"
    bucket: BUCKET
    prefix: PREFIX # 'PREFIX' is optional
    # NOTE: if source is remote then target must be "local"
    # endpoint: ENDPOINT
    # credentials:
    #   accessKey: ACCESS-KEY
    #   secretKey: SECRET-KEY
    #   sessionToken: SESSION-TOKEN # Optional only available when rotating credentials are used

  # target where the objects must be replicated
  target:
    type: TYPE # valid values are "minio"
    bucket: BUCKET
    prefix: PREFIX # 'PREFIX' is optional
    # NOTE: if target is remote then source must be "local"
    # endpoint: ENDPOINT
    # credentials:
    #   accessKey: ACCESS-KEY
    #   secretKey: SECRET-KEY
    #   sessionToken: SESSION-TOKEN # Optional only available when rotating credentials are used

  # NOTE: All flags are optional
  # - filtering criteria only applies for all source objects match the criteria
  # - configurable notification endpoints
  # - configurable retries for the job (each retry skips successfully previously replaced objects)
  flags:
    filter:
      newerThan: "7d" # match objects newer than this value (e.g. 7d10h31s)
      olderThan: "7d" # match objects older than this value (e.g. 7d10h31s)
      createdAfter: "date" # match objects created after "date"
      createdBefore: "date" # match objects created before "date"

      ## NOTE: tags are not supported when "source" is remote.
      # tags:
      #   - key: "name"
      #     value: "pick*" # match objects with tag 'name', with all values starting with 'pick'

      ## NOTE: metadata filter not supported when "source" is non MinIO.
      # metadata:
      #   - key: "content-type"
      #     value: "image/*" # match objects with 'content-type', with all values starting with 'image/'

    notify:
      endpoint: "https://notify.endpoint" # notification endpoint to receive job status events
      token: "Bearer xxxxx" # optional authentication token for the notification endpoint

    retry:
      attempts: 10 # number of retries for the job before giving up
      delay: "500ms" # least amount of delay between each retry
`

// BatchJobResult returned by StartBatchJob
type BatchJobResult struct {
	ID      string        `json:"id"`
	Type    BatchJobType  `json:"type"`
	User    string        `json:"user,omitempty"`
	Started time.Time     `json:"started"`
	Elapsed time.Duration `json:"elapsed,omitempty"`
}

// StartBatchJob start a new batch job, input job description is in YAML.
func (adm *AdminClient) StartBatchJob(ctx context.Context, job string) (BatchJobResult, error) {
	resp, err := adm.executeMethod(ctx, http.MethodPost,
		requestData{
			relPath: adminAPIPrefix + "/start-job",
			content: []byte(job),
		},
	)
	if err != nil {
		return BatchJobResult{}, err
	}
	defer closeResponse(resp)
	if resp.StatusCode != http.StatusOK {
		return BatchJobResult{}, httpRespToErrorResponse(resp)
	}

	res := BatchJobResult{}
	dec := json.NewDecoder(resp.Body)
	if err = dec.Decode(&res); err != nil {
		return res, err
	}

	return res, nil
}

// DescribeBatchJob - describes a currently running Job.
func (adm *AdminClient) DescribeBatchJob(ctx context.Context, jobID string) (string, error) {
	values := make(url.Values)
	values.Set("jobId", jobID)

	resp, err := adm.executeMethod(ctx, http.MethodGet,
		requestData{
			relPath:     adminAPIPrefix + "/describe-job",
			queryValues: values,
		},
	)
	if err != nil {
		return "", err
	}
	defer closeResponse(resp)
	if resp.StatusCode != http.StatusOK {
		return "", httpRespToErrorResponse(resp)
	}

	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(buf), nil
}

// GenerateBatchJobOpts is to be implemented in future.
type GenerateBatchJobOpts struct {
	Type BatchJobType
}

// GenerateBatchJob creates a new job template from standard template
// TODO: allow configuring yaml values
func (adm *AdminClient) GenerateBatchJob(ctx context.Context, opts GenerateBatchJobOpts) (string, error) {
	if opts.Type == BatchJobReplicate {
		// TODO: allow configuring the template to fill values from GenerateBatchJobOpts
		return BatchJobReplicateTemplate, nil
	}
	return "", fmt.Errorf("unsupported batch type requested: %s", opts.Type)
}

// ListBatchJobsResult contains entries for all current jobs.
type ListBatchJobsResult struct {
	Jobs []BatchJobResult `json:"jobs"`
}

// ListBatchJobsFilter returns list based on following
// filtering params.
type ListBatchJobsFilter struct {
	ByJobType string
}

// ListBatchJobs list all the currently active batch jobs
func (adm *AdminClient) ListBatchJobs(ctx context.Context, fl *ListBatchJobsFilter) (ListBatchJobsResult, error) {
	if fl == nil {
		return ListBatchJobsResult{}, errors.New("ListBatchJobsFilter cannot be nil")
	}

	values := make(url.Values)
	values.Set("jobType", fl.ByJobType)

	resp, err := adm.executeMethod(ctx, http.MethodGet,
		requestData{
			relPath:     adminAPIPrefix + "/list-jobs",
			queryValues: values,
		},
	)
	if err != nil {
		return ListBatchJobsResult{}, err
	}
	defer closeResponse(resp)

	if resp.StatusCode != http.StatusOK {
		return ListBatchJobsResult{}, httpRespToErrorResponse(resp)
	}

	d := json.NewDecoder(resp.Body)
	result := ListBatchJobsResult{}
	if err = d.Decode(&result); err != nil {
		return result, err
	}

	return result, nil
}
