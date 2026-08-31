package mattermost

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

// Env var assertion helpers, previously in mattermost_test.go alongside the
// v1alpha1 resource generators that were removed.

func assertEnvVarExists(t *testing.T, name string, env []corev1.EnvVar) {
	t.Helper()
	for _, e := range env {
		if e.Name == name {
			return
		}
	}

	assert.Fail(t, fmt.Sprintf("failed to find env var %s", name))
}

func assertEnvVarNotExist(t *testing.T, name string, env []corev1.EnvVar) {
	t.Helper()
	for _, e := range env {
		if e.Name == name {
			assert.Fail(t, fmt.Sprintf("found env var that should not exist: %s", name))
		}
	}
}
