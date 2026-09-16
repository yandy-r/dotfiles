package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
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
