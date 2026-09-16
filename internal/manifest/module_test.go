package manifest_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/manifest"
)

// TestParse_ModuleFields asserts a [[module]] decodes its name/description and
// the typed step kinds (custom-script + a firewall step with ports) off the
// manifest into Module/ModuleStep.
func TestParse_ModuleFields(t *testing.T) {
	const data = `
[[package]]
name = "ufw"
backend = "pkg"
names = { apt = "ufw" }

[[module]]
name = "fingerprint"
description = "PAM fingerprint auth"
steps = [{ custom-script = "bin/setup-fingerprint.sh" }]

[[module]]
name = "ssh"
description = "SSH server"
steps = [
  { pkg = "ufw" },
  { sysconfig = "firewall", ports = ["22/tcp"] },
  { symlink = "ssh/sshd_config" },
]
`

	m, err := manifest.Parse(strings.NewReader(data))
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}

	if len(m.Modules) != 2 {
		t.Fatalf("len(Modules) = %d, want 2", len(m.Modules))
	}

	fp := m.Modules[0]
	if fp.Name != "fingerprint" || fp.Description != "PAM fingerprint auth" {
		t.Errorf("fingerprint module = %+v, want name/description populated", fp)
	}
	if len(fp.Steps) != 1 || fp.Steps[0].CustomScript != "bin/setup-fingerprint.sh" {
		t.Errorf("fingerprint steps = %+v, want one custom-script step", fp.Steps)
	}
	if kind, target, ok := fp.Steps[0].Resolve(); !ok || kind != manifest.StepCustomScript || target != "bin/setup-fingerprint.sh" {
		t.Errorf("Resolve() = (%q, %q, %v), want (custom-script, bin/setup-fingerprint.sh, true)", kind, target, ok)
	}

	ssh := m.Modules[1]
	if len(ssh.Steps) != 3 {
		t.Fatalf("ssh steps = %d, want 3", len(ssh.Steps))
	}
	if ssh.Steps[0].Pkg != "ufw" {
		t.Errorf("ssh step 0 pkg = %q, want ufw", ssh.Steps[0].Pkg)
	}
	if ssh.Steps[1].Sysconfig != "firewall" || len(ssh.Steps[1].Ports) != 1 || ssh.Steps[1].Ports[0] != "22/tcp" {
		t.Errorf("ssh step 1 = %+v, want firewall + [22/tcp]", ssh.Steps[1])
	}
	if ssh.Steps[2].Symlink != "ssh/sshd_config" {
		t.Errorf("ssh step 2 symlink = %q, want ssh/sshd_config", ssh.Steps[2].Symlink)
	}
}

// TestValidate_Module exercises validateModule's rules: exactly-one-kind (zero
// and multiple), unknown pkg reference, sysconfig domain enum, ports-only-on-
// firewall, custom-script containment, name-required, and duplicate names.
func TestValidate_Module(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string // "" means must parse
	}{
		{
			name: "single custom-script step accepted",
			body: `
[[module]]
name = "fingerprint"
steps = [{ custom-script = "bin/setup-fingerprint.sh" }]`,
			wantErr: "",
		},
		{
			name: "step with no kind rejected",
			body: `
[[module]]
name = "empty"
steps = [{ ports = ["22/tcp"] }]`,
			wantErr: "must set exactly one of symlink, pkg, sysconfig, custom-script",
		},
		{
			name: "step with two kinds rejected",
			body: `
[[module]]
name = "twokinds"
steps = [{ pkg = "git", custom-script = "bin/x.sh" }]`,
			wantErr: "must set exactly one kind field, got 2",
		},
		{
			name: "pkg step names unknown package",
			body: `
[[module]]
name = "badpkg"
steps = [{ pkg = "does-not-exist" }]`,
			wantErr: `pkg "does-not-exist" names an unknown package`,
		},
		{
			name: "sysconfig domain must be known",
			body: `
[[module]]
name = "badsys"
steps = [{ sysconfig = "bogus" }]`,
			wantErr: `sysconfig domain "bogus" must be one of firewall, hostname, locale, shell, timezone`,
		},
		{
			name: "ports only valid on firewall",
			body: `
[[module]]
name = "badports"
steps = [{ sysconfig = "timezone", ports = ["22/tcp"] }]`,
			wantErr: "ports is only valid on a sysconfig firewall step",
		},
		{
			name: "ports on firewall accepted",
			body: `
[[module]]
name = "fw"
steps = [{ sysconfig = "firewall", ports = ["22/tcp"] }]`,
			wantErr: "",
		},
		{
			name: "custom-script must be relative",
			body: `
[[module]]
name = "absscript"
steps = [{ custom-script = "/etc/passwd" }]`,
			wantErr: "must be relative",
		},
		{
			name: "custom-script must not escape",
			body: `
[[module]]
name = "escape"
steps = [{ custom-script = "../../etc/passwd" }]`,
			wantErr: "must not contain ..",
		},
		{
			name: "module requires a name",
			body: `
[[module]]
steps = [{ custom-script = "bin/x.sh" }]`,
			wantErr: "module missing required name",
		},
		{
			name: "duplicate module name rejected",
			body: `
[[module]]
name = "dup"
steps = [{ custom-script = "bin/x.sh" }]

[[module]]
name = "dup"
steps = [{ custom-script = "bin/y.sh" }]`,
			wantErr: "duplicate module name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := manifest.Parse(strings.NewReader(tt.body + "\n"))

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Parse() error = %v, want nil", err)
				}
				if m == nil {
					t.Fatal("Parse() manifest = nil, want non-nil")
				}
				return
			}

			if err == nil {
				t.Fatalf("Parse() error = nil, want %q", tt.wantErr)
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
