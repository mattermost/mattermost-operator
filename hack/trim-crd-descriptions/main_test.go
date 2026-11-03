// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
