package exec

import (
	"context"
	"os"

	osexec "os/exec"
)

// WithContext returns a Runner that executes commands under ctx: canceling
// ctx kills the in-flight process (and its whole process group on unix)
// and waits for it to exit before the call returns. Output/Run/RunDir
// otherwise mirror osRunner semantics (stdio wiring, RunDir working dir).
//
// Non-OS runners (Fake, test doubles) are returned unchanged: they never
// spawn processes, so there is nothing to cancel.
func WithContext(ctx context.Context, r Runner) Runner {
	if _, ok := r.(osRunner); !ok {
		// ponytail: ctx ignored for non-OS runners; add a delegating wrapper
		// if a custom blocking Runner ever needs cancellation.
		return r
	}
	return ctxRunner{ctx: ctx}
}

// ctxRunner implements Runner by building each command via commandContext.
type ctxRunner struct{ ctx context.Context }

// Output runs commandContext(ctx, name, args...).Output().
func (c ctxRunner) Output(name string, args ...string) ([]byte, error) {
	return commandContext(c.ctx, name, args...).Output()
}

// LookPath calls os/exec.LookPath (not cancelable, mirroring osRunner).
func (c ctxRunner) LookPath(name string) (string, error) {
	return osexec.LookPath(name)
}

// Run is osRunner.Run under ctx.
func (c ctxRunner) Run(name string, args ...string) error {
	cmd := commandContext(c.ctx, name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// RunDir is osRunner.RunDir under ctx.
func (c ctxRunner) RunDir(dir, name string, args ...string) error {
	cmd := commandContext(c.ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

var _ Runner = ctxRunner{}
