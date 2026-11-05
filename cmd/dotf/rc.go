package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/rc"
)

// This file is the COMMAND half of `dotf rc`: the flag surface, the verb
// parse, and the config verbs (list/add/remove). The up/status flow lives in
// rcup.go and the systemd unit verbs in rcservice.go; the logic they drive is
// internal/rc.

// rcVerbs is the ordered, closed set of `dotf rc` verbs: the single source for
// dispatch, usage errors, and (via completionRcVerbs) shell completions.
var rcVerbs = []string{"list", "add", "remove", "up", "status", "enable", "disable"}

// rcOpts holds the parsed flag values for `dotf rc`.
type rcOpts struct {
	config         string
	name           string
	permissionMode string
	spawn          string
	only           string
	yes            bool
	dryRun         bool
	now            bool
	force          bool
	jsonOut        bool
}

// setRcFlags registers rc's flags on fs and returns the opts pointer they
// parse into. Single source of truth for the flag surface, shared by runRc,
// the help renderer, and the completion generator. --config's usage says
// "path" so the completion generator offers file completion for it.
func setRcFlags(fs *flag.FlagSet) *rcOpts {
	o := &rcOpts{}
	fs.StringVar(&o.config, "config", "", "path to the rc config file (default: ${XDG_CONFIG_HOME:-~/.config}/dotf/rc.toml)")
	fs.StringVar(&o.name, "name", "", "session/workspace name for the added repo (add; default: directory name)")
	fs.StringVar(&o.permissionMode, "permission-mode", "",
		"per-repo permission mode override: "+strings.Join(rc.PermissionModes, ", ")+" (add)")
	fs.StringVar(&o.spawn, "spawn", "", "per-repo spawn mode override: "+strings.Join(rc.SpawnModes, ", ")+" (add)")
	fs.StringVar(&o.only, "only", "", "comma-separated repo names to act on (up, status; default: all)")
	fs.BoolVar(&o.yes, "yes", false, "restart already-running servers without a confirmation prompt (up)")
	fs.BoolVar(&o.dryRun, "dry-run", false, "print the plan without executing")
	fs.BoolVar(&o.now, "now", false, "also start (enable) or stop (disable) the service immediately")
	fs.BoolVar(&o.force, "force", false, "overwrite or remove a dotf-rc.service unit dotf did not write (enable, disable)")
	fs.BoolVar(&o.jsonOut, "json", false, jsonFlagUsage)
	return o
}

// rcFlagVerbs maps each verb-specific flag to the verbs that accept it;
// --config and --dry-run apply everywhere and are absent here.
var rcFlagVerbs = map[string][]string{
	"name":            {"add"},
	"permission-mode": {"add"},
	"spawn":           {"add"},
	"only":            {"up", "status"},
	"yes":             {"up"},
	"now":             {"enable", "disable"},
	"force":           {"enable", "disable"},
	"json":            {"list", "up", "status"},
}

// rcEnv bundles what every verb needs.
type rcEnv struct {
	stdout, stderr io.Writer
	stdin          io.Reader
	r              exec.Runner
	home           string
	cfgPath        string
	o              *rcOpts
}

// runRc manages per-repo Claude Code Remote Control servers hosted in Herdr.
//
// Exit codes: 0 success, 1 runtime failure or refusal, 2 usage, 130 on a
// Ctrl+C at a restart confirmation.
func runRc(args []string, stdout, stderr io.Writer, stdin io.Reader, r exec.Runner) int {
	fs := flag.NewFlagSet("rc", flag.ContinueOnError)
	fs.SetOutput(stderr)
	o := setRcFlags(fs)

	var positionals []string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			return 2
		}
		if fs.NArg() == 0 {
			break
		}
		positionals = append(positionals, fs.Arg(0))
		rest = fs.Args()[1:]
	}
	if len(positionals) == 0 {
		_, _ = fmt.Fprintf(stderr, "%s: rc requires a verb; available: %s\n", progName, strings.Join(rcVerbs, ", "))
		return 2
	}
	verb, operands := positionals[0], positionals[1:]
	if !slices.Contains(rcVerbs, verb) {
		_, _ = fmt.Fprintf(stderr, "%s: unknown verb %q; available: %s\n", progName, verb, strings.Join(rcVerbs, ", "))
		return 2
	}
	if !rcCheckFlags(fs, verb, stderr) {
		return 2
	}
	if takesOperands := verb == "add" || verb == "remove"; takesOperands != (len(operands) > 0) {
		if takesOperands {
			_, _ = fmt.Fprintf(stderr, "%s: rc %s requires at least one argument\n", progName, verb)
		} else {
			_, _ = fmt.Fprintf(stderr, "%s: rc %s takes no arguments, got %d\n", progName, verb, len(operands))
		}
		return 2
	}
	if verb == "add" && len(operands) > 1 && o.name != "" {
		_, _ = fmt.Fprintf(stderr, "%s: --name needs exactly one path\n", progName)
		return 2
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return reportErr(stderr, fmt.Errorf("resolve home dir: %w", err))
	}
	env := rcEnv{stdout: stdout, stderr: stderr, stdin: stdin, r: r, home: home, cfgPath: o.config, o: o}
	if env.cfgPath == "" {
		env.cfgPath = rc.DefaultPath(home)
	} else if env.cfgPath, err = rc.ExpandPath(env.cfgPath, home); err != nil {
		return reportErr(stderr, err)
	}

	switch verb {
	case "list":
		return rcList(env)
	case "add":
		return rcAdd(env, operands)
	case "remove":
		return rcRemove(env, operands)
	case "up":
		return rcUp(env)
	case "status":
		return rcStatus(env)
	case "enable":
		return rcEnable(env)
	default:
		return rcDisable(env)
	}
}

// rcCheckFlags rejects a verb-specific flag explicitly set on a verb it does
// not apply to.
func rcCheckFlags(fs *flag.FlagSet, verb string, stderr io.Writer) bool {
	ok := true
	fs.Visit(func(f *flag.Flag) {
		verbs, scoped := rcFlagVerbs[f.Name]
		if ok && scoped && !slices.Contains(verbs, verb) {
			_, _ = fmt.Fprintf(stderr, "%s: --%s does not apply to \"dotf rc %s\"\n", progName, f.Name, verb)
			ok = false
		}
	})
	return ok
}

// rcLoad reads the config, mapping errors to exit 1.
func rcLoad(env rcEnv) (*rc.Config, bool, int) {
	cfg, exists, err := rc.Load(env.cfgPath)
	if err != nil {
		return nil, false, reportErr(env.stderr, err)
	}
	return cfg, exists, 0
}

// rcRepoJSON is one repo in `dotf rc list --json`.
type rcRepoJSON struct {
	Name           string `json:"name"`
	Path           string `json:"path"`
	Exists         bool   `json:"exists"`
	PermissionMode string `json:"permission_mode"`
	Spawn          string `json:"spawn"`
	AllowBypass    bool   `json:"allow_bypass"`
	Command        string `json:"command"`
}

// rcList prints the configured repos with their effective settings.
func rcList(env rcEnv) int {
	cfg, exists, code := rcLoad(env)
	if code != 0 {
		return code
	}
	repos, err := cfg.Resolve(env.home)
	if err != nil {
		return reportErr(env.stderr, err)
	}
	if env.o.jsonOut {
		doc := struct {
			Command      string       `json:"command"`
			Verb         string       `json:"verb"`
			Config       string       `json:"config"`
			ConfigExists bool         `json:"config_exists"`
			Repos        []rcRepoJSON `json:"repos"`
		}{"rc", "list", env.cfgPath, exists, []rcRepoJSON{}}
		for _, r := range repos {
			doc.Repos = append(doc.Repos, rcRepoJSON{
				Name: r.Name, Path: r.Path, Exists: dirExists(r.Path), PermissionMode: r.PermissionMode,
				Spawn: r.Spawn, AllowBypass: r.AllowBypass, Command: r.ShellLine(),
			})
		}
		if err := writeJSON(env.stdout, doc); err != nil {
			return reportErr(env.stderr, err)
		}
		return 0
	}
	var b strings.Builder
	fmt.Fprintf(&b, "rc.config=%s exists=%t\n", env.cfgPath, exists)
	for _, r := range repos {
		fmt.Fprintf(&b, "rc.repo=%s path=%s exists=%t permission_mode=%s spawn=%s allow_bypass=%t\n",
			r.Name, r.Path, dirExists(r.Path), r.PermissionMode, r.Spawn, r.AllowBypass)
	}
	fmt.Fprintf(&b, "rc.repos=%d\n", len(repos))
	if _, err := io.WriteString(env.stdout, b.String()); err != nil {
		return reportErr(env.stderr, err)
	}
	if len(repos) == 0 {
		_, _ = fmt.Fprintf(env.stderr, "%s: no repos configured; add one with: dotf rc add <path>\n", progName)
	}
	return 0
}

// rcAdd appends each path to the config, creating it with explicit defaults
// on first use. Paths are stored as given (~ kept) but compared expanded.
func rcAdd(env rcEnv, paths []string) int {
	cfg, exists, code := rcLoad(env)
	if code != 0 {
		return code
	}
	if !exists {
		cfg = rc.NewConfig()
	}
	// Lines are buffered and printed only once the config is saved, so a later
	// failing path never leaves stdout claiming adds that were not persisted.
	var lines strings.Builder
	added := 0
	for _, p := range paths {
		abs, err := rc.ExpandPath(p, env.home)
		if err != nil {
			return reportErr(env.stderr, err)
		}
		if !dirExists(abs) {
			return reportErr(env.stderr, fmt.Errorf("%s is not a directory", abs))
		}
		repo := rc.Repo{Path: rcStoredPath(abs, env.home), Name: env.o.name,
			PermissionMode: env.o.permissionMode, Spawn: env.o.spawn}
		ok, err := cfg.Add(repo, env.home)
		if err != nil {
			return reportErr(env.stderr, err)
		}
		if !ok {
			fmt.Fprintf(&lines, "rc.add=%s status=already-listed\n", repo.EffectiveName())
			continue
		}
		added++
		fmt.Fprintf(&lines, "rc.add=%s path=%s\n", repo.EffectiveName(), abs)
	}
	return rcSave(env, cfg, added, "added", lines.String())
}

// rcRemove drops each name-or-path from the config.
func rcRemove(env rcEnv, keys []string) int {
	cfg, exists, code := rcLoad(env)
	if code != 0 {
		return code
	}
	if !exists {
		return reportErr(env.stderr, fmt.Errorf("no rc config at %s", env.cfgPath))
	}
	var lines strings.Builder
	removed := 0
	for _, k := range keys {
		repo, ok, err := cfg.Remove(k, env.home)
		if err != nil {
			return reportErr(env.stderr, err)
		}
		if !ok {
			return reportErr(env.stderr, fmt.Errorf("no configured repo matches %q (see dotf rc list)", k))
		}
		removed++
		fmt.Fprintf(&lines, "rc.remove=%s path=%s\n", repo.EffectiveName(), repo.Path)
	}
	return rcSave(env, cfg, removed, "removed", lines.String())
}

// rcSave writes the config unless nothing changed or this is a dry run, then
// prints the per-repo lines.
func rcSave(env rcEnv, cfg *rc.Config, changed int, what, lines string) int {
	if env.o.dryRun {
		_, _ = fmt.Fprintf(env.stdout, "dry-run=true\n%s", lines)
		_, _ = fmt.Fprintf(env.stderr, "%s: dry run; %d repo(s) would be %s in %s\n", progName, changed, what, env.cfgPath)
		return 0
	}
	if changed > 0 {
		if err := rc.Save(env.cfgPath, cfg); err != nil {
			return reportErr(env.stderr, err)
		}
	}
	_, _ = io.WriteString(env.stdout, lines)
	if changed == 0 {
		return 0
	}
	_, _ = fmt.Fprintf(env.stderr, "%s: %d repo(s) %s in %s\n", progName, changed, what, env.cfgPath)
	return 0
}

// rcStoredPath stores paths under home as ~/... so the config stays portable
// between machines with different usernames.
func rcStoredPath(abs, home string) string {
	if rel, ok := strings.CutPrefix(abs, home+string(os.PathSeparator)); ok {
		return "~/" + rel
	}
	return abs
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
