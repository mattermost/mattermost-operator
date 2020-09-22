package clusterinstallation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnsureLabels(t *testing.T) {
	tests := []struct {
		name     string
		required map[string]string
		final    map[string]string
		expected map[string]string
	}{
		{
			"nil-nil",
			nil,
			nil,
			nil,
		}, {
			"one-nil",
			map[string]string{"key1": "value1"},
			nil,
			map[string]string{"key1": "value1"},
		}, {
			"same-labels",
			map[string]string{"key1": "value1"},
			map[string]string{"key1": "value1"},
			map[string]string{"key1": "value1"},
		}, {
			"add-key",
			map[string]string{"key1": "value1"},
			map[string]string{"notkey1": "notvalue1"},
			map[string]string{"key1": "value1", "notkey1": "notvalue1"},
		}, {
			"fix-value",
			map[string]string{"key1": "value1"},
			map[string]string{"key1": "notvalue1"},
			map[string]string{"key1": "value1"},
		}, {
			"complex",
			map[string]string{"key1": "value1", "key2": "value2"},
			map[string]string{"key1": "notvalue1", "key3": "value3"},
			map[string]string{"key1": "value1", "key2": "value2", "key3": "value3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, ensureLabels(tt.required, tt.final))
		})
	}
}
