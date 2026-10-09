//go:build unix

package exec

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
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
	t.Cleanup(cancel)
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

	// Also kill the child on a failed assertion, including when a regression
	// leaves it outside the canceled process group.
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	if processExited(t, pid) {
		t.Fatal("sleep child exited before cancellation")
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

	// The sleep grandchild must have exited. Container PID 1 may never reap
	// orphans, so a Linux zombie is also proof of exit; kill(pid, 0) alone
	// reports that an unreaped PID exists, not that it is still running.
	deadline = time.Now().Add(2 * time.Second)
	for !processExited(t, pid) {
		if time.Now().After(deadline) {
			t.Errorf("sleep child pid %d still running after cancel", pid)
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func processExited(t *testing.T, pid int) bool {
	t.Helper()
	err := syscall.Kill(pid, 0)
	if err == syscall.ESRCH {
		return true
	}
	if err != nil {
		t.Fatalf("probe child pid %d: %v", pid, err)
	}
	if runtime.GOOS != "linux" {
		return false
	}

	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "status"))
	if os.IsNotExist(err) || errors.Is(err, syscall.ESRCH) {
		return true // Reaped between the signal probe and the procfs read.
	}
	if err != nil {
		t.Fatalf("read child pid %d status: %v", pid, err)
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if strings.HasPrefix(line, "State:") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				t.Fatalf("malformed child pid %d state: %q", pid, line)
			}
			return fields[1] == "Z"
		}
	}
	t.Fatalf("child pid %d status has no State field", pid)
	return false
}
