package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/failure"
	"github.com/yandy-r/dotfiles/internal/plasma"
	"github.com/yandy-r/dotfiles/internal/platform"
	"github.com/yandy-r/dotfiles/internal/symlink"
)

// TestApplyDirs_CreatesStandardDirs asserts the create_directories apply step
// materializes the standard XDG/dev directories under $HOME and records a
// successful "directories" outcome.
func TestApplyDirs_CreatesStandardDirs(t *testing.T) {
	home := t.TempDir()
	var coll failure.Collector

	applyDirs(home, &coll)

	if coll.Failed() {
		t.Fatalf("applyDirs recorded a failure: %+v", coll.Failures())
	}
	for _, rel := range []string{".config", ".local/bin", ".zsh/completions", "go/bin", "go/src", "go/pkg"} {
		fi, err := os.Stat(filepath.Join(home, rel))
		if err != nil {
			t.Errorf("expected dir %q under home: %v", rel, err)
			continue
		}
		if !fi.IsDir() {
			t.Errorf("%q exists but is not a directory", rel)
		}
	}
}

// TestApplyDirs_Idempotent asserts a second run over an already-created tree is
// a clean no-op (MkdirAll on an existing dir is not an error).
func TestApplyDirs_Idempotent(t *testing.T) {
	home := t.TempDir()
	var coll failure.Collector

	applyDirs(home, &coll)
	applyDirs(home, &coll)

	if coll.Failed() {
		t.Fatalf("applyDirs is not idempotent: %+v", coll.Failures())
	}
}

// TestApplyUserToolPath_PrependsUserToolDirs asserts the apply step puts every
// user-level tool directory on the process PATH ahead of the inherited entries,
// so a tool installed earlier in the same run (npm under the mise shims, cargo
// under ~/.cargo/bin) resolves for the phases that follow.
func TestApplyUserToolPath_PrependsUserToolDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PATH", "/usr/bin")

	applyUserToolPath(home)

	got := filepath.SplitList(os.Getenv("PATH"))
	for _, rel := range userToolDirs {
		want := filepath.Join(home, rel)
		if !slices.Contains(got, want) {
			t.Errorf("PATH %v missing %q", got, want)
		}
	}
	for _, want := range systemToolDirs {
		if !slices.Contains(got, want) {
			t.Errorf("PATH %v missing %q", got, want)
		}
	}
	if got[len(got)-1] != "/usr/bin" {
		t.Errorf("inherited PATH entry = %q, want it last (/usr/bin)", got[len(got)-1])
	}
}

// TestApplyUserToolPath_Idempotent asserts a second call adds no duplicates —
// prependPath skips a directory already on PATH.
func TestApplyUserToolPath_Idempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PATH", "/usr/bin")

	applyUserToolPath(home)
	first := os.Getenv("PATH")
	applyUserToolPath(home)

	if second := os.Getenv("PATH"); second != first {
		t.Errorf("PATH = %q after second call, want unchanged %q", second, first)
	}
}

func TestRunApply_DryRunNoSudoNoMutation(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	t.Setenv("DOTFILES_ROOT", repoRoot)
	t.Setenv("HOME", home)

	manifestPath := writeTestManifest(t, testToolchainManifest)
	f := newApplyFake(t)

	var stdout, stderr bytes.Buffer
	code := runApply([]string{"--manifest", manifestPath, "--dry-run"}, &stdout, &stderr, f)

	if code != 0 {
		t.Fatalf("runApply(--dry-run) code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if !strings.HasPrefix(stdout.String(), "dry-run=true\n") {
		t.Errorf("stdout = %q, want dry-run=true header", stdout.String())
	}
	if !strings.Contains(stdout.String(), "packages=rg,fd") {
		t.Errorf("stdout = %q, want packages=rg,fd", stdout.String())
	}

	for _, call := range f.CallsSnapshot() {
		if call.Name == "sudo" {
			t.Errorf("dry-run invoked sudo (%+v), want none", call)
		}
	}

	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatalf("ReadDir(home) error = %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("home has %d entries after dry-run, want 0 (no mutation)", len(entries))
	}
}

// TestRunApply_DryRunPlansRuntimeManagers covers acceptance criterion (2):
// `dotf apply --dry-run --intent runtimes` plans the runtime managers (mise,
// rust) as custom-script packages, with no package manager or sudo involved.
func TestRunApply_DryRunPlansRuntimeManagers(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	t.Setenv("DOTFILES_ROOT", repoRoot)
	t.Setenv("HOME", home)

	manifestPath := writeTestManifest(t, applyRuntimesManifest)
	f := newApplyFake(t)

	var stdout, stderr bytes.Buffer
	code := runApply([]string{"--manifest", manifestPath, "--dry-run", "--headless", "--intent", "runtimes"}, &stdout, &stderr, f)

	if code != 0 {
		t.Fatalf("runApply(--dry-run --intent runtimes) code = %d, want 0; stderr=%q", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"packages=mise,rust", "plan.custom-script=mise,rust"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run stdout = %q, want substring %q", out, want)
		}
	}
	for _, call := range f.CallsSnapshot() {
		if call.Name == "sudo" {
			t.Errorf("dry-run invoked sudo (%+v), want none", call)
		}
	}
}

// TestRunApply_UnknownIntent verifies a mistyped --intent is a usage error
// (exit 2) before resolveIntents runs, covering both the dry-run and the real
// headless path — never a silent empty-package apply. The gate fires before
// sudo is primed, so no commands run at all.
func TestRunApply_UnknownIntent(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"dry-run", []string{"--dry-run", "--headless", "--intent", "bogus"}},
		{"headless", []string{"--headless", "--intent", "bogus"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoRoot := t.TempDir()
			home := t.TempDir()
			t.Setenv("DOTFILES_ROOT", repoRoot)
			t.Setenv("HOME", home)

			manifestPath := writeTestManifest(t, testIntentManifest)
			f := newApplyFake(t)

			var stdout, stderr bytes.Buffer
			code := runApply(append([]string{"--manifest", manifestPath}, tt.args...), &stdout, &stderr, f)

			if code != 2 {
				t.Fatalf("runApply(%v) code = %d, want 2; stderr=%q", tt.args, code, stderr.String())
			}
			if !strings.Contains(stderr.String(), `unknown intent "bogus"; manifest defines: base, dev`) {
				t.Errorf("stderr = %q, want unknown-intent diagnostic listing base, dev", stderr.String())
			}
			if calls := f.CallsSnapshot(); len(calls) != 0 {
				t.Errorf("commands ran (%+v), want none (no sudo prime, no phases)", calls)
			}
		})
	}
}

// TestRunApply_KnownIntentStillResolves verifies the validation gate does not
// reject valid manifest intents: a dry-run narrowed to base plans exactly the
// base intent's packages.
func TestRunApply_KnownIntentStillResolves(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	t.Setenv("DOTFILES_ROOT", repoRoot)
	t.Setenv("HOME", home)

	manifestPath := writeTestManifest(t, testIntentManifest)
	f := newApplyFake(t)

	var stdout, stderr bytes.Buffer
	code := runApply([]string{"--manifest", manifestPath, "--dry-run", "--headless", "--intent", "base"}, &stdout, &stderr, f)

	if code != 0 {
		t.Fatalf("runApply(--intent base) code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "packages=rg\n") {
		t.Errorf("stdout = %q, want packages=rg (base intent only)", stdout.String())
	}
}

// TestRunApply_OnlyFilterHeadless asserts the #42 package-level --only narrows a
// headless apply to exactly the named package, excluding the rest of the set.
func TestRunApply_OnlyFilterHeadless(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	t.Setenv("DOTFILES_ROOT", repoRoot)
	t.Setenv("HOME", home)

	manifestPath := writeTestManifest(t, testToolchainManifest) // rg, fd
	f := newApplyFake(t)

	var stdout, stderr bytes.Buffer
	code := runApply([]string{"--manifest", manifestPath, "--headless", "--only", "rg"}, &stdout, &stderr, f)

	if code != 0 {
		t.Fatalf("runApply(--only rg) code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "result.toolchain=rg:ok") {
		t.Errorf("stdout = %q, want result.toolchain=rg:ok", stdout.String())
	}
	if cargoInstallIndex(f.CallsSnapshot(), "rg") < 0 {
		t.Errorf("--only rg did not install rg; calls=%+v", f.CallsSnapshot())
	}
	if cargoInstallIndex(f.CallsSnapshot(), "fd") >= 0 {
		t.Errorf("--only rg installed fd; want rg only")
	}
}

// TestRunApply_SkipFilterHeadless asserts --skip excludes a package from a
// headless apply while the rest of the set still installs.
func TestRunApply_SkipFilterHeadless(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	t.Setenv("DOTFILES_ROOT", repoRoot)
	t.Setenv("HOME", home)

	manifestPath := writeTestManifest(t, testToolchainManifest) // rg, fd
	f := newApplyFake(t)

	var stdout, stderr bytes.Buffer
	code := runApply([]string{"--manifest", manifestPath, "--headless", "--skip", "fd"}, &stdout, &stderr, f)

	if code != 0 {
		t.Fatalf("runApply(--skip fd) code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if cargoInstallIndex(f.CallsSnapshot(), "rg") < 0 {
		t.Errorf("--skip fd did not install rg; calls=%+v", f.CallsSnapshot())
	}
	if cargoInstallIndex(f.CallsSnapshot(), "fd") >= 0 {
		t.Errorf("--skip fd still installed fd; want it excluded")
	}
}

// TestRunApply_UnknownFilterNameExit2 verifies a mistyped --only/--skip package
// is a usage error (exit 2) on both the real headless and the dry-run path, with
// no sudo/package command run.
func TestRunApply_UnknownFilterNameExit2(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"headless unknown only", []string{"--headless", "--only", "nope"}},
		{"headless unknown skip", []string{"--headless", "--skip", "nope"}},
		{"dry-run unknown only", []string{"--dry-run", "--headless", "--only", "nope"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoRoot := t.TempDir()
			home := t.TempDir()
			t.Setenv("DOTFILES_ROOT", repoRoot)
			t.Setenv("HOME", home)

			manifestPath := writeTestManifest(t, testToolchainManifest) // rg, fd
			f := newApplyFake(t)

			var stdout, stderr bytes.Buffer
			code := runApply(append([]string{"--manifest", manifestPath}, tt.args...), &stdout, &stderr, f)

			if code != 2 {
				t.Fatalf("runApply(%v) code = %d, want 2; stderr=%q", tt.args, code, stderr.String())
			}
			if !strings.Contains(stderr.String(), `unknown package "nope"; manifest defines: fd, rg`) {
				t.Errorf("stderr = %q, want unknown-package diagnostic listing fd, rg", stderr.String())
			}
			for _, c := range f.CallsSnapshot() {
				if c.Name == "sudo" || c.Name == "cargo" {
					t.Errorf("ran %s %v after a usage error; want no install/sudo commands", c.Name, c.Args)
				}
			}
		})
	}
}

func TestRunApply_HeadlessHappyPath_OrderAndEndState(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	t.Setenv("DOTFILES_ROOT", repoRoot)
	t.Setenv("HOME", home)

	// Seed a source so the symlink phase creates a real link in $HOME.
	writeRepoFile(t, repoRoot, filepath.Join("bash", "bashrc"), "# bashrc\n")

	manifestPath := writeTestManifest(t, applyLocalePkgManifest)
	f := newApplyFake(t)

	var stdout, stderr bytes.Buffer
	code := runApply([]string{"--manifest", manifestPath, "--headless"}, &stdout, &stderr, f)

	if code != 0 {
		t.Fatalf("runApply(--headless) code = %d, want 0; stderr=%q", code, stderr.String())
	}

	// Ordering: sudo prime < first sysconfig mutation < first package install.
	calls := f.CallsSnapshot()
	primeIdx := sudoCallIndex(calls, "-v")
	localeIdx := setLocaleIndex(calls)
	cargoIdx := cargoInstallIndex(calls, "rg")
	if primeIdx < 0 {
		t.Fatalf("no `sudo -v` prime recorded; calls=%+v", calls)
	}
	if localeIdx < 0 {
		t.Fatalf("no sysconfig locale mutation recorded; calls=%+v", calls)
	}
	if cargoIdx < 0 {
		t.Fatalf("no `cargo install rg` recorded; calls=%+v", calls)
	}
	if primeIdx >= localeIdx || localeIdx >= cargoIdx {
		t.Errorf("ordering wrong: prime=%d sysconfig=%d package=%d (want prime<sysconfig<package)", primeIdx, localeIdx, cargoIdx)
	}

	// End state: a real symlink now exists in $HOME.
	fi, err := os.Lstat(filepath.Join(home, ".bashrc"))
	if err != nil {
		t.Fatalf("Lstat(.bashrc) error = %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf(".bashrc mode = %v, want a symlink", fi.Mode())
	}

	out := stdout.String()
	for _, want := range []string{"result.locale=ok", "result.toolchain=rg:ok", "link.create=.bashrc"} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout = %q, want substring %q", out, want)
		}
	}

	// A domain applied by the identity pass must NOT also appear in the merged
	// firewall pass's skipped= list (regression guard for mergeSysconfigResults).
	for line := range strings.SplitSeq(out, "\n") {
		if strings.HasPrefix(line, "skipped=") && strings.Contains(line, "locale") {
			t.Errorf("applied domain 'locale' must not be reported as skipped: %q", line)
		}
	}

	errOut := stderr.String()
	if !strings.Contains(errOut, "apply summary: steps=") {
		t.Errorf("stderr = %q, want an apply summary line", errOut)
	}
	if !strings.Contains(errOut, "failed=0") {
		t.Errorf("stderr = %q, want failed=0", errOut)
	}

	// Firewall is unset in this manifest → no ufw enable.
	if n := countSudoCalls(f, "ufw", "--force", "enable"); n != 0 {
		t.Errorf("ufw enable calls = %d, want 0 (firewall unset)", n)
	}
}

func TestRunApply_PackageFailureAccumulatesExit1(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	t.Setenv("DOTFILES_ROOT", repoRoot)
	t.Setenv("HOME", home)

	manifestPath := writeTestManifest(t, testToolchainManifest) // rg, fd
	f := newApplyFake(t)
	f.RunErrors[exec.CommandKey("cargo", "install", "rg")] = os.ErrPermission

	var stdout, stderr bytes.Buffer
	code := runApply([]string{"--manifest", manifestPath, "--headless"}, &stdout, &stderr, f)

	if code != 1 {
		t.Fatalf("runApply() code = %d, want 1 (soft failure); stderr=%q", code, stderr.String())
	}

	// The next package still installs after rg fails.
	calls := f.CallsSnapshot()
	rgIdx := cargoInstallIndex(calls, "rg")
	fdIdx := cargoInstallIndex(calls, "fd")
	if rgIdx < 0 || fdIdx < 0 {
		t.Fatalf("expected both rg and fd installs; calls=%+v", calls)
	}
	if rgIdx >= fdIdx {
		t.Errorf("fd (%d) must install after the failed rg (%d)", fdIdx, rgIdx)
	}

	out := stdout.String()
	if !strings.Contains(out, "rg:failed") {
		t.Errorf("stdout = %q, want rg:failed", out)
	}
	if !strings.Contains(out, "fd:ok") {
		t.Errorf("stdout = %q, want fd:ok", out)
	}

	errOut := stderr.String()
	if !strings.Contains(errOut, "failed=1") {
		t.Errorf("stderr = %q, want failed=1", errOut)
	}
	if !strings.Contains(errOut, "packages:rg:") {
		t.Errorf("stderr = %q, want packages:rg: failure line", errOut)
	}
}

func TestRunApply_MultiPhaseFailuresAllListed(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	t.Setenv("DOTFILES_ROOT", repoRoot)
	t.Setenv("HOME", home)

	manifestPath := writeTestManifest(t, applyLocalePkgManifest) // locale + rg
	f := newApplyFake(t)
	// The locale the sysconfig pass settles on is family-dependent, so key the
	// injected failure to the argv this host's family actually runs.
	f.RunErrors[exec.CommandKey("sudo", "localectl", "set-locale", setLocaleLang(f))] = os.ErrPermission
	f.RunErrors[exec.CommandKey("cargo", "install", "rg")] = os.ErrPermission

	var stdout, stderr bytes.Buffer
	code := runApply([]string{"--manifest", manifestPath, "--headless"}, &stdout, &stderr, f)

	if code != 1 {
		t.Fatalf("runApply() code = %d, want 1; stderr=%q", code, stderr.String())
	}

	errOut := stderr.String()
	if !strings.Contains(errOut, "failed=2") {
		t.Errorf("stderr = %q, want failed=2 (sysconfig + package)", errOut)
	}
	if !strings.Contains(errOut, "sysconfig:locale:") {
		t.Errorf("stderr = %q, want sysconfig:locale: failure line", errOut)
	}
	if !strings.Contains(errOut, "packages:rg:") {
		t.Errorf("stderr = %q, want packages:rg: failure line", errOut)
	}
}

func TestRunApply_SnapperInvokedBeforeMutations(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	t.Setenv("DOTFILES_ROOT", repoRoot)
	t.Setenv("HOME", home)

	manifestPath := writeTestManifest(t, applyLocalePkgManifest) // locale + rg
	f := newApplyFake(t)
	f.AddPath("snapper", "/usr/bin/snapper")
	// The snapshot itself fails — it must stay non-fatal (exit unchanged).
	f.RunErrors[exec.CommandKey("sudo", "snapper", "create", "-d", "dotf apply: pre-apply")] = os.ErrPermission

	var stdout, stderr bytes.Buffer
	code := runApply([]string{"--manifest", manifestPath, "--headless", "--snapper"}, &stdout, &stderr, f)

	if code != 0 {
		t.Fatalf("runApply(--snapper) code = %d, want 0 (snapshot failure is non-fatal); stderr=%q", code, stderr.String())
	}

	calls := f.CallsSnapshot()
	snapIdx := sudoCallIndex(calls, "snapper", "create", "-d", "dotf apply: pre-apply")
	primeIdx := sudoCallIndex(calls, "-v")
	localeIdx := setLocaleIndex(calls)
	cargoIdx := cargoInstallIndex(calls, "rg")
	if snapIdx < 0 {
		t.Fatalf("no `sudo snapper create` recorded; calls=%+v", calls)
	}
	if primeIdx < 0 || primeIdx >= snapIdx {
		t.Errorf("prime=%d must precede snapshot=%d", primeIdx, snapIdx)
	}
	// Fail (not skip) if a downstream mutation is missing: a regression that
	// silently dropped the sysconfig/packages phase would otherwise leave the
	// ordering assertions unexecuted.
	if localeIdx < 0 {
		t.Fatalf("no sysconfig locale mutation recorded; calls=%+v", calls)
	}
	if cargoIdx < 0 {
		t.Fatalf("no `cargo install rg` recorded; calls=%+v", calls)
	}
	if snapIdx >= localeIdx {
		t.Errorf("snapshot=%d must precede sysconfig mutation=%d", snapIdx, localeIdx)
	}
	if snapIdx >= cargoIdx {
		t.Errorf("snapshot=%d must precede package install=%d", snapIdx, cargoIdx)
	}

	errOut := stderr.String()
	if !strings.Contains(errOut, "snapper") {
		t.Errorf("stderr = %q, want a snapper warning", errOut)
	}
	if !strings.Contains(errOut, "failed=0") {
		t.Errorf("stderr = %q, want failed=0 (snapshot never counted)", errOut)
	}
}

func TestRunApply_SnapperNoOpNonLinux(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	t.Setenv("DOTFILES_ROOT", repoRoot)
	t.Setenv("HOME", home)

	manifestPath := writeTestManifest(t, testToolchainManifest)
	f := newApplyDarwinFake(t)

	var stdout, stderr bytes.Buffer
	code := runApply([]string{"--manifest", manifestPath, "--headless", "--snapper"}, &stdout, &stderr, f)

	if code != 0 {
		t.Fatalf("runApply(--snapper on macOS) code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if n := countSudoCalls(f, "snapper", "create", "-d", "dotf apply: pre-apply"); n != 0 {
		t.Errorf("`sudo snapper create` calls = %d, want 0 on macOS", n)
	}
	if !strings.Contains(stderr.String(), "snapper requested but unavailable") {
		t.Errorf("stderr = %q, want an unavailable-snapper warning", stderr.String())
	}
}

func TestRunApply_SymlinkBrokenLinkAccumulates(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	t.Setenv("DOTFILES_ROOT", repoRoot)
	t.Setenv("HOME", home)

	// A real file where the .bashrc symlink is expected, with NO source in the
	// repo → it stays a real file, so post-apply validation reports it broken.
	if err := os.WriteFile(filepath.Join(home, ".bashrc"), []byte("real file\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(.bashrc) error = %v", err)
	}

	manifestPath := writeTestManifest(t, testToolchainManifest) // rg, fd
	f := newApplyFake(t)

	var stdout, stderr bytes.Buffer
	code := runApply([]string{"--manifest", manifestPath, "--headless"}, &stdout, &stderr, f)

	if code != 1 {
		t.Fatalf("runApply() code = %d, want 1 (broken link); stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "symlinks:") {
		t.Errorf("stderr = %q, want a symlinks: failure line", stderr.String())
	}

	// The packages phase still ran despite the symlink failure.
	if cargoInstallIndex(f.CallsSnapshot(), "rg") < 0 {
		t.Errorf("expected `cargo install rg` to still run alongside a broken symlink")
	}
}

// TestRunApply_BacksUpRealFileByDefault is the YAN-1304 acceptance pin:
// `dotf apply --headless` over a real ~/.bashrc leaves a recoverable copy and
// names the backup dir on stderr; the dry run previews it as link.backup=; and
// --no-backup replaces the file without a copy.
func TestRunApply_BacksUpRealFileByDefault(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantBackup bool
	}{
		{"default", nil, true},
		{"no-backup", []string{"--no-backup"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoRoot := t.TempDir()
			home := t.TempDir()
			t.Setenv("DOTFILES_ROOT", repoRoot)
			t.Setenv("HOME", home)
			writeRepoFile(t, repoRoot, filepath.Join("bash", "bashrc"), "# repo bashrc\n")
			if err := os.WriteFile(filepath.Join(home, ".bashrc"), []byte("# user bashrc\n"), 0o644); err != nil {
				t.Fatalf("WriteFile(.bashrc) error = %v", err)
			}
			manifestPath := writeTestManifest(t, testToolchainManifest)

			base := append([]string{"--manifest", manifestPath, "--headless"}, tt.args...)

			var dryOut, dryErr bytes.Buffer
			if code := runApply(append(base, "--dry-run"), &dryOut, &dryErr, newApplyFake(t)); code != 0 {
				t.Fatalf("runApply(--dry-run) code = %d, want 0; stderr=%q", code, dryErr.String())
			}
			if got := strings.Contains(dryOut.String(), "link.backup=.bashrc"); got != tt.wantBackup {
				t.Errorf("dry-run stdout has link.backup=.bashrc = %t, want %t\n%s", got, tt.wantBackup, dryOut.String())
			}

			var stdout, stderr bytes.Buffer
			if code := runApply(base, &stdout, &stderr, newApplyFake(t)); code != 0 {
				t.Fatalf("runApply() code = %d, want 0; stderr=%q", code, stderr.String())
			}
			backups, err := filepath.Glob(filepath.Join(home, ".config", "dotfiles_backup_*"))
			if err != nil {
				t.Fatalf("Glob error = %v", err)
			}
			if !tt.wantBackup {
				if len(backups) != 0 {
					t.Errorf("backup dirs = %v, want none with --no-backup", backups)
				}
				return
			}
			if len(backups) != 1 {
				t.Fatalf("backup dirs = %v, want exactly one", backups)
			}
			got, err := os.ReadFile(filepath.Join(backups[0], ".bashrc"))
			if err != nil {
				t.Fatalf("ReadFile(backup/.bashrc) error = %v", err)
			}
			if string(got) != "# user bashrc\n" {
				t.Errorf("backup content = %q, want the user's original", got)
			}
			if want := "dotf: link backups: " + backups[0] + "\n"; !strings.Contains(stderr.String(), want) {
				t.Errorf("stderr = %q, want %q", stderr.String(), want)
			}
		})
	}
}

// TestRunApply_HyprlandNoBackupIsUsageError: on a host where the desktop set
// applies it needs backups, so --hyprland with --no-backup is rejected as a
// usage error (exit 2) before sudo is primed or any phase runs — only
// platform detection may have executed.
func TestRunApply_HyprlandNoBackupIsUsageError(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	t.Setenv("DOTFILES_ROOT", repoRoot)
	t.Setenv("HOME", home)
	manifestPath := writeTestManifest(t, testToolchainManifest)
	f := newApplyFake(t)
	if info := platform.Detect(f); !symlink.DesktopGuardApplies(info.OS) {
		t.Skipf("host os-release resolves to %q, where the desktop guard does not apply; skipping", info.OS)
	}
	before := len(f.CallsSnapshot())

	var stdout, stderr bytes.Buffer
	code := runApply([]string{"--manifest", manifestPath, "--headless", "--hyprland", "--no-backup"}, &stdout, &stderr, f)
	if code != 2 {
		t.Fatalf("runApply(--hyprland --no-backup) code = %d, want 2; stderr=%q", code, stderr.String())
	}
	if want := "dotf: apply: --hyprland cannot be combined with --no-backup"; !strings.HasPrefix(stderr.String(), want) {
		t.Errorf("stderr = %q, want prefix %q", stderr.String(), want)
	}
	for _, call := range f.CallsSnapshot()[before:] {
		if call.Name != "uname" {
			t.Errorf("unexpected call %+v before the usage error (only platform detection may run)", call)
		}
	}
}

func TestRunApply_SudoPrimeFailureFatal(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	t.Setenv("DOTFILES_ROOT", repoRoot)
	t.Setenv("HOME", home)

	manifestPath := writeTestManifest(t, testToolchainManifest)
	f := newApplyFake(t)
	f.RunErrors[exec.CommandKey("sudo", "-v")] = os.ErrPermission

	var stdout, stderr bytes.Buffer
	code := runApply([]string{"--manifest", manifestPath, "--headless"}, &stdout, &stderr, f)

	if code != 3 {
		t.Fatalf("runApply() code = %d, want 3 (sudo prime hard abort); stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "sudo prime") {
		t.Errorf("stderr = %q, want a sudo prime diagnostic", stderr.String())
	}
	if strings.Contains(stderr.String(), "apply summary:") {
		t.Errorf("stderr = %q, must not print a summary on a hard abort", stderr.String())
	}
	if cargoInstallIndex(f.CallsSnapshot(), "rg") >= 0 {
		t.Errorf("must not install packages after a failed sudo prime")
	}
}

func TestRunApply_NonTTYWithoutHeadlessRefuses(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	t.Setenv("DOTFILES_ROOT", repoRoot)
	t.Setenv("HOME", home)

	manifestPath := writeTestManifest(t, testToolchainManifest)
	f := newApplyFake(t)

	// stdout is a bytes.Buffer (non-TTY); without --headless the apply refuses.
	var stdout, stderr bytes.Buffer
	code := runApply([]string{"--manifest", manifestPath}, &stdout, &stderr, f)

	if code != 3 {
		t.Fatalf("runApply(non-tty, no --headless) code = %d, want 3; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "refusing") {
		t.Errorf("stderr = %q, want a refusing message", stderr.String())
	}
	for _, call := range f.CallsSnapshot() {
		if call.Name == "sudo" {
			t.Errorf("recorded a sudo call on refusal: %+v", call)
		}
	}
}

func TestRunApply_BadFlagExit2(t *testing.T) {
	f := exec.NewFake()
	var stdout, stderr bytes.Buffer
	code := runApply([]string{"--bogus"}, &stdout, &stderr, f)

	if code != 2 {
		t.Errorf("runApply(--bogus) code = %d, want 2", code)
	}
}

func TestRunApply_InvalidHomebrewExit2(t *testing.T) {
	f := exec.NewFake()
	var stdout, stderr bytes.Buffer
	code := runApply([]string{"--headless", "--homebrew=maybe"}, &stdout, &stderr, f)

	if code != 2 {
		t.Fatalf("runApply(--homebrew=maybe) code = %d, want 2; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "invalid --homebrew") {
		t.Errorf("stderr = %q, want an invalid --homebrew diagnostic", stderr.String())
	}
	for _, call := range f.CallsSnapshot() {
		if call.Name == "sudo" {
			t.Errorf("recorded a sudo call on a usage error: %+v", call)
		}
	}
}

func TestRunApply_ManifestResolutionErrorFatal(t *testing.T) {
	t.Setenv("DOTFILES_ROOT", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	f := newApplyFake(t)
	var stdout, stderr bytes.Buffer
	// A --manifest path that does not exist fails manifest.Resolve before the
	// mode gate: a hard abort (exit 3), not a soft completed-with-failures run.
	code := runApply([]string{"--headless", "--manifest", filepath.Join(t.TempDir(), "nope.toml")}, &stdout, &stderr, f)

	if code != 3 {
		t.Fatalf("runApply(missing manifest) code = %d, want 3 (hard abort); stderr=%q", code, stderr.String())
	}
	if !strings.HasPrefix(stderr.String(), "dotf: ") {
		t.Errorf("stderr = %q, want a dotf: diagnostic", stderr.String())
	}
	if strings.Contains(stderr.String(), "apply summary:") {
		t.Errorf("hard abort must not print an apply summary; stderr=%q", stderr.String())
	}
	for _, call := range f.CallsSnapshot() {
		if call.Name == "sudo" {
			t.Errorf("recorded a sudo call before a resolvable manifest: %+v", call)
		}
	}
}

// TestRunApply_HomebrewTrueOnLinuxBootstrapsLinuxbrew asserts that --homebrew=true
// on Linux is no longer inert: it runs the official installer to bootstrap
// linuxbrew (the pre-parity behavior only printed a "no-op on Linux" warning).
func TestRunApply_HomebrewTrueOnLinuxBootstrapsLinuxbrew(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	t.Setenv("DOTFILES_ROOT", repoRoot)
	t.Setenv("HOME", home)
	// applyBrew prepends the linuxbrew bin dir to PATH on success; snapshot PATH
	// so t restores it after this test rather than leaking into later tests.
	t.Setenv("PATH", os.Getenv("PATH"))

	manifestPath := writeTestManifest(t, testToolchainManifest)
	f := newApplyFake(t) // Linux, brew NOT on PATH

	var stdout, stderr bytes.Buffer
	code := runApply([]string{"--manifest", manifestPath, "--headless", "--homebrew=true"}, &stdout, &stderr, f)

	if code != 0 {
		t.Fatalf("runApply(--homebrew=true) code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if !brewInstallerCalled(f) {
		t.Errorf("expected the Homebrew installer to run for --homebrew=true on Linux; calls=%+v", f.CallsSnapshot())
	}
}

// TestRunApply_MacOSBootstrapsHomebrew covers the brew-bootstrap step: on a bare
// macOS host (brew absent) a headless apply runs the official installer before
// the packages phase, unblocking the pkg backend's `brew install` path.
func TestRunApply_MacOSBootstrapsHomebrew(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	t.Setenv("DOTFILES_ROOT", repoRoot)
	t.Setenv("HOME", home)
	t.Setenv("PATH", os.Getenv("PATH")) // restored after the test (applyBrew mutates it)

	// A cargo package needs no PkgMgr, so the packages phase stays green even
	// though the Fake cannot simulate brew appearing on PATH post-install.
	manifestPath := writeTestManifest(t, testToolchainManifest)
	f := newApplyDarwinFake(t) // macOS, brew NOT on PATH

	var stdout, stderr bytes.Buffer
	code := runApply([]string{"--manifest", manifestPath, "--headless"}, &stdout, &stderr, f)

	if code != 0 {
		t.Fatalf("runApply(macOS headless) code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if !brewInstallerCalled(f) {
		t.Errorf("expected a Homebrew bootstrap on bare macOS; calls=%+v", f.CallsSnapshot())
	}
}

// TestRunApply_MacOSBrewPresentNoBootstrap asserts the bootstrap is skipped when
// brew is already installed — the installer must not run on a converged host.
func TestRunApply_MacOSBrewPresentNoBootstrap(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	t.Setenv("DOTFILES_ROOT", repoRoot)
	t.Setenv("HOME", home)

	manifestPath := writeTestManifest(t, testToolchainManifest)
	f := newApplyDarwinFake(t)
	f.AddPath("brew", "/opt/homebrew/bin/brew") // brew already present

	var stdout, stderr bytes.Buffer
	code := runApply([]string{"--manifest", manifestPath, "--headless"}, &stdout, &stderr, f)

	if code != 0 {
		t.Fatalf("runApply(macOS, brew present) code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if brewInstallerCalled(f) {
		t.Errorf("Homebrew installer must not run when brew is already present; calls=%+v", f.CallsSnapshot())
	}
}

func TestRunApply_LinuxDesktopFlagPassthrough(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	t.Setenv("DOTFILES_ROOT", repoRoot)
	t.Setenv("HOME", home)

	manifestPath := writeTestManifest(t, testToolchainManifest)
	f := newApplyFake(t)

	info := platform.Detect(f)
	if !isDesktopFamily(info.OS) {
		t.Skipf("host os-release resolves to %q, not a desktop-capable Linux family; skipping", info.OS)
	}

	// Seed a desktop source so the opt-in facet links it into $HOME.
	writeRepoFile(t, repoRoot, filepath.Join("hypr", "hyprland.conf"), "# hypr\n")

	var stdout, stderr bytes.Buffer
	code := runApply([]string{"--manifest", manifestPath, "--headless", "--linux-desktop"}, &stdout, &stderr, f)

	if code != 0 {
		t.Fatalf("runApply(--linux-desktop) code = %d, want 0 (no LinuxDesktopGuardError); stderr=%q", code, stderr.String())
	}

	// Backups are on by default, so the engine guard admits --linux-desktop and
	// .config/hypr is a real symlink in $HOME.
	fi, err := os.Lstat(filepath.Join(home, ".config", "hypr"))
	if err != nil {
		t.Fatalf("Lstat(.config/hypr) error = %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf(".config/hypr mode = %v, want a symlink (linux-desktop set managed)", fi.Mode())
	}
	if !strings.Contains(stdout.String(), ".config/hypr") {
		t.Errorf("stdout = %q, want a .config/hypr link line", stdout.String())
	}
}

// TestRunApply_DryRunPlansSSHServer asserts `dotf apply --dry-run` emits
// sysconfig.step.ssh-server= lines for a manifest with ssh_server=on without
// priming sudo or mutating $HOME.
func TestRunApply_DryRunPlansSSHServer(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	t.Setenv("DOTFILES_ROOT", repoRoot)
	t.Setenv("HOME", home)

	manifestPath := writeTestManifest(t, applySSHServerPkgManifest)
	f := newApplyFake(t)
	f.AddPath("ufw", "/usr/bin/ufw")
	f.AddCommand("sudo", []string{"-n", "ufw", "status"}, []byte("Status: inactive\n"), nil)
	f.AddCommand("systemctl", []string{"is-active", "ssh"}, []byte("inactive\n"), nil)
	f.AddCommand("systemctl", []string{"is-active", "sshd"}, []byte("inactive\n"), nil)

	var stdout, stderr bytes.Buffer
	code := runApply([]string{"--manifest", manifestPath, "--dry-run", "--headless"}, &stdout, &stderr, f)

	if code != 0 {
		t.Fatalf("runApply(--dry-run ssh_server=on) code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "sysconfig.step.ssh-server=") {
		t.Errorf("stdout = %q, want sysconfig.step.ssh-server= lines", stdout.String())
	}
	for _, call := range f.CallsSnapshot() {
		if call.Name != "sudo" {
			continue
		}
		if len(call.Args) >= 3 && call.Args[0] == "-n" && call.Args[1] == "ufw" && call.Args[2] == "status" {
			continue
		}
		t.Errorf("dry-run invoked sudo (%+v), want at most `sudo -n ufw status`", call)
	}
}

// TestRunApply_SSHServerRunsAfterFirewall asserts applySSHServer runs after the
// firewall pass and prints result.ssh-server= on stdout.
func TestRunApply_SSHServerRunsAfterFirewall(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	t.Setenv("DOTFILES_ROOT", repoRoot)
	t.Setenv("HOME", home)

	manifestPath := writeTestManifest(t, applySSHServerFirewallPkgManifest)
	f := newApplyFake(t)
	// ufw is NOT registered on PATH → the firewall pass emits install steps.
	f.AddCommand("systemctl", []string{"is-active", "ssh"}, []byte("inactive\n"), nil)
	f.AddCommand("systemctl", []string{"is-active", "sshd"}, []byte("inactive\n"), nil)

	var stdout, stderr bytes.Buffer
	code := runApply([]string{"--manifest", manifestPath, "--headless"}, &stdout, &stderr, f)

	if code != 0 {
		t.Fatalf("runApply() code = %d, want 0; stderr=%q", code, stderr.String())
	}

	calls := f.CallsSnapshot()
	fwEnableIdx := sudoCallIndex(calls, "ufw", "--force", "enable")
	sshEnableIdx := sshServerEnableCallIndex(calls)
	if fwEnableIdx < 0 {
		t.Fatalf("no `sudo ufw --force enable` recorded; calls=%+v", calls)
	}
	if sshEnableIdx < 0 {
		t.Fatalf("no ssh-server enable step recorded; calls=%+v", calls)
	}
	if fwEnableIdx >= sshEnableIdx {
		t.Errorf("ssh-server enable (%d) must run after firewall enable (%d)", sshEnableIdx, fwEnableIdx)
	}
	if !strings.Contains(stdout.String(), "result.ssh-server=ok") {
		t.Errorf("stdout = %q, want result.ssh-server=ok", stdout.String())
	}
}

// sshServerEnableCallIndex returns the index of the first recorded sudo
// systemctl enable --now ssh|sshd call from the ssh-server pass, or -1.
func sshServerEnableCallIndex(calls []exec.FakeCall) int {
	for i, call := range calls {
		if call.Name != "sudo" || len(call.Args) < 4 {
			continue
		}
		if call.Args[0] != "systemctl" || call.Args[1] != "enable" || call.Args[2] != "--now" {
			continue
		}
		if call.Args[3] == "ssh" || call.Args[3] == "sshd" {
			return i
		}
	}
	return -1
}

func TestRunApply_FirewallRunsAfterPackages(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	t.Setenv("DOTFILES_ROOT", repoRoot)
	t.Setenv("HOME", home)

	manifestPath := writeTestManifest(t, applyFirewallPkgManifest) // firewall=ufw + rg
	f := newApplyFake(t)
	// ufw is NOT registered on PATH → the plan emits install + allow + enable
	// steps without a `sudo -n ufw status` probe.

	var stdout, stderr bytes.Buffer
	code := runApply([]string{"--manifest", manifestPath, "--headless"}, &stdout, &stderr, f)

	if code != 0 {
		t.Fatalf("runApply() code = %d, want 0; stderr=%q", code, stderr.String())
	}

	calls := f.CallsSnapshot()
	cargoIdx := cargoInstallIndex(calls, "rg")
	fwEnableIdx := sudoCallIndex(calls, "ufw", "--force", "enable")
	if cargoIdx < 0 {
		t.Fatalf("no `cargo install rg` recorded; calls=%+v", calls)
	}
	if fwEnableIdx < 0 {
		t.Fatalf("no `sudo ufw --force enable` recorded; calls=%+v", calls)
	}
	if cargoIdx >= fwEnableIdx {
		t.Errorf("firewall enable (%d) must run after the packages phase (%d)", fwEnableIdx, cargoIdx)
	}
}

// TestApplyPackages_NotApplicableRecordedOK is the apply-phase issue #14
// regression test: a platform-inapplicable package is skipped as
// "not-applicable", recorded as an ok step in the failure collector (so a
// converged host exits 0), and never reaches the backend's Install.
func TestApplyPackages_NotApplicableRecordedOK(t *testing.T) {
	tests := []struct {
		name    string
		pkgName string
		info    platform.Info
	}{
		{"apt-repo backend on arch host", "docker-ce", archInfo()},
		{"aur backend on debian host", "lutris", debianInfo()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := exec.NewFake()
			engine := testDriftEngine(t, fake)
			m := parseManifestString(t, applicabilityTestManifest)
			pkgs := packagesByName(m, []string{tt.pkgName})
			if len(pkgs) != 1 {
				t.Fatalf("packagesByName(%q) = %d packages, want 1", tt.pkgName, len(pkgs))
			}

			var coll failure.Collector
			results := applyPackages(context.Background(), engine, pkgs, tt.info, false, &coll)

			if len(results) != 1 {
				t.Fatalf("applyPackages() results = %d, want 1", len(results))
			}
			if results[0].status != "not-applicable" {
				t.Errorf("status = %q, want not-applicable", results[0].status)
			}
			if results[0].err != nil {
				t.Errorf("err = %v, want nil", results[0].err)
			}
			if coll.Failed() {
				t.Errorf("collector failed = true, want false: %+v", coll.Failures())
			}
			if calls := fake.CallsSnapshot(); len(calls) != 0 {
				t.Errorf("commands ran = %+v, want none", calls)
			}
		})
	}
}

// TestApplyPackages_SkipsAlreadyInstalled is the apply-phase idempotency
// regression test (#40/#44): a package the host already has (git, reported
// installed via `pacman -Q git`) is skipped as "already-installed" and never
// reinstalled — and a second identical pass over the converged host runs no
// install argv either.
func TestApplyPackages_SkipsAlreadyInstalled(t *testing.T) {
	fake := archPacmanFake(t) // pacman -Q git reports git installed
	engine := testDriftEngine(t, fake)
	m := parseManifestString(t, applicabilityTestManifest)
	pkgs := packagesByName(m, []string{"git"})
	if len(pkgs) != 1 {
		t.Fatalf("packagesByName(git) = %d packages, want 1", len(pkgs))
	}

	var coll failure.Collector
	results := applyPackages(context.Background(), engine, pkgs, archInfo(), false, &coll)

	if len(results) != 1 {
		t.Fatalf("applyPackages() results = %d, want 1", len(results))
	}
	if results[0].status != "already-installed" {
		t.Errorf("status = %q, want already-installed", results[0].status)
	}
	if coll.Failed() {
		t.Errorf("collector failed = true, want false: %+v", coll.Failures())
	}
	if n := countSudoCalls(fake, "pacman", "-S", "--noconfirm", "--needed", "git"); n != 0 {
		t.Errorf("install argv ran %d times, want 0 (git already installed)", n)
	}

	// A second identical pass over a converged host installs nothing either.
	_ = applyPackages(context.Background(), engine, pkgs, archInfo(), false, &coll)
	if n := countSudoCalls(fake, "pacman", "-S", "--noconfirm", "--needed", "git"); n != 0 {
		t.Errorf("install argv ran %d times across two passes, want 0", n)
	}
}

// ----------------------------------------------------------------------------
// plasma phase
// ----------------------------------------------------------------------------

// plasmaSteps renders the collector's aggregate line so a test can assert the
// EXACT number of steps a phase recorded. The count is otherwise unexported,
// and "exactly one Record per path" is the phase's core invariant.
func plasmaSteps(t *testing.T, coll *failure.Collector) string {
	t.Helper()
	var buf bytes.Buffer
	coll.WriteSummary(&buf, progName)
	return buf.String()
}

// newPlasmaPhaseEngine builds an engine over fresh repo/config temp trees with
// both KDE session variables cleared, so the phase never inherits the
// developer's running desktop.
func newPlasmaPhaseEngine(t *testing.T) (*plasma.Engine, string, string) {
	t.Helper()
	repo, config := t.TempDir(), t.TempDir()
	t.Setenv("KDE_FULL_SESSION", "")
	t.Setenv("XDG_CURRENT_DESKTOP", "")
	return plasma.NewEngine(repo, config, "", nil, exec.NewFake()), repo, config
}

// plasmaOnlyKiorc narrows the phase to the one managed file with no volatile
// strip rules, keeping these tests about the PHASE contract rather than the
// strip table.
func plasmaOnlyKiorc(t *testing.T) []plasma.FileSpec {
	t.Helper()
	specs, err := plasmaSelectFiles(plasma.DefaultFiles(), "kiorc")
	if err != nil {
		t.Fatalf("plasmaSelectFiles error = %v", err)
	}
	return specs
}

// TestApplyPlasma_RecordsOneStepOnSuccess proves the happy path records exactly
// one collector step — Record("plasma", nil) — so the run-wide step count stays
// honest even when nothing failed.
func TestApplyPlasma_RecordsOneStepOnSuccess(t *testing.T) {
	eng, repo, config := newPlasmaPhaseEngine(t)
	writeUnder(t, filepath.Join(repo, "plasma", "files"), "kiorc", repoKiorc)

	var coll failure.Collector
	var stderr bytes.Buffer
	report := applyPlasma(eng, plasmaOnlyKiorc(t), plasma.Options{Validate: true}, &stderr, &coll)

	if report == nil {
		t.Fatal("report = nil, want a report")
	}
	if coll.Failed() {
		t.Errorf("collector failed: %+v", coll.Failures())
	}
	if got := plasmaSteps(t, &coll); !strings.Contains(got, "steps=1 ok=1 failed=0") {
		t.Errorf("summary = %q, want exactly one ok step", got)
	}
	if _, err := os.Stat(filepath.Join(config, "kiorc")); err != nil {
		t.Errorf("phase did not materialise the live file: %v", err)
	}
}

// TestApplyPlasma_RecordsOneStepOnPlanError proves a plan failure is recorded
// once and never aborts the run.
func TestApplyPlasma_RecordsOneStepOnPlanError(t *testing.T) {
	t.Setenv("KDE_FULL_SESSION", "")
	t.Setenv("XDG_CURRENT_DESKTOP", "")
	// A repo root that does not exist at all is a plan-time hard failure,
	// distinct from the "exists but was never exported" case below.
	eng := plasma.NewEngine(filepath.Join(t.TempDir(), "absent"), t.TempDir(), "", nil, exec.NewFake())

	var coll failure.Collector
	var stderr bytes.Buffer
	if report := applyPlasma(eng, plasmaOnlyKiorc(t), plasma.Options{Validate: true}, &stderr, &coll); report != nil {
		t.Errorf("report = %+v, want nil on a plan error", report)
	}
	if got := plasmaSteps(t, &coll); !strings.Contains(got, "steps=1 ok=0 failed=1") {
		t.Errorf("summary = %q, want exactly one failed step", got)
	}
}

// TestApplyPlasma_RecordsOneStepOnExecuteError proves an execute failure is
// recorded once. A regular file where the backup directory must be created is a
// hermetic, root-safe way to make Execute fail before touching any config file.
func TestApplyPlasma_RecordsOneStepOnExecuteError(t *testing.T) {
	eng, repo, config := newPlasmaPhaseEngine(t)
	writeUnder(t, filepath.Join(repo, "plasma", "files"), "kiorc", repoKiorc)
	if err := os.WriteFile(filepath.Join(config, "dotf"), []byte("not a directory\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var coll failure.Collector
	var stderr bytes.Buffer
	applyPlasma(eng, plasmaOnlyKiorc(t), plasma.Options{Backup: true, Validate: true}, &stderr, &coll)

	if got := plasmaSteps(t, &coll); !strings.Contains(got, "steps=1 ok=0 failed=1") {
		t.Errorf("summary = %q, want exactly one failed step", got)
	}
}

// TestApplyPlasma_RecordsOneStepOnValidateError proves post-apply drift is
// recorded once. DryRun makes Execute a provable no-op, so the live file stays
// unwritten and Validate reports the drift the phase then records.
func TestApplyPlasma_RecordsOneStepOnValidateError(t *testing.T) {
	eng, repo, config := newPlasmaPhaseEngine(t)
	writeUnder(t, filepath.Join(repo, "plasma", "files"), "kiorc", repoKiorc)
	writeUnder(t, config, "kiorc", liveKiorc)

	var coll failure.Collector
	var stderr bytes.Buffer
	applyPlasma(eng, plasmaOnlyKiorc(t), plasma.Options{DryRun: true, Validate: true}, &stderr, &coll)

	if !coll.Failed() {
		t.Fatal("collector recorded no failure, want the post-apply drift")
	}
	if got := plasmaSteps(t, &coll); !strings.Contains(got, "steps=1 ok=0 failed=1") {
		t.Errorf("summary = %q, want exactly one failed step", got)
	}
}

// TestApplyPlasma_MissingRepoTreeRecordsNothing proves a repo that has never run
// `dotf plasma export` is a warning, not a failed apply.
func TestApplyPlasma_MissingRepoTreeRecordsNothing(t *testing.T) {
	eng, _, _ := newPlasmaPhaseEngine(t)

	var coll failure.Collector
	var stderr bytes.Buffer
	if report := applyPlasma(eng, plasmaOnlyKiorc(t), plasma.Options{Validate: true}, &stderr, &coll); report != nil {
		t.Errorf("report = %+v, want nil", report)
	}
	if got := plasmaSteps(t, &coll); !strings.Contains(got, "steps=0") {
		t.Errorf("summary = %q, want no recorded step", got)
	}
	if !strings.Contains(stderr.String(), "dotf plasma export") {
		t.Errorf("stderr = %q, want the export remedy", stderr.String())
	}
}

// TestApplyPlasmaPhase_NonLinuxWarnsAndRecordsNothing proves requesting the
// phase on a non-Linux host warns and records nothing, following applySnapshot's
// requested-but-unavailable precedent.
func TestApplyPlasmaPhase_NonLinuxWarnsAndRecordsNothing(t *testing.T) {
	t.Setenv("KDE_FULL_SESSION", "")
	t.Setenv("XDG_CURRENT_DESKTOP", "")

	var coll failure.Collector
	var stderr bytes.Buffer
	report := applyPlasmaPhase(exec.NewFake(), platform.Info{OS: platform.OSMacOS},
		applyOptions{Plasma: true}, t.TempDir(), t.TempDir(), &stderr, &coll)

	if report != nil {
		t.Errorf("report = %+v, want nil on an unsupported host", report)
	}
	if got := plasmaSteps(t, &coll); !strings.Contains(got, "steps=0") {
		t.Errorf("summary = %q, want no recorded step", got)
	}
	if want := "dotf: plasma requested but the host is not Linux; skipping\n"; stderr.String() != want {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
}

// TestApplyPlasmaOptions_BackupFollowsOptIn locks the deliberate choices:
// Backup follows the opt-in flag (the safety net comes with the destructive
// phase), Validate is always on, and Link/Recommended/Reload are never set from
// the headless orchestrator.
func TestApplyPlasmaOptions_BackupFollowsOptIn(t *testing.T) {
	t.Setenv("KDE_FULL_SESSION", "")
	t.Setenv("XDG_CURRENT_DESKTOP", "")

	got := applyPlasmaOptions(applyOptions{Plasma: true, DryRun: true})
	if !got.Backup || !got.Validate || !got.DryRun {
		t.Errorf("options = %+v, want Backup/Validate/DryRun all true", got)
	}
	if got.Link || got.Recommended || got.Reload {
		t.Errorf("options = %+v, want Link/Recommended/Reload all false", got)
	}

	off := applyPlasmaOptions(applyOptions{})
	if off.Backup {
		t.Errorf("options = %+v, want Backup false when the phase was not opted into", off)
	}
}

// TestRunApply_PlasmaOffByDefault proves the phase contributes nothing to a
// dry run that did not ask for it.
func TestRunApply_PlasmaOffByDefault(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	t.Setenv("DOTFILES_ROOT", repoRoot)
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeUnder(t, filepath.Join(repoRoot, "plasma", "files"), "kiorc", repoKiorc)

	manifestPath := writeTestManifest(t, testToolchainManifest)

	var stdout, stderr bytes.Buffer
	if code := runApply([]string{"--manifest", manifestPath, "--dry-run"}, &stdout, &stderr, newApplyFake(t)); code != 0 {
		t.Fatalf("code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "plasma.") {
		t.Errorf("stdout = %q, want no plasma lines without --plasma", stdout.String())
	}
}

// TestRunApply_PlasmaDryRunComposesBucketLines proves --plasma routes the phase
// preview through the SAME plasmaActionLines formatter `dotf plasma apply
// --dry-run` uses, so there is no second renderer to drift from.
func TestRunApply_PlasmaDryRunComposesBucketLines(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	t.Setenv("DOTFILES_ROOT", repoRoot)
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("KDE_FULL_SESSION", "")
	t.Setenv("XDG_CURRENT_DESKTOP", "")
	writeUnder(t, filepath.Join(repoRoot, "plasma", "files"), "kiorc", repoKiorc)

	manifestPath := writeTestManifest(t, testToolchainManifest)

	var stdout, stderr bytes.Buffer
	code := runApply([]string{"--manifest", manifestPath, "--dry-run", "--plasma"}, &stdout, &stderr, newApplyFake(t))
	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "plasma.create=kiorc") {
		t.Errorf("stdout = %q, want the plasma.create bucket line", stdout.String())
	}
}
