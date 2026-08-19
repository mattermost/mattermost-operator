package mattermost

import (
	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	corev1 "k8s.io/api/core/v1"
)

// UnmanagedDBConfig is a no-op DatabaseConfig for databases the Operator does
// not manage. The user is expected to supply MM_SQLSETTINGS_* (and any related
// connection settings) via spec.mattermostEnv, so the Operator injects neither
// environment variables nor readiness init containers. Returning this instead
// of a nil DatabaseConfig keeps the pipeline free of nil checks.
type UnmanagedDBConfig struct{}

func (c *UnmanagedDBConfig) EnvVars(_ *mmv1beta.Mattermost) []corev1.EnvVar {
	return nil
}

func (c *UnmanagedDBConfig) InitContainers(_ *mmv1beta.Mattermost) []corev1.Container {
	return nil
}
