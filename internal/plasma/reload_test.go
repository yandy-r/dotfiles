package plasma

import (
	"errors"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
)

// planWith builds a minimal plan whose modified-file set is exactly paths.
func planWith(lookAndFeel string, kinds ActionKind, paths ...string) *Plan {
	p := &Plan{LookAndFeel: lookAndFeel}
	for _, path := range paths {
		p.Files = append(p.Files, FileAction{Spec: FileSpec{Path: path}, Kind: kinds})
	}
	p.Reloads = ReloadPlan(p)
	return p
}

func TestReloadPlan_DerivesFromModifiedFiles(t *testing.T) {
	tests := []struct {
		name       string
		paths      []string
		wantArgv0  []string
		wantAbsent []string
	}{
		{
			name:      "kwinrc reconfigures kwin",
			paths:     []string{"kwinrc"},
			wantArgv0: []string{"qdbus"},
		},
		{
			name:      "kglobalshortcutsrc restarts kglobalaccel",
			paths:     []string{"kglobalshortcutsrc"},
			wantArgv0: []string{"systemctl"},
		},
		{
			name:      "plasmarc quits then starts plasmashell",
			paths:     []string{"plasmarc"},
			wantArgv0: []string{"kquitapp6", "kstart"},
		},
		{
			// kdeglobals triggers BOTH kwin and plasmashell, in that order.
			name:      "kdeglobals triggers two components",
			paths:     []string{"kdeglobals"},
			wantArgv0: []string{"qdbus", "kquitapp6", "kstart"},
		},
		{
			name:       "an unlisted file needs no reload",
			paths:      []string{"dolphinrc", "katerc", "mimeapps.list"},
			wantAbsent: []string{"qdbus", "kquitapp6", "systemctl", "plasma-apply-lookandfeel"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ReloadPlan(planWith("org.kde.breezedark.desktop", ActionMerge, tc.paths...))

			var argv0 []string
			for _, cmd := range got {
				argv0 = append(argv0, cmd.Argv[0])
			}
			if tc.wantArgv0 != nil && strings.Join(argv0, ",") != strings.Join(tc.wantArgv0, ",") {
				t.Errorf("commands = %v, want %v", argv0, tc.wantArgv0)
			}
			for _, absent := range tc.wantAbsent {
				for _, a := range argv0 {
					if a == absent {
						t.Errorf("unexpected command %q", absent)
					}
				}
			}
		})
	}
}

// TestReloadPlan_OrderAndDeduplication: the overlay must land before anything
// that recomputes a palette from it, and each component contributes once.
func TestReloadPlan_OrderAndDeduplication(t *testing.T) {
	got := ReloadPlan(planWith("org.kde.breezedark.desktop", ActionMerge,
		"plasmarc", "kwinrc", "kdedefaults/kwinrc", "kdedefaults/kdeglobals",
		"klipperrc", "kcminputrc"))

	var components []string
	for _, cmd := range got {
		components = append(components, cmd.Component)
	}
	want := []string{"lookandfeel", "kwin", "plasmashell", "plasmashell"}
	if strings.Join(components, ",") != strings.Join(want, ",") {
		t.Errorf("components = %v, want %v", components, want)
	}
	if got[0].Argv[2] != "org.kde.breezedark.desktop" {
		t.Errorf("lookandfeel argv = %v, want it to carry the package name", got[0].Argv)
	}
}

// TestReloadPlan_UnknownLookAndFeel: a reload we cannot name is not planned at
// all, rather than planned with a missing argument.
func TestReloadPlan_UnknownLookAndFeel(t *testing.T) {
	got := ReloadPlan(planWith("", ActionMerge, "kdedefaults/kwinrc"))
	for _, cmd := range got {
		if cmd.Component == "lookandfeel" {
			t.Fatalf("planned %v with no package name", cmd.Argv)
		}
	}
}

// TestReloadPlan_IgnoresNonMutatingActions: a converged or exported file has
// changed nothing in the session.
func TestReloadPlan_IgnoresNonMutatingActions(t *testing.T) {
	for _, kind := range []ActionKind{ActionSkip, ActionBackup, ActionExport, ActionPrune} {
		if got := ReloadPlan(planWith("pkg", kind, "kwinrc")); len(got) != 0 {
			t.Errorf("%v produced reloads %v, want none", kind, got)
		}
	}
}

// TestReload_GatedOffWithoutFlagOrSession: the commands are information, not an
// action, until both the flag and a live session say otherwise.
func TestReload_GatedOffWithoutFlagOrSession(t *testing.T) {
	e, _, _ := testEngine(t)
	plan := planWith("pkg", ActionMerge, "kwinrc")

	for _, opts := range []Options{
		{Reload: false, Session: SessionLive},
		{Reload: true, Session: SessionAbsent},
		{Reload: true, Session: SessionUnknown},
	} {
		got, err := e.Reload(plan, opts)
		if got != nil || err != nil {
			t.Errorf("Reload(%+v) = (%v, %v), want (nil, nil)", opts, got, err)
		}
	}
	// The commands are still planned in every one of those cases.
	if len(plan.Reloads) == 0 {
		t.Error("the plan lost its reload commands")
	}
}

// TestReload_ProbesQdbusVariants: the binary name varies across distributions,
// and every command goes through the injected Runner rather than a shell.
func TestReload_ProbesQdbusVariants(t *testing.T) {
	tests := []struct {
		name      string
		available map[string]string
		wantArgv0 string
		wantRun   bool
	}{
		{"busctl preferred over every qdbus variant", map[string]string{"busctl": "/usr/bin/busctl", "qdbus6": "/usr/bin/qdbus6"}, "busctl", true},
		{"qdbus6 preferred", map[string]string{"qdbus6": "/usr/bin/qdbus6", "qdbus": "/usr/bin/qdbus"}, "qdbus6", true},
		{"qdbus fallback", map[string]string{"qdbus": "/usr/bin/qdbus"}, "qdbus", true},
		{"neither resolves", map[string]string{}, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := exec.NewFake()
			for name, path := range tc.available {
				fake.AddPath(name, path)
			}
			e, _, _ := testEngine(t)
			e.r = fake

			plan := planWith("pkg", ActionMerge, "kwinrc")
			ran, err := e.Reload(plan, Options{Reload: true, Session: SessionLive})
			if err != nil {
				t.Fatalf("Reload error = %v; a missing reload is never a run failure", err)
			}
			if !tc.wantRun {
				if len(ran) != 0 {
					t.Fatalf("ran = %v, want none", ran)
				}
				if len(fake.CallsSnapshot()) != 0 {
					t.Errorf("calls = %v, want none", fake.CallsSnapshot())
				}
				return
			}
			if len(ran) != 1 || ran[0].Argv[0] != tc.wantArgv0 {
				t.Fatalf("ran = %+v, want a single %q command", ran, tc.wantArgv0)
			}
			calls := fake.CallsSnapshot()
			var invoked []string
			for _, c := range calls {
				if c.Name == "sh" || c.Name == "bash" {
					t.Fatalf("a reload went through a shell: %+v", c)
				}
				invoked = append(invoked, c.Name)
			}
			if !containsString(invoked, tc.wantArgv0) {
				t.Errorf("calls = %v, want one to %q", invoked, tc.wantArgv0)
			}
		})
	}
}

// TestReload_CollectsFailuresAndContinues: a failed reload contributes to the
// run's error but never aborts the remaining reloads.
func TestReload_CollectsFailuresAndContinues(t *testing.T) {
	fake := exec.NewFake()
	fake.AddPath("qdbus6", "/usr/bin/qdbus6")
	fake.AddPath("kquitapp6", "/usr/bin/kquitapp6")
	fake.AddPath("kstart", "/usr/bin/kstart")
	fake.RunErrors[exec.CommandKey("qdbus6", "org.kde.KWin", "/KWin", "reconfigure")] =
		errors.New("no session bus")

	e, _, _ := testEngine(t)
	e.r = fake

	plan := planWith("pkg", ActionMerge, "kdeglobals")
	ran, err := e.Reload(plan, Options{Reload: true, Session: SessionLive})
	if err == nil {
		t.Fatal("Reload error = nil, want the failed kwin reconfigure reported")
	}
	if !strings.Contains(err.Error(), "kwin") {
		t.Errorf("error = %v, want it to name the component", err)
	}
	if len(ran) != 2 {
		t.Errorf("ran = %+v, want the two plasmashell commands to have run anyway", ran)
	}
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// TestPlanApply_ResolvesReloadArgvWithoutTheReloadFlag pins the fix for the
// print-only path: Plan.Reloads must name a binary that exists on THIS host
// even when Options.Reload is false and Engine.Reload therefore never runs.
//
// ReloadPlan is pure and emits the generic "qdbus", which ships on no Plasma 6
// host (only qdbus6 does). Resolution used to live solely inside Engine.Reload,
// which returns early without the flag — so the far commoner path, printing the
// commands the run did NOT execute for the user to copy-paste, advertised a
// command that fails with "command not found".
func TestPlanApply_ResolvesReloadArgvWithoutTheReloadFlag(t *testing.T) {
	tests := []struct {
		name      string
		available map[string]string
		wantArgv0 string
	}{
		{"qdbus6 only, as on a real Plasma 6 host", map[string]string{"qdbus6": "/usr/bin/qdbus6"}, "qdbus6"},
		{"both present prefers qdbus6", map[string]string{"qdbus6": "/usr/bin/qdbus6", "qdbus": "/usr/bin/qdbus"}, "qdbus6"},
		{"neither resolves leaves the plan legible", map[string]string{}, "qdbus"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := exec.NewFake()
			for name, path := range tc.available {
				fake.AddPath(name, path)
			}
			e, _, _ := testEngine(t)
			e.r = fake

			plan := planWith("pkg", ActionMerge, "kwinrc")
			plan.Reloads = e.resolveReloads(plan.Reloads)

			if len(plan.Reloads) != 1 {
				t.Fatalf("Reloads = %+v, want exactly one kwin command", plan.Reloads)
			}
			if got := plan.Reloads[0].Argv[0]; got != tc.wantArgv0 {
				t.Errorf("Reloads[0].Argv[0] = %q, want %q — the printed command must name a binary the user can actually run", got, tc.wantArgv0)
			}
			if len(fake.CallsSnapshot()) != 0 {
				t.Errorf("calls = %v, want none: resolving a reload must not execute it", fake.CallsSnapshot())
			}
		})
	}
}
