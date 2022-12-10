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
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
)

// ExportBucketMetadata makes an admin call to export bucket metadata of a bucket
func (adm *AdminClient) ExportBucketMetadata(ctx context.Context, bucket string) (io.ReadCloser, error) {
	path := adminAPIPrefix + "/export-bucket-metadata"
	queryValues := url.Values{}
	queryValues.Set("bucket", bucket)

	resp, err := adm.executeMethod(ctx,
		http.MethodGet, requestData{
			relPath:     path,
			queryValues: queryValues,
		},
	)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		closeResponse(resp)
		return nil, httpRespToErrorResponse(resp)
	}
	return resp.Body, nil
}

// MetaStatus status of metadata import
type MetaStatus struct {
	IsSet bool   `json:"isSet"`
	Err   string `json:"error,omitempty"`
}

// BucketStatus reflects status of bucket metadata import
type BucketStatus struct {
	ObjectLock   MetaStatus `json:"olock"`
	Versioning   MetaStatus `json:"versioning"`
	Policy       MetaStatus `json:"policy"`
	Tagging      MetaStatus `json:"tagging"`
	SSEConfig    MetaStatus `json:"sse"`
	Lifecycle    MetaStatus `json:"lifecycle"`
	Notification MetaStatus `json:"notification"`
	Quota        MetaStatus `json:"quota"`
	Err          string     `json:"error,omitempty"`
}

// BucketMetaImportErrs reports on bucket metadata import status.
type BucketMetaImportErrs struct {
	Buckets map[string]BucketStatus `json:"buckets,omitempty"`
}

// ImportBucketMetadata makes an admin call to set bucket metadata of a bucket from imported content
func (adm *AdminClient) ImportBucketMetadata(ctx context.Context, bucket string, contentReader io.ReadCloser) (r BucketMetaImportErrs, err error) {
	content, err := ioutil.ReadAll(contentReader)
	if err != nil {
		return r, err
	}

	path := adminAPIPrefix + "/import-bucket-metadata"
	queryValues := url.Values{}
	queryValues.Set("bucket", bucket)

	resp, err := adm.executeMethod(ctx,
		http.MethodPut, requestData{
			relPath:     path,
			queryValues: queryValues,
			content:     content,
		},
	)
	defer closeResponse(resp)

	if err != nil {
		return r, err
	}

	if resp.StatusCode != http.StatusOK {
		return r, httpRespToErrorResponse(resp)
	}

	err = json.NewDecoder(resp.Body).Decode(&r)
	return r, err
}
