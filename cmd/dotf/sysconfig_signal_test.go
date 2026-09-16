//go:build unix

package main

import (
	"io"
	"syscall"
	"testing"
	"time"
)

// TestConfirmApply_RealSIGINT proves the end-to-end signal wiring, not just the
// context-injection logic that TestConfirmApplyCtx covers: confirmApply installs
// signal.NotifyContext(os.Interrupt) itself, so a real SIGINT delivered while it
// blocks on the prompt must abort it (and be absorbed, not terminate the test
// process). The read side is an io.Pipe that never yields, so the only way out is
// the interrupt.
func TestConfirmApply_RealSIGINT(t *testing.T) {
	pr, pw := io.Pipe()
	defer func() { _ = pw.Close() }()

	// A synchronizing writer: closing promptReady the moment confirmApply emits
	// its prompt guarantees the SIGINT is sent only AFTER NotifyContext has
	// registered (registration happens before the prompt is written), so the
	// signal can never slip through to the default terminate handler.
	promptReady := make(chan struct{})
	stdout := &notifyingWriter{ready: promptReady}

	result := make(chan confirmDecision, 1)
	go func() { result <- confirmApply(stdout, pr) }()

	select {
	case <-promptReady:
	case <-time.After(5 * time.Second):
		t.Fatal("confirmApply never wrote its prompt")
	}

	if err := syscall.Kill(syscall.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("send SIGINT: %v", err)
	}

	select {
	case got := <-result:
		if got != confirmAborted {
			t.Errorf("confirmApply after SIGINT = %v, want confirmAborted", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("confirmApply did not abort within 5s of SIGINT")
	}
}

// notifyingWriter closes ready on the first write, signaling that the prompt has
// been emitted. Subsequent writes (the abort's trailing newline) are discarded.
type notifyingWriter struct {
	ready  chan struct{}
	closed bool
}

func (w *notifyingWriter) Write(p []byte) (int, error) {
	if !w.closed {
		w.closed = true
		close(w.ready)
	}
	return len(p), nil
}
