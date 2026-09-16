// Package version holds the build-time version metadata for the dotf CLI.
//
// Version and Commit are intended to be overridden at build time via:
//
//	go build -ldflags "-X github.com/yandy-r/dotfiles/internal/version.Version=v1.2.3 \
//	  -X github.com/yandy-r/dotfiles/internal/version.Commit=abc1234" ./cmd/dotf
package version

import "fmt"

// Version is the semantic version of the build. Overridable via -ldflags.
var Version = "dev"

// Commit is the VCS commit hash of the build. Overridable via -ldflags.
var Commit = "none"

// String formats the version metadata as "<version> (<commit>)", e.g. "dev (none)".
func String() string {
	return fmt.Sprintf("%s (%s)", Version, Commit)
}
