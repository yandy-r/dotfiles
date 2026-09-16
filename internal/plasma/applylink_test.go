package plasma

// Regression tests for the --link/--unlink state machine in applylink.go. Each
// one pins a behaviour that was WRONG before applylink.go existed: --link had
// no way back, and the guard refused runs over files it would not have touched.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// linkKwinrc links <configDir>/kwinrc at plasma/files/kwinrc, leaving the
// engine in exactly the state `plasma apply --link --backup --yes` produces.
func linkKwinrc(t *testing.T, repoRoot, configDir, repoData string) (repoPath, livePath string) {
	t.Helper()
	repoPath = seedRepo(t, repoRoot, "kwinrc", repoData)
	livePath = filepath.Join(configDir, "kwinrc")
	if err := os.Symlink(repoPath, livePath); err != nil {
		t.Fatalf("Symlink error = %v", err)
	}
	// Sanity: the state under test really is a symlink into the repo.
	if fi, err := os.Lstat(livePath); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("Lstat(%q) = %v, %v; want a symlink", livePath, fi, err)
	}
	return repoPath, livePath
}

// TestPlanApply_UnlinkRestoresCopyMode is the fix for --link being a ONE-WAY
// DOOR. Reading a linked live path reads the repo's own bytes, so the merge was
// always converged and every subsequent apply planned ActionSkip — the symlink
// survived --backup, --only, everything short of deleting it by hand.
func TestPlanApply_UnlinkRestoresCopyMode(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	const repoData = "[Windows]\nFocusStealingPreventionLevel=1\n"
	repoPath, livePath := linkKwinrc(t, repoRoot, configDir, repoData)

	opts := Options{Unlink: true}
	plan, err := e.PlanApply([]FileSpec{specFor(t, "kwinrc")}, opts)
	if err != nil {
		t.Fatalf("PlanApply(--unlink) error = %v", err)
	}
	act, ok := planFileFor(t, plan, "kwinrc")
	if !ok || act.Kind != ActionUnlink {
		t.Fatalf("Kind = %v (%q), want ActionUnlink", act.Kind, act.Reason)
	}

	rep, err := e.Execute(plan, opts)
	if err != nil {
		t.Fatalf("Execute error = %v", err)
	}
	if len(rep.Applied) != 1 {
		t.Fatalf("Applied = %d, want 1", len(rep.Applied))
	}

	fi, err := os.Lstat(livePath)
	if err != nil {
		t.Fatalf("Lstat error = %v", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Fatal("the live path is STILL a symlink after --unlink")
	}
	// Nothing was discarded on the way back: the materialised file carries the
	// content KDE was already reading, and the repo copy is untouched.
	if got := readFile(t, livePath); !strings.Contains(got, "FocusStealingPreventionLevel=1") {
		t.Errorf("live content = %q, want the repo's keys", got)
	}
	if got := readFile(t, repoPath); got != repoData {
		t.Errorf("repo file = %q, want it untouched (%q)", got, repoData)
	}

	// And the round trip closes: a second --unlink now has nothing to do.
	plan2, err := e.PlanApply([]FileSpec{specFor(t, "kwinrc")}, opts)
	if err != nil {
		t.Fatalf("PlanApply(--unlink, second) error = %v", err)
	}
	if act2, _ := planFileFor(t, plan2, "kwinrc"); act2.Kind != ActionSkip {
		t.Errorf("second --unlink Kind = %v (%q), want ActionSkip", act2.Kind, act2.Reason)
	}
}

// TestPlanApply_UnlinkRestoresCopyModeForRaw covers the FormatRaw member on the
// same path: bytes.Equal against a symlink's target is trivially true, so the
// raw copy had the identical one-way-door bug.
func TestPlanApply_UnlinkRestoresCopyModeForRaw(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	const rel = "kdedefaults/package"
	repoPath := seedRepo(t, repoRoot, rel, "org.kde.breezedark.desktop\n")
	livePath := filepath.Join(configDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(livePath), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.Symlink(repoPath, livePath); err != nil {
		t.Fatalf("Symlink error = %v", err)
	}

	opts := Options{Unlink: true}
	spec := specFor(t, rel)
	if spec.Format != FormatRaw {
		t.Fatalf("%s Format = %v, want FormatRaw; pick another raw member", rel, spec.Format)
	}
	plan, err := e.PlanApply([]FileSpec{spec}, opts)
	if err != nil {
		t.Fatalf("PlanApply(--unlink) error = %v", err)
	}
	if act, _ := planFileFor(t, plan, rel); act.Kind != ActionUnlink {
		t.Fatalf("Kind = %v, want ActionUnlink", act.Kind)
	}
	if _, err := e.Execute(plan, opts); err != nil {
		t.Fatalf("Execute error = %v", err)
	}
	fi, err := os.Lstat(livePath)
	if err != nil {
		t.Fatalf("Lstat error = %v", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Fatal("the raw live path is STILL a symlink after --unlink")
	}
	if got := readFile(t, livePath); got != "org.kde.breezedark.desktop\n" {
		t.Errorf("live content = %q, want the repo bytes", got)
	}
}

// TestPlanApply_PlainApplyLeavesLinkAlone pins the deliberate half of the
// decision: restoring copy mode is opt-in. A plain apply — the one
// `dotf apply --plasma` runs headlessly — must never tear down a --link
// deployment as a side effect.
func TestPlanApply_PlainApplyLeavesLinkAlone(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	_, livePath := linkKwinrc(t, repoRoot, configDir, "[Windows]\nFocusStealingPreventionLevel=1\n")

	plan, err := e.PlanApply([]FileSpec{specFor(t, "kwinrc")}, Options{})
	if err != nil {
		t.Fatalf("PlanApply error = %v", err)
	}
	act, _ := planFileFor(t, plan, "kwinrc")
	if act.Kind != ActionSkip || !strings.HasPrefix(act.Reason, "already linked") {
		t.Fatalf("Kind = %v (%q), want ActionSkip \"already linked…\"", act.Kind, act.Reason)
	}
	if _, err := e.Execute(plan, Options{}); err != nil {
		t.Fatalf("Execute error = %v", err)
	}
	if fi, err := os.Lstat(livePath); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("a plain apply replaced the symlink (%v, %v)", fi, err)
	}
	// The reason names the way back, so the state is not a dead end.
	if !strings.Contains(act.Reason, "--unlink") {
		t.Errorf("Reason = %q, want it to name --unlink", act.Reason)
	}
}

// TestPlanApply_LinkedTreeIsNotRefused is the fix for the OVER-BROAD guard. A
// fully linked, converged tree made a plain `plasma apply --yes` exit 1 while
// naming every managed file — none of which the run would have touched.
func TestPlanApply_LinkedTreeIsNotRefused(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	specs := []FileSpec{specFor(t, "kwinrc"), specFor(t, "kdeglobals"), specFor(t, "kiorc")}
	for _, spec := range specs {
		repoPath := seedRepo(t, repoRoot, spec.Path, "[G]\nk=repo\n")
		if err := os.Symlink(repoPath, filepath.Join(configDir, spec.Path)); err != nil {
			t.Fatalf("Symlink(%q) error = %v", spec.Path, err)
		}
	}

	plan, err := e.PlanApply(specs, Options{})
	if err != nil {
		t.Fatalf("PlanApply over a fully linked tree = %v, want no refusal", err)
	}
	if got := countKind(plan, ActionSkip); got != len(specs) {
		t.Errorf("ActionSkip count = %d, want %d (every file converged)", got, len(specs))
	}
	if len(plan.LinkConflicts) != 0 {
		t.Errorf("LinkConflicts = %v, want none", plan.LinkConflicts)
	}
}

// TestPlanApply_ConvergedForeignLinkIsNotAConflict narrows the guard on its own
// terms, independently of the linked-tree case: a symlink pointing OUTSIDE the
// repo is a genuine mode transition, but only when the merge would actually
// write. When it is converged the run touches nothing, so there is nothing to
// refuse.
func TestPlanApply_ConvergedForeignLinkIsNotAConflict(t *testing.T) {
	const data = "[Confirmations]\nConfirmDelete=false\n"

	e, repoRoot, configDir := testEngine(t)
	seedRepo(t, repoRoot, "kiorc", data)
	// The foreign target already agrees with the repo, so a plain apply is a
	// no-op even though the destination is a symlink.
	foreign := filepath.Join(t.TempDir(), "elsewhere")
	writeFile(t, foreign, data)
	if err := os.Symlink(foreign, filepath.Join(configDir, "kiorc")); err != nil {
		t.Fatalf("Symlink error = %v", err)
	}

	plan, err := e.PlanApply([]FileSpec{specFor(t, "kiorc")}, Options{})
	if err != nil {
		t.Fatalf("PlanApply = %v, want no refusal for a converged foreign link", err)
	}
	if act, _ := planFileFor(t, plan, "kiorc"); act.Kind != ActionSkip {
		t.Errorf("Kind = %v (%q), want ActionSkip", act.Kind, act.Reason)
	}

	// A DIVERGENT foreign link is still refused: that run would write through
	// a link it does not own.
	writeFile(t, foreign, "[Confirmations]\nConfirmDelete=true\n")
	_, err = e.PlanApply([]FileSpec{specFor(t, "kiorc")}, Options{})
	var conflict *LinkModeConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("PlanApply over a divergent foreign link = %v, want *LinkModeConflictError", err)
	}
	if len(conflict.Paths) != 1 || conflict.Paths[0] != "kiorc" {
		t.Errorf("conflict.Paths = %v, want [kiorc]", conflict.Paths)
	}
}

// TestPlanApply_DryRunPreviewsTheRefusal is the fix for the misleading preview:
// --dry-run is what AUTHORISED the transition, so without carrying the refusal
// the preview printed a confident plan the real run rejected with exit 1.
func TestPlanApply_DryRunPreviewsTheRefusal(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	seedRepo(t, repoRoot, "kwinrc", "[G]\nk=repo\n")
	seedLive(t, configDir, "kwinrc", "[G]\nk=live\n")
	specs := []FileSpec{specFor(t, "kwinrc")}

	plan, err := e.PlanApply(specs, Options{Link: true, DryRun: true})
	if err != nil {
		t.Fatalf("PlanApply(--link --dry-run) error = %v", err)
	}
	if got := plan.LinkConflicts; len(got) != 1 || got[0] != "kwinrc" {
		t.Fatalf("LinkConflicts = %v, want [kwinrc]", got)
	}
	// The preview's warning names exactly what the real run refuses.
	_, rerr := e.PlanApply(specs, Options{Link: true})
	var conflict *LinkModeConflictError
	if !errors.As(rerr, &conflict) {
		t.Fatalf("real run error = %v, want *LinkModeConflictError", rerr)
	}
	if strings.Join(conflict.Paths, ",") != strings.Join(plan.LinkConflicts, ",") {
		t.Errorf("preview named %v, refusal named %v", plan.LinkConflicts, conflict.Paths)
	}
}

// TestPlanApply_LinkAndUnlinkAreExclusive: the two flags are exact inverses, so
// there is no per-file resolution and no order that honours both.
func TestPlanApply_LinkAndUnlinkAreExclusive(t *testing.T) {
	e, repoRoot, _ := testEngine(t)
	seedRepo(t, repoRoot, "kwinrc", "[G]\nk=repo\n")

	_, err := e.PlanApply([]FileSpec{specFor(t, "kwinrc")}, Options{Link: true, Unlink: true})
	var target *LinkUnlinkError
	if !errors.As(err, &target) {
		t.Fatalf("PlanApply(--link --unlink) error = %v, want *LinkUnlinkError", err)
	}
}
