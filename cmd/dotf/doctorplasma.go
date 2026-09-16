package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/plasma"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// checkPlasma composes the plasma engine's read-only Support/Diff into the two
// plasma checks: plasma.drift warns when a managed file has a difference apply
// would act on, and plasma.repo warns when plasma/files/ has never been
// exported. Neither ever fails — an unsynced desktop is not a broken host — and
// on an OS plasma does not target both are omitted entirely (not ok, not warn),
// the same way `dotf plasma` refuses such a host outright rather than reporting
// on it.
//
// The omission also covers an unresolvable repo root or a non-absolute config
// dir: symlinks.broken already fails on the former, and the latter would point
// the engine at paths under the current working directory.
//
// dataHome is the caller-resolved $XDG_DATA_HOME rather than an engine-side
// derivation so the colour-scheme guard resolves identically here and in
// `dotf plasma diff` — see plasmaDriftCheck on why the two cannot disagree.
func checkPlasma(repoRoot, configDir, dataHome string, info platform.Info, r exec.Runner) []doctorCheck {
	if plasma.Support(info) == plasma.SupportUnsupported {
		return nil
	}
	if !filepath.IsAbs(repoRoot) || !filepath.IsAbs(configDir) {
		return nil
	}

	if fi, err := os.Stat(filepath.Join(repoRoot, "plasma", "files")); err != nil || !fi.IsDir() {
		// Nothing has been exported, so there is nothing to compare against and
		// the live desktop is not read at all. That keeps an un-exported host
		// fast and keeps the drift row from claiming a comparison it never made.
		return []doctorCheck{
			{
				Category: "plasma", Name: "drift", Status: checkOK,
				Detail: "no exported plasma files to compare",
			},
			{
				Category: "plasma", Name: "repo", Status: checkWarn,
				Detail: "plasma/files/ does not exist",
				Remedy: "run `dotf plasma export`",
			},
		}
	}

	return []doctorCheck{
		plasmaDriftCheck(plasma.NewEngine(repoRoot, configDir, dataHome, plasmaSystemColorSchemeDirs(), r)),
		{Category: "plasma", Name: "repo", Status: checkOK, Detail: "plasma/files/ present"},
	}
}

// plasmaDriftCheck resolves drift through the engine's Diff over the full
// managed table with Options{Validate: true} and every other switch false — a
// pure read of both sides, no mutation and no privilege. Drifted files come from
// FileDiff.Drifted, the package's single definition of "what apply would act
// on", so doctor and `dotf plasma diff` cannot disagree.
//
// A *DriftError is the expected companion of a drifted result and is absorbed
// (Status does the same); any other error is reported as a warn rather than a
// fail, because a plasma file dotf cannot read still does not make the host
// unhealthy.
func plasmaDriftCheck(eng *plasma.Engine) doctorCheck {
	files := plasma.DefaultFiles()
	diffs, err := eng.Diff(files, plasma.Options{Validate: true})

	var drift *plasma.DriftError
	if err != nil && !errors.As(err, &drift) {
		return doctorCheck{
			Category: "plasma", Name: "drift", Status: checkWarn,
			Detail: "plasma diff failed: " + err.Error(),
			Remedy: "resolve the offending plasma file, then re-run `dotf plasma diff`",
		}
	}

	var items []doctorItem
	for _, fd := range diffs {
		if fd.Drifted() {
			items = append(items, doctorItem{Kind: "plasma", Action: "drift", Dest: fd.Spec.Path})
		}
	}
	if len(items) > 0 {
		return doctorCheck{
			Category: "plasma", Name: "drift", Status: checkWarn,
			Detail: fmt.Sprintf("%d of %d managed plasma file(s) drifted", len(items), len(files)),
			Remedy: "run `dotf plasma apply`",
			Items:  items,
		}
	}
	return doctorCheck{
		Category: "plasma", Name: "drift", Status: checkOK,
		Detail: fmt.Sprintf("all %d managed plasma files converged", len(files)),
	}
}
