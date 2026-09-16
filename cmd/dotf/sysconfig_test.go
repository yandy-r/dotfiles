package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/sysconfig"
)

// TestConfirmApplyCtx covers the context-injected core of the sysconfig confirm
// prompt: a cancelled context (Ctrl+C) aborts without depending on stdin, and
// the normal y/N/EOF answers keep their historical meaning.
func TestConfirmApplyCtx(t *testing.T) {
	t.Run("cancelled context aborts without consuming stdin", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		// A blocking reader that never yields — proves the abort path does not
		// depend on stdin producing anything.
		pr, pw := io.Pipe()
		defer func() { _ = pw.Close() }()

		var stdout strings.Builder
		if got := confirmApplyCtx(ctx, &stdout, pr); got != confirmAborted {
			t.Errorf("confirmApplyCtx(cancelled) = %v, want confirmAborted", got)
		}
	})

	cases := []struct {
		name string
		in   string
		want confirmDecision
	}{
		{"yes", "y\n", confirmYes},
		{"yes word", "yes\n", confirmYes},
		{"no", "n\n", confirmNo},
		{"empty defaults no", "\n", confirmNo},
		{"eof defaults no", "", confirmNo},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout strings.Builder
			got := confirmApplyCtx(context.Background(), &stdout, strings.NewReader(tc.in))
			if got != tc.want {
				t.Errorf("confirmApplyCtx(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// sysconfigTestManifest is a minimal valid manifest with no [sysconfig] block,
// so resolveSysconfig sees no manifest defaults and a bare invocation leaves
// every domain unmanaged.
const sysconfigTestManifest = "# sysconfig test fixture: no [sysconfig] block\n"

// writeSysconfigManifest writes sysconfigTestManifest to a temp file and
// returns its path.
func writeSysconfigManifest(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "packages.toml")
	if err := os.WriteFile(path, []byte(sysconfigTestManifest), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

// assertNoSudoCalls fails when the fake recorded any sudo invocation, proving
// a read-only path never escalated privilege.
func assertNoSudoCalls(t *testing.T, f *exec.Fake) {
	t.Helper()

	for _, call := range f.CallsSnapshot() {
		if call.Name == "sudo" {
			t.Errorf("recorded a sudo call (%+v), want none on a read-only path", call)
		}
	}
}

func TestRunSysconfig_DryRun(t *testing.T) {
	manifestPath := writeSysconfigManifest(t)
	f := exec.NewFake()

	var stdout, stderr bytes.Buffer
	code := runSysconfig([]string{"--manifest", manifestPath, "--hostname", "web01", "--dry-run"}, &stdout, &stderr, f)

	if code != 0 {
		t.Fatalf("runSysconfig(--dry-run) code = %d, want 0; stderr=%q", code, stderr.String())
	}

	out := stdout.String()
	if !strings.HasPrefix(out, "dry-run=true\n") {
		t.Errorf("stdout = %q, want dry-run=true header", out)
	}
	if !strings.Contains(out, "hostname=web01") {
		t.Errorf("stdout = %q, want hostname=web01", out)
	}

	assertNoSudoCalls(t, f)
}

func TestRunSysconfig_ConvergedNoOp(t *testing.T) {
	manifestPath := writeSysconfigManifest(t)
	f := exec.NewFake()

	var stdout, stderr bytes.Buffer
	code := runSysconfig([]string{"--manifest", manifestPath}, &stdout, &stderr, f)

	if code != 0 {
		t.Fatalf("runSysconfig() code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if got := stdout.String(); got != "sysconfig=converged\n" {
		t.Errorf("stdout = %q, want %q", got, "sysconfig=converged\n")
	}

	assertNoSudoCalls(t, f)
}

func TestRunSysconfig_BadFlag(t *testing.T) {
	f := exec.NewFake()

	var stdout, stderr bytes.Buffer
	code := runSysconfig([]string{"--bogus"}, &stdout, &stderr, f)

	if code != 2 {
		t.Errorf("runSysconfig(--bogus) code = %d, want 2", code)
	}
}

// TestResolveSysconfig_SSHServer covers flag > manifest > skip precedence for
// the ssh-server domain at the command boundary.
func TestResolveSysconfig_SSHServer(t *testing.T) {
	t.Run("flag maps to Config.SSHServer", func(t *testing.T) {
		cfg := resolveSysconfig("", "", "", "", "", "on", nil)
		if cfg.SSHServer != sysconfig.SSHServerOn {
			t.Errorf("SSHServer = %q, want on", cfg.SSHServer)
		}
	})

	t.Run("flag beats manifest", func(t *testing.T) {
		m := &manifest.Sysconfig{SSHServer: "off"}
		cfg := resolveSysconfig("", "", "", "", "", "on", m)
		if cfg.SSHServer != sysconfig.SSHServerOn {
			t.Errorf("SSHServer = %q, want on (flag > manifest)", cfg.SSHServer)
		}
	})

	t.Run("manifest default when flag empty", func(t *testing.T) {
		m := &manifest.Sysconfig{SSHServer: "off"}
		cfg := resolveSysconfig("", "", "", "", "", "", m)
		if cfg.SSHServer != sysconfig.SSHServerOff {
			t.Errorf("SSHServer = %q, want off", cfg.SSHServer)
		}
	})

	t.Run("skip when both empty", func(t *testing.T) {
		cfg := resolveSysconfig("", "", "", "", "", "", nil)
		if cfg.SSHServer != sysconfig.SSHServerUnset {
			t.Errorf("SSHServer = %q, want unset", cfg.SSHServer)
		}
	})
}

// TestRunSysconfig_InvalidSSHServer proves the command boundary fails fast on a
// mistyped --ssh-server value: exit 2 (usage error), a dotf: diagnostic on
// stderr, and zero mutating (sudo) calls because it returns before Plan/
// privilege work.
func TestRunSysconfig_InvalidSSHServer(t *testing.T) {
	manifestPath := writeSysconfigManifest(t)
	f := exec.NewFake()

	var stdout, stderr bytes.Buffer
	code := runSysconfig([]string{"--manifest", manifestPath, "--ssh-server", "bogus"}, &stdout, &stderr, f)

	if code != 2 {
		t.Fatalf("runSysconfig(--ssh-server bogus) code = %d, want 2; stderr=%q", code, stderr.String())
	}
	if stderr.Len() == 0 {
		t.Error("stderr = empty, want a dotf: ssh-server error")
	}
	if !strings.HasPrefix(stderr.String(), "dotf: ") {
		t.Errorf("stderr = %q, want dotf: prefix", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty (no plan/preview on a usage error)", stdout.String())
	}
	assertNoSudoCalls(t, f)
}

// TestRunSysconfig_InvalidFirewall proves the command boundary fails fast on a
// mistyped --firewall value: exit 2 (usage error), a dotf: diagnostic on
// stderr, and zero mutating (sudo) calls because it returns before Plan/
// privilege work.
func TestRunSysconfig_InvalidFirewall(t *testing.T) {
	manifestPath := writeSysconfigManifest(t)
	f := exec.NewFake()

	var stdout, stderr bytes.Buffer
	code := runSysconfig([]string{"--manifest", manifestPath, "--firewall", "bogus"}, &stdout, &stderr, f)

	if code != 2 {
		t.Fatalf("runSysconfig(--firewall bogus) code = %d, want 2; stderr=%q", code, stderr.String())
	}
	if stderr.Len() == 0 {
		t.Error("stderr = empty, want a dotf: firewall error")
	}
	if !strings.HasPrefix(stderr.String(), "dotf: ") {
		t.Errorf("stderr = %q, want dotf: prefix", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty (no plan/preview on a usage error)", stdout.String())
	}
	assertNoSudoCalls(t, f)
}

func TestRunSysconfig_ManifestResolutionError(t *testing.T) {
	f := exec.NewFake()

	var stdout, stderr bytes.Buffer
	code := runSysconfig([]string{"--manifest", "/nonexistent/does-not-exist.toml"}, &stdout, &stderr, f)

	if code != 1 {
		t.Errorf("runSysconfig() code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	if !strings.HasPrefix(stderr.String(), "dotf: ") {
		t.Errorf("stderr = %q, want dotf: prefix", stderr.String())
	}
}

// countSudoCalls returns how many recorded calls match `sudo` with exactly the
// given args.
func countSudoCalls(f *exec.Fake, args ...string) int {
	n := 0
	for _, call := range f.CallsSnapshot() {
		if call.Name != "sudo" || len(call.Args) != len(args) {
			continue
		}
		match := true
		for i := range args {
			if call.Args[i] != args[i] {
				match = false
				break
			}
		}
		if match {
			n++
		}
	}
	return n
}

// sudoCallIndex returns the index of the first recorded `sudo` call with exactly
// the given args, or -1.
func sudoCallIndex(calls []exec.FakeCall, args ...string) int {
	for i, call := range calls {
		if call.Name != "sudo" || len(call.Args) != len(args) {
			continue
		}
		match := true
		for j := range args {
			if call.Args[j] != args[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// TestRunSysconfig_UfwConvergedPrimesBeforePlan proves the idempotency fix: a
// managed ufw firewall primes sudo BEFORE Plan so the `sudo -n ufw status`
// probe succeeds, an already-active+allowed host produces an empty plan, and
// the command reports converged instead of re-enabling ufw. Regression guard
// for issue #11 AC2 (idempotent re-runs).
func TestRunSysconfig_UfwConvergedPrimesBeforePlan(t *testing.T) {
	manifestPath := writeSysconfigManifest(t)
	f := exec.NewFake()
	f.AddCommand("uname", []string{"-s"}, []byte("Linux\n"), nil) // force linux-family dispatch
	f.AddPath("ufw", "/usr/bin/ufw")
	f.AddCommand("sudo", []string{"-n", "ufw", "status"},
		[]byte("Status: active\nOpenSSH                    ALLOW       Anywhere\n"), nil)

	var stdout, stderr bytes.Buffer
	code := runSysconfig([]string{"--manifest", manifestPath, "--firewall", "ufw", "--yes"}, &stdout, &stderr, f)

	if code != 0 {
		t.Fatalf("runSysconfig(--firewall ufw --yes) code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if got := stdout.String(); got != "sysconfig=converged\n" {
		t.Errorf("stdout = %q, want %q", got, "sysconfig=converged\n")
	}

	calls := f.CallsSnapshot()
	primeIdx := sudoCallIndex(calls, "-v")
	statusIdx := sudoCallIndex(calls, "-n", "ufw", "status")
	if primeIdx < 0 {
		t.Fatalf("no `sudo -v` prime recorded; a managed ufw firewall must prime before Plan\ncalls=%+v", calls)
	}
	if statusIdx < 0 {
		t.Fatalf("no `sudo -n ufw status` probe recorded\ncalls=%+v", calls)
	}
	if primeIdx >= statusIdx {
		t.Errorf("prime index = %d, status index = %d; sudo must be primed BEFORE the ufw status read", primeIdx, statusIdx)
	}
}

// TestRunSysconfig_UfwDryRunNoPrimeNoFabricate proves --dry-run never primes
// sudo and, when the non-interactive `sudo -n ufw status` probe fails (unprimed),
// defers the firewall domain instead of fabricating allow/enable steps.
func TestRunSysconfig_UfwDryRunNoPrimeNoFabricate(t *testing.T) {
	manifestPath := writeSysconfigManifest(t)
	f := exec.NewFake()
	f.AddCommand("uname", []string{"-s"}, []byte("Linux\n"), nil)
	f.AddPath("ufw", "/usr/bin/ufw")
	// `sudo -n ufw status` intentionally NOT registered → probe fails → unknown.

	var stdout, stderr bytes.Buffer
	code := runSysconfig([]string{"--manifest", manifestPath, "--firewall", "ufw", "--dry-run"}, &stdout, &stderr, f)

	if code != 0 {
		t.Fatalf("runSysconfig(--firewall ufw --dry-run) code = %d, want 0; stderr=%q", code, stderr.String())
	}

	out := stdout.String()
	if !strings.HasPrefix(out, "dry-run=true\n") {
		t.Errorf("stdout = %q, want dry-run=true header", out)
	}
	if strings.Contains(out, "step.firewall=") {
		t.Errorf("stdout = %q, must not fabricate firewall steps when status is unknown", out)
	}

	if n := countSudoCalls(f, "-v"); n != 0 {
		t.Errorf("`sudo -v` prime count = %d, want 0 on --dry-run", n)
	}
	// The only sanctioned sudo on a read-only path is the non-interactive probe.
	for _, call := range f.CallsSnapshot() {
		if call.Name != "sudo" {
			continue
		}
		if len(call.Args) >= 3 && call.Args[0] == "-n" && call.Args[1] == "ufw" && call.Args[2] == "status" {
			continue
		}
		t.Errorf("unexpected sudo call on --dry-run: sudo %v (only `sudo -n ufw status` is allowed)", call.Args)
	}
}

// TestRunSysconfig_SSHServerDryRunNoMutation proves --dry-run prints the
// resolved ssh-server value and planned ssh-server steps without priming sudo
// or executing mutating commands.
func TestRunSysconfig_SSHServerDryRunNoMutation(t *testing.T) {
	manifestPath := writeSysconfigManifest(t)
	f := exec.NewFake()
	f.AddCommand("uname", []string{"-s"}, []byte("Linux\n"), nil)
	f.AddPath("ufw", "/usr/bin/ufw")
	f.AddCommand("sudo", []string{"-n", "ufw", "status"}, []byte("Status: inactive\n"), nil)
	// Force an enable step even when the host already has sshd installed.
	f.AddCommand("systemctl", []string{"is-active", "ssh"}, []byte("inactive\n"), nil)
	f.AddCommand("systemctl", []string{"is-active", "sshd"}, []byte("inactive\n"), nil)

	var stdout, stderr bytes.Buffer
	code := runSysconfig([]string{"--manifest", manifestPath, "--ssh-server", "on", "--dry-run"}, &stdout, &stderr, f)

	if code != 0 {
		t.Fatalf("runSysconfig(--ssh-server on --dry-run) code = %d, want 0; stderr=%q", code, stderr.String())
	}

	out := stdout.String()
	if !strings.HasPrefix(out, "dry-run=true\n") {
		t.Errorf("stdout = %q, want dry-run=true header", out)
	}
	if !strings.Contains(out, "ssh-server=on") {
		t.Errorf("stdout = %q, want ssh-server=on", out)
	}
	if !strings.Contains(out, "step.ssh-server=") {
		t.Errorf("stdout = %q, want step.ssh-server= lines", out)
	}

	if n := countSudoCalls(f, "-v"); n != 0 {
		t.Errorf("`sudo -v` prime count = %d, want 0 on --dry-run", n)
	}
	for _, call := range f.CallsSnapshot() {
		if call.Name != "sudo" {
			continue
		}
		if len(call.Args) >= 3 && call.Args[0] == "-n" && call.Args[1] == "ufw" && call.Args[2] == "status" {
			continue
		}
		t.Errorf("unexpected sudo call on --dry-run: sudo %v (only `sudo -n ufw status` is allowed)", call.Args)
	}
}

func TestRunSysconfig_NonTTYApplyWithoutYes(t *testing.T) {
	manifestPath := writeSysconfigManifest(t)
	f := exec.NewFake()

	// stdout is a bytes.Buffer (non-TTY); without --yes the apply must refuse.
	var stdout, stderr bytes.Buffer
	code := runSysconfig([]string{"--manifest", manifestPath, "--hostname", "web01"}, &stdout, &stderr, f)

	if code != 1 {
		t.Fatalf("runSysconfig(non-tty apply) code = %d, want 1; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "refusing") {
		t.Errorf("stderr = %q, want a refusing message", stderr.String())
	}

	assertNoSudoCalls(t, f)
}
