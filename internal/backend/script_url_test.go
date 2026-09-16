package backend

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

func TestScriptURL_VerifiedThenExecute(t *testing.T) {
	script := []byte("#!/usr/bin/env bash\nset -e\necho installed\n")
	wantSHA := sha256Hex(script)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(script)
	}))
	defer srv.Close()

	origHTTPGet := httpGet
	httpGet = srv.Client().Get
	t.Cleanup(func() { httpGet = origHTTPGet })

	fake := exec.NewFake()
	b := newScriptURLBackend(fake, nil)

	pkg := manifest.Package{
		Name:    "starship",
		Backend: manifest.BackendScriptURL,
		URL:     srv.URL + "/install.sh",
		SHA256:  wantSHA,
	}

	if err := b.Install(pkg, platform.Info{}); err != nil {
		t.Fatalf("Install() error = %v, want nil", err)
	}

	calls := fake.CallsSnapshot()
	if len(calls) != 1 {
		t.Fatalf("Run calls = %d, want 1", len(calls))
	}
	if calls[0].Name != "sh" {
		t.Errorf("Run command = %q, want sh", calls[0].Name)
	}
	if len(calls[0].Args) != 1 || calls[0].Args[0] == "" {
		t.Errorf("Run args = %v, want single script path", calls[0].Args)
	}
}

func TestScriptURL_NoSHA256_FetchesAndRuns(t *testing.T) {
	script := []byte("#!/usr/bin/env bash\nset -e\necho installed\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(script)
	}))
	defer srv.Close()

	origHTTPGet := httpGet
	httpGet = srv.Client().Get
	t.Cleanup(func() { httpGet = origHTTPGet })

	fake := exec.NewFake()
	b := newScriptURLBackend(fake, nil)

	// SHA256 empty: a rotating vendor installer fetches over HTTPS and runs
	// without a checksum gate (VerifySHA256 skips on empty).
	pkg := manifest.Package{
		Name:    "starship",
		Backend: manifest.BackendScriptURL,
		URL:     srv.URL + "/install.sh",
		SHA256:  "",
	}

	if err := b.Install(pkg, platform.Info{}); err != nil {
		t.Fatalf("Install() error = %v, want nil", err)
	}

	calls := fake.CallsSnapshot()
	if len(calls) != 1 {
		t.Fatalf("Run calls = %d, want 1", len(calls))
	}
	if calls[0].Name != "sh" {
		t.Errorf("Run command = %q, want sh", calls[0].Name)
	}
	if len(calls[0].Args) != 1 || calls[0].Args[0] == "" {
		t.Errorf("Run args = %v, want single script path", calls[0].Args)
	}
}

// TestScriptURL_ArgsAppendedToInstaller pins the non-interactive install path:
// pkg.Args must reach the installer as literal argv after the script path.
// Without it starship's install.sh prompts on /dev/tty and exits 1 under a
// headless `dotf apply`.
func TestScriptURL_ArgsAppendedToInstaller(t *testing.T) {
	script := []byte("#!/bin/sh\necho installed\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(script)
	}))
	defer srv.Close()

	origHTTPGet := httpGet
	httpGet = srv.Client().Get
	t.Cleanup(func() { httpGet = origHTTPGet })

	fake := exec.NewFake()
	b := newScriptURLBackend(fake, nil)

	pkg := manifest.Package{
		Name:    "starship",
		Backend: manifest.BackendScriptURL,
		URL:     srv.URL + "/install.sh",
		Args:    []string{"--yes", "--bin-dir", "/usr/local/bin"},
	}

	if err := b.Install(pkg, platform.Info{}); err != nil {
		t.Fatalf("Install() error = %v, want nil", err)
	}

	calls := fake.CallsSnapshot()
	if len(calls) != 1 {
		t.Fatalf("Run calls = %d, want 1", len(calls))
	}
	if calls[0].Name != "sh" {
		t.Errorf("Run command = %q, want sh", calls[0].Name)
	}
	if len(calls[0].Args) != 4 {
		t.Fatalf("Run args = %v, want script path plus 3 args", calls[0].Args)
	}
	want := []string{"--yes", "--bin-dir", "/usr/local/bin"}
	for i, arg := range want {
		if calls[0].Args[i+1] != arg {
			t.Errorf("Run args[%d] = %q, want %q", i+1, calls[0].Args[i+1], arg)
		}
	}
}

func TestScriptURL_SHA256Mismatch_AbortsBeforeExecute(t *testing.T) {
	script := []byte("#!/usr/bin/env bash\necho nope\n")
	wrongSHA := strings.Repeat("0", 64)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(script)
	}))
	defer srv.Close()

	origHTTPGet := httpGet
	httpGet = srv.Client().Get
	t.Cleanup(func() { httpGet = origHTTPGet })

	fake := exec.NewFake()
	b := newScriptURLBackend(fake, nil)

	pkg := manifest.Package{
		Name:    "starship",
		Backend: manifest.BackendScriptURL,
		URL:     srv.URL + "/install.sh",
		SHA256:  wrongSHA,
	}

	err := b.Install(pkg, platform.Info{})
	if err == nil {
		t.Fatal("Install() error = nil, want SHA256 mismatch error")
	}
	if !strings.Contains(err.Error(), "SHA256 mismatch") {
		t.Fatalf("Install() error = %v, want SHA256 mismatch", err)
	}

	if len(fake.CallsSnapshot()) != 0 {
		t.Fatalf("Run calls = %d after mismatch, want 0", len(fake.CallsSnapshot()))
	}
}

func TestScriptURL_Installed_LookPath(t *testing.T) {
	t.Parallel()

	fake := exec.NewFake()
	fake.AddPath("starship", "/usr/local/bin/starship")
	b := newScriptURLBackend(fake, nil)

	pkg := manifest.Package{Name: "starship", Backend: manifest.BackendScriptURL}

	installed, err := b.Installed(pkg, platform.Info{})
	if err != nil {
		t.Fatalf("Installed() error = %v, want nil", err)
	}
	if !installed {
		t.Error("Installed() = false, want true when LookPath resolves")
	}
}

func TestScriptURL_Installed_NotFound(t *testing.T) {
	t.Parallel()

	fake := exec.NewFake()
	b := newScriptURLBackend(fake, nil)

	pkg := manifest.Package{Name: "starship", Backend: manifest.BackendScriptURL}

	installed, err := b.Installed(pkg, platform.Info{})
	if err != nil {
		t.Fatalf("Installed() error = %v, want nil", err)
	}
	if installed {
		t.Error("Installed() = true, want false when LookPath misses")
	}
}

func TestFetchVerifyScript_MatchesFetchVerifyExtractGate(t *testing.T) {
	payload := []byte("#!/bin/sh\necho verify\n")
	wantSHA := sha256Hex(payload)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	origHTTPGet := httpGet
	httpGet = srv.Client().Get
	t.Cleanup(func() { httpGet = origHTTPGet })

	path, err := fetchVerifyScript(srv.URL+"/install.sh", wantSHA)
	if err != nil {
		t.Fatalf("fetchVerifyScript() error = %v, want nil", err)
	}
	defer os.Remove(path)

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("script content = %q, want %q", got, payload)
	}
}
