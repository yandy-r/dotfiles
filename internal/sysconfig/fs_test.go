package sysconfig

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestOSFileSystem_ReadFile_Rooted(t *testing.T) {
	root := t.TempDir()
	want := []byte("127.0.0.1 localhost\n")
	seedFile(t, root, "etc/hosts", want)

	fs := NewOSFileSystem(root)

	got, err := fs.ReadFile("/etc/hosts")
	if err != nil {
		t.Fatalf("ReadFile(/etc/hosts) error = %v, want nil", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("ReadFile(/etc/hosts) = %q, want %q", got, want)
	}
}

func TestOSFileSystem_Exists_Rooted(t *testing.T) {
	root := t.TempDir()
	seedFile(t, root, "usr/share/zoneinfo/America/New_York", []byte("TZif"))

	fs := NewOSFileSystem(root)

	if !fs.Exists("/usr/share/zoneinfo/America/New_York") {
		t.Error("Exists(seeded zoneinfo) = false, want true")
	}
	if fs.Exists("/usr/share/zoneinfo/Mars/Olympus") {
		t.Error("Exists(missing zoneinfo) = true, want false")
	}
}

func TestOSFileSystem_WriteTemp_Perm0600(t *testing.T) {
	fs := NewOSFileSystem("")
	content := []byte("staged content\n")

	path, err := fs.WriteTemp("dotf-sysconfig", content)
	if err != nil {
		t.Fatalf("WriteTemp() error = %v, want nil", err)
	}
	defer func() { _ = os.Remove(path) }()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", path, err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("temp perm = %o, want 600", perm)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("temp content = %q, want %q", got, content)
	}

	if base := filepath.Base(path); len(base) < len("dotf-sysconfig") {
		t.Errorf("temp base = %q, want dotf-sysconfig prefix", base)
	}
}
