package main

import (
	"bytes"
	"flag"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// dispatchableCommands are the seventeen commands routed by main.go's switch.
// "help" is a registry entry but is handled before the switch, so it is
// excluded here.
var dispatchableCommands = []string{
	"version", "platform", "plan", "install", "status",
	"diff", "doctor", "link", "unlink", "backup", "sysconfig", "apply", "lock", "self-update", "setup", "plasma", "completion",
}

// allRegistryNames is the full ordered set the registry must expose.
var allRegistryNames = []string{
	"version", "platform", "plan", "install", "status",
	"diff", "doctor", "link", "unlink", "backup", "sysconfig", "apply", "lock", "self-update", "setup", "plasma", "deb", "completion", "help",
}

func TestIsHelpToken(t *testing.T) {
	cases := map[string]bool{
		"help":     true,
		"-h":       true,
		"-help":    true,
		"--help":   true,
		"version":  false,
		"":         false,
		"-badflag": false,
		"helper":   false,
		"-hh":      false,
	}
	for in, want := range cases {
		if got := isHelpToken(in); got != want {
			t.Errorf("isHelpToken(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestWantsHelp(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"nil", nil, false},
		{"dash-h", []string{"-h"}, true},
		{"dash-dash-help", []string{"--help"}, true},
		{"dash-help", []string{"-help"}, true},
		{"flag-then-help", []string{"--backup", "--help"}, true},
		{"flag-only", []string{"--backup"}, false},
		{"intent-then-h", []string{"--intent", "-h"}, true},
		{"terminator-stops-scan", []string{"--", "-h"}, false},
		{"bare-help-word", []string{"help"}, false},
	}
	for _, tc := range cases {
		if got := wantsHelp(tc.args); got != tc.want {
			t.Errorf("wantsHelp(%v) = %v, want %v", tc.args, got, tc.want)
		}
	}
}

func TestLookupHelp(t *testing.T) {
	for _, name := range allRegistryNames {
		ch, ok := lookupHelp(name)
		if !ok {
			t.Errorf("lookupHelp(%q) ok = false, want true", name)
			continue
		}
		if ch.name != name {
			t.Errorf("lookupHelp(%q).name = %q, want %q", name, ch.name, name)
		}
	}
	for _, name := range []string{"bogus", ""} {
		if _, ok := lookupHelp(name); ok {
			t.Errorf("lookupHelp(%q) ok = true, want false", name)
		}
	}
}

// TestHelpRegistry_CoversDispatch is the drift guard: every dispatchable
// command must have a registry entry, and the registry's ordered name set must
// exactly equal allRegistryNames. It fails loudly if a switch case is added
// without a registry entry (or vice versa).
func TestHelpRegistry_CoversDispatch(t *testing.T) {
	for _, name := range dispatchableCommands {
		if _, ok := lookupHelp(name); !ok {
			t.Errorf("dispatchable command %q has no registry entry", name)
		}
	}

	got := make([]string, len(helpRegistry))
	for i, ch := range helpRegistry {
		got[i] = ch.name
	}
	if !equalStringSlices(got, allRegistryNames) {
		t.Errorf("helpRegistry names = %v, want %v", got, allRegistryNames)
	}
}

func TestRun_Help_Global(t *testing.T) {
	stdout, stderr, code := captureOutput(t, func() int { return run([]string{"help"}) })

	if code != 0 {
		t.Errorf("run([help]) code = %d, want 0", code)
	}
	if stderr != "" {
		t.Errorf("run([help]) stderr = %q, want empty", stderr)
	}

	for _, want := range []string{
		tagline, "Usage:", "Commands:", "Conventions:", "Environment:",
		"single dash (-name) is equivalent",
		`Run "dotf help <command>" for detailed help on a command.`,
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("run([help]) stdout missing %q", want)
		}
	}
	for _, name := range allRegistryNames {
		if !strings.Contains(stdout, name) {
			t.Errorf("run([help]) stdout missing command %q", name)
		}
	}
}

func TestRun_DashDashHelp_Global(t *testing.T) {
	helpOut, _, _ := captureOutput(t, func() int { return run([]string{"help"}) })
	stdout, stderr, code := captureOutput(t, func() int { return run([]string{"--help"}) })

	if code != 0 {
		t.Errorf("run([--help]) code = %d, want 0", code)
	}
	if stderr != "" {
		t.Errorf("run([--help]) stderr = %q, want empty", stderr)
	}
	if stdout != helpOut {
		t.Errorf("run([--help]) stdout != run([help]) stdout")
	}
}

func TestRun_DashH_Global(t *testing.T) {
	helpOut, _, _ := captureOutput(t, func() int { return run([]string{"help"}) })
	stdout, stderr, code := captureOutput(t, func() int { return run([]string{"-h"}) })

	if code != 0 {
		t.Errorf("run([-h]) code = %d, want 0", code)
	}
	if stderr != "" {
		t.Errorf("run([-h]) stderr = %q, want empty", stderr)
	}
	if stdout != helpOut {
		t.Errorf("run([-h]) stdout != run([help]) stdout")
	}
}

func TestRun_HelpToken_First_WithCommand(t *testing.T) {
	stdout, stderr, code := captureOutput(t, func() int { return run([]string{"-h", "link"}) })

	if code != 0 {
		t.Errorf("run([-h link]) code = %d, want 0", code)
	}
	if stderr != "" {
		t.Errorf("run([-h link]) stderr = %q, want empty", stderr)
	}
	if !strings.Contains(stdout, "dotf link - ") {
		t.Errorf("run([-h link]) stdout = %q, want link help", stdout)
	}
}

func TestRun_Help_Command_All(t *testing.T) {
	for _, name := range allRegistryNames {
		t.Run(name, func(t *testing.T) {
			stdout, stderr, code := captureOutput(t, func() int { return run([]string{"help", name}) })

			if code != 0 {
				t.Errorf("run([help %s]) code = %d, want 0", name, code)
			}
			if stderr != "" {
				t.Errorf("run([help %s]) stderr = %q, want empty", name, stderr)
			}
			prefix := "dotf " + name + " - "
			if !strings.HasPrefix(stdout, prefix) {
				t.Errorf("run([help %s]) stdout = %q, want prefix %q", name, stdout, prefix)
			}
			if !strings.Contains(stdout, "Usage:") {
				t.Errorf("run([help %s]) stdout missing Usage:", name)
			}
		})
	}
}

func TestRun_CommandDashDashHelp_All(t *testing.T) {
	for _, name := range dispatchableCommands {
		t.Run(name, func(t *testing.T) {
			helpOut, _, _ := captureOutput(t, func() int { return run([]string{"help", name}) })
			stdout, stderr, code := captureOutput(t, func() int { return run([]string{name, "--help"}) })

			if code != 0 {
				t.Errorf("run([%s --help]) code = %d, want 0", name, code)
			}
			if stderr != "" {
				t.Errorf("run([%s --help]) stderr = %q, want empty", name, stderr)
			}
			if !strings.Contains(stdout, "dotf "+name+" - ") {
				t.Errorf("run([%s --help]) stdout = %q, want command header", name, stdout)
			}
			if stdout != helpOut {
				t.Errorf("run([%s --help]) stdout != run([help %s]) stdout", name, name)
			}
		})
	}
}

func TestRun_CommandDashH_All(t *testing.T) {
	for _, name := range dispatchableCommands {
		t.Run(name, func(t *testing.T) {
			helpOut, _, _ := captureOutput(t, func() int { return run([]string{"help", name}) })
			stdout, stderr, code := captureOutput(t, func() int { return run([]string{name, "-h"}) })

			if code != 0 {
				t.Errorf("run([%s -h]) code = %d, want 0", name, code)
			}
			if stderr != "" {
				t.Errorf("run([%s -h]) stderr = %q, want empty", name, stderr)
			}
			if !strings.Contains(stdout, "dotf "+name+" - ") {
				t.Errorf("run([%s -h]) stdout = %q, want command header", name, stdout)
			}
			if stdout != helpOut {
				t.Errorf("run([%s -h]) stdout != run([help %s]) stdout", name, name)
			}
		})
	}
}

func TestRun_Version_Help_ShortCircuits(t *testing.T) {
	stdout, stderr, code := captureOutput(t, func() int { return run([]string{"version", "--help"}) })

	if code != 0 {
		t.Errorf("run([version --help]) code = %d, want 0", code)
	}
	if stderr != "" {
		t.Errorf("run([version --help]) stderr = %q, want empty", stderr)
	}
	if !strings.Contains(stdout, "Usage:") {
		t.Errorf("run([version --help]) stdout missing Usage:")
	}
	versionLine := regexp.MustCompile(`^dotf \S+ \(.*\)\n$`)
	if versionLine.MatchString(stdout) {
		t.Errorf("run([version --help]) printed the version string, want help")
	}
}

func TestRun_Platform_Help_ShortCircuits(t *testing.T) {
	stdout, stderr, code := captureOutput(t, func() int { return run([]string{"platform", "-h"}) })

	if code != 0 {
		t.Errorf("run([platform -h]) code = %d, want 0", code)
	}
	if stderr != "" {
		t.Errorf("run([platform -h]) stderr = %q, want empty", stderr)
	}
	if !strings.Contains(stdout, "Usage:") {
		t.Errorf("run([platform -h]) stdout missing Usage:")
	}
	if strings.Contains(stdout, "os=") {
		t.Errorf("run([platform -h]) printed platform facts, want help")
	}
}

func TestRun_Apply_Help_ShortCircuits(t *testing.T) {
	stdout, stderr, code := captureOutput(t, func() int { return run([]string{"apply", "--help"}) })

	if code != 0 {
		t.Errorf("run([apply --help]) code = %d, want 0", code)
	}
	if stderr != "" {
		t.Errorf("run([apply --help]) stderr = %q, want empty", stderr)
	}
	if !strings.Contains(stdout, "dotf apply - ") {
		t.Errorf("run([apply --help]) stdout = %q, want apply help", stdout)
	}
}

func TestRun_Sysconfig_Help_ShortCircuits(t *testing.T) {
	stdout, stderr, code := captureOutput(t, func() int { return run([]string{"sysconfig", "-h"}) })

	if code != 0 {
		t.Errorf("run([sysconfig -h]) code = %d, want 0", code)
	}
	if stderr != "" {
		t.Errorf("run([sysconfig -h]) stderr = %q, want empty", stderr)
	}
	if !strings.Contains(stdout, "dotf sysconfig - ") {
		t.Errorf("run([sysconfig -h]) stdout = %q, want sysconfig help", stdout)
	}
}

func TestRun_Help_UnknownCommand(t *testing.T) {
	stdout, stderr, code := captureOutput(t, func() int { return run([]string{"help", "bogus"}) })

	if code != 1 {
		t.Errorf("run([help bogus]) code = %d, want 1", code)
	}
	if stdout != "" {
		t.Errorf("run([help bogus]) stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, `unknown subcommand "bogus"`) {
		t.Errorf("run([help bogus]) stderr = %q, want unknown-subcommand message", stderr)
	}
	if !strings.Contains(stderr, "Commands:") {
		t.Errorf("run([help bogus]) stderr missing Commands table")
	}
}

func TestRun_NoArgs_Enriched(t *testing.T) {
	stdout, stderr, code := captureOutput(t, func() int { return run(nil) })

	if code != 1 {
		t.Errorf("run(nil) code = %d, want 1", code)
	}
	if stdout != "" {
		t.Errorf("run(nil) stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "missing subcommand") {
		t.Errorf("run(nil) stderr = %q, want missing-subcommand message", stderr)
	}
	if !strings.Contains(stderr, "Commands:") {
		t.Errorf("run(nil) stderr missing Commands table")
	}
	for _, name := range dispatchableCommands {
		if !strings.Contains(stderr, name) {
			t.Errorf("run(nil) stderr missing command %q", name)
		}
	}
}

func TestRun_UnknownSubcommand_Enriched(t *testing.T) {
	stdout, stderr, code := captureOutput(t, func() int { return run([]string{"bogus"}) })

	if code != 1 {
		t.Errorf("run([bogus]) code = %d, want 1", code)
	}
	if stdout != "" {
		t.Errorf("run([bogus]) stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, `unknown subcommand "bogus"`) {
		t.Errorf("run([bogus]) stderr = %q, want unknown-subcommand message", stderr)
	}
	if !strings.Contains(stderr, "Commands:") {
		t.Errorf("run([bogus]) stderr missing Commands table")
	}
	if !strings.Contains(stderr, `Run "dotf help <command>"`) {
		t.Errorf("run([bogus]) stderr missing help hint")
	}
}

func TestRun_Plan_BadFlag_NoStdoutLeak(t *testing.T) {
	stdout, stderr, code := captureOutput(t, func() int { return run([]string{"plan", "--bogus"}) })

	if code != 2 {
		t.Errorf("run([plan --bogus]) code = %d, want 2", code)
	}
	if stdout != "" {
		t.Errorf("run([plan --bogus]) stdout = %q, want empty (no rich help leak)", stdout)
	}
	if stderr == "" {
		t.Error("run([plan --bogus]) stderr is empty, want a flag-parse error")
	}
}

func TestRun_CommandBadFlag_NoStdoutLeak(t *testing.T) {
	for _, name := range []string{"plan", "install", "status", "diff", "link", "sysconfig", "apply"} {
		t.Run(name, func(t *testing.T) {
			stdout, stderr, code := captureOutput(t, func() int { return run([]string{name, "--bogus"}) })

			if code != 2 {
				t.Errorf("run([%s --bogus]) code = %d, want 2", name, code)
			}
			if stdout != "" {
				t.Errorf("run([%s --bogus]) stdout = %q, want empty", name, stdout)
			}
			if stderr == "" {
				t.Errorf("run([%s --bogus]) stderr is empty, want a flag-parse error", name)
			}
		})
	}
}

func TestRun_CommandHelp_BeatsBadFlag(t *testing.T) {
	stdout, stderr, code := captureOutput(t, func() int { return run([]string{"link", "--bogus", "--help"}) })

	if code != 0 {
		t.Errorf("run([link --bogus --help]) code = %d, want 0", code)
	}
	if stderr != "" {
		t.Errorf("run([link --bogus --help]) stderr = %q, want empty", stderr)
	}
	if !strings.Contains(stdout, "dotf link - ") {
		t.Errorf("run([link --bogus --help]) stdout = %q, want link help", stdout)
	}
}

func TestRun_Install_IntentDashH_ShowsHelp(t *testing.T) {
	stdout, _, code := captureOutput(t, func() int { return run([]string{"install", "--intent", "-h"}) })

	if code != 0 {
		t.Errorf("run([install --intent -h]) code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "Usage:") {
		t.Errorf("run([install --intent -h]) stdout = %q, want help", stdout)
	}
}

func TestPrintGlobalHelp(t *testing.T) {
	var buf bytes.Buffer
	printGlobalHelp(&buf)
	out := buf.String()

	for _, want := range []string{
		tagline, "Usage:", "Commands:", "Conventions:", "Environment:",
		`Run "dotf help <command>" for detailed help on a command.`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("printGlobalHelp missing %q", want)
		}
	}
	for _, ch := range helpRegistry {
		if !strings.Contains(out, ch.name) {
			t.Errorf("printGlobalHelp missing name %q", ch.name)
		}
		if !strings.Contains(out, ch.summary) {
			t.Errorf("printGlobalHelp missing summary %q", ch.summary)
		}
	}
	if !strings.HasSuffix(out, "\n") {
		t.Error("printGlobalHelp output does not end with a newline")
	}
}

func TestPrintCommandHelp_Link(t *testing.T) {
	ch, _ := lookupHelp("link")
	var buf bytes.Buffer
	printCommandHelp(&buf, ch)
	out := buf.String()

	for _, want := range []string{
		"dotf link - ", "Usage:", "Description:", "Flags:", "Examples:",
		"--backup", "--dry-run", "--force", "--hyprland", "--linux-desktop", "--no-backup", "--no-validate",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("printCommandHelp(link) missing %q", want)
		}
	}
}

func TestPrintCommandHelp_NoFlagsSection(t *testing.T) {
	for _, name := range []string{"version"} {
		ch, _ := lookupHelp(name)
		var buf bytes.Buffer
		printCommandHelp(&buf, ch)
		if strings.Contains(buf.String(), "Flags:") {
			t.Errorf("printCommandHelp(%s) contains a Flags section, want none", name)
		}
	}
}

// TestCommandHelp_DRY_NoDrift mechanically proves flag usage is single-sourced:
// every usage string registered by a command's setFlags must appear verbatim in
// its rendered help.
func TestCommandHelp_DRY_NoDrift(t *testing.T) {
	for _, ch := range helpRegistry {
		if ch.setFlags == nil {
			continue
		}
		t.Run(ch.name, func(t *testing.T) {
			fs := flag.NewFlagSet(ch.name, flag.ContinueOnError)
			ch.setFlags(fs)

			var usages []string
			fs.VisitAll(func(f *flag.Flag) { usages = append(usages, f.Usage) })

			var buf bytes.Buffer
			printCommandHelp(&buf, ch)
			out := buf.String()

			for _, u := range usages {
				if !strings.Contains(out, u) {
					t.Errorf("printCommandHelp(%s) missing verbatim flag usage %q", ch.name, u)
				}
			}
		})
	}
}

func TestRenderFlags_DoubleDashAndSorted(t *testing.T) {
	var buf bytes.Buffer
	renderFlags(&buf, func(fs *flag.FlagSet) { setLinkFlags(fs) })
	out := buf.String()

	// Every flag line uses a double dash, never a single dash.
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "--") {
			continue
		}
		if strings.HasPrefix(trimmed, "---") {
			t.Errorf("flag line has triple dash: %q", line)
		}
	}

	order := []string{"--backup", "--dry-run", "--force", "--hyprland", "--linux-desktop", "--no-backup", "--no-validate"}
	last := -1
	for _, name := range order {
		// Match the flag's own line, not a mention inside another usage string.
		idx := strings.Index(out, "  "+name+" ")
		if idx < 0 {
			t.Fatalf("renderFlags(link) missing %q", name)
		}
		if idx < last {
			t.Errorf("renderFlags(link) %q out of alphabetical order", name)
		}
		last = idx
	}
}

func TestRenderFlags_Placeholders(t *testing.T) {
	var buf bytes.Buffer
	renderFlags(&buf, func(fs *flag.FlagSet) { setInstallFlags(fs) })
	out := buf.String()

	if !strings.Contains(out, "--manifest string") {
		t.Errorf("renderFlags(install) missing %q", "--manifest string")
	}
	if !strings.Contains(out, "--intent value") {
		t.Errorf("renderFlags(install) missing %q", "--intent value")
	}
	// A bool flag renders with no placeholder: "--dry-run" is immediately
	// followed by whitespace, not a "string"/"value" token.
	dryRun := regexp.MustCompile(`--dry-run\s{2,}`)
	if !dryRun.MatchString(out) {
		t.Errorf("renderFlags(install) --dry-run rendered with a placeholder, want none:\n%s", out)
	}
}

func TestRenderFlags_NilNoop(t *testing.T) {
	for _, name := range []string{"version", "help"} {
		ch, _ := lookupHelp(name)
		if ch.setFlags != nil {
			t.Errorf("%s has a non-nil setFlags, want nil", name)
		}
		var buf bytes.Buffer
		printCommandHelp(&buf, ch)
		if strings.Contains(buf.String(), "Flags:") {
			t.Errorf("printCommandHelp(%s) contains Flags:, want none", name)
		}
	}
}

func TestSetManifestFlag_Shared(t *testing.T) {
	for i := 0; i < 2; i++ {
		fs := flag.NewFlagSet("t", flag.ContinueOnError)
		ptr := setManifestFlag(fs)
		f := fs.Lookup("manifest")
		if f == nil {
			t.Fatal("setManifestFlag did not register --manifest")
			return
		}
		if f.Usage != manifestFlagUsage {
			t.Errorf("--manifest usage = %q, want %q", f.Usage, manifestFlagUsage)
		}
		if err := fs.Parse([]string{"--manifest", "x"}); err != nil {
			t.Fatalf("Parse error = %v", err)
		}
		if *ptr != "x" {
			t.Errorf("*ptr = %q, want %q", *ptr, "x")
		}
	}
}

func TestSetLinkFlags_Parses(t *testing.T) {
	fs := flag.NewFlagSet("link", flag.ContinueOnError)
	o := setLinkFlags(fs)
	if err := fs.Parse([]string{"--dry-run", "--backup"}); err != nil {
		t.Fatalf("Parse error = %v", err)
	}
	if !o.dryRun || !o.backup {
		t.Errorf("dryRun=%v backup=%v, want both true", o.dryRun, o.backup)
	}
	if o.force || o.noValidate || o.hyprland || o.linuxDesktop {
		t.Errorf("unexpected true flag: force=%v noValidate=%v hyprland=%v linuxDesktop=%v",
			o.force, o.noValidate, o.hyprland, o.linuxDesktop)
	}
}

func TestSetInstallFlags_Parses(t *testing.T) {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	o := setInstallFlags(fs)
	if err := fs.Parse([]string{"--manifest", "m", "--dry-run", "--intent", "a", "--intent", "b"}); err != nil {
		t.Fatalf("Parse error = %v", err)
	}
	if o.manifest != "m" {
		t.Errorf("manifest = %q, want m", o.manifest)
	}
	if !o.dryRun {
		t.Error("dryRun = false, want true")
	}
	if got := []string(o.intents); !equalStringSlices(got, []string{"a", "b"}) {
		t.Errorf("intents = %v, want [a b]", got)
	}
}

func TestSetSysconfigFlags_Parses(t *testing.T) {
	fs := flag.NewFlagSet("sysconfig", flag.ContinueOnError)
	o := setSysconfigFlags(fs)
	args := []string{
		"--manifest", "m", "--hostname", "h", "--timezone", "tz",
		"--locale", "lc", "--shell", "sh", "--firewall", "ufw",
		"--ssh-server", "on", "--dry-run", "--yes",
	}
	if err := fs.Parse(args); err != nil {
		t.Fatalf("Parse error = %v", err)
	}
	if o.manifest != "m" || o.hostname != "h" || o.timezone != "tz" ||
		o.locale != "lc" || o.shell != "sh" || o.firewall != "ufw" ||
		o.sshServer != "on" {
		t.Errorf("string fields mismatch: %+v", *o)
	}
	if !o.dryRun || !o.yes {
		t.Errorf("dryRun=%v yes=%v, want both true", o.dryRun, o.yes)
	}
}

func TestSetApplyFlags_Parses(t *testing.T) {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	f := setApplyFlags(fs)
	args := []string{
		"--manifest", "m", "--homebrew", "true", "--headless", "--force",
		"--gaming", "--enable-ollama", "--hyprland", "--linux-desktop", "--snapper", "--plasma",
		"--dry-run", "--intent", "core",
	}
	if err := fs.Parse(args); err != nil {
		t.Fatalf("Parse error = %v", err)
	}
	if f.Manifest != "m" || f.Homebrew != "true" {
		t.Errorf("string fields mismatch: %+v", *f)
	}
	if !f.Headless || !f.Force || !f.Gaming || !f.Ollama || !f.Hyprland || !f.LinuxDesktop || !f.Snapper || !f.Plasma || !f.DryRun {
		t.Errorf("bool fields mismatch: %+v", *f)
	}
	if got := []string(f.Intents); !equalStringSlices(got, []string{"core"}) {
		t.Errorf("intents = %v, want [core]", got)
	}

	// Feeds resolveApplyOptions unchanged.
	opts := resolveApplyOptions(*f, func(string) string { return "" })
	if !opts.Headless {
		t.Error("resolveApplyOptions(headless) = false, want true")
	}
}

// TestSetPlasmaFlags_Parses asserts every plasmaOpts field is wired to its flag,
// so a field added to the struct without a fs.*Var call is caught here rather
// than silently defaulting at runtime.
func TestSetPlasmaFlags_Parses(t *testing.T) {
	fs := flag.NewFlagSet("plasma", flag.ContinueOnError)
	o := setPlasmaFlags(fs)
	args := []string{
		"--only", "kwinrc,kdeglobals", "--dry-run", "--backup", "--link",
		"--recommended", "--reload", "--show-volatile", "--yes", "--json",
	}
	if err := fs.Parse(args); err != nil {
		t.Fatalf("Parse error = %v", err)
	}
	if o.only != "kwinrc,kdeglobals" {
		t.Errorf("only = %q, want %q", o.only, "kwinrc,kdeglobals")
	}
	if !o.dryRun || !o.backup || !o.link || !o.recommended || !o.reload ||
		!o.showVolatile || !o.yes || !o.jsonOut {
		t.Errorf("bool fields mismatch: %+v", *o)
	}
}

func TestSetFlags_ExactFlagNames(t *testing.T) {
	cases := []struct {
		name  string
		set   func(*flag.FlagSet)
		names []string
	}{
		{"link", func(fs *flag.FlagSet) { setLinkFlags(fs) },
			[]string{"backup", "dry-run", "force", "hyprland", "linux-desktop", "no-backup", "no-validate", "prune"}},
		{"unlink", func(fs *flag.FlagSet) { setUnlinkFlags(fs) },
			[]string{"all", "dry-run", "restore"}},
		// install/plan/status/diff/sysconfig/lock each add --no-user-config via a
		// SEPARATE setNoUserConfigFlag call (their runX + help closures), which is
		// why these cases wrap the base setter with it — mirroring the real
		// registration — while the bare "read"/"manifest" cases below stay pure,
		// proving setReadFlags/setDriftFlags return arities are UNCHANGED.
		{"install", func(fs *flag.FlagSet) { setInstallFlags(fs); setNoUserConfigFlag(fs) },
			[]string{"dry-run", "force", "intent", "manifest", "no-user-config", "only", "remove", "skip", "update"}},
		{"manifest", func(fs *flag.FlagSet) { setManifestFlag(fs) },
			[]string{"manifest"}},
		{"read", func(fs *flag.FlagSet) { setReadFlags(fs) },
			[]string{"json", "manifest"}},
		{"plan", func(fs *flag.FlagSet) { setReadFlags(fs); setNoUserConfigFlag(fs) },
			[]string{"json", "manifest", "no-user-config"}},
		// #88 doctor: setReadFlags + its own --verbose, plus --no-user-config.
		{"doctor", func(fs *flag.FlagSet) { setDoctorFlags(fs); setNoUserConfigFlag(fs) },
			[]string{"json", "manifest", "no-user-config", "verbose"}},
		{"status", func(fs *flag.FlagSet) { setDriftFlags(fs); setNoUserConfigFlag(fs) },
			[]string{"counts", "json", "manifest", "no-user-config"}},
		{"diff", func(fs *flag.FlagSet) { setDriftFlags(fs); setNoUserConfigFlag(fs) },
			[]string{"counts", "json", "manifest", "no-user-config"}},
		{"sysconfig", func(fs *flag.FlagSet) { setSysconfigFlags(fs); setNoUserConfigFlag(fs) },
			[]string{"dry-run", "firewall", "hostname", "locale", "manifest", "no-user-config", "shell", "ssh-server", "timezone", "yes"}},
		// apply registers --no-user-config inside setApplyFlags (its own env-twin
		// path), so this case does not wrap and just expects the name.
		{"apply", func(fs *flag.FlagSet) { setApplyFlags(fs) },
			[]string{"dry-run", "enable-ollama", "force", "gaming", "headless", "homebrew", "hyprland", "intent", "linux-desktop", "manifest", "no-backup", "no-user-config", "only", "plasma", "skip", "snapper"}},
		{"lock", func(fs *flag.FlagSet) { setLockFlags(fs); setNoUserConfigFlag(fs) },
			[]string{"check", "dry-run", "json", "lock", "manifest", "no-user-config", "only", "platform", "skip"}},
		// self-update is flag-only: four booleans, and deliberately NO
		// --no-user-config (it never resolves the manifest overlay).
		{"self-update", func(fs *flag.FlagSet) { setSelfUpdateFlags(fs) },
			[]string{"check", "dry-run", "force", "json"}},
		// setup resolves the manifest overlay, so it adds --no-user-config
		// alongside its base flags.
		{"setup", func(fs *flag.FlagSet) { setSetupFlags(fs); setNoUserConfigFlag(fs) },
			[]string{"dry-run", "manifest", "no-user-config", "yes"}},
		// plasma is flag-only and deliberately has NO --manifest and NO
		// --no-user-config: the managed set is the built-in DefaultFiles table,
		// so it never resolves the manifest overlay (like link and self-update).
		{"plasma", func(fs *flag.FlagSet) { setPlasmaFlags(fs) },
			[]string{"backup", "dry-run", "json", "link", "only", "recommended", "reload",
				"show-volatile", "unlink", "yes"}},
		// backup is flag-only like link: it reads the backup dirs under
		// $HOME, never the manifest.
		{"backup", func(fs *flag.FlagSet) { setBackupFlags(fs) },
			[]string{"dry-run", "force", "json", "keep", "older-than", "yes"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := flag.NewFlagSet(tc.name, flag.ContinueOnError)
			tc.set(fs)
			var got []string
			fs.VisitAll(func(f *flag.Flag) { got = append(got, f.Name) })
			sort.Strings(got)
			want := append([]string(nil), tc.names...)
			sort.Strings(want)
			if !equalStringSlices(got, want) {
				t.Errorf("%s flag names = %v, want %v", tc.name, got, want)
			}
		})
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
