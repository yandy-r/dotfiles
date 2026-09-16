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

	out := s.Create("pre-apply")

	if !out.Created {
		t.Errorf("Created = false, want true")
	}
	if out.Skipped {
		t.Errorf("Skipped = true, want false")
	}
	if out.Err != nil {
		t.Errorf("Err = %v, want nil", out.Err)
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

	out := s.Create("pre-apply")

	if !out.Skipped {
		t.Errorf("Skipped = false, want true")
	}
	if out.Reason != "non-linux" {
		t.Errorf("Reason = %q, want %q", out.Reason, "non-linux")
	}
	if out.Created {
		t.Errorf("Created = true, want false")
	}
	if s.Available() {
		t.Errorf("Available() = true, want false")
	}
	assertZeroCalls(t, fake)
}

func TestCreate_MissingBinary(t *testing.T) {
	fake := exec.NewFake() // no AddPath: snapper does not resolve
	s := New(fake, platform.Info{OS: platform.OSDebian})

	out := s.Create("pre-apply")

	if !out.Skipped {
		t.Errorf("Skipped = false, want true")
	}
	if out.Reason != "snapper-not-installed" {
		t.Errorf("Reason = %q, want %q", out.Reason, "snapper-not-installed")
	}
	if out.Created {
		t.Errorf("Created = true, want false")
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

	out := s.Create("pre-apply")

	if out.Err == nil {
		t.Fatalf("Err = nil, want non-nil")
	}
	if !errors.Is(out.Err, wantErr) {
		t.Errorf("Err = %v, want wrapping %v", out.Err, wantErr)
	}
	if out.Created {
		t.Errorf("Created = true, want false")
	}
	if out.Skipped {
		t.Errorf("Skipped = true, want false")
	}
	assertOneCall(t, fake, "sudo",
		[]string{"snapper", "create", "-d", "dotf apply: pre-apply"})
}

func TestCreate_CustomDesc(t *testing.T) {
	fake := exec.NewFake()
	fake.AddPath("snapper", "/usr/bin/snapper")
	s := New(fake, platform.Info{OS: platform.OSSUSE})

	out := s.Create("nightly-2026")

	if !out.Created {
		t.Errorf("Created = false, want true")
	}
	assertOneCall(t, fake, "sudo",
		[]string{"snapper", "create", "-d", "dotf apply: nightly-2026"})
}
