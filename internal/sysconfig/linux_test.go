package sysconfig

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// assertFileStep asserts a file Step's destination, backup flag, and a content
// substring.
func assertFileStep(t *testing.T, got Step, wantDest string, wantBackup bool, contentSubstr string) {
	t.Helper()
	if got.File == nil {
		t.Fatalf("step %q is not a file step", got.Desc)
	}
	if got.File.Dest != wantDest {
		t.Errorf("file dest = %q, want %q", got.File.Dest, wantDest)
	}
	if got.File.Backup != wantBackup {
		t.Errorf("file backup = %v, want %v", got.File.Backup, wantBackup)
	}
	if contentSubstr != "" && !strings.Contains(string(got.File.Content), contentSubstr) {
		t.Errorf("file content = %q, want substring %q", got.File.Content, contentSubstr)
	}
}

// assertReadOnlyPlanCalls fails if any recorded call is a privileged mutation.
// The only allowed privileged READS are the two non-interactive idempotency
// probes — `sudo -n ufw status` and `sudo -n firewall-cmd --permanent
// --query-service=ssh` — which need root (the latter is polkit-gated) but never
// prompt. Any other sudo call recorded during Plan is still a failure.
func assertReadOnlyPlanCalls(t *testing.T, calls []exec.FakeCall) {
	t.Helper()
	for _, call := range calls {
		if call.Name != "sudo" {
			continue
		}
		if len(call.Args) >= 3 && call.Args[0] == "-n" && call.Args[1] == "ufw" && call.Args[2] == "status" {
			continue
		}
		if len(call.Args) >= 4 && call.Args[0] == "-n" && call.Args[1] == "firewall-cmd" &&
			call.Args[2] == "--permanent" && call.Args[3] == "--query-service=ssh" {
			continue
		}
		t.Errorf("mutating command recorded during Plan: sudo %v", call.Args)
	}
}

// stepIndex returns the index of the first command Step whose argv contains
// token, or -1.
func stepIndex(steps []Step, token string) int {
	for i, s := range steps {
		for _, a := range s.Argv {
			if a == token {
				return i
			}
		}
	}
	return -1
}

func newLinuxTest(fake *exec.Fake, root string) *linuxConfigurer {
	return newLinuxConfigurer(fake, NewOSFileSystem(root))
}

func TestLinux_Plan_AllDomains(t *testing.T) {
	root := t.TempDir()
	seedFile(t, root, "etc/hosts", []byte("127.0.0.1 localhost\n"))
	seedFile(t, root, "usr/share/zoneinfo/America/New_York", []byte("TZif"))
	seedFile(t, root, "etc/shells", []byte("/bin/bash\n/bin/sh\n"))

	fake := exec.NewFake()
	fake.AddCommand("hostnamectl", []string{"--static"}, []byte("oldhost\n"), nil)
	fake.AddCommand("timedatectl", []string{"show", "-p", "Timezone", "--value"}, []byte("UTC\n"), nil)
	fake.AddCommand("locale", []string{"-a"}, []byte("C\nC.UTF-8\nPOSIX\n"), nil)
	fake.AddPath("locale-gen", "/usr/bin/locale-gen")
	fake.AddCommand("localectl", []string{"status"}, []byte("   System Locale: LANG=C.UTF-8\n"), nil)
	fake.AddPath("zsh", "/usr/bin/zsh")
	fake.AddPath("chsh", "/usr/bin/chsh")
	fake.AddCommand("getent", []string{"passwd", "yandy"}, []byte("yandy:x:1000:1000::/home/yandy:/bin/bash\n"), nil)
	fake.AddPath("ufw", "/usr/bin/ufw")
	fake.AddCommand("sudo", []string{"-n", "ufw", "status"}, []byte("Status: inactive\n"), nil)

	cfg := Config{
		Hostname: "web01",
		Timezone: "America/New_York",
		Locale:   "en_US.UTF-8",
		Shell:    "zsh",
		Firewall: FirewallAuto,
		User:     "yandy",
	}

	plan, err := newLinuxTest(fake, root).Plan(cfg, platform.Info{OS: platform.OSDebian})
	if err != nil {
		t.Fatalf("Plan() error = %v, want nil", err)
	}
	if len(plan.Steps) != 9 {
		t.Fatalf("Plan() steps = %d, want 9:\n%s", len(plan.Steps), formatSteps(plan.Steps))
	}

	assertStepArgv(t, plan.Steps[0], []string{"sudo", "hostnamectl", "set-hostname", "web01"})
	assertFileStep(t, plan.Steps[1], "/etc/hosts", true, "127.0.1.1\tweb01")
	assertStepArgv(t, plan.Steps[2], []string{"sudo", "timedatectl", "set-timezone", "America/New_York"})
	assertStepArgv(t, plan.Steps[3], []string{"sudo", "locale-gen", "en_US.UTF-8"})
	assertStepArgv(t, plan.Steps[4], []string{"sudo", "localectl", "set-locale", "LANG=en_US.UTF-8"})
	assertFileStep(t, plan.Steps[5], "/etc/shells", true, "/usr/bin/zsh")
	assertStepArgv(t, plan.Steps[6], []string{"sudo", "chsh", "-s", "/usr/bin/zsh", "yandy"})
	assertStepArgv(t, plan.Steps[7], []string{"sudo", "ufw", "allow", "OpenSSH"})
	assertStepArgv(t, plan.Steps[8], []string{"sudo", "ufw", "--force", "enable"})

	assertReadOnlyPlanCalls(t, fake.CallsSnapshot())
}

func TestLinux_Plan_Idempotent(t *testing.T) {
	root := t.TempDir()
	seedFile(t, root, "etc/hosts", []byte("127.0.0.1 localhost\n127.0.1.1 web01\n"))
	seedFile(t, root, "usr/share/zoneinfo/America/New_York", []byte("TZif"))
	seedFile(t, root, "etc/shells", []byte("/bin/bash\n/usr/bin/zsh\n"))

	fake := exec.NewFake()
	fake.AddCommand("hostnamectl", []string{"--static"}, []byte("web01\n"), nil)
	fake.AddCommand("timedatectl", []string{"show", "-p", "Timezone", "--value"}, []byte("America/New_York\n"), nil)
	fake.AddCommand("locale", []string{"-a"}, []byte("en_US.UTF-8\nC\nC.UTF-8\n"), nil)
	fake.AddCommand("localectl", []string{"status"}, []byte("   System Locale: LANG=en_US.UTF-8\n"), nil)
	fake.AddPath("zsh", "/usr/bin/zsh")
	fake.AddCommand("getent", []string{"passwd", "yandy"}, []byte("yandy:x:1000:1000::/home/yandy:/usr/bin/zsh\n"), nil)
	fake.AddPath("ufw", "/usr/bin/ufw")
	fake.AddCommand("sudo", []string{"-n", "ufw", "status"}, []byte("Status: active\n\nTo                         Action      From\n--                         ------      ----\nOpenSSH                    ALLOW       Anywhere\n"), nil)

	cfg := Config{
		Hostname: "web01",
		Timezone: "America/New_York",
		Locale:   "en_US.UTF-8",
		Shell:    "zsh",
		Firewall: FirewallAuto,
		User:     "yandy",
	}

	plan, err := newLinuxTest(fake, root).Plan(cfg, platform.Info{OS: platform.OSDebian})
	if err != nil {
		t.Fatalf("Plan() error = %v, want nil", err)
	}
	if len(plan.Steps) != 0 {
		t.Fatalf("Plan() steps = %d, want 0 (converged host):\n%s", len(plan.Steps), formatSteps(plan.Steps))
	}

	// A converged Plan executes nothing → zero mutating commands.
	eng := NewEngine(nil, fake, NewOSFileSystem(root))
	res, err := eng.Execute(plan)
	if err != nil {
		t.Fatalf("Execute(empty) error = %v", err)
	}
	if len(res.Applied) != 0 {
		t.Errorf("Execute(empty) applied = %d steps, want 0", len(res.Applied))
	}
	assertReadOnlyPlanCalls(t, fake.CallsSnapshot())
}

func TestLinux_Locale_GenBeforeSet(t *testing.T) {
	root := t.TempDir()

	fake := exec.NewFake()
	fake.AddCommand("locale", []string{"-a"}, []byte("C\nC.UTF-8\nen_US.UTF-8\n"), nil)
	fake.AddPath("locale-gen", "/usr/bin/locale-gen")
	fake.AddCommand("localectl", []string{"status"}, []byte("System Locale: LANG=en_US.UTF-8\n"), nil)

	cfg := Config{Locale: "de_DE.UTF-8", User: "yandy"}
	plan, err := newLinuxTest(fake, root).Plan(cfg, platform.Info{OS: platform.OSDebian})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	genIdx := stepIndex(plan.Steps, "locale-gen")
	setIdx := stepIndex(plan.Steps, "set-locale")
	if genIdx < 0 || setIdx < 0 {
		t.Fatalf("expected both locale-gen and set-locale steps:\n%s", formatSteps(plan.Steps))
	}
	if genIdx >= setIdx {
		t.Errorf("locale-gen index = %d, set-locale index = %d; want gen before set", genIdx, setIdx)
	}
	assertStepArgv(t, plan.Steps[genIdx], []string{"sudo", "locale-gen", "de_DE.UTF-8"})
	assertStepArgv(t, plan.Steps[setIdx], []string{"sudo", "localectl", "set-locale", "LANG=de_DE.UTF-8"})
}

func TestLinux_Locale_ArchUncommentThenGen(t *testing.T) {
	root := t.TempDir()
	seedFile(t, root, "etc/locale.gen", []byte("#de_DE.UTF-8 UTF-8\n#en_US.UTF-8 UTF-8\n"))

	fake := exec.NewFake()
	fake.AddCommand("locale", []string{"-a"}, []byte("C\nC.UTF-8\n"), nil)
	fake.AddPath("locale-gen", "/usr/bin/locale-gen")
	fake.AddCommand("localectl", []string{"status"}, []byte("System Locale: LANG=C\n"), nil)

	cfg := Config{Locale: "de_DE.UTF-8", User: "yandy"}
	plan, err := newLinuxTest(fake, root).Plan(cfg, platform.Info{OS: platform.OSArch})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(plan.Steps) != 3 {
		t.Fatalf("Plan() steps = %d, want 3 (uncomment file, locale-gen, set):\n%s", len(plan.Steps), formatSteps(plan.Steps))
	}
	assertFileStep(t, plan.Steps[0], "/etc/locale.gen", true, "de_DE.UTF-8 UTF-8")
	if strings.Contains(string(plan.Steps[0].File.Content), "#de_DE.UTF-8") {
		t.Errorf("locale.gen file still comments de_DE:\n%s", plan.Steps[0].File.Content)
	}
	assertStepArgv(t, plan.Steps[1], []string{"sudo", "locale-gen"})
	assertStepArgv(t, plan.Steps[2], []string{"sudo", "localectl", "set-locale", "LANG=de_DE.UTF-8"})
}

func TestLinux_Locale_FallbackWhenNoLocaleGen(t *testing.T) {
	root := t.TempDir()

	fake := exec.NewFake()
	fake.AddCommand("locale", []string{"-a"}, []byte("C\nC.UTF-8\nPOSIX\n"), nil)
	// locale-gen intentionally NOT on PATH.
	fake.AddCommand("localectl", []string{"status"}, []byte("System Locale: LANG=C\n"), nil)

	cfg := Config{Locale: "de_DE.UTF-8", User: "yandy"}
	plan, err := newLinuxTest(fake, root).Plan(cfg, platform.Info{OS: platform.OSDebian})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if stepIndex(plan.Steps, "locale-gen") != -1 {
		t.Errorf("locale-gen step emitted without locale-gen on PATH:\n%s", formatSteps(plan.Steps))
	}
	setIdx := stepIndex(plan.Steps, "set-locale")
	if setIdx < 0 {
		t.Fatalf("no set-locale step:\n%s", formatSteps(plan.Steps))
	}
	assertStepArgv(t, plan.Steps[setIdx], []string{"sudo", "localectl", "set-locale", "LANG=C.UTF-8"})
}

func TestLinux_Locale_NormalizedAvailableNoGen(t *testing.T) {
	root := t.TempDir()

	fake := exec.NewFake()
	fake.AddCommand("locale", []string{"-a"}, []byte("en_us.utf8\nC\n"), nil)
	fake.AddPath("locale-gen", "/usr/bin/locale-gen") // present but must NOT be used
	fake.AddCommand("localectl", []string{"status"}, []byte("System Locale: LANG=C\n"), nil)

	cfg := Config{Locale: "en_US.UTF-8", User: "yandy"}
	plan, err := newLinuxTest(fake, root).Plan(cfg, platform.Info{OS: platform.OSDebian})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if stepIndex(plan.Steps, "locale-gen") != -1 {
		t.Errorf("locale-gen emitted for an already-available locale (utf-8 vs utf8):\n%s", formatSteps(plan.Steps))
	}
	setIdx := stepIndex(plan.Steps, "set-locale")
	if setIdx < 0 {
		t.Fatalf("no set-locale step:\n%s", formatSteps(plan.Steps))
	}
	assertStepArgv(t, plan.Steps[setIdx], []string{"sudo", "localectl", "set-locale", "LANG=en_US.UTF-8"})
}

// TestLinux_Locale_RedHatLangpack proves Fedora installs the glibc langpack
// rather than silently downgrading LANG to C: it has no locale-gen and no
// /etc/locale.gen, so the pre-fix code fell through resolveLocale to "C".
func TestLinux_Locale_RedHatLangpack(t *testing.T) {
	fake := exec.NewFake()
	fake.AddCommand("locale", []string{"-a"}, []byte("C\nC.UTF-8\nPOSIX\n"), nil)
	// locale-gen intentionally NOT on PATH (Fedora ships none).
	fake.AddCommand("localectl", []string{"status"}, []byte("System Locale: LANG=C\n"), nil)

	cfg := Config{Locale: "de_DE.UTF-8", User: "yandy"}
	plan, err := newLinuxTest(fake, t.TempDir()).Plan(cfg, platform.Info{OS: platform.OSRedHat})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(plan.Steps) != 2 {
		t.Fatalf("Plan() steps = %d, want 2 (langpack install, set-locale):\n%s", len(plan.Steps), formatSteps(plan.Steps))
	}
	assertStepArgv(t, plan.Steps[0], []string{"sudo", "dnf", "install", "-y", "glibc-langpack-de"})
	assertStepArgv(t, plan.Steps[1], []string{"sudo", "localectl", "set-locale", "LANG=de_DE.UTF-8"})
}

func TestLinux_Shell_EtcShellsGuard(t *testing.T) {
	tests := []struct {
		name         string
		etcShells    string
		currentShell string
		wantFile     bool
		wantChsh     bool
	}{
		{name: "path absent → guard before chsh", etcShells: "/bin/bash\n", currentShell: "/bin/bash", wantFile: true, wantChsh: true},
		{name: "path present → chsh only", etcShells: "/bin/bash\n/usr/bin/zsh\n", currentShell: "/bin/bash", wantFile: false, wantChsh: true},
		{name: "already current → no chsh", etcShells: "/bin/bash\n/usr/bin/zsh\n", currentShell: "/usr/bin/zsh", wantFile: false, wantChsh: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			seedFile(t, root, "etc/shells", []byte(tt.etcShells))

			fake := exec.NewFake()
			fake.AddPath("zsh", "/usr/bin/zsh")
			fake.AddPath("chsh", "/usr/bin/chsh")
			fake.AddCommand("getent", []string{"passwd", "yandy"}, []byte("yandy:x:1000:1000::/home/yandy:"+tt.currentShell+"\n"), nil)

			cfg := Config{Shell: "zsh", User: "yandy"}
			plan, err := newLinuxTest(fake, root).Plan(cfg, platform.Info{OS: platform.OSDebian})
			if err != nil {
				t.Fatalf("Plan() error = %v", err)
			}

			fileIdx, chshIdx := -1, -1
			for i, s := range plan.Steps {
				if s.File != nil && s.File.Dest == "/etc/shells" {
					fileIdx = i
				}
				if len(s.Argv) >= 2 && s.Argv[1] == "chsh" {
					chshIdx = i
				}
			}

			if tt.wantFile && fileIdx < 0 {
				t.Errorf("expected /etc/shells file step:\n%s", formatSteps(plan.Steps))
			}
			if !tt.wantFile && fileIdx >= 0 {
				t.Errorf("unexpected /etc/shells file step:\n%s", formatSteps(plan.Steps))
			}
			if tt.wantChsh && chshIdx < 0 {
				t.Errorf("expected chsh step:\n%s", formatSteps(plan.Steps))
			}
			if !tt.wantChsh && chshIdx >= 0 {
				t.Errorf("unexpected chsh step:\n%s", formatSteps(plan.Steps))
			}
			if tt.wantFile && tt.wantChsh && fileIdx >= chshIdx {
				t.Errorf("/etc/shells step (%d) must precede chsh (%d)", fileIdx, chshIdx)
			}
			if tt.wantChsh && chshIdx >= 0 {
				assertStepArgv(t, plan.Steps[chshIdx], []string{"sudo", "chsh", "-s", "/usr/bin/zsh", "yandy"})
			}
		})
	}
}

// TestLinux_Shell_UsermodFallbackWhenNoChsh proves ADR-8: on a host with no
// chsh (some Fedora releases ship it only in util-linux-user), the login-shell
// change is planned via `usermod -s` from shadow-utils rather than emitting an
// argv whose binary does not exist.
func TestLinux_Shell_UsermodFallbackWhenNoChsh(t *testing.T) {
	root := t.TempDir()
	seedFile(t, root, "etc/shells", []byte("/bin/bash\n/usr/bin/zsh\n"))

	fake := exec.NewFake()
	fake.AddPath("zsh", "/usr/bin/zsh")
	// chsh intentionally NOT on PATH.
	fake.AddCommand("getent", []string{"passwd", "yandy"}, []byte("yandy:x:1000:1000::/home/yandy:/bin/bash\n"), nil)

	cfg := Config{Shell: "zsh", User: "yandy"}
	plan, err := newLinuxTest(fake, root).Plan(cfg, platform.Info{OS: platform.OSRedHat})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("Plan() steps = %d, want 1:\n%s", len(plan.Steps), formatSteps(plan.Steps))
	}
	assertStepArgv(t, plan.Steps[0], []string{"sudo", "usermod", "-s", "/usr/bin/zsh", "yandy"})
}

// TestLinux_Shell_EmptyUserSkips proves that when no real user resolves
// (cfg.User == "", e.g. root with no SUDO_USER) the shell domain is skipped
// entirely: no chsh Step and no /etc/shells file step are emitted, so the
// documented "skip" guarantee holds instead of running `chsh -s <path> ""`.
func TestLinux_Shell_EmptyUserSkips(t *testing.T) {
	root := t.TempDir()
	seedFile(t, root, "etc/shells", []byte("/bin/bash\n"))

	fake := exec.NewFake()
	fake.AddPath("zsh", "/usr/bin/zsh")

	cfg := Config{Shell: "zsh", User: ""}
	plan, err := newLinuxTest(fake, root).Plan(cfg, platform.Info{OS: platform.OSDebian})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	for _, s := range plan.Steps {
		if s.Domain == "shell" {
			t.Errorf("emitted a shell step for an empty user, want none: %+v", s)
		}
	}
	if !containsStr(plan.Skipped, "shell") {
		t.Errorf("Plan().Skipped = %v, want to include shell for empty user", plan.Skipped)
	}
}

func TestLinux_Firewall_UfwSelection(t *testing.T) {
	tests := []struct {
		os          platform.OS
		wantInstall []string
	}{
		{os: platform.OSArch, wantInstall: []string{"sudo", "pacman", "-S", "--noconfirm", "--needed", "ufw"}},
		{os: platform.OSDebian, wantInstall: []string{"sudo", "apt-get", "install", "-y", "ufw"}},
	}

	for _, tt := range tests {
		t.Run(string(tt.os), func(t *testing.T) {
			fake := exec.NewFake() // ufw absent, status unregistered → inactive

			cfg := Config{Firewall: FirewallAuto, User: "yandy"}
			plan, err := newLinuxTest(fake, t.TempDir()).Plan(cfg, platform.Info{OS: tt.os})
			if err != nil {
				t.Fatalf("Plan() error = %v", err)
			}
			if len(plan.Steps) != 3 {
				t.Fatalf("Plan() firewall steps = %d, want 3:\n%s", len(plan.Steps), formatSteps(plan.Steps))
			}
			assertStepArgv(t, plan.Steps[0], tt.wantInstall)
			assertStepArgv(t, plan.Steps[1], []string{"sudo", "ufw", "allow", "OpenSSH"})
			assertStepArgv(t, plan.Steps[2], []string{"sudo", "ufw", "--force", "enable"})
		})
	}
}

func TestLinux_Firewall_UfwStatusReadUsesSudo(t *testing.T) {
	fake := exec.NewFake()
	fake.AddPath("ufw", "/usr/bin/ufw")
	fake.AddCommand("sudo", []string{"-n", "ufw", "status"}, []byte("Status: active\nOpenSSH                    ALLOW       Anywhere\n"), nil)

	cfg := Config{Firewall: FirewallUfw, User: "yandy"}
	plan, err := newLinuxTest(fake, t.TempDir()).Plan(cfg, platform.Info{OS: platform.OSDebian})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(plan.Steps) != 0 {
		t.Errorf("converged ufw Plan steps = %d, want 0:\n%s", len(plan.Steps), formatSteps(plan.Steps))
	}

	// The idempotency READ must be the non-interactive `sudo -n ufw status`,
	// never a prompting `sudo ufw status` or a bare `ufw status`.
	var sawSudoStatus bool
	for _, call := range fake.CallsSnapshot() {
		if call.Name == "sudo" && len(call.Args) >= 3 && call.Args[0] == "-n" && call.Args[1] == "ufw" && call.Args[2] == "status" {
			sawSudoStatus = true
		}
		if call.Name == "sudo" && len(call.Args) >= 1 && call.Args[0] == "ufw" {
			t.Errorf("ufw status read must be non-interactive (sudo -n), got prompting: sudo %v", call.Args)
		}
		if call.Name == "ufw" {
			t.Errorf("ufw status read must keep the sudo prefix, got bare: ufw %v", call.Args)
		}
	}
	if !sawSudoStatus {
		t.Error("expected a `sudo -n ufw status` read")
	}
}

// TestLinux_Firewall_UfwUnknownStatusDefers proves that when ufw is installed
// but the status probe fails (e.g. sudo is not primed on a read-only path), the
// planner treats the state as UNKNOWN and skips the domain rather than
// fabricating an inactive firewall and re-enabling ufw on every run.
func TestLinux_Firewall_UfwUnknownStatusDefers(t *testing.T) {
	fake := exec.NewFake()
	fake.AddPath("ufw", "/usr/bin/ufw")
	// `sudo -n ufw status` intentionally NOT registered → Output errors →
	// status unknown.

	cfg := Config{Firewall: FirewallUfw, User: "yandy"}
	plan, err := newLinuxTest(fake, t.TempDir()).Plan(cfg, platform.Info{OS: platform.OSDebian})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(plan.Steps) != 0 {
		t.Fatalf("unknown-status ufw Plan steps = %d, want 0 (no fabricated enable):\n%s", len(plan.Steps), formatSteps(plan.Steps))
	}

	var firewallSkipped bool
	for _, d := range plan.Skipped {
		if d == "firewall" {
			firewallSkipped = true
		}
	}
	if !firewallSkipped {
		t.Errorf("firewall not in Skipped %v; want the domain deferred when status is unknown", plan.Skipped)
	}
}

// TestLinux_Firewall_UfwNotInstalledSkipsProbe proves that when ufw is absent
// the planner does not probe status at all (a fresh ufw is definitionally
// inactive with no rules) and still plans install+allow+enable.
func TestLinux_Firewall_UfwNotInstalledSkipsProbe(t *testing.T) {
	fake := exec.NewFake() // ufw absent, no status registered

	cfg := Config{Firewall: FirewallUfw, User: "yandy"}
	plan, err := newLinuxTest(fake, t.TempDir()).Plan(cfg, platform.Info{OS: platform.OSDebian})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(plan.Steps) != 3 {
		t.Fatalf("Plan() steps = %d, want 3 (install, allow, enable):\n%s", len(plan.Steps), formatSteps(plan.Steps))
	}
	assertStepArgv(t, plan.Steps[0], []string{"sudo", "apt-get", "install", "-y", "ufw"})
	assertStepArgv(t, plan.Steps[1], []string{"sudo", "ufw", "allow", "OpenSSH"})
	assertStepArgv(t, plan.Steps[2], []string{"sudo", "ufw", "--force", "enable"})

	for _, call := range fake.CallsSnapshot() {
		if call.Name == "sudo" && len(call.Args) >= 1 && call.Args[len(call.Args)-1] == "status" {
			t.Errorf("status probed for an uninstalled ufw: sudo %v", call.Args)
		}
	}
}

func TestLinux_Firewall_Firewalld(t *testing.T) {
	tests := []struct {
		os          platform.OS
		wantInstall []string
	}{
		{os: platform.OSRedHat, wantInstall: []string{"sudo", "dnf", "install", "-y", "firewalld"}},
		{os: platform.OSSUSE, wantInstall: []string{"sudo", "zypper", "--non-interactive", "install", "firewalld"}},
	}
	for _, tt := range tests {
		t.Run(string(tt.os), func(t *testing.T) {
			fake := exec.NewFake() // firewall-cmd absent → install, enable, allow, reload

			cfg := Config{Firewall: FirewallAuto, User: "yandy"}
			plan, err := newLinuxTest(fake, t.TempDir()).Plan(cfg, platform.Info{OS: tt.os})
			if err != nil {
				t.Fatalf("Plan() error = %v", err)
			}
			if len(plan.Steps) != 4 {
				t.Fatalf("Plan() firewalld steps = %d, want 4:\n%s", len(plan.Steps), formatSteps(plan.Steps))
			}
			assertStepArgv(t, plan.Steps[0], tt.wantInstall)
			assertStepArgv(t, plan.Steps[1], []string{"sudo", "systemctl", "enable", "--now", "firewalld"})
			assertStepArgv(t, plan.Steps[2], []string{"sudo", "firewall-cmd", "--permanent", "--add-service=ssh"})
			assertStepArgv(t, plan.Steps[3], []string{"sudo", "firewall-cmd", "--reload"})
		})
	}
}

func TestFirewallPlanNeedsSudo(t *testing.T) {
	tests := []struct {
		name string
		mode FirewallMode
		os   platform.OS
		want bool
	}{
		{name: "ufw explicit on debian", mode: FirewallUfw, os: platform.OSDebian, want: true},
		{name: "ufw explicit on arch", mode: FirewallUfw, os: platform.OSArch, want: true},
		{name: "auto on debian resolves to ufw", mode: FirewallAuto, os: platform.OSDebian, want: true},
		{name: "auto on arch resolves to ufw", mode: FirewallAuto, os: platform.OSArch, want: true},
		{name: "auto on redhat resolves to firewalld (polkit-gated probe)", mode: FirewallAuto, os: platform.OSRedHat, want: true},
		{name: "auto on suse resolves to firewalld (polkit-gated probe)", mode: FirewallAuto, os: platform.OSSUSE, want: true},
		{name: "firewalld explicit needs sudo for the query-service probe", mode: FirewallFirewalld, os: platform.OSDebian, want: true},
		{name: "off skips", mode: FirewallOff, os: platform.OSDebian, want: false},
		{name: "unset skips", mode: FirewallUnset, os: platform.OSDebian, want: false},
		{name: "macos uses socketfilterfw", mode: FirewallUfw, os: platform.OSMacOS, want: false},
		{name: "macos auto", mode: FirewallAuto, os: platform.OSMacOS, want: false},
		{name: "windows never", mode: FirewallUfw, os: platform.OSWindows, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FirewallPlanNeedsSudo(tt.mode, platform.Info{OS: tt.os})
			if got != tt.want {
				t.Errorf("FirewallPlanNeedsSudo(%q, %q) = %v, want %v", tt.mode, tt.os, got, tt.want)
			}
		})
	}
}

func TestSSHServerPlanNeedsSudo(t *testing.T) {
	tests := []struct {
		name string
		mode SSHServerMode
		os   platform.OS
		want bool
	}{
		{name: "on debian probes ufw under sudo", mode: SSHServerOn, os: platform.OSDebian, want: true},
		{name: "on arch probes ufw under sudo", mode: SSHServerOn, os: platform.OSArch, want: true},
		{name: "on redhat probes firewalld under sudo", mode: SSHServerOn, os: platform.OSRedHat, want: true},
		{name: "on suse probes firewalld under sudo", mode: SSHServerOn, os: platform.OSSUSE, want: true},
		{name: "off skips", mode: SSHServerOff, os: platform.OSRedHat, want: false},
		{name: "unset skips", mode: SSHServerUnset, os: platform.OSRedHat, want: false},
		{name: "macos never", mode: SSHServerOn, os: platform.OSMacOS, want: false},
		{name: "windows never", mode: SSHServerOn, os: platform.OSWindows, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SSHServerPlanNeedsSudo(tt.mode, platform.Info{OS: tt.os}); got != tt.want {
				t.Errorf("SSHServerPlanNeedsSudo(%q, %q) = %v, want %v", tt.mode, tt.os, got, tt.want)
			}
		})
	}
}

func TestLinux_Firewall_OffAndUnsetSkip(t *testing.T) {
	for _, mode := range []FirewallMode{FirewallUnset, FirewallOff} {
		fake := exec.NewFake()
		cfg := Config{Firewall: mode, User: "yandy"}
		plan, err := newLinuxTest(fake, t.TempDir()).Plan(cfg, platform.Info{OS: platform.OSDebian})
		if err != nil {
			t.Fatalf("Plan(firewall=%q) error = %v", mode, err)
		}
		if len(plan.Steps) != 0 {
			t.Errorf("Plan(firewall=%q) steps = %d, want 0", mode, len(plan.Steps))
		}
		if len(fake.CallsSnapshot()) != 0 {
			t.Errorf("Plan(firewall=%q) made calls, want none for skip", mode)
		}
	}
}

// seedLinuxSSHConverged seeds filesystem and exec.Fake reads for a host where
// openssh is installed, host keys exist, the ssh unit is active, and the SSH
// firewall rule is already present.
func seedLinuxSSHConverged(t *testing.T, root string, fake *exec.Fake, os platform.OS) {
	t.Helper()
	seedFile(t, root, "usr/sbin/sshd", []byte("fake\n"))
	seedFile(t, root, "etc/ssh/ssh_host_ed25519_key", []byte("fake-key\n"))
	unit := "sshd"
	if os == platform.OSDebian || os == platform.OSWSL {
		unit = "ssh"
	}
	fake.AddCommand("systemctl", []string{"is-active", unit}, []byte("active\n"), nil)
	if os == platform.OSRedHat || os == platform.OSSUSE {
		fake.AddPath("firewall-cmd", "/usr/bin/firewall-cmd")
		fake.AddCommand("sudo", []string{"-n", "firewall-cmd", "--permanent", "--query-service=ssh"}, []byte("yes\n"), nil)
		return
	}
	fake.AddPath("ufw", "/usr/bin/ufw")
	fake.AddCommand("sudo", []string{"-n", "ufw", "status"}, []byte("Status: active\n\nOpenSSH                    ALLOW       Anywhere\n"), nil)
}

// sshServerSteps returns only steps in the ssh-server domain.
func sshServerSteps(steps []Step) []Step {
	var out []Step
	for _, s := range steps {
		if s.Domain == "ssh-server" {
			out = append(out, s)
		}
	}
	return out
}

func TestLinux_SSHServer_InstallArgvPerFamily(t *testing.T) {
	tests := []struct {
		os          platform.OS
		wantInstall []string
	}{
		{os: platform.OSArch, wantInstall: []string{"sudo", "pacman", "-S", "--noconfirm", "--needed", "openssh"}},
		{os: platform.OSDebian, wantInstall: []string{"sudo", "apt-get", "install", "-y", "openssh-server"}},
		{os: platform.OSRedHat, wantInstall: []string{"sudo", "dnf", "install", "-y", "openssh-server"}},
		{os: platform.OSSUSE, wantInstall: []string{"sudo", "zypper", "--non-interactive", "install", "openssh-server"}},
	}

	for _, tt := range tests {
		t.Run(string(tt.os), func(t *testing.T) {
			root := t.TempDir()
			fake := exec.NewFake()
			unit := "sshd"
			if tt.os == platform.OSDebian || tt.os == platform.OSWSL {
				unit = "ssh"
			}
			fake.AddCommand("systemctl", []string{"is-active", unit}, []byte("inactive\n"), nil)
			if tt.os == platform.OSRedHat || tt.os == platform.OSSUSE {
				fake.AddPath("firewall-cmd", "/usr/bin/firewall-cmd")
				fake.AddCommand("sudo", []string{"-n", "firewall-cmd", "--permanent", "--query-service=ssh"}, []byte("no\n"), nil)
			}

			cfg := Config{SSHServer: SSHServerOn, User: "yandy"}
			plan, err := newLinuxTest(fake, root).Plan(cfg, platform.Info{OS: tt.os})
			if err != nil {
				t.Fatalf("Plan() error = %v", err)
			}

			steps := sshServerSteps(plan.Steps)
			if len(steps) == 0 {
				t.Fatalf("expected ssh-server steps, got none:\n%s", formatSteps(plan.Steps))
			}
			assertStepArgv(t, steps[0], tt.wantInstall)
			assertReadOnlyPlanCalls(t, fake.CallsSnapshot())
		})
	}
}

func TestLinux_SSHServer_ServiceUnitPerOS(t *testing.T) {
	tests := []struct {
		os       platform.OS
		wantUnit string
	}{
		{os: platform.OSDebian, wantUnit: "ssh"},
		{os: platform.OSWSL, wantUnit: "ssh"},
		{os: platform.OSArch, wantUnit: "sshd"},
		{os: platform.OSRedHat, wantUnit: "sshd"},
		{os: platform.OSSUSE, wantUnit: "sshd"},
	}

	for _, tt := range tests {
		t.Run(string(tt.os), func(t *testing.T) {
			root := t.TempDir()
			seedFile(t, root, "usr/sbin/sshd", []byte("fake\n"))
			seedFile(t, root, "etc/ssh/ssh_host_ed25519_key", []byte("fake-key\n"))

			fake := exec.NewFake()
			fake.AddCommand("systemctl", []string{"is-active", tt.wantUnit}, []byte("inactive\n"), nil)
			if tt.os == platform.OSRedHat || tt.os == platform.OSSUSE {
				fake.AddPath("firewall-cmd", "/usr/bin/firewall-cmd")
				fake.AddCommand("sudo", []string{"-n", "firewall-cmd", "--permanent", "--query-service=ssh"}, []byte("yes\n"), nil)
			}

			cfg := Config{SSHServer: SSHServerOn, User: "yandy"}
			plan, err := newLinuxTest(fake, root).Plan(cfg, platform.Info{OS: tt.os})
			if err != nil {
				t.Fatalf("Plan() error = %v", err)
			}

			enableIdx := stepIndex(plan.Steps, "enable")
			if enableIdx < 0 {
				t.Fatalf("expected systemctl enable step:\n%s", formatSteps(plan.Steps))
			}
			assertStepArgv(t, plan.Steps[enableIdx], []string{"sudo", "systemctl", "enable", "--now", tt.wantUnit})
			assertReadOnlyPlanCalls(t, fake.CallsSnapshot())
		})
	}
}

func TestLinux_SSHServer_HostKeyGating(t *testing.T) {
	tests := []struct {
		name       string
		seedKey    bool
		wantKeygen bool
	}{
		{name: "host key absent → ssh-keygen -A", seedKey: false, wantKeygen: true},
		{name: "host key present → no keygen", seedKey: true, wantKeygen: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			seedFile(t, root, "usr/sbin/sshd", []byte("fake\n"))
			if tt.seedKey {
				seedFile(t, root, "etc/ssh/ssh_host_ed25519_key", []byte("fake-key\n"))
			}

			fake := exec.NewFake()
			fake.AddCommand("systemctl", []string{"is-active", "ssh"}, []byte("active\n"), nil)
			fake.AddPath("ufw", "/usr/bin/ufw")
			fake.AddCommand("sudo", []string{"-n", "ufw", "status"}, []byte("Status: active\nOpenSSH                    ALLOW       Anywhere\n"), nil)

			cfg := Config{SSHServer: SSHServerOn, User: "yandy"}
			plan, err := newLinuxTest(fake, root).Plan(cfg, platform.Info{OS: platform.OSDebian})
			if err != nil {
				t.Fatalf("Plan() error = %v", err)
			}

			keygenIdx := stepIndex(plan.Steps, "ssh-keygen")
			if tt.wantKeygen && keygenIdx < 0 {
				t.Fatalf("expected ssh-keygen step:\n%s", formatSteps(plan.Steps))
			}
			if !tt.wantKeygen && keygenIdx >= 0 {
				t.Fatalf("unexpected ssh-keygen step:\n%s", formatSteps(plan.Steps))
			}
			if tt.wantKeygen {
				assertStepArgv(t, plan.Steps[keygenIdx], []string{"sudo", "ssh-keygen", "-A"})
			}
			assertReadOnlyPlanCalls(t, fake.CallsSnapshot())
		})
	}
}

func TestLinux_SSHServer_EnableWhenInactive(t *testing.T) {
	tests := []struct {
		name       string
		active     string
		wantEnable bool
	}{
		{name: "inactive → enable --now", active: "inactive", wantEnable: true},
		{name: "active → no enable", active: "active", wantEnable: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			seedFile(t, root, "usr/sbin/sshd", []byte("fake\n"))
			seedFile(t, root, "etc/ssh/ssh_host_ed25519_key", []byte("fake-key\n"))

			fake := exec.NewFake()
			fake.AddCommand("systemctl", []string{"is-active", "ssh"}, []byte(tt.active+"\n"), nil)
			fake.AddPath("ufw", "/usr/bin/ufw")
			fake.AddCommand("sudo", []string{"-n", "ufw", "status"}, []byte("Status: active\nOpenSSH                    ALLOW       Anywhere\n"), nil)

			cfg := Config{SSHServer: SSHServerOn, User: "yandy"}
			plan, err := newLinuxTest(fake, root).Plan(cfg, platform.Info{OS: platform.OSDebian})
			if err != nil {
				t.Fatalf("Plan() error = %v", err)
			}

			enableIdx := stepIndex(plan.Steps, "enable")
			if tt.wantEnable && enableIdx < 0 {
				t.Fatalf("expected systemctl enable step:\n%s", formatSteps(plan.Steps))
			}
			if !tt.wantEnable && enableIdx >= 0 {
				t.Fatalf("unexpected systemctl enable step:\n%s", formatSteps(plan.Steps))
			}
			if tt.wantEnable {
				assertStepArgv(t, plan.Steps[enableIdx], []string{"sudo", "systemctl", "enable", "--now", "ssh"})
			}
			assertReadOnlyPlanCalls(t, fake.CallsSnapshot())
		})
	}
}

func TestLinux_SSHServer_FirewallOpenPresent(t *testing.T) {
	tests := []struct {
		name     string
		os       platform.OS
		seedFake func(*exec.Fake)
		want     [][]string
	}{
		{
			name: "debian ufw without ssh rule",
			os:   platform.OSDebian,
			seedFake: func(fake *exec.Fake) {
				fake.AddPath("ufw", "/usr/bin/ufw")
				fake.AddCommand("sudo", []string{"-n", "ufw", "status"}, []byte("Status: inactive\n"), nil)
			},
			want: [][]string{{"sudo", "ufw", "allow", "OpenSSH"}},
		},
		{
			name: "redhat firewalld without ssh service",
			os:   platform.OSRedHat,
			seedFake: func(fake *exec.Fake) {
				fake.AddPath("firewall-cmd", "/usr/bin/firewall-cmd")
				fake.AddCommand("sudo", []string{"-n", "firewall-cmd", "--permanent", "--query-service=ssh"}, []byte("no\n"), nil)
			},
			want: [][]string{
				{"sudo", "firewall-cmd", "--permanent", "--add-service=ssh"},
				{"sudo", "firewall-cmd", "--reload"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			seedFile(t, root, "usr/sbin/sshd", []byte("fake\n"))
			seedFile(t, root, "etc/ssh/ssh_host_ed25519_key", []byte("fake-key\n"))

			fake := exec.NewFake()
			unit := "ssh"
			if tt.os != platform.OSDebian && tt.os != platform.OSWSL {
				unit = "sshd"
			}
			fake.AddCommand("systemctl", []string{"is-active", unit}, []byte("active\n"), nil)
			tt.seedFake(fake)

			cfg := Config{SSHServer: SSHServerOn, User: "yandy"}
			plan, err := newLinuxTest(fake, root).Plan(cfg, platform.Info{OS: tt.os})
			if err != nil {
				t.Fatalf("Plan() error = %v", err)
			}

			steps := sshServerSteps(plan.Steps)
			if len(steps) != len(tt.want) {
				t.Fatalf("ssh-server steps = %d, want %d:\n%s", len(steps), len(tt.want), formatSteps(steps))
			}
			for i, wantArgv := range tt.want {
				assertStepArgv(t, steps[i], wantArgv)
			}
			assertReadOnlyPlanCalls(t, fake.CallsSnapshot())
		})
	}
}

func TestLinux_SSHServer_Converged(t *testing.T) {
	for _, os := range []platform.OS{platform.OSDebian, platform.OSArch, platform.OSRedHat} {
		t.Run(string(os), func(t *testing.T) {
			root := t.TempDir()
			fake := exec.NewFake()
			seedLinuxSSHConverged(t, root, fake, os)

			cfg := Config{SSHServer: SSHServerOn, User: "yandy"}
			plan, err := newLinuxTest(fake, root).Plan(cfg, platform.Info{OS: os})
			if err != nil {
				t.Fatalf("Plan() error = %v", err)
			}
			if len(plan.Steps) != 0 {
				t.Fatalf("Plan() steps = %d, want 0 (converged host):\n%s", len(plan.Steps), formatSteps(plan.Steps))
			}
			if !containsStr(plan.Skipped, "ssh-server") {
				t.Errorf("Plan().Skipped = %v, want to include ssh-server for converged host", plan.Skipped)
			}
			assertReadOnlyPlanCalls(t, fake.CallsSnapshot())
		})
	}
}

func TestLinux_SSHServer_OffAndUnsetSkip(t *testing.T) {
	for _, mode := range []SSHServerMode{SSHServerUnset, SSHServerOff} {
		t.Run(string(mode), func(t *testing.T) {
			fake := exec.NewFake()
			cfg := Config{SSHServer: mode, User: "yandy"}
			plan, err := newLinuxTest(fake, t.TempDir()).Plan(cfg, platform.Info{OS: platform.OSDebian})
			if err != nil {
				t.Fatalf("Plan(ssh-server=%q) error = %v", mode, err)
			}
			if len(plan.Steps) != 0 {
				t.Errorf("Plan(ssh-server=%q) steps = %d, want 0:\n%s", mode, len(plan.Steps), formatSteps(plan.Steps))
			}
			if !containsStr(plan.Skipped, "ssh-server") {
				t.Errorf("Plan(ssh-server=%q).Skipped = %v, want ssh-server", mode, plan.Skipped)
			}
			if len(fake.CallsSnapshot()) != 0 {
				t.Errorf("Plan(ssh-server=%q) made calls, want none for skip", mode)
			}
		})
	}
}

func TestLinux_Timezone_InvalidZoneErrors(t *testing.T) {
	root := t.TempDir() // no zoneinfo seeded

	fake := exec.NewFake()
	cfg := Config{Timezone: "Mars/Olympus", User: "yandy"}
	plan, err := newLinuxTest(fake, root).Plan(cfg, platform.Info{OS: platform.OSDebian})
	if err == nil {
		t.Fatalf("Plan() error = nil, want error for invalid timezone; steps:\n%s", formatSteps(plan.Steps))
	}
	if !strings.Contains(err.Error(), "timezone") {
		t.Errorf("Plan() error = %v, want a timezone error", err)
	}
	for _, call := range fake.CallsSnapshot() {
		if call.Name == "sudo" {
			t.Errorf("no mutating command should run for an invalid timezone, got: sudo %v", call.Args)
		}
	}
}

// formatSteps renders steps for readable test failure messages.
func formatSteps(steps []Step) string {
	var b strings.Builder
	for i, s := range steps {
		if s.File != nil {
			fmt.Fprintf(&b, "  [%d] file %s (backup=%v)\n", i, s.File.Dest, s.File.Backup)
			continue
		}
		fmt.Fprintf(&b, "  [%d] %v\n", i, s.Argv)
	}
	return b.String()
}
