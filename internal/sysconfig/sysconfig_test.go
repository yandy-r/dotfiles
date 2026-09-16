package sysconfig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// assertCall compares a recorded exec.FakeCall against an expected name+argv
// using the repo's fmt.Sprint slice-compare convention.
func assertCall(t *testing.T, got exec.FakeCall, wantName string, wantArgs []string) {
	t.Helper()
	if got.Name != wantName {
		t.Errorf("call name = %q, want %q", got.Name, wantName)
	}
	if fmt.Sprint(got.Args) != fmt.Sprint(wantArgs) {
		t.Errorf("call args = %v, want %v", got.Args, wantArgs)
	}
}

// assertStepArgv asserts a command Step's exact argv (name+args as one slice).
func assertStepArgv(t *testing.T, got Step, wantArgv []string) {
	t.Helper()
	if got.File != nil {
		t.Fatalf("step %q is a file step, want argv %v", got.Desc, wantArgv)
	}
	if fmt.Sprint(got.Argv) != fmt.Sprint(wantArgv) {
		t.Errorf("step argv = %v, want %v", got.Argv, wantArgv)
	}
}

// seedFile writes name (rooted at root) with content, creating parents.
func seedFile(t *testing.T, root, name string, content []byte) {
	t.Helper()
	full := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, content, 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", full, err)
	}
}

func TestEngine_Plan_UnknownOS(t *testing.T) {
	eng := Default(exec.NewFake(), "")

	_, err := eng.Plan(Config{Hostname: "web01"}, platform.Info{OS: platform.OSUnknown})

	var unsupported *UnsupportedOSError
	if !errors.As(err, &unsupported) {
		t.Fatalf("Plan(OSUnknown) error = %v, want *UnsupportedOSError", err)
	}
	if unsupported.OS != platform.OSUnknown {
		t.Errorf("UnsupportedOSError.OS = %q, want %q", unsupported.OS, platform.OSUnknown)
	}
}

func TestEngine_Plan_WindowsNotImplemented(t *testing.T) {
	eng := Default(exec.NewFake(), "")

	plan, err := eng.Plan(Config{Hostname: "web01"}, platform.Info{OS: platform.OSWindows})
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("Plan(OSWindows) error = %v, want ErrNotImplemented", err)
	}
	if len(plan.Steps) != 0 {
		t.Errorf("Plan(OSWindows) steps = %d, want 0", len(plan.Steps))
	}
}

func TestEngine_Execute_RunsArgvStepsInOrder(t *testing.T) {
	fake := exec.NewFake()
	eng := NewEngine(nil, fake, NewOSFileSystem(t.TempDir()))

	plan := Plan{
		Steps: []Step{
			{Domain: "hostname", Argv: []string{"sudo", "hostnamectl", "set-hostname", "web01"}},
			{Domain: "timezone", Argv: []string{"sudo", "timedatectl", "set-timezone", "America/New_York"}},
		},
		Skipped: []string{"locale"},
	}

	res, err := eng.Execute(plan)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}
	if len(res.Applied) != 2 {
		t.Fatalf("Result.Applied len = %d, want 2", len(res.Applied))
	}
	for i, sr := range res.Applied {
		if sr.Err != nil {
			t.Errorf("Applied[%d].Err = %v, want nil", i, sr.Err)
		}
	}
	if fmt.Sprint(res.Skipped) != fmt.Sprint([]string{"locale"}) {
		t.Errorf("Result.Skipped = %v, want [locale]", res.Skipped)
	}

	calls := fake.CallsSnapshot()
	if len(calls) != 2 {
		t.Fatalf("CallsSnapshot() len = %d, want 2", len(calls))
	}
	assertCall(t, calls[0], "sudo", []string{"hostnamectl", "set-hostname", "web01"})
	assertCall(t, calls[1], "sudo", []string{"timedatectl", "set-timezone", "America/New_York"})
}

func TestEngine_Execute_CollectsStepFailuresNonFatally(t *testing.T) {
	fake := exec.NewFake()
	// Fail the middle step; the last must still run.
	fake.RunErrors[exec.CommandKey("sudo", "timedatectl", "set-timezone", "America/New_York")] = errors.New("boom")

	eng := NewEngine(nil, fake, NewOSFileSystem(t.TempDir()))
	plan := Plan{Steps: []Step{
		{Domain: "hostname", Argv: []string{"sudo", "hostnamectl", "set-hostname", "web01"}},
		{Domain: "timezone", Argv: []string{"sudo", "timedatectl", "set-timezone", "America/New_York"}},
		{Domain: "firewall", Argv: []string{"sudo", "ufw", "--force", "enable"}},
	}}

	res, err := eng.Execute(plan)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil (per-step failures are non-fatal)", err)
	}
	if len(res.Applied) != 3 {
		t.Fatalf("Result.Applied len = %d, want 3", len(res.Applied))
	}
	if res.Applied[0].Err != nil {
		t.Errorf("Applied[0].Err = %v, want nil", res.Applied[0].Err)
	}
	if res.Applied[1].Err == nil {
		t.Error("Applied[1].Err = nil, want the injected failure")
	}
	if res.Applied[2].Err != nil {
		t.Errorf("Applied[2].Err = %v, want nil (later steps still run)", res.Applied[2].Err)
	}

	if len(fake.CallsSnapshot()) != 3 {
		t.Errorf("CallsSnapshot() len = %d, want 3 (all steps run)", len(fake.CallsSnapshot()))
	}
}

func TestEngine_Execute_UnknownStepRecordsError(t *testing.T) {
	fake := exec.NewFake()
	eng := NewEngine(nil, fake, NewOSFileSystem(t.TempDir()))

	res, err := eng.Execute(Plan{Steps: []Step{{Domain: "hostname"}}})
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}
	if len(res.Applied) != 1 || res.Applied[0].Err == nil {
		t.Fatalf("Execute() malformed-step result = %+v, want a recorded error", res.Applied)
	}
	if len(fake.CallsSnapshot()) != 0 {
		t.Errorf("CallsSnapshot() len = %d, want 0 for a malformed step", len(fake.CallsSnapshot()))
	}
}

func TestEngine_Execute_FileStepBacksUpThenInstalls(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir()) // keep WriteTemp staging hermetic + auto-cleaned

	root := t.TempDir()
	seedFile(t, root, "etc/hosts", []byte("127.0.0.1 localhost\n"))

	fake := exec.NewFake()
	eng := NewEngine(nil, fake, NewOSFileSystem(root))

	plan := Plan{Steps: []Step{{
		Domain: "hostname",
		File:   &FileInstall{Dest: "/etc/hosts", Content: []byte("127.0.0.1 localhost\n127.0.1.1\tweb01\n"), Backup: true},
	}}}

	res, err := eng.Execute(plan)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}
	if res.Applied[0].Err != nil {
		t.Fatalf("Applied[0].Err = %v, want nil", res.Applied[0].Err)
	}

	calls := fake.CallsSnapshot()
	if len(calls) != 2 {
		t.Fatalf("CallsSnapshot() len = %d, want 2 (backup + install)", len(calls))
	}
	// Backup: sudo cp /etc/hosts /etc/hosts.dotf.bak
	assertCall(t, calls[0], "sudo", []string{"cp", "/etc/hosts", "/etc/hosts.dotf.bak"})
	// Install: sudo cp <tmp> /etc/hosts (tmp path is nondeterministic).
	if calls[1].Name != "sudo" || len(calls[1].Args) != 3 || calls[1].Args[0] != "cp" || calls[1].Args[2] != "/etc/hosts" {
		t.Errorf("install call = %s %v, want sudo cp <tmp> /etc/hosts", calls[1].Name, calls[1].Args)
	}
}

func TestEngine_Execute_FileStepSkipsBackupWhenDestMissing(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())

	root := t.TempDir() // no /etc/hosts seeded → Exists is false → no backup

	fake := exec.NewFake()
	eng := NewEngine(nil, fake, NewOSFileSystem(root))

	plan := Plan{Steps: []Step{{
		Domain: "hostname",
		File:   &FileInstall{Dest: "/etc/hosts", Content: []byte("127.0.1.1\tweb01\n"), Backup: true},
	}}}

	if _, err := eng.Execute(plan); err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	calls := fake.CallsSnapshot()
	if len(calls) != 1 {
		t.Fatalf("CallsSnapshot() len = %d, want 1 (install only, no backup)", len(calls))
	}
	if calls[0].Args[len(calls[0].Args)-1] != "/etc/hosts" {
		t.Errorf("install call args = %v, want dest /etc/hosts", calls[0].Args)
	}
}
