package plasma

// The tests for lock.go — KConfig's <path>.lock protocol. They live apart from
// apply_test.go for the same reason lock.go lives apart from applyexec.go: the
// payload shape and the staleness rules are QLockFile's contract, not this
// package's own logic, and every live write depends on getting them right.

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestAcquireLock covers the QLockFile payload, release, contention timeout,
// and stale reclamation.
func TestAcquireLock(t *testing.T) {
	e, _, configDir := testEngine(t)
	path := filepath.Join(configDir, "kwinrc")

	release, err := e.acquireLock(path)
	if err != nil {
		t.Fatalf("acquireLock error = %v", err)
	}
	payload := readFile(t, path+".lock")
	lines := strings.Split(payload, "\n")
	if len(lines) < 3 || lines[1] != "dotf" {
		t.Errorf("lock payload = %q, want QLockFile's pid/appname/hostname shape", payload)
	}

	// A live lock is contended, not stolen.
	if _, err := e.acquireLock(path); err == nil {
		t.Fatal("acquireLock succeeded while the lock was held")
	} else {
		var lockErr *LockError
		if !errors.As(err, &lockErr) {
			t.Fatalf("error = %v, want *LockError", err)
		}
	}

	release()
	if _, err := os.Stat(path + ".lock"); !os.IsNotExist(err) {
		t.Errorf("the lock file survived release: %v", err)
	}
}

func TestAcquireLock_ReclaimsStaleLock(t *testing.T) {
	e, _, configDir := testEngine(t)
	path := filepath.Join(configDir, "kwinrc")
	lockPath := path + ".lock"

	// A dead pid in a lock older than the timeout is reclaimable. Pid 0 is
	// never a live user process and parses cleanly.
	writeFile(t, lockPath, "0\nplasmashell\nsomehost\n")
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatalf("Chtimes error = %v", err)
	}

	release, err := e.acquireLock(path)
	if err != nil {
		t.Fatalf("acquireLock did not reclaim a stale lock: %v", err)
	}
	release()
}

func TestStaleLock_KeepsFreshAndLiveLocks(t *testing.T) {
	dir := t.TempDir()
	fresh := filepath.Join(dir, "fresh.lock")
	writeFile(t, fresh, "0\ndotf\nhost\n")
	if staleLock(fresh, time.Hour) {
		t.Error("a fresh lock was treated as stale")
	}

	live := filepath.Join(dir, "live.lock")
	writeFile(t, live, strconv.Itoa(os.Getpid())+"\ndotf\nhost\n")
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(live, old, old); err != nil {
		t.Fatalf("Chtimes error = %v", err)
	}
	if staleLock(live, time.Minute) {
		t.Error("a lock held by a LIVE process was treated as stale")
	}
}
