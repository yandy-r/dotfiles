// Package version holds the build-time version metadata for the dotf CLI.
//
// Version and Commit are intended to be overridden at build time via:
//
//	go build -ldflags "-X github.com/yandy-r/dotfiles/internal/version.Version=v1.2.3 \
//	  -X github.com/yandy-r/dotfiles/internal/version.Commit=abc1234" ./cmd/dotf
package version

import (
	"fmt"
	"runtime/debug"
)

// unstampedCommit is Commit's value when no -ldflags override was applied.
const unstampedCommit = "none"

// shortCommitLen matches git's default abbreviated hash length.
const shortCommitLen = 7

// Version is the semantic version of the build. Overridable via -ldflags.
var Version = "dev"

// Commit is the VCS commit hash of the build. Overridable via -ldflags.
var Commit = unstampedCommit

// readBuildSettings returns the toolchain-embedded build settings; a seam for tests.
var readBuildSettings = func() []debug.BuildSetting {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return nil
	}
	return info.Settings
}

// String formats the version metadata as "<version> (<commit>)", e.g.
// "v1.2.3 (abc1234)". An unstamped build (plain `go build` / `go install`)
// reports the toolchain-embedded VCS revision instead, e.g. "dev (6ae754d)" or
// "dev (6ae754d-dirty)", so a stale local binary can be told apart from HEAD;
// "dev (none)" remains only when the build carries no VCS information at all.
func String() string {
	commit := Commit
	if commit == unstampedCommit {
		if vcs := vcsCommit(readBuildSettings()); vcs != "" {
			commit = vcs
		}
	}
	return fmt.Sprintf("%s (%s)", Version, commit)
}

// vcsCommit renders the abbreviated vcs.revision from settings, suffixed with
// "-dirty" when the tree was modified at build time; "" when no revision is
// recorded.
func vcsCommit(settings []debug.BuildSetting) string {
	var revision string
	var modified bool
	for _, s := range settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	if revision == "" {
		return ""
	}
	if len(revision) > shortCommitLen {
		revision = revision[:shortCommitLen]
	}
	if modified {
		revision += "-dirty"
	}
	return revision
}
