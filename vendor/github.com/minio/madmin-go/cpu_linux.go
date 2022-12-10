//go:build linux
// +build linux

//
// MinIO Object Storage (c) 2021-2022 MinIO, Inc.
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
	"github.com/prometheus/procfs/sysfs"
)

func isFreqGovPerf() (bool, error) {
	fs, err := sysfs.NewFS("/sys")
	if err != nil {
		return false, err
	}

	stats, err := fs.SystemCpufreq()
	if err != nil {
		return false, err
	}

	for _, s := range stats {
		if s.Governor != "performance" {
			return false, nil
		}
	}

	return true, nil
}
