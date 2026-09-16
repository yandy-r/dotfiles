package exec

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOSRunner_Output_PassesArgvThrough(t *testing.T) {
	r := New()

	out, err := r.Output("echo", "hi")
	if err != nil {
		t.Fatalf("Output() error = %v, want nil", err)
	}
	if got, want := string(out), "hi\n"; got != want {
		t.Errorf("Output() = %q, want %q", got, want)
	}
}

func TestOSRunner_LookPath(t *testing.T) {
	r := New()

	if _, err := r.LookPath("echo"); err != nil {
		t.Errorf("LookPath(%q) error = %v, want nil", "echo", err)
	}

	if _, err := r.LookPath("definitely-not-a-real-command-xyz"); err == nil {
		t.Errorf("LookPath(%q) error = nil, want error", "definitely-not-a-real-command-xyz")
	}
}

func TestOSRunner_RunDir_RunsInWorkingDir(t *testing.T) {
	r := New()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "marker"), []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// `test -f marker` resolves marker relative to CWD, so it succeeds only
	// when RunDir set the working directory to dir.
	if err := r.RunDir(dir, "test", "-f", "marker"); err != nil {
		t.Errorf("RunDir(dir, test -f marker) error = %v, want nil (should run in dir)", err)
	}

	other := t.TempDir()
	if err := r.RunDir(other, "test", "-f", "marker"); err == nil {
		t.Errorf("RunDir(other, test -f marker) error = nil, want failure (marker absent in other)")
	}
}

func TestFake_Output_RoutesByNameAndArgs(t *testing.T) {
	f := NewFake()
	f.AddCommand("git", []string{"status"}, []byte("clean\n"), nil)
	f.AddCommand("git", []string{"status", "--short"}, []byte(""), nil)

	out, err := f.Output("git", "status")
	if err != nil {
		t.Fatalf("Output(git, status) error = %v, want nil", err)
	}
	if got, want := string(out), "clean\n"; got != want {
		t.Errorf("Output(git, status) = %q, want %q", got, want)
	}

	out, err = f.Output("git", "status", "--short")
	if err != nil {
		t.Fatalf("Output(git, status, --short) error = %v, want nil", err)
	}
	if got, want := string(out), ""; got != want {
		t.Errorf("Output(git, status, --short) = %q, want %q", got, want)
	}

	if len(f.Calls) != 2 {
		t.Fatalf("len(Calls) = %d, want 2", len(f.Calls))
	}
	if f.Calls[0].Name != "git" || len(f.Calls[0].Args) != 1 || f.Calls[0].Args[0] != "status" {
		t.Errorf("Calls[0] = %+v, want {git [status]}", f.Calls[0])
	}
}

func TestFake_Output_UnregisteredCommandErrors(t *testing.T) {
	f := NewFake()

	if _, err := f.Output("git", "status"); err == nil {
		t.Error("Output() error = nil, want error for unregistered command")
	}
}

func TestFake_Output_PropagatesCannedError(t *testing.T) {
	f := NewFake()
	wantErr := errors.New("boom")
	f.AddCommand("brew", []string{"list"}, nil, wantErr)

	_, err := f.Output("brew", "list")
	if !errors.Is(err, wantErr) {
		t.Errorf("Output() error = %v, want %v", err, wantErr)
	}
}

func TestFake_Run_RecordsArgv(t *testing.T) {
	f := NewFake()

	if err := f.Run("brew", "install", "git"); err != nil {
		t.Fatalf("Run(brew, install, git) error = %v, want nil", err)
	}

	if len(f.Calls) != 1 {
		t.Fatalf("len(Calls) = %d, want 1", len(f.Calls))
	}
	if f.Calls[0].Name != "brew" || len(f.Calls[0].Args) != 2 ||
		f.Calls[0].Args[0] != "install" || f.Calls[0].Args[1] != "git" {
		t.Errorf("Calls[0] = %+v, want {brew [install git]}", f.Calls[0])
	}
}

func TestFake_Run_PropagatesCannedError(t *testing.T) {
	f := NewFake()
	wantErr := errors.New("install failed")
	f.RunErrors[CommandKey("brew", "install", "git")] = wantErr

	err := f.Run("brew", "install", "git")
	if !errors.Is(err, wantErr) {
		t.Errorf("Run() error = %v, want %v", err, wantErr)
	}
}

func TestFake_Run_UnregisteredCommandSucceeds(t *testing.T) {
	f := NewFake()

	if err := f.Run("brew", "install", "git"); err != nil {
		t.Errorf("Run() error = %v, want nil for unregistered command", err)
	}
}

func TestFake_RunDir_RecordsDirAndArgv(t *testing.T) {
	f := NewFake()

	if err := f.RunDir("/clone/dest", "/clone/dest/install", "--all"); err != nil {
		t.Fatalf("RunDir() error = %v, want nil", err)
	}

	if len(f.Calls) != 1 {
		t.Fatalf("len(Calls) = %d, want 1", len(f.Calls))
	}
	c := f.Calls[0]
	if c.Dir != "/clone/dest" || c.Name != "/clone/dest/install" ||
		len(c.Args) != 1 || c.Args[0] != "--all" {
		t.Errorf("Calls[0] = %+v, want {Dir:/clone/dest Name:/clone/dest/install Args:[--all]}", c)
	}
}

func TestFake_RunDir_PropagatesCannedError(t *testing.T) {
	f := NewFake()
	wantErr := errors.New("post-install failed")
	f.RunErrors[CommandKey("/clone/dest/install", "--all")] = wantErr

	if err := f.RunDir("/clone/dest", "/clone/dest/install", "--all"); !errors.Is(err, wantErr) {
		t.Errorf("RunDir() error = %v, want %v", err, wantErr)
	}
}

func TestFake_LookPath_HitAndMiss(t *testing.T) {
	f := NewFake()
	f.AddPath("git", "/usr/bin/git")

	path, err := f.LookPath("git")
	if err != nil {
		t.Fatalf("LookPath(git) error = %v, want nil", err)
	}
	if got, want := path, "/usr/bin/git"; got != want {
		t.Errorf("LookPath(git) = %q, want %q", got, want)
	}

	if _, err := f.LookPath("nope"); err == nil {
		t.Error("LookPath(nope) error = nil, want error")
	}
}

var _ Runner = New()
