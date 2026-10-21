package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/platform"
	"github.com/yandy-r/dotfiles/internal/symlink"
)

// seedRepoFile writes content to <repoRoot>/<rel>, creating parent dirs, and
// returns the absolute path written.
func seedRepoFile(t *testing.T, repoRoot, rel, content string) string {
	t.Helper()

	abs := filepath.Join(repoRoot, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(abs), err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", abs, err)
	}
	return abs
}

func TestRunLink_DryRun(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	t.Setenv("DOTFILES_ROOT", repoRoot)
	t.Setenv("HOME", home)

	var stdout, stderr bytes.Buffer
	code := runLink([]string{"--dry-run"}, &stdout, &stderr, exec.NewFake())

	if code != 0 {
		t.Fatalf("runLink(--dry-run) code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
	if !strings.HasPrefix(stdout.String(), "dry-run=true\n") {
		t.Errorf("stdout = %q, want dry-run=true header", stdout.String())
	}

	// A dry run must not touch $HOME: no backup dir, no created links.
	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatalf("ReadDir(home) error = %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("dry run created %d entries in $HOME, want 0", len(entries))
	}
}

func TestRunLink_BadFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runLink([]string{"--bogus"}, &stdout, &stderr, exec.NewFake())

	if code != 2 {
		t.Errorf("runLink(--bogus) code = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
}

func TestRunLink_MissingSourceTree(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	t.Setenv("DOTFILES_ROOT", missing)
	t.Setenv("HOME", t.TempDir())

	var stdout, stderr bytes.Buffer
	code := runLink([]string{"--dry-run"}, &stdout, &stderr, exec.NewFake())

	if code != 1 {
		t.Errorf("runLink(missing source tree) code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	if !strings.HasPrefix(stderr.String(), "dotf: ") {
		t.Errorf("stderr = %q, want dotf: prefix", stderr.String())
	}
}

func TestRunLink_AppliesToTempHome(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	src := seedRepoFile(t, repoRoot, "bash/bashrc", "# bashrc\n")
	t.Setenv("DOTFILES_ROOT", repoRoot)
	t.Setenv("HOME", home)

	var stdout, stderr bytes.Buffer
	code := runLink(nil, &stdout, &stderr, exec.NewFake())

	if code != 0 {
		t.Fatalf("runLink() code = %d, want 0; stderr=%q", code, stderr.String())
	}

	dest := filepath.Join(home, ".bashrc")
	target, err := os.Readlink(dest)
	if err != nil {
		t.Fatalf("Readlink(%q) error = %v", dest, err)
	}
	if target != src {
		t.Errorf("Readlink(%q) = %q, want %q", dest, target, src)
	}
	if !strings.Contains(stdout.String(), "link.create=") {
		t.Errorf("stdout = %q, want link.create= line", stdout.String())
	}
}

// TestRunLink_BackupDefault pins the consumer contract of the default-on
// backup: a plain `dotf link` copies a real file aside before replacing it and
// names the backup dir on stderr; --no-backup replaces it without a copy and
// creates no backup dir; --backup with --no-backup is a usage error.
func TestRunLink_BackupDefault(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantBackup bool
	}{
		{"default-backs-up", nil, 0, true},
		{"explicit-backup", []string{"--backup"}, 0, true},
		{"no-backup-opts-out", []string{"--no-backup"}, 0, false},
		{"both-is-usage-error", []string{"--backup", "--no-backup"}, 2, false},
		{"hyprland-no-backup-is-usage-error", []string{"--hyprland", "--no-backup"}, 2, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if slices.Contains(tt.args, "--hyprland") && !symlink.DesktopGuardApplies(platform.Detect(exec.NewFake()).OS) {
				t.Skip("desktop guard does not apply on this host")
			}
			repoRoot := t.TempDir()
			home := t.TempDir()
			seedRepoFile(t, repoRoot, "bash/bashrc", "# repo bashrc\n")
			seedRepoFile(t, home, ".bashrc", "# user bashrc\n")
			t.Setenv("DOTFILES_ROOT", repoRoot)
			t.Setenv("HOME", home)

			var stdout, stderr bytes.Buffer
			code := runLink(tt.args, &stdout, &stderr, exec.NewFake())
			if code != tt.wantCode {
				t.Fatalf("runLink(%v) code = %d, want %d; stderr=%q", tt.args, code, tt.wantCode, stderr.String())
			}
			if tt.wantCode == 2 {
				if fi, err := os.Lstat(filepath.Join(home, ".bashrc")); err != nil || fi.Mode()&os.ModeSymlink != 0 {
					t.Errorf("usage error touched .bashrc (err=%v), want the real file untouched", err)
				}
			}

			backups, err := filepath.Glob(filepath.Join(home, ".config", "dotfiles_backup_*"))
			if err != nil {
				t.Fatalf("Glob error = %v", err)
			}
			if !tt.wantBackup {
				if len(backups) != 0 {
					t.Errorf("backup dirs = %v, want none", backups)
				}
				if strings.Contains(stderr.String(), "link backups:") {
					t.Errorf("stderr = %q, want no backup dir line", stderr.String())
				}
				return
			}
			if len(backups) != 1 {
				t.Fatalf("backup dirs = %v, want exactly one", backups)
			}
			got, err := os.ReadFile(filepath.Join(backups[0], ".bashrc"))
			if err != nil {
				t.Fatalf("ReadFile(backup/.bashrc) error = %v", err)
			}
			if string(got) != "# user bashrc\n" {
				t.Errorf("backup content = %q, want the user's original", got)
			}
			if want := "dotf: link backups: " + backups[0] + "\n"; !strings.Contains(stderr.String(), want) {
				t.Errorf("stderr = %q, want %q", stderr.String(), want)
			}
			if !strings.Contains(stdout.String(), "link.backup=.bashrc") {
				t.Errorf("stdout = %q, want link.backup=.bashrc", stdout.String())
			}
		})
	}
}
