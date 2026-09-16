package plasma

// KConfig's own <path>.lock protocol. It lives apart from the write path
// because it is a protocol implementation — QLockFile's on-disk payload and
// staleness rules — rather than part of this package's own logic, and because
// every live write in applyexec.go is wrapped in it.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// acquireLock creates <path>.lock with QLockFile's on-disk payload so a running
// Plasma respects it, polling until the engine's timeout. A lock older than the
// timeout whose recorded pid is not alive is reclaimed exactly once; anything
// else times out into a *LockError.
//
// The returned release closure must cover the ENTIRE read-modify-write cycle,
// not just the final rename: a lock held only across the write is a lock that
// protects nothing, because the bytes being written were computed from a read
// that no lock covered.
func (e *Engine) acquireLock(path string) (func(), error) {
	lockPath := path + ".lock"
	if err := os.MkdirAll(filepath.Dir(lockPath), dirPerm); err != nil {
		return nil, fmt.Errorf("plasma: create parent dir for %q: %w", lockPath, err)
	}

	deadline := time.Now().Add(e.lockTimeout)
	reclaimed := false
	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, livePerm)
		if err == nil {
			host, _ := os.Hostname()
			_, werr := f.WriteString(strconv.Itoa(os.Getpid()) + "\ndotf\n" + host + "\n")
			cerr := f.Close()
			if werr != nil || cerr != nil {
				_ = os.Remove(lockPath)
				return nil, fmt.Errorf("plasma: write lock %q: %w", lockPath, errors.Join(werr, cerr))
			}
			return func() { _ = os.Remove(lockPath) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("plasma: acquire lock %q: %w", lockPath, err)
		}

		if !reclaimed && staleLock(lockPath, e.lockTimeout) {
			reclaimed = true
			if rerr := os.Remove(lockPath); rerr == nil {
				continue
			}
		}
		if time.Now().After(deadline) {
			return nil, &LockError{Path: lockPath, Held: e.lockTimeout}
		}
		time.Sleep(e.lockPoll)
	}
}

// staleLock reports whether an existing lock file is older than the timeout AND
// records a pid that is no longer alive. Both conditions are required: a
// long-running Plasma holding a fresh lock must never be evicted.
func staleLock(lockPath string, timeout time.Duration) bool {
	fi, err := os.Stat(lockPath)
	if err != nil || time.Since(fi.ModTime()) < timeout {
		return false
	}
	data, err := os.ReadFile(lockPath) //nolint:gosec // engine-derived path
	if err != nil {
		return false
	}
	first, _, _ := strings.Cut(string(data), "\n")
	pid, err := strconv.Atoi(strings.TrimSpace(first))
	if err != nil || pid <= 0 {
		// An unparseable lock older than the timeout is not attributable to a
		// live process, so it is reclaimable.
		return true
	}
	return !pidAlive(pid)
}

// pidAlive reports whether a pid is a live process, via the signal-0 probe.
func pidAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}
