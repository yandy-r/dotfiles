package plasma

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestDiff_ReturnsResultsAlongsideDriftError: a caller that wants to RENDER the
// drift must not have the results taken away because drift exists.
func TestDiff_ReturnsResultsAlongsideDriftError(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	seedRepo(t, repoRoot, "kwinrc", "[G]\nk=repo\n")
	seedLive(t, configDir, "kwinrc", "[G]\nk=live\n")

	diffs, err := e.Diff([]FileSpec{specFor(t, "kwinrc")}, Options{})
	var drift *DriftError
	if !errors.As(err, &drift) {
		t.Fatalf("Diff error = %v, want *DriftError", err)
	}
	if len(drift.Files) != 1 || drift.Files[0] != "kwinrc" {
		t.Errorf("DriftError.Files = %v, want [kwinrc]", drift.Files)
	}
	if len(diffs) != 1 || len(diffs[0].Keys) != 1 {
		t.Fatalf("diffs = %+v, want one file with one key diff", diffs)
	}
	if diffs[0].Keys[0].Kind != DiffValueMismatch {
		t.Errorf("Kind = %v, want DiffValueMismatch", diffs[0].Keys[0].Kind)
	}
}

// TestDiff_PresenceMatrix mirrors §6.6's diff column.
func TestDiff_PresenceMatrix(t *testing.T) {
	t.Run("absent both sides yields no entry", func(t *testing.T) {
		e, repoRoot, _ := testEngine(t)
		seedRepo(t, repoRoot, "keepalive", "[G]\nk=1\n")

		diffs, err := e.Diff([]FileSpec{specFor(t, "kwinrc")}, Options{})
		if err != nil {
			t.Fatalf("Diff error = %v", err)
		}
		if len(diffs) != 0 {
			t.Errorf("diffs = %+v, want none", diffs)
		}
	})

	t.Run("repo absent is not drift", func(t *testing.T) {
		e, repoRoot, configDir := testEngine(t)
		seedRepo(t, repoRoot, "keepalive", "[G]\nk=1\n")
		seedLive(t, configDir, "kwinrc", "[G]\nk=live\n")

		diffs, err := e.Diff([]FileSpec{specFor(t, "kwinrc")}, Options{})
		if err != nil {
			t.Fatalf("Diff error = %v, want nil (an un-exported file asserts nothing)", err)
		}
		if len(diffs) != 1 || diffs[0].RepoPresent || !diffs[0].LivePresent {
			t.Fatalf("diffs = %+v, want one live-only entry", diffs)
		}
		if len(diffs[0].Keys) != 0 || diffs[0].Drifted() {
			t.Error("a repo-absent file must report no keys and no drift")
		}
	})

	t.Run("live absent drifts on every repo key", func(t *testing.T) {
		e, repoRoot, _ := testEngine(t)
		seedRepo(t, repoRoot, "kwinrc", "[G]\na=1\nb=2\n")

		diffs, err := e.Diff([]FileSpec{specFor(t, "kwinrc")}, Options{})
		var drift *DriftError
		if !errors.As(err, &drift) {
			t.Fatalf("Diff error = %v, want *DriftError", err)
		}
		if len(diffs[0].Keys) != 2 {
			t.Fatalf("keys = %+v, want both repo keys", diffs[0].Keys)
		}
		for _, k := range diffs[0].Keys {
			if k.Kind != DiffMissingLive {
				t.Errorf("%s Kind = %v, want DiffMissingLive", k.Key, k.Kind)
			}
		}
	})
}

// TestDiff_LiveOnlyKeysAreNotDrift is the subset rule seen from the diff side.
func TestDiff_LiveOnlyKeysAreNotDrift(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	seedRepo(t, repoRoot, "kwinrc", "[G]\nk=1\n")
	seedLive(t, configDir, "kwinrc", "[G]\nk=1\nextra=2\n\n[Other]\nz=3\n")

	diffs, err := e.Diff([]FileSpec{specFor(t, "kwinrc")}, Options{})
	if err != nil {
		t.Fatalf("Diff error = %v, want nil", err)
	}
	if diffs[0].Drifted() {
		t.Error("live-only keys made a converged file drift")
	}
	var informational int
	for _, k := range diffs[0].Keys {
		if k.Kind == DiffMissingRepo {
			informational++
		}
	}
	if informational != 2 {
		t.Errorf("DiffMissingRepo count = %d, want 2", informational)
	}
}

// TestDiff_RawUsesByteEquality: kdedefaults/package has no keys, so its whole
// content is compared as one pseudo-entry.
func TestDiff_RawUsesByteEquality(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	seedRepo(t, repoRoot, "kdedefaults/package", "org.kde.breezedark.desktop\n")
	seedLive(t, configDir, "kdedefaults/package", "org.kde.breeze.desktop\n")

	diffs, err := e.Diff([]FileSpec{specFor(t, "kdedefaults/package")}, Options{})
	var drift *DriftError
	if !errors.As(err, &drift) {
		t.Fatalf("Diff error = %v, want *DriftError", err)
	}
	if len(diffs[0].Keys) != 1 || diffs[0].Keys[0].Key != rawContentKey {
		t.Fatalf("keys = %+v, want one %q entry", diffs[0].Keys, rawContentKey)
	}

	seedLive(t, configDir, "kdedefaults/package", "org.kde.breezedark.desktop\n")
	if _, err := e.Diff([]FileSpec{specFor(t, "kdedefaults/package")}, Options{}); err != nil {
		t.Errorf("Diff error = %v after matching bytes, want nil", err)
	}
}

// TestValidate_IsDiff proves the "one comparator" invariant: Validate cannot be
// a second implementation free to disagree with the plan.
func TestValidate_IsDiff(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	seedRepo(t, repoRoot, "kwinrc", "[G]\nk=repo\n")
	seedLive(t, configDir, "kwinrc", "[G]\nk=live\nextra=1\n")

	files := []FileSpec{specFor(t, "kwinrc")}
	wantDiffs, wantErr := e.Diff(files, Options{})
	gotDiffs, gotErr := e.Validate(files, Options{})

	if !reflect.DeepEqual(gotDiffs, wantDiffs) {
		t.Errorf("Validate results differ from Diff:\n got %+v\nwant %+v", gotDiffs, wantDiffs)
	}
	if (gotErr == nil) != (wantErr == nil) || (gotErr != nil && gotErr.Error() != wantErr.Error()) {
		t.Errorf("Validate error = %v, Diff error = %v", gotErr, wantErr)
	}
}

// TestValidate_DetectsDriftAfterAFailedWrite: the post-apply audit is what
// catches a live session racing the write.
func TestValidate_DetectsDrift(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	seedRepo(t, repoRoot, "kwinrc", "[G]\nk=repo\n")
	seedLive(t, configDir, "kwinrc", "[G]\nk=live\n")
	files := []FileSpec{specFor(t, "kwinrc")}

	plan, err := e.PlanApply(files, Options{})
	if err != nil {
		t.Fatalf("PlanApply error = %v", err)
	}
	if _, err := e.Execute(plan, Options{}); err != nil {
		t.Fatalf("Execute error = %v", err)
	}
	if _, err := e.Validate(files, Options{}); err != nil {
		t.Fatalf("Validate error = %v after a successful apply, want nil", err)
	}

	// Simulate a running Plasma rewriting the file behind us.
	seedLive(t, configDir, "kwinrc", "[G]\nk=clobbered\n")
	var drift *DriftError
	if _, err := e.Validate(files, Options{}); !errors.As(err, &drift) {
		t.Fatalf("Validate error = %v, want *DriftError", err)
	}
}

func TestStatus(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	seedRepo(t, repoRoot, "kwinrc", "[G]\nk=repo\n")  // drifted
	seedRepo(t, repoRoot, "katerc", "[G]\nk=1\n")     // missing live
	seedLive(t, configDir, "kwinrc", "[G]\nk=live\n") // drifted
	seedLive(t, configDir, "klipperrc", "[G]\nk=1\n") // missing repo
	// kiorc is absent on both sides and must be counted in neither.

	files := []FileSpec{
		specFor(t, "kwinrc"), specFor(t, "katerc"),
		specFor(t, "klipperrc"), specFor(t, "kiorc"),
	}
	got, err := e.Status(files, Options{Session: SessionLive})
	if err != nil {
		t.Fatalf("Status error = %v, want drift to be absorbed", err)
	}

	if got.Managed != 4 {
		t.Errorf("Managed = %d, want 4", got.Managed)
	}
	if got.RepoPresent != 2 {
		t.Errorf("RepoPresent = %d, want 2", got.RepoPresent)
	}
	if got.LivePresent != 2 {
		t.Errorf("LivePresent = %d, want 2", got.LivePresent)
	}
	if !reflect.DeepEqual(got.MissingLive, []string{"katerc"}) {
		t.Errorf("MissingLive = %v, want [katerc]", got.MissingLive)
	}
	if !reflect.DeepEqual(got.MissingRepo, []string{"klipperrc"}) {
		t.Errorf("MissingRepo = %v, want [klipperrc]", got.MissingRepo)
	}
	if !reflect.DeepEqual(got.DriftedFiles, []string{"kwinrc", "katerc"}) {
		t.Errorf("DriftedFiles = %v, want [kwinrc katerc]", got.DriftedFiles)
	}
	if got.DriftedKeys != 2 {
		t.Errorf("DriftedKeys = %d, want 2", got.DriftedKeys)
	}
}

func TestDiff_RecommendedChangesTheQuestion(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	seedRepo(t, repoRoot, "kwinrc", "[Windows]\nRollOverDesktops=false\n")
	seedRecommended(t, repoRoot, "kwinrc", "[Windows]\nRollOverDesktops=true\n")
	seedLive(t, configDir, "kwinrc", "[Windows]\nRollOverDesktops=false\n")
	files := []FileSpec{specFor(t, "kwinrc")}

	if _, err := e.Diff(files, Options{}); err != nil {
		t.Fatalf("plain Diff error = %v, want nil (converged on the snapshot)", err)
	}
	var drift *DriftError
	if _, err := e.Diff(files, Options{Recommended: true}); !errors.As(err, &drift) {
		t.Fatalf("Diff --recommended error = %v, want *DriftError", err)
	}
}

// TestDiff_RecommendedOnlyFileIsCompared is the diff half of the overlay
// regression: diffFile short-circuited on the missing plasma/files/ copy before
// the --recommended overlay was consulted, so the four recommended-only files
// were reported CONVERGED while none of their settings had ever been applied —
// defeating the verification step both READMEs prescribe.
func TestDiff_RecommendedOnlyFileIsCompared(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	seedRepo(t, repoRoot, "keepalive", "[G]\nk=1\n")
	seedRecommended(t, repoRoot, "baloofilerc", "[Basic Settings]\nIndexing-Enabled=false\n")
	seedLive(t, configDir, "baloofilerc", "[Basic Settings]\nIndexing-Enabled=true\n")
	files := []FileSpec{specFor(t, "baloofilerc")}

	// Without the overlay the repo asserts nothing about this file.
	diffs, err := e.Diff(files, Options{})
	if err != nil {
		t.Fatalf("Diff error = %v, want nil", err)
	}
	if len(diffs) != 1 || diffs[0].Drifted() {
		t.Fatalf("diffs = %+v, want one non-drifted entry", diffs)
	}

	diffs, err = e.Diff(files, Options{Recommended: true})
	var drift *DriftError
	if !errors.As(err, &drift) {
		t.Fatalf("Diff --recommended error = %v, want *DriftError", err)
	}
	if len(diffs) != 1 || !diffs[0].Drifted() {
		t.Fatalf("diffs = %+v, want the overlay's key reported as drift", diffs)
	}
	if diffs[0].Keys[0].Kind != DiffValueMismatch {
		t.Errorf("Kind = %v, want DiffValueMismatch", diffs[0].Keys[0].Kind)
	}
}

// TestDiff_ExpectedVolatileKeysAreDistinguished: a live-only key the strip
// table deliberately removes is the design working, not unmanaged state. Mixed
// into one plasma.only-live= list it buried the genuinely unmanaged keys under
// hundreds of [Tiling][<uuid>] and Id_* entries, so it is classified apart and
// suppressed unless asked for.
func TestDiff_ExpectedVolatileKeysAreDistinguished(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	seedRepo(t, repoRoot, "kwinrc", "[Windows]\nFocusPolicy=ClickToFocus\n")
	seedLive(t, configDir, "kwinrc",
		"[Windows]\nFocusPolicy=ClickToFocus\n"+
			"\n[Desktops]\nId_1=00000000-0000-4000-8000-000000000001\nNumber=4\n"+
			"\n[Tiling][6a1f]\nlayout=x\n")
	files := []FileSpec{specFor(t, "kwinrc")}

	// By default only the genuinely unmanaged key is reported.
	diffs, err := e.Diff(files, Options{})
	if err != nil {
		t.Fatalf("Diff error = %v, want nil", err)
	}
	if len(diffs[0].Keys) != 1 {
		t.Fatalf("keys = %+v, want only the unmanaged [Desktops]/Number", diffs[0].Keys)
	}
	if diffs[0].Keys[0].Key != "Number" || diffs[0].Keys[0].Kind != DiffMissingRepo {
		t.Errorf("key = %+v, want Number as DiffMissingRepo", diffs[0].Keys[0])
	}

	// --show-volatile keeps them, tagged so text and JSON can tell them apart.
	diffs, err = e.Diff(files, Options{ShowVolatile: true})
	if err != nil {
		t.Fatalf("Diff error = %v, want nil", err)
	}
	byKey := make(map[string]KeyDiff, len(diffs[0].Keys))
	for _, k := range diffs[0].Keys {
		byKey[k.Key] = k
	}
	if got := byKey["Id_1"].Kind; got != DiffExpectedVolatile {
		t.Errorf("Id_1 Kind = %v, want DiffExpectedVolatile (a [Desktops]/Id_* key rule)", got)
	}
	if got := byKey["layout"].Kind; got != DiffExpectedVolatile {
		t.Errorf("layout Kind = %v, want DiffExpectedVolatile (a [Tiling][*] group rule)", got)
	}
	if got := byKey["Number"].Kind; got != DiffMissingRepo {
		t.Errorf("Number Kind = %v, want DiffMissingRepo: no rule explains it", got)
	}
	if got := DiffExpectedVolatile.String(); got != "expected-volatile" {
		t.Errorf("DiffExpectedVolatile.String() = %q, want %q", got, "expected-volatile")
	}
	if diffs[0].Drifted() {
		t.Error("an expected-volatile key made the file drift")
	}

	// Neither flavour of live-only key counts towards status's drift keys.
	st, err := e.Status(files, Options{ShowVolatile: true})
	if err != nil {
		t.Fatalf("Status error = %v", err)
	}
	if st.DriftedKeys != 0 {
		t.Errorf("DriftedKeys = %d, want 0", st.DriftedKeys)
	}
}

// TestDiff_UnreadableFileIsAnError: diff must not report a file it could not
// read as "missing live" — that is the same conflation apply used to act on,
// seen from the read-only side.
func TestDiff_UnreadableFileIsAnError(t *testing.T) {
	e, repoRoot, configDir := testEngine(t)
	seedRepo(t, repoRoot, "kwinrc", "[G]\nk=repo\n")
	if err := os.MkdirAll(filepath.Join(configDir, "kwinrc"), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}

	_, err := e.Diff([]FileSpec{specFor(t, "kwinrc")}, Options{})
	var unreadable *UnreadableFileError
	if !errors.As(err, &unreadable) {
		t.Fatalf("Diff error = %v, want *UnreadableFileError", err)
	}
}
