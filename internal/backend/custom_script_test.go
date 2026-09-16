package backend

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

func TestCustomScript_Install_RejectsUnsafePaths(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	b := newCustomScriptBackend(exec.NewFake(), repoRoot)

	tests := []struct {
		name   string
		script string
	}{
		{name: "absolute path", script: "/etc/passwd"},
		{name: "parent traversal", script: "../outside.sh"},
		{name: "embedded traversal", script: "bin/../../outside.sh"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			pkg := manifest.Package{
				Name:    "example",
				Backend: manifest.BackendCustomScript,
				Script:  tc.script,
			}

			err := b.Install(pkg, platform.Info{})
			if err == nil {
				t.Fatal("Install() error = nil, want path rejection")
			}
			if !strings.Contains(err.Error(), "..") && !strings.Contains(err.Error(), "relative") {
				t.Fatalf("Install() error = %v, want path safety rejection", err)
			}
		})
	}
}

func TestCustomScript_Install_ExecutesRepoRelativeScript(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	scriptRel := filepath.Join("bin", "install-example.sh")
	scriptPath := filepath.Join(repoRoot, scriptRel)
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(scriptPath, []byte("#!/usr/bin/env bash\n"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	fake := exec.NewFake()
	b := newCustomScriptBackend(fake, repoRoot)

	pkg := manifest.Package{
		Name:    "example",
		Backend: manifest.BackendCustomScript,
		Script:  filepath.ToSlash(scriptRel),
	}

	if err := b.Install(pkg, platform.Info{}); err != nil {
		t.Fatalf("Install() error = %v, want nil", err)
	}

	calls := fake.CallsSnapshot()
	if len(calls) != 1 {
		t.Fatalf("Calls len = %d, want 1", len(calls))
	}
	// The script runs as `env NON_INTERACTIVE=true bash <abs-script>` so wrapped
	// installers auto-approve prompts headlessly (see Install's doc comment).
	if calls[0].Name != "env" {
		t.Errorf("Calls[0].Name = %q, want env", calls[0].Name)
	}
	if len(calls[0].Args) != 3 {
		t.Fatalf("Calls[0].Args len = %d, want 3 (NON_INTERACTIVE=true bash <script>)", len(calls[0].Args))
	}
	if calls[0].Args[0] != "NON_INTERACTIVE=true" || calls[0].Args[1] != "bash" {
		t.Errorf("Calls[0].Args[:2] = %v, want [NON_INTERACTIVE=true bash]", calls[0].Args[:2])
	}
	wantScript, err := filepath.Abs(scriptPath)
	if err != nil {
		t.Fatalf("Abs() error = %v", err)
	}
	if calls[0].Args[2] != wantScript {
		t.Errorf("Calls[0].Args[2] = %q, want %q", calls[0].Args[2], wantScript)
	}
}

func TestCustomScript_Install_UsesDOTFILES_ROOTWhenRepoRootEmpty(t *testing.T) {
	repoRoot := t.TempDir()
	scriptRel := "bin/install-env.sh"
	scriptPath := filepath.Join(repoRoot, scriptRel)
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(scriptPath, []byte("#!/usr/bin/env bash\n"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Setenv("DOTFILES_ROOT", repoRoot)

	fake := exec.NewFake()
	b := newCustomScriptBackend(fake, "")

	pkg := manifest.Package{
		Name:    "example",
		Backend: manifest.BackendCustomScript,
		Script:  scriptRel,
	}

	if err := b.Install(pkg, platform.Info{}); err != nil {
		t.Fatalf("Install() error = %v, want nil", err)
	}

	calls := fake.CallsSnapshot()
	if len(calls) != 1 {
		t.Fatalf("Calls len = %d, want 1", len(calls))
	}
	if calls[0].Name != "env" {
		t.Errorf("Calls[0].Name = %q, want env", calls[0].Name)
	}
}

// TestCustomScript_Install_ContainsAgainstScriptRoot proves an overlay package's
// Script is resolved and contained under pkg.ScriptRoot (the XDG user config
// root), NOT the backend's repoRoot — so a user-local script under
// ~/.config/dotf/bin/ runs while keeping the no-absolute-path / no-".." invariant.
func TestCustomScript_Install_ContainsAgainstScriptRoot(t *testing.T) {
	t.Parallel()

	scriptRoot := t.TempDir()
	scriptRel := filepath.Join("bin", "install-user.sh")
	scriptPath := filepath.Join(scriptRoot, scriptRel)
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(scriptPath, []byte("#!/usr/bin/env bash\n"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	fake := exec.NewFake()
	// repoRoot is a DIFFERENT dir with no such script; ScriptRoot must win.
	b := newCustomScriptBackend(fake, t.TempDir())

	pkg := manifest.Package{
		Name:       "user-tool",
		Backend:    manifest.BackendCustomScript,
		Script:     filepath.ToSlash(scriptRel),
		ScriptRoot: scriptRoot,
	}

	if err := b.Install(pkg, platform.Info{}); err != nil {
		t.Fatalf("Install() error = %v, want nil", err)
	}

	calls := fake.CallsSnapshot()
	if len(calls) != 1 {
		t.Fatalf("Calls len = %d, want 1", len(calls))
	}
	wantScript, err := filepath.Abs(scriptPath)
	if err != nil {
		t.Fatalf("Abs() error = %v", err)
	}
	if calls[0].Args[2] != wantScript {
		t.Errorf("resolved script = %q, want %q (contained under ScriptRoot)", calls[0].Args[2], wantScript)
	}
}

// TestCustomScript_Install_ScriptRootRejectsUnsafePaths proves the same
// containment fence applies when the root is a user config root: an absolute or
// parent-escaping Script is rejected even with ScriptRoot set.
func TestCustomScript_Install_ScriptRootRejectsUnsafePaths(t *testing.T) {
	t.Parallel()

	scriptRoot := t.TempDir()
	b := newCustomScriptBackend(exec.NewFake(), t.TempDir())

	tests := []struct {
		name   string
		script string
	}{
		{name: "absolute path", script: "/etc/passwd"},
		{name: "parent traversal", script: "../outside.sh"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			pkg := manifest.Package{
				Name:       "user-tool",
				Backend:    manifest.BackendCustomScript,
				Script:     tc.script,
				ScriptRoot: scriptRoot,
			}
			err := b.Install(pkg, platform.Info{})
			if err == nil {
				t.Fatal("Install() error = nil, want path rejection under ScriptRoot")
			}
			if !strings.Contains(err.Error(), "..") && !strings.Contains(err.Error(), "relative") {
				t.Fatalf("Install() error = %v, want path safety rejection", err)
			}
		})
	}
}

// writeCustomScript creates an executable bin/<name> under root and returns the
// repo-relative slash path a custom-script package would carry.
func writeCustomScript(t *testing.T, root, name string) string {
	t.Helper()
	rel := filepath.Join("bin", name)
	abs := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(abs, []byte("#!/usr/bin/env bash\n"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return filepath.ToSlash(rel)
}

// TestCustomScript_Update_AppendsUpdateFlag proves Update reuses the same
// containment + NON_INTERACTIVE invocation as Install, differing only by the
// trailing --update flag, when the package declares the update action.
func TestCustomScript_Update_AppendsUpdateFlag(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	script := writeCustomScript(t, root, "install-claude.sh")
	fake := exec.NewFake()
	b := newCustomScriptBackend(fake, root)

	pkg := manifest.Package{
		Name:    "claude",
		Backend: manifest.BackendCustomScript,
		Script:  script,
		Actions: []string{"update", "remove"},
	}

	if err := b.Update(pkg, platform.Info{}); err != nil {
		t.Fatalf("Update() error = %v, want nil", err)
	}
	calls := fake.CallsSnapshot()
	if len(calls) != 1 {
		t.Fatalf("Calls len = %d, want 1", len(calls))
	}
	if calls[0].Name != "env" {
		t.Errorf("Calls[0].Name = %q, want env", calls[0].Name)
	}
	// env NON_INTERACTIVE=true bash <script> --update
	if len(calls[0].Args) != 4 {
		t.Fatalf("Calls[0].Args = %v, want 4 (NON_INTERACTIVE=true bash <script> --update)", calls[0].Args)
	}
	if calls[0].Args[0] != "NON_INTERACTIVE=true" || calls[0].Args[1] != "bash" {
		t.Errorf("Calls[0].Args[:2] = %v, want [NON_INTERACTIVE=true bash]", calls[0].Args[:2])
	}
	if calls[0].Args[3] != "--update" {
		t.Errorf("Calls[0].Args[3] = %q, want --update", calls[0].Args[3])
	}
}

// TestCustomScript_Remove_AppendsUninstall is the Remove analogue: --uninstall
// trailing flag, gated on the remove action.
func TestCustomScript_Remove_AppendsUninstall(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	script := writeCustomScript(t, root, "install-lua.sh")
	fake := exec.NewFake()
	b := newCustomScriptBackend(fake, root)

	pkg := manifest.Package{
		Name:    "lua",
		Backend: manifest.BackendCustomScript,
		Script:  script,
		Actions: []string{"remove"},
	}

	if err := b.Remove(pkg, platform.Info{}); err != nil {
		t.Fatalf("Remove() error = %v, want nil", err)
	}
	calls := fake.CallsSnapshot()
	if len(calls) != 1 {
		t.Fatalf("Calls len = %d, want 1", len(calls))
	}
	if len(calls[0].Args) != 4 || calls[0].Args[3] != "--uninstall" {
		t.Errorf("Calls[0].Args = %v, want trailing --uninstall", calls[0].Args)
	}
}

// TestCustomScript_Update_GatedByActions proves an undeclared action fails fast
// with ActionUnsupportedError and never runs the script (the capability fence
// that stops --update reaching a script that cannot parse it).
func TestCustomScript_Update_GatedByActions(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	script := writeCustomScript(t, root, "install-coderabbit.sh")
	fake := exec.NewFake()
	b := newCustomScriptBackend(fake, root)

	// Declares only "remove": update must be refused.
	pkg := manifest.Package{
		Name:    "coderabbit",
		Backend: manifest.BackendCustomScript,
		Script:  script,
		Actions: []string{"remove"},
	}

	err := b.Update(pkg, platform.Info{})
	var ue *ActionUnsupportedError
	if !errors.As(err, &ue) {
		t.Fatalf("Update() error = %v, want *ActionUnsupportedError", err)
	}
	if ue.Action != ActionUpdate || ue.Package != "coderabbit" {
		t.Errorf("ActionUnsupportedError = %+v, want {update ... coderabbit}", ue)
	}
	if calls := fake.CallsSnapshot(); len(calls) != 0 {
		t.Errorf("ungated Update ran the script %+v, want no run", calls)
	}
}

// TestCustomScript_Remove_GatedByActions is the Remove analogue of the gate.
func TestCustomScript_Remove_GatedByActions(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	script := writeCustomScript(t, root, "install-opencode.sh")
	fake := exec.NewFake()
	b := newCustomScriptBackend(fake, root)

	// Declares only "update": remove must be refused.
	pkg := manifest.Package{
		Name:    "opencode",
		Backend: manifest.BackendCustomScript,
		Script:  script,
		Actions: []string{"update"},
	}

	err := b.Remove(pkg, platform.Info{})
	var ue *ActionUnsupportedError
	if !errors.As(err, &ue) {
		t.Fatalf("Remove() error = %v, want *ActionUnsupportedError", err)
	}
	if calls := fake.CallsSnapshot(); len(calls) != 0 {
		t.Errorf("ungated Remove ran the script %+v, want no run", calls)
	}
}

// TestCustomScript_Update_Containment proves the update path reuses the same
// path-safety fence as Install: an unsafe Script is rejected even when the update
// action is declared.
func TestCustomScript_Update_Containment(t *testing.T) {
	t.Parallel()

	b := newCustomScriptBackend(exec.NewFake(), t.TempDir())

	for _, script := range []string{"/etc/passwd", "../outside.sh", "bin/../../outside.sh"} {
		pkg := manifest.Package{
			Name:    "claude",
			Backend: manifest.BackendCustomScript,
			Script:  script,
			Actions: []string{"update"},
		}
		err := b.Update(pkg, platform.Info{})
		if err == nil {
			t.Fatalf("Update(%q) error = nil, want path rejection", script)
		}
		if !strings.Contains(err.Error(), "..") && !strings.Contains(err.Error(), "relative") {
			t.Fatalf("Update(%q) error = %v, want path safety rejection", script, err)
		}
	}
}

func TestCustomScript_Installed_LookPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		found bool
		want  bool
	}{
		{name: "binary present", found: true, want: true},
		{name: "binary absent", found: false, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fake := exec.NewFake()
			if tc.found {
				fake.AddPath("atuin", "/usr/local/bin/atuin")
			}

			b := newCustomScriptBackend(fake, t.TempDir())
			pkg := manifest.Package{
				Name:    "atuin",
				Backend: manifest.BackendCustomScript,
				Script:  "bin/install-atuin.sh",
			}

			got, err := b.Installed(pkg, platform.Info{})
			if err != nil {
				t.Fatalf("Installed() error = %v, want nil", err)
			}
			if got != tc.want {
				t.Errorf("Installed() = %v, want %v", got, tc.want)
			}
		})
	}
}
