package privilege

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/yandy-r/dotfiles/internal/exec"
)

func TestSudo_Prime_emitsSudoV(t *testing.T) {
	f := exec.NewFake()
	s := NewSudo()

	if err := s.Prime(f); err != nil {
		t.Fatalf("Prime() error = %v, want nil", err)
	}

	if len(f.Calls) != 1 {
		t.Fatalf("len(Calls) = %d, want 1", len(f.Calls))
	}
	if f.Calls[0].Name != "sudo" || len(f.Calls[0].Args) != 1 || f.Calls[0].Args[0] != "-v" {
		t.Errorf("Calls[0] = %+v, want {sudo [-v]}", f.Calls[0])
	}
}

func TestSudo_KeepAlive_emitsSudoNVThenStops(t *testing.T) {
	old := keepAliveInterval
	keepAliveInterval = 50 * time.Millisecond
	t.Cleanup(func() { keepAliveInterval = old })

	f := exec.NewFake()
	s := NewSudo()
	ctx := context.Background()

	cancel := s.KeepAlive(ctx, f)
	t.Cleanup(cancel)

	deadline := time.Now().Add(500 * time.Millisecond)
	var calls []exec.FakeCall
	for len(calls) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		calls = f.CallsSnapshot()
	}
	if len(calls) == 0 {
		t.Fatal("KeepAlive() did not emit sudo -n -v")
	}

	call := calls[0]
	if call.Name != "sudo" || len(call.Args) != 2 || call.Args[0] != "-n" || call.Args[1] != "-v" {
		t.Errorf("Calls[0] = %+v, want {sudo [-n -v]}", call)
	}

	before := len(calls)
	cancel()

	time.Sleep(150 * time.Millisecond)
	if got := len(f.CallsSnapshot()); got != before {
		t.Errorf("len(Calls) after cancel = %d, want %d (keep-alive did not stop)", got, before)
	}
}

func TestSudo_KeepAlive_surfacesRefreshFailure(t *testing.T) {
	old := keepAliveInterval
	keepAliveInterval = time.Hour
	t.Cleanup(func() { keepAliveInterval = old })

	wantErr := errors.New("timestamp expired")
	f := exec.NewFake()
	f.RunErrors[exec.CommandKey("sudo", "-n", "-v")] = wantErr

	s := NewSudo()
	cancel := s.KeepAlive(context.Background(), f)
	t.Cleanup(cancel)

	deadline := time.Now().Add(500 * time.Millisecond)
	for s.KeepAliveErr() == nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	if !errors.Is(s.KeepAliveErr(), wantErr) {
		t.Errorf("KeepAliveErr() = %v, want %v", s.KeepAliveErr(), wantErr)
	}
}

func TestSudo_Prefix(t *testing.T) {
	s := NewSudo()
	if got, want := s.Prefix(), []string{"sudo"}; len(got) != 1 || got[0] != want[0] {
		t.Errorf("Prefix() = %v, want %v", got, want)
	}
}

func TestRealUser_nonRootUsesCurrentAccount(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires non-root euid")
	}

	t.Setenv("SUDO_USER", "")
	t.Setenv("SUDO_UID", "")
	t.Setenv("SUDO_GID", "")

	u, err := RealUser()
	if err != nil {
		t.Fatalf("RealUser() error = %v, want nil", err)
	}
	if u.Name == "" {
		t.Error("RealUser().Name is empty")
	}
	if u.UID == 0 {
		t.Error("RealUser().UID = 0, want non-zero")
	}
}

func TestRealUser_rootWithoutSudoUserErrors(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root euid")
	}

	t.Setenv("SUDO_USER", "")
	t.Setenv("SUDO_UID", "")
	t.Setenv("SUDO_GID", "")

	if _, err := RealUser(); !errors.Is(err, ErrNoSudoUser) {
		t.Errorf("RealUser() error = %v, want ErrNoSudoUser", err)
	}
}

func TestRealUser_rootWithSudoUserSet(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root euid")
	}

	t.Setenv("SUDO_USER", "builder")
	t.Setenv("SUDO_UID", "1000")
	t.Setenv("SUDO_GID", "1000")

	u, err := RealUser()
	if err != nil {
		t.Fatalf("RealUser() error = %v, want nil", err)
	}
	if u.Name != "builder" || u.UID != 1000 || u.GID != 1000 {
		t.Errorf("RealUser() = %+v, want {Name:builder UID:1000 GID:1000}", u)
	}
}

func TestRealUser_rootWithRootSudoUserErrors(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root euid")
	}

	t.Setenv("SUDO_USER", "root")
	t.Setenv("SUDO_UID", "0")
	t.Setenv("SUDO_GID", "0")

	if _, err := RealUser(); !errors.Is(err, ErrNoSudoUser) {
		t.Errorf("RealUser() error = %v, want ErrNoSudoUser", err)
	}
}

func TestDropper_nonRootNoOps(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires non-root euid")
	}

	d := NewDropper()
	if err := d.Run([]string{"true"}, t.TempDir()); err != nil {
		t.Fatalf("Run(true) error = %v, want nil", err)
	}
}

func TestFakeDropper_recordsArgvAndUID(t *testing.T) {
	f := NewFakeDropper()
	argv := []string{"makepkg", "-si"}

	if err := f.Run(argv, "/home/test"); err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if len(f.Runs) != 1 {
		t.Fatalf("len(Runs) = %d, want 1", len(f.Runs))
	}
	if got, want := f.Runs[0].Argv, argv; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("Runs[0].Argv = %v, want %v", got, want)
	}
	if f.Runs[0].Home != "/home/test" {
		t.Errorf("Runs[0].Home = %q, want %q", f.Runs[0].Home, "/home/test")
	}
	if f.Runs[0].UID == 0 && os.Geteuid() != 0 {
		t.Errorf("Runs[0].UID = 0, want current user uid")
	}
}

func TestDropEnv_explicitFields(t *testing.T) {
	t.Setenv("PATH", "/usr/bin:/bin")

	env := DropEnv("alice", "/home/alice")
	want := map[string]string{
		"HOME":    "/home/alice",
		"USER":    "alice",
		"LOGNAME": "alice",
		"PATH":    "/usr/bin:/bin",
	}
	got := map[string]string{}
	for _, kv := range env {
		for i := 0; i < len(kv); i++ {
			if kv[i] == '=' {
				got[kv[:i]] = kv[i+1:]
				break
			}
		}
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("env[%q] = %q, want %q", k, got[k], v)
		}
	}
}
