package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yandy-r/dotfiles/internal/symlink"
)

// unlinkEnv points DOTFILES_ROOT and HOME at fresh temp dirs with
// bash/bashrc seeded and ~/.bashrc linked to it; it returns (repoRoot, home).
func unlinkEnv(t *testing.T) (string, string) {
	t.Helper()
	repoRoot := t.TempDir()
	home := t.TempDir()
	src := seedRepoFile(t, repoRoot, "bash/bashrc", "# repo bashrc\n")
	if err := os.Symlink(src, filepath.Join(home, ".bashrc")); err != nil {
		t.Fatalf("Symlink error = %v", err)
	}
	t.Setenv("DOTFILES_ROOT", repoRoot)
	t.Setenv("HOME", home)
	return repoRoot, home
}

func runUnlinkT(args ...string) (stdout, stderr string, code int) {
	var out, errb bytes.Buffer
	code = runUnlink(args, &out, &errb)
	return out.String(), errb.String(), code
}

// isSymlink reports whether path is a symlink.
func isSymlink(path string) bool {
	fi, err := os.Lstat(path)
	return err == nil && fi.Mode()&os.ModeSymlink != 0
}

func TestRunUnlink_UsageErrors(t *testing.T) {
	_, home := unlinkEnv(t)
	cases := []struct {
		name string
		args []string
	}{
		{"no-dest-no-all", nil},
		{"dest-with-all", []string{".bashrc", "--all"}},
		{"bad-flag", []string{"--bogus"}},
		{"unknown-dest", []string{".bashrc", ".not-in-table"}},
		{"absolute-outside-home", []string{"/etc/.bashrc"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, code := runUnlinkT(tc.args...)
			if code != 2 {
				t.Fatalf("runUnlink(%v) code = %d, want 2; stderr=%q", tc.args, code, stderr)
			}
			if stdout != "" {
				t.Errorf("stdout = %q, want empty", stdout)
			}
			// Rejected before any change: the managed link survives.
			if !isSymlink(filepath.Join(home, ".bashrc")) {
				t.Errorf(".bashrc link removed by a rejected invocation")
			}
		})
	}
}

func TestRunUnlink_Named(t *testing.T) {
	_, home := unlinkEnv(t)
	realFile := filepath.Join(home, ".zshrc")
	if err := os.WriteFile(realFile, []byte("real\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Dry run first: previews, changes nothing. Flags after operands parse.
	stdout, stderr, code := runUnlinkT("~/.bashrc", ".zshrc", "--dry-run")
	if code != 0 {
		t.Fatalf("dry run code = %d; stderr=%q", code, stderr)
	}
	if !strings.HasPrefix(stdout, "dry-run=true\n") || !strings.Contains(stdout, "unlink.remove=.bashrc\n") {
		t.Errorf("dry-run stdout = %q", stdout)
	}
	if !isSymlink(filepath.Join(home, ".bashrc")) {
		t.Fatalf("dry run removed .bashrc")
	}

	stdout, stderr, code = runUnlinkT(".bashrc", ".zshrc")
	if code != 0 {
		t.Fatalf("code = %d; stderr=%q", code, stderr)
	}
	for _, want := range []string{"unlink.remove=.bashrc\n", "unlink.skip=.zshrc\n"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
	}
	for _, want := range []string{"dotf: unlink: skip .zshrc: real file", "dotf: unlink summary: removed=1 skipped=1 restored=0"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr = %q, want %q", stderr, want)
		}
	}
	if _, err := os.Lstat(filepath.Join(home, ".bashrc")); !os.IsNotExist(err) {
		t.Errorf(".bashrc still present (err=%v)", err)
	}
	if data, err := os.ReadFile(realFile); err != nil || string(data) != "real\n" {
		t.Errorf("real .zshrc touched: %q, %v", data, err)
	}
}

func TestRunUnlink_Restore(t *testing.T) {
	_, home := unlinkEnv(t)
	seedBackup(t, home, "1700000000", time.Unix(1700000000, 0), false, map[string]string{".bashrc": "# old user bashrc\n"})
	seedBackup(t, home, "1800000000", time.Unix(1800000000, 0), false, map[string]string{".bashrc": "# newest user bashrc\n"})

	stdout, stderr, code := runUnlinkT("--restore", ".bashrc")
	if code != 0 {
		t.Fatalf("code = %d; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "unlink.restore=.bashrc\n") {
		t.Errorf("stdout = %q, want unlink.restore=.bashrc", stdout)
	}
	dest := filepath.Join(home, ".bashrc")
	if isSymlink(dest) {
		t.Fatalf(".bashrc is still a symlink")
	}
	if data, err := os.ReadFile(dest); err != nil || string(data) != "# newest user bashrc\n" {
		t.Errorf(".bashrc = %q, %v; want the newest backup's copy", data, err)
	}
}

// TestRunUnlink_AbsoluteDest pins the shell-expanded form of the documented
// `dotf unlink ~/.bashrc --restore`: the shell passes an absolute path under
// $HOME, which names the same table dest.
func TestRunUnlink_AbsoluteDest(t *testing.T) {
	_, home := unlinkEnv(t)
	seedBackup(t, home, "1800000000", time.Unix(1800000000, 0), false, map[string]string{".bashrc": "# user bashrc\n"})

	dest := filepath.Join(home, ".bashrc")
	stdout, stderr, code := runUnlinkT(dest, "--restore")
	if code != 0 {
		t.Fatalf("code = %d; stderr=%q", code, stderr)
	}
	for _, want := range []string{"unlink.remove=.bashrc\n", "unlink.restore=.bashrc\n"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
	}
	if data, err := os.ReadFile(dest); isSymlink(dest) || err != nil || string(data) != "# user bashrc\n" {
		t.Errorf(".bashrc = %q, %v; want the backup's copy", data, err)
	}
}

func TestRunUnlink_RestoreNoBackup(t *testing.T) {
	_, home := unlinkEnv(t)
	stdout, stderr, code := runUnlinkT("--all", "--restore")
	if code != 0 {
		t.Fatalf("code = %d; stderr=%q", code, stderr)
	}
	if strings.Contains(stdout, "unlink.restore=") {
		t.Errorf("stdout = %q, want no restore line", stdout)
	}
	if !strings.Contains(stderr, "no backup for .bashrc; left unlinked") {
		t.Errorf("stderr = %q, want the no-backup notice", stderr)
	}
	if _, err := os.Lstat(filepath.Join(home, ".bashrc")); !os.IsNotExist(err) {
		t.Errorf(".bashrc still present (err=%v)", err)
	}
}

// TestPrintUnlinkReport_RestoreNotices pins the --restore notices: a removed
// dest with no backup is "left unlinked", one with a backup whose restore was
// not performed gets the retry hint, and unlink.restore lists only Restored.
func TestPrintUnlinkReport_RestoreNotices(t *testing.T) {
	report := &symlink.UnlinkReport{Items: []symlink.UnlinkItem{
		{Dest: ".bashrc", Action: symlink.UnlinkActionRemove, BackupID: "1800000000", Restored: true},
		{Dest: ".zshrc", Action: symlink.UnlinkActionRemove, BackupID: "1800000000"},
		{Dest: ".config/nvim", Action: symlink.UnlinkActionRemove},
	}}
	var stdout, stderr bytes.Buffer
	if err := printUnlinkReport(&stdout, &stderr, report, true, false); err != nil {
		t.Fatalf("printUnlinkReport() error = %v", err)
	}
	if want := "unlink.remove=.bashrc,.config/nvim,.zshrc\nunlink.restore=.bashrc\n"; stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
	for _, want := range []string{
		"dotf: unlink: no backup for .config/nvim; left unlinked\n",
		"dotf: unlink: restore of .zshrc from backup 1800000000 not performed; left unlinked (retry: dotf backup restore 1800000000 .zshrc)\n",
		"dotf: unlink summary: removed=3 skipped=0 restored=1\n",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("stderr = %q, want %q", stderr.String(), want)
		}
	}
	if strings.Contains(stderr.String(), ".bashrc") {
		t.Errorf("stderr = %q, want no notice for the restored .bashrc", stderr.String())
	}
}

// TestRunUnlink_AllQuietOnMissing proves --all lists only what it removed or
// refused, not the many table dests absent on this host.
func TestRunUnlink_AllQuietOnMissing(t *testing.T) {
	unlinkEnv(t)
	stdout, stderr, code := runUnlinkT("--all")
	if code != 0 {
		t.Fatalf("code = %d; stderr=%q", code, stderr)
	}
	if stdout != "unlink.remove=.bashrc\n" {
		t.Errorf("stdout = %q, want only unlink.remove=.bashrc", stdout)
	}
	if strings.Contains(stderr, ": missing") {
		t.Errorf("stderr lists missing dests under --all: %q", stderr)
	}
}
