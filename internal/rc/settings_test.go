package rc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func strs(s ...string) *[]string { return &s }

func readSettings(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("%v\n%s", err, data)
	}
	return m
}

func TestSyncPermissions_CreatesMissing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cfg")
	changed, path, err := SyncPermissions(dir, Permissions{Allow: strs("Edit"), Deny: strs()}, false)
	if err != nil || !changed {
		t.Fatalf("SyncPermissions = %v, %v", changed, err)
	}
	st, _ := os.Stat(path)
	if st.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", st.Mode().Perm())
	}
	perms := readSettings(t, path)["permissions"].(map[string]any)
	if len(perms["allow"].([]any)) != 1 || len(perms["deny"].([]any)) != 0 {
		t.Errorf("permissions = %v", perms)
	}
	if _, ok := perms["ask"]; ok {
		t.Errorf("omitted list was written: %v", perms)
	}
}

func TestSyncPermissions_PreservesAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, SettingsFile)
	orig := `{"model":"opus","cleanupPeriodDays":30.5,"permissions":{"defaultMode":"auto","ask":["Bash(x)"],"allow":["Old"]}}`
	if err := os.WriteFile(path, []byte(orig), 0o640); err != nil {
		t.Fatal(err)
	}
	p := Permissions{Allow: strs("Bash(git *)", "Edit")}
	if changed, _, err := SyncPermissions(dir, p, false); err != nil || !changed {
		t.Fatalf("first sync = %v, %v", changed, err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "30.5") {
		t.Errorf("number not preserved:\n%s", data)
	}
	m := readSettings(t, path)
	perms := m["permissions"].(map[string]any)
	if m["model"] != "opus" || perms["defaultMode"] != "auto" || perms["ask"].([]any)[0] != "Bash(x)" {
		t.Errorf("unrelated keys lost: %v", m)
	}
	if got := perms["allow"].([]any); len(got) != 2 || got[0] != "Bash(git *)" {
		t.Errorf("allow = %v", got)
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o640 {
		t.Errorf("mode = %v, want 0640 preserved", st.Mode().Perm())
	}

	old := time.Now().Add(-time.Hour)
	_ = os.Chtimes(path, old, old)
	if changed, _, err := SyncPermissions(dir, p, false); err != nil || changed {
		t.Fatalf("second sync = %v, %v; want unchanged", changed, err)
	}
	if st, _ := os.Stat(path); !st.ModTime().Equal(old) {
		t.Error("unchanged sync rewrote the file")
	}
}

func TestSyncPermissions_DryRunAndErrors(t *testing.T) {
	dir := t.TempDir()
	changed, path, err := SyncPermissions(dir, Permissions{Allow: strs("Edit")}, true)
	if err != nil || !changed {
		t.Fatalf("dry run = %v, %v", changed, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("dry run wrote the file")
	}
	for name, body := range map[string]string{"invalid": "{nope", "perms not object": `{"permissions":[1]}`} {
		t.Run(name, func(t *testing.T) {
			d := t.TempDir()
			if err := os.WriteFile(filepath.Join(d, SettingsFile), []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := SyncPermissions(d, Permissions{Allow: strs("Edit")}, false); err == nil {
				t.Error("want error")
			}
			if data, _ := os.ReadFile(filepath.Join(d, SettingsFile)); string(data) != body {
				t.Error("file modified on error")
			}
		})
	}
}

func TestPermissionTargets(t *testing.T) {
	p := &Permissions{Allow: strs("Edit")}
	got := PermissionTargets([]Resolved{
		{Profile: DefaultProfile},
		{Profile: "work", ConfigDir: "/w", Permissions: p},
		{Profile: "plain", ConfigDir: "/p"},
		{Profile: "work", ConfigDir: "/w", Permissions: p},
	})
	if len(got) != 1 || got[0].Profile != "work" || got[0].ConfigDir != "/w" {
		t.Errorf("PermissionTargets = %+v", got)
	}
}
