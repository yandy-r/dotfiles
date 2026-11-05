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
	// Repos is the ordered list of repos to host a server for.
	Repos []Repo `toml:"repo"`
}

// Repo is one [[repo]] entry. Only Path is required.
type Repo struct {
	Path           string `toml:"path"`
	Name           string `toml:"name,omitempty"`
	PermissionMode string `toml:"permission_mode,omitempty"`
	Spawn          string `toml:"spawn,omitempty"`
}

// Resolved is a Repo with every default applied and its path made absolute
// and symlink-free.
type Resolved struct {
	Name           string
	Path           string
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
	"# Keys: permission_mode, allow_bypass, spawn, claude, [env], and [[repo]]\n" +
	"# entries with path (required), name, permission_mode, spawn.\n\n"

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
	for k := range c.Env {
		if !envKeyRe.MatchString(k) {
			return fmt.Errorf("env: invalid variable name %q", k)
		}
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
		// Resolve symlinks: /proc reports a server's cwd fully resolved, so an
		// unresolved path (~/code -> /data/code) would never match it. A path
		// that does not exist yet stays as written.
		if resolved, err := filepath.EvalSymlinks(p); err == nil {
			p = resolved
		}
		out = append(out, Resolved{
			Name:           r.EffectiveName(),
			Path:           p,
			PermissionMode: firstNonEmpty(r.PermissionMode, c.PermissionMode, DefaultPermissionMode),
			Spawn:          firstNonEmpty(r.Spawn, c.Spawn, DefaultSpawn),
			AllowBypass:    allow,
			Claude:         claude,
			Env:            c.Env,
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
