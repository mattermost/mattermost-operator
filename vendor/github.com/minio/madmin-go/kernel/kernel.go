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

//go:build linux
// +build linux

package kernel

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

var versionRegex = regexp.MustCompile(`^(\d+)\.(\d+).(\d+).*$`)

// VersionFromRelease converts a release string with format
// 4.4.2[-1] to a kernel version number in LINUX_VERSION_CODE format.
// That is, for kernel "a.b.c", the version number will be (a<<16 + b<<8 + c)
func VersionFromRelease(releaseString string) (uint32, error) {
	versionParts := versionRegex.FindStringSubmatch(releaseString)
	if len(versionParts) != 4 {
		return 0, fmt.Errorf("got invalid release version %q (expected format '4.3.2-1')", releaseString)
	}
	major, err := strconv.Atoi(versionParts[1])
	if err != nil {
		return 0, err
	}

	minor, err := strconv.Atoi(versionParts[2])
	if err != nil {
		return 0, err
	}

	patch, err := strconv.Atoi(versionParts[3])
	if err != nil {
		return 0, err
	}
	return Version(major, minor, patch), nil
}

// Version implements KERNEL_VERSION equivalent macro
// #define KERNEL_VERSION(a,b,c) (((a) << 16) + ((b) << 8) + ((c) > 255 ? 255 : (c)))
func Version(major, minor, patch int) uint32 {
	if patch > 255 {
		patch = 255
	}
	out := major<<16 + minor<<8 + patch
	return uint32(out)
}

func currentReleaseUname() (string, error) {
	var buf syscall.Utsname
	if err := syscall.Uname(&buf); err != nil {
		return "", err
	}
	releaseString := strings.Trim(utsnameStr(buf.Release[:]), "\x00")
	return releaseString, nil
}

func currentReleaseUbuntu() (string, error) {
	procVersion, err := ioutil.ReadFile("/proc/version_signature")
	if err != nil {
		return "", err
	}
	var u1, u2, releaseString string
	_, err = fmt.Sscanf(string(procVersion), "%s %s %s", &u1, &u2, &releaseString)
	if err != nil {
		return "", err
	}
	return releaseString, nil
}

var debianVersionRegex = regexp.MustCompile(`.* SMP Debian (\d+\.\d+.\d+-\d+)(?:\+[[:alnum:]]*)?.*`)

func parseDebianRelease(str string) (string, error) {
	match := debianVersionRegex.FindStringSubmatch(str)
	if len(match) != 2 {
		return "", fmt.Errorf("failed to parse kernel version from /proc/version: %s", str)
	}
	return match[1], nil
}

func currentReleaseDebian() (string, error) {
	procVersion, err := ioutil.ReadFile("/proc/version")
	if err != nil {
		return "", fmt.Errorf("error reading /proc/version: %s", err)
	}

	return parseDebianRelease(string(procVersion))
}

// CurrentRelease returns the current kernel release ensuring that
// ubuntu and debian release numbers are accurate.
func CurrentRelease() (string, error) {
	// We need extra checks for Debian and Ubuntu as they modify
	// the kernel version patch number for compatibility with
	// out-of-tree modules. Linux perf tools do the same for Ubuntu
	// systems: https://github.com/torvalds/linux/commit/d18acd15c
	//
	// See also:
	// https://kernel-team.pages.debian.net/kernel-handbook/ch-versions.html
	// https://wiki.ubuntu.com/Kernel/FAQ
	version, err := currentReleaseUbuntu()
	if err == nil {
		return version, nil
	}
	version, err = currentReleaseDebian()
	if err == nil {
		return version, nil
	}
	return currentReleaseUname()
}

// CurrentVersion returns the current kernel version in
// LINUX_VERSION_CODE format (see VersionFromRelease())
func CurrentVersion() (uint32, error) {
	release, err := CurrentRelease()
	if err == nil {
		return VersionFromRelease(release)
	}
	return 0, err
}
