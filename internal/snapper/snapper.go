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

// Create runs `sudo snapper create -d "dotf apply: <desc>"`. It is a no-op
// returning nil when Available is false (non-Linux, or snapper not on PATH),
// recording zero Run calls. A snapper failure is returned wrapped; the caller
// treats it as non-fatal.
func (s *Snapshotter) Create(desc string) error {
	if !s.Available() {
		return nil
	}
	if err := s.r.Run("sudo", binary, "create", "-d", "dotf apply: "+desc); err != nil {
		return fmt.Errorf("snapper: create snapshot: %w", err)
	}
	return nil
}
