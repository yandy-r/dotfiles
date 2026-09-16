package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
)

// These are end-to-end tests through runDeb: real flag parsing, the real
// pipeline, real files on disk. Only the subprocess boundary is faked.
//
// A plain exec.Fake is not enough, because a conversion is DEFINED by the files
// bsdtar and debtap leave behind — a runner that only returns canned bytes
// would let the pipeline "succeed" while producing nothing. debRunner therefore
// reproduces those filesystem side effects, so the assertions below exercise
// the same code paths a real run does.

// debRunner is an exec.Fake that also performs the filesystem side effects of
// the commands the pipeline shells out to.
type debRunner struct {
	*exec.Fake
	t *testing.T

	// control is the body written as the extracted Debian control file.
	control string
	// artifact is the package filename debtap "builds" in its work directory.
	artifact string
	// listing and pkginfo are returned for the bsdtar operations whose operand
	// is a private staging path the test cannot predict (see Output).
	listing string
	pkginfo string
}

// Output falls back to path-independent answers for the two bsdtar reads whose
// operand is a private, randomly-named staging path. Acquire deliberately
// copies the input somewhere the caller does not control, so a test cannot key
// those calls on an exact argv — but it can still assert on the operation.
func (f *debRunner) Output(name string, args ...string) ([]byte, error) {
	out, err := f.Fake.Output(name, args...)
	if err == nil {
		return out, nil
	}
	switch {
	case name == "bsdtar" && len(args) >= 2 && args[0] == "-tf":
		return []byte(f.listing), nil
	case name == "bsdtar" && len(args) >= 3 && args[0] == "-xOf" && args[2] == ".PKGINFO":
		return []byte(f.pkginfo), nil
	}
	return out, err
}

func (f *debRunner) Run(name string, args ...string) error {
	if err := f.Fake.Run(name, args...); err != nil {
		return err
	}
	return f.sideEffect("", name, args)
}

func (f *debRunner) RunDir(dir, name string, args ...string) error {
	if err := f.Fake.RunDir(dir, name, args...); err != nil {
		return err
	}
	return f.sideEffect(dir, name, args)
}

// sideEffect mirrors what bsdtar and debtap actually leave on disk.
func (f *debRunner) sideEffect(dir, name string, args []string) error {
	switch {
	// bsdtar -xf <src> -C <destDir> [member]
	case name == "bsdtar" && len(args) >= 4 && args[0] == "-xf":
		dest := args[3]
		if len(args) >= 5 {
			// Extracting the control tarball member out of the .deb.
			return os.WriteFile(filepath.Join(dest, filepath.Base(args[4])), []byte("stub"), 0o600)
		}
		// Extracting the control tarball itself yields the control file.
		return os.WriteFile(filepath.Join(dest, "control"), []byte(f.control), 0o600)

	case name == "debtap":
		return os.WriteFile(filepath.Join(dir, f.artifact), []byte("built package"), 0o600)
	}
	return nil
}

// debFixtureBytes is a structurally valid .deb: the ar global header plus a
// first member named "debian-binary".
func debFixtureBytes() []byte {
	var b []byte
	b = append(b, "!<arch>\n"...)
	b = append(b, "debian-binary   "...)
	b = append(b, "1700000000  0     0     100644  4         `\n"...)
	b = append(b, "2.0\n"...)
	return b
}

// debEnv is one wired-up scenario: a valid .deb on disk, an output directory, a
// populated debtap database, and a runner primed with the canned command output
// the pipeline reads.
type debEnv struct {
	debPath  string
	outDir   string
	artifact string
	runner   *debRunner
}

// newDebEnv builds the happy-path scenario. Individual tests override the
// canned results they care about before invoking runDeb.
func newDebEnv(t *testing.T) *debEnv {
	t.Helper()

	debPath := filepath.Join(t.TempDir(), "slack-desktop.deb")
	if err := os.WriteFile(debPath, debFixtureBytes(), 0o600); err != nil {
		t.Fatalf("write .deb fixture: %v", err)
	}
	outDir := t.TempDir()
	artifact := "slack-desktop-4.36.140-1-x86_64.pkg.tar.zst"

	dbDir := t.TempDir()
	// debtap's own start-up gate: its database files plus a pkgfile cache
	// entry matching the pattern debtap greps for.
	for _, name := range []string{
		"debian-main-packages-files",
		"extended-base-packages-list",
		"aur-packages",
		"virtual-packages",
	} {
		if err := os.WriteFile(filepath.Join(dbDir, name), []byte("x"), 0o600); err != nil {
			t.Fatalf("write db fixture: %v", err)
		}
	}
	pkgfileDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(pkgfileDir, "core.files.000"), nil, 0o600); err != nil {
		t.Fatalf("write pkgfile fixture: %v", err)
	}

	origDB, origPkg := debDBDir, debPkgfileDir
	debDBDir, debPkgfileDir = dbDir, pkgfileDir
	t.Cleanup(func() { debDBDir, debPkgfileDir = origDB, origPkg })

	// Acquire stages into the cache dir; point it at a temp dir so tests never
	// touch the developer's real ~/.cache.
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	f := exec.NewFake()
	f.AddPath("debtap", "/usr/bin/debtap")
	f.AddPath("bsdtar", "/usr/bin/bsdtar")
	f.AddPath("pacman", "/usr/bin/pacman")
	f.AddPath("pkgfile", "/usr/bin/pkgfile")
	// platform.Detect probes for package managers by LookPath.
	f.AddCommand("uname", []string{"-s"}, []byte("Linux\n"), nil)
	f.AddCommand("uname", []string{"-m"}, []byte("x86_64\n"), nil)

	// No native package of this name exists, and every dependency is satisfied.
	f.AddCommand("pacman", []string{"-Ssq", "^slack-desktop$"}, nil, nil)
	f.AddCommand("pacman", []string{"-T", "gtk3", "libxss"}, nil, nil)

	return &debEnv{
		debPath:  debPath,
		outDir:   outDir,
		artifact: artifact,
		runner: &debRunner{
			Fake:     f,
			t:        t,
			control:  "Package: slack-desktop\nVersion: 4.36.140\nArchitecture: amd64\n",
			artifact: artifact,
			listing:  "debian-binary\ncontrol.tar.zst\ndata.tar.zst\n",
			pkginfo: strings.Join([]string{
				"pkgname = slack-desktop",
				"pkgver = 4.36.140-1",
				"size = 112894976",
				"depend = gtk3",
				"depend = libxss",
			}, "\n"),
		},
	}
}

// ranCommand reports whether name was invoked with exactly args.
func ranCommand(calls []exec.FakeCall, name string, args ...string) bool {
	want := exec.CommandKey(name, args...)
	for _, c := range calls {
		if exec.CommandKey(c.Name, c.Args...) == want {
			return true
		}
	}
	return false
}

// ranAny reports whether name was invoked at all.
func ranAny(calls []exec.FakeCall, name string) bool {
	for _, c := range calls {
		if c.Name == name {
			return true
		}
	}
	return false
}

// TestRunDeb_ConvertOnly_LocalFile is the primary happy path: a local .deb is
// verified, converted, and inspected, and nothing is installed.
func TestRunDeb_ConvertOnly_LocalFile(t *testing.T) {
	env := newDebEnv(t)
	var stdout, stderr bytes.Buffer

	code := runDeb([]string{env.debPath, "--out-dir", env.outDir}, &stdout, &stderr, env.runner)
	if code != 0 {
		t.Fatalf("runDeb() = %d, want 0\nstderr: %s", code, stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{
		"source=file",
		"deb_package=slack-desktop",
		"deb_version=4.36.140",
		"precedence=none",
		"package=slack-desktop",
		"version=4.36.140-1",
		"size=112894976",
		"depends=2",
		"installed=false",
		"artifact=" + filepath.Join(env.outDir, env.artifact),
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout missing %q\ngot:\n%s", want, out)
		}
	}

	// The digest is always reported, even without --sha256.
	if !strings.Contains(out, "sha256=") {
		t.Errorf("stdout missing sha256 line\ngot:\n%s", out)
	}

	// The artifact really exists where the output claims it does.
	if _, err := os.Stat(filepath.Join(env.outDir, env.artifact)); err != nil {
		t.Errorf("artifact not written: %v", err)
	}

	calls := env.runner.CallsSnapshot()
	// debtap must run fully non-interactively, or a scripted run would hang.
	if !ranCommand(calls, "debtap", "-Q", filepath.Join(debtapWorkDirOf(t, calls), "slack-desktop.deb")) {
		t.Errorf("debtap was not invoked with -Q\ncalls: %+v", calls)
	}
	// Convert-only must never elevate.
	if ranAny(calls, "sudo") {
		t.Errorf("convert-only run invoked sudo\ncalls: %+v", calls)
	}
}

// debtapWorkDirOf recovers the temp work directory debtap ran in, so the
// argv assertion above can name the staged file exactly.
func debtapWorkDirOf(t *testing.T, calls []exec.FakeCall) string {
	t.Helper()
	for _, c := range calls {
		if c.Name == "debtap" {
			return c.Dir
		}
	}
	t.Fatal("debtap was never invoked")
	return ""
}

// TestRunDeb_InstallYes proves --install --yes elevates exactly once, passes
// --noconfirm (the user already answered), and reports what pacman actually
// registered rather than what the artifact claimed.
func TestRunDeb_InstallYes(t *testing.T) {
	env := newDebEnv(t)
	env.runner.AddCommand("pacman", []string{"-Qq", "slack-desktop"}, []byte("slack-desktop\n"), nil)
	env.runner.AddCommand("pacman", []string{"-Qql", "slack-desktop"},
		[]byte("/usr/bin/slack\n/usr/lib/slack/slack.bin\n/usr/share/applications/slack.desktop\n"), nil)

	var stdout, stderr bytes.Buffer
	code := runDeb([]string{env.debPath, "--out-dir", env.outDir, "--install", "--yes"},
		&stdout, &stderr, env.runner)
	if code != 0 {
		t.Fatalf("runDeb() = %d, want 0\nstderr: %s", code, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "installed=true") {
		t.Errorf("stdout missing installed=true\ngot:\n%s", out)
	}
	if !strings.Contains(out, "files=3") {
		t.Errorf("stdout missing files=3 (pacman's own file count)\ngot:\n%s", out)
	}

	calls := env.runner.CallsSnapshot()
	if !ranCommand(calls, "sudo", "pacman", "-U", "--noconfirm",
		filepath.Join(env.outDir, env.artifact)) {
		t.Errorf("expected `sudo pacman -U --noconfirm <artifact>`\ncalls: %+v", calls)
	}
}

// TestRunDeb_PrecedenceGate proves a name that already exists in the official
// repositories stops the run, and that --force is the documented override.
func TestRunDeb_PrecedenceGate(t *testing.T) {
	t.Run("blocks without --force", func(t *testing.T) {
		env := newDebEnv(t)
		env.runner.AddCommand("pacman", []string{"-Ssq", "^slack-desktop$"},
			[]byte("slack-desktop\n"), nil)

		var stdout, stderr bytes.Buffer
		code := runDeb([]string{env.debPath, "--out-dir", env.outDir}, &stdout, &stderr, env.runner)
		if code != 1 {
			t.Fatalf("runDeb() = %d, want 1", code)
		}
		if !strings.Contains(stderr.String(), "already in the official repositories") {
			t.Errorf("stderr missing the precedence diagnostic\ngot:\n%s", stderr.String())
		}
		if !strings.Contains(stdout.String(), "precedence=repo") {
			t.Errorf("stdout missing precedence=repo\ngot:\n%s", stdout.String())
		}
		// The gate must stop the run BEFORE any conversion work happens.
		if ranAny(env.runner.CallsSnapshot(), "debtap") {
			t.Error("debtap ran despite the precedence gate")
		}
	})

	t.Run("proceeds with --force", func(t *testing.T) {
		env := newDebEnv(t)
		env.runner.AddCommand("pacman", []string{"-Ssq", "^slack-desktop$"},
			[]byte("slack-desktop\n"), nil)

		var stdout, stderr bytes.Buffer
		code := runDeb([]string{env.debPath, "--out-dir", env.outDir, "--force"},
			&stdout, &stderr, env.runner)
		if code != 0 {
			t.Fatalf("runDeb() = %d, want 0\nstderr: %s", code, stderr.String())
		}
		if !strings.Contains(stderr.String(), "converting anyway (--force)") {
			t.Errorf("stderr missing the --force warning\ngot:\n%s", stderr.String())
		}
		if !ranAny(env.runner.CallsSnapshot(), "debtap") {
			t.Error("debtap did not run under --force")
		}
	})
}

// TestRunDeb_ReportsUnresolvableDependencies proves debtap's approximate
// dependency translation surfaces as a warning without failing the run —
// pacman remains the authority on installability.
func TestRunDeb_ReportsUnresolvableDependencies(t *testing.T) {
	env := newDebEnv(t)
	env.runner.AddCommand("pacman", []string{"-T", "gtk3", "libxss"},
		[]byte("libxss\n"), nil)
	env.runner.AddCommand("pacman", []string{"-Ssq", "^libxss$"}, nil, nil)

	var stdout, stderr bytes.Buffer
	code := runDeb([]string{env.debPath, "--out-dir", env.outDir}, &stdout, &stderr, env.runner)
	if code != 0 {
		t.Fatalf("runDeb() = %d, want 0 (dependency problems are advisory)\nstderr: %s",
			code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "depend_unresolvable=libxss") {
		t.Errorf("stdout missing depend_unresolvable=libxss\ngot:\n%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "resolve to no known package") {
		t.Errorf("stderr missing the dependency warning\ngot:\n%s", stderr.String())
	}
}

// TestRunDeb_PrecedenceGateUsesTheInstalledName is a gate-bypass regression
// test.
//
// The pre-conversion check reads the .deb's SELF-DECLARED Package field, but
// what actually gets installed is the built artifact's pkgname, which debtap
// can change. A .deb declaring an obscure name must not be able to slip past
// the gate and then install a package that shadows a real repo package.
func TestRunDeb_PrecedenceGateUsesTheInstalledName(t *testing.T) {
	env := newDebEnv(t)

	// The .deb claims an obscure name that has no native counterpart...
	env.runner.control = "Package: obscure-vendor-app\nVersion: 1.0\n"
	env.runner.AddCommand("pacman", []string{"-Ssq", "^obscure-vendor-app$"}, nil, nil)

	// ...but the built package is named after a real repo package.
	env.runner.pkginfo = "pkgname = ripgrep\npkgver = 14.0.0-1\nsize = 4096\n"
	env.runner.AddCommand("pacman", []string{"-Ssq", "^ripgrep$"}, []byte("ripgrep\n"), nil)

	var stdout, stderr bytes.Buffer
	code := runDeb([]string{env.debPath, "--out-dir", env.outDir, "--install", "--yes"},
		&stdout, &stderr, env.runner)

	if code != 1 {
		t.Fatalf("runDeb() = %d, want 1 (the gate must catch the real name)\nstderr: %s",
			code, stderr.String())
	}
	if !strings.Contains(stderr.String(), `"ripgrep" is already in the official repositories`) {
		t.Errorf("stderr must name the ACTUAL package\ngot:\n%s", stderr.String())
	}
	if ranAny(env.runner.CallsSnapshot(), "sudo") {
		t.Error("the shadowing package was installed despite the gate")
	}
}

// TestRunDeb_DryRunExecutesNothing proves --dry-run reports the plan without
// converting, downloading, or installing.
func TestRunDeb_DryRunExecutesNothing(t *testing.T) {
	env := newDebEnv(t)
	var stdout, stderr bytes.Buffer

	code := runDeb([]string{env.debPath, "--out-dir", env.outDir, "--dry-run"},
		&stdout, &stderr, env.runner)
	if code != 0 {
		t.Fatalf("runDeb() = %d, want 0\nstderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "dry-run=true") {
		t.Errorf("stdout missing dry-run=true\ngot:\n%s", stdout.String())
	}

	calls := env.runner.CallsSnapshot()
	for _, forbidden := range []string{"debtap", "bsdtar", "sudo"} {
		if ranAny(calls, forbidden) {
			t.Errorf("--dry-run invoked %q\ncalls: %+v", forbidden, calls)
		}
	}
	if entries, _ := os.ReadDir(env.outDir); len(entries) != 0 {
		t.Errorf("--dry-run wrote %d file(s) to the output dir", len(entries))
	}
}

// TestRunDeb_UsageErrors covers the argument contract. Each of these is a
// malformed invocation (exit 2), not a failed run (exit 1).
func TestRunDeb_UsageErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no positional", []string{}, "missing <url|path>"},
		{"two positionals", []string{"a.deb", "b.deb"}, "want exactly one"},
		{"--yes without --install", []string{"a.deb", "--yes"}, "--yes requires --install"},
		{"unknown flag", []string{"a.deb", "--nope"}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := runDeb(tt.args, &stdout, &stderr, exec.NewFake())
			if code != 2 {
				t.Fatalf("runDeb(%v) = %d, want 2", tt.args, code)
			}
			if tt.want != "" && !strings.Contains(stderr.String(), tt.want) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.want)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty on a usage error", stdout.String())
			}
		})
	}
}

// TestRunDeb_UnsupportedHost proves a non-pacman host is a soft failure with an
// explanatory diagnostic, not a usage error and not exit 3 (which is reserved
// for `dotf apply`'s hard aborts).
func TestRunDeb_UnsupportedHost(t *testing.T) {
	f := exec.NewFake()
	f.AddCommand("uname", []string{"-s"}, []byte("Darwin\n"), nil)
	f.AddCommand("uname", []string{"-m"}, []byte("arm64\n"), nil)

	var stdout, stderr bytes.Buffer
	code := runDeb([]string{"app.deb"}, &stdout, &stderr, f)
	if code != 1 {
		t.Fatalf("runDeb() = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "pacman-based host") {
		t.Errorf("stderr = %q, want it to explain the host requirement", stderr.String())
	}
}

// TestRunDeb_PkgfileGate_DiagnosesAndOffersFix covers the debtap bug that
// motivated this diagnosis: a fully synced host still fails debtap's pkgfile
// check, and debtap's own message tells the user to run `debtap -u`, which can
// never help.
//
// Critically it also pins that nothing is executed without consent: with no
// terminal to confirm at, the command must be PRINTED, not run.
func TestRunDeb_PkgfileGate_DiagnosesAndOffersFix(t *testing.T) {
	env := newDebEnv(t)

	// A realistically-synced pkgfile cache: plain "<repo>.files" names, which
	// debtap's pattern cannot match.
	pkgfileDir := t.TempDir()
	for _, name := range []string{"core.files", "extra.files", "multilib.files"} {
		if err := os.WriteFile(filepath.Join(pkgfileDir, name), []byte("x"), 0o600); err != nil {
			t.Fatalf("write pkgfile fixture: %v", err)
		}
	}
	debPkgfileDir = pkgfileDir

	var stdout, stderr bytes.Buffer
	code := runDeb([]string{env.debPath, "--out-dir", env.outDir}, &stdout, &stderr, env.runner)
	if code != 1 {
		t.Fatalf("runDeb() = %d, want 1", code)
	}

	diag := stderr.String()
	for _, want := range []string{
		"pkgfile check requires a cache entry",
		"bug in debtap, not a missing sync",
		"cannot fix it",
		"debtap-gate.files.000",
		"Undo:",
	} {
		if !strings.Contains(diag, want) {
			t.Errorf("diagnosis missing %q\ngot:\n%s", want, diag)
		}
	}

	// The whole point: it must not run the fix on its own.
	for _, c := range env.runner.CallsSnapshot() {
		if c.Name == "sudo" {
			t.Fatalf("ran a fix without consent: sudo %v", c.Args)
		}
	}
}

// TestRunDeb_MissingDatabase_OffersSync proves a genuinely un-synced database
// gets the `debtap -u` remedy — the case where debtap's own advice IS right.
func TestRunDeb_MissingDatabase_OffersSync(t *testing.T) {
	env := newDebEnv(t)
	debDBDir = t.TempDir() // present but empty

	var stdout, stderr bytes.Buffer
	if code := runDeb([]string{env.debPath, "--out-dir", env.outDir},
		&stdout, &stderr, env.runner); code != 1 {
		t.Fatalf("runDeb() = %d, want 1", code)
	}

	diag := stderr.String()
	if !strings.Contains(diag, "translation database") {
		t.Errorf("diagnosis missing the database problem\ngot:\n%s", diag)
	}
	if !strings.Contains(diag, "sudo debtap -u") {
		t.Errorf("diagnosis missing the `sudo debtap -u` remedy\ngot:\n%s", diag)
	}
	// This remedy has nothing to undo, so no Undo line should be offered.
	if strings.Contains(diag, "Undo:") {
		t.Errorf("a sync remedy should offer no Undo\ngot:\n%s", diag)
	}
}

// TestShellQuoteArgv checks the displayed command is copy-pasteable.
func TestShellQuoteArgv(t *testing.T) {
	tests := []struct {
		argv []string
		want string
	}{
		{[]string{"sudo", "debtap", "-u"}, "sudo debtap -u"},
		{[]string{"sudo", "touch", "/var/cache/pkgfile/debtap-gate.files.000"},
			"sudo touch /var/cache/pkgfile/debtap-gate.files.000"},
		{[]string{"sudo", "touch", "/tmp/a b"}, "sudo touch '/tmp/a b'"},
	}
	for _, tt := range tests {
		if got := shellQuoteArgv(tt.argv); got != tt.want {
			t.Errorf("shellQuoteArgv(%v) = %q, want %q", tt.argv, got, tt.want)
		}
	}
}
