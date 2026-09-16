package plasma

import (
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
)

// planOn builds a minimal plan on a given display server whose modified-file
// set is exactly paths.
func planOn(display DisplayServer, paths ...string) *Plan {
	p := &Plan{LookAndFeel: "org.kde.breezedark.desktop", Display: display}
	for _, path := range paths {
		p.Files = append(p.Files, FileAction{Spec: FileSpec{Path: path}, Kind: ActionMerge})
	}
	p.Reloads = ReloadPlan(p)
	return p
}

func TestDetectDisplayServer(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want DisplayServer
	}{
		{
			name: "XDG_SESSION_TYPE wayland",
			env:  map[string]string{"XDG_SESSION_TYPE": "wayland"},
			want: DisplayWayland,
		},
		{
			name: "XDG_SESSION_TYPE is case-insensitive",
			env:  map[string]string{"XDG_SESSION_TYPE": "Wayland"},
			want: DisplayWayland,
		},
		{
			name: "XDG_SESSION_TYPE x11",
			env:  map[string]string{"XDG_SESSION_TYPE": "x11"},
			want: DisplayX11,
		},
		{
			// A bare WAYLAND_DISPLAY is enough: some session managers leave
			// XDG_SESSION_TYPE unset but always export the socket name.
			name: "WAYLAND_DISPLAY without XDG_SESSION_TYPE",
			env:  map[string]string{"WAYLAND_DISPLAY": "wayland-0"},
			want: DisplayWayland,
		},
		{
			name: "DISPLAY alone is X11",
			env:  map[string]string{"DISPLAY": ":0"},
			want: DisplayX11,
		},
		{
			// Both set means a Wayland session running Xwayland, which is
			// still Wayland: KWin owns the shortcuts server either way.
			name: "WAYLAND_DISPLAY wins over DISPLAY",
			env:  map[string]string{"WAYLAND_DISPLAY": "wayland-0", "DISPLAY": ":1"},
			want: DisplayWayland,
		},
		{
			name: "nothing set is unknown",
			env:  map[string]string{},
			want: DisplayUnknown,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DetectDisplayServer(func(k string) string { return tc.env[k] })
			if got != tc.want {
				t.Errorf("DetectDisplayServer() = %v, want %v", got, tc.want)
			}
		})
	}
}

// On Wayland, kwin_wayland owns org.kde.kglobalaccel, so the standalone
// plasma-kglobalaccel unit starts and immediately exits 0. Restarting it is a
// guaranteed no-op that silently reports success, so the plan must mark the
// reload skipped and say a re-login is required instead.
func TestReloadPlan_KGlobalAccelSkippedOnWayland(t *testing.T) {
	plan := planOn(DisplayWayland, "kglobalshortcutsrc")

	var found *ReloadCommand
	for i := range plan.Reloads {
		if plan.Reloads[i].Component == "kglobalaccel" {
			found = &plan.Reloads[i]
		}
	}
	if found == nil {
		t.Fatal("kglobalaccel reload absent from plan; it must be PLANNED and marked skipped, not dropped")
	}
	if !found.Skipped {
		t.Error("kglobalaccel reload on Wayland: Skipped = false, want true")
	}
	if found.SkipReason == "" {
		t.Error("kglobalaccel reload on Wayland: SkipReason is empty, want a machine-readable reason")
	}
	if !strings.Contains(strings.ToLower(found.Note), "log") {
		t.Errorf("Note = %q, want it to tell the user a re-login is required", found.Note)
	}
}

func TestReloadPlan_KGlobalAccelRunsOnX11(t *testing.T) {
	plan := planOn(DisplayX11, "kglobalshortcutsrc")

	for _, cmd := range plan.Reloads {
		if cmd.Component != "kglobalaccel" {
			continue
		}
		if cmd.Skipped {
			t.Error("kglobalaccel reload on X11: Skipped = true, want false — the unit genuinely owns the bus name there")
		}
		if cmd.Argv[0] != "systemctl" {
			t.Errorf("Argv[0] = %q, want systemctl", cmd.Argv[0])
		}
		return
	}
	t.Fatal("no kglobalaccel reload planned on X11")
}

// An unknown display server keeps the historical behaviour: attempting the
// restart is harmless, and refusing to try on a host we cannot classify would
// break the X11 path for anyone whose session leaves the variables unset.
func TestReloadPlan_KGlobalAccelAttemptedWhenDisplayUnknown(t *testing.T) {
	plan := planOn(DisplayUnknown, "kglobalshortcutsrc")

	for _, cmd := range plan.Reloads {
		if cmd.Component == "kglobalaccel" && cmd.Skipped {
			t.Error("kglobalaccel reload with unknown display: Skipped = true, want false")
		}
	}
}

// Wayland must not suppress anything else: kwin and plasmashell reloads work
// exactly as before.
func TestReloadPlan_OtherComponentsUnaffectedOnWayland(t *testing.T) {
	plan := planOn(DisplayWayland, "kwinrc", "plasmarc")

	for _, cmd := range plan.Reloads {
		if cmd.Skipped {
			t.Errorf("component %q was skipped on Wayland; only kglobalaccel should be", cmd.Component)
		}
	}
}

// The skipped command must never reach the Runner — that is the whole point.
func TestReload_DoesNotRunSkippedCommands(t *testing.T) {
	fake := exec.NewFake()
	fake.Found["systemctl"] = "/usr/bin/systemctl"
	e := &Engine{r: fake}

	plan := planOn(DisplayWayland, "kglobalshortcutsrc")
	ran, err := e.Reload(plan, Options{Reload: true, Session: SessionLive})
	if err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	for _, cmd := range ran {
		if cmd.Component == "kglobalaccel" {
			t.Error("Reload() reported kglobalaccel as run, want it absent from the ran set")
		}
	}
	for _, call := range fake.Calls {
		if strings.Contains(strings.Join(append([]string{call.Name}, call.Args...), " "), "plasma-kglobalaccel") {
			t.Errorf("Reload() executed %v, want the skipped command never to reach the Runner", call)
		}
	}
}
