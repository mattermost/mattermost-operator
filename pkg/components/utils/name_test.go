package utils

import "testing"

func TestHashedName(t *testing.T) {
	tests := []struct {
		name           string
		mattermostName string
		want           string
	}{
		{
			name:           "basic",
			mattermostName: "some-deployment",
			want:           "db-VSbJJ1",
		},
		{
			name:           "short 1",
			mattermostName: "s",
			want:           "db-A8fArO",
		},
		{
			name:           "short 1",
			mattermostName: "a",
			want:           "db-DMF1uc",
		},
		{
			name:           "short 2",
			mattermostName: "ab",
			want:           "db-GH70Q2",
		},
		{
			name:           "short 0",
			mattermostName: "",
			want:           "db-1B2M2Y",
		},
		{
			name:           "test-mm",
			mattermostName: "test-mm",
			want:           "db-iEihUe",
		},
		{
			name:           "test-mm2",
			mattermostName: "test-mm2",
			want:           "db-lbLy8q",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HashWithPrefix("db", tt.mattermostName); got != tt.want {
				t.Errorf("Name() = %v, want %v", got, tt.want)
			}
		})
	}
}
