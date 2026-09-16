package sysconfig

import (
	"fmt"
	"os"
	"path/filepath"
)

// FileSystem is the seam for reading system files during Plan and staging
// file installs during Execute. Production uses NewOSFileSystem("") rooted at
// the real /; tests use NewOSFileSystem(t.TempDir()) with seeded /etc/* files.
// This is the one seam sysconfig needs beyond exec.Runner.
type FileSystem interface {
	// ReadFile reads the file at name resolved against the filesystem root.
	ReadFile(name string) ([]byte, error)

	// WriteTemp writes content to a fresh 0600 temp file and returns its path
	// for a subsequent privileged copy. The temp file is NOT rooted — it is a
	// staging file destined for a "sudo cp" to its final path.
	WriteTemp(prefix string, content []byte) (string, error)

	// Exists reports whether name (resolved against the root) exists.
	Exists(name string) bool
}

// osFileSystem is the real FileSystem, rooted at root ("" = the real /). Only
// ReadFile and Exists are rooted; WriteTemp stages to the OS temp directory.
type osFileSystem struct {
	root string
}

// NewOSFileSystem returns a FileSystem rooted at root. An empty root means the
// real filesystem (/); tests pass t.TempDir() to sandbox reads and existence
// checks.
func NewOSFileSystem(root string) FileSystem {
	return osFileSystem{root: root}
}

// ReadFile reads filepath.Join(root, name).
func (fs osFileSystem) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(fs.root, name))
}

// WriteTemp creates a 0600 temp file, writes content to it, and returns its
// path.
func (fs osFileSystem) WriteTemp(prefix string, content []byte) (string, error) {
	f, err := os.CreateTemp("", prefix)
	if err != nil {
		return "", fmt.Errorf("sysconfig: create temp: %w", err)
	}
	name := f.Name()

	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return "", fmt.Errorf("sysconfig: chmod temp %q: %w", name, err)
	}
	if _, err := f.Write(content); err != nil {
		_ = f.Close()
		return "", fmt.Errorf("sysconfig: write temp %q: %w", name, err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("sysconfig: close temp %q: %w", name, err)
	}
	return name, nil
}

// Exists stats filepath.Join(root, name).
func (fs osFileSystem) Exists(name string) bool {
	_, err := os.Stat(filepath.Join(fs.root, name))
	return err == nil
}

var _ FileSystem = osFileSystem{}
