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

//go:build !linux
// +build !linux

package kernel

// VersionFromRelease only implemented on Linux.
func VersionFromRelease(_ string) (uint32, error) {
	return 0, nil
}

// Version only implemented on Linux.
func Version(_, _, _ int) uint32 {
	return 0
}

// CurrentRelease only implemented on Linux.
func CurrentRelease() (string, error) {
	return "", nil
}

// CurrentVersion only implemented on Linux.
func CurrentVersion() (uint32, error) {
	return 0, nil
}
