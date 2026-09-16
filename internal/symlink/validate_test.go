package symlink

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/yandy-r/dotfiles/internal/platform"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T, home string)
		wantValid  bool
		wantBroken bool
		wantErr    bool
	}{
		{
			name: "correct-symlink",
			setup: func(t *testing.T, home string) {
				writeFile(t, filepath.Join(home, "target"), []byte("x"))
				mkSymlink(t, filepath.Join(home, "target"), filepath.Join(home, ".bashrc"))
			},
			wantValid: true,
		},
		{
			name: "dangling-symlink-passes",
			setup: func(t *testing.T, home string) {
				mkSymlink(t, filepath.Join(home, "gone"), filepath.Join(home, ".bashrc"))
			},
			wantValid: true,
		},
		{
			name: "real-file-broken",
			setup: func(t *testing.T, home string) {
				writeFile(t, filepath.Join(home, ".bashrc"), []byte("real"))
			},
			wantBroken: true,
			wantErr:    true,
		},
		{
			name:  "missing-dest",
			setup: func(_ *testing.T, _ string) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			tt.setup(t, home)

			eng := NewEngine(t.TempDir(), home)
			specs := []LinkSpec{{Src: "bash/bashrc", Dest: ".bashrc", Facet: FacetAll}}
			results, err := eng.Validate(specs, platform.Info{OS: platform.OSDebian}, Options{})

			if len(results) != 1 {
				t.Fatalf("len(results) = %d, want 1", len(results))
			}
			if results[0].Valid != tt.wantValid {
				t.Errorf("Valid = %t, want %t", results[0].Valid, tt.wantValid)
			}
			if results[0].Broken != tt.wantBroken {
				t.Errorf("Broken = %t, want %t", results[0].Broken, tt.wantBroken)
			}

			var broken *BrokenLinkError
			gotErr := errors.As(err, &broken)
			if gotErr != tt.wantErr {
				t.Errorf("BrokenLinkError present = %t, want %t (err=%v)", gotErr, tt.wantErr, err)
			}
		})
	}
}

func TestValidate_FailsOnRealFile_ListsDest(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".bashrc"), []byte("real"))
	writeFile(t, filepath.Join(home, ".zshrc"), []byte("real"))

	eng := NewEngine(t.TempDir(), home)
	specs := []LinkSpec{
		{Src: "bash/bashrc", Dest: ".bashrc", Facet: FacetAll},
		{Src: "zsh/zshrc", Dest: ".zshrc", Facet: FacetAll},
	}
	_, err := eng.Validate(specs, platform.Info{OS: platform.OSDebian}, Options{})

	var broken *BrokenLinkError
	if !errors.As(err, &broken) {
		t.Fatalf("Validate() error = %v, want BrokenLinkError", err)
	}
	if len(broken.Dests) != 2 {
		t.Errorf("BrokenLinkError.Dests = %v, want 2 dests", broken.Dests)
	}
}

func TestValidate_AllSymlinks_NoError(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	buildFullSourceTree(t, repoRoot)

	// Apply first so every managed dest becomes a symlink.
	planApply(t, repoRoot, home, DefaultTable(), platform.Info{OS: platform.OSDebian}, Options{})

	eng := NewEngine(repoRoot, home)
	results, err := eng.Validate(DefaultTable(), platform.Info{OS: platform.OSDebian}, Options{})
	if err != nil {
		t.Fatalf("Validate() error = %v, want nil after full apply", err)
	}
	for _, r := range results {
		if !r.Valid {
			t.Errorf("dest %q Valid = false, want true", r.Dest)
		}
	}
}

func TestValidate_ExcludesZed(t *testing.T) {
	repoRoot := t.TempDir()
	home := t.TempDir()
	buildFullSourceTree(t, repoRoot)
	// A real (non-symlink) zed settings file must NOT trigger BrokenLinkError,
	// because the Zed copy dest is excluded from validation.
	writeFile(t, filepath.Join(home, ".config/zed/settings.json"), []byte("real settings"))

	eng := NewEngine(repoRoot, home)
	results, err := eng.Validate(DefaultTable(), platform.Info{OS: platform.OSDebian}, Options{})
	if err != nil {
		t.Fatalf("Validate() error = %v, want nil (zed excluded)", err)
	}
	for _, r := range results {
		if r.Dest == ".config/zed/settings.json" {
			t.Error("zed settings.json appeared in validation results, want excluded")
		}
	}
}

func TestValidate_RunsRegardlessOfDryRun(t *testing.T) {
	home := t.TempDir()
	// A pre-existing real file at a managed dest is reported Broken even though
	// Validate is a read-only check independent of dry-run.
	writeFile(t, filepath.Join(home, ".bashrc"), []byte("real"))

	eng := NewEngine(t.TempDir(), home)
	specs := []LinkSpec{{Src: "bash/bashrc", Dest: ".bashrc", Facet: FacetAll}}
	results, err := eng.Validate(specs, platform.Info{OS: platform.OSDebian}, Options{DryRun: true})

	if len(results) != 1 || !results[0].Broken {
		t.Fatalf("results = %+v, want one Broken result", results)
	}
	var broken *BrokenLinkError
	if !errors.As(err, &broken) {
		t.Errorf("Validate() error = %v, want BrokenLinkError under dry-run", err)
	}
}

// TestValidate_ExcludesNonApplicable confirms faceting also applies in Validate:
// a broken macOS-only dest on Linux is not checked.
func TestValidate_ExcludesNonApplicable(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".config/aerospace"), []byte("real"))

	eng := NewEngine(t.TempDir(), home)
	specs := []LinkSpec{{Src: "aerospace", Dest: ".config/aerospace", IsDir: true, Facet: FacetMacOS}}
	results, err := eng.Validate(specs, platform.Info{OS: platform.OSDebian}, Options{})
	if err != nil {
		t.Fatalf("Validate() error = %v, want nil (macOS entry skipped on Linux)", err)
	}
	if len(results) != 0 {
		t.Errorf("len(results) = %d, want 0 (non-applicable entry)", len(results))
	}
}
