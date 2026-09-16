package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// brewInstallerCalled reports whether the official Homebrew installer was
// invoked through f (the constant `bash -c <curl … Homebrew/install …>` call
// issued by the brew-bootstrap step).
func brewInstallerCalled(f *exec.Fake) bool {
	for _, c := range f.CallsSnapshot() {
		if c.Name == "bash" && len(c.Args) == 2 && c.Args[0] == "-c" && strings.Contains(c.Args[1], "Homebrew/install") {
			return true
		}
	}
	return false
}

// applyLocalePkgManifest declares a locale (so the identity sysconfig pass emits
// a `sudo localectl set-locale …` mutation on any Linux family) plus one cargo
// package (so the packages phase runs a `cargo install rg`). The concrete
// mutation is family-dependent: on RedHat the locale step installs a langpack
// and keeps the target at en_US.UTF-8, while on other families the missing
// locale-gen degrades the target to C. Match it with setLocaleIndex rather than
// hard-coding a LANG= value.
const applyLocalePkgManifest = `
[sysconfig]
locale = "en_US.UTF-8"

[[package]]
name = "rg"
backend = "toolchain"
tool = "cargo"
`

// applyFirewallPkgManifest declares an explicit ufw firewall (so the firewall
// pass emits ufw steps regardless of the host's distro family) plus one cargo
// package (so ordering relative to the packages phase is observable).
const applyFirewallPkgManifest = `
[sysconfig]
firewall = "ufw"

[[package]]
name = "rg"
backend = "toolchain"
tool = "cargo"
`

// applySSHServerPkgManifest declares ssh_server=on (so the ssh-server pass
// emits steps regardless of other manifest defaults) plus one cargo package.
const applySSHServerPkgManifest = `
[sysconfig]
ssh_server = "on"

[[package]]
name = "rg"
backend = "toolchain"
tool = "cargo"
`

// applySSHServerFirewallPkgManifest declares both ufw and ssh_server=on so
// applyFirewall and applySSHServer both run and their ordering is observable.
const applySSHServerFirewallPkgManifest = `
[sysconfig]
firewall = "ufw"
ssh_server = "on"

[[package]]
name = "rg"
backend = "toolchain"
tool = "cargo"
`

// applyRuntimesManifest declares an opt-in `runtimes` intent whose packages use
// the custom-script backend, so a dry-run of `--intent runtimes` exercises the
// provisioning-parity planning path without needing a package manager.
const applyRuntimesManifest = `
[intents.runtimes]
description = "runtimes"
default = false
packages = ["mise", "rust"]

[[package]]
name = "mise"
backend = "custom-script"
script = "bin/install-mise.sh"
class = "rolling"
justification = "self-updating runtime manager"

[[package]]
name = "rust"
backend = "custom-script"
script = "bin/install-rust.sh"
class = "rolling"
justification = "rustup self-updating toolchain"
`

// newApplyFake returns a fake that forces a Linux-family OS via `uname -s`.
// The concrete distro still comes from the host's /etc/os-release: every Linux
// family dispatches to the same sysconfig/snapper/symlink code paths, but the
// concrete sysconfig mutations those paths emit are family-dependent, so a test
// must not pin a distro-specific argv (see setLocaleIndex).
func newApplyFake(t *testing.T) *exec.Fake {
	t.Helper()
	f := exec.NewFake()
	f.AddCommand("uname", []string{"-s"}, []byte("Linux\n"), nil)
	f.AddCommand("uname", []string{"-m"}, []byte("x86_64\n"), nil)
	return f
}

// newApplyDarwinFake forces macOS via `uname -s`, so snapper is unavailable and
// the sysconfig identity pass dispatches to the darwin configurer regardless of
// the host the test runs on.
func newApplyDarwinFake(t *testing.T) *exec.Fake {
	t.Helper()
	f := exec.NewFake()
	f.AddCommand("uname", []string{"-s"}, []byte("Darwin\n"), nil)
	f.AddCommand("uname", []string{"-m"}, []byte("arm64\n"), nil)
	return f
}

// writeRepoFile writes content to root/rel, creating parent directories, so a
// symlink source exists for the symlink phase to link into $HOME.
func writeRepoFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(p), err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", p, err)
	}
}

// cargoInstallIndex returns the index of the first `cargo install <name>` call,
// or -1.
func cargoInstallIndex(calls []exec.FakeCall, name string) int {
	for i, c := range calls {
		if c.Name == "cargo" && len(c.Args) == 2 && c.Args[0] == "install" && c.Args[1] == name {
			return i
		}
	}
	return -1
}

// setLocaleIndex returns the index of the first recorded
// `sudo localectl set-locale …` call, or -1. It matches on the verb alone,
// irrespective of the LANG= value, because the locale the sysconfig pass
// settles on is family-dependent (see applyLocalePkgManifest) — an ordering
// assertion must not be coupled to the host distro the test runs on.
func setLocaleIndex(calls []exec.FakeCall) int {
	for i, c := range calls {
		if c.Name == "sudo" && len(c.Args) >= 2 && c.Args[0] == "localectl" && c.Args[1] == "set-locale" {
			return i
		}
	}
	return -1
}

// setLocaleLang returns the `LANG=<value>` argument the sysconfig locale step
// emits on the host family the fake resolves to, so a test can inject a failure
// keyed to the argv that actually runs.
func setLocaleLang(f *exec.Fake) string {
	if platform.Detect(f).OS == platform.OSRedHat {
		return "LANG=en_US.UTF-8"
	}
	return "LANG=C"
}

// isDesktopFamily reports whether os is a Linux family on which the opt-in
// desktop symlink facet applies (mirrors symlink.isLinuxFamily, which excludes
// bare OSLinux). Used to skip the desktop test on a host whose os-release does
// not resolve to a concrete family.
func isDesktopFamily(os platform.OS) bool {
	switch os {
	case platform.OSDebian, platform.OSRedHat, platform.OSArch, platform.OSSUSE, platform.OSWSL:
		return true
	default:
		return false
	}
}
