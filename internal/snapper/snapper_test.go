package snapper

import (
	"errors"
	"fmt"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// assertOneCall asserts the Fake recorded exactly one invocation whose
// name+args match want, using the repo's fmt.Sprint slice-compare convention.
func assertOneCall(t *testing.T, fake *exec.Fake, wantName string, wantArgs []string) {
	t.Helper()
	calls := fake.CallsSnapshot()
	if len(calls) != 1 {
		t.Fatalf("CallsSnapshot() len = %d, want 1", len(calls))
	}
	if calls[0].Name != wantName {
		t.Errorf("call name = %q, want %q", calls[0].Name, wantName)
	}
	if fmt.Sprint(calls[0].Args) != fmt.Sprint(wantArgs) {
		t.Errorf("call args = %v, want %v", calls[0].Args, wantArgs)
	}
}

// assertZeroCalls asserts the Fake recorded no Run/Output invocations. LookPath
// probes are not recorded, so a guarded no-op must leave Calls empty.
func assertZeroCalls(t *testing.T, fake *exec.Fake) {
	t.Helper()
	if n := len(fake.CallsSnapshot()); n != 0 {
		t.Errorf("CallsSnapshot() len = %d, want 0", n)
	}
}

func TestCreate_LinuxWithBinary(t *testing.T) {
	fake := exec.NewFake()
	fake.AddPath("snapper", "/usr/bin/snapper")
	s := New(fake, platform.Info{OS: platform.OSArch})

	if err := s.Create("pre-apply"); err != nil {
		t.Errorf("Create() = %v, want nil", err)
	}
	if !s.Available() {
		t.Errorf("Available() = false, want true")
	}
	assertOneCall(t, fake, "sudo",
		[]string{"snapper", "create", "-d", "dotf apply: pre-apply"})
}

func TestCreate_NonLinuxNoOp(t *testing.T) {
	fake := exec.NewFake()
	// snapper resolves, proving the OS guard (not LookPath) short-circuits.
	fake.AddPath("snapper", "/usr/bin/snapper")
	s := New(fake, platform.Info{OS: platform.OSMacOS})

	if err := s.Create("pre-apply"); err != nil {
		t.Errorf("Create() = %v, want nil", err)
	}
	if s.Available() {
		t.Errorf("Available() = true, want false")
	}
	assertZeroCalls(t, fake)
}

func TestCreate_MissingBinary(t *testing.T) {
	fake := exec.NewFake() // no AddPath: snapper does not resolve
	s := New(fake, platform.Info{OS: platform.OSDebian})

	if err := s.Create("pre-apply"); err != nil {
		t.Errorf("Create() = %v, want nil", err)
	}
	if s.Available() {
		t.Errorf("Available() = true, want false")
	}
	assertZeroCalls(t, fake)
}

func TestCreate_RunErrorNonFatal(t *testing.T) {
	wantErr := errors.New("boom")
	fake := exec.NewFake()
	fake.AddPath("snapper", "/usr/bin/snapper")
	fake.RunErrors[exec.CommandKey("sudo", "snapper", "create", "-d", "dotf apply: pre-apply")] = wantErr
	s := New(fake, platform.Info{OS: platform.OSArch})

	err := s.Create("pre-apply")

	if err == nil {
		t.Fatalf("Create() = nil, want non-nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("Create() = %v, want wrapping %v", err, wantErr)
	}
	assertOneCall(t, fake, "sudo",
		[]string{"snapper", "create", "-d", "dotf apply: pre-apply"})
}

func TestCreate_CustomDesc(t *testing.T) {
	fake := exec.NewFake()
	fake.AddPath("snapper", "/usr/bin/snapper")
	s := New(fake, platform.Info{OS: platform.OSSUSE})

	if err := s.Create("nightly-2026"); err != nil {
		t.Errorf("Create() = %v, want nil", err)
	}
	assertOneCall(t, fake, "sudo",
		[]string{"snapper", "create", "-d", "dotf apply: nightly-2026"})
}
