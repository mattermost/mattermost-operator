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
	"io"
	"io/ioutil"
	"net/http"
)

// ExportIAM makes an admin call to export IAM data
func (adm *AdminClient) ExportIAM(ctx context.Context) (io.ReadCloser, error) {
	path := adminAPIPrefix + "/export-iam"

	resp, err := adm.executeMethod(ctx,
		http.MethodGet, requestData{
			relPath: path,
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

// ImportIAM makes an admin call to setup IAM  from imported content
func (adm *AdminClient) ImportIAM(ctx context.Context, contentReader io.ReadCloser) error {
	content, err := ioutil.ReadAll(contentReader)
	if err != nil {
		return err
	}

	path := adminAPIPrefix + "/import-iam"
	resp, err := adm.executeMethod(ctx,
		http.MethodPut, requestData{
			relPath: path,
			content: content,
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
