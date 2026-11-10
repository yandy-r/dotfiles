package main

import (
	"flag"
	"io"
	"os"
	"testing"
)

// TestDesktopFlag: --desktop on install/apply/plan forces the platform
// desktop gate on via DOTF_DESKTOP, so GUI packages install from tmux/herdr/SSH
// shells whose environment lacks the graphical-session variables.
func TestDesktopFlag(t *testing.T) {
	t.Setenv("DOTF_DESKTOP", "")

	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	o := setInstallFlags(fs)
	if err := fs.Parse([]string{"--desktop"}); err != nil || !o.desktop {
		t.Fatalf("install --desktop parse: desktop=%v err=%v", o.desktop, err)
	}

	afs := flag.NewFlagSet("apply", flag.ContinueOnError)
	afs.SetOutput(io.Discard)
	af := setApplyFlags(afs)
	if err := afs.Parse([]string{"--desktop"}); err != nil || !af.Desktop {
		t.Fatalf("apply --desktop parse: desktop=%v err=%v", af.Desktop, err)
	}

	forceDesktop(false)
	if got := os.Getenv("DOTF_DESKTOP"); got != "" {
		t.Fatalf("forceDesktop(false) set DOTF_DESKTOP=%q; want untouched", got)
	}
	forceDesktop(true)
	if got := os.Getenv("DOTF_DESKTOP"); got != "1" {
		t.Fatalf("forceDesktop(true) DOTF_DESKTOP=%q; want 1", got)
	}
}
