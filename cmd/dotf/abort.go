package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/yandy-r/dotfiles/internal/tui"
)

// abortExitCode is the process exit code for a user-initiated abort of an
// interactive step (Ctrl+C or Esc). It follows the Unix convention of
// 128 + SIGINT(2) = 130, kept deliberately distinct from the repo's 0/1/2/3
// result codes so a cancellation is never mistaken for a soft failure (1), a
// usage error (2), or a hard precondition abort (3).
const abortExitCode = 130

// reportAbort writes a single clean line to stderr and returns abortExitCode.
// Unlike reportErr it wraps no error: a user cancellation is not a failure, so
// it gets no "progName: <err>" diagnostic and no stack.
func reportAbort(stderr io.Writer) int {
	_, _ = fmt.Fprintf(stderr, "%s: aborted\n", progName)
	return abortExitCode
}

// wizardExit maps a tui.Run error to an exit code: a user cancellation
// (Ctrl+C/Esc, surfaced as tui.ErrAborted) becomes a clean abort (130), while
// any other form failure keeps the standard reportErr diagnostic (1). It is the
// single decision point shared by the plan and apply guided paths.
func wizardExit(stderr io.Writer, err error) int {
	if errors.Is(err, tui.ErrAborted) {
		return reportAbort(stderr)
	}
	return reportErr(stderr, err)
}
