// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package main

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mmv1beta1 "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
)

func TestTrimSchema(t *testing.T) {
	const long = "First sentence. Second sentence."
	const trimmed = "First sentence."
	schemaWithDescription := func(extra map[string]interface{}) map[string]interface{} {
		node := map[string]interface{}{"description": long}
		for key, value := range extra {
			node[key] = value
		}
		return node
	}

	t.Run("keeps operator descriptions and trims core-type ones", func(t *testing.T) {
		nodeSelector := schemaWithDescription(map[string]interface{}{"additionalProperties": schemaWithDescription(nil)})
		nodeAffinity := schemaWithDescription(nil)
		affinity := schemaWithDescription(map[string]interface{}{"properties": map[string]interface{}{"nodeAffinity": nodeAffinity}})
		toleration := schemaWithDescription(nil)
		tolerations := schemaWithDescription(map[string]interface{}{"items": toleration})
		scheduling := schemaWithDescription(map[string]interface{}{"properties": map[string]interface{}{
			"nodeSelector": nodeSelector,
			"affinity":     affinity,
			"tolerations":  tolerations,
		}})

		require.NoError(t, trimSchema(scheduling, reflect.TypeOf(mmv1beta1.Scheduling{}), true, 20))

		assert.Equal(t, long, scheduling["description"])
		assert.Equal(t, long, nodeSelector["description"])
		assert.Equal(t, trimmed, nodeSelector["additionalProperties"].(map[string]interface{})["description"])
		assert.Equal(t, long, affinity["description"])
		assert.Equal(t, trimmed, nodeAffinity["description"])
		assert.Equal(t, long, tolerations["description"])
		assert.Equal(t, trimmed, toleration["description"])
	})

	t.Run("fails on property without Go field", func(t *testing.T) {
		scheduling := map[string]interface{}{"properties": map[string]interface{}{"unknown": map[string]interface{}{}}}

		assert.Error(t, trimSchema(scheduling, reflect.TypeOf(mmv1beta1.Scheduling{}), true, 20))
	})
}

func TestTrimDescription(t *testing.T) {
	for _, tc := range []struct {
		name        string
		description string
		maxLen      int
		expected    string
	}{
		{
			name:        "short description is kept",
			description: "Image defines the Mattermost Docker image.",
			maxLen:      200,
			expected:    "Image defines the Mattermost Docker image.",
		},
		{
			name:        "keeps whole sentences that fit",
			description: "First sentence.\nSecond sentence. Third sentence is too long to fit.",
			maxLen:      40,
			expected:    "First sentence.\nSecond sentence.",
		},
		{
			name:        "keeps first sentence even if longer than limit",
			description: "A first sentence that is longer than the limit. Second.",
			maxLen:      10,
			expected:    "A first sentence that is longer than the limit.",
		},
		{
			name:        "drops later paragraphs",
			description: "Summary of the field.\n\nMore info: https://kubernetes.io/docs/concepts/storage/volumes",
			maxLen:      30,
			expected:    "Summary of the field.",
		},
		{
			name:        "does not split on abbreviations or dotted names",
			description: "Selects a field, e.g. metadata.name of the pod. Second sentence.",
			maxLen:      50,
			expected:    "Selects a field, e.g. metadata.name of the pod.",
		},
		{
			name:        "keeps paragraph without sentence end",
			description: "a description without any period that is longer than the limit",
			maxLen:      10,
			expected:    "a description without any period that is longer than the limit",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, trimDescription(tc.description, tc.maxLen))
		})
	}
}
