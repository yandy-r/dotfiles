package rc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
)

// SettingsFile is the user settings file inside a Claude config dir: the one
// sessions spawned by a Remote Control server read. (Flags such as --settings
// are not forwarded to spawned sessions, so permission rules must live here.)
const SettingsFile = "settings.json"

// SyncPermissions makes <configDir>/settings.json hold exactly p's non-nil
// rule lists under "permissions", preserving every other key. It writes only
// when a list changes, and never in a dry run. It returns whether a change
// was (or, in a dry run, would be) made and the settings path.
func SyncPermissions(configDir string, p Permissions, dryRun bool) (changed bool, path string, err error) {
	path = filepath.Join(configDir, SettingsFile)
	doc := map[string]any{}
	mode := fs.FileMode(0o600)
	data, err := os.ReadFile(path) //nolint:gosec // path derived from the user's rc.toml
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return false, path, fmt.Errorf("read %s: %w", path, err)
	default:
		if st, err := os.Stat(path); err == nil {
			mode = st.Mode().Perm()
		}
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.UseNumber() // keep numbers exactly as written
		if err := dec.Decode(&doc); err != nil {
			return false, path, fmt.Errorf("parse %s: %w", path, err)
		}
		if doc == nil { // the file holds `null`
			doc = map[string]any{}
		}
	}

	perms := map[string]any{}
	if raw, ok := doc["permissions"]; ok && raw != nil {
		m, ok := raw.(map[string]any)
		if !ok {
			return false, path, fmt.Errorf("%s: \"permissions\" is not an object", path)
		}
		perms = m
	}
	keys := []string{"allow", "deny", "ask"}
	lists := p.lists()
	for _, k := range keys {
		list := lists[k]
		if list == nil {
			continue
		}
		want := make([]any, 0, len(*list))
		for _, rule := range *list {
			want = append(want, rule)
		}
		if have, ok := perms[k]; ok && reflect.DeepEqual(have, want) {
			continue
		}
		perms[k] = want
		changed = true
	}
	if !changed || dryRun {
		return changed, path, nil
	}
	doc["permissions"] = perms

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return false, path, fmt.Errorf("encode %s: %w", path, err)
	}
	if err := writeFileAtomic(path, buf.Bytes(), mode); err != nil {
		return false, path, err
	}
	return true, path, nil
}

// writeFileAtomic writes data to path via a temp file and rename, creating the
// parent directory (0700: it holds credentials).
func writeFileAtomic(path string, data []byte, mode fs.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// ProfilePermissions is one profile whose permissions `up` syncs.
type ProfilePermissions struct {
	Profile     string
	ConfigDir   string
	Permissions Permissions
}

// PermissionTargets returns, in first-use order, each distinct profile among
// repos that has a permissions table.
func PermissionTargets(repos []Resolved) []ProfilePermissions {
	var out []ProfilePermissions
	for _, r := range repos {
		if r.Permissions == nil || r.ConfigDir == "" {
			continue
		}
		if slices.ContainsFunc(out, func(t ProfilePermissions) bool { return t.Profile == r.Profile }) {
			continue
		}
		out = append(out, ProfilePermissions{Profile: r.Profile, ConfigDir: r.ConfigDir, Permissions: *r.Permissions})
	}
	return out
}
