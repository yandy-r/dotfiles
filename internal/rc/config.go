// Package rc manages per-repo Claude Code Remote Control servers: a user-local
// list of repo paths (rc.toml), the shell command that starts one server, the
// discovery of servers already running in a directory, the Herdr workspace/tab
// plumbing that hosts them, and the systemd user unit that starts them at
// login.
//
// The package is a library: it never prompts, never prints, and reaches the
// outside world only through an injected exec.Runner (Herdr, systemctl) or an
// injectable /proc root, so cmd/dotf owns every interactive decision and every
// test stays hermetic.
package rc

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

// Default values applied when rc.toml leaves a setting unset.
const (
	DefaultPermissionMode = "auto"
	DefaultSpawn          = "worktree"
	DefaultClaude         = "claude"
	DefaultTabLabel       = "rc"
	// DefaultProfile is the implicit profile: the top-level keys, run with
	// Claude's own default config dir. It cannot be configured.
	DefaultProfile = "claude"
	// ConfigDirEnv is the variable a profile's config_dir is exported as.
	ConfigDirEnv = "CLAUDE_CONFIG_DIR"
)

// PermissionModes is the closed set `claude remote-control --permission-mode`
// accepts.
var PermissionModes = []string{"acceptEdits", "auto", "bypassPermissions", "default", "dontAsk", "plan"}

// SpawnModes is the closed set `claude remote-control --spawn` accepts.
var SpawnModes = []string{"same-dir", "worktree", "session"}

// defaultEnv is written into a freshly created rc.toml. The dotfiles export
// DO_NOT_TRACK=1 globally; Remote Control needs it off for the server process.
var defaultEnv = map[string]string{"DO_NOT_TRACK": "0"}

var (
	envKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	nameRe   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

// Config is the rc.toml document.
type Config struct {
	// PermissionMode is the default --permission-mode for spawned sessions.
	PermissionMode string `toml:"permission_mode,omitempty"`
	// AllowBypass adds --allow-dangerously-skip-permissions so bypass mode stays
	// reachable from a session without being the starting mode. Nil means true.
	AllowBypass *bool `toml:"allow_bypass,omitempty"`
	// Spawn is the default --spawn mode.
	Spawn string `toml:"spawn,omitempty"`
	// Claude is the claude executable to run (default "claude").
	Claude string `toml:"claude,omitempty"`
	// Env is prepended to the command as KEY=VALUE assignments.
	Env map[string]string `toml:"env,omitempty"`
	// Profiles are the named Claude subscriptions, keyed by name. The default
	// profile (DefaultProfile) is implicit and absent from this map.
	Profiles map[string]Profile `toml:"profile,omitempty"`
	// Repos is the ordered list of repos to host a server for.
	Repos []Repo `toml:"repo"`
}

// Profile is one [profile.<name>] table: a Claude subscription identified by
// its config dir, with its own defaults. Only ConfigDir is required.
type Profile struct {
	// ConfigDir is exported as CLAUDE_CONFIG_DIR (absolute or ~/...).
	ConfigDir      string            `toml:"config_dir"`
	PermissionMode string            `toml:"permission_mode,omitempty"`
	AllowBypass    *bool             `toml:"allow_bypass,omitempty"`
	Spawn          string            `toml:"spawn,omitempty"`
	Env            map[string]string `toml:"env,omitempty"`
	// Permissions, when set, are synced into <ConfigDir>/settings.json by up.
	Permissions *Permissions `toml:"permissions,omitempty"`
}

// Permissions are Claude Code permission rules. A nil list is left alone in
// settings.json; a non-nil (even empty) list replaces it.
type Permissions struct {
	Allow *[]string `toml:"allow,omitempty"`
	Deny  *[]string `toml:"deny,omitempty"`
	Ask   *[]string `toml:"ask,omitempty"`
}

// lists returns the rule lists keyed by their settings.json name.
func (p Permissions) lists() map[string]*[]string {
	return map[string]*[]string{"allow": p.Allow, "deny": p.Deny, "ask": p.Ask}
}

// Repo is one [[repo]] entry. Only Path is required.
type Repo struct {
	Path           string `toml:"path"`
	Name           string `toml:"name,omitempty"`
	Profile        string `toml:"profile,omitempty"`
	PermissionMode string `toml:"permission_mode,omitempty"`
	Spawn          string `toml:"spawn,omitempty"`
}

// Resolved is a Repo with every default applied and its path made absolute
// and symlink-free.
type Resolved struct {
	Name           string
	Path           string
	Profile        string // DefaultProfile when the repo names none
	ConfigDir      string // expanded config_dir; empty for the default profile
	Permissions    *Permissions
	PermissionMode string
	Spawn          string
	AllowBypass    bool
	Claude         string
	Env            map[string]string
}

// DefaultPath is ${XDG_CONFIG_HOME:-<home>/.config}/dotf/rc.toml. A
// non-absolute XDG_CONFIG_HOME is ignored per the XDG spec.
func DefaultPath(home string) string {
	base := filepath.Join(home, ".config")
	if v := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(v) {
		base = v
	}
	return filepath.Join(base, "dotf", "rc.toml")
}

// NewConfig returns the document a first `dotf rc add` writes: explicit
// defaults so the user can see and edit them.
func NewConfig() *Config {
	allow := true
	env := make(map[string]string, len(defaultEnv))
	for k, v := range defaultEnv {
		env[k] = v
	}
	return &Config{PermissionMode: DefaultPermissionMode, AllowBypass: &allow, Spawn: DefaultSpawn, Env: env}
}

// Load reads path. A missing file is not an error: it returns an empty Config
// and exists=false so read-only verbs work before the first add.
func Load(path string) (cfg *Config, exists bool, err error) {
	data, err := os.ReadFile(path) //nolint:gosec // user-chosen config path
	if errors.Is(err, fs.ErrNotExist) {
		return &Config{}, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read %s: %w", path, err)
	}
	cfg = &Config{}
	dec := toml.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(cfg); err != nil {
		return nil, true, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, true, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, true, nil
}

const fileHeader = "# dotf rc: Claude Code Remote Control servers, one per repo.\n" +
	"# Managed by `dotf rc add/remove`; hand edits are kept but comments are not.\n" +
	"# Keys: permission_mode, allow_bypass, spawn, claude, [env], [profile.<name>]\n" +
	"# tables (config_dir required; permission_mode, allow_bypass, spawn, env,\n" +
	"# [profile.<name>.permissions] allow/deny/ask), and [[repo]] entries with\n" +
	"# path (required), name, profile, permission_mode, spawn.\n\n"

// Save writes cfg to path atomically (temp file + rename), creating the parent
// directory.
func Save(path string, cfg *Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	body, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode rc config: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".rc.toml.*")
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.WriteString(fileHeader + string(body)); err != nil {
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

// Validate checks enumerated values, env keys, and that repo names (explicit
// or derived from the path) are well-formed and unique.
func (c *Config) Validate() error {
	if err := checkEnum("permission_mode", c.PermissionMode, PermissionModes); err != nil {
		return err
	}
	if err := checkEnum("spawn", c.Spawn, SpawnModes); err != nil {
		return err
	}
	if err := checkEnv("env", c.Env); err != nil {
		return err
	}
	if err := c.validateProfiles(); err != nil {
		return err
	}
	seen := map[string]string{}
	for i, r := range c.Repos {
		if strings.TrimSpace(r.Path) == "" {
			return fmt.Errorf("repo[%d]: path is required", i)
		}
		if err := checkEnum(fmt.Sprintf("repo[%d].permission_mode", i), r.PermissionMode, PermissionModes); err != nil {
			return err
		}
		if err := checkEnum(fmt.Sprintf("repo[%d].spawn", i), r.Spawn, SpawnModes); err != nil {
			return err
		}
		if _, ok := c.Profiles[r.Profile]; r.Profile != "" && r.Profile != DefaultProfile && !ok {
			return fmt.Errorf("repo[%d]: profile %q is not defined (add a [profile.%s] table)", i, r.Profile, r.Profile)
		}
		name := r.EffectiveName()
		if !nameRe.MatchString(name) {
			return fmt.Errorf("repo[%d]: name %q must match %s (set name explicitly)", i, name, nameRe)
		}
		if prev, dup := seen[name]; dup {
			return fmt.Errorf("repo name %q is used by both %s and %s; set a distinct name", name, prev, r.Path)
		}
		seen[name] = r.Path
	}
	return nil
}

// validateProfiles checks every [profile.<name>] table.
func (c *Config) validateProfiles() error {
	names := make([]string, 0, len(c.Profiles))
	for n := range c.Profiles {
		names = append(names, n)
	}
	slices.Sort(names) // deterministic first error
	dirs := map[string]string{}
	for _, n := range names {
		p, field := c.Profiles[n], "profile."+n
		switch {
		case n == DefaultProfile:
			return fmt.Errorf("%s: %q is the reserved default profile; configure it with the top-level keys", field, n)
		case !nameRe.MatchString(n):
			return fmt.Errorf("%s: name must match %s", field, nameRe)
		case strings.TrimSpace(p.ConfigDir) == "":
			return fmt.Errorf("%s: config_dir is required", field)
		case !filepath.IsAbs(p.ConfigDir) && p.ConfigDir != "~" && !strings.HasPrefix(p.ConfigDir, "~/"):
			return fmt.Errorf("%s: config_dir %q must be absolute or start with ~/", field, p.ConfigDir)
		}
		key := filepath.Clean(p.ConfigDir)
		if prev, dup := dirs[key]; dup {
			return fmt.Errorf("profiles %q and %q share config_dir %s", prev, n, p.ConfigDir)
		}
		dirs[key] = n
		if err := checkEnum(field+".permission_mode", p.PermissionMode, PermissionModes); err != nil {
			return err
		}
		if err := checkEnum(field+".spawn", p.Spawn, SpawnModes); err != nil {
			return err
		}
		if err := checkEnv(field+".env", p.Env); err != nil {
			return err
		}
		if p.Permissions != nil {
			for k, list := range p.Permissions.lists() {
				if list != nil && slices.ContainsFunc(*list, func(s string) bool { return strings.TrimSpace(s) == "" }) {
					return fmt.Errorf("%s.permissions.%s: empty rule", field, k)
				}
			}
		}
	}
	return nil
}

// checkEnv validates variable names and keeps CLAUDE_CONFIG_DIR owned by
// profiles.
func checkEnv(field string, env map[string]string) error {
	for k := range env {
		if !envKeyRe.MatchString(k) {
			return fmt.Errorf("%s: invalid variable name %q", field, k)
		}
		if k == ConfigDirEnv {
			return fmt.Errorf("%s: %s is set by a profile's config_dir, not env", field, ConfigDirEnv)
		}
	}
	return nil
}

func checkEnum(field, v string, allowed []string) error {
	if v == "" || slices.Contains(allowed, v) {
		return nil
	}
	return fmt.Errorf("%s: %q is not one of %s", field, v, strings.Join(allowed, ", "))
}

// EffectiveName is Name, or the path's base name.
func (r Repo) EffectiveName() string {
	if r.Name != "" {
		return r.Name
	}
	return filepath.Base(filepath.Clean(r.Path))
}

// ExpandPath expands a leading ~ against home and makes p absolute and clean.
func ExpandPath(p, home string) (string, error) {
	if p == "~" || strings.HasPrefix(p, "~/") {
		p = filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", p, err)
	}
	return abs, nil
}

// Resolve applies defaults to every repo, in config order.
func (c *Config) Resolve(home string) ([]Resolved, error) {
	allow := c.AllowBypass == nil || *c.AllowBypass
	claude := c.Claude
	switch {
	case claude == "":
		claude = DefaultClaude
	case strings.HasPrefix(claude, "~/"):
		claude = filepath.Join(home, claude[2:])
	}
	out := make([]Resolved, 0, len(c.Repos))
	for _, r := range c.Repos {
		p, err := ExpandPath(r.Path, home)
		if err != nil {
			return nil, err
		}
		profName := firstNonEmpty(r.Profile, DefaultProfile)
		prof := c.Profiles[profName] // zero Profile for the default
		repoAllow, env, configDir := allow, c.Env, ""
		if profName != DefaultProfile {
			if configDir, err = ExpandPath(prof.ConfigDir, home); err != nil {
				return nil, err
			}
			if prof.AllowBypass != nil {
				repoAllow = *prof.AllowBypass
			}
			env = make(map[string]string, len(c.Env)+len(prof.Env)+1)
			maps.Copy(env, c.Env)
			maps.Copy(env, prof.Env)
			env[ConfigDirEnv] = configDir
		}
		// Resolve symlinks: /proc reports a server's cwd fully resolved, so an
		// unresolved path (~/code -> /data/code) would never match it. A path
		// that does not exist yet stays as written.
		if resolved, err := filepath.EvalSymlinks(p); err == nil {
			p = resolved
		}
		out = append(out, Resolved{
			Name:           r.EffectiveName(),
			Path:           p,
			Profile:        profName,
			ConfigDir:      configDir,
			Permissions:    prof.Permissions,
			PermissionMode: firstNonEmpty(r.PermissionMode, prof.PermissionMode, c.PermissionMode, DefaultPermissionMode),
			Spawn:          firstNonEmpty(r.Spawn, prof.Spawn, c.Spawn, DefaultSpawn),
			AllowBypass:    repoAllow,
			Claude:         claude,
			Env:            env,
		})
	}
	return out, nil
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

// Add appends a repo for path. home is used to compare entries by their
// expanded path, so "~/x" and "/home/u/x" are the same repo. It returns
// added=false when the path is already listed.
func (c *Config) Add(r Repo, home string) (added bool, err error) {
	want, err := ExpandPath(r.Path, home)
	if err != nil {
		return false, err
	}
	for _, have := range c.Repos {
		hp, err := ExpandPath(have.Path, home)
		if err != nil {
			return false, err
		}
		if hp == want {
			return false, nil
		}
	}
	c.Repos = append(c.Repos, r)
	if err := c.Validate(); err != nil {
		c.Repos = c.Repos[:len(c.Repos)-1]
		return false, err
	}
	return true, nil
}

// Remove drops the repo matching key, which is a repo name or a path (compared
// after expansion). It returns the removed entry and whether one matched.
func (c *Config) Remove(key, home string) (Repo, bool, error) {
	keyPath, err := ExpandPath(key, home)
	if err != nil {
		return Repo{}, false, err
	}
	for i, r := range c.Repos {
		rp, err := ExpandPath(r.Path, home)
		if err != nil {
			return Repo{}, false, err
		}
		if r.EffectiveName() == key || rp == keyPath {
			c.Repos = slices.Delete(c.Repos, i, i+1)
			return r, true, nil
		}
	}
	return Repo{}, false, nil
}
