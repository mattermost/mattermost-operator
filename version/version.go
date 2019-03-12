package version

import (
	"fmt"
)

var (
	version   = "0.0.1"
	buildTime string
	buildHash string
)

// GetVersionString returns a standard version header
func GetVersionString() string {
	return fmt.Sprintf("Mattermost Operator: version %v, built %v, hash %v", version, buildTime, buildHash)
}

// GetVersion returns the semver compatible version number
func GetVersion() string {
	return version
}

// GetBuildTime returns the time at which the build took place
func GetBuildTime() string {
	return buildTime
}

// GetBuildHash returns the git hash at which the build took place
func GetBuildHash() string {
	return buildHash
}
