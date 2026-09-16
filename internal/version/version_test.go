package version

import (
	"strings"
	"testing"
)

func TestString(t *testing.T) {
	tests := []struct {
		name    string
		version string
		commit  string
		want    string
	}{
		{
			name:    "default dev build",
			version: "dev",
			commit:  "none",
			want:    "dev (none)",
		},
		{
			name:    "injected release build",
			version: "v1.2.3",
			commit:  "abc1234",
			want:    "v1.2.3 (abc1234)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origVersion, origCommit := Version, Commit
			defer func() { Version, Commit = origVersion, origCommit }()

			Version = tt.version
			Commit = tt.commit

			if got := String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestString_DefaultContainsDev(t *testing.T) {
	if !strings.Contains(String(), "dev") {
		t.Errorf("String() = %q, want it to contain %q", String(), "dev")
	}
}
