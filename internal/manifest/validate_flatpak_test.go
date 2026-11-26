package manifest_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/manifest"
)

func TestValidate_Flatpak(t *testing.T) {
	t.Parallel()

	const base = `
name = "bottles"
backend = "flatpak"
`
	tests := []struct {
		name    string
		pkg     string
		wantErr string
	}{
		{"valid id", base + `flatpak_id = "com.usebottles.bottles"`, ""},
		{"valid id and remote", base + `flatpak_id = "com.usebottles.bottles"` + "\n" + `flatpak_remote = "flathub-beta"`, ""},
		{"missing id", base, "flatpak backend requires flatpak_id"},
		{"two-segment id", base + `flatpak_id = "usebottles.bottles"`, "is not a valid application ID"},
		{"flag-like id", base + `flatpak_id = "--user.a.b"`, "is not a valid application ID"},
		{"flag-like remote", base + `flatpak_id = "com.usebottles.bottles"` + "\n" + `flatpak_remote = "-x"`, "is not a valid remote name"},
		{"names rejected", base + `flatpak_id = "com.usebottles.bottles"` + "\n" + `names = { dnf = "bottles" }`, "names is not valid for the flatpak backend"},
		{"non-managed class rejected", base + `flatpak_id = "com.usebottles.bottles"` + "\n" + `class = "rolling"`, "class"},
		{"flatpak_id on another backend", `
name = "tool"
backend = "pkg"
names = { dnf = "tool" }
flatpak_id = "com.example.Tool"`, "flatpak_id is only valid for the flatpak backend"},
		{"flatpak_remote on another backend", `
name = "tool"
backend = "pkg"
names = { dnf = "tool" }
flatpak_remote = "flathub"`, "flatpak_remote is only valid for the flatpak backend"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := manifest.Parse(strings.NewReader("[[package]]" + tt.pkg + "\n"))
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Parse() error = %v, want nil", err)
				}
				return
			}
			var valErr *manifest.ValidationError
			if !errors.As(err, &valErr) {
				t.Fatalf("Parse() error = %v, want *manifest.ValidationError", err)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Parse() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}
