package platform

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func noSocket(string) bool { return false }

func TestDetectDesktop(t *testing.T) {
	for _, tc := range []struct {
		name   string
		os     OS
		env    map[string]string
		socket bool
		want   bool
	}{
		{"macos", OSMacOS, nil, false, true},
		{"windows", OSWindows, map[string]string{"WAYLAND_DISPLAY": "w"}, false, false},
		{"linux empty env", OSDebian, nil, false, false},
		{"wayland session", OSArch, map[string]string{"XDG_SESSION_TYPE": " Wayland "}, false, true},
		{"x11 session", OSRedHat, map[string]string{"XDG_SESSION_TYPE": "x11"}, false, true},
		{"tty session", OSDebian, map[string]string{"XDG_SESSION_TYPE": "tty"}, false, false},
		{"wayland socket only", OSWSL, map[string]string{"WAYLAND_DISPLAY": "wayland-0"}, false, true},
		{"ssh x11 forwarding DISPLAY only", OSDebian, map[string]string{"DISPLAY": "localhost:10.0"}, false, false},
		// tmux/herdr/SSH on a machine with a live graphical session: no session
		// env vars, but the compositor's socket sits in XDG_RUNTIME_DIR.
		{"tmux on desktop host (runtime socket)", OSRedHat, map[string]string{"XDG_RUNTIME_DIR": "/run/user/1000"}, true, true},
		{"runtime socket needs XDG_RUNTIME_DIR", OSRedHat, nil, true, false},
		{"override on, headless", OSDebian, map[string]string{"DOTF_DESKTOP": "1"}, false, true},
		{"override true word", OSDebian, map[string]string{"DOTF_DESKTOP": "true"}, false, true},
		{"override off beats session", OSArch, map[string]string{"DOTF_DESKTOP": "0", "XDG_SESSION_TYPE": "wayland"}, true, false},
		{"override off beats macos", OSMacOS, map[string]string{"DOTF_DESKTOP": "false"}, false, false},
		{"override cannot enable windows", OSWindows, map[string]string{"DOTF_DESKTOP": "1"}, false, false},
		{"override garbage ignored", OSDebian, map[string]string{"DOTF_DESKTOP": "maybe"}, false, false},
	} {
		sock := noSocket
		if tc.socket {
			sock = func(string) bool { return true }
		}
		got := detectDesktop(tc.os, func(k string) string { return tc.env[k] }, sock)
		if got != tc.want {
			t.Errorf("%s: detectDesktop() = %t, want %t", tc.name, got, tc.want)
		}
	}
}

func TestHasWaylandSocket(t *testing.T) {
	dir, err := os.MkdirTemp("", "wl") // short path: unix socket paths are length-limited
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if hasWaylandSocket(dir) {
		t.Fatal("empty dir reported a wayland socket")
	}
	// A regular file or lock named like the socket must not count.
	_ = os.WriteFile(filepath.Join(dir, "wayland-0.lock"), nil, 0o600)
	_ = os.WriteFile(filepath.Join(dir, "wayland-9"), nil, 0o600)
	if hasWaylandSocket(dir) {
		t.Fatal("regular files reported as a wayland socket")
	}
	l, err := net.Listen("unix", filepath.Join(dir, "wayland-1"))
	if err != nil {
		t.Skipf("cannot create unix socket: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	if !hasWaylandSocket(dir) {
		t.Fatal("live wayland-1 socket not detected")
	}
	if hasWaylandSocket("") || hasWaylandSocket("relative/dir") {
		t.Fatal("empty or relative runtime dir must not be probed")
	}
}
