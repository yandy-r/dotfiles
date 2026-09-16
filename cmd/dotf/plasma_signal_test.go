//go:build unix

package main

import (
	"bytes"
	"io"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/yandy-r/dotfiles/internal/plasma"
)

// These tests deliver a REAL SIGINT to the test process, so they must not run
// concurrently with anything that would be surprised by one: no t.Parallel
// anywhere in this file.

// TestConfirmPlasmaApply_RealSIGINT is the plasma counterpart of
// TestConfirmApply_RealSIGINT and proves the same end-to-end signal wiring for
// this command's own prompt: confirmPlasmaApply installs
// signal.NotifyContext(os.Interrupt) itself, so a real SIGINT delivered while it
// blocks on the prompt must abort it (and be absorbed, not terminate the test
// process). The read side is an io.Pipe that never yields, so the only way out
// is the interrupt.
//
// It is a separate test from sysconfig's, not a shared helper, because
// confirmPlasmaApply deliberately does NOT call confirmApplyCtx — it owns its
// own prompt text and therefore its own NotifyContext call, which is exactly the
// wiring under test.
func TestConfirmPlasmaApply_RealSIGINT(t *testing.T) {
	pr, pw := io.Pipe()
	defer func() { _ = pw.Close() }()

	// The same synchronizing writer sysconfig's test uses: closing promptReady
	// on the first write guarantees the SIGINT is sent only AFTER NotifyContext
	// has registered (registration happens before the prompt is written), so
	// the signal can never slip through to the default terminate handler.
	promptReady := make(chan struct{})
	stdout := &notifyingWriter{ready: promptReady}

	result := make(chan confirmDecision, 1)
	go func() { result <- confirmPlasmaApply(stdout, pr) }()

	select {
	case <-promptReady:
	case <-time.After(5 * time.Second):
		t.Fatal("confirmPlasmaApply never wrote its prompt")
	}

	if err := syscall.Kill(syscall.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("send SIGINT: %v", err)
	}

	select {
	case got := <-result:
		if got != confirmAborted {
			t.Errorf("confirmPlasmaApply after SIGINT = %v, want confirmAborted", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("confirmPlasmaApply did not abort within 5s of SIGINT")
	}
}

// TestPlasmaConfirm_SIGINTExits130 carries the previous test's decision through
// the CLI layer to the number a shell actually sees: a Ctrl+C at the plasma
// apply prompt must produce the single line "dotf: aborted" on stderr and exit
// 130 (128 + SIGINT), never the soft-failure 1 that a decline gets.
//
// It drives the real plasmaRun.confirm, so all three of its gates are exercised
// in place — the --yes bypass is off, the TTY check passes, and the abort branch
// is reached by a genuine signal rather than a hand-made confirmDecision:
//
//   - stdout is os.DevNull, a CHARACTER device, which is precisely what
//     isTTY tests for (os.ModeCharDevice). That satisfies stdoutIsTTY without a
//     pseudo-terminal or a new dependency.
//   - stdin is a blockingReader, whose first Read closes readStarted. Read is
//     called strictly after the prompt is written, which is strictly after
//     NotifyContext registered, so it is the same "registration has happened"
//     barrier notifyingWriter provides above — needed here because stdout is
//     /dev/null and its writes are unobservable.
func TestPlasmaConfirm_SIGINTExits130(t *testing.T) {
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	defer func() { _ = devNull.Close() }()
	if !stdoutIsTTY(devNull) {
		t.Skipf("%s is not a character device on this host; the TTY gate cannot be satisfied", os.DevNull)
	}

	stdin := newBlockingReader()
	defer stdin.release()

	var stderr bytes.Buffer
	p := &plasmaRun{
		stdout: devNull,
		stderr: &stderr,
		stdin:  stdin,
		o:      &plasmaOpts{},
		popts:  plasma.Options{},
	}

	type outcome struct {
		code int
		ok   bool
	}
	done := make(chan outcome, 1)
	go func() {
		code, ok := p.confirm()
		done <- outcome{code: code, ok: ok}
	}()

	select {
	case <-stdin.readStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("confirm never read stdin, so the prompt was never written")
	}

	if err := syscall.Kill(syscall.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("send SIGINT: %v", err)
	}

	select {
	case got := <-done:
		if got.ok {
			t.Error("confirm after SIGINT returned ok=true, want a refusal")
		}
		if got.code != abortExitCode {
			t.Errorf("confirm after SIGINT = %d, want %d", got.code, abortExitCode)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("confirm did not abort within 5s of SIGINT")
	}

	if want := "dotf: aborted\n"; stderr.String() != want {
		t.Errorf("stderr = %q, want exactly %q", stderr.String(), want)
	}
}

// blockingReader is a stdin stand-in whose first Read announces itself and then
// blocks until released, so a test can synchronize on "the prompt has been
// written and the reader is waiting" without ever satisfying the read.
type blockingReader struct {
	readStarted chan struct{}
	released    chan struct{}
	once        sync.Once
}

// newBlockingReader returns a blockingReader ready for one synchronized read.
func newBlockingReader() *blockingReader {
	return &blockingReader{
		readStarted: make(chan struct{}),
		released:    make(chan struct{}),
	}
}

// Read closes readStarted on its first call, then blocks until release, after
// which it reports EOF so the parked goroutine unwinds instead of leaking.
func (r *blockingReader) Read([]byte) (int, error) {
	r.once.Do(func() { close(r.readStarted) })
	<-r.released
	return 0, io.EOF
}

// release unblocks any parked Read. It is idempotent-by-construction only when
// called once, which the deferred single call in each test guarantees.
func (r *blockingReader) release() { close(r.released) }
