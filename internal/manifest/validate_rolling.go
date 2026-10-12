package manifest

import (
	"fmt"
	"path/filepath"
	"strings"
)

// onRollingRepoRoute reports whether p resolves through the rolling
// latest-release route: an app-artifact or pkg-url package with a repo,
// class = "rolling", and no tag. It mirrors backend.resolveArtifact's routing.
func onRollingRepoRoute(p Package) bool {
	if p.Backend != BackendAppArtifact && p.Backend != BackendPkgURL {
		return false
	}
	return p.Repo != "" && p.Class == ClassRolling && p.Tag == ""
}

// validateRollingRelease checks the rolling latest-release route. There the
// asset is the glob that must select exactly one release asset: it is required,
// must be a well-formed filepath.Match pattern, and is never templated, so a
// "{{" is rejected rather than left to match nothing at install time. preview
// only has meaning on this route and is rejected anywhere else so it is never
// silently ignored.
func validateRollingRelease(p Package) []error {
	if !onRollingRepoRoute(p) {
		if p.Preview {
			return []error{&ValidationError{Package: p.Name, Msg: `preview is only valid on the rolling repo route (app-artifact or pkg-url with repo, class = "rolling", and no tag)`}}
		}
		return nil
	}

	switch _, err := filepath.Match(p.Asset, ""); {
	case p.Asset == "":
		return []error{&ValidationError{Package: p.Name, Msg: fmt.Sprintf("%s rolling repo route requires asset (a glob matching exactly one release asset)", p.Backend)}}
	case strings.Contains(p.Asset, "{{"):
		return []error{&ValidationError{Package: p.Name, Msg: fmt.Sprintf("%s asset %q: templates are not expanded on the rolling repo route; use a glob", p.Backend, p.Asset)}}
	case err != nil:
		return []error{&ValidationError{Package: p.Name, Msg: fmt.Sprintf("%s asset %q is not a valid glob", p.Backend, p.Asset)}}
	}
	return nil
}
