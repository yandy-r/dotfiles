// Package exec provides a mockable seam around external command execution.
//
// Production code should depend on the Runner interface rather than calling
// os/exec directly. This allows tests throughout the codebase to substitute
// Fake (see fake.go) for deterministic, hermetic behavior.
//
// NEVER shell out via `sh -c` with interpolated strings. Always pass
// arguments as a slice (the second parameter to Output) so that user-
// controlled input cannot be reinterpreted by a shell.
package exec

import (
	"os"

	osexec "os/exec"
)

// Runner executes external commands. Implementations must not invoke a
// shell with interpolated strings; arguments are always passed as a slice.
//
// Runner is intentionally defined and exported here (rather than at each
// consumer) because it is a deliberately shared test seam: other packages
// depend on this interface and substitute Fake (see fake.go) to get
// hermetic, deterministic tests without invoking real external commands.
type Runner interface {
	// Output runs name with args and returns its standard output.
	Output(name string, args ...string) ([]byte, error)

	// LookPath reports the resolved path of name, mirroring the behavior of
	// the `command -v` shell builtin. It returns an error if name cannot be
	// found.
	LookPath(name string) (string, error)

	// Run executes name with args, wiring stdin/stdout/stderr to the
	// process's standard streams.
	Run(name string, args ...string) error

	// RunDir is Run with the command's working directory set to dir, so a
	// step that expects a specific CWD (a git-clone post_install exec runs
	// from inside the clone dest) is not tied to the caller's CWD.
	RunDir(dir, name string, args ...string) error
}

// osRunner is the real Runner implementation backed by os/exec.
type osRunner struct{}

// New returns a Runner backed by the real os/exec package.
func New() Runner {
	return osRunner{}
}

// Output runs exec.Command(name, args...).Output().
func (osRunner) Output(name string, args ...string) ([]byte, error) {
	return osexec.Command(name, args...).Output()
}

// LookPath calls os/exec.LookPath.
func (osRunner) LookPath(name string) (string, error) {
	return osexec.LookPath(name)
}

// Run executes exec.Command(name, args...) with stdin/stdout/stderr wired to
// the process's standard streams.
func (osRunner) Run(name string, args ...string) error {
	cmd := osexec.Command(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// RunDir executes exec.Command(name, args...) from working directory dir, with
// stdin/stdout/stderr wired to the process's standard streams.
func (osRunner) RunDir(dir, name string, args ...string) error {
	cmd := osexec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

var _ Runner = osRunner{}
