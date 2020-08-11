package version

import (
	"fmt"
)

var version = "1.6.2"
var buildTime string
var buildHash string

// GetVersionString returns a standard version header
func GetVersionString() string {
	return fmt.Sprintf("Mattermost Operator: version %v, build time %v, hash %v", version, buildTime, buildHash)
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
