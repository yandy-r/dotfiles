package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

	// script-url implements neither Updater nor Remover → fail-fast.
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
