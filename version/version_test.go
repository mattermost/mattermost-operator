package version

import (
	"fmt"
	"regexp"
	"testing"
)

func TestVersion(t *testing.T) {
	repoVersion := version
	progBuildTime := GetBuildTime()
	progVersion := GetVersion()
	progBuildHash := GetBuildHash()

	if repoVersion != progVersion {
		t.Errorf("Version did not match repo, got: %s, want: %s.", progVersion, repoVersion)
	}

	expectedVersionStr := fmt.Sprintf("Mattermost Operator: version %v, built %v, hash %v", progVersion, progBuildTime, progBuildHash)
	getVersionStr := GetVersionString()
	if getVersionStr != expectedVersionStr {
		t.Errorf("Version did not match got: %s, want: %s.", getVersionStr, expectedVersionStr)
	}

	// YYYYmmdd.HHMMSS
	matched, _ := regexp.MatchString("\\d{8}\\.\\d{6}", progBuildTime)
	if !matched {
		t.Errorf("Build time did not match the expected syntax, got %s, want: YYYYmmdd.HHMMSS", progBuildTime)
	}

}
