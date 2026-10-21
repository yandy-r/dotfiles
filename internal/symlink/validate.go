package symlink

import (
	"os"
	"path/filepath"

	"github.com/yandy-r/dotfiles/internal/platform"
)

// Validate checks every applicable, non-Copy managed dest and returns a
// per-dest result. It is not gated by DryRun — it reports actual on-disk
// state. A real file where a symlink was expected is Broken, and so is a
// dangling or looping symlink whose source exists (os.Stat on the dest fails).
// A dangling symlink whose source is also missing is not flagged: dotf cannot
// fix it and Plan skips it. A missing dest is neither Valid nor Broken. When
// any dest is Broken it returns a BrokenLinkError alongside the results. A
// dest that already is its source through a symlinked parent dir (the case
// Plan skips as already correct) is Valid. The Zed copy dest is excluded from
// validation.
func (e *Engine) Validate(specs []LinkSpec, info platform.Info, opts Options) ([]ValidationResult, error) {
	var results []ValidationResult
	var broken []string

	for _, spec := range specs {
		if !applies(spec, info, opts) || spec.Copy {
			continue
		}

		destAbs := filepath.Join(e.home, spec.Dest)
		res := ValidationResult{Dest: spec.Dest}

		if e.destIsSource(spec, destAbs) {
			res.Valid = true
		} else if fi, err := os.Lstat(destAbs); err == nil {
			switch {
			case fi.Mode()&os.ModeSymlink == 0:
				res.Broken = true
			case e.danglingWithSource(spec, destAbs):
				res.Broken = true
			default:
				res.Valid = true
			}
			if res.Broken {
				broken = append(broken, spec.Dest)
			}
		}
		results = append(results, res)
	}

	if len(broken) > 0 {
		return results, &BrokenLinkError{Dests: broken}
	}
	return results, nil
}

// danglingWithSource reports whether the symlink at destAbs fails to resolve
// (dangling or a loop) while the spec's source exists, i.e. a link dotf manages
// and could repair.
func (e *Engine) danglingWithSource(spec LinkSpec, destAbs string) bool {
	if _, err := os.Stat(destAbs); err == nil {
		return false
	}
	_, err := os.Stat(filepath.Join(e.repoRoot, spec.Src))
	return err == nil
}

// destIsSource reports whether destAbs, reached through symlinked parent dirs,
// already is the spec's existing source (e.g. ~/.config/zed -> $REPO/zed). Plan
// skips such a dest as already correct, so Validate must treat it as Valid.
func (e *Engine) destIsSource(spec LinkSpec, destAbs string) bool {
	srcReal, err := filepath.EvalSymlinks(filepath.Join(e.repoRoot, spec.Src))
	if err != nil {
		return false
	}
	destLoc, ok := resolvedLocation(destAbs)
	return ok && destLoc == srcReal
}
