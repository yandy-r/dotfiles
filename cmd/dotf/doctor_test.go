package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/plasma"
	"github.com/yandy-r/dotfiles/internal/platform"
	"github.com/yandy-r/dotfiles/internal/symlink"
)

// findCheck returns the check with the given category/name, or (zero, false).
func findCheck(checks []doctorCheck, category, name string) (doctorCheck, bool) {
	for _, c := range checks {
		if c.Category == category && c.Name == name {
			return c, true
		}
	}
	return doctorCheck{}, false
}

// macInfo is a macOS/Homebrew platform fixture for the OS-specific builders.
func macInfo() platform.Info {
	return platform.Info{
		OS:        platform.OSMacOS,
		Arch:      platform.ArchARM64,
		PkgMgr:    platform.PkgMgrHomebrew,
		Container: platform.ContainerNone,
		Distro:    "macos",
	}
}

// parsePkgManifest parses an inline TOML manifest, failing the test on error.
func parsePkgManifest(t *testing.T, toml string) *manifest.Manifest {
	t.Helper()
	m, err := manifest.Parse(strings.NewReader(toml))
	if err != nil {
		t.Fatalf("manifest.Parse() error = %v", err)
	}
	return m
}

const singlePkgManifest = `[[package]]
name = "git"
backend = "pkg"
names = { pacman = "git", apt = "git", dnf = "git", yum = "git", zypper = "git", homebrew = "git" }
`

const twoPkgManifest = singlePkgManifest + `
[[package]]
name = "zoxide"
backend = "pkg"
names = { pacman = "zoxide", apt = "zoxide", dnf = "zoxide", zypper = "zoxide", homebrew = "zoxide" }
`

// ----------------------------------------------------------------------------
// Builder tests (host-independent: platform.Info is injected directly).
// ----------------------------------------------------------------------------

func TestCheckPlatform(t *testing.T) {
	cases := []struct {
		name string
		info platform.Info
		want checkStatus
	}{
		{"unknown-os", platform.Info{OS: platform.OSUnknown}, checkFail},
		{"arch-ok", archInfo(), checkOK},
		{"linux-no-pkgmgr", platform.Info{OS: platform.OSLinux, PkgMgr: platform.PkgMgrUnknown}, checkWarn},
		{"macos-ok", macInfo(), checkOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := checkPlatform(tc.info)
			if c.Category != "platform" || c.Name != "detect" {
				t.Fatalf("checkPlatform identity = %s.%s, want platform.detect", c.Category, c.Name)
			}
			if c.Status != tc.want {
				t.Errorf("checkPlatform(%+v).Status = %q, want %q", tc.info, c.Status, tc.want)
			}
			if tc.want != checkOK && c.Remedy == "" {
				t.Errorf("checkPlatform(%s) non-ok check has empty remedy", tc.name)
			}
		})
	}
}

func TestCheckSysconfig(t *testing.T) {
	cases := []struct {
		os   platform.OS
		want checkStatus
	}{
		{platform.OSArch, checkOK},
		{platform.OSMacOS, checkOK},
		{platform.OSWindows, checkWarn},
		{platform.OSUnknown, checkFail},
	}
	for _, tc := range cases {
		t.Run(string(tc.os), func(t *testing.T) {
			c := checkSysconfig(platform.Info{OS: tc.os})
			if c.Category != "sysconfig" || c.Name != "support" {
				t.Fatalf("checkSysconfig identity = %s.%s, want sysconfig.support", c.Category, c.Name)
			}
			if c.Status != tc.want {
				t.Errorf("checkSysconfig(%q).Status = %q, want %q", tc.os, c.Status, tc.want)
			}
		})
	}
}

func TestCheckPrerequisites_AllPresent(t *testing.T) {
	fake := exec.NewFake()
	for _, bin := range []string{"git", "curl", "pacman", "sudo"} {
		fake.AddPath(bin, "/usr/bin/"+bin)
	}

	checks := checkPrerequisites(fake, archInfo())

	for _, name := range []string{"git", "download", "pkg-manager"} {
		c, ok := findCheck(checks, "prerequisites", name)
		if !ok {
			t.Fatalf("prerequisites.%s not emitted", name)
		}
		if c.Status != checkOK {
			t.Errorf("prerequisites.%s = %q, want ok", name, c.Status)
		}
	}
	// sudo is only probed when not already root.
	if os.Geteuid() != 0 {
		if c, ok := findCheck(checks, "prerequisites", "sudo"); !ok || c.Status != checkOK {
			t.Errorf("prerequisites.sudo = %v/%q, want ok", ok, c.Status)
		}
	}
	// homebrew is macOS-only and must not appear on a Linux fixture.
	if _, ok := findCheck(checks, "prerequisites", "homebrew"); ok {
		t.Errorf("prerequisites.homebrew emitted on a Linux host, want none")
	}
}

func TestCheckPrerequisites_MissingGitFails(t *testing.T) {
	fake := exec.NewFake()
	fake.AddPath("curl", "/usr/bin/curl")
	fake.AddPath("pacman", "/usr/bin/pacman")

	checks := checkPrerequisites(fake, archInfo())

	git, ok := findCheck(checks, "prerequisites", "git")
	if !ok {
		t.Fatal("prerequisites.git not emitted")
	}
	if git.Status != checkFail {
		t.Errorf("prerequisites.git = %q, want fail", git.Status)
	}
	if !strings.Contains(git.Remedy, "git") {
		t.Errorf("prerequisites.git remedy = %q, want it to name git", git.Remedy)
	}
}

func TestCheckPrerequisites_MissingDownloadWarns(t *testing.T) {
	fake := exec.NewFake()
	fake.AddPath("git", "/usr/bin/git")
	fake.AddPath("pacman", "/usr/bin/pacman")

	checks := checkPrerequisites(fake, archInfo())

	dl, ok := findCheck(checks, "prerequisites", "download")
	if !ok {
		t.Fatal("prerequisites.download not emitted")
	}
	if dl.Status != checkWarn {
		t.Errorf("prerequisites.download = %q, want warn", dl.Status)
	}
}

func TestCheckPrerequisites_MacOSHomebrew(t *testing.T) {
	fake := exec.NewFake()
	fake.AddPath("git", "/usr/bin/git")
	fake.AddPath("curl", "/usr/bin/curl")
	// brew intentionally absent.

	checks := checkPrerequisites(fake, macInfo())

	brew, ok := findCheck(checks, "prerequisites", "homebrew")
	if !ok {
		t.Fatal("prerequisites.homebrew not emitted on macOS")
	}
	if brew.Status != checkWarn {
		t.Errorf("prerequisites.homebrew = %q, want warn", brew.Status)
	}
	pm, ok := findCheck(checks, "prerequisites", "pkg-manager")
	if !ok {
		t.Fatal("prerequisites.pkg-manager not emitted")
	}
	if pm.Status != checkFail || !strings.Contains(pm.Detail, "brew") {
		t.Errorf("pkg-manager on macOS = %q detail=%q, want fail naming brew", pm.Status, pm.Detail)
	}
}

func TestCheckPrerequisites_UnknownPkgMgrSkipsRow(t *testing.T) {
	fake := exec.NewFake()
	info := platform.Info{OS: platform.OSLinux, PkgMgr: platform.PkgMgrUnknown}
	checks := checkPrerequisites(fake, info)
	if _, ok := findCheck(checks, "prerequisites", "pkg-manager"); ok {
		t.Errorf("pkg-manager row emitted with an unknown package manager, want skipped")
	}
}

func TestCheckPackages(t *testing.T) {
	t.Run("all-installed-ok", func(t *testing.T) {
		fake := archPacmanFake(t) // registers `pacman -Q git`
		engine := testDriftEngine(t, fake)
		c := checkPackages(parsePkgManifest(t, singlePkgManifest), engine, archInfo())
		if c.Status != checkOK {
			t.Errorf("checkPackages = %q, want ok (detail=%q)", c.Status, c.Detail)
		}
	})

	t.Run("drift-warns", func(t *testing.T) {
		fake := archPacmanFake(t) // git installed; zoxide query unregistered => not installed
		engine := testDriftEngine(t, fake)
		c := checkPackages(parsePkgManifest(t, twoPkgManifest), engine, archInfo())
		if c.Status != checkWarn {
			t.Fatalf("checkPackages = %q, want warn", c.Status)
		}
		if !strings.Contains(c.Detail, "1 of 2") {
			t.Errorf("detail = %q, want it to report 1 of 2", c.Detail)
		}
		if !strings.Contains(c.Remedy, "dotf apply") {
			t.Errorf("remedy = %q, want it to name dotf apply", c.Remedy)
		}
	})

	t.Run("probe-error-fails", func(t *testing.T) {
		// The manifest declares a yum name so the entry IS applicable, but the
		// pkg backend has no yum argv, so Installed returns an
		// *UnsupportedPkgMgrError: a real probe error, exercising the fail
		// branch. (Post-ADR-2, PkgMgrUnknown no longer reaches the probe at all
		// — it is a clean not-applicable.)
		engine := testDriftEngine(t, exec.NewFake())
		info := platform.Info{OS: platform.OSRedHat, Arch: platform.ArchX86_64, PkgMgr: platform.PkgMgrYUM}
		c := checkPackages(parsePkgManifest(t, singlePkgManifest), engine, info)
		if c.Status != checkFail {
			t.Errorf("checkPackages = %q, want fail (detail=%q)", c.Status, c.Detail)
		}
	})

	t.Run("items-name-backend-class", func(t *testing.T) {
		// git installed, zoxide not => one itemized package finding carrying the
		// missing package's identity (name + backend + effective class).
		fake := archPacmanFake(t)
		engine := testDriftEngine(t, fake)
		c := checkPackages(parsePkgManifest(t, twoPkgManifest), engine, archInfo())
		if len(c.Items) != 1 {
			t.Fatalf("items = %d (%+v), want 1", len(c.Items), c.Items)
		}
		it := c.Items[0]
		if it.Kind != "package" || it.Name != "zoxide" || it.Backend != "pkg" || it.Class != "managed" {
			t.Errorf("item = %+v, want package/zoxide/pkg/managed", it)
		}
		// The aggregate count in Detail is preserved alongside the items.
		if !strings.Contains(c.Detail, "1 of 2") {
			t.Errorf("detail = %q, want it to still report 1 of 2", c.Detail)
		}
	})

	t.Run("no-items-when-clean", func(t *testing.T) {
		fake := archPacmanFake(t) // git installed
		engine := testDriftEngine(t, fake)
		c := checkPackages(parsePkgManifest(t, singlePkgManifest), engine, archInfo())
		if c.Status != checkOK || len(c.Items) != 0 {
			t.Errorf("clean check = %q items=%+v, want ok with no items", c.Status, c.Items)
		}
	})
}

func TestCheckSymlinks(t *testing.T) {
	t.Run("no-repo-root", func(t *testing.T) {
		checks := checkSymlinks("", t.TempDir(), archInfo())
		if len(checks) != 1 {
			t.Fatalf("checks = %d, want 1", len(checks))
		}
		if checks[0].Status != checkFail || checks[0].Name != "broken" {
			t.Errorf("check = %+v, want broken/fail", checks[0])
		}
	})

	t.Run("missing-source-tree", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "nope")
		checks := checkSymlinks(missing, t.TempDir(), archInfo())
		if len(checks) != 1 || checks[0].Status != checkFail {
			t.Fatalf("checks = %+v, want a single broken fail", checks)
		}
	})

	t.Run("no-home", func(t *testing.T) {
		checks := checkSymlinks(t.TempDir(), "", archInfo())
		if len(checks) != 1 || checks[0].Status != checkFail || checks[0].Name != "broken" {
			t.Fatalf("checks = %+v, want a single broken fail for an unresolvable home", checks)
		}
	})

	t.Run("clean-empty-repo", func(t *testing.T) {
		checks := checkSymlinks(t.TempDir(), t.TempDir(), archInfo())
		broken, _ := findCheck(checks, "symlinks", "broken")
		pending, _ := findCheck(checks, "symlinks", "pending")
		stale, ok := findCheck(checks, "symlinks", "stale")
		if broken.Status != checkOK || pending.Status != checkOK {
			t.Errorf("broken=%q pending=%q, want both ok", broken.Status, pending.Status)
		}
		if !ok || stale.Status != checkOK {
			t.Errorf("stale = %+v (found=%v), want ok", stale, ok)
		}
	})

	t.Run("stale-warns", func(t *testing.T) {
		// ~/.oldrc points into the repo at a file that no longer exists and is
		// no table entry's link: an orphan, surfaced as a warn item with the
		// prune remedy.
		repoRoot := t.TempDir()
		home := t.TempDir()
		old := filepath.Join(repoRoot, "bash/oldrc")
		if err := os.Symlink(old, filepath.Join(home, ".oldrc")); err != nil {
			t.Fatalf("Symlink error = %v", err)
		}
		checks := checkSymlinks(repoRoot, home, archInfo())
		stale, _ := findCheck(checks, "symlinks", "stale")
		if stale.Status != checkWarn {
			t.Fatalf("stale = %+v, want warn", stale)
		}
		if !strings.Contains(stale.Remedy, "dotf link --prune") {
			t.Errorf("stale remedy = %q, want dotf link --prune", stale.Remedy)
		}
		if len(stale.Items) != 1 || stale.Items[0].Kind != "symlink" ||
			stale.Items[0].Action != symlink.StaleReasonOrphaned || stale.Items[0].Dest != ".oldrc" {
			t.Errorf("stale items = %+v, want one symlink/orphaned/.oldrc", stale.Items)
		}
	})

	t.Run("pending-warns", func(t *testing.T) {
		repoRoot := t.TempDir()
		home := t.TempDir()
		seedRepoFile(t, repoRoot, "bash/bashrc", "# bashrc\n")
		checks := checkSymlinks(repoRoot, home, archInfo())
		pending, _ := findCheck(checks, "symlinks", "pending")
		if pending.Status != checkWarn {
			t.Errorf("pending = %q, want warn", pending.Status)
		}
		broken, _ := findCheck(checks, "symlinks", "broken")
		if broken.Status != checkOK {
			t.Errorf("broken = %q, want ok (dest not present)", broken.Status)
		}
	})

	t.Run("pending-items-action-dest", func(t *testing.T) {
		// Only bash/bashrc exists in the repo, so exactly one link is pending
		// (the rest skip on a missing source) and the item carries the
		// action + $HOME-relative dest + repo-relative src.
		repoRoot := t.TempDir()
		home := t.TempDir()
		seedRepoFile(t, repoRoot, "bash/bashrc", "# bashrc\n")
		checks := checkSymlinks(repoRoot, home, archInfo())
		pending, _ := findCheck(checks, "symlinks", "pending")
		if len(pending.Items) != 1 {
			t.Fatalf("pending items = %d (%+v), want 1", len(pending.Items), pending.Items)
		}
		it := pending.Items[0]
		if it.Kind != "symlink" || it.Action != "create" || it.Dest != ".bashrc" || it.Src != "bash/bashrc" {
			t.Errorf("item = %+v, want symlink/create/.bashrc/bash/bashrc", it)
		}
	})

	t.Run("broken-fails", func(t *testing.T) {
		repoRoot := t.TempDir()
		home := t.TempDir()
		seedRepoFile(t, repoRoot, "bash/bashrc", "# bashrc\n")
		if err := os.WriteFile(filepath.Join(home, ".bashrc"), []byte("real file\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(.bashrc) error = %v", err)
		}
		checks := checkSymlinks(repoRoot, home, archInfo())
		broken, _ := findCheck(checks, "symlinks", "broken")
		if broken.Status != checkFail {
			t.Errorf("broken = %q, want fail", broken.Status)
		}
		if !strings.Contains(broken.Remedy, "dotf link") {
			t.Errorf("broken remedy = %q, want it to name dotf link", broken.Remedy)
		}
	})
}

// seedPlasmaFile writes one plasma INI file at dir/rel, creating parents. It is
// used for both sides of the comparison: the repo snapshot under
// <repoRoot>/plasma/files and the live file under the fake config dir.
func seedPlasmaFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

// plasmaRepoDir returns <repoRoot>/plasma/files, the directory checkPlasma
// probes for the plasma.repo check.
func plasmaRepoDir(repoRoot string) string {
	return filepath.Join(repoRoot, "plasma", "files")
}

// kiorc is a managed INI file with no volatile strip rules and no colour-scheme
// coupling, so a repo/live pair of it exercises Diff's convergence predicate
// with nothing else in the way.
const (
	plasmaRepoKiorc      = "[Confirmations]\nConfirmDelete=true\n"
	plasmaLiveConverged  = "[Confirmations]\nConfirmDelete=true\nConfirmEmptyTrash=false\n"
	plasmaLiveDriftedRc  = "[Confirmations]\nConfirmDelete=false\n"
	plasmaManagedFileRel = "kiorc"
)

func TestCheckPlasma(t *testing.T) {
	t.Run("unsupported-os-skips", func(t *testing.T) {
		checks := checkPlasma(t.TempDir(), t.TempDir(), t.TempDir(), macInfo(), exec.NewFake())
		if len(checks) != 0 {
			t.Errorf("checkPlasma(macOS) = %+v, want no checks at all", checks)
		}
	})

	t.Run("relative-config-dir-skips", func(t *testing.T) {
		checks := checkPlasma(t.TempDir(), ".config", t.TempDir(), archInfo(), exec.NewFake())
		if len(checks) != 0 {
			t.Errorf("checkPlasma(relative config dir) = %+v, want no checks", checks)
		}
	})

	t.Run("repo-absent-warns", func(t *testing.T) {
		checks := checkPlasma(t.TempDir(), t.TempDir(), t.TempDir(), archInfo(), exec.NewFake())

		repo, ok := findCheck(checks, "plasma", "repo")
		if !ok {
			t.Fatalf("plasma.repo not emitted: %+v", checks)
		}
		if repo.Status != checkWarn {
			t.Errorf("plasma.repo = %q, want warn", repo.Status)
		}
		if !strings.Contains(repo.Remedy, "dotf plasma export") {
			t.Errorf("plasma.repo remedy = %q, want it to name dotf plasma export", repo.Remedy)
		}
		// Nothing was exported, so drift has nothing to compare and must not
		// invent a verdict of its own.
		drift, ok := findCheck(checks, "plasma", "drift")
		if !ok || drift.Status != checkOK {
			t.Errorf("plasma.drift = %v/%q, want ok", ok, drift.Status)
		}
	})

	t.Run("converged-ok", func(t *testing.T) {
		repoRoot, configDir := t.TempDir(), t.TempDir()
		seedPlasmaFile(t, plasmaRepoDir(repoRoot), plasmaManagedFileRel, plasmaRepoKiorc)
		seedPlasmaFile(t, configDir, plasmaManagedFileRel, plasmaLiveConverged)

		checks := checkPlasma(repoRoot, configDir, t.TempDir(), archInfo(), exec.NewFake())

		drift, ok := findCheck(checks, "plasma", "drift")
		if !ok {
			t.Fatalf("plasma.drift not emitted: %+v", checks)
		}
		if drift.Status != checkOK {
			t.Errorf("plasma.drift = %q (detail=%q, items=%+v), want ok", drift.Status, drift.Detail, drift.Items)
		}
		if len(drift.Items) != 0 {
			t.Errorf("converged drift carries items: %+v", drift.Items)
		}
		repo, _ := findCheck(checks, "plasma", "repo")
		if repo.Status != checkOK {
			t.Errorf("plasma.repo = %q, want ok once plasma/files exists", repo.Status)
		}
	})

	t.Run("drift-warns", func(t *testing.T) {
		repoRoot, configDir := t.TempDir(), t.TempDir()
		seedPlasmaFile(t, plasmaRepoDir(repoRoot), plasmaManagedFileRel, plasmaRepoKiorc)
		seedPlasmaFile(t, configDir, plasmaManagedFileRel, plasmaLiveDriftedRc)

		checks := checkPlasma(repoRoot, configDir, t.TempDir(), archInfo(), exec.NewFake())

		drift, ok := findCheck(checks, "plasma", "drift")
		if !ok {
			t.Fatalf("plasma.drift not emitted: %+v", checks)
		}
		if drift.Status != checkWarn {
			t.Fatalf("plasma.drift = %q (detail=%q), want warn", drift.Status, drift.Detail)
		}
		if !strings.Contains(drift.Remedy, "dotf plasma apply") {
			t.Errorf("plasma.drift remedy = %q, want it to name dotf plasma apply", drift.Remedy)
		}
		if len(drift.Items) != 1 {
			t.Fatalf("drift items = %+v, want exactly the one drifted file", drift.Items)
		}
		it := drift.Items[0]
		if it.Kind != "plasma" || it.Action != "drift" || it.Dest != plasmaManagedFileRel {
			t.Errorf("drift item = %+v, want plasma/drift/%s", it, plasmaManagedFileRel)
		}
	})

	t.Run("never-fails-on-an-unreadable-file", func(t *testing.T) {
		// A repo file the KConfig codec refuses is a plasma problem, never a
		// broken host: it must degrade to warn, not fail.
		repoRoot, configDir := t.TempDir(), t.TempDir()
		seedPlasmaFile(t, plasmaRepoDir(repoRoot), plasmaManagedFileRel, "not a group header\n")
		seedPlasmaFile(t, configDir, plasmaManagedFileRel, plasmaLiveConverged)

		checks := checkPlasma(repoRoot, configDir, t.TempDir(), archInfo(), exec.NewFake())

		for _, c := range checks {
			if c.Status == checkFail {
				t.Errorf("plasma check %s.%s failed (%q); plasma must never fail", c.Category, c.Name, c.Detail)
			}
		}
		drift, _ := findCheck(checks, "plasma", "drift")
		if drift.Status != checkWarn || drift.Remedy == "" {
			t.Errorf("plasma.drift = %q remedy=%q, want a warn with a remedy", drift.Status, drift.Remedy)
		}
		// Pin the branch: this is the engine-error arm, not the drifted arm.
		if !strings.Contains(drift.Detail, "plasma diff failed") {
			t.Errorf("plasma.drift detail = %q, want the diff-failure detail", drift.Detail)
		}
	})
}

// TestDoctorItemLine_Plasma pins the plasma item's rendering: its dest is
// config-dir-relative, so it is printed bare rather than with the symlink
// kind's ~/ prefix.
func TestDoctorItemLine_Plasma(t *testing.T) {
	got := doctorItemLine(doctorItem{Kind: "plasma", Action: "drift", Dest: "kdedefaults/kwinrc"})
	if want := "drift kdedefaults/kwinrc"; got != want {
		t.Errorf("doctorItemLine(plasma) = %q, want %q", got, want)
	}
}

func TestSummarizeChecks_And_ExitCode(t *testing.T) {
	checks := []doctorCheck{
		{Status: checkOK}, {Status: checkOK}, {Status: checkWarn}, {Status: checkFail},
	}
	sum := summarizeChecks(checks)
	if sum.Total != 4 || sum.OK != 2 || sum.Warn != 1 || sum.Fail != 1 {
		t.Errorf("summary = %+v, want total=4 ok=2 warn=1 fail=1", sum)
	}
	if doctorExitCode(sum) != 1 {
		t.Errorf("exit code with a fail = %d, want 1", doctorExitCode(sum))
	}

	warnOnly := summarizeChecks([]doctorCheck{{Status: checkOK}, {Status: checkWarn}})
	if doctorExitCode(warnOnly) != 0 {
		t.Errorf("exit code with warn-only = %d, want 0 (warnings stay green)", doctorExitCode(warnOnly))
	}
}

func TestManifestFailCheck(t *testing.T) {
	c := manifestFailCheck(os.ErrNotExist)
	if c.Category != "manifest" || c.Name != "load" || c.Status != checkFail {
		t.Errorf("manifestFailCheck = %+v, want manifest.load fail", c)
	}
}

// ----------------------------------------------------------------------------
// runDoctor integration (host-independent: OS family only ever affects
// supported/ok verdicts here; assertions never depend on the real distro).
// ----------------------------------------------------------------------------

// healthyDoctorFake returns a Fake presenting a fully provisioned Linux host:
// git/curl/pacman/sudo on PATH and `pacman -Q git` reporting git installed.
func healthyDoctorFake() *exec.Fake {
	fake := exec.NewFake()
	fake.AddCommand("uname", []string{"-s"}, []byte("Linux\n"), nil)
	fake.AddCommand("uname", []string{"-m"}, []byte("x86_64\n"), nil)
	for _, bin := range []string{"git", "curl", "pacman", "sudo"} {
		fake.AddPath(bin, "/usr/bin/"+bin)
	}
	fake.AddCommand("pacman", []string{"-Q", "git"}, []byte("git 2.43.0-1\n"), nil)
	return fake
}

// writeDoctorManifest writes the single-git manifest to a temp file and returns
// its path.
func writeDoctorManifest(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "packages.toml")
	if err := os.WriteFile(path, []byte(singlePkgManifest), 0o644); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}
	return path
}

func TestRunDoctor_Healthy(t *testing.T) {
	repoRoot := t.TempDir() // exists but empty => all symlinks skip (source missing)
	t.Setenv("DOTFILES_ROOT", repoRoot)
	t.Setenv("HOME", t.TempDir())

	var stdout, stderr bytes.Buffer
	code := runDoctor([]string{"--manifest", writeDoctorManifest(t), "--no-user-config"},
		&stdout, &stderr, healthyDoctorFake())

	if code != 0 {
		t.Fatalf("runDoctor() code = %d, want 0; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "doctor.fail=0") {
		t.Errorf("stdout = %q, want doctor.fail=0", out)
	}
	for _, key := range []string{
		"check.platform.detect=ok",
		"check.backups.count=ok",
		"check.prerequisites.git=ok",
		"check.packages.drift=ok",
		"check.symlinks.broken=ok",
		"check.symlinks.pending=ok",
		"check.symlinks.stale=ok",
		"check.sysconfig.support=ok",
	} {
		if !strings.Contains(out, key) {
			t.Errorf("stdout missing %q\n%s", key, out)
		}
	}
	// Lines are sorted and the summary line is last.
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if !strings.HasPrefix(lines[len(lines)-1], "doctor.total=") {
		t.Errorf("last line = %q, want the doctor.total summary", lines[len(lines)-1])
	}
}

// TestRunDoctor_PlasmaChecks proves the two plasma rows reach the stdout
// contract, and that they are omitted entirely on a host plasma does not
// target. The host's real OS decides which arm runs — platform.Detect reads
// /etc/os-release, which no fake can override — so the expectation is derived
// from plasma.Support rather than hard-coded.
func TestRunDoctor_PlasmaChecks(t *testing.T) {
	repoRoot := t.TempDir() // no plasma/files => repo warns, drift has nothing to read
	t.Setenv("DOTFILES_ROOT", repoRoot)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	fake := healthyDoctorFake()
	var stdout, stderr bytes.Buffer
	code := runDoctor([]string{"--manifest", writeDoctorManifest(t), "--no-user-config"},
		&stdout, &stderr, fake)
	if code != 0 {
		t.Fatalf("runDoctor() code = %d, want 0 (plasma never fails); stderr=%q", code, stderr.String())
	}
	out := stdout.String()

	if plasma.Support(platform.Detect(fake)) == plasma.SupportUnsupported {
		if strings.Contains(out, "check.plasma.") {
			t.Errorf("stdout carries plasma checks on an unsupported host:\n%s", out)
		}
		return
	}
	for _, key := range []string{"check.plasma.drift=ok", "check.plasma.repo=warn"} {
		if !strings.Contains(out, key) {
			t.Errorf("stdout missing %q\n%s", key, out)
		}
	}
	if !strings.Contains(stderr.String(), "dotf plasma export") {
		t.Errorf("stderr missing the plasma.repo remedy:\n%s", stderr.String())
	}
}

func TestRunDoctor_MissingGitExits1(t *testing.T) {
	t.Setenv("DOTFILES_ROOT", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	fake := healthyDoctorFake()
	delete(fake.Found, "git") // remove git from PATH

	var stdout, stderr bytes.Buffer
	code := runDoctor([]string{"--manifest", writeDoctorManifest(t), "--no-user-config"},
		&stdout, &stderr, fake)

	if code != 1 {
		t.Fatalf("runDoctor(no git) code = %d, want 1", code)
	}
	if !strings.Contains(stdout.String(), "check.prerequisites.git=fail") {
		t.Errorf("stdout = %q, want git=fail", stdout.String())
	}
	if !strings.Contains(stderr.String(), "prerequisites.git") {
		t.Errorf("stderr = %q, want a git remedy line", stderr.String())
	}
}

func TestRunDoctor_BadFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runDoctor([]string{"--bogus"}, &stdout, &stderr, exec.NewFake())
	if code != 2 {
		t.Errorf("runDoctor(--bogus) code = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
}

func TestRunDoctor_JSON(t *testing.T) {
	t.Setenv("DOTFILES_ROOT", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	fake := healthyDoctorFake()
	delete(fake.Found, "git") // force a fail so the JSON exercises a remedy + fail tally

	var stdout, stderr bytes.Buffer
	code := runDoctor([]string{"--json", "--manifest", writeDoctorManifest(t), "--no-user-config"},
		&stdout, &stderr, fake)

	if code != 1 {
		t.Fatalf("runDoctor(--json, no git) code = %d, want 1", code)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty in JSON mode", stderr.String())
	}

	var rep doctorReport
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("json.Unmarshal error = %v; stdout=%q", err, stdout.String())
	}
	if rep.Command != "doctor" {
		t.Errorf("command = %q, want doctor", rep.Command)
	}

	// Summary tallies must equal the check statuses.
	recomputed := summarizeChecks(rep.Checks)
	if recomputed != rep.Summary {
		t.Errorf("summary = %+v, want %+v (recomputed from checks)", rep.Summary, recomputed)
	}
	if rep.Summary.Fail < 1 {
		t.Errorf("summary.fail = %d, want >=1", rep.Summary.Fail)
	}
	// Every non-ok check carries a remedy.
	for _, c := range rep.Checks {
		if c.Status != checkOK && c.Remedy == "" {
			t.Errorf("check %s.%s is %q but has no remedy", c.Category, c.Name, c.Status)
		}
	}
}

func TestRunDoctor_UnresolvableManifest(t *testing.T) {
	t.Setenv("DOTFILES_ROOT", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	var stdout, stderr bytes.Buffer
	code := runDoctor([]string{"--manifest", "/nonexistent/packages.toml", "--no-user-config"},
		&stdout, &stderr, healthyDoctorFake())

	if code != 1 {
		t.Fatalf("runDoctor(bad manifest) code = %d, want 1", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "check.manifest.load=fail") {
		t.Errorf("stdout = %q, want manifest.load=fail", out)
	}
	// The packages check is skipped when the manifest will not resolve, but the
	// rest of the host is still diagnosed.
	if strings.Contains(out, "check.packages.drift") {
		t.Errorf("stdout has a packages.drift check with no manifest: %q", out)
	}
	if !strings.Contains(out, "check.platform.detect=") {
		t.Errorf("stdout = %q, want platform.detect still diagnosed", out)
	}
}

func TestRunDoctor_ReadOnly(t *testing.T) {
	t.Setenv("DOTFILES_ROOT", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	fake := healthyDoctorFake()
	var stdout, stderr bytes.Buffer
	if code := runDoctor([]string{"--manifest", writeDoctorManifest(t), "--no-user-config"},
		&stdout, &stderr, fake); code != 0 {
		t.Fatalf("runDoctor() code = %d, want 0", code)
	}

	// No sudo, no mutating command may have been recorded.
	assertReadOnlyCalls(t, fake.CallsSnapshot())
}

func TestPrintDoctorRemedies_Items(t *testing.T) {
	checks := []doctorCheck{
		{
			Category: "packages", Name: "drift", Status: checkWarn,
			Detail: "1 of 34 applicable packages not installed",
			Remedy: "run `dotf install` or `dotf apply`",
			Items:  []doctorItem{{Kind: "package", Name: "starship", Backend: "github-release", Class: "pinned"}},
		},
		{
			Category: "symlinks", Name: "pending", Status: checkWarn,
			Detail: "1 link(s) not yet created", Remedy: "run `dotf link`",
			Items: []doctorItem{{Kind: "symlink", Action: "create", Dest: ".config/starship.toml", Src: "starship.toml"}},
		},
		{
			// No items: must print exactly its single aggregate line, no bullets.
			Category: "sysconfig", Name: "support", Status: checkWarn,
			Detail: "sysconfig is deferred on Windows",
			Remedy: "sysconfig support for Windows is not yet implemented",
		},
	}
	sum := summarizeChecks(checks)

	var stderr bytes.Buffer
	printDoctorRemedies(&stderr, checks, sum, false)
	out := stderr.String()

	// Aggregate lines are unchanged (single line per check, "detail; remedy").
	if !strings.Contains(out, "[warn] packages.drift: 1 of 34 applicable packages not installed; run `dotf install` or `dotf apply`") {
		t.Errorf("stderr missing the packages.drift aggregate line\n%s", out)
	}
	// One indented bullet per item, with the rendered identity/action.
	if !strings.Contains(out, "\n  - starship (backend=github-release, class=pinned)\n") {
		t.Errorf("stderr missing the itemized package line\n%s", out)
	}
	if !strings.Contains(out, "\n  - create ~/.config/starship.toml\n") {
		t.Errorf("stderr missing the itemized symlink line\n%s", out)
	}
	// The check without items contributes exactly one line and no bullet.
	if !strings.Contains(out, "[warn] sysconfig.support: sysconfig is deferred on Windows") {
		t.Errorf("stderr missing the item-less aggregate line\n%s", out)
	}
	if got := strings.Count(out, "\n  - "); got != 2 {
		t.Errorf("indented item lines = %d, want 2 (only the two checks with items)\n%s", got, out)
	}
}

// TestRunDoctor_VerboseEvidence proves --verbose prints ok-check evidence on
// stderr while the default run does not (a healthy host emits no non-ok checks,
// so its default stderr is summary-only).
func TestRunDoctor_VerboseEvidence(t *testing.T) {
	t.Setenv("DOTFILES_ROOT", t.TempDir()) // empty repo => all symlinks skip/ok
	t.Setenv("HOME", t.TempDir())

	base := []string{"--manifest", writeDoctorManifest(t), "--no-user-config"}

	var defStdout, defStderr bytes.Buffer
	if code := runDoctor(base, &defStdout, &defStderr, healthyDoctorFake()); code != 0 {
		t.Fatalf("runDoctor() default code = %d, want 0; stderr=%q", code, defStderr.String())
	}
	if strings.Contains(defStderr.String(), "[ok]") {
		t.Errorf("default stderr leaked ok-check evidence:\n%s", defStderr.String())
	}

	var vbStdout, vbStderr bytes.Buffer
	if code := runDoctor(append([]string{"--verbose"}, base...), &vbStdout, &vbStderr, healthyDoctorFake()); code != 0 {
		t.Fatalf("runDoctor(--verbose) code = %d, want 0; stderr=%q", code, vbStderr.String())
	}
	vb := vbStderr.String()
	if !strings.Contains(vb, "[ok] sysconfig.support: sysconfig supported on this OS") {
		t.Errorf("--verbose stderr missing sysconfig ok evidence:\n%s", vb)
	}
	if !strings.Contains(vb, "[ok] symlinks.pending: all managed links present") {
		t.Errorf("--verbose stderr missing symlink ok evidence:\n%s", vb)
	}
	// stdout is status-only and identical regardless of verbosity (Option A).
	if defStdout.String() != vbStdout.String() {
		t.Errorf("stdout differs between default and --verbose:\ndefault=%q\nverbose=%q", defStdout.String(), vbStdout.String())
	}
}

// TestRunDoctor_JSONItems proves --json carries the additive items[] array for a
// warn check, and that an item-less check still encodes without an "items" key
// (default shape unchanged, no --verbose needed).
func TestRunDoctor_JSONItems(t *testing.T) {
	t.Setenv("DOTFILES_ROOT", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	var stdout, stderr bytes.Buffer
	code := runDoctor([]string{"--json", "--manifest", writeManifestFile(t, twoPkgManifest), "--no-user-config"},
		&stdout, &stderr, healthyDoctorFake())
	if code != 0 {
		t.Fatalf("runDoctor(--json) code = %d, want 0 (drift is a warn); stderr=%q", code, stderr.String())
	}

	var rep doctorReport
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("json.Unmarshal error = %v; stdout=%q", err, stdout.String())
	}

	drift, ok := findCheck(rep.Checks, "packages", "drift")
	if !ok {
		t.Fatal("packages.drift check absent from JSON report")
	}
	if drift.Status != checkWarn || len(drift.Items) != 1 {
		t.Fatalf("drift = %q items=%+v, want warn with 1 item", drift.Status, drift.Items)
	}
	it := drift.Items[0]
	if it.Kind != "package" || it.Name != "zoxide" || it.Backend != "pkg" || it.Class != "managed" {
		t.Errorf("drift item = %+v, want package/zoxide/pkg/managed", it)
	}

	// An item-less ok check must not encode an "items" key — the default --json
	// shape is byte-for-byte backward compatible for such checks.
	sysCheck, ok := findCheck(rep.Checks, "sysconfig", "support")
	if !ok {
		t.Fatal("sysconfig.support check absent from JSON report")
	}
	if len(sysCheck.Items) != 0 {
		t.Errorf("item-less check carries items: %+v", sysCheck.Items)
	}
	raw, err := json.Marshal(sysCheck)
	if err != nil {
		t.Fatalf("json.Marshal(sysCheck) error = %v", err)
	}
	if strings.Contains(string(raw), "items") {
		t.Errorf("item-less check encoded with an items key: %s", raw)
	}
}
