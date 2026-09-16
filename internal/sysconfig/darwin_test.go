package sysconfig

import (
	"errors"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/platform"
)

func newDarwinTest(fake *exec.Fake, root string) *darwinConfigurer {
	return newDarwinConfigurer(fake, NewOSFileSystem(root))
}

func containsStr(items []string, want string) bool {
	for _, s := range items {
		if s == want {
			return true
		}
	}
	return false
}

func TestDarwin_Plan_AllDomains(t *testing.T) {
	root := t.TempDir()
	seedFile(t, root, "etc/shells", []byte("/bin/bash\n/bin/zsh\n"))

	fake := exec.NewFake()
	fake.AddCommand("scutil", []string{"--get", "ComputerName"}, []byte("oldmac\n"), nil)
	fake.AddCommand("scutil", []string{"--get", "LocalHostName"}, []byte("oldmac\n"), nil)
	fake.AddCommand("scutil", []string{"--get", "HostName"}, nil, errors.New("HostName: not set"))
	fake.AddCommand("systemsetup", []string{"-gettimezone"}, []byte("Time Zone: UTC\n"), nil)
	fake.AddPath("zsh", "/opt/homebrew/bin/zsh")
	fake.AddCommand("dscl", []string{".", "-read", "/Users/yandy", "UserShell"}, []byte("UserShell: /bin/bash\n"), nil)
	fake.AddCommand(socketfilterfwPath, []string{"--getglobalstate"}, []byte("Firewall is disabled. (State = 0)\n"), nil)

	cfg := Config{
		Hostname: "web01",
		Timezone: "America/New_York",
		Locale:   "en_US.UTF-8",
		Shell:    "zsh",
		Firewall: FirewallAuto,
		User:     "yandy",
	}

	plan, err := newDarwinTest(fake, root).Plan(cfg, platform.Info{OS: platform.OSMacOS})
	if err != nil {
		t.Fatalf("Plan() error = %v, want nil", err)
	}
	if len(plan.Steps) != 7 {
		t.Fatalf("Plan() steps = %d, want 7:\n%s", len(plan.Steps), formatSteps(plan.Steps))
	}

	assertStepArgv(t, plan.Steps[0], []string{"sudo", "scutil", "--set", "ComputerName", "web01"})
	assertStepArgv(t, plan.Steps[1], []string{"sudo", "scutil", "--set", "LocalHostName", "web01"})
	assertStepArgv(t, plan.Steps[2], []string{"sudo", "scutil", "--set", "HostName", "web01"})
	assertStepArgv(t, plan.Steps[3], []string{"sudo", "systemsetup", "-settimezone", "America/New_York"})
	assertFileStep(t, plan.Steps[4], "/etc/shells", true, "/opt/homebrew/bin/zsh")
	assertStepArgv(t, plan.Steps[5], []string{"sudo", "chsh", "-s", "/opt/homebrew/bin/zsh", "yandy"})
	assertStepArgv(t, plan.Steps[6], []string{"sudo", socketfilterfwPath, "--setglobalstate", "on"})

	if !containsStr(plan.Skipped, "locale") {
		t.Errorf("Plan().Skipped = %v, want to include locale (macOS no-op)", plan.Skipped)
	}
	assertReadOnlyPlanCalls(t, fake.CallsSnapshot())
}

func TestDarwin_Plan_Idempotent(t *testing.T) {
	root := t.TempDir()
	seedFile(t, root, "etc/shells", []byte("/bin/bash\n/opt/homebrew/bin/zsh\n"))

	fake := exec.NewFake()
	for _, key := range []string{"ComputerName", "LocalHostName", "HostName"} {
		fake.AddCommand("scutil", []string{"--get", key}, []byte("web01\n"), nil)
	}
	fake.AddCommand("systemsetup", []string{"-gettimezone"}, []byte("Time Zone: America/New_York\n"), nil)
	fake.AddPath("zsh", "/opt/homebrew/bin/zsh")
	fake.AddCommand("dscl", []string{".", "-read", "/Users/yandy", "UserShell"}, []byte("UserShell: /opt/homebrew/bin/zsh\n"), nil)
	fake.AddCommand(socketfilterfwPath, []string{"--getglobalstate"}, []byte("Firewall is enabled. (State = 1)\n"), nil)

	cfg := Config{
		Hostname: "web01",
		Timezone: "America/New_York",
		Locale:   "en_US.UTF-8",
		Shell:    "zsh",
		Firewall: FirewallAuto,
		User:     "yandy",
	}

	plan, err := newDarwinTest(fake, root).Plan(cfg, platform.Info{OS: platform.OSMacOS})
	if err != nil {
		t.Fatalf("Plan() error = %v, want nil", err)
	}
	if len(plan.Steps) != 0 {
		t.Fatalf("Plan() steps = %d, want 0 (converged macOS host):\n%s", len(plan.Steps), formatSteps(plan.Steps))
	}
	assertReadOnlyPlanCalls(t, fake.CallsSnapshot())
}

func TestDarwin_Hostname_HostNameUnsetTreatedAsUnset(t *testing.T) {
	fake := exec.NewFake()
	fake.AddCommand("scutil", []string{"--get", "ComputerName"}, []byte("web01\n"), nil)
	fake.AddCommand("scutil", []string{"--get", "LocalHostName"}, []byte("web01\n"), nil)
	fake.AddCommand("scutil", []string{"--get", "HostName"}, nil, errors.New("HostName: not set"))

	cfg := Config{Hostname: "web01", User: "yandy"}
	plan, err := newDarwinTest(fake, t.TempDir()).Plan(cfg, platform.Info{OS: platform.OSMacOS})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	var hostnameSteps []Step
	for _, s := range plan.Steps {
		if s.Domain == "hostname" {
			hostnameSteps = append(hostnameSteps, s)
		}
	}
	if len(hostnameSteps) != 1 {
		t.Fatalf("hostname steps = %d, want 1 (only HostName differs):\n%s", len(hostnameSteps), formatSteps(hostnameSteps))
	}
	assertStepArgv(t, hostnameSteps[0], []string{"sudo", "scutil", "--set", "HostName", "web01"})
}

func TestDarwin_Firewall_socketfilterfw(t *testing.T) {
	tests := []struct {
		name  string
		mode  FirewallMode
		state string
		want  int
	}{
		{name: "auto enables when off", mode: FirewallAuto, state: "Firewall is disabled. (State = 0)\n", want: 1},
		{name: "ufw resolves to socketfilterfw", mode: FirewallUfw, state: "Firewall is disabled. (State = 0)\n", want: 1},
		{name: "firewalld resolves to socketfilterfw", mode: FirewallFirewalld, state: "Firewall is disabled. (State = 0)\n", want: 1},
		{name: "idempotent when already on", mode: FirewallAuto, state: "Firewall is enabled. (State = 1)\n", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := exec.NewFake()
			fake.AddCommand(socketfilterfwPath, []string{"--getglobalstate"}, []byte(tt.state), nil)

			cfg := Config{Firewall: tt.mode, User: "yandy"}
			plan, err := newDarwinTest(fake, t.TempDir()).Plan(cfg, platform.Info{OS: platform.OSMacOS})
			if err != nil {
				t.Fatalf("Plan() error = %v", err)
			}

			var fwSteps []Step
			for _, s := range plan.Steps {
				if s.Domain == "firewall" {
					fwSteps = append(fwSteps, s)
				}
			}
			if len(fwSteps) != tt.want {
				t.Fatalf("firewall steps = %d, want %d:\n%s", len(fwSteps), tt.want, formatSteps(fwSteps))
			}
			if tt.want == 1 {
				assertStepArgv(t, fwSteps[0], []string{"sudo", socketfilterfwPath, "--setglobalstate", "on"})
			}
		})
	}
}

// TestDarwin_Shell_EmptyUserSkips proves that when no real user resolves
// (cfg.User == "") the shell domain is skipped entirely: no chsh Step and no
// /etc/shells file step are emitted, so the documented "skip" guarantee holds
// instead of running `chsh -s <path> ""`.
func TestDarwin_Shell_EmptyUserSkips(t *testing.T) {
	root := t.TempDir()
	seedFile(t, root, "etc/shells", []byte("/bin/bash\n"))

	fake := exec.NewFake()
	fake.AddPath("zsh", "/opt/homebrew/bin/zsh")

	cfg := Config{Shell: "zsh", User: ""}
	plan, err := newDarwinTest(fake, root).Plan(cfg, platform.Info{OS: platform.OSMacOS})
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

func TestDarwin_SSHServer_NoOp(t *testing.T) {
	fake := exec.NewFake()

	cfg := Config{SSHServer: SSHServerOn, User: "yandy"}
	plan, err := newDarwinTest(fake, t.TempDir()).Plan(cfg, platform.Info{OS: platform.OSMacOS})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	if len(plan.Steps) != 0 {
		t.Fatalf("Plan() steps = %d, want 0 (macOS ssh-server no-op):\n%s", len(plan.Steps), formatSteps(plan.Steps))
	}
	if !containsStr(plan.Skipped, "ssh-server") {
		t.Errorf("Plan().Skipped = %v, want to include ssh-server", plan.Skipped)
	}
	for _, call := range fake.CallsSnapshot() {
		switch call.Name {
		case "systemctl", "ssh-keygen", "firewall-cmd", "ufw":
			t.Errorf("macOS probed a Linux ssh-server tool: %s %v", call.Name, call.Args)
		}
	}
	assertReadOnlyPlanCalls(t, fake.CallsSnapshot())
}

func TestDarwin_Locale_NoOp(t *testing.T) {
	fake := exec.NewFake()

	cfg := Config{Locale: "en_US.UTF-8", User: "yandy"}
	plan, err := newDarwinTest(fake, t.TempDir()).Plan(cfg, platform.Info{OS: platform.OSMacOS})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	for _, s := range plan.Steps {
		if s.Domain == "locale" {
			t.Errorf("macOS emitted a locale step, want none: %v", s.Argv)
		}
	}
	if !containsStr(plan.Skipped, "locale") {
		t.Errorf("Plan().Skipped = %v, want to include locale", plan.Skipped)
	}
	// No localectl/locale-gen probing on macOS.
	for _, call := range fake.CallsSnapshot() {
		if call.Name == "localectl" || call.Name == "locale-gen" || (call.Name == "locale" && containsStr(call.Args, "-a")) {
			t.Errorf("macOS probed a Linux locale tool: %s %v", call.Name, call.Args)
		}
	}
}
