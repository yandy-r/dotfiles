package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	dotfiles "github.com/yandy-r/dotfiles"
	"github.com/yandy-r/dotfiles/internal/backend"
	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

const testToolchainManifest = `
[[package]]
name = "rg"
backend = "toolchain"
tool = "cargo"

[[package]]
name = "fd"
backend = "toolchain"
tool = "cargo"
`

const testIntentManifest = `
[intents.base]
description = "base"
default = true
packages = ["rg"]

[intents.dev]
description = "dev"
default = false
packages = ["fd"]

[[package]]
name = "rg"
backend = "toolchain"
tool = "cargo"

[[package]]
name = "fd"
backend = "toolchain"
tool = "cargo"
`

func writeTestManifest(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func newInstallFake(t *testing.T) *exec.Fake {
	t.Helper()
	f := exec.NewFake()
	f.AddPath("cargo", "/usr/bin/cargo")
	return f
}

func TestRunInstall_DryRun(t *testing.T) {
	manifestPath := writeTestManifest(t, testToolchainManifest)
	f := exec.NewFake()

	var stdout, stderr bytes.Buffer
	code := runInstall([]string{"--manifest", manifestPath, "--dry-run"}, &stdout, &stderr, f)

	if code != 0 {
		t.Fatalf("runInstall() code = %d, want 0", code)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{
		"action=install",
		"dry-run=true",
		"packages=rg,fd",
		"plan.toolchain=rg,fd",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout = %q, want substring %q", out, want)
		}
	}

	for _, call := range f.CallsSnapshot() {
		if call.Name == "sudo" {
			t.Errorf("dry-run invoked sudo (%+v), want no sudo calls", call)
		}
	}
}

func TestRunInstall_SudoPrimeOnce(t *testing.T) {
	manifestPath := writeTestManifest(t, testToolchainManifest)
	f := newInstallFake(t)

	var stdout, stderr bytes.Buffer
	code := runInstall([]string{"--manifest", manifestPath}, &stdout, &stderr, f)

	if code != 0 {
		t.Fatalf("runInstall() code = %d, want 0; stderr=%q", code, stderr.String())
	}

	primeCount := 0
	for _, call := range f.CallsSnapshot() {
		if call.Name == "sudo" && len(call.Args) == 1 && call.Args[0] == "-v" {
			primeCount++
		}
	}
	if primeCount != 1 {
		t.Errorf("sudo -v prime count = %d, want 1", primeCount)
	}

	cargoInstalls := 0
	for _, call := range f.CallsSnapshot() {
		if call.Name == "cargo" && len(call.Args) >= 2 && call.Args[0] == "install" {
			cargoInstalls++
		}
	}
	if cargoInstalls != 2 {
		t.Errorf("cargo install calls = %d, want 2 (one per package)", cargoInstalls)
	}
}

func TestRunInstall_IntentFilter(t *testing.T) {
	manifestPath := writeTestManifest(t, testIntentManifest)
	f := newInstallFake(t)

	var stdout, stderr bytes.Buffer
	code := runInstall([]string{"--manifest", manifestPath, "--intent", "base"}, &stdout, &stderr, f)

	if code != 0 {
		t.Fatalf("runInstall() code = %d, want 0; stderr=%q", code, stderr.String())
	}

	cargoInstalls := 0
	for _, call := range f.CallsSnapshot() {
		if call.Name == "cargo" && len(call.Args) >= 2 && call.Args[0] == "install" {
			cargoInstalls++
			if len(call.Args) >= 2 && call.Args[1] != "rg" {
				t.Errorf("cargo install target = %q, want rg only", call.Args[1])
			}
		}
	}
	if cargoInstalls != 1 {
		t.Errorf("cargo install calls = %d, want 1", cargoInstalls)
	}

	if !strings.Contains(stdout.String(), "result.toolchain=rg:ok") {
		t.Errorf("stdout = %q, want result.toolchain=rg:ok", stdout.String())
	}
}

// TestRunInstall_UnknownIntent verifies a mistyped --intent is a usage error
// (exit 2) naming the unknown intent and the manifest's valid intents — never
// a silent zero-package success. In real mode the gate must fire before sudo
// is primed, so no commands run at all.
func TestRunInstall_UnknownIntent(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"dry-run", []string{"--dry-run", "--intent", "typo"}},
		{"real", []string{"--intent", "typo"}},
		{"valid then unknown", []string{"--intent", "base", "--intent", "typo"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifestPath := writeTestManifest(t, testIntentManifest)
			f := newInstallFake(t)

			var stdout, stderr bytes.Buffer
			code := runInstall(append([]string{"--manifest", manifestPath}, tt.args...), &stdout, &stderr, f)

			if code != 2 {
				t.Fatalf("runInstall(%v) code = %d, want 2; stderr=%q", tt.args, code, stderr.String())
			}
			if !strings.Contains(stderr.String(), `unknown intent "typo"; manifest defines: base, dev`) {
				t.Errorf("stderr = %q, want unknown-intent diagnostic listing base, dev", stderr.String())
			}
			if calls := f.CallsSnapshot(); len(calls) != 0 {
				t.Errorf("commands ran (%+v), want none (no sudo prime, no installs)", calls)
			}
		})
	}
}

// TestValidateIntents covers the helper directly: nil and known names pass,
// the first unknown name errors with the sorted valid-intent list.
func TestValidateIntents(t *testing.T) {
	m, err := manifest.Parse(strings.NewReader(testIntentManifest))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	tests := []struct {
		name    string
		intents []string
		wantErr string
	}{
		{"nil", nil, ""},
		{"known", []string{"base", "dev"}, ""},
		{"unknown", []string{"devs"}, `unknown intent "devs"; manifest defines: base, dev`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateIntents(m, tt.intents)
			switch {
			case tt.wantErr == "" && err != nil:
				t.Errorf("validateIntents(%v) error = %v, want nil", tt.intents, err)
			case tt.wantErr != "" && (err == nil || err.Error() != tt.wantErr):
				t.Errorf("validateIntents(%v) error = %v, want %q", tt.intents, err, tt.wantErr)
			}
		})
	}
}

// TestFilterPackages covers the #42 package-level filter helper directly:
// --only replaces the base set independent of the intent-derived pkgs, --skip
// excludes after selection, the two compose, an empty result is valid, and an
// unknown name in either flag is a clear error listing the valid packages.
func TestFilterPackages(t *testing.T) {
	m := parseManifestString(t, testIntentManifest) // packages in manifest order: rg, fd
	all := m.Packages
	// Simulate an intent-derived base set (as `--intent dev` would produce).
	fromIntent := packagesByName(m, []string{"fd"})

	names := func(pkgs []manifest.Package) []string {
		out := make([]string, len(pkgs))
		for i, p := range pkgs {
			out[i] = p.Name
		}
		return out
	}

	tests := []struct {
		name    string
		pkgs    []manifest.Package
		only    []string
		skip    []string
		want    []string
		wantErr string
	}{
		{"no filters pass through", all, nil, nil, []string{"rg", "fd"}, ""},
		{"only selects named independent of intent", fromIntent, []string{"rg"}, nil, []string{"rg"}, ""},
		{"only is manifest-ordered and deduped", all, []string{"fd", "rg", "fd"}, nil, []string{"rg", "fd"}, ""},
		{"skip excludes from base", all, nil, []string{"fd"}, []string{"rg"}, ""},
		{"only and skip compose", all, []string{"rg", "fd"}, []string{"fd"}, []string{"rg"}, ""},
		{"empty result is valid", all, []string{"rg"}, []string{"rg"}, []string{}, ""},
		{"unknown only name errors", all, []string{"nope"}, nil, nil, `unknown package "nope"; manifest defines: fd, rg`},
		{"unknown skip name errors", all, nil, []string{"nope"}, nil, `unknown package "nope"; manifest defines: fd, rg`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := filterPackages(m, tt.pkgs, tt.only, tt.skip)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("filterPackages() err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("filterPackages() err = %v, want nil", err)
			}
			if !equalStringSlices(names(got), tt.want) {
				t.Errorf("filterPackages() = %v, want %v", names(got), tt.want)
			}
		})
	}
}

// TestRunInstall_OnlyOverridesIntent proves the #42 "--only is independent of
// --intent" acceptance: `--intent dev` alone would install fd, but `--only rg`
// replaces that selection with rg from the whole manifest.
func TestRunInstall_OnlyOverridesIntent(t *testing.T) {
	manifestPath := writeTestManifest(t, testIntentManifest) // rg, fd
	f := newInstallFake(t)

	var stdout, stderr bytes.Buffer
	code := runInstall([]string{"--manifest", manifestPath, "--intent", "dev", "--only", "rg"}, &stdout, &stderr, f)

	if code != 0 {
		t.Fatalf("runInstall(--intent dev --only rg) code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "result.toolchain=rg:ok") {
		t.Errorf("stdout = %q, want result.toolchain=rg:ok", stdout.String())
	}
	if cargoInstallIndex(f.CallsSnapshot(), "rg") < 0 {
		t.Errorf("--only rg did not install rg; calls=%+v", f.CallsSnapshot())
	}
	if cargoInstallIndex(f.CallsSnapshot(), "fd") >= 0 {
		t.Errorf("--only rg installed fd (the intent package); want rg only")
	}
}

// TestRunInstall_OnlyDryRun asserts the dry-run plan reflects the --only set.
func TestRunInstall_OnlyDryRun(t *testing.T) {
	manifestPath := writeTestManifest(t, testIntentManifest) // rg, fd
	f := newInstallFake(t)

	var stdout, stderr bytes.Buffer
	code := runInstall([]string{"--manifest", manifestPath, "--dry-run", "--only", "rg"}, &stdout, &stderr, f)

	if code != 0 {
		t.Fatalf("runInstall(--dry-run --only rg) code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "packages=rg\n") {
		t.Errorf("stdout = %q, want packages=rg (only rg planned)", stdout.String())
	}
}

// TestRunInstall_SkipFilter asserts --skip removes a package from the selection.
func TestRunInstall_SkipFilter(t *testing.T) {
	manifestPath := writeTestManifest(t, testIntentManifest) // rg, fd
	f := newInstallFake(t)

	var stdout, stderr bytes.Buffer
	code := runInstall([]string{"--manifest", manifestPath, "--skip", "fd"}, &stdout, &stderr, f)

	if code != 0 {
		t.Fatalf("runInstall(--skip fd) code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if cargoInstallIndex(f.CallsSnapshot(), "rg") < 0 {
		t.Errorf("--skip fd did not install rg; calls=%+v", f.CallsSnapshot())
	}
	if cargoInstallIndex(f.CallsSnapshot(), "fd") >= 0 {
		t.Errorf("--skip fd still installed fd; want it excluded")
	}
}

// TestRunInstall_UnknownFilterName verifies a mistyped --only/--skip package is
// a usage error (exit 2) naming the unknown package and the valid set, on both
// the real and dry-run paths, and that no install/sudo command runs.
func TestRunInstall_UnknownFilterName(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"unknown only", []string{"--only", "nope"}},
		{"unknown skip", []string{"--skip", "nope"}},
		{"unknown only dry-run", []string{"--dry-run", "--only", "nope"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifestPath := writeTestManifest(t, testIntentManifest)
			f := newInstallFake(t)

			var stdout, stderr bytes.Buffer
			code := runInstall(append([]string{"--manifest", manifestPath}, tt.args...), &stdout, &stderr, f)

			if code != 2 {
				t.Fatalf("runInstall(%v) code = %d, want 2; stderr=%q", tt.args, code, stderr.String())
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

func TestRunInstall_FailureSummaryNonZeroExit(t *testing.T) {
	manifestPath := writeTestManifest(t, `[[package]]
name = "rg"
backend = "toolchain"
tool = "cargo"
`)
	f := newInstallFake(t)
	f.RunErrors[exec.CommandKey("cargo", "install", "rg")] = os.ErrPermission

	var stdout, stderr bytes.Buffer
	code := runInstall([]string{"--manifest", manifestPath}, &stdout, &stderr, f)

	if code != 1 {
		t.Errorf("runInstall() code = %d, want 1", code)
	}
	if !strings.Contains(stdout.String(), "result.toolchain=rg:failed") {
		t.Errorf("stdout = %q, want result.toolchain=rg:failed", stdout.String())
	}
	if !strings.Contains(stderr.String(), "install summary:") {
		t.Errorf("stderr = %q, want install summary", stderr.String())
	}
	if !strings.Contains(stderr.String(), "rg:") {
		t.Errorf("stderr = %q, want per-package failure line for rg", stderr.String())
	}
}

// TestInstallPackage_Applicability covers the applicability probe (issue #14):
// platform-inapplicable backends are skipped as "not-applicable" with no error
// and no commands run, applicable packages still install, and an unknown
// backend surfaces the probe error as a failure.
func TestInstallPackage_Applicability(t *testing.T) {
	m := parseManifestString(t, applicabilityTestManifest)
	byName := make(map[string]manifest.Package, len(m.Packages))
	for _, pkg := range m.Packages {
		byName[pkg.Name] = pkg
	}

	tests := []struct {
		name       string
		pkg        manifest.Package
		info       platform.Info
		wantStatus string
		wantErr    bool
		wantCalls  bool
	}{
		{"apt-repo not applicable on arch host", byName["docker-ce"], archInfo(), "not-applicable", false, false},
		{"aur not applicable on debian host", byName["lutris"], debianInfo(), "not-applicable", false, false},
		{"pkg applicable on arch host installs", byName["git"], archInfo(), "ok", false, true},
		{"unknown backend fails", manifest.Package{Name: "mystery", Backend: "bogus"}, archInfo(), "failed", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := exec.NewFake()
			engine := testDriftEngine(t, fake)

			res := performPackage(engine, backend.ActionInstall, tt.pkg, tt.info, false)

			if res.status != tt.wantStatus {
				t.Errorf("status = %q, want %q", res.status, tt.wantStatus)
			}
			if (res.err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr %t", res.err, tt.wantErr)
			}
			if tt.wantErr {
				var ube *backend.UnknownBackendError
				if !errors.As(res.err, &ube) {
					t.Errorf("err = %T, want *backend.UnknownBackendError", res.err)
				}
			}
			if gotCalls := len(fake.CallsSnapshot()) > 0; gotCalls != tt.wantCalls {
				t.Errorf("commands ran = %t (%v), want %t", gotCalls, fake.CallsSnapshot(), tt.wantCalls)
			}
		})
	}
}

// TestInstallPackage_AlreadyInstalledGate covers the idempotency gate (#40): an
// applicable, already-installed package is skipped as "already-installed" with
// no install command run, and --force overrides the gate to reinstall it.
func TestInstallPackage_AlreadyInstalledGate(t *testing.T) {
	m := parseManifestString(t, applicabilityTestManifest)
	byName := make(map[string]manifest.Package, len(m.Packages))
	for _, pkg := range m.Packages {
		byName[pkg.Name] = pkg
	}
	git := byName["git"]

	t.Run("skips already-installed", func(t *testing.T) {
		fake := archPacmanFake(t) // pacman -Q git reports git installed
		engine := testDriftEngine(t, fake)

		res := performPackage(engine, backend.ActionInstall, git, archInfo(), false)

		if res.status != "already-installed" {
			t.Errorf("status = %q, want already-installed", res.status)
		}
		if res.err != nil {
			t.Errorf("err = %v, want nil", res.err)
		}
		if n := countSudoCalls(fake, "pacman", "-S", "--noconfirm", "--needed", "git"); n != 0 {
			t.Errorf("install argv ran %d times, want 0 (package already installed)", n)
		}
	})

	t.Run("force reinstalls already-installed", func(t *testing.T) {
		fake := archPacmanFake(t)
		engine := testDriftEngine(t, fake)

		res := performPackage(engine, backend.ActionInstall, git, archInfo(), true)

		if res.status != "ok" {
			t.Errorf("status = %q, want ok (forced reinstall)", res.status)
		}
		if n := countSudoCalls(fake, "pacman", "-S", "--noconfirm", "--needed", "git"); n != 1 {
			t.Errorf("install argv ran %d times, want 1 (--force overrides the gate)", n)
		}
	})
}

// TestRunInstall_NotApplicableBackendExitsZero is the command-level issue #14
// regression test: on a pacman host, the apt-repo package is reported as
// not-applicable, never attempted, and the run exits 0.
func TestRunInstall_NotApplicableBackendExitsZero(t *testing.T) {
	manifestPath := writeManifestFile(t, cmdApplicabilityManifest)
	fake := archPacmanFake(t)

	var stdout, stderr bytes.Buffer
	code := runInstall([]string{"--manifest", manifestPath}, &stdout, &stderr, fake)

	if code != 0 {
		t.Fatalf("runInstall() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "result.apt-repo=docker-ce:not-applicable") {
		t.Errorf("stdout = %q, want result.apt-repo=docker-ce:not-applicable", stdout.String())
	}
	// archPacmanFake reports git installed via `pacman -Q git`, so the
	// idempotency gate (#40) skips it as already-installed rather than "ok".
	if !strings.Contains(stdout.String(), "result.pkg=git:already-installed") {
		t.Errorf("stdout = %q, want result.pkg=git:already-installed", stdout.String())
	}
	if !strings.Contains(stderr.String(), "not-applicable=1 failed=0") {
		t.Errorf("stderr = %q, want not-applicable=1 failed=0 in summary", stderr.String())
	}
	if strings.Contains(stderr.String(), "docker-ce") {
		t.Errorf("stderr = %q, want no docker-ce failure diagnostic", stderr.String())
	}

	for _, call := range fake.CallsSnapshot() {
		switch call.Name {
		case "apt-get", "curl", "gpg", "wget":
			t.Errorf("apt-repo command ran on non-apt host: %s %v", call.Name, call.Args)
		}
	}
}

// TestResolveAction covers the --update/--remove → Action mapping and the
// mutually-exclusive usage error.
func TestResolveAction(t *testing.T) {
	tests := []struct {
		name           string
		update, remove bool
		want           backend.Action
		wantErr        bool
	}{
		{"neither is install", false, false, backend.ActionInstall, false},
		{"update", true, false, backend.ActionUpdate, false},
		{"remove", false, true, backend.ActionRemove, false},
		{"both is a usage error", true, true, backend.ActionInstall, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveAction(tt.update, tt.remove)
			if (err != nil) != tt.wantErr {
				t.Fatalf("resolveAction(%v,%v) err = %v, wantErr = %v", tt.update, tt.remove, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("resolveAction(%v,%v) = %v, want %v", tt.update, tt.remove, got, tt.want)
			}
		})
	}
}

// TestRunInstall_NamedPositional proves a positional name folds into the --only
// set: `install rg` installs only rg from a multi-package manifest.
func TestRunInstall_NamedPositional(t *testing.T) {
	manifestPath := writeTestManifest(t, testIntentManifest) // rg, fd
	f := newInstallFake(t)

	var stdout, stderr bytes.Buffer
	code := runInstall([]string{"--manifest", manifestPath, "rg"}, &stdout, &stderr, f)

	if code != 0 {
		t.Fatalf("runInstall(rg) code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "result.toolchain=rg:ok") {
		t.Errorf("stdout = %q, want result.toolchain=rg:ok", stdout.String())
	}
	if cargoInstallIndex(f.CallsSnapshot(), "rg") < 0 {
		t.Errorf("positional rg did not install rg; calls=%+v", f.CallsSnapshot())
	}
	if cargoInstallIndex(f.CallsSnapshot(), "fd") >= 0 {
		t.Errorf("positional rg also installed fd; want rg only")
	}
}

// TestRunInstall_UnknownPositional proves an unknown positional name is a usage
// error (exit 2), matching --only.
func TestRunInstall_UnknownPositional(t *testing.T) {
	manifestPath := writeTestManifest(t, testIntentManifest)
	f := newInstallFake(t)

	var stdout, stderr bytes.Buffer
	code := runInstall([]string{"--manifest", manifestPath, "nope"}, &stdout, &stderr, f)

	if code != 2 {
		t.Fatalf("runInstall(nope) code = %d, want 2; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), `unknown package "nope"; manifest defines: fd, rg`) {
		t.Errorf("stderr = %q, want unknown-package diagnostic", stderr.String())
	}
}

// TestRunInstall_PositionalWithTrailingFlag exercises the loop-parse idiom: a
// value-flag-safe parse must collect the positional AND the trailing boolean flag
// (`install rg --update`), regardless of order.
func TestRunInstall_PositionalWithTrailingFlag(t *testing.T) {
	for _, args := range [][]string{
		{"--dry-run", "rg", "--update"}, // flag trails positional
		{"--dry-run", "--update", "rg"}, // flag precedes positional
		{"--update", "--dry-run", "rg"}, // interleaved
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			manifestPath := writeTestManifest(t, testIntentManifest)
			f := newInstallFake(t)

			var stdout, stderr bytes.Buffer
			code := runInstall(append([]string{"--manifest", manifestPath}, args...), &stdout, &stderr, f)

			if code != 0 {
				t.Fatalf("runInstall(%v) code = %d, want 0; stderr=%q", args, code, stderr.String())
			}
			out := stdout.String()
			if !strings.Contains(out, "action=update") {
				t.Errorf("stdout = %q, want action=update (loop-parse consumed --update)", out)
			}
			if !strings.Contains(out, "packages=rg") {
				t.Errorf("stdout = %q, want packages=rg (loop-parse consumed the positional)", out)
			}
		})
	}
}

// TestRunInstall_BothActionFlags proves --update and --remove together is a usage
// error (exit 2).
func TestRunInstall_BothActionFlags(t *testing.T) {
	manifestPath := writeTestManifest(t, testIntentManifest)
	f := newInstallFake(t)

	var stdout, stderr bytes.Buffer
	code := runInstall([]string{"--manifest", manifestPath, "rg", "--update", "--remove"}, &stdout, &stderr, f)

	if code != 2 {
		t.Fatalf("runInstall(--update --remove) code = %d, want 2; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "mutually exclusive") {
		t.Errorf("stderr = %q, want mutually-exclusive diagnostic", stderr.String())
	}
}

// TestRunInstall_UpdateRequiresTarget proves update/remove without a target (no
// name, --only, or --intent) is a usage error (exit 2) — lifecycle verbs never
// operate on the implicit whole-manifest set.
func TestRunInstall_UpdateRequiresTarget(t *testing.T) {
	for _, flag := range []string{"--update", "--remove"} {
		t.Run(flag, func(t *testing.T) {
			f := newInstallFake(t)
			var stdout, stderr bytes.Buffer
			// No --manifest needed: the target gate fires before manifest resolve.
			code := runInstall([]string{flag}, &stdout, &stderr, f)

			if code != 2 {
				t.Fatalf("runInstall(%s) code = %d, want 2; stderr=%q", flag, code, stderr.String())
			}
			if !strings.Contains(stderr.String(), "requires a package name") {
				t.Errorf("stderr = %q, want require-a-target diagnostic", stderr.String())
			}
			if calls := f.CallsSnapshot(); len(calls) != 0 {
				t.Errorf("ran commands %+v after a usage error, want none", calls)
			}
		})
	}
}

// TestRunInstall_UpdateNamed proves the end-to-end update path: `install rg
// --update` on an installed toolchain package runs `cargo install --force rg` and
// reports status "updated".
func TestRunInstall_UpdateNamed(t *testing.T) {
	manifestPath := writeTestManifest(t, testIntentManifest) // rg, fd
	f := newInstallFake(t)
	f.AddPath("rg", "/home/user/.cargo/bin/rg") // installed → update proceeds

	var stdout, stderr bytes.Buffer
	code := runInstall([]string{"--manifest", manifestPath, "rg", "--update"}, &stdout, &stderr, f)

	if code != 0 {
		t.Fatalf("runInstall(rg --update) code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "result.toolchain=rg:updated") {
		t.Errorf("stdout = %q, want result.toolchain=rg:updated", stdout.String())
	}
	found := false
	for _, c := range f.CallsSnapshot() {
		if c.Name == "cargo" && len(c.Args) == 3 && c.Args[0] == "install" && c.Args[1] == "--force" && c.Args[2] == "rg" {
			found = true
		}
	}
	if !found {
		t.Errorf("cargo install --force rg not run; calls=%+v", f.CallsSnapshot())
	}
}

// TestPerformPackage_RemoveSkipsWhenAbsent proves Remove is a no-op
// ("already-absent") when the package is not installed — Engine.Remove is never
// called, so a non-idempotent uninstall is never triggered.
func TestPerformPackage_RemoveSkipsWhenAbsent(t *testing.T) {
	m := parseManifestString(t, applicabilityTestManifest)
	byName := make(map[string]manifest.Package, len(m.Packages))
	for _, pkg := range m.Packages {
		byName[pkg.Name] = pkg
	}
	git := byName["git"]

	fake := exec.NewFake() // pacman -Q git NOT registered → probes as absent
	engine := testDriftEngine(t, fake)

	// force must NOT bypass the remove guard.
	res := performPackage(engine, backend.ActionRemove, git, archInfo(), true)

	if res.status != "already-absent" {
		t.Errorf("status = %q, want already-absent", res.status)
	}
	if res.err != nil {
		t.Errorf("err = %v, want nil", res.err)
	}
	if n := countSudoCalls(fake, "pacman", "-Rns", "--noconfirm", "git"); n != 0 {
		t.Errorf("remove argv ran %d times, want 0 (package absent, guard unbypassable)", n)
	}
}

// TestPerformPackage_UpdateSkipsWhenAbsent proves Update is a no-op
// ("not-installed") when the package is not present.
func TestPerformPackage_UpdateSkipsWhenAbsent(t *testing.T) {
	m := parseManifestString(t, applicabilityTestManifest)
	byName := make(map[string]manifest.Package, len(m.Packages))
	for _, pkg := range m.Packages {
		byName[pkg.Name] = pkg
	}
	git := byName["git"]

	fake := exec.NewFake() // absent
	engine := testDriftEngine(t, fake)

	res := performPackage(engine, backend.ActionUpdate, git, archInfo(), true)

	if res.status != "not-installed" {
		t.Errorf("status = %q, want not-installed", res.status)
	}
	if n := countSudoCalls(fake, "pacman", "-S", "--noconfirm", "--needed", "git"); n != 0 {
		t.Errorf("update argv ran %d times, want 0 (package absent)", n)
	}
}

// TestPerformPackage_UnsupportedIsFailed proves an ActionUnsupportedError from
// the engine surfaces as status "failed" (exit 1), not a usage error.
func TestPerformPackage_UnsupportedIsFailed(t *testing.T) {
	fake := exec.NewFake()
	fake.AddPath("starship", "/usr/local/bin/starship") // installed → Remove attempted
	engine := testDriftEngine(t, fake)

	// script-url does not implement Remover → fail-fast.
	pkg := manifest.Package{Name: "starship", Backend: manifest.BackendScriptURL, URL: "https://example.com/i.sh"}

	res := performPackage(engine, backend.ActionRemove, pkg, archInfo(), false)

	if res.status != "failed" {
		t.Errorf("status = %q, want failed", res.status)
	}
	var ue *backend.ActionUnsupportedError
	if !errors.As(res.err, &ue) {
		t.Fatalf("err = %v, want *backend.ActionUnsupportedError", res.err)
	}
	if code := installExitCode([]installResult{res}); code != 1 {
		t.Errorf("installExitCode = %d, want 1", code)
	}
}

func TestRunInstall_BadFlag(t *testing.T) {
	f := exec.NewFake()
	var stdout, stderr bytes.Buffer
	code := runInstall([]string{"--bogus"}, &stdout, &stderr, f)

	if code != 2 {
		t.Errorf("runInstall([--bogus]) code = %d, want 2", code)
	}
}

func TestRunInstall_ManifestResolutionError(t *testing.T) {
	f := exec.NewFake()
	var stdout, stderr bytes.Buffer
	code := runInstall([]string{"--manifest", "/nonexistent/does-not-exist.toml"}, &stdout, &stderr, f)

	if code != 1 {
		t.Errorf("runInstall() code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	if !strings.HasPrefix(stderr.String(), "dotf: ") {
		t.Errorf("stderr = %q, want dotf: prefix", stderr.String())
	}
}

func TestPackagesForInstall_AllPackages(t *testing.T) {
	m := testPlanManifest()
	got := packagesForInstall(m, nil)
	if len(got) != len(m.Packages) {
		t.Fatalf("len(packagesForInstall) = %d, want %d", len(got), len(m.Packages))
	}
}

func TestPackagesByName(t *testing.T) {
	m := testPlanManifest() // packages in manifest order: git, starship, eza, rust

	// A subset is returned in manifest order, deduped, ignoring unknown names.
	got := packagesByName(m, []string{"rust", "git", "git", "nonexistent"})
	if len(got) != 2 || got[0].Name != "git" || got[1].Name != "rust" {
		t.Errorf("packagesByName = %+v, want [git rust] in manifest order", got)
	}

	// An empty selection installs nothing — unlike packagesForInstall, whose
	// empty argument means "every package". This is the guided-mode contract:
	// deselecting everything in the TUI must not install the whole manifest.
	if got := packagesByName(m, nil); len(got) != 0 {
		t.Errorf("packagesByName(nil) = %+v, want empty", got)
	}
}

func TestResolveRepoRootFromCWD(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	root := findRepoRootFromCWD()
	manifestPath := filepath.Join(root, "manifest", "packages.toml")
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("repo root %q missing manifest/packages.toml: %v", root, err)
	}
	if !strings.HasPrefix(wd, root) && wd != root {
		t.Errorf("findRepoRootFromCWD() = %q, want ancestor of cwd %q", root, wd)
	}
}

func TestResolveRepoRootEnv(t *testing.T) {
	t.Setenv("DOTFILES_ROOT", "/custom/root")
	if got := resolveRepoRoot(); got != "/custom/root" {
		t.Errorf("resolveRepoRoot() = %q, want /custom/root", got)
	}
}

// TestFilterPackages_GitHubMCPServer: within the opt-in ai intent, --only
// narrows to exactly github-mcp-server (no siblings, no prerequisites).
func TestFilterPackages_GitHubMCPServer(t *testing.T) {
	m, err := manifest.Resolve("", "", dotfiles.ManifestFS, manifest.ResolveOptions{})
	if err != nil {
		t.Fatalf("resolve embedded manifest: %v", err)
	}

	pkgs := packagesForInstall(m, []string{"ai"})
	if !slices.ContainsFunc(pkgs, func(p manifest.Package) bool { return p.Name == "github-mcp-server" }) {
		t.Fatal("--intent ai does not include github-mcp-server")
	}

	got, err := filterPackages(m, pkgs, []string{"github-mcp-server"}, nil)
	if err != nil {
		t.Fatalf("filterPackages(--intent ai --only github-mcp-server) error = %v", err)
	}
	if len(got) != 1 || got[0].Name != "github-mcp-server" || got[0].Backend != manifest.BackendGitHubRelease {
		t.Errorf("--only github-mcp-server = %+v, want exactly the github-release package", got)
	}
}

// TestFilterPackages_DesktopApps: the embedded manifest makes espanso and
// claude-desktop selectable via --only; claude-desktop pulls in its macOS cask
// sibling (and --skip drops both), while the claude CLI stays distinct.
func TestFilterPackages_DesktopApps(t *testing.T) {
	m, err := manifest.Resolve("", "", dotfiles.ManifestFS, manifest.ResolveOptions{})
	if err != nil {
		t.Fatalf("resolve embedded manifest: %v", err)
	}
	names := func(pkgs []manifest.Package) string {
		out := make([]string, 0, len(pkgs))
		for _, p := range pkgs {
			out = append(out, p.Name)
		}
		return strings.Join(out, ",")
	}

	got, err := filterPackages(m, nil, []string{"claude-desktop", "espanso"}, nil)
	if err != nil {
		t.Fatalf("filterPackages(--only claude-desktop espanso) error = %v", err)
	}
	if s := names(got); s != "espanso,claude-desktop,claude-desktop-macos,claude-desktop-fedora" && s != "claude-desktop,claude-desktop-macos,claude-desktop-fedora,espanso" {
		t.Errorf("--only claude-desktop espanso = %q, want both claude-desktop siblings and espanso", s)
	}
	for _, p := range got {
		if !p.RequiresDesktop {
			t.Errorf("%s: RequiresDesktop = false, want true", p.Name)
		}
	}

	got, _ = filterPackages(m, nil, []string{"herdr"}, nil)
	if s := names(got); s != "herdr,herdr-macos" {
		t.Errorf("--only herdr = %q, want the existing herdr/herdr-macos pair", s)
	}

	got, _ = filterPackages(m, nil, []string{"claude"}, nil)
	if s := names(got); s != "claude" {
		t.Errorf("--only claude = %q, want just the CLI", s)
	}

	got, _ = filterPackages(m, m.Packages, nil, []string{"claude-desktop"})
	for _, p := range got {
		if strings.HasPrefix(p.Name, "claude-desktop") {
			t.Errorf("--skip claude-desktop kept %s", p.Name)
		}
	}

	desktop := m.Intents["desktop"]
	if !optInIntents["desktop"] || desktop.Default {
		t.Errorf("desktop intent = %+v, opt-in %t; want opt-in, non-default", desktop, optInIntents["desktop"])
	}
	for _, name := range []string{"claude-desktop", "espanso"} {
		if !slices.Contains(desktop.Packages, name) {
			t.Errorf("desktop intent missing %q", name)
		}
	}

	// Each logical desktop app is selected by its bare --only name and pulls in
	// exactly its OS-suffix siblings (declared in the manifest, in file order).
	apps := map[string]string{
		"wezterm":          "wezterm,wezterm-linux,wezterm-macos",
		"ghostty":          "ghostty,ghostty-macos,ghostty-fedora",
		"1password":        "1password-linux,1password,1password-macos",
		"cursor":           "cursor,cursor-arch,cursor-macos",
		"chatgpt":          "chatgpt,chatgpt-arch,chatgpt-macos",
		"obs-studio":       "obs-studio,obs-studio-macos",
		"gimp":             "gimp,gimp-macos",
		"darktable":        "darktable,darktable-macos",
		"kdenlive":         "kdenlive,kdenlive-macos",
		"meld":             "meld,meld-macos",
		"pinta":            "pinta,pinta-macos",
		"vlc":              "vlc,vlc-macos",
		"qalculate":        "qalculate,qalculate-macos",
		"mediawriter":      "mediawriter,mediawriter-macos",
		"libreoffice":      "libreoffice,libreoffice-macos",
		"virt-manager":     "virt-manager,virt-runtime,libvirt-runtime,libvirt-network",
		"whatsapp-macos":   "whatsapp-macos",
		"grok-bot":         "grok-bot,grok-bot-arch,grok-bot-macos",
		"zed-preview":      "zed-preview,zed-preview-arm64,zed-preview-macos",
		"lm-studio":        "lm-studio,lm-studio-arm64,lm-studio-macos,appimage-runtime",
		"obsidian":         "obsidian,obsidian-arm64,obsidian-macos,appimage-runtime,appimage-libz",
		"code-insiders":    "code-insiders,code-insiders-arch,code-insiders-macos",
		"google-chrome":    "google-chrome,google-chrome-arch,google-chrome-macos",
		"discord":          "discord,discord-macos",
		"slack":            "slack,slack-arch,slack-macos",
		"zoom":             "zoom,zoom-arch,zoom-macos",
		"dropbox":          "dropbox,dropbox-arch,dropbox-macos",
		"teams-for-linux":  "teams-for-linux,teams-for-linux-arch,teams-for-linux-arm64,teams-for-linux-macos",
		"beekeeper-studio": "beekeeper-studio,beekeeper-studio-arch,beekeeper-studio-arm64,beekeeper-studio-macos",
		"localsend":        "localsend,localsend-macos,appimage-runtime",
		"t3-code-client":   "t3-code-client,t3-code-client-arm64,t3-code-client-debian,t3-code-client-macos,appimage-runtime",
		"winboat":          "winboat,winboat-arch",
	}
	for app, want := range apps {
		got, err := filterPackages(m, nil, []string{app}, nil)
		if err != nil {
			t.Errorf("--only %s error = %v", app, err)
			continue
		}
		gotNames := strings.Split(names(got), ",")
		wantNames := strings.Split(want, ",")
		slices.Sort(gotNames)
		slices.Sort(wantNames)
		if !slices.Equal(gotNames, wantNames) {
			t.Errorf("--only %s = %v, want %v", app, gotNames, wantNames)
		}
		if !slices.Contains(desktop.Packages, app) && app != "1password" {
			t.Errorf("desktop intent missing %q", app)
		}
	}
	for _, name := range []string{"code-insiders", "code-insiders-macos"} {
		if !slices.Contains(desktop.Packages, name) {
			t.Errorf("desktop intent missing %q", name)
		}
	}
}

const testSiblingManifest = `
[[package]]
name = "tool"
backend = "toolchain"
tool = "cargo"

[[package]]
name = "tool-arch"
backend = "toolchain"
tool = "cargo"

[[package]]
name = "tool-debian"
backend = "toolchain"
tool = "cargo"

[[package]]
name = "tool-linux"
backend = "toolchain"
tool = "cargo"

[[package]]
name = "tool-macos"
backend = "toolchain"
tool = "cargo"

[[package]]
name = "tool-fedora"
backend = "toolchain"
tool = "cargo"

[[package]]
name = "tool-arm64"
backend = "toolchain"
tool = "cargo"

[[package]]
name = "tool-desktop"
backend = "toolchain"
tool = "cargo"

[[package]]
name = "other"
backend = "toolchain"
tool = "cargo"
`

// TestFilterPackages_PlatformSiblings: a bare name selects/skips its exact
// -arch/-debian/-linux/-macos/-fedora/-arm64 siblings (never a mere prefix match such
// as tool-desktop), an explicit sibling stays exact, and output is manifest-ordered.
func TestFilterPackages_PlatformSiblings(t *testing.T) {
	m := parseManifestString(t, testSiblingManifest)
	const family = "tool,tool-arch,tool-debian,tool-linux,tool-macos,tool-fedora,tool-arm64"
	join := func(pkgs []manifest.Package) string {
		out := make([]string, 0, len(pkgs))
		for _, p := range pkgs {
			out = append(out, p.Name)
		}
		return strings.Join(out, ",")
	}

	tests := []struct {
		name       string
		only, skip []string
		want       string
	}{
		{"only expands all suffixes", []string{"tool"}, nil, family},
		{"only dedupes explicit sibling", []string{"tool", "tool-linux", "tool"}, nil, family},
		{"only explicit sibling is exact", []string{"tool-arch"}, nil, "tool-arch"},
		{"only explicit -linux is exact", []string{"tool-linux"}, nil, "tool-linux"},
		{"unrelated prefix not selected", []string{"tool"}, nil, family}, // tool-desktop absent
		{"skip expands all suffixes", nil, []string{"tool"}, "tool-desktop,other"},
		{"skip explicit sibling is exact", nil, []string{"tool-macos"}, "tool,tool-arch,tool-debian,tool-linux,tool-fedora,tool-arm64,tool-desktop,other"},
		{"only and skip compose", []string{"tool", "other"}, []string{"tool"}, "other"},
		{"only family skip one sibling", []string{"tool"}, []string{"tool-debian"}, "tool,tool-arch,tool-linux,tool-macos,tool-fedora,tool-arm64"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := filterPackages(m, m.Packages, tt.only, tt.skip)
			if err != nil {
				t.Fatalf("filterPackages() error = %v", err)
			}
			if s := join(got); s != tt.want {
				t.Errorf("filterPackages(only=%v skip=%v) = %q, want %q", tt.only, tt.skip, s, tt.want)
			}
		})
	}

	// Validation precedes expansion: an unknown name errors even beside a valid one.
	if _, err := filterPackages(m, m.Packages, []string{"tool", "tool-nope"}, nil); err == nil {
		t.Error("filterPackages(unknown name) error = nil, want unknown package")
	}
}

func TestFilterPackages_SkipDropsOrphanedPrerequisite(t *testing.T) {
	m := &manifest.Manifest{Packages: []manifest.Package{
		{Name: "runtime"},
		{Name: "app", Requires: []string{"runtime"}},
		{Name: "other-app", Requires: []string{"runtime"}},
		{Name: "plain"},
	}}
	byName := func(names ...string) []manifest.Package { return packagesByName(m, names) }

	tests := []struct {
		name string
		pkgs []manifest.Package
		only []string
		skip []string
		want string
	}{
		{"only and skip the same package selects nothing", nil, []string{"app"}, []string{"app"}, ""},
		{"prerequisite of a skipped intent package is dropped", byName("app", "plain"), nil, []string{"app"}, "plain"},
		{"prerequisite shared with a kept package survives", nil, []string{"app", "other-app"}, []string{"app"}, "runtime,other-app"},
		{"prerequisite selected in its own right survives", byName("runtime", "app"), nil, []string{"app"}, "runtime"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := filterPackages(m, tt.pkgs, tt.only, tt.skip)
			if err != nil {
				t.Fatalf("filterPackages() error = %v", err)
			}
			names := make([]string, len(got))
			for i, pkg := range got {
				names[i] = pkg.Name
			}
			if s := strings.Join(names, ","); s != tt.want {
				t.Errorf("filterPackages(only=%v skip=%v) = %q, want %q", tt.only, tt.skip, s, tt.want)
			}
		})
	}
}

// TestFilterPackagesForHost_RequiresOnlyFromApplicable covers YAN-1070: a
// platform sibling the host cannot run (an arm64-gated entry on x86_64, a
// desktop-gated entry on a headless host) must not pull in its prerequisite,
// nor fail closed when that prerequisite is skipped. On a host where the
// sibling applies, the prerequisite is still ordered first and skipping it
// still fails closed. The host-independent filterPackages (dotf lock) keeps
// expanding every prerequisite.
func TestFilterPackagesForHost_RequiresOnlyFromApplicable(t *testing.T) {
	m := &manifest.Manifest{Packages: []manifest.Package{
		{Name: "runtime"},
		{Name: "gui-runtime"},
		{Name: "app"},
		{Name: "app-arm64", Arch: []string{"arm64"}, Requires: []string{"runtime"}},
		{Name: "app-macos", OS: "macos", Requires: []string{"runtime"}},
		{Name: "app-linux", OS: "linux", RequiresDesktop: true, Requires: []string{"gui-runtime"}},
	}}
	amd64 := platform.Info{OS: platform.OSDebian, Arch: platform.ArchX86_64}
	arm64 := platform.Info{OS: platform.OSDebian, Arch: platform.ArchARM64}
	desktop := platform.Info{OS: platform.OSDebian, Arch: platform.ArchX86_64, Desktop: true}

	tests := []struct {
		name    string
		info    platform.Info
		only    []string
		skip    []string
		want    string
		wantErr string
	}{
		{"non-applicable siblings pull in no prerequisite", amd64, []string{"app"}, nil, "app,app-arm64,app-macos,app-linux", ""},
		{"skipping a non-applicable sibling's prerequisite succeeds", amd64, []string{"app"}, []string{"runtime"}, "app,app-arm64,app-macos,app-linux", ""},
		{"applicable sibling orders its prerequisite first", arm64, []string{"app"}, nil, "runtime,app,app-arm64,app-macos,app-linux", ""},
		{"skipping an applicable sibling's prerequisite fails closed", arm64, []string{"app"}, []string{"runtime"}, "", `required package "runtime" excluded or missing`},
		{"headless host: desktop sibling pulls in no prerequisite", amd64, []string{"app"}, []string{"gui-runtime"}, "app,app-arm64,app-macos,app-linux", ""},
		{"desktop host: desktop sibling orders its prerequisite first", desktop, []string{"app"}, nil, "gui-runtime,app,app-arm64,app-macos,app-linux", ""},
		{"desktop host: skipping the desktop sibling's prerequisite fails closed", desktop, []string{"app"}, []string{"gui-runtime"}, "", `required package "gui-runtime" excluded or missing`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := filterPackagesForHost(m, nil, tt.only, tt.skip, tt.info)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("filterPackagesForHost() err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("filterPackagesForHost() err = %v, want nil", err)
			}
			names := make([]string, len(got))
			for i, pkg := range got {
				names[i] = pkg.Name
			}
			if s := strings.Join(names, ","); s != tt.want {
				t.Errorf("filterPackagesForHost(only=%v skip=%v) = %q, want %q", tt.only, tt.skip, s, tt.want)
			}
		})
	}

	// The host-independent lock path still follows every prerequisite.
	got, err := filterPackages(m, nil, []string{"app"}, nil)
	if err != nil {
		t.Fatalf("filterPackages() err = %v", err)
	}
	names := make([]string, len(got))
	for i, pkg := range got {
		names[i] = pkg.Name
	}
	if s, want := strings.Join(names, ","), "runtime,gui-runtime,app,app-arm64,app-macos,app-linux"; s != want {
		t.Errorf("filterPackages(--only app) = %q, want %q", s, want)
	}
}

// TestFilterPackagesForHost_T3CodeClientPrerequisites: on Debian-family hosts
// (apt, or Linuxbrew primary) both AppImage entries are exclude_os-gated away,
// so --only t3-code-client pulls no appimage-runtime and skipping it does not
// fail closed. Everywhere else (Fedora dnf, Fedora Linuxbrew) the AppImage
// route applies and the prerequisite is ordered first and fails closed on skip.
func TestFilterPackagesForHost_T3CodeClientPrerequisites(t *testing.T) {
	m, err := manifest.Resolve("", "", dotfiles.ManifestFS, manifest.ResolveOptions{})
	if err != nil {
		t.Fatalf("resolve embedded manifest: %v", err)
	}
	debianAPT := platform.Info{OS: platform.OSDebian, Arch: platform.ArchX86_64, PkgMgr: platform.PkgMgrAPT, Desktop: true}
	debianBrew := platform.Info{OS: platform.OSDebian, Arch: platform.ArchX86_64, PkgMgr: platform.PkgMgrHomebrew, Desktop: true}
	fedoraDNF := platform.Info{OS: platform.OSRedHat, Arch: platform.ArchX86_64, PkgMgr: platform.PkgMgrDNF, Desktop: true}
	fedoraBrew := platform.Info{OS: platform.OSRedHat, Arch: platform.ArchX86_64, PkgMgr: platform.PkgMgrHomebrew, Desktop: true}

	tests := []struct {
		name    string
		info    platform.Info
		skip    []string
		wantRT  bool
		wantErr string
	}{
		{"debian apt pulls no runtime", debianAPT, nil, false, ""},
		{"debian apt skip runtime succeeds", debianAPT, []string{"appimage-runtime"}, false, ""},
		{"debian linuxbrew pulls no runtime", debianBrew, nil, false, ""},
		{"debian linuxbrew skip runtime succeeds", debianBrew, []string{"appimage-runtime"}, false, ""},
		{"fedora dnf pulls runtime", fedoraDNF, nil, true, ""},
		{"fedora dnf skip runtime fails closed", fedoraDNF, []string{"appimage-runtime"}, false, `required package "appimage-runtime" excluded or missing`},
		{"fedora linuxbrew pulls runtime", fedoraBrew, nil, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := filterPackagesForHost(m, nil, []string{"t3-code-client"}, tt.skip, tt.info)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("filterPackagesForHost() err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("filterPackagesForHost() err = %v, want nil", err)
			}
			names := make([]string, len(got))
			hasRT := false
			for i, pkg := range got {
				names[i] = pkg.Name
				hasRT = hasRT || pkg.Name == "appimage-runtime"
			}
			if hasRT != tt.wantRT {
				t.Errorf("appimage-runtime selected = %t, want %t (got %v)", hasRT, tt.wantRT, names)
			}
			if tt.wantRT && names[0] != "appimage-runtime" {
				t.Errorf("appimage-runtime not ordered first: %v", names)
			}
		})
	}
}

// testUnreachableSiblingManifest has a tool whose only prerequisite-bearing
// entry is a platform sibling (the "-macos" suffix makes --only tool select it)
// gated to os = "windows", which no test host matches.
const testUnreachableSiblingManifest = `
[[package]]
name = "tool-runtime"
backend = "custom-script"
script = "bin/install-tool-runtime.sh"
class = "rolling"
justification = "test prerequisite"

[[package]]
name = "tool"
backend = "custom-script"
script = "bin/install-tool.sh"
class = "rolling"
justification = "test tool"

[[package]]
name = "tool-macos"
backend = "custom-script"
os = "windows"
requires = ["tool-runtime"]
script = "bin/install-tool-win.sh"
class = "rolling"
justification = "test sibling gated off every test host"
`

// TestRunInstall_DryRunSkipsPrerequisiteOfNonApplicableSibling wires the
// YAN-1070 rule through runInstall: --skip of a prerequisite needed only by a
// sibling the host can never run succeeds and leaves it out of the plan.
func TestRunInstall_DryRunSkipsPrerequisiteOfNonApplicableSibling(t *testing.T) {
	manifestPath := writeTestManifest(t, testUnreachableSiblingManifest)

	var stdout, stderr bytes.Buffer
	code := runInstall([]string{"--manifest", manifestPath, "--dry-run", "--only", "tool", "--skip", "tool-runtime"}, &stdout, &stderr, exec.NewFake())

	if code != 0 {
		t.Fatalf("runInstall() code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if out := stdout.String(); !strings.Contains(out, "packages=tool,tool-macos\n") || strings.Contains(out, "tool-runtime") {
		t.Errorf("stdout = %q, want packages=tool,tool-macos without tool-runtime", out)
	}
}

// TestRunApply_DryRunSkipsPrerequisiteOfNonApplicableSibling is the apply
// --dry-run counterpart: applyDryRun filters with the host-aware rule too.
func TestRunApply_DryRunSkipsPrerequisiteOfNonApplicableSibling(t *testing.T) {
	t.Setenv("DOTFILES_ROOT", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	manifestPath := writeTestManifest(t, testUnreachableSiblingManifest)

	var stdout, stderr bytes.Buffer
	code := runApply([]string{"--manifest", manifestPath, "--dry-run", "--headless", "--only", "tool", "--skip", "tool-runtime"}, &stdout, &stderr, newApplyFake(t))

	if code != 0 {
		t.Fatalf("runApply() code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if out := stdout.String(); !strings.Contains(out, "packages=tool\n") || strings.Contains(out, "tool-runtime") {
		t.Errorf("stdout = %q, want packages=tool without tool-runtime", out)
	}
}

func TestFilterPackages_PrerequisiteOrderAndSkip(t *testing.T) {
	m, err := manifest.LoadFS(dotfiles.ManifestFS, "manifest/packages.toml")
	if err != nil {
		t.Fatal(err)
	}
	got, err := filterPackages(m, nil, []string{"obsidian"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Name != "appimage-runtime" {
		t.Fatalf("--only obsidian first = %s, want prerequisite first", got[0].Name)
	}
	if _, err := filterPackages(m, nil, []string{"obsidian"}, []string{"appimage-runtime"}); err == nil {
		t.Fatal("--skip accepted removing obsidian prerequisite")
	}
}
