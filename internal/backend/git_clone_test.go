package backend

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// newGitCloneTest builds a git-clone backend whose home seam resolves to a
// hermetic temp dir and whose fake runner resolves git on PATH. It returns the
// backend, the fake, and the temp home so a test can assert clone argv and probe
// the resolved dest. Each test gets its OWN fake, so the tests are parallel-safe
// (no shared package-level stub is mutated).
func newGitCloneTest(t *testing.T) (*gitCloneBackend, *exec.Fake, string) {
	t.Helper()

	home := t.TempDir()
	fake := exec.NewFake()
	fake.AddPath("git", "/usr/bin/git")

	b := newGitCloneBackend(fake)
	b.home = func() (string, error) { return home, nil }
	return b, fake, home
}

func TestGitClone_Install_ClonesToContainedDest(t *testing.T) {
	t.Parallel()

	b, fake, home := newGitCloneTest(t)
	pkg := manifest.Package{
		Name:    "tpm",
		Backend: manifest.BackendGitClone,
		URL:     "https://github.com/tmux-plugins/tpm",
		Dest:    ".tmux/plugins/tpm",
	}

	if err := b.Install(pkg, platform.Info{}); err != nil {
		t.Fatalf("Install() error = %v, want nil", err)
	}

	dest := filepath.Join(home, ".tmux", "plugins", "tpm")
	calls := fake.CallsSnapshot()
	if len(calls) != 1 {
		t.Fatalf("Calls len = %d, want 1 (single git clone; LookPath is not recorded)", len(calls))
	}
	wantArgs := []string{"clone", "--depth", "1", pkg.URL, dest}
	if calls[0].Name != "git" || !slices.Equal(calls[0].Args, wantArgs) {
		t.Errorf("Calls[0] = {%q %v}, want {git %v}", calls[0].Name, calls[0].Args, wantArgs)
	}
}

func TestGitClone_Install_WithRef(t *testing.T) {
	t.Parallel()

	b, fake, home := newGitCloneTest(t)
	pkg := manifest.Package{
		Name:    "tool",
		Backend: manifest.BackendGitClone,
		URL:     "https://github.com/example/tool",
		Ref:     "v3.0.0",
		Dest:    "code/tool",
	}

	if err := b.Install(pkg, platform.Info{}); err != nil {
		t.Fatalf("Install() error = %v, want nil", err)
	}

	dest := filepath.Join(home, "code", "tool")
	wantArgs := []string{"clone", "--depth", "1", "--branch", "v3.0.0", pkg.URL, dest}
	calls := fake.CallsSnapshot()
	if len(calls) != 1 || calls[0].Name != "git" || !slices.Equal(calls[0].Args, wantArgs) {
		t.Fatalf("git clone argv = %+v, want {git %v}", calls, wantArgs)
	}
}

func TestGitClone_Install_RejectsUnsafeDest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		dest string
	}{
		{name: "absolute", dest: "/etc/cron.d/x"},
		{name: "parent", dest: "../evil"},
		{name: "embedded traversal", dest: "a/../../b"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b, fake, _ := newGitCloneTest(t)
			pkg := manifest.Package{
				Name:    "tool",
				Backend: manifest.BackendGitClone,
				URL:     "https://github.com/example/tool",
				Dest:    tc.dest,
			}

			if err := b.Install(pkg, platform.Info{}); err == nil {
				t.Fatal("Install() error = nil, want unsafe-dest rejection")
			}
			if calls := fake.CallsSnapshot(); len(calls) != 0 {
				t.Errorf("Calls = %+v, want none (clone must not run for an unsafe dest)", calls)
			}
		})
	}
}

func TestGitClone_Install_Idempotent_SkipsWhenDestExists(t *testing.T) {
	t.Parallel()

	b, fake, home := newGitCloneTest(t)
	dest := filepath.Join(home, ".tmux", "plugins", "tpm")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	pkg := manifest.Package{
		Name:    "tpm",
		Backend: manifest.BackendGitClone,
		URL:     "https://github.com/tmux-plugins/tpm",
		Dest:    ".tmux/plugins/tpm",
	}

	if err := b.Install(pkg, platform.Info{}); err != nil {
		t.Fatalf("Install() error = %v, want nil (idempotent no-op)", err)
	}
	if calls := fake.CallsSnapshot(); len(calls) != 0 {
		t.Errorf("Calls = %+v, want none (dest exists => zero git calls)", calls)
	}
}

func TestGitClone_Install_MissingGit_Errors(t *testing.T) {
	t.Parallel()

	b, fake, _ := newGitCloneTest(t)
	fake.Found = map[string]string{} // git absent from PATH, dest absent

	pkg := manifest.Package{
		Name:    "tpm",
		Backend: manifest.BackendGitClone,
		URL:     "https://github.com/tmux-plugins/tpm",
		Dest:    ".tmux/plugins/tpm",
	}

	err := b.Install(pkg, platform.Info{})
	var notFound *GitNotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("Install() error = %v, want *GitNotFoundError", err)
	}
	if calls := fake.CallsSnapshot(); len(calls) != 0 {
		t.Errorf("Calls = %+v, want none (no clone attempted without git)", calls)
	}
}

func TestGitClone_Installed_DestProbe(t *testing.T) {
	t.Parallel()

	b, _, home := newGitCloneTest(t)
	pkg := manifest.Package{
		Name:    "tpm",
		Backend: manifest.BackendGitClone,
		URL:     "https://github.com/tmux-plugins/tpm",
		Dest:    ".tmux/plugins/tpm",
	}

	if got, err := b.Installed(pkg, platform.Info{}); err != nil || got {
		t.Fatalf("Installed() = (%v, %v), want (false, nil) when dest absent", got, err)
	}

	if err := os.MkdirAll(filepath.Join(home, ".tmux", "plugins", "tpm"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if got, err := b.Installed(pkg, platform.Info{}); err != nil || !got {
		t.Fatalf("Installed() = (%v, %v), want (true, nil) when dest present", got, err)
	}
}

func TestGitClone_PostInstall_RunsContainedExec(t *testing.T) {
	t.Parallel()

	b, fake, home := newGitCloneTest(t)
	pkg := manifest.Package{
		Name:    "tool",
		Backend: manifest.BackendGitClone,
		URL:     "https://github.com/example/tool",
		Dest:    "code/tool",
		PostInstall: []manifest.PostInstallStep{
			{Run: []string{"install", "--all"}},
		},
	}

	if err := b.Install(pkg, platform.Info{}); err != nil {
		t.Fatalf("Install() error = %v, want nil", err)
	}

	dest := filepath.Join(home, "code", "tool")
	calls := fake.CallsSnapshot()
	if len(calls) != 2 {
		t.Fatalf("Calls len = %d, want 2 (git clone + post_install exec)", len(calls))
	}

	post := calls[1]
	if want := filepath.Join(dest, "install"); post.Name != want {
		t.Errorf("post_install exec = %q, want %q (contained to dest)", post.Name, want)
	}
	if !slices.Equal(post.Args, []string{"--all"}) {
		t.Errorf("post_install args = %v, want [--all]", post.Args)
	}
	if post.Dir != dest {
		t.Errorf("post_install CWD = %q, want %q (runs from clone dest)", post.Dir, dest)
	}
}

func TestGitClone_PostInstall_RejectsEscapingExec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		exe  string
	}{
		{name: "parent", exe: "../x"},
		{name: "absolute", exe: "/bin/sh"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b, fake, _ := newGitCloneTest(t)
			pkg := manifest.Package{
				Name:    "tool",
				Backend: manifest.BackendGitClone,
				URL:     "https://github.com/example/tool",
				Dest:    "code/tool",
				PostInstall: []manifest.PostInstallStep{
					{Run: []string{tc.exe}},
				},
			}

			if err := b.Install(pkg, platform.Info{}); err == nil {
				t.Fatal("Install() error = nil, want escaping-exec rejection")
			}
			// The clone itself ran (recorded with no working dir); the escaping
			// post_install exec, which alone would carry a RunDir working dir,
			// must NOT have run.
			for _, c := range fake.CallsSnapshot() {
				if c.Dir != "" {
					t.Errorf("a RunDir exec ran (%+v), want none for an escaping post_install", c)
				}
			}
		})
	}
}

// cloneMaterializingRunner wraps exec.Fake to actually create the clone dest on a
// `git clone`, so a test can exercise the post_install-failure ROLLBACK of a real
// directory (the plain Fake records calls but performs no filesystem side effect).
// Each test builds its own, so it stays parallel-safe.
type cloneMaterializingRunner struct {
	*exec.Fake
}

func (c cloneMaterializingRunner) Run(name string, args ...string) error {
	if name == "git" && len(args) >= 2 && args[0] == "clone" {
		_ = os.MkdirAll(args[len(args)-1], 0o755) // materialize the clone dest
	}
	return c.Fake.Run(name, args...)
}

func TestGitClone_Install_CloneFailure_Aborts(t *testing.T) {
	t.Parallel()

	b, fake, home := newGitCloneTest(t)
	dest := filepath.Join(home, "code", "tool")
	fake.RunErrors[exec.CommandKey("git", "clone", "--depth", "1", "https://github.com/example/tool", dest)] = errors.New("clone boom")

	pkg := manifest.Package{
		Name:        "tool",
		Backend:     manifest.BackendGitClone,
		URL:         "https://github.com/example/tool",
		Dest:        "code/tool",
		PostInstall: []manifest.PostInstallStep{{Run: []string{"install"}}},
	}

	err := b.Install(pkg, platform.Info{})
	if err == nil || !strings.Contains(err.Error(), "clone boom") {
		t.Fatalf("Install() error = %v, want the clone failure wrapped", err)
	}
	// A failed clone must NOT run post_install (a RunDir carries a working dir).
	for _, c := range fake.CallsSnapshot() {
		if c.Dir != "" {
			t.Errorf("post_install ran after a failed clone: %+v", c)
		}
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Error("dest exists after a failed clone, want none")
	}
}

func TestGitClone_PostInstall_StepFailure_RollsBackAndAbortsRemaining(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	fake := exec.NewFake()
	fake.AddPath("git", "/usr/bin/git")
	b := newGitCloneBackend(cloneMaterializingRunner{fake})
	b.home = func() (string, error) { return home, nil }

	dest := filepath.Join(home, "code", "tool")
	// The first post_install step fails; the second must never run.
	fake.RunErrors[exec.CommandKey(filepath.Join(dest, "install"))] = errors.New("post-install boom")

	pkg := manifest.Package{
		Name:    "tool",
		Backend: manifest.BackendGitClone,
		URL:     "https://github.com/example/tool",
		Dest:    "code/tool",
		PostInstall: []manifest.PostInstallStep{
			{Run: []string{"install"}},
			{Run: []string{"activate"}},
		},
	}

	err := b.Install(pkg, platform.Info{})
	if err == nil || !strings.Contains(err.Error(), "post-install boom") {
		t.Fatalf("Install() error = %v, want the post_install failure", err)
	}
	// The second step (activate) must NOT have run after the first failed.
	for _, c := range fake.CallsSnapshot() {
		if c.Name == filepath.Join(dest, "activate") {
			t.Error("a later post_install step ran after an earlier failure")
		}
	}
	// Rollback: the half-installed dest is removed, so it does not report
	// installed and a re-apply re-clones + re-runs post_install.
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Errorf("dest not rolled back after post_install failure: stat err = %v", statErr)
	}
	if installed, _ := b.Installed(pkg, platform.Info{}); installed {
		t.Error("Installed() = true after a failed post_install, want false")
	}
}
