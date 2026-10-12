package manifest_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/manifest"
)

func TestValidate_RollingRepoRoute(t *testing.T) {
	t.Parallel()

	const appArtifact = `
name = "tool"
backend = "app-artifact"
repo = "o/tool"
app = { icon = "https://example.com/tool.png" }
`
	const pkgURL = `
name = "tool"
backend = "pkg-url"
repo = "o/tool"
pkg_name = "tool"
format = "rpm"
`

	tests := []struct {
		name    string
		pkg     string
		wantErr string
	}{
		{"glob asset accepted", appArtifact + `class = "rolling"` + "\n" + `asset = "tool-*.AppImage"`, ""},
		{"preview accepted on the rolling repo route", appArtifact + `class = "rolling"` + "\n" + `asset = "tool-*.AppImage"` + "\npreview = true", ""},
		{"pkg-url glob asset accepted", pkgURL + `class = "rolling"` + "\n" + `asset = "tool-*.x86_64.rpm"`, ""},
		{"empty asset rejected", appArtifact + `class = "rolling"`, "rolling repo route requires asset"},
		{"pkg-url empty asset rejected", pkgURL + `class = "rolling"`, "rolling repo route requires asset"},
		{"malformed glob rejected", appArtifact + `class = "rolling"` + "\n" + `asset = "tool-[.AppImage"`, "is not a valid glob"},
		{"templated asset rejected", appArtifact + `class = "rolling"` + "\n" + `asset = "tool-{{.Arch}}.AppImage"`, "templates are not expanded"},
		{"preview rejected on a pinned repo route", appArtifact + `tag = "v1"` + "\n" + `sha256 = "` + strings.Repeat("a", 64) + `"` + "\n" + `asset = "tool.AppImage"` + "\npreview = true",
			"preview is only valid"},
		{"preview rejected on a tagged rolling entry", appArtifact + `class = "rolling"` + "\n" + `tag = "v1"` + "\n" + `asset = "tool.AppImage"` + "\npreview = true",
			"preview is only valid"},
		{"preview rejected on another backend", `
name = "tool"
backend = "pkg"
preview = true`, "preview is only valid"},
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
