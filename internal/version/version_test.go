package version

import (
	"runtime/debug"
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
			origVersion, origCommit, origRead := Version, Commit, readBuildSettings
			defer func() { Version, Commit, readBuildSettings = origVersion, origCommit, origRead }()
			readBuildSettings = func() []debug.BuildSetting { return nil }

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

func TestVCSCommit(t *testing.T) {
	tests := []struct {
		name     string
		settings []debug.BuildSetting
		want     string
	}{
		{name: "no vcs settings", settings: nil, want: ""},
		{
			name:     "clean revision is shortened",
			settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "6ae754dd1989af09eebe5035d3963fc9e9a68088"}, {Key: "vcs.modified", Value: "false"}},
			want:     "6ae754d",
		},
		{
			name:     "modified tree is marked dirty",
			settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "0b7814154466218f2ad3ac96376a926e7de13052"}, {Key: "vcs.modified", Value: "true"}},
			want:     "0b78141-dirty",
		},
		{
			name:     "short revision is kept whole",
			settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "abc12"}},
			want:     "abc12",
		},
		{
			name:     "modified without a revision is ignored",
			settings: []debug.BuildSetting{{Key: "vcs.modified", Value: "true"}},
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := vcsCommit(tt.settings); got != tt.want {
				t.Errorf("vcsCommit() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestString_FallsBackToVCSCommit(t *testing.T) {
	origVersion, origCommit, origRead := Version, Commit, readBuildSettings
	defer func() { Version, Commit, readBuildSettings = origVersion, origCommit, origRead }()
	readBuildSettings = func() []debug.BuildSetting {
		return []debug.BuildSetting{{Key: "vcs.revision", Value: "6ae754dd1989af09eebe5035d3963fc9e9a68088"}}
	}

	Version, Commit = "dev", unstampedCommit
	if got, want := String(), "dev (6ae754d)"; got != want {
		t.Errorf("unstamped String() = %q, want %q", got, want)
	}

	Version, Commit = "v1.2.3", "abc1234"
	if got, want := String(), "v1.2.3 (abc1234)"; got != want {
		t.Errorf("stamped String() = %q, want %q", got, want)
	}
}
