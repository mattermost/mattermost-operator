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
			want:           "db-5526c9",
		},
		{
			name:           "short 1",
			mattermostName: "s",
			want:           "db-03c7c0",
		},
		{
			name:           "short 1",
			mattermostName: "a",
			want:           "db-0cc175",
		},
		{
			name:           "short 2",
			mattermostName: "ab",
			want:           "db-187ef4",
		},
		{
			name:           "short 0",
			mattermostName: "",
			want:           "db-d41d8c",
		},
		{
			name:           "test-mm",
			mattermostName: "test-mm",
			want:           "db-8848a1",
		},
		{
			name:           "test-mm2",
			mattermostName: "test-mm2",
			want:           "db-95b2f2",
		},
		{
			name:           "mm-jw",
			mattermostName: "mm-jw",
			want:           "db-f64f9d",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HashWithPrefix("db", tt.mattermostName)
			require.Equal(t, tt.want, got)
		})
	}
}
