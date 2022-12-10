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

//go:generate msgp -file $GOFILE

// TierMinIO represents the remote tier configuration for MinIO object storage backend.
type TierMinIO struct {
	Endpoint  string `json:",omitempty"`
	AccessKey string `json:",omitempty"`
	SecretKey string `json:",omitempty"`
	Bucket    string `json:",omitempty"`
	Prefix    string `json:",omitempty"`
	Region    string `json:",omitempty"`
}

// MinIOOptions supports NewTierMinIO to take variadic options
type MinIOOptions func(*TierMinIO) error

// MinIORegion helper to supply optional region to NewTierMinIO
func MinIORegion(region string) func(m *TierMinIO) error {
	return func(m *TierMinIO) error {
		m.Region = region
		return nil
	}
}

// MinIOPrefix helper to supply optional object prefix to NewTierMinIO
func MinIOPrefix(prefix string) func(m *TierMinIO) error {
	return func(m *TierMinIO) error {
		m.Prefix = prefix
		return nil
	}
}

func NewTierMinIO(name, endpoint, accessKey, secretKey, bucket string, options ...MinIOOptions) (*TierConfig, error) {
	if name == "" {
		return nil, ErrTierNameEmpty
	}
	m := &TierMinIO{
		AccessKey: accessKey,
		SecretKey: secretKey,
		Bucket:    bucket,
		Endpoint:  endpoint,
	}

	for _, option := range options {
		err := option(m)
		if err != nil {
			return nil, err
		}
	}

	return &TierConfig{
		Version: TierConfigVer,
		Type:    MinIO,
		Name:    name,
		MinIO:   m,
	}, nil
}
