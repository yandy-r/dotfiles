package rc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderUnit(t *testing.T) {
	u := RenderUnit("/opt/my tools/dotf", "/c/$HOME/rc 100%.toml", "/a:/b")
	for _, want := range []string{
		unitMarker + "\n",
		"Type=oneshot\n",
		"KillMode=process\n",
		"WantedBy=default.target\n",
		"Environment=PATH=/a:/b\n",
		`ExecStart="/opt/my tools/dotf" rc up --config "/c/$$HOME/rc 100%%.toml"` + "\n",
		"TimeoutStartSec=infinity\n",
	} {
		if !strings.Contains(u, want) {
			t.Errorf("unit lacks %q:\n%s", want, u)
		}
	}
	if strings.Contains(u, "After=default.target") || strings.Contains(u, "network-online") {
		t.Errorf("unit has an ordering cycle or a system-only target:\n%s", u)
	}
	if strings.Contains(RenderUnit("/d", "", "/p"), "--config") {
		t.Error("empty configPath still rendered --config")
	}
}

func TestReadUnitState(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "u.service")
	if s, err := ReadUnitState(p); s != UnitAbsent || err != nil {
		t.Fatalf("absent = %v, %v", s, err)
	}
	if err := WriteUnit(p, RenderUnit("/d", "", "/p")); err != nil {
		t.Fatal(err)
	}
	if s, _ := ReadUnitState(p); s != UnitManaged {
		t.Fatalf("managed = %v", s)
	}
	if err := os.WriteFile(p, []byte("[Unit]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if s, _ := ReadUnitState(p); s != UnitForeign {
		t.Fatalf("foreign = %v", s)
	}
}

func TestUnitPath_XDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	if got := UnitPath("/h"); got != "/xdg/systemd/user/"+UnitName {
		t.Errorf("UnitPath = %s", got)
	}
}
