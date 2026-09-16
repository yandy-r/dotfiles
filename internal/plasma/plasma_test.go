package plasma

import (
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/platform"
	"github.com/yandy-r/dotfiles/internal/symlink"
)

// TestSupport_GatesNonLinux: KDE Plasma is a Linux desktop, and macOS/Windows
// are permanent non-goals rather than deferred work — so there is no Deferred
// level and Windows is Unsupported, not Deferred.
func TestSupport_GatesNonLinux(t *testing.T) {
	tests := []struct {
		os   platform.OS
		want SupportLevel
	}{
		{platform.OSArch, SupportSupported},
		{platform.OSDebian, SupportSupported},
		{platform.OSRedHat, SupportSupported},
		{platform.OSSUSE, SupportSupported},
		{platform.OSLinux, SupportSupported},
		{platform.OSWSL, SupportSupported},
		{platform.OSMacOS, SupportUnsupported},
		{platform.OSWindows, SupportUnsupported},
		{platform.OSUnknown, SupportUnsupported},
	}
	for _, tc := range tests {
		if got := Support(platform.Info{OS: tc.os}); got != tc.want {
			t.Errorf("Support(%s) = %v, want %v", tc.os, got, tc.want)
		}
	}
}

func TestUnsupportedOSError_Message(t *testing.T) {
	err := &UnsupportedOSError{OS: platform.OSMacOS}
	if got := err.Error(); got != "plasma: unsupported on macos (Linux only)" {
		t.Errorf("Error() = %q", got)
	}
}

// TestDetectSession walks the KDE_FULL_SESSION / XDG_CURRENT_DESKTOP matrix.
// The Unknown state is load-bearing: a non-graphical shell has no evidence
// either way, and treating that as Absent would turn a fresh-machine apply into
// a spurious warning.
func TestDetectSession(t *testing.T) {
	tests := []struct {
		name    string
		full    string
		desktop string
		want    SessionState
	}{
		{"neither set", "", "", SessionUnknown},
		{"KDE_FULL_SESSION=true", "true", "", SessionLive},
		{"KDE_FULL_SESSION=false", "false", "", SessionAbsent},
		{"plain KDE", "", "KDE", SessionLive},
		{"colon separated", "", "KDE:plasma", SessionLive},
		{"colon separated, KDE last", "", "plasma:KDE", SessionLive},
		{"case insensitive", "", "kde", SessionLive},
		{"another desktop", "", "GNOME", SessionAbsent},
		{"a substring is not a match", "", "NOTKDEATALL", SessionAbsent},
		{"empty full, sway desktop", "", "sway:wlroots", SessionAbsent},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			getenv := func(name string) string {
				switch name {
				case "KDE_FULL_SESSION":
					return tc.full
				case "XDG_CURRENT_DESKTOP":
					return tc.desktop
				default:
					return ""
				}
			}
			if got := DetectSession(getenv); got != tc.want {
				t.Errorf("DetectSession() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSessionState_String(t *testing.T) {
	tests := map[SessionState]string{
		SessionUnknown: "unknown",
		SessionAbsent:  "absent",
		SessionLive:    "live",
	}
	for state, want := range tests {
		if got := state.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", state, got, want)
		}
	}
}

// TestActionKind_String pins the stdout bucket names. The CLI reads the KIND,
// never the human-readable Reason, so these strings are contract.
func TestActionKind_String(t *testing.T) {
	tests := map[ActionKind]string{
		ActionMerge:      "merge",
		ActionCreateFile: "create",
		ActionCopyRaw:    "copy",
		ActionExport:     "export",
		ActionPrune:      "prune",
		ActionSymlink:    "link",
		ActionBackup:     "backup",
		ActionSkip:       "skip",
		ActionKind(99):   "unknown",
	}
	for kind, want := range tests {
		if got := kind.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", kind, got, want)
		}
	}
}

// TestNoOverlapWithSymlinkTable is the §3.1 ownership guarantee: `dotf link`
// and `dotf plasma` must never both own a path, or one engine would clobber the
// other's work on every run.
//
// It is the ONLY thing enforcing that guarantee — there is deliberately no
// runtime ownership check, because both tables are compile-time constants (see
// the errors.go header). Do not delete this test as redundant: without it a
// widening of DefaultFiles or symlink.DefaultTable can double-claim a path with
// nothing to catch it.
func TestNoOverlapWithSymlinkTable(t *testing.T) {
	// Managed plasma paths resolve to $HOME/.config/<path>; symlink dests are
	// already $HOME-relative.
	claimed := make(map[string]bool)
	for _, spec := range symlink.DefaultTable() {
		claimed[spec.Dest] = true
	}
	for _, spec := range DefaultFiles() {
		dest := ".config/" + spec.Path
		if claimed[dest] {
			t.Errorf("%q is claimed by both dotf link and dotf plasma", dest)
		}
	}
}

// TestErrorMessages pins the `plasma: ` prefix every typed error carries, which
// is what lets the CLI layer print `dotf: <err>` uniformly.
func TestErrorMessages(t *testing.T) {
	errs := []error{
		&MissingSourceTreeError{Root: "/repo"},
		&MissingRepoTreeError{Dir: "/repo/plasma/files"},
		&NoLiveFilesError{ConfigDir: "/home/u/.config"},
		&UnsupportedOSError{OS: platform.OSMacOS},
		&UnsafePathError{Path: "../x"},
		&DenylistError{Path: "kdeconnect/privateKey.pem"},
		&PrivacyLeakError{Path: "katerc", Line: 3, Pattern: "/home/<user>/"},
		&ImmutableError{Path: "/live/kwinrc"},
		&ImmutableError{Path: "/live/kwinrc", Group: []string{"Locked"}},
		&LinkModeConflictError{Paths: []string{"kwinrc"}},
		&LockError{Path: "/live/kwinrc.lock"},
		&DriftError{Files: []string{"kwinrc"}},
	}
	for _, err := range errs {
		if !strings.HasPrefix(err.Error(), "plasma: ") {
			t.Errorf("%T message = %q, want a \"plasma: \" prefix", err, err.Error())
		}
	}
}
