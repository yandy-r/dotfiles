package main

import (
	"io"
	"os"
	"regexp"
	"runtime"
	"sync"
	"testing"
)

// captureOutput redirects os.Stdout and os.Stderr for the duration of fn,
// returning what fn wrote to each along with fn's return value.
func captureOutput(t *testing.T, fn func() int) (stdout, stderr string, code int) {
	t.Helper()

	origStdout, origStderr := os.Stdout, os.Stderr
	t.Cleanup(func() {
		os.Stdout, os.Stderr = origStdout, origStderr
	})

	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() (stdout) error = %v", err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() (stderr) error = %v", err)
	}

	os.Stdout, os.Stderr = outW, errW

	// Drain both pipes concurrently WHILE fn runs. A command whose output
	// exceeds the OS pipe-buffer capacity would otherwise block on write
	// before fn returns, deadlocking a read-after-return pattern. The generated
	// completion scripts already exceed the 8 KiB pipe buffer that some hosts
	// default to, so the reader must not wait for fn to finish.
	var wg sync.WaitGroup
	var outBytes, errBytes []byte
	var outErr, errErr error
	wg.Add(2)
	go func() { defer wg.Done(); outBytes, outErr = io.ReadAll(outR) }()
	go func() { defer wg.Done(); errBytes, errErr = io.ReadAll(errR) }()

	code = fn()

	if err := outW.Close(); err != nil {
		t.Fatalf("outW.Close() error = %v", err)
	}
	if err := errW.Close(); err != nil {
		t.Fatalf("errW.Close() error = %v", err)
	}
	wg.Wait()

	if outErr != nil {
		t.Fatalf("io.ReadAll(stdout) error = %v", outErr)
	}
	if errErr != nil {
		t.Fatalf("io.ReadAll(stderr) error = %v", errErr)
	}

	return string(outBytes), string(errBytes), code
}

func TestRun_NoArgs(t *testing.T) {
	stdout, stderr, code := captureOutput(t, func() int { return run(nil) })

	if code != 1 {
		t.Errorf("run(nil) code = %d, want 1", code)
	}
	if stdout != "" {
		t.Errorf("run(nil) stdout = %q, want empty", stdout)
	}
	if stderr == "" {
		t.Error("run(nil) stderr is empty, want a missing-subcommand message")
	}
}

func TestRun_UnknownSubcommand(t *testing.T) {
	stdout, stderr, code := captureOutput(t, func() int { return run([]string{"bogus"}) })

	if code != 1 {
		t.Errorf("run([bogus]) code = %d, want 1", code)
	}
	if stdout != "" {
		t.Errorf("run([bogus]) stdout = %q, want empty", stdout)
	}
	if stderr == "" {
		t.Error("run([bogus]) stderr is empty, want an unknown-subcommand message")
	}
}

func TestRun_Version(t *testing.T) {
	stdout, stderr, code := captureOutput(t, func() int { return run([]string{"version"}) })

	if code != 0 {
		t.Errorf("run([version]) code = %d, want 0", code)
	}
	if stderr != "" {
		t.Errorf("run([version]) stderr = %q, want empty", stderr)
	}

	want := regexp.MustCompile(`^dotf .+\n$`)
	if !want.MatchString(stdout) {
		t.Errorf("run([version]) stdout = %q, want match of %q", stdout, want.String())
	}
}

func TestRun_Platform(t *testing.T) {
	stdout, stderr, code := captureOutput(t, func() int { return run([]string{"platform"}) })

	if code != 0 {
		t.Errorf("run([platform]) code = %d, want 0", code)
	}
	if stderr != "" {
		t.Errorf("run([platform]) stderr = %q, want empty", stderr)
	}

	want := regexp.MustCompile(`^os=\S+ arch=\S+ pkg=\S+ container=\S+ distro=\S+\n$`)
	if !want.MatchString(stdout) {
		t.Errorf("run([platform]) stdout = %q, want match of %q", stdout, want.String())
	}
}

func TestRun_Install(t *testing.T) {
	stdout, stderr, code := captureOutput(t, func() int { return run([]string{"install", "--dry-run"}) })

	if code != 0 {
		t.Errorf("run([install --dry-run]) code = %d, want 0", code)
	}
	if stderr != "" {
		t.Errorf("run([install --dry-run]) stderr = %q, want empty", stderr)
	}

	want := regexp.MustCompile(`^action=install\ndry-run=true\n`)
	if !want.MatchString(stdout) {
		t.Errorf("run([install --dry-run]) stdout = %q, want match of %q", stdout, want.String())
	}
}

func TestRun_Link(t *testing.T) {
	// Hermetic: point the repo root and $HOME at empty temp dirs so the dry
	// run plans against a controlled tree instead of the developer's real
	// $HOME (which could hold real dotfiles symlinks).
	t.Setenv("DOTFILES_ROOT", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	stdout, stderr, code := captureOutput(t, func() int { return run([]string{"link", "--dry-run"}) })

	if code != 0 {
		t.Errorf("run([link --dry-run]) code = %d, want 0; stderr=%q", code, stderr)
	}
	if stderr != "" {
		t.Errorf("run([link --dry-run]) stderr = %q, want empty", stderr)
	}

	want := regexp.MustCompile(`^dry-run=true\n`)
	if !want.MatchString(stdout) {
		t.Errorf("run([link --dry-run]) stdout = %q, want match of %q", stdout, want.String())
	}
}

// TestRun_Plasma is the dispatch-level counterpart to plasma_test.go's direct
// runPlasma coverage: it proves `plasma` is reachable through run's switch with
// the real os.Stdout/os.Stderr and a real exec.Runner, which the plasma_test.go
// cases — which call runPlasma with buffers and a fake — cannot show.
//
// status is the verb used because it is the only one that neither mutates nor
// signals drift through its exit code, so the assertion stays about dispatch.
func TestRun_Plasma(t *testing.T) {
	// plasma hard-gates on Linux and exits 1 elsewhere, by design. The gate
	// itself is covered by TestRunPlasma_NonLinuxGate with a fake runner; here
	// the runner is real, so there is nothing to assert off Linux.
	if runtime.GOOS != "linux" {
		t.Skipf("dotf plasma is Linux-only; GOOS=%s", runtime.GOOS)
	}

	// Hermetic: repo root, $HOME, and the KDE config dir all point at empty
	// temp dirs, so status reports against a controlled tree instead of the
	// developer's live desktop.
	t.Setenv("DOTFILES_ROOT", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	stdout, stderr, code := captureOutput(t, func() int {
		return run([]string{"plasma", "status", "--dry-run"})
	})

	if code != 0 {
		t.Errorf("run([plasma status --dry-run]) code = %d, want 0; stderr=%q", code, stderr)
	}
	if stderr != "" {
		t.Errorf("run([plasma status --dry-run]) stderr = %q, want empty", stderr)
	}

	want := regexp.MustCompile(`^dry-run=true\n`)
	if !want.MatchString(stdout) {
		t.Errorf("run([plasma status --dry-run]) stdout = %q, want match of %q", stdout, want.String())
	}
}

func TestRun_Apply(t *testing.T) {
	// Hermetic: point the repo root and $HOME at empty temp dirs so the dry
	// run previews against a controlled tree instead of the developer's real
	// $HOME (symlink sources under the empty repo root are skipped).
	t.Setenv("DOTFILES_ROOT", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	stdout, stderr, code := captureOutput(t, func() int { return run([]string{"apply", "--dry-run"}) })

	if code != 0 {
		t.Errorf("run([apply --dry-run]) code = %d, want 0; stderr=%q", code, stderr)
	}
	if stderr != "" {
		t.Errorf("run([apply --dry-run]) stderr = %q, want empty", stderr)
	}

	want := regexp.MustCompile(`^dry-run=true\n`)
	if !want.MatchString(stdout) {
		t.Errorf("run([apply --dry-run]) stdout = %q, want match of %q", stdout, want.String())
	}
}

func TestRun_Sysconfig(t *testing.T) {
	stdout, stderr, code := captureOutput(t, func() int { return run([]string{"sysconfig", "--dry-run"}) })

	if code != 0 {
		t.Errorf("run([sysconfig --dry-run]) code = %d, want 0; stderr=%q", code, stderr)
	}
	if stderr != "" {
		t.Errorf("run([sysconfig --dry-run]) stderr = %q, want empty", stderr)
	}

	want := regexp.MustCompile(`^dry-run=true\n`)
	if !want.MatchString(stdout) {
		t.Errorf("run([sysconfig --dry-run]) stdout = %q, want match of %q", stdout, want.String())
	}
}

func TestRun_Status(t *testing.T) {
	manifestPath := writeStatusTestManifest(t)
	stdout, stderr, code := captureOutput(t, func() int {
		return run([]string{"status", "--manifest", manifestPath})
	})

	if code != 0 {
		t.Errorf("run([status --manifest %q]) code = %d, want 0", manifestPath, code)
	}
	if stderr != "" {
		t.Errorf("run([status --manifest %q]) stderr = %q, want empty", manifestPath, stderr)
	}

	want := regexp.MustCompile(`(?m)^package=\S+ backend=\S+ installed=(true|false)$`)
	if !want.MatchString(stdout) {
		t.Errorf("run([status --manifest %q]) stdout = %q, want match of %q", manifestPath, stdout, want.String())
	}
}

func TestRun_Diff(t *testing.T) {
	manifestPath := writeStatusTestManifest(t)
	stdout, stderr, code := captureOutput(t, func() int {
		return run([]string{"diff", "--manifest", manifestPath})
	})

	if code != 0 && code != 1 {
		t.Errorf("run([diff --manifest %q]) code = %d, want 0 or 1", manifestPath, code)
	}
	if stderr != "" {
		t.Errorf("run([diff --manifest %q]) stderr = %q, want empty", manifestPath, stderr)
	}
	if stdout == "" {
		return
	}

	want := regexp.MustCompile(`(?m)^drift=declared-not-installed package=\S+ backend=\S+$`)
	if !want.MatchString(stdout) {
		t.Errorf("run([diff --manifest %q]) stdout = %q, want match of %q", manifestPath, stdout, want.String())
	}
}

func TestRun_Plan_ManifestResolutionError(t *testing.T) {
	stdout, stderr, code := captureOutput(t, func() int {
		return run([]string{"plan", "--manifest", "/nonexistent/does-not-exist.toml"})
	})

	if code != 1 {
		t.Errorf("run([plan --manifest /nonexistent/does-not-exist.toml]) code = %d, want 1", code)
	}
	if stdout != "" {
		t.Errorf("run([plan --manifest /nonexistent/does-not-exist.toml]) stdout = %q, want empty", stdout)
	}

	want := regexp.MustCompile(`^dotf: `)
	if !want.MatchString(stderr) {
		t.Errorf("run([plan --manifest /nonexistent/does-not-exist.toml]) stderr = %q, want match of %q", stderr, want.String())
	}
}

func TestRun_Plan_BadFlag(t *testing.T) {
	stdout, stderr, code := captureOutput(t, func() int { return run([]string{"plan", "--bogus"}) })

	if code != 2 {
		t.Errorf("run([plan --bogus]) code = %d, want 2", code)
	}
	if stdout != "" {
		t.Errorf("run([plan --bogus]) stdout = %q, want empty", stdout)
	}
	if stderr == "" {
		t.Error("run([plan --bogus]) stderr is empty, want a flag-parse error message")
	}
}

func TestRun_BadFlag_DoesNotExitProcess(t *testing.T) {
	stdout, stderr, code := captureOutput(t, func() int { return run([]string{"-badflag"}) })

	if code != 2 {
		t.Errorf("run([-badflag]) code = %d, want 2", code)
	}
	if stdout != "" {
		t.Errorf("run([-badflag]) stdout = %q, want empty", stdout)
	}
	if stderr == "" {
		t.Error("run([-badflag]) stderr is empty, want a flag-parse error message")
	}
}
