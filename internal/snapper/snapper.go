// Package snapper takes a best-effort snapper rollback snapshot before a dotf
// apply run. It is Linux-only and a silent no-op when snapper is unavailable,
// mirroring bootstrap.sh's create_snapper_snapshot guards. Failures are
// non-fatal: returned for the caller to log as a warning, never aborting apply.
package snapper

import (
	"fmt"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// binary is the snapper executable resolved via the Runner's LookPath.
const binary = "snapper"

// Snapshotter takes a single pre-apply snapper snapshot over an exec.Runner,
// guarded by the detected platform.Info so it is a no-op off the Linux family.
type Snapshotter struct {
	r    exec.Runner
	info platform.Info
}

// New constructs a Snapshotter backed by r and the detected platform info.
func New(r exec.Runner, info platform.Info) *Snapshotter {
	return &Snapshotter{r: r, info: info}
}

// Available reports Linux-family OS AND a resolvable snapper binary. The
// LookPath probe is not recorded in the Fake's Calls, so callers can use it
// without polluting argv assertions.
func (s *Snapshotter) Available() bool {
	if !platform.IsLinuxFamily(s.info.OS) {
		return false
	}
	_, err := s.r.LookPath(binary)
	return err == nil
}

// Outcome is the non-fatal result of a Create call. At most one of Created,
// Skipped is true; Err carries a snapper failure (non-fatal to the caller).
type Outcome struct {
	// Created is true when a snapshot was successfully taken.
	Created bool

	// Skipped is true when Create was a no-op; Reason explains why.
	Skipped bool

	// Reason is the skip reason ("non-linux" or "snapper-not-installed").
	Reason string

	// Err is a non-nil snapper failure. It is non-fatal: the caller logs it
	// as a warning and continues the apply run.
	Err error
}

// Create runs `sudo snapper create -d "dotf apply: <desc>"`. It is a no-op
// (Skipped) when Available is false: a non-Linux OS yields
// Reason "non-linux" and an unresolvable binary yields
// Reason "snapper-not-installed", both recording zero Run calls. A snapper
// failure yields Err (non-fatal).
func (s *Snapshotter) Create(desc string) Outcome {
	if !platform.IsLinuxFamily(s.info.OS) {
		return Outcome{Skipped: true, Reason: "non-linux"}
	}
	if _, err := s.r.LookPath(binary); err != nil {
		return Outcome{Skipped: true, Reason: "snapper-not-installed"}
	}
	if err := s.r.Run("sudo", binary, "create", "-d", "dotf apply: "+desc); err != nil {
		return Outcome{Err: fmt.Errorf("snapper: create snapshot: %w", err)}
	}
	return Outcome{Created: true}
}
