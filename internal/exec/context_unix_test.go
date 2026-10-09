//go:build unix

package exec

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestWithContext_Cancel_KillsProcessGroup spawns a shell that forks `sleep`
// as a child, cancels the context, and asserts both that Run returns promptly
// and that the grandchild sleep process is dead (group kill).
func TestWithContext_Cancel_KillsProcessGroup(t *testing.T) {
	dir := t.TempDir()
	pidfile := filepath.Join(dir, "sleep.pid")
	script := filepath.Join(dir, "spawn.sh")
	// Script is static; the pid file path arrives as $1 (argv, no
	// interpolation). `sleep &` + `wait` keeps sleep as a shell child.
	content := "#!/bin/sh\nsleep 30 &\necho $! > \"$1\"\nwait\n"
	if err := os.WriteFile(script, []byte(content), 0o700); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	r := WithContext(ctx, New())

	done := make(chan error, 1)
	go func() { done <- r.Run("sh", script, pidfile) }()

	// Wait for the shell to report the sleep child's pid.
	pid := 0
	deadline := time.Now().Add(5 * time.Second)
	for pid == 0 {
		if data, err := os.ReadFile(pidfile); err == nil {
			if n, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
				pid = n
			}
		}
		if pid == 0 {
			if time.Now().After(deadline) {
				cancel()
				t.Fatal("sleep child never started")
			}
			time.Sleep(20 * time.Millisecond)
		}
	}

	start := time.Now()
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Error("Run() error = nil, want cancellation error")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run() did not return within 10s of cancel")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("Run() took %v to return after cancel, want prompt", elapsed)
	}

	// The sleep grandchild must be gone. It may linger briefly as a zombie
	// until init reaps it, so poll for ESRCH.
	deadline = time.Now().Add(2 * time.Second)
	for {
		err := syscall.Kill(pid, 0)
		if err == syscall.ESRCH {
			break
		}
		if time.Now().After(deadline) {
			t.Errorf("sleep child pid %d still alive after cancel (kill(0) = %v)", pid, err)
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
}
