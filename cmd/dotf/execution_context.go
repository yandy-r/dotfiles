package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/yandy-r/dotfiles/internal/backend"
	"github.com/yandy-r/dotfiles/internal/exec"
)

// withExecutionSignals wires cancellation for the install/apply execution
// phases: a context canceled by SIGINT/SIGTERM/SIGHUP is installed as the
// backend download context, and r is wrapped with exec.WithContext so a cancel
// kills the in-flight child (and its process group) instead of leaving it
// running. The returned stop restores signal delivery and download context, so
// a later interactive prompt (sysconfig/firewall confirmations, deb prompts)
// keeps its own Ctrl+C semantics — see the apply/install call sites.
//
// r may be nil for a context-only scope (no command execution).
func withExecutionSignals(r exec.Runner) (context.Context, exec.Runner, func()) {
	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	restoreDownload := backend.WithDownloadContext(ctx)
	if r != nil {
		r = exec.WithContext(ctx, r)
	}
	return ctx, r, func() {
		stop()
		restoreDownload()
	}
}

// ctxAbortExitCode reports whether ctx was canceled (a user signal during the
// execution phase), for the caller's deferred exit-130 mapping.
func ctxAbortExitCode(ctx context.Context) bool {
	return ctx.Err() != nil
}
