package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHashedName(t *testing.T) {
	tests := []struct {
		name           string
		mattermostName string
		want           string
	}{
		{
			name:           "basic",
			mattermostName: "some-deployment",
			want:           "db-vsbjj1",
		},
		{
			name:           "short 1",
			mattermostName: "s",
			want:           "db-a8faro",
		},
		{
			name:           "short 1",
			mattermostName: "a",
			want:           "db-dmf1uc",
		},
		{
			name:           "short 2",
			mattermostName: "ab",
			want:           "db-gh70q2",
		},
		{
			name:           "short 0",
			mattermostName: "",
			want:           "db-1b2m2y",
		},
		{
			name:           "test-mm",
			mattermostName: "test-mm",
			want:           "db-ieihue",
		},
		{
			name:           "test-mm2",
			mattermostName: "test-mm2",
			want:           "db-lbly8q",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HashWithPrefix("db", tt.mattermostName)
			require.Equal(t, tt.want, got)
		})
	}
}
