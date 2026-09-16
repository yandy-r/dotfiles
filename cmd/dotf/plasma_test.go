package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/plasma"
)

// These tests drive runPlasma end to end against t.TempDir()-backed repo, home,
// and KDE config trees with a fake exec.Runner, so nothing touches the real
// host. They deliberately do NOT call t.Parallel: every case uses t.Setenv, and
// the two are mutually exclusive.

// plasmaEnv is one hermetic invocation environment: a dotfiles repo root, a
// $HOME the run must leave untouched, and a KDE config dir pointed at by
// XDG_CONFIG_HOME so "did the run write into $HOME?" stays a trivial check.
type plasmaEnv struct {
	repo   string
	home   string
	config string
}

// newPlasmaEnv wires DOTFILES_ROOT, HOME, and XDG_CONFIG_HOME at fresh temp
// dirs and clears both KDE session variables, so DetectSession resolves to
// "unknown" (a non-graphical shell) rather than inheriting the developer's
// running desktop.
func newPlasmaEnv(t *testing.T) plasmaEnv {
	t.Helper()
	env := plasmaEnv{repo: t.TempDir(), home: t.TempDir(), config: t.TempDir()}
	t.Setenv("DOTFILES_ROOT", env.repo)
	t.Setenv("HOME", env.home)
	t.Setenv("XDG_CONFIG_HOME", env.config)
	t.Setenv("KDE_FULL_SESSION", "")
	t.Setenv("XDG_CURRENT_DESKTOP", "")
	return env
}

// writeLive writes a managed file into the KDE config dir.
func (e plasmaEnv) writeLive(t *testing.T, rel, content string) {
	t.Helper()
	writeUnder(t, e.config, rel, content)
}

// writeRepoPlasma writes a managed file into <repo>/plasma/files/.
func (e plasmaEnv) writeRepoPlasma(t *testing.T, rel, content string) {
	t.Helper()
	writeUnder(t, filepath.Join(e.repo, "plasma", "files"), rel, content)
}

// writeUnder writes content to root/rel, creating parent directories.
func writeUnder(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(p), err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", p, err)
	}
}

// newPlasmaFake forces a Linux-family OS via `uname -s`, so Support() passes
// regardless of the host the test runs on.
func newPlasmaFake(t *testing.T) *exec.Fake {
	t.Helper()
	f := exec.NewFake()
	f.AddCommand("uname", []string{"-s"}, []byte("Linux\n"), nil)
	f.AddCommand("uname", []string{"-m"}, []byte("x86_64\n"), nil)
	return f
}

// newPlasmaDarwinFake forces macOS, exercising the hard host gate.
func newPlasmaDarwinFake(t *testing.T) *exec.Fake {
	t.Helper()
	f := exec.NewFake()
	f.AddCommand("uname", []string{"-s"}, []byte("Darwin\n"), nil)
	f.AddCommand("uname", []string{"-m"}, []byte("arm64\n"), nil)
	return f
}

// runPlasmaCapture invokes runPlasma with buffered streams. A *bytes.Buffer is
// definitively non-interactive, so every apply in these tests takes the
// non-TTY branch unless it passes --yes.
func runPlasmaCapture(t *testing.T, f *exec.Fake, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = runPlasma(args, &out, &errOut, f)
	return code, out.String(), errOut.String()
}

// kiorc is a managed file with NO volatile strip rules, so the fixtures below
// exercise plan/merge/diff without also depending on the strip table.
const (
	liveKiorc = "[Confirmations]\nConfirmDelete=true\n"
	repoKiorc = "[Confirmations]\nConfirmDelete=false\n"
)

func TestRunPlasma_UsageErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "no verb",
			args:    []string{},
			wantErr: "dotf: plasma requires a verb; available: export, apply, diff, status\n",
		},
		{
			name:    "two verbs",
			args:    []string{"diff", "status"},
			wantErr: "dotf: plasma takes exactly one verb, got 2\n",
		},
		{
			name:    "unknown verb",
			args:    []string{"aply"},
			wantErr: "dotf: unknown verb \"aply\"; available: export, apply, diff, status\n",
		},
		{
			name:    "link outside apply",
			args:    []string{"diff", "--link"},
			wantErr: "dotf: --link applies only to \"dotf plasma apply\"\n",
		},
		{
			name:    "reload outside apply",
			args:    []string{"status", "--reload"},
			wantErr: "dotf: --reload applies only to \"dotf plasma apply\"\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newPlasmaEnv(t)
			code, stdout, stderr := runPlasmaCapture(t, newPlasmaFake(t), tt.args...)
			if code != 2 {
				t.Errorf("exit = %d, want 2 (stderr=%q)", code, stderr)
			}
			if stdout != "" {
				t.Errorf("stdout = %q, want empty", stdout)
			}
			if stderr != tt.wantErr {
				t.Errorf("stderr = %q, want %q", stderr, tt.wantErr)
			}
		})
	}
}

// TestRunPlasma_BadFlag proves an unparseable flag is a usage error with a clean
// stdout; the diagnostic itself comes from the flag package on stderr.
func TestRunPlasma_BadFlag(t *testing.T) {
	newPlasmaEnv(t)
	code, stdout, stderr := runPlasmaCapture(t, newPlasmaFake(t), "status", "--bogus")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "bogus") {
		t.Errorf("stderr = %q, want it to name the bad flag", stderr)
	}
}

func TestRunPlasma_UnknownOnlyPath(t *testing.T) {
	newPlasmaEnv(t)
	code, stdout, stderr := runPlasmaCapture(t, newPlasmaFake(t), "status", "--only", "kwinrcx")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if want := "dotf: unknown --only path \"kwinrcx\"\n"; stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
}

// TestRunPlasma_NonLinuxGate proves a non-Linux host is a soft error (exit 1),
// not a usage error: the invocation was well-formed, the host simply has no KDE.
func TestRunPlasma_NonLinuxGate(t *testing.T) {
	newPlasmaEnv(t)
	for _, verb := range plasmaVerbs {
		t.Run(verb, func(t *testing.T) {
			code, stdout, stderr := runPlasmaCapture(t, newPlasmaDarwinFake(t), verb, "--dry-run")
			if code != 1 {
				t.Errorf("exit = %d, want 1", code)
			}
			if stdout != "" {
				t.Errorf("stdout = %q, want empty", stdout)
			}
			if !strings.Contains(stderr, "plasma: unsupported on") {
				t.Errorf("stderr = %q, want the unsupported-OS diagnostic", stderr)
			}
		})
	}
}

// TestRunPlasma_DryRunPreviews proves each verb's --dry-run leads with the
// dry-run=true marker, keeps stderr silent, and leaves $HOME untouched.
func TestRunPlasma_DryRunPreviews(t *testing.T) {
	tests := []struct {
		verb string
		// repo is the plasma/files/kiorc fixture. diff exits 1 on drift by
		// design, so its row is seeded converged to keep this table about the
		// preview CONTRACT (marker line, silent stderr, untouched $HOME) rather
		// than about drift; TestRunPlasma_DiffExitsOneOnDrift owns that rule.
		repo      string
		wantLines []string
	}{
		{verb: "status", repo: repoKiorc, wantLines: []string{"plasma.verb=status", "plasma.managed=1"}},
		{verb: "diff", repo: liveKiorc, wantLines: []string{"plasma.verb=diff"}},
		{verb: "export", repo: repoKiorc, wantLines: []string{"plasma.verb=export", "plasma.export=kiorc"}},
		{verb: "apply", repo: repoKiorc, wantLines: []string{"plasma.verb=apply", "plasma.merge=kiorc"}},
	}

	for _, tt := range tests {
		t.Run(tt.verb, func(t *testing.T) {
			env := newPlasmaEnv(t)
			env.writeLive(t, "kiorc", liveKiorc)
			env.writeRepoPlasma(t, "kiorc", tt.repo)

			code, stdout, stderr := runPlasmaCapture(t, newPlasmaFake(t),
				tt.verb, "--dry-run", "--only", "kiorc")
			if code != 0 {
				t.Fatalf("exit = %d, want 0 (stderr=%q)", code, stderr)
			}
			if stderr != "" {
				t.Errorf("stderr = %q, want empty on a preview", stderr)
			}
			if !strings.HasPrefix(stdout, "dry-run=true\n") {
				t.Errorf("stdout = %q, want it to lead with dry-run=true", stdout)
			}
			if !strings.Contains(stdout, "plasma.session=unknown") {
				t.Errorf("stdout = %q, want a plasma.session line", stdout)
			}
			for _, want := range tt.wantLines {
				if !strings.Contains(stdout, want) {
					t.Errorf("stdout = %q, want it to contain %q", stdout, want)
				}
			}

			entries, err := os.ReadDir(env.home)
			if err != nil {
				t.Fatalf("ReadDir(home) error = %v", err)
			}
			if len(entries) != 0 {
				t.Errorf("dry run wrote %d entries into $HOME, want 0", len(entries))
			}
		})
	}
}

// TestRunPlasma_LoopParseOrderIndependent proves the positional loop-parse makes
// `plasma export --dry-run` and `plasma --dry-run export` identical.
func TestRunPlasma_LoopParseOrderIndependent(t *testing.T) {
	run := func(args ...string) (int, string) {
		env := newPlasmaEnv(t)
		env.writeLive(t, "kiorc", liveKiorc)
		code, stdout, stderr := runPlasmaCapture(t, newPlasmaFake(t), args...)
		if code != 0 {
			t.Fatalf("exit = %d, want 0 (stderr=%q)", code, stderr)
		}
		return code, stdout
	}

	_, verbFirst := run("export", "--dry-run", "--only", "kiorc")
	_, flagFirst := run("--dry-run", "export", "--only", "kiorc")
	if verbFirst != flagFirst {
		t.Errorf("verb-first stdout = %q, flag-first stdout = %q; want identical", verbFirst, flagFirst)
	}
}

// TestRunPlasma_OnlyFiltersManagedSet proves --only narrows the managed set the
// status counters report.
func TestRunPlasma_OnlyFiltersManagedSet(t *testing.T) {
	newPlasmaEnv(t)

	_, all, _ := runPlasmaCapture(t, newPlasmaFake(t), "status")
	if strings.Contains(all, "plasma.managed=1\n") {
		t.Errorf("unfiltered status reported one managed file: %q", all)
	}

	_, one, stderr := runPlasmaCapture(t, newPlasmaFake(t), "status", "--only", "kiorc,kwinrc")
	if !strings.Contains(one, "plasma.managed=2") {
		t.Errorf("stdout = %q, want plasma.managed=2 (stderr=%q)", one, stderr)
	}
}

// TestRunPlasma_DiffExitsOneOnDrift locks diff's deliberate divergence from
// link's always-0 dry-run rule: diff is a drift detector, so --dry-run is inert
// and the exit code still signals non-convergence to CI.
func TestRunPlasma_DiffExitsOneOnDrift(t *testing.T) {
	env := newPlasmaEnv(t)
	env.writeLive(t, "kiorc", liveKiorc)
	env.writeRepoPlasma(t, "kiorc", repoKiorc)

	code, stdout, stderr := runPlasmaCapture(t, newPlasmaFake(t), "diff", "--dry-run", "--only", "kiorc")
	if code != 1 {
		t.Fatalf("exit = %d, want 1 on drift (stderr=%q)", code, stderr)
	}
	if want := "plasma.drift=kiorc:[Confirmations]/ConfirmDelete"; !strings.Contains(stdout, want) {
		t.Errorf("stdout = %q, want it to contain %q", stdout, want)
	}
}

// TestRunPlasma_DiffConverged proves a converged pair exits 0 with no drift line.
func TestRunPlasma_DiffConverged(t *testing.T) {
	env := newPlasmaEnv(t)
	env.writeLive(t, "kiorc", liveKiorc)
	env.writeRepoPlasma(t, "kiorc", liveKiorc)

	code, stdout, stderr := runPlasmaCapture(t, newPlasmaFake(t), "diff", "--only", "kiorc")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", code, stderr)
	}
	if strings.Contains(stdout, "plasma.drift=") {
		t.Errorf("stdout = %q, want no drift line", stdout)
	}
}

// TestRunPlasma_JSONReplacesKeyValue proves --json emits a parseable document
// INSTEAD OF the key=value lines, and leaves the exit code alone.
func TestRunPlasma_JSONReplacesKeyValue(t *testing.T) {
	env := newPlasmaEnv(t)
	env.writeLive(t, "kiorc", liveKiorc)
	env.writeRepoPlasma(t, "kiorc", repoKiorc)

	code, stdout, stderr := runPlasmaCapture(t, newPlasmaFake(t), "diff", "--json", "--only", "kiorc")
	if code != 1 {
		t.Fatalf("exit = %d, want 1 on drift (stderr=%q)", code, stderr)
	}
	if strings.Contains(stdout, "plasma.drift=") {
		t.Errorf("stdout = %q, want JSON only, no key=value lines", stdout)
	}

	var got plasmaReport
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout)
	}
	if got.Command != "plasma" || got.Verb != "diff" {
		t.Errorf("command/verb = %q/%q, want plasma/diff", got.Command, got.Verb)
	}
	if got.Session != "unknown" {
		t.Errorf("session = %q, want unknown", got.Session)
	}
	if got.Summary.DriftFiles != 1 || got.Summary.DriftKeys != 1 {
		t.Errorf("summary = %+v, want 1 drifted file and 1 drifted key", got.Summary)
	}
	if len(got.Files) != 1 || got.Files[0].Action != "drift" {
		t.Errorf("files = %+v, want one row with action=drift", got.Files)
	}
}

// TestRunPlasma_JSONStatusAlwaysEmitsArray proves the Files slice encodes as an
// array (never null) so consumers can iterate unconditionally.
func TestRunPlasma_JSONStatusAlwaysEmitsArray(t *testing.T) {
	newPlasmaEnv(t)
	code, stdout, stderr := runPlasmaCapture(t, newPlasmaFake(t), "status", "--json", "--only", "kiorc")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", code, stderr)
	}
	if !strings.Contains(stdout, `"files"`) || strings.Contains(stdout, `"files": null`) {
		t.Errorf("stdout = %q, want a non-null files array", stdout)
	}
}

// TestRunPlasma_ApplyRefusesNonInteractive proves the mutating apply path
// refuses to run without --yes when stdout is not a terminal.
func TestRunPlasma_ApplyRefusesNonInteractive(t *testing.T) {
	env := newPlasmaEnv(t)
	env.writeRepoPlasma(t, "kiorc", repoKiorc)

	code, stdout, stderr := runPlasmaCapture(t, newPlasmaFake(t), "apply", "--only", "kiorc")
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (stderr=%q)", code, stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "refusing to run plasma apply without --yes (non-interactive)") {
		t.Errorf("stderr = %q, want the non-interactive refusal", stderr)
	}
}

// TestRunPlasma_ApplyWritesLiveFile proves --yes takes the mutating path: the
// repo's keys land in the live file and the run reports converged.
func TestRunPlasma_ApplyWritesLiveFile(t *testing.T) {
	env := newPlasmaEnv(t)
	env.writeRepoPlasma(t, "kiorc", repoKiorc)

	code, stdout, stderr := runPlasmaCapture(t, newPlasmaFake(t), "apply", "--yes", "--only", "kiorc")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", code, stderr)
	}
	if !strings.Contains(stdout, "plasma.create=kiorc") {
		t.Errorf("stdout = %q, want a plasma.create line", stdout)
	}
	if !strings.Contains(stderr, "dotf: plasma summary: applied=1 planned=1") {
		t.Errorf("stderr = %q, want the summary line", stderr)
	}

	got, err := os.ReadFile(filepath.Join(env.config, "kiorc"))
	if err != nil {
		t.Fatalf("live kiorc not written: %v", err)
	}
	if string(got) != repoKiorc {
		t.Errorf("live kiorc = %q, want %q", got, repoKiorc)
	}
}

// TestRunPlasma_ApplyMissingRepoTree proves an un-exported repo is a soft error
// naming the remedy, not a crash or a usage error.
func TestRunPlasma_ApplyMissingRepoTree(t *testing.T) {
	newPlasmaEnv(t)
	code, stdout, stderr := runPlasmaCapture(t, newPlasmaFake(t), "apply", "--dry-run")
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (stderr=%q)", code, stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.HasPrefix(stderr, "dotf: ") || !strings.Contains(stderr, "dotf plasma export") {
		t.Errorf("stderr = %q, want a dotf-prefixed line naming the export remedy", stderr)
	}
}

// TestRunPlasma_ExportNoLiveFiles proves export fails loudly when the host has
// none of the managed files, rather than silently writing an empty tree.
func TestRunPlasma_ExportNoLiveFiles(t *testing.T) {
	newPlasmaEnv(t)
	code, stdout, stderr := runPlasmaCapture(t, newPlasmaFake(t), "export", "--dry-run", "--only", "kiorc")
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (stderr=%q)", code, stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "no managed KDE config files found") {
		t.Errorf("stderr = %q, want the no-live-files diagnostic", stderr)
	}
}

func TestPlasmaSelectFiles(t *testing.T) {
	table := plasma.DefaultFiles()

	tests := []struct {
		name    string
		only    string
		want    []string
		wantErr bool
	}{
		{name: "empty selects everything", only: "", want: nil},
		{name: "whitespace only selects everything", only: "   ", want: nil},
		{name: "single", only: "kiorc", want: []string{"kiorc"}},
		{name: "trims and drops empties", only: " kiorc , ,kwinrc ", want: []string{"kwinrc", "kiorc"}},
		{name: "unknown fails fast", only: "kwinrcx", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := plasmaSelectFiles(table, tt.only)
			if tt.wantErr {
				if err == nil {
					t.Fatal("error = nil, want an unknown-path error")
				}
				return
			}
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			if tt.want == nil {
				if len(got) != len(table) {
					t.Errorf("selected %d files, want the whole table", len(got))
				}
				return
			}
			var paths []string
			for _, spec := range got {
				paths = append(paths, spec.Path)
			}
			// Selection preserves TABLE order, not the order the user typed.
			if len(paths) != len(tt.want) {
				t.Fatalf("selected %v, want %v", paths, tt.want)
			}
			for _, want := range tt.want {
				if !containsString(paths, want) {
					t.Errorf("selected %v, want it to contain %q", paths, want)
				}
			}
		})
	}
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// TestPlasmaToken locks the stdout token encoding: exactly %, comma, equals, and
// control bytes are escaped, and nothing else — the common case must stay
// byte-identical to the raw name.
func TestPlasmaToken(t *testing.T) {
	cases := map[string]string{
		"kwinrc":               "kwinrc",
		"kdedefaults/kwinrc":   "kdedefaults/kwinrc",
		"a,b":                  "a%2Cb",
		"a=b":                  "a%3Db",
		"100%":                 "100%25",
		"tab\there":            "tab%09here",
		"[Colors:Header][Alt]": "[Colors:Header][Alt]",
		"a%2Cb":                "a%252Cb",
	}
	for in, want := range cases {
		if got := plasmaToken(in); got != want {
			t.Errorf("plasmaToken(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPlasmaGroupHeader(t *testing.T) {
	cases := []struct {
		path []string
		want string
	}{
		{nil, "[<default>]"},
		{[]string{}, "[<default>]"},
		{[]string{"Desktops"}, "[Desktops]"},
		{[]string{"Tiling", "x", "y"}, "[Tiling][x][y]"},
	}
	for _, tc := range cases {
		if got := plasmaGroupHeader(tc.path); got != tc.want {
			t.Errorf("plasmaGroupHeader(%v) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestPlasmaKeyRef(t *testing.T) {
	if got, want := plasmaKeyRef("kwinrc", []string{"Desktops"}, "Number", ""),
		"kwinrc:[Desktops]/Number"; got != want {
		t.Errorf("plasmaKeyRef = %q, want %q", got, want)
	}
	if got, want := plasmaKeyRef("kdeglobals", nil, "Name", "de_DE"),
		"kdeglobals:[<default>]/Name[de_DE]"; got != want {
		t.Errorf("plasmaKeyRef with locale = %q, want %q", got, want)
	}
}

// ----------------------------------------------------------------------------
// diff key detail: one line per entry, with the engine's classification
// ----------------------------------------------------------------------------

// TestPlasmaDiffLines_OneEntryPerLine pins diff's per-key rendering: ONE line
// per difference, sorted by reference, each carrying the engine's DiffKind.
//
// The comma-joined single line this replaces reached over 8KB on a real
// desktop — unreadable in a terminal and impossible to narrow with grep. The
// exact-match assertion also guards the line shape itself: the reference comes
// first so lines sort by identity, and the attributes follow as name=value
// pairs.
func TestPlasmaDiffLines_OneEntryPerLine(t *testing.T) {
	diffs := []plasma.FileDiff{
		{
			Spec: plasma.FileSpec{Path: "kwinrc"},
			Keys: []plasma.KeyDiff{
				{Group: []string{"Windows"}, Key: "FocusPolicy", Kind: plasma.DiffValueMismatch},
				{Group: []string{"Desktops"}, Key: "Number", Kind: plasma.DiffMissingLive},
				{Group: []string{"Tiling", "abc"}, Key: "layout", Kind: plasma.DiffMissingRepo},
			},
		},
		{
			Spec: plasma.FileSpec{Path: "kdeglobals"},
			Keys: []plasma.KeyDiff{
				{Group: []string{"General"}, Key: "Name", Locale: "de_DE", Kind: plasma.DiffFlagsMismatch},
				{Key: "Root", Kind: plasma.DiffMissingRepo},
			},
		},
	}

	want := []string{
		"plasma.drift=kdeglobals:[General]/Name[de_DE] kind=flags-mismatch",
		"plasma.drift=kwinrc:[Desktops]/Number kind=missing-live",
		"plasma.drift=kwinrc:[Windows]/FocusPolicy kind=value-mismatch",
		"plasma.only-live=kdeglobals:[<default>]/Root kind=missing-repo",
		"plasma.only-live=kwinrc:[Tiling][abc]/layout kind=missing-repo",
	}
	got := plasmaDiffLines(diffs)
	if !slices.Equal(got, want) {
		t.Errorf("plasmaDiffLines =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	// A raw comma can only mean two entries were joined onto one line: every
	// comma inside a KConfig name is percent-encoded by plasmaToken.
	for _, line := range got {
		if strings.Contains(line, ",") {
			t.Errorf("line %q carries a raw comma; entries must not be comma-joined", line)
		}
	}
}

// TestPlasmaDiffLines_Empty proves a converged run emits no key-detail lines at
// all rather than empty "plasma.drift=" / "plasma.only-live=" headers.
func TestPlasmaDiffLines_Empty(t *testing.T) {
	if got := plasmaDiffLines([]plasma.FileDiff{{Spec: plasma.FileSpec{Path: "kiorc"}}}); len(got) != 0 {
		t.Errorf("plasmaDiffLines on a converged diff = %v, want no lines", got)
	}
}

// TestRunPlasma_DiffRendersOneLinePerDriftedKey drives the per-line rendering
// end to end: two drifted keys in one file produce two independent, greppable
// plasma.drift= lines.
func TestRunPlasma_DiffRendersOneLinePerDriftedKey(t *testing.T) {
	env := newPlasmaEnv(t)
	env.writeLive(t, "kiorc", "[Confirmations]\nConfirmDelete=true\nConfirmTrash=true\n")
	env.writeRepoPlasma(t, "kiorc", "[Confirmations]\nConfirmDelete=false\nConfirmTrash=false\n")

	code, stdout, stderr := runPlasmaCapture(t, newPlasmaFake(t), "diff", "--only", "kiorc")
	if code != 1 {
		t.Fatalf("exit = %d, want 1 on drift (stderr=%q)", code, stderr)
	}
	want := []string{
		"plasma.drift=kiorc:[Confirmations]/ConfirmDelete kind=value-mismatch",
		"plasma.drift=kiorc:[Confirmations]/ConfirmTrash kind=value-mismatch",
	}
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	for _, w := range want {
		if !slices.Contains(lines, w) {
			t.Errorf("stdout lines = %v, want the standalone line %q", lines, w)
		}
	}
}

// TestPlasmaDiffReports_PinsDiffKindLabels pins the four machine-readable
// DiffKind labels the `--json` keys[].kind field carries. They are the only way
// a consumer can tell the informational missing-repo row — apply never deletes a
// live-only key — from genuine drift, so a collapse to "unknown", or a reorder
// of the iota constants that silently shifts their meaning, must fail here.
func TestPlasmaDiffReports_PinsDiffKindLabels(t *testing.T) {
	diffs := []plasma.FileDiff{{
		Spec: plasma.FileSpec{Path: "kiorc"},
		Keys: []plasma.KeyDiff{
			{Group: []string{"G"}, Key: "mismatch", Kind: plasma.DiffValueMismatch, Repo: []byte("r"), Live: []byte("l")},
			{Group: []string{"G"}, Key: "onlyrepo", Kind: plasma.DiffMissingLive, Repo: []byte("r")},
			{Group: []string{"G"}, Key: "onlylive", Kind: plasma.DiffMissingRepo, Live: []byte("l")},
			{Group: []string{"G"}, Key: "flags", Kind: plasma.DiffFlagsMismatch, Repo: []byte("v"), Live: []byte("v")},
		},
	}}

	rows := plasmaDiffReports(diffs)
	if len(rows) != 1 || len(rows[0].Keys) != 4 {
		t.Fatalf("plasmaDiffReports = %+v, want one file row with four keys", rows)
	}

	want := map[string]string{
		"mismatch": "value-mismatch",
		"onlyrepo": "missing-live",
		"onlylive": "missing-repo",
		"flags":    "flags-mismatch",
	}
	seen := make(map[string]bool, len(want))
	for _, k := range rows[0].Keys {
		if got := k.Kind; got != want[k.Key] {
			t.Errorf("key %q kind = %q, want %q", k.Key, got, want[k.Key])
		}
		if seen[k.Kind] {
			t.Errorf("kind %q is emitted for more than one DiffKind; the labels must stay distinct", k.Kind)
		}
		seen[k.Kind] = true
	}
}

// TestRunPlasma_DiffJSONCarriesKindLabel binds the label to the real `--json`
// document: a value mismatch reaches stdout as keys[].kind = "value-mismatch",
// verbatim, not merely as a Go-level string.
func TestRunPlasma_DiffJSONCarriesKindLabel(t *testing.T) {
	env := newPlasmaEnv(t)
	env.writeLive(t, "kiorc", liveKiorc)
	env.writeRepoPlasma(t, "kiorc", repoKiorc)

	code, stdout, stderr := runPlasmaCapture(t, newPlasmaFake(t), "diff", "--json", "--only", "kiorc")
	if code != 1 {
		t.Fatalf("exit = %d, want 1 on drift (stderr=%q)", code, stderr)
	}
	if !strings.Contains(stdout, `"kind": "value-mismatch"`) {
		t.Errorf("stdout = %q, want it to carry the verbatim kind label", stdout)
	}

	var got plasmaReport
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout)
	}
	if len(got.Files) != 1 || len(got.Files[0].Keys) != 1 {
		t.Fatalf("files = %+v, want one file row with one key", got.Files)
	}
	if k := got.Files[0].Keys[0]; k.Kind != "value-mismatch" || k.Repo != "false" || k.Live != "true" {
		t.Errorf("key row = %+v, want kind=value-mismatch repo=false live=true", k)
	}
}

// ----------------------------------------------------------------------------
// --show-volatile: expected-volatile vs genuinely unmanaged live-only keys
// ----------------------------------------------------------------------------

// kwinrc fixtures for the --show-volatile split. Live carries three live-only
// keys of two different classes: Id_1 matches kwinrc's own "[Desktops]/Id_*"
// strip rule (expected-volatile — export removes it by design, so it can never
// reach plasma/files/), while [Windows]/Placement matches no rule at all and is
// therefore genuinely unmanaged state.
const (
	liveKwinrc = "[Desktops]\nNumber=4\nId_1=e6f2\n\n[Windows]\nPlacement=Centered\n"
	repoKwinrc = "[Desktops]\nNumber=4\n"
)

// TestPlasmaDiffLines_SplitsExpectedVolatile pins the three-way bucket split of
// diff's key detail. Before --show-volatile existed the renderer bucketed on
// `Kind == DiffMissingRepo` alone, so an expected-volatile key fell through to
// the DEFAULT branch and was rendered as drift — the loudest possible wrong
// answer for the one class of key that is guaranteed not to be drift.
func TestPlasmaDiffLines_SplitsExpectedVolatile(t *testing.T) {
	diffs := []plasma.FileDiff{{
		Spec: plasma.FileSpec{Path: "kwinrc"},
		Keys: []plasma.KeyDiff{
			{Group: []string{"Windows"}, Key: "FocusPolicy", Kind: plasma.DiffValueMismatch},
			{Group: []string{"Windows"}, Key: "Placement", Kind: plasma.DiffMissingRepo},
			{Group: []string{"Desktops"}, Key: "Id_1", Kind: plasma.DiffExpectedVolatile},
			{Group: []string{"Tiling", "abc"}, Key: "layout", Kind: plasma.DiffExpectedVolatile},
		},
	}}

	want := []string{
		"plasma.drift=kwinrc:[Windows]/FocusPolicy kind=value-mismatch",
		"plasma.only-live=kwinrc:[Windows]/Placement kind=missing-repo",
		"plasma.expected-volatile=kwinrc:[Desktops]/Id_1 kind=expected-volatile",
		"plasma.expected-volatile=kwinrc:[Tiling][abc]/layout kind=expected-volatile",
	}
	if got := plasmaDiffLines(diffs); !slices.Equal(got, want) {
		t.Errorf("plasmaDiffLines =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestRunPlasma_ShowVolatileSuppressedByDefault proves the default report hides
// the keys export strips by design: no expected-volatile line, no drift line for
// them, and — because neither live-only class is actionable — exit 0.
func TestRunPlasma_ShowVolatileSuppressedByDefault(t *testing.T) {
	env := newPlasmaEnv(t)
	env.writeLive(t, "kwinrc", liveKwinrc)
	env.writeRepoPlasma(t, "kwinrc", repoKwinrc)

	code, stdout, stderr := runPlasmaCapture(t, newPlasmaFake(t), "diff", "--only", "kwinrc")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (live-only keys are never drift); stderr=%q", code, stderr)
	}
	if strings.Contains(stdout, "expected-volatile") {
		t.Errorf("stdout = %q, want no expected-volatile detail without --show-volatile", stdout)
	}
	if strings.Contains(stdout, "Id_1") {
		t.Errorf("stdout = %q, want the stripped Id_1 key suppressed", stdout)
	}
	// The genuinely unmanaged key is NOT suppressed: that is the whole point of
	// telling the two apart.
	if want := "plasma.only-live=kwinrc:[Windows]/Placement kind=missing-repo"; !strings.Contains(stdout, want) {
		t.Errorf("stdout = %q, want it to still report %q", stdout, want)
	}
}

// TestRunPlasma_ShowVolatileRevealsStrippedKeys proves --show-volatile reaches
// the engine, that the revealed keys land in their own bucket rather than in
// drift, and that revealing them does not turn a converged file into a failure.
func TestRunPlasma_ShowVolatileRevealsStrippedKeys(t *testing.T) {
	env := newPlasmaEnv(t)
	env.writeLive(t, "kwinrc", liveKwinrc)
	env.writeRepoPlasma(t, "kwinrc", repoKwinrc)

	code, stdout, stderr := runPlasmaCapture(t, newPlasmaFake(t),
		"diff", "--show-volatile", "--only", "kwinrc")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (revealing volatile keys is not drift); stderr=%q", code, stderr)
	}
	want := "plasma.expected-volatile=kwinrc:[Desktops]/Id_1 kind=expected-volatile"
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if !slices.Contains(lines, want) {
		t.Errorf("stdout lines = %v, want the standalone line %q", lines, want)
	}
	if strings.Contains(stdout, "plasma.drift=") {
		t.Errorf("stdout = %q, want no drift line: an expected-volatile key is not drift", stdout)
	}
}

// TestRunPlasma_ShowVolatileJSONSeparatesCounts proves the distinction survives
// into --json: the revealed rows carry kind=expected-volatile and are tallied in
// summary.volatileKeys, kept apart from summary.driftKeys so the two totals stay
// independently meaningful.
func TestRunPlasma_ShowVolatileJSONSeparatesCounts(t *testing.T) {
	env := newPlasmaEnv(t)
	env.writeLive(t, "kwinrc", liveKwinrc)
	env.writeRepoPlasma(t, "kwinrc", repoKwinrc)

	code, stdout, stderr := runPlasmaCapture(t, newPlasmaFake(t),
		"diff", "--json", "--show-volatile", "--only", "kwinrc")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", code, stderr)
	}

	var got plasmaReport
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout)
	}
	if got.Summary.DriftKeys != 0 {
		t.Errorf("summary.driftKeys = %d, want 0: neither live-only class is drift", got.Summary.DriftKeys)
	}
	if got.Summary.VolatileKeys != 1 {
		t.Errorf("summary.volatileKeys = %d, want 1", got.Summary.VolatileKeys)
	}
	if got.Summary.DriftFiles != 0 {
		t.Errorf("summary.driftFiles = %d, want 0", got.Summary.DriftFiles)
	}
	if len(got.Files) != 1 || got.Files[0].Action != "converged" {
		t.Fatalf("files = %+v, want one row with action=converged", got.Files)
	}
	var kinds []string
	for _, k := range got.Files[0].Keys {
		kinds = append(kinds, k.Kind)
	}
	if !slices.Contains(kinds, "expected-volatile") {
		t.Errorf("key kinds = %v, want one expected-volatile row", kinds)
	}
}

// TestRunPlasma_ShowVolatileJSONDefaultTally is the default-path counterpart:
// with the flag absent the engine drops the rows entirely, so volatileKeys is 0
// and the field never reports keys the document does not contain.
func TestRunPlasma_ShowVolatileJSONDefaultTally(t *testing.T) {
	env := newPlasmaEnv(t)
	env.writeLive(t, "kwinrc", liveKwinrc)
	env.writeRepoPlasma(t, "kwinrc", repoKwinrc)

	code, stdout, stderr := runPlasmaCapture(t, newPlasmaFake(t), "diff", "--json", "--only", "kwinrc")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", code, stderr)
	}

	var got plasmaReport
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout)
	}
	if got.Summary.VolatileKeys != 0 {
		t.Errorf("summary.volatileKeys = %d, want 0 without --show-volatile", got.Summary.VolatileKeys)
	}
	for _, k := range got.Files[0].Keys {
		if k.Kind == "expected-volatile" {
			t.Errorf("keys = %+v, want no expected-volatile row without the flag", got.Files[0].Keys)
		}
	}
}

// TestRunPlasma_LinkWithRecommendedIsUsageError pins the exit code of the one
// mutually exclusive flag PAIR. The engine also refuses the combination
// (plasma.LinkRecommendedError) but only from inside PlanApply, where the CLI's
// typed-error path renders it as a soft failure (1). Two flags the tool
// documents as exclusive is a malformed invocation, which this repo's contract
// puts at 2 — the same code every other plasma flag rejection returns.
func TestRunPlasma_LinkWithRecommendedIsUsageError(t *testing.T) {
	env := newPlasmaEnv(t)
	// Seeded so the run would otherwise reach PlanApply: without it the
	// missing repo tree would short-circuit first and the exit code would not
	// be attributable to the flag pair.
	env.writeRepoPlasma(t, "kiorc", repoKiorc)

	code, stdout, stderr := runPlasmaCapture(t, newPlasmaFake(t),
		"apply", "--link", "--recommended", "--dry-run", "--only", "kiorc")
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (usage error); stderr=%q", code, stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.HasPrefix(stderr, "dotf: plasma: --link cannot be combined with --recommended") {
		t.Errorf("stderr = %q, want the engine's own exclusivity diagnostic", stderr)
	}
}
