package plasma

import (
	"errors"
	"fmt"
	"strings"
)

// DisplayServer is the windowing system the session runs on. It matters to
// reloads because Plasma 6 splits the global-shortcuts server by display
// server: see the kglobalaccel case in ReloadPlan.
type DisplayServer int

// Display servers.
const (
	// DisplayUnknown: neither a Wayland nor an X11 marker is set, so the
	// session cannot be classified.
	DisplayUnknown DisplayServer = iota
	// DisplayX11: a classic X11 session, where plasma-kglobalaccel.service
	// runs the standalone kglobalacceld and owns org.kde.kglobalaccel.
	DisplayX11
	// DisplayWayland: a Wayland session, where the compositor owns
	// org.kde.kglobalaccel instead.
	DisplayWayland
)

// String renders a display server for the plasma.display= stdout line.
func (d DisplayServer) String() string {
	switch d {
	case DisplayWayland:
		return "wayland"
	case DisplayX11:
		return "x11"
	default:
		return "unknown"
	}
}

// DetectDisplayServer classifies the windowing system from the environment.
// getenv is injected so the engine performs no environment lookups and the CLI
// passes os.Getenv, matching DetectSession.
//
// XDG_SESSION_TYPE is authoritative when set. Otherwise the socket variables
// decide, and WAYLAND_DISPLAY wins over DISPLAY: a Wayland session running
// Xwayland exports both, and it is still the compositor that owns the
// shortcuts server there.
func DetectDisplayServer(getenv func(string) string) DisplayServer {
	switch strings.ToLower(strings.TrimSpace(getenv("XDG_SESSION_TYPE"))) {
	case "wayland":
		return DisplayWayland
	case "x11":
		return DisplayX11
	}
	if strings.TrimSpace(getenv("WAYLAND_DISPLAY")) != "" {
		return DisplayWayland
	}
	if strings.TrimSpace(getenv("DISPLAY")) != "" {
		return DisplayX11
	}
	return DisplayUnknown
}

// ReloadCommand is one external command that makes a written setting take
// effect in the running session.
type ReloadCommand struct {
	// Component names what the command reloads: "lookandfeel", "kwin",
	// "plasmashell", or "kglobalaccel". Two commands may share a Component when
	// the reload is a quit-then-start pair.
	Component string
	// Argv is the command and its arguments. NEVER a shell string: internal/exec
	// forbids `sh -c` with interpolated input, and a shell would also make these
	// tests non-hermetic.
	Argv []string
	// Reason names the managed file that triggered it.
	Reason string
	// Skipped marks a reload that is PLANNED but must never run, because on
	// this host it cannot possibly work. Engine.Reload refuses to execute it
	// and the CLI renders it as plasma.reload.skipped= rather than as a
	// numbered command.
	//
	// It is planned-and-marked rather than omitted so the output still names
	// the component whose settings did not take effect. Silently dropping it
	// would leave the user with a converged file, no reload line, and no
	// explanation for why the session ignores the change.
	Skipped bool
	// SkipReason is the machine-readable token explaining Skipped, printed as
	// the reason= field. Empty when Skipped is false.
	SkipReason string
	// Note is the human-readable sentence telling the user what to do
	// instead. Empty when Skipped is false.
	Note string
}

// reloadTriggers maps a component to the managed files whose modification
// requires it. Files absent from this table need no reload: their application
// reads them at next start.
var reloadTriggers = []struct {
	component string
	files     []string
}{
	// The look-and-feel overlay is what regenerates the derived colour blocks,
	// so any kdedefaults/ change re-applies the package as a whole.
	{"lookandfeel", []string{
		"kdedefaults/kdeglobals", "kdedefaults/kwinrc", "kdedefaults/kcminputrc",
		"kdedefaults/ksplashrc", "kdedefaults/plasmarc", "kdedefaults/package",
	}},
	{"kwin", []string{"kwinrc", "kcminputrc", "kdeglobals"}},
	{"kglobalaccel", []string{"kglobalshortcutsrc"}},
	{"plasmashell", []string{"plasmarc", "plasmanotifyrc", "klipperrc", "krunnerrc", "kdeglobals"}},
}

// kwinReconfigureCandidates are COMPLETE argv vectors for "make KWin re-read
// its config", probed in order and substituted wholesale for the generic
// placeholder ReloadPlan emits. busctl leads because it ships with systemd on
// every Linux host and needs no Qt tooling at all; the qdbus variants follow
// because the binary name varies per distribution (only qdbus6 on a Plasma 6
// Arch/Debian host, only qdbus-qt6 in Fedora's qt6-qttools). A full-argv table
// rather than an argv[0] candidate list is load-bearing: busctl's call syntax
// takes the interface as its own argument, so the ARITY differs.
var kwinReconfigureCandidates = [][]string{
	{"busctl", "--user", "call", "org.kde.KWin", "/KWin", "org.kde.KWin", "reconfigure"},
	{"qdbus6", "org.kde.KWin", "/KWin", "reconfigure"},
	{"qdbus-qt6", "org.kde.KWin", "/KWin", "reconfigure"},
	{"qdbus", "org.kde.KWin", "/KWin", "reconfigure"},
}

// ReloadPlan derives the reload set from the files a plan will actually modify.
// It is pure: it takes no Runner and reads nothing, which is why the
// look-and-feel package name is resolved at plan time into Plan.LookAndFeel
// rather than read from disk here.
//
// Commands are deduplicated by component and emitted in the fixed order
// lookandfeel → kwin → kglobalaccel → plasmashell, so the overlay lands before
// anything that recomputes a palette from it.
func ReloadPlan(plan *Plan) []ReloadCommand {
	if plan == nil {
		return nil
	}

	modified := make(map[string]bool)
	for _, act := range plan.Files {
		switch act.Kind {
		case ActionSkip, ActionBackup, ActionExport, ActionPrune:
			continue
		default:
			modified[act.Spec.Path] = true
		}
	}

	var out []ReloadCommand
	for _, trigger := range reloadTriggers {
		reason, ok := firstModified(trigger.files, modified)
		if !ok {
			continue
		}
		switch trigger.component {
		case "lookandfeel":
			if plan.LookAndFeel == "" {
				// A reload we cannot name is not a reload we can run.
				continue
			}
			out = append(out, ReloadCommand{
				Component: "lookandfeel",
				Argv:      []string{"plasma-apply-lookandfeel", "-a", plan.LookAndFeel},
				Reason:    reason,
			})
		case "kwin":
			out = append(out, ReloadCommand{
				Component: "kwin",
				Argv:      []string{"qdbus", "org.kde.KWin", "/KWin", "reconfigure"},
				Reason:    reason,
			})
		case "kglobalaccel":
			cmd := ReloadCommand{
				Component: "kglobalaccel",
				Argv:      []string{"systemctl", "--user", "restart", "plasma-kglobalaccel"},
				Reason:    reason,
			}
			// Plasma 6 moved the global-shortcuts server INTO the compositor
			// on Wayland: kwin_wayland owns org.kde.kglobalaccel, and the
			// standalone plasma-kglobalaccel.service is vestigial there. It
			// still starts on request, finds the bus name taken, and exits
			// 0/SUCCESS within the same second — so restarting it is not a
			// weak reload, it is a guaranteed no-op that reports success. The
			// compositor reads kglobalshortcutsrc only at its own startup and
			// exposes no reload method (org.kde.KGlobalAccel has none, and
			// org.kde.KWin's reconfigure does not cover shortcuts), so a
			// re-login is genuinely the only way to apply them.
			//
			// Unknown display servers keep attempting the restart: it is
			// harmless where it cannot work, and refusing to try would break
			// the X11 path for any session that leaves the variables unset.
			if plan.Display == DisplayWayland {
				cmd.Skipped = true
				cmd.SkipReason = "wayland-kwin-owns-server"
				cmd.Note = "kglobalshortcutsrc applied; log out and back in for it to take effect"
			}
			out = append(out, cmd)
		case "plasmashell":
			// One component, two commands: plasmashell has no reconfigure verb.
			out = append(out,
				ReloadCommand{Component: "plasmashell", Argv: []string{"kquitapp6", "plasmashell"}, Reason: reason},
				ReloadCommand{Component: "plasmashell", Argv: []string{"kstart", "plasmashell"}, Reason: reason},
			)
		}
	}
	return out
}

// firstModified returns the first trigger file the plan modifies, which becomes
// the command's Reason. Deduplication is implicit: each component contributes
// its commands at most once.
func firstModified(files []string, modified map[string]bool) (string, bool) {
	for _, f := range files {
		if modified[f] {
			return f, true
		}
	}
	return "", false
}

// Reload runs the plan's reload commands through the injected Runner.
//
// It is a no-op returning (nil, nil) when Options.Reload is false or the
// session is not live. The commands are still PLANNED and still printed in both
// cases — they are information, not an action — because kquitapp6 plasmashell
// tears down the panel, tray, and every widget mid-session, which is not what
// someone syncing a keybinding expects.
//
// A command whose binary cannot be resolved is skipped, not failed: a missing
// reload is never a run failure. A command that runs and fails is collected,
// and the remaining reloads still run.
func (e *Engine) Reload(plan *Plan, opts Options) ([]ReloadCommand, error) {
	if plan == nil || !opts.Reload || opts.Session != SessionLive {
		return nil, nil
	}

	var (
		ran      []ReloadCommand
		failures []error
	)
	for _, cmd := range plan.Reloads {
		// A skipped command is planned for its explanation, never for its
		// effect: running it anyway is what made the no-op look like success.
		if cmd.Skipped {
			continue
		}
		argv, ok := e.resolveArgv(cmd.Argv)
		if !ok {
			continue
		}
		if err := e.r.Run(argv[0], argv[1:]...); err != nil {
			failures = append(failures, fmt.Errorf("plasma: reload %s: %w", cmd.Component, err))
			continue
		}
		cmd.Argv = argv
		ran = append(ran, cmd)
	}
	return ran, errors.Join(failures...)
}

// resolveArgv resolves a command's binary name. The kwin reconfigure is the one
// command resolved from a candidate TABLE: its WHOLE argv is substituted, not
// just argv[0], because the candidates differ in arity as well as in binary
// name. Every other command keeps the argv the plan gave it and only has its
// binary looked up. It reports false when nothing resolves, which the caller
// treats as "skip this one command".
func (e *Engine) resolveArgv(argv []string) ([]string, bool) {
	if len(argv) == 0 {
		return nil, false
	}
	if argv[0] != "qdbus" {
		if _, err := e.r.LookPath(argv[0]); err != nil {
			return nil, false
		}
		return argv, true
	}
	for _, candidate := range kwinReconfigureCandidates {
		if _, err := e.r.LookPath(candidate[0]); err == nil {
			// Copied, never aliased: the caller writes the result back into
			// the plan, and the table is package-level.
			return append([]string(nil), candidate...), true
		}
	}
	return nil, false
}
