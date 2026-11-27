package lockfile

import (
	"fmt"
	"strings"
)

// Validate rejects a lock that carries an artifact with no checksum. An empty
// sha256 is never a valid lock entry: on that host the lock hit would win with
// nothing to verify, and Install would fail closed forever while a re-lock
// re-recorded the same empty value. Parse and Write both call it, so an empty
// checksum can neither be read from nor written to dotf.lock.
func (l *Lock) Validate() error {
	for _, p := range l.Packages {
		for _, a := range p.Artifacts {
			if strings.TrimSpace(a.SHA256) == "" {
				return fmt.Errorf("lockfile: package %q artifact %s has an empty sha256", p.Name, artifactTarget(a))
			}
		}
	}
	return nil
}

// artifactTarget renders an artifact's target as "os/arch", or "any" for a
// platform-agnostic pin.
func artifactTarget(a Artifact) string {
	if a.OS == "" && a.Arch == "" {
		return "any"
	}
	return a.OS + "/" + a.Arch
}
