package symlink

import (
	"os"
	"path/filepath"

	"github.com/yandy-r/dotfiles/internal/platform"
)

// Validate lstats every applicable, non-Copy managed dest and returns a
// per-dest result. It is not gated by DryRun — it reports actual on-disk state.
// A dangling symlink still reports Valid (lstat sees the link itself), matching
// Ansible. Only a real file where a symlink was expected (exists && !islnk)
// counts as Broken; when any dest is Broken it returns a BrokenLinkError
// alongside the results. The Zed copy dest is excluded from validation.
func (e *Engine) Validate(specs []LinkSpec, info platform.Info, opts Options) ([]ValidationResult, error) {
	var results []ValidationResult
	var broken []string

	for _, spec := range specs {
		if !applies(spec, info, opts) || spec.Copy {
			continue
		}

		destAbs := filepath.Join(e.home, spec.Dest)
		res := ValidationResult{Dest: spec.Dest}

		if fi, err := os.Lstat(destAbs); err == nil {
			if fi.Mode()&os.ModeSymlink != 0 {
				res.Valid = true
			} else {
				res.Broken = true
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
