package exec

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWithContext_Fake_StaysCompatible(t *testing.T) {
	f := NewFake()
	f.AddCommand("git", []string{"status"}, []byte("ok\n"), nil)

	r := WithContext(context.Background(), f)

	out, err := r.Output("git", "status")
	if err != nil {
		t.Fatalf("Output() error = %v, want nil", err)
	}
	if got, want := string(out), "ok\n"; got != want {
		t.Errorf("Output() = %q, want %q", got, want)
	}
	if err := r.Run("brew", "install", "git"); err != nil {
		t.Errorf("Run() error = %v, want nil", err)
	}
	if err := r.RunDir("/d", "cmd", "--flag"); err != nil {
		t.Errorf("RunDir() error = %v, want nil", err)
	}
	if _, err := r.LookPath("git"); err == nil {
		t.Error("LookPath(git) error = nil, want not-found error (Fake unchanged)")
	}

	calls := f.CallsSnapshot()
	if len(calls) != 3 {
		t.Fatalf("len(Calls) = %d, want 3", len(calls))
	}
	if calls[2].Dir != "/d" || calls[2].Name != "cmd" {
		t.Errorf("Calls[2] = %+v, want {Dir:/d Name:cmd}", calls[2])
	}
}

func TestWithContext_RunDir_PreservesBehavior(t *testing.T) {
	r := WithContext(context.Background(), New())

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "marker"), []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := r.RunDir(dir, "test", "-f", "marker"); err != nil {
		t.Errorf("RunDir(dir, test -f marker) error = %v, want nil (should run in dir)", err)
	}

	other := t.TempDir()
	if err := r.RunDir(other, "test", "-f", "marker"); err == nil {
		t.Errorf("RunDir(other, test -f marker) error = nil, want failure (marker absent in other)")
	}
}

func TestWithContext_Output_Uncanceled(t *testing.T) {
	r := WithContext(context.Background(), New())

	out, err := r.Output("echo", "hi")
	if err != nil {
		t.Fatalf("Output() error = %v, want nil", err)
	}
	if got, want := string(out), "hi\n"; got != want {
		t.Errorf("Output() = %q, want %q", got, want)
	}
}

func TestWithContext_PreCanceled_RunFailsFast(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	r := WithContext(ctx, New())
	start := time.Now()
	if err := r.Run("sleep", "30"); err == nil {
		t.Error("Run() error = nil, want error for pre-canceled context")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("Run() took %v with pre-canceled context, want prompt failure", elapsed)
	}
}
