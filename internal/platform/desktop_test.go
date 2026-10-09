package platform

import "testing"

func TestDetectDesktop(t *testing.T) {
	for _, tc := range []struct {
		name string
		os   OS
		env  map[string]string
		want bool
	}{
		{"macos", OSMacOS, nil, true},
		{"windows", OSWindows, map[string]string{"WAYLAND_DISPLAY": "w"}, false},
		{"linux empty env", OSDebian, nil, false},
		{"wayland session", OSArch, map[string]string{"XDG_SESSION_TYPE": " Wayland "}, true},
		{"x11 session", OSRedHat, map[string]string{"XDG_SESSION_TYPE": "x11"}, true},
		{"tty session", OSDebian, map[string]string{"XDG_SESSION_TYPE": "tty"}, false},
		{"wayland socket only", OSWSL, map[string]string{"WAYLAND_DISPLAY": "wayland-0"}, true},
		{"ssh x11 forwarding DISPLAY only", OSDebian, map[string]string{"DISPLAY": "localhost:10.0"}, false},
	} {
		got := detectDesktop(tc.os, func(k string) string { return tc.env[k] })
		if got != tc.want {
			t.Errorf("%s: detectDesktop() = %t, want %t", tc.name, got, tc.want)
		}
	}
}
