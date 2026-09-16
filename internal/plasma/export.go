package plasma

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// PlanExport computes the live → repo plan: read each live file, strip its
// volatile groups/keys/values, serialise canonically, and emit ActionExport,
// ActionCopyRaw, ActionPrune, or ActionSkip per file. It reads the filesystem
// and writes nothing.
//
// Every candidate is serialised into FileAction.Content here, at plan time,
// which is what lets the privacy scan (§14.1) reject an absolute home path
// BEFORE anything is written and lets --dry-run preview the exact bytes.
//
// DefaultFiles is a list of CANDIDATES: a file that reduces to zero entries
// after stripping is not written (ActionSkip), and a previously-written repo
// file that now reduces to zero is deleted (ActionPrune). That keeps the table
// stable as the user's config grows, without committing empty stubs.
func (e *Engine) PlanExport(files []FileSpec, opts Options) (*Plan, error) {
	if _, err := os.Stat(e.repoRoot); err != nil {
		return nil, &MissingSourceTreeError{Root: e.repoRoot}
	}

	plan := &Plan{BackupDir: e.newBackupDir()}
	keepColors, advisory := e.colorSchemeGuardFor(files)
	if advisory != "" {
		plan.Advisories = append(plan.Advisories, advisory)
	}

	liveFound := false
	for _, spec := range files {
		if err := checkSpec(spec); err != nil {
			return nil, err
		}

		livePath := e.livePath(spec)
		repoPath := e.repoPath(spec)

		data, err := os.ReadFile(livePath) //nolint:gosec // table-derived, denylist-checked path
		if err != nil {
			plan.Files = append(plan.Files, FileAction{
				Spec: spec, Kind: ActionSkip, SrcAbs: repoPath, DestAbs: livePath,
				Reason: "source missing",
			})
			continue
		}
		liveFound = true

		action, err := e.planExportFile(spec, livePath, repoPath, data, keepColors)
		if err != nil {
			return nil, err
		}
		if opts.Backup && action.Kind != ActionSkip {
			plan.Files = append(plan.Files, FileAction{
				Spec: spec, Kind: ActionBackup, SrcAbs: repoPath, DestAbs: livePath,
				Reason: "back up the live file before re-stripping",
			})
		}
		plan.Files = append(plan.Files, action)
	}

	if !liveFound && len(files) > 0 {
		return nil, &NoLiveFilesError{ConfigDir: e.configDir}
	}

	orderActions(plan.Files)
	return plan, nil
}

// planExportFile builds the single action for one candidate whose live bytes
// have already been read.
func (e *Engine) planExportFile(
	spec FileSpec,
	livePath, repoPath string,
	data []byte,
	keepColors bool,
) (FileAction, error) {
	act := FileAction{
		Spec: spec, SrcAbs: repoPath, DestAbs: livePath,
		ToRepo: true, Mode: repoMode(repoPath),
	}
	if linked, _ := e.isLinkedIntoRepo(livePath, repoPath); linked {
		// Reading and re-writing the same inode is a no-op when converged and a
		// deliberate re-strip when KDE has written volatile state into the repo
		// file through the symlink. Correct, but surprising enough to name.
		act.Reason = "re-stripping a linked file"
	}

	if spec.Format == FormatRaw {
		act.Kind = ActionCopyRaw
		act.Content = data
		if act.Reason == "" {
			act.Reason = "verbatim copy"
		}
		if existing, err := os.ReadFile(repoPath); err == nil && bytes.Equal(existing, data) { //nolint:gosec // repo-relative
			act.Kind = ActionSkip
			act.Content = nil
			act.Reason = "already converged"
		}
		if err := e.scanPrivacy(spec.Path, act.Content); err != nil {
			return FileAction{}, err
		}
		return act, nil
	}

	doc, err := Parse(livePath, data)
	if err != nil {
		return FileAction{}, err
	}
	stripped := Strip(doc, spec, keepColors)
	if !hasEntries(stripped) {
		if _, statErr := os.Stat(repoPath); statErr == nil {
			act.Kind = ActionPrune
			act.Reason = "no keys survive the volatile strip"
			return act, nil
		}
		act.Kind = ActionSkip
		act.Reason = "empty after strip"
		return act, nil
	}

	content, err := Write(dropEmptyGroups(stripped))
	if err != nil {
		return FileAction{}, fmt.Errorf("plasma: serialise %s: %w", spec.Path, err)
	}
	if err := e.scanPrivacy(spec.Path, content); err != nil {
		return FileAction{}, err
	}

	act.Kind = ActionExport
	act.Content = content
	if act.Reason == "" {
		act.Reason = "capture the stripped live file"
	}
	if existing, rerr := os.ReadFile(repoPath); rerr == nil && bytes.Equal(existing, content) { //nolint:gosec // repo-relative
		act.Kind = ActionSkip
		act.Content = nil
		act.Reason = "already converged"
	}
	return act, nil
}

// dropEmptyGroups removes groups the strip emptied. An empty group would
// otherwise be written as a bare header carrying no intent, and every such
// header is diff noise on the next Plasma upgrade. A group the user marked
// immutable is kept even when empty, because the marker itself is the intent.
func dropEmptyGroups(doc *Doc) *Doc {
	out := &Doc{Immutable: doc.Immutable}
	for _, g := range doc.Groups {
		if len(g.Entries) == 0 && !g.Immutable {
			continue
		}
		out.Groups = append(out.Groups, g)
	}
	return out
}

// repoMode picks the mode for a repo-side write: the existing file's mode when
// it exists, else KDE's own 0600 for a freshly created config file.
func repoMode(path string) os.FileMode {
	if fi, err := os.Stat(path); err == nil {
		return fi.Mode().Perm()
	}
	return livePerm
}

// isLinkedIntoRepo reports whether a live path is a symlink whose target is the
// repo copy — the state `dotf plasma apply --link` leaves behind.
func (e *Engine) isLinkedIntoRepo(livePath, repoPath string) (bool, string) {
	fi, err := os.Lstat(livePath)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		return false, ""
	}
	target, err := os.Readlink(livePath)
	if err != nil {
		return false, ""
	}
	return target == repoPath, target
}

// colorSchemeGuardFor resolves the §7.4 colour-scheme guard, but only when the
// selection actually contains the file the guard protects. A --only run that
// never touches kdeglobals must not pay for two file reads and a directory
// scan.
func (e *Engine) colorSchemeGuardFor(files []FileSpec) (keep bool, advisory string) {
	for _, spec := range files {
		if spec.Path == "kdeglobals" {
			return e.colorSchemeGuard()
		}
	}
	return false, ""
}

// homePathPatterns are the §14.1 privacy rules. They deliberately do NOT match
// the six-character literal "$HOME": that is an unexpanded KConfig [$e]
// reference and is exactly the portable form the design wants — the recommended
// baloofilerc "folders[$e]=$HOME/" entry relies on it.
var homePathPatterns = []struct {
	name string
	re   *regexp.Regexp
}{
	{"/home/<user>/", regexp.MustCompile(`/home/[^/\s]+/`)},
	{"/Users/<user>/", regexp.MustCompile(`/Users/[^/\s]+/`)},
	{"file:///home/", regexp.MustCompile(`file:///home/`)},
	{"file:///Users/", regexp.MustCompile(`file:///Users/`)},
}

// scanPrivacy rejects content about to be written into plasma/files/ that
// carries an absolute home path. It runs at plan time, so a violation aborts
// the run with nothing written.
//
// The correct fix for a hit is either a new strip rule or moving the setting
// into plasma/recommended/ re-authored with a [$e]-flagged $HOME-relative
// value.
func (e *Engine) scanPrivacy(specPath string, content []byte) error {
	if len(content) == 0 {
		return nil
	}
	home := e.homeDir()
	for i, line := range strings.Split(string(content), "\n") {
		for _, p := range homePathPatterns {
			if p.re.MatchString(line) {
				return &PrivacyLeakError{Path: specPath, Line: i + 1, Pattern: p.name}
			}
		}
		if home != "" && home != "/" && strings.Contains(line, home) {
			return &PrivacyLeakError{Path: specPath, Line: i + 1, Pattern: "the expanded home directory " + home}
		}
	}
	return nil
}

// orderActions sorts a plan's files by ApplyOrder phase, then
// path-lexicographically within a phase. The sort is STABLE, which is what
// keeps a file's ActionBackup entry immediately before the action it protects:
// the two share a phase and a path, so their relative order is the order the
// planner appended them in.
func orderActions(files []FileAction) {
	sort.SliceStable(files, func(i, j int) bool {
		if files[i].Spec.ApplyOrder != files[j].Spec.ApplyOrder {
			return files[i].Spec.ApplyOrder < files[j].Spec.ApplyOrder
		}
		return files[i].Spec.Path < files[j].Spec.Path
	})
}
