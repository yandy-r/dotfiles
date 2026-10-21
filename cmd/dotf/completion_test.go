package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	dotfiles "github.com/yandy-r/dotfiles"
	"github.com/yandy-r/dotfiles/internal/manifest"
)

// completionCases maps each supported shell to a substring that must appear in
// its generated script — a cheap registration-line smoke check.
var completionMarkers = map[string]string{
	"bash": "complete -F _dotf dotf",
	"zsh":  "#compdef dotf",
	"fish": "complete -c dotf",
}

func TestRun_Completion_PrintsScript(t *testing.T) {
	for shell, marker := range completionMarkers {
		t.Run(shell, func(t *testing.T) {
			stdout, stderr, code := captureOutput(t, func() int {
				return run([]string{"completion", shell})
			})
			if code != 0 {
				t.Fatalf("run([completion %s]) code = %d, want 0 (stderr=%q)", shell, code, stderr)
			}
			if stderr != "" {
				t.Errorf("run([completion %s]) stderr = %q, want empty", shell, stderr)
			}
			if !strings.Contains(stdout, marker) {
				t.Errorf("run([completion %s]) stdout missing %q", shell, marker)
			}
			// Every command name must appear in every script.
			for _, name := range buildCompletionModel().commandNames() {
				if !strings.Contains(stdout, name) {
					t.Errorf("run([completion %s]) stdout missing command %q", shell, name)
				}
			}
		})
	}
}

func TestRun_Completion_UnsupportedShell(t *testing.T) {
	stdout, stderr, code := captureOutput(t, func() int {
		return run([]string{"completion", "tcsh"})
	})
	if code != 2 {
		t.Errorf("run([completion tcsh]) code = %d, want 2", code)
	}
	if stdout != "" {
		t.Errorf("run([completion tcsh]) stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "unsupported shell") {
		t.Errorf("run([completion tcsh]) stderr = %q, want it to mention the unsupported shell", stderr)
	}
}

func TestRun_Completion_DetectsShellFromEnv(t *testing.T) {
	t.Setenv("SHELL", "/usr/bin/zsh")
	stdout, _, code := captureOutput(t, func() int { return run([]string{"completion"}) })
	if code != 0 {
		t.Fatalf("run([completion]) with SHELL=zsh code = %d, want 0", code)
	}
	if !strings.Contains(stdout, completionMarkers["zsh"]) {
		t.Errorf("run([completion]) with SHELL=zsh did not emit the zsh script")
	}
}

func TestRun_Completion_UndetectableShell(t *testing.T) {
	t.Setenv("SHELL", "/usr/bin/nonsense")
	_, stderr, code := captureOutput(t, func() int { return run([]string{"completion"}) })
	if code != 2 {
		t.Errorf("run([completion]) with unknown SHELL code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "cannot detect shell") {
		t.Errorf("run([completion]) stderr = %q, want it to mention detection failure", stderr)
	}
}

func TestSplitCompletionArgs(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		wantShell string
		wantFlags []string
		wantErr   bool
	}{
		{"shell only", []string{"zsh"}, "zsh", nil, false},
		{"flags only", []string{"--install"}, "", []string{"--install"}, false},
		{"shell then flags", []string{"zsh", "--install", "--force"}, "zsh", []string{"--install", "--force"}, false},
		{"flags then shell", []string{"--install", "zsh"}, "zsh", []string{"--install"}, false},
		{"interleaved", []string{"--install", "bash", "--force"}, "bash", []string{"--install", "--force"}, false},
		{"terminator ignored", []string{"--", "fish"}, "fish", nil, false},
		{"two positionals", []string{"bash", "zsh"}, "", nil, true},
		{"empty", nil, "", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			shell, flags, err := splitCompletionArgs(tc.args)
			if (err != nil) != tc.wantErr {
				t.Fatalf("splitCompletionArgs(%v) err = %v, wantErr = %v", tc.args, err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if shell != tc.wantShell {
				t.Errorf("shell = %q, want %q", shell, tc.wantShell)
			}
			if !slices.Equal(flags, tc.wantFlags) {
				t.Errorf("flags = %v, want %v", flags, tc.wantFlags)
			}
		})
	}
}

func TestDetectShell(t *testing.T) {
	cases := map[string]string{
		"/bin/bash":           "bash",
		"/usr/bin/zsh":        "zsh",
		"/usr/local/bin/fish": "fish",
		"/bin/sh":             "",
		"":                    "",
	}
	for shellEnv, want := range cases {
		t.Run(shellEnv, func(t *testing.T) {
			t.Setenv("SHELL", shellEnv)
			if got := detectShell(); got != want {
				t.Errorf("detectShell() with SHELL=%q = %q, want %q", shellEnv, got, want)
			}
		})
	}
}

func TestCompletionInstallPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "cfg"))

	cases := map[string]string{
		"bash": filepath.Join(home, "data", "bash-completion", "completions", "dotf"),
		"zsh":  filepath.Join(home, "data", "zsh", "site-functions", "_dotf"),
		"fish": filepath.Join(home, "cfg", "fish", "completions", "dotf.fish"),
	}
	for shell, want := range cases {
		got, err := completionInstallPath(shell)
		if err != nil {
			t.Fatalf("completionInstallPath(%q) error = %v", shell, err)
		}
		if got != want {
			t.Errorf("completionInstallPath(%q) = %q, want %q", shell, got, want)
		}
	}
}

// TestCompletionInstallPath_XDGFallback verifies the ~/.local/share and
// ~/.config defaults kick in when the XDG vars are unset.
func TestCompletionInstallPath_XDGFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	bash, _ := completionInstallPath("bash")
	if want := filepath.Join(home, ".local", "share", "bash-completion", "completions", "dotf"); bash != want {
		t.Errorf("bash path = %q, want %q", bash, want)
	}
	fish, _ := completionInstallPath("fish")
	if want := filepath.Join(home, ".config", "fish", "completions", "dotf.fish"); fish != want {
		t.Errorf("fish path = %q, want %q", fish, want)
	}
}

func TestRun_Completion_Install(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))

	path := filepath.Join(home, "data", "zsh", "site-functions", "_dotf")

	// First install writes the file.
	_, stderr, code := captureOutput(t, func() int {
		return run([]string{"completion", "zsh", "--install"})
	})
	if code != 0 {
		t.Fatalf("first install code = %d, want 0 (stderr=%q)", code, stderr)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading installed script: %v", err)
	}
	if !strings.Contains(string(got), completionMarkers["zsh"]) {
		t.Errorf("installed script missing the zsh marker")
	}
	if !strings.Contains(stderr, path) {
		t.Errorf("install stderr = %q, want it to name the target path", stderr)
	}

	// Second install without --force refuses and leaves the file intact.
	if err := os.WriteFile(path, []byte("SENTINEL"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, code = captureOutput(t, func() int {
		return run([]string{"completion", "zsh", "--install"})
	})
	if code != 1 {
		t.Errorf("re-install without --force code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "--force") {
		t.Errorf("re-install stderr = %q, want it to suggest --force", stderr)
	}
	if data, _ := os.ReadFile(path); string(data) != "SENTINEL" {
		t.Errorf("re-install without --force overwrote the file")
	}

	// --force overwrites.
	_, _, code = captureOutput(t, func() int {
		return run([]string{"completion", "zsh", "--install", "--force"})
	})
	if code != 0 {
		t.Errorf("re-install with --force code = %d, want 0", code)
	}
	if data, _ := os.ReadFile(path); string(data) == "SENTINEL" {
		t.Errorf("--force did not overwrite the file")
	}
}

func TestRun_Completion_InstallDryRun(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	path := filepath.Join(home, "data", "zsh", "site-functions", "_dotf")

	stdout, _, code := captureOutput(t, func() int {
		return run([]string{"completion", "zsh", "--install", "--dry-run"})
	})
	if code != 0 {
		t.Fatalf("dry-run install code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "dry-run=true") || !strings.Contains(stdout, "completion.path="+path) {
		t.Errorf("dry-run stdout = %q, want dry-run=true and the target path", stdout)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("dry-run wrote a file at %q, want none", path)
	}
}

// TestRun_Completion_InstallOrderIndependent proves the shell positional and the
// flags may appear in either order.
func TestRun_Completion_InstallOrderIndependent(t *testing.T) {
	for _, args := range [][]string{
		{"completion", "--install", "fish"},
		{"completion", "fish", "--install"},
	} {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "cfg"))
		_, stderr, code := captureOutput(t, func() int { return run(args) })
		if code != 0 {
			t.Fatalf("run(%v) code = %d, want 0 (stderr=%q)", args, code, stderr)
		}
		path := filepath.Join(home, "cfg", "fish", "completions", "dotf.fish")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("run(%v) did not install to %q: %v", args, path, err)
		}
	}
}

// TestBuildCompletionModel_MatchesRegistry is the model drift guard: the model's
// commands must equal the help registry, and known flag surfaces must be present
// with the right value/repeatable classification.
func TestBuildCompletionModel_MatchesRegistry(t *testing.T) {
	m := buildCompletionModel()
	if !slices.Equal(m.commandNames(), allRegistryNames) {
		t.Fatalf("model commands = %v, want %v", m.commandNames(), allRegistryNames)
	}

	// completion's own flags come from setCompletionFlags.
	comp := m.command("completion")
	var compFlags []string
	for _, f := range comp.Flags {
		compFlags = append(compFlags, f.Name)
		if f.TakesValue {
			t.Errorf("completion flag %q should be boolean", f.Name)
		}
	}
	slices.Sort(compFlags)
	if want := []string{"dry-run", "force", "install"}; !slices.Equal(compFlags, want) {
		t.Errorf("completion flags = %v, want %v", compFlags, want)
	}

	// --manifest is a value flag; --intent is a repeatable value flag.
	assertFlag := func(cmd, name string, takesValue, repeatable bool) {
		t.Helper()
		for _, f := range m.command(cmd).Flags {
			if f.Name == name {
				if f.TakesValue != takesValue || f.Repeatable != repeatable {
					t.Errorf("%s --%s: TakesValue=%v Repeatable=%v, want %v/%v",
						cmd, name, f.TakesValue, f.Repeatable, takesValue, repeatable)
				}
				return
			}
		}
		t.Errorf("%s has no --%s flag", cmd, name)
	}
	assertFlag("install", "manifest", true, false)
	assertFlag("install", "intent", true, true)
	assertFlag("install", "dry-run", false, false)
	assertFlag("install", "force", false, false)
	// #55 lifecycle: --update/--remove are booleans that auto-derive into
	// completions (install stays a flag-only command; the positional name set is
	// large + overlay-dynamic, so no baked candidates).
	assertFlag("install", "update", false, false)
	assertFlag("install", "remove", false, false)
	// #42 package-level filters: repeatable value flags on install and apply.
	assertFlag("install", "only", true, true)
	assertFlag("install", "skip", true, true)
	assertFlag("apply", "only", true, true)
	assertFlag("apply", "skip", true, true)

	// #46 lock: --manifest/--lock are value flags; --only/--skip/--platform are
	// repeatable value flags; --check/--dry-run/--json are boolean.
	assertFlag("lock", "manifest", true, false)
	assertFlag("lock", "lock", true, false)
	assertFlag("lock", "only", true, true)
	assertFlag("lock", "skip", true, true)
	assertFlag("lock", "platform", true, true)
	assertFlag("lock", "check", false, false)
	assertFlag("lock", "dry-run", false, false)
	assertFlag("lock", "json", false, false)

	// #53 self-update: flag-only surface, four booleans, no positional.
	assertFlag("self-update", "check", false, false)
	assertFlag("self-update", "force", false, false)
	assertFlag("self-update", "dry-run", false, false)
	assertFlag("self-update", "json", false, false)

	// #55 setup: --manifest value flag; --dry-run/--yes booleans. Its <domain>
	// positional is baked separately (completionModules), not a flag.
	assertFlag("setup", "manifest", true, false)
	assertFlag("setup", "dry-run", false, false)
	assertFlag("setup", "yes", false, false)

	// The read-only reporting commands share --manifest (value) and --json
	// (boolean) via setReadFlags, so completions offer --json under each. #52
	// doctor uses the same setReadFlags surface.
	for _, cmd := range []string{"status", "diff", "plan", "doctor"} {
		assertFlag(cmd, "manifest", true, false)
		assertFlag(cmd, "json", false, false)
	}

	// #52 doctor: read-only diagnostic sharing setReadFlags + --no-user-config.
	// #88 adds doctor's own --verbose (boolean) via setDoctorFlags.
	assertFlag("doctor", "manifest", true, false)
	assertFlag("doctor", "json", false, false)
	assertFlag("doctor", "no-user-config", false, false)
	assertFlag("doctor", "verbose", false, false)

	// #48 governance: status/diff additionally expose --counts (boolean) via
	// setDriftFlags; plan keeps setReadFlags and deliberately has no --counts.
	assertFlag("status", "counts", false, false)
	assertFlag("diff", "counts", false, false)
	for _, f := range m.command("plan").Flags {
		if f.Name == "counts" {
			t.Errorf("plan should not expose --counts (it uses setReadFlags, not setDriftFlags)")
		}
	}

	// #49 XDG overlay: --no-user-config (boolean, non-value, non-repeatable) on
	// every manifest-consuming command, and NOT on the commands that never
	// resolve the manifest (link/completion/version/platform).
	hasFlag := func(cmd, name string) bool {
		for _, f := range m.command(cmd).Flags {
			if f.Name == name {
				return true
			}
		}
		return false
	}
	for _, cmd := range []string{"install", "plan", "status", "diff", "doctor", "apply", "sysconfig", "lock", "setup"} {
		assertFlag(cmd, "no-user-config", false, false)
	}
	for _, cmd := range []string{"link", "completion", "version", "platform", "self-update"} {
		if hasFlag(cmd, "no-user-config") {
			t.Errorf("%s should not expose --no-user-config (it does not resolve the manifest overlay)", cmd)
		}
	}
}

// TestCompletionModel_ValueClassification checks how value flags are classified
// for argument completion: an enum documented in the usage becomes a value set,
// a "path" flag becomes file completion, and a freeform flag gets neither.
func TestCompletionModel_ValueClassification(t *testing.T) {
	m := buildCompletionModel()
	flagOf := func(cmd, name string) completionFlag {
		t.Helper()
		for _, f := range m.command(cmd).Flags {
			if f.Name == name {
				return f
			}
		}
		t.Fatalf("%s has no --%s flag", cmd, name)
		return completionFlag{}
	}

	firewall := flagOf("sysconfig", "firewall")
	if firewall.kind() != valueEnum || !slices.Equal(firewall.Values, []string{"ufw", "firewalld", "auto", "off"}) {
		t.Errorf("sysconfig --firewall: kind=%d values=%v, want enum [ufw firewalld auto off]", firewall.kind(), firewall.Values)
	}
	sshServer := flagOf("sysconfig", "ssh-server")
	if sshServer.kind() != valueEnum || !slices.Equal(sshServer.Values, []string{"on", "off"}) {
		t.Errorf("sysconfig --ssh-server: kind=%d values=%v, want enum [on off]", sshServer.kind(), sshServer.Values)
	}
	homebrew := flagOf("apply", "homebrew")
	if homebrew.kind() != valueEnum || !slices.Equal(homebrew.Values, []string{"true", "false"}) {
		t.Errorf("apply --homebrew: kind=%d values=%v, want enum [true false]", homebrew.kind(), homebrew.Values)
	}
	if got := flagOf("install", "manifest"); got.kind() != valuePath {
		t.Errorf("install --manifest kind = %d, want valuePath", got.kind())
	}
	if got := flagOf("install", "intent"); got.kind() != valueNone {
		t.Errorf("install --intent kind = %d, want valueNone (freeform)", got.kind())
	}
}

func TestParseEnumUsage(t *testing.T) {
	cases := map[string][]string{
		"firewall backend to enable: ufw|firewalld|auto|off": {"ufw", "firewalld", "auto", "off"},
		"Homebrew tristate: true|false ('' = OS default)":    {"true", "false"},
		"path to a manifest TOML file":                       nil,
		"set the IANA timezone (e.g. America/New_York)":      nil,
	}
	for usage, want := range cases {
		if got := parseEnumUsage(usage); !slices.Equal(got, want) {
			t.Errorf("parseEnumUsage(%q) = %v, want %v", usage, got, want)
		}
	}
}

// TestXDGHome_IgnoresRelative verifies the XDG spec rule that a relative env
// value is ignored in favor of the default (a relative install path no shell
// would auto-load).
func TestXDGHome_IgnoresRelative(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_DATA_HOME", "relative/not-absolute")
	t.Setenv("XDG_CONFIG_HOME", "also/relative")
	if got, want := xdgDataHome(home), filepath.Join(home, ".local", "share"); got != want {
		t.Errorf("xdgDataHome with relative env = %q, want %q", got, want)
	}
	if got, want := xdgConfigHome(home), filepath.Join(home, ".config"); got != want {
		t.Errorf("xdgConfigHome with relative env = %q, want %q", got, want)
	}
}

// TestRun_Completion_InstallReplacesSymlink guards against writing through a
// symlink at the target path: --force must replace the link itself, never
// clobber the file it points at.
func TestRun_Completion_InstallReplacesSymlink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))

	target := filepath.Join(home, "data", "zsh", "site-functions", "_dotf")
	external := filepath.Join(home, "external", "_dotf_src")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(external), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(external, []byte("ORIGINAL"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, target); err != nil {
		t.Fatal(err)
	}

	_, _, code := captureOutput(t, func() int {
		return run([]string{"completion", "zsh", "--install", "--force"})
	})
	if code != 0 {
		t.Fatalf("install --force over symlink code = %d, want 0", code)
	}
	if data, _ := os.ReadFile(external); string(data) != "ORIGINAL" {
		t.Errorf("--force wrote through the symlink and clobbered the target file: %q", data)
	}
	info, err := os.Lstat(target)
	if err != nil {
		t.Fatalf("lstat target: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Errorf("target is still a symlink; want it replaced by a regular file")
	}
}

// TestCompletionModules_MatchManifest is the setup-positional drift guard: the
// baked completionModules must equal the embedded manifest's [[module]] names,
// so the completion candidates can never drift from the real module set (the
// same idea as the command registry drift guard).
func TestCompletionModules_MatchManifest(t *testing.T) {
	m, err := manifest.Resolve("", "", dotfiles.ManifestFS, manifest.ResolveOptions{})
	if err != nil {
		t.Fatalf("resolve embedded manifest: %v", err)
	}
	got := make([]string, 0, len(m.Modules))
	for _, mod := range m.Modules {
		got = append(got, mod.Name)
	}
	want := append([]string(nil), completionModules...)
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("embedded manifest modules = %v, completionModules = %v; keep them in sync", got, want)
	}
}

// TestGeneratedScripts_SetupPositional asserts each generated script bakes the
// setup <domain> module positional, mirroring the completion shell positional.
func TestGeneratedScripts_SetupPositional(t *testing.T) {
	m := buildCompletionModel()
	modules := strings.Join(completionModules, " ")

	bash := generateCompletion("bash", m)
	if !strings.Contains(bash, "setup)") || !strings.Contains(bash, modules) {
		t.Errorf("bash completion missing the setup module positional (%q)", modules)
	}

	zsh := generateCompletion("zsh", m)
	if !strings.Contains(zsh, "1:domain:("+modules+")") {
		t.Errorf("zsh completion missing the setup domain positional (%q)", modules)
	}

	fish := generateCompletion("fish", m)
	if !strings.Contains(fish, "__fish_seen_subcommand_from setup") || !strings.Contains(fish, "-a '"+modules+"'") {
		t.Errorf("fish completion missing the setup module positional (%q)", modules)
	}
}

// TestGeneratedScripts_BackupPositional asserts each generated script bakes the
// `backup <verb>` positional and backup's prune/restore flags.
func TestGeneratedScripts_BackupPositional(t *testing.T) {
	m := buildCompletionModel()
	verbs := strings.Join(completionBackupVerbs, " ")

	bash := generateCompletion("bash", m)
	if !strings.Contains(bash, "backup)") || !strings.Contains(bash, verbs) {
		t.Errorf("bash completion missing the backup verb positional (%q)", verbs)
	}
	for _, flag := range []string{"--keep", "--older-than", "--force"} {
		if !strings.Contains(bash, flag) {
			t.Errorf("bash completion missing backup's flag word %s", flag)
		}
	}

	zsh := generateCompletion("zsh", m)
	if !strings.Contains(zsh, "1:verb:("+verbs+")") {
		t.Errorf("zsh completion missing the backup verb positional (%q)", verbs)
	}

	fish := generateCompletion("fish", m)
	if !strings.Contains(fish, "__fish_seen_subcommand_from backup") || !strings.Contains(fish, "-a '"+verbs+"'") {
		t.Errorf("fish completion missing the backup verb positional (%q)", verbs)
	}
}

// TestGeneratedScripts_PlasmaPositional asserts each generated script bakes the
// `plasma <verb>` positional, and that the pre-existing completion/help/setup
// positional rules now exclude plasma — __fish_seen_subcommand_from matches a
// token ANYWHERE on the line, so without the exclusions `dotf plasma <TAB>`
// would also offer shell names, command names, and module names.
func TestGeneratedScripts_PlasmaPositional(t *testing.T) {
	m := buildCompletionModel()
	verbs := strings.Join(completionPlasmaVerbs, " ")

	bash := generateCompletion("bash", m)
	if !strings.Contains(bash, "plasma)") || !strings.Contains(bash, verbs) {
		t.Errorf("bash completion missing the plasma verb positional (%q)", verbs)
	}
	// --unlink and --show-volatile are the two newest plasma flags and exactly
	// the ones a generator that hard-codes a flag list instead of deriving it
	// from setPlasmaFlags would silently omit, so they are asserted alongside
	// the older --recommended.
	for _, flag := range []string{"--recommended", "--show-volatile", "--unlink"} {
		if !strings.Contains(bash, flag) {
			t.Errorf("bash completion missing plasma's flag word %s", flag)
		}
	}

	zsh := generateCompletion("zsh", m)
	if !strings.Contains(zsh, "1:verb:("+verbs+")") {
		t.Errorf("zsh completion missing the plasma verb positional (%q)", verbs)
	}

	fish := generateCompletion("fish", m)
	if !strings.Contains(fish, "__fish_seen_subcommand_from plasma") || !strings.Contains(fish, "-a '"+verbs+"'") {
		t.Errorf("fish completion missing the plasma verb positional (%q)", verbs)
	}
	for _, rule := range []string{"# completion: shell argument", "# help: command argument", "# setup: domain argument"} {
		line := fishRuleLine(t, fish, rule)
		if !strings.Contains(line, "not __fish_seen_subcommand_from plasma") {
			t.Errorf("fish rule %q does not exclude plasma:\n%s", rule, line)
		}
	}
}

// fishRuleLine returns the generated `complete` line that follows the given
// comment header, so a rule's guard clause can be asserted in isolation.
func fishRuleLine(t *testing.T, script, header string) string {
	t.Helper()
	lines := strings.Split(script, "\n")
	for i, l := range lines {
		if l == header && i+1 < len(lines) {
			return lines[i+1]
		}
	}
	t.Fatalf("fish completion has no %q section", header)
	return ""
}

// TestGeneratedScripts_ShellSyntax syntax-checks each generated script with the
// real shell when it is installed, catching quoting/escaping regressions. It is
// skipped for any shell not present on the runner.
func TestGeneratedScripts_ShellSyntax(t *testing.T) {
	m := buildCompletionModel()
	checks := map[string]func(path string) *exec.Cmd{
		"bash": func(p string) *exec.Cmd { return exec.Command("bash", "-n", p) },
		"zsh":  func(p string) *exec.Cmd { return exec.Command("zsh", "-n", p) },
		"fish": func(p string) *exec.Cmd { return exec.Command("fish", "--no-execute", p) },
	}
	for shell, cmdFor := range checks {
		t.Run(shell, func(t *testing.T) {
			bin := shell
			if _, err := exec.LookPath(bin); err != nil {
				t.Skipf("%s not installed", bin)
			}
			path := filepath.Join(t.TempDir(), "completion")
			if err := os.WriteFile(path, []byte(generateCompletion(shell, m)), 0o644); err != nil {
				t.Fatal(err)
			}
			if out, err := cmdFor(path).CombinedOutput(); err != nil {
				t.Errorf("%s syntax check failed: %v\n%s", shell, err, out)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// cross-fire disambiguation (fish flag rules, bash positional scan)
// ----------------------------------------------------------------------------

// fishFlagRules returns the generated `complete` lines in a command's
// "# <cmd> flags" block, so one command's flag guards can be asserted in
// isolation from every other rule in the script.
func fishFlagRules(t *testing.T, script, cmd string) []string {
	t.Helper()
	var rules []string
	lines := strings.Split(script, "\n")
	for i, l := range lines {
		if l != "# "+cmd+" flags" {
			continue
		}
		for _, r := range lines[i+1:] {
			if !strings.HasPrefix(r, "complete ") {
				break
			}
			rules = append(rules, r)
		}
		return rules
	}
	t.Fatalf("fish completion has no %q flag block", cmd)
	return nil
}

// TestGeneratedFish_FlagRulesDoNotCrossFire pins the guard that stops a command
// name appearing as ANOTHER command's positional value from claiming the flag
// rules. `dotf plasma apply` puts the token "apply" on the line, which
// __fish_seen_subcommand_from matches wherever it sits, so without the
// exclusion the top-level apply command's flags are offered for a plasma verb —
// and every one of them is an exit-2 usage error there.
//
// The expectation is DERIVED, not listed: any command name that is also a
// plasma verb (or a setup module, or a completion shell) must exclude the
// command that owns that positional, so a fifth verb colliding with a command
// name is covered the day it is added.
func TestGeneratedFish_FlagRulesDoNotCrossFire(t *testing.T) {
	m := buildCompletionModel()
	fish := generateCompletion("fish", m)

	for _, c := range m.Commands {
		if len(c.Flags) == 0 {
			continue
		}
		for _, owner := range positionalOwnersOf(m, c.Name) {
			want := "and not __fish_seen_subcommand_from " + owner
			for _, rule := range fishFlagRules(t, fish, c.Name) {
				if !strings.Contains(rule, want) {
					t.Errorf("fish %s flag rule does not exclude %s (%q):\n%s", c.Name, owner, want, rule)
				}
			}
		}
	}

	// The concrete collisions this guard exists for: three of plasma's four
	// verbs are also top-level command names.
	for _, verb := range []string{"apply", "diff", "status"} {
		if !slices.Contains(completionPlasmaVerbs, verb) || !slices.Contains(m.commandNames(), verb) {
			t.Fatalf("%q is no longer both a plasma verb and a command; update this test", verb)
		}
		if owners := positionalOwnersOf(m, verb); !slices.Contains(owners, "plasma") {
			t.Errorf("positionalOwnersOf(%q) = %v, want it to include plasma", verb, owners)
		}
	}
}

// fishComplete runs `complete -C <line>` under a real fish against the given
// script and returns the candidate words (the tab-separated description is
// dropped). It skips when fish is missing or too old for --no-config, so the
// test never depends on the developer's own fish configuration.
func fishComplete(t *testing.T, script, line string) []string {
	t.Helper()
	if _, err := exec.LookPath("fish"); err != nil {
		t.Skip("fish not installed")
	}
	home := t.TempDir()
	env := []string{"HOME=" + home, "XDG_CONFIG_HOME=" + filepath.Join(home, "config"),
		"XDG_DATA_HOME=" + filepath.Join(home, "data"), "PATH=" + os.Getenv("PATH")}

	probe := exec.Command("fish", "--no-config", "-c", "true")
	probe.Env = env
	if err := probe.Run(); err != nil {
		t.Skipf("fish does not support --no-config: %v", err)
	}

	path := filepath.Join(home, "dotf.fish")
	if err := os.WriteFile(path, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("fish", "--no-config", "-c",
		"source '"+path+"'\ncomplete -C '"+line+"'")
	cmd.Env = env
	cmd.Dir = home
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("fish complete -C %q failed: %v", line, err)
	}

	var words []string
	for _, l := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if l == "" {
			continue
		}
		words = append(words, strings.SplitN(l, "\t", 2)[0])
	}
	return words
}

// TestGeneratedFish_PlasmaVerbFlagsInRealFish drives the generated script
// through a real fish: `dotf plasma apply --` must offer plasma's flags and
// NONE of the top-level apply command's, while `dotf apply --` is unaffected.
func TestGeneratedFish_PlasmaVerbFlagsInRealFish(t *testing.T) {
	script := generateCompletion("fish", buildCompletionModel())

	got := fishComplete(t, script, "dotf plasma apply --")
	for _, want := range []string{
		"--recommended", "--reload", "--only", "--yes", "--show-volatile", "--link", "--unlink",
	} {
		if !slices.Contains(got, want) {
			t.Errorf("`dotf plasma apply --` = %v, want it to offer %s", got, want)
		}
	}
	for _, leaked := range []string{"--headless", "--gaming", "--snapper", "--enable-ollama", "--plasma"} {
		if slices.Contains(got, leaked) {
			t.Errorf("`dotf plasma apply --` leaks the top-level apply flag %s: %v", leaked, got)
		}
	}

	top := fishComplete(t, script, "dotf apply --")
	for _, want := range []string{"--headless", "--snapper", "--plasma"} {
		if !slices.Contains(top, want) {
			t.Errorf("`dotf apply --` = %v, want it to still offer %s", top, want)
		}
	}
	for _, leaked := range []string{"--recommended", "--show-volatile"} {
		if slices.Contains(top, leaked) {
			t.Errorf("`dotf apply --` leaks plasma's %s: %v", leaked, top)
		}
	}
}

// bashComplete sources the generated bash script, drives _dotf with the given
// COMP_WORDS (the last element is the word under the cursor), and returns
// COMPREPLY. It runs in an EMPTY directory so a file-completion result is
// deterministically empty.
func bashComplete(t *testing.T, script string, words ...string) []string {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not installed")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "dotf.bash")
	if err := os.WriteFile(path, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(dir, "empty")
	if err := os.Mkdir(empty, 0o755); err != nil {
		t.Fatal(err)
	}

	quoted := make([]string, len(words))
	for i, w := range words {
		quoted[i] = `"` + w + `"`
	}
	driver := "source '" + path + "'\n" +
		"COMP_WORDS=(" + strings.Join(quoted, " ") + ")\n" +
		"COMP_CWORD=" + strconv.Itoa(len(words)-1) + "\n" +
		"COMPREPLY=()\n_dotf\n" +
		`printf '%s\n' "${COMPREPLY[@]}"` + "\n"

	cmd := exec.Command("bash", "-c", driver)
	cmd.Dir = empty
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("bash driver failed: %v", err)
	}

	var got []string
	for _, l := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if l != "" {
			got = append(got, l)
		}
	}
	return got
}

// TestGeneratedBash_PositionalScanSkipsFlagValues pins the flag-value-aware
// positional scan. The old scan counted ANY non-dash word after the subcommand
// as the positional, so `--only kwinrc` was read as the verb: the verb list was
// then never offered again, and `--only <TAB>` offered the verb names as if
// they were file paths — actively steering the user into an exit-2 usage error.
func TestGeneratedBash_PositionalScanSkipsFlagValues(t *testing.T) {
	script := generateCompletion("bash", buildCompletionModel())
	verbs := completionPlasmaVerbs

	t.Run("verb list survives a value flag", func(t *testing.T) {
		got := bashComplete(t, script, "dotf", "plasma", "--only", "kwinrc", "")
		for _, verb := range verbs {
			if !slices.Contains(got, verb) {
				t.Errorf("`dotf plasma --only kwinrc <TAB>` = %v, want the verb %q", got, verb)
			}
		}
	})

	t.Run("attached value form also survives", func(t *testing.T) {
		got := bashComplete(t, script, "dotf", "plasma", "--only=kwinrc", "")
		for _, verb := range verbs {
			if !slices.Contains(got, verb) {
				t.Errorf("`dotf plasma --only=kwinrc <TAB>` = %v, want the verb %q", got, verb)
			}
		}
	})

	t.Run("a value flag's argument is not the verb list", func(t *testing.T) {
		got := bashComplete(t, script, "dotf", "plasma", "--only", "")
		for _, verb := range verbs {
			if slices.Contains(got, verb) {
				t.Errorf("`dotf plasma --only <TAB>` offers the verb %q as a value: %v", verb, got)
			}
		}
	})

	t.Run("a supplied verb still suppresses the list", func(t *testing.T) {
		if got := bashComplete(t, script, "dotf", "plasma", "export", ""); len(got) != 0 {
			t.Errorf("`dotf plasma export <TAB>` = %v, want nothing (the verb is supplied)", got)
		}
	})

	t.Run("setup module list survives --manifest", func(t *testing.T) {
		got := bashComplete(t, script, "dotf", "setup", "--manifest", "m.toml", "")
		for _, mod := range completionModules {
			if !slices.Contains(got, mod) {
				t.Errorf("`dotf setup --manifest m.toml <TAB>` = %v, want the module %q", got, mod)
			}
		}
	})
}

// TestGeneratedScripts_PathPositional pins file completion for `dotf deb`'s
// <url|path> argument.
//
// This is a regression guard for a real gap: the generator completes flags and
// fixed-candidate positionals, so a command whose positional is a free
// filesystem path offered NOTHING at all until positionalCommand grew IsPath.
// Each shell needs its own idiom, so all three are asserted.
func TestGeneratedScripts_PathPositional(t *testing.T) {
	m := buildCompletionModel()

	t.Run("zsh uses _files", func(t *testing.T) {
		if got := generateZshCompletion(m); !strings.Contains(got, "'1:path:_files'") {
			t.Error("zsh completion does not offer file completion for deb's positional")
		}
	})

	t.Run("bash uses compgen -f", func(t *testing.T) {
		got := generateBashCompletion(m)
		i := strings.Index(got, "        deb)\n")
		if i < 0 {
			t.Fatal("bash completion has no deb case block")
		}
		block := got[i:]
		if j := strings.Index(block, ";;"); j > 0 {
			block = block[:j]
		}
		if !strings.Contains(block, "compgen -f") {
			t.Errorf("bash deb block does not offer file completion:\n%s", block)
		}
	})

	t.Run("fish forces files", func(t *testing.T) {
		got := generateFishCompletion(m)
		if !strings.Contains(got, "# deb: path argument") {
			t.Fatal("fish completion has no deb positional rule")
		}
		i := strings.Index(got, "# deb: path argument")
		line := got[i:]
		if j := strings.IndexByte(line, '\n'); j >= 0 {
			line = line[j+1:]
		}
		if j := strings.IndexByte(line, '\n'); j >= 0 {
			line = line[:j]
		}
		if !strings.HasSuffix(strings.TrimSpace(line), "-F") {
			t.Errorf("fish deb rule does not force file completion: %q", line)
		}
	})
}
