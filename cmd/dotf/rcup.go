package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/yandy-r/dotfiles/internal/rc"
)

// This file holds `dotf rc up` and `dotf rc status`: selecting repos, finding
// servers already running in them, the restart confirmation, and the Herdr
// start sequence.

// rcHerdrTimeout bounds the wait for a freshly started Herdr server.
var rcHerdrTimeout = 30 * time.Second

// rcInteractive reports whether a restart confirmation can be asked: stdin and
// the prompt writer must both be terminals. A package var so tests can drive
// the prompt.
var rcInteractive = func(stdin io.Reader, promptOut io.Writer) bool {
	f, ok := stdin.(*os.File)
	return ok && isTTY(f) && stdoutIsTTY(promptOut)
}

// rcSelect resolves the config and applies --only.
func rcSelect(env rcEnv) ([]rc.Resolved, int) {
	cfg, _, code := rcLoad(env)
	if code != 0 {
		return nil, code
	}
	repos, err := cfg.Resolve(env.home)
	if err != nil {
		return nil, reportErr(env.stderr, err)
	}
	if env.o.only == "" {
		return repos, 0
	}
	var out []rc.Resolved
	for _, want := range strings.Split(env.o.only, ",") {
		want = strings.TrimSpace(want)
		i := slices.IndexFunc(repos, func(r rc.Resolved) bool { return r.Name == want })
		if i < 0 {
			_, _ = fmt.Fprintf(env.stderr, "%s: --only: no configured repo named %q\n", progName, want)
			return nil, 2
		}
		out = append(out, repos[i])
	}
	return out, 0
}

// rcServers scans for running servers, returning nil (not an error) on hosts
// without procfs.
func rcServers() (map[string][]rc.Server, bool, error) {
	if !rc.ProcSupported() {
		return nil, false, nil
	}
	s, err := rc.FindServers()
	return s, true, err
}

func pidList(ss []rc.Server) string {
	ps := make([]string, 0, len(ss))
	for _, s := range ss {
		ps = append(ps, strconv.Itoa(s.PID))
	}
	return strings.Join(ps, ",")
}

// rcUpJSON is one repo's outcome in `dotf rc up --json`.
type rcUpJSON struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Action    string `json:"action"`
	Running   []int  `json:"running,omitempty"`
	Stopped   []int  `json:"stopped,omitempty"`
	Workspace string `json:"workspace,omitempty"`
	Tab       string `json:"tab,omitempty"`
	Verified  *bool  `json:"verified,omitempty"`
	Command   string `json:"command"`
	Error     string `json:"error,omitempty"`
}

// rcUp starts a Remote Control server for every selected repo.
func rcUp(env rcEnv) int {
	repos, code := rcSelect(env)
	if code != 0 {
		return code
	}
	if len(repos) == 0 {
		_, _ = fmt.Fprintf(env.stderr, "%s: no repos configured; add one with: dotf rc add <path>\n", progName)
		return 0
	}
	servers, procOK, err := rcServers()
	if err != nil {
		return reportErr(env.stderr, fmt.Errorf("scan running servers: %w", err))
	}
	plans := rc.PlanRepos(repos, servers)
	h := rc.Herdr{R: env.r}

	promptOut := env.stdout
	if env.o.jsonOut {
		promptOut = env.stderr
	}
	interactive := rcInteractive(env.stdin, promptOut)

	if env.o.dryRun {
		return rcUpDryRun(env, plans, h, interactive)
	}
	if !h.Available() {
		return reportErr(env.stderr, fmt.Errorf("herdr is not installed (dotf install --intent dev provides it)"))
	}
	started, err := h.EnsureServer(rcHerdrTimeout)
	if err != nil {
		return reportErr(env.stderr, err)
	}
	if started {
		_, _ = fmt.Fprintf(env.stderr, "%s: started the herdr server\n", progName)
	}

	starter := rc.Starter{H: h}
	var results []rcUpJSON
	failed := 0
	for i := range plans {
		p := &plans[i]
		res := rcUpJSON{Name: p.Repo.Name, Path: p.Repo.Path, Command: p.Repo.ShellLine()}
		for _, s := range p.Running {
			res.Running = append(res.Running, s.PID)
		}
		if p.Reason != "" {
			res.Action, res.Error = string(rc.ActionSkip), p.Reason
			failed++
			results = append(results, res)
			continue
		}
		ws, _, err := starter.Prepare(p)
		if err != nil {
			res.Action, res.Error = string(rc.ActionSkip), err.Error()
			failed++
			results = append(results, res)
			continue
		}
		res.Workspace = ws.ID
		action := rc.ActionStart
		if p.Busy(procOK) {
			switch decision := rcDecideRestart(env, *p, procOK, interactive, promptOut); decision {
			case confirmAborted:
				return reportAbort(env.stderr)
			case confirmYes:
				action = rc.ActionRestart
			default:
				action = rc.ActionKeep
			}
		}
		res.Action = string(action)
		if action == rc.ActionKeep {
			results = append(results, res)
			continue
		}
		out, err := starter.Apply(*p, ws, action == rc.ActionRestart)
		res.Stopped, res.Tab = out.Stopped, out.Tab.ID
		if err != nil {
			res.Error = err.Error()
			failed++
		} else if procOK {
			v := out.Verified
			res.Verified = &v
			if !v {
				// claude exited at once (logged out, trust declined, bad flag)
				// or never started: that is a failed start.
				res.Error = fmt.Sprintf("no server process appeared within %s; check its rc tab", rc.VerifyTimeout)
				failed++
			}
		}
		results = append(results, res)
	}
	return rcPrintUp(env, results, failed, false)
}

// rcDecideRestart asks (or, with --yes / without a terminal, decides) whether
// to restart a repo whose server is already running. Ctrl+C is intercepted only
// while the prompt is open, so every other phase of up keeps default SIGINT
// behavior and stays interruptible.
func rcDecideRestart(env rcEnv, p rc.Plan, procOK, interactive bool, promptOut io.Writer) confirmDecision {
	if env.o.yes {
		return confirmYes
	}
	if !interactive {
		_, _ = fmt.Fprintf(env.stderr, "%s: %s: a Remote Control server is already running (pid %s); "+
			"keeping it (pass --yes to restart non-interactively)\n", progName, p.Repo.Name, pidList(p.Running))
		return confirmNo
	}
	what := "a Remote Control server is running in " + p.Repo.Path + " (pid " + pidList(p.Running) + ")"
	if !procOK {
		what = "an rc tab already exists for " + p.Repo.Name + " and may be running a server"
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return confirmPromptCtx(ctx, promptOut, what+"; stop it and start a new one? [y/N] ", env.stdin)
}

// rcUpDryRun prints what up would do without touching Herdr or any process.
func rcUpDryRun(env rcEnv, plans []rc.Plan, h rc.Herdr, interactive bool) int {
	var results []rcUpJSON
	for _, p := range plans {
		res := rcUpJSON{Name: p.Repo.Name, Path: p.Repo.Path, Command: p.Repo.ShellLine(), Action: string(rc.ActionStart)}
		for _, s := range p.Running {
			res.Running = append(res.Running, s.PID)
		}
		switch {
		case p.Reason != "":
			res.Action, res.Error = string(rc.ActionSkip), p.Reason
		case len(p.Running) > 0 && env.o.yes:
			res.Action = string(rc.ActionRestart)
		case len(p.Running) > 0 && interactive:
			res.Action = "confirm-restart"
		case len(p.Running) > 0:
			res.Action = string(rc.ActionKeep)
		}
		results = append(results, res)
	}
	if !env.o.jsonOut {
		_, _ = fmt.Fprintf(env.stdout, "dry-run=true herdr.available=%t herdr.running=%t\n", h.Available(), h.Available() && h.ServerRunning())
	}
	return rcPrintUp(env, results, 0, true)
}

// rcPrintUp renders up's results and maps failures to exit 1.
func rcPrintUp(env rcEnv, results []rcUpJSON, failed int, dryRun bool) int {
	if env.o.jsonOut {
		doc := struct {
			Command string     `json:"command"`
			Verb    string     `json:"verb"`
			DryRun  bool       `json:"dry_run"`
			Repos   []rcUpJSON `json:"repos"`
			Failed  int        `json:"failed"`
		}{"rc", "up", dryRun, results, failed}
		if doc.Repos == nil {
			doc.Repos = []rcUpJSON{}
		}
		if err := writeJSON(env.stdout, doc); err != nil {
			return reportErr(env.stderr, err)
		}
	} else {
		var b strings.Builder
		for _, r := range results {
			fmt.Fprintf(&b, "rc.up=%s action=%s path=%s", r.Name, r.Action, r.Path)
			if len(r.Running) > 0 {
				fmt.Fprintf(&b, " running=%s", joinInts(r.Running))
			}
			if len(r.Stopped) > 0 {
				fmt.Fprintf(&b, " stopped=%s", joinInts(r.Stopped))
			}
			if r.Tab != "" {
				fmt.Fprintf(&b, " tab=%s", r.Tab)
			}
			if r.Verified != nil {
				fmt.Fprintf(&b, " verified=%t", *r.Verified)
			}
			if r.Error != "" {
				fmt.Fprintf(&b, " error=%q", r.Error)
			}
			b.WriteByte('\n')
			if dryRun {
				fmt.Fprintf(&b, "rc.command=%s %s\n", r.Name, r.Command)
			}
		}
		fmt.Fprintf(&b, "rc.repos=%d rc.failed=%d\n", len(results), failed)
		if _, err := io.WriteString(env.stdout, b.String()); err != nil {
			return reportErr(env.stderr, err)
		}
	}
	if failed > 0 {
		_, _ = fmt.Fprintf(env.stderr, "%s: %d repo(s) failed\n", progName, failed)
		return 1
	}
	return 0
}

func joinInts(ns []int) string {
	s := make([]string, 0, len(ns))
	for _, n := range ns {
		s = append(s, strconv.Itoa(n))
	}
	return strings.Join(s, ",")
}

// rcStatusJSON is one repo in `dotf rc status --json`.
type rcStatusJSON struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Exists  bool   `json:"exists"`
	Running []int  `json:"running"`
}

// rcStatus reports, per repo, whether a server is running, plus the service.
func rcStatus(env rcEnv) int {
	repos, code := rcSelect(env)
	if code != 0 {
		return code
	}
	servers, procOK, err := rcServers()
	if err != nil {
		return reportErr(env.stderr, fmt.Errorf("scan running servers: %w", err))
	}
	enabled, active := "unsupported", "unsupported"
	if rcServiceSupported(env) {
		enabled, active = rc.UnitEnabled(env.r)
	}
	rows := make([]rcStatusJSON, 0, len(repos))
	for _, r := range repos {
		row := rcStatusJSON{Name: r.Name, Path: r.Path, Exists: dirExists(r.Path), Running: []int{}}
		for _, s := range servers[r.Path] {
			row.Running = append(row.Running, s.PID)
		}
		rows = append(rows, row)
	}
	if env.o.jsonOut {
		doc := struct {
			Command        string         `json:"command"`
			Verb           string         `json:"verb"`
			ProcSupported  bool           `json:"proc_supported"`
			ServiceEnabled string         `json:"service_enabled"`
			ServiceActive  string         `json:"service_active"`
			Repos          []rcStatusJSON `json:"repos"`
		}{"rc", "status", procOK, enabled, active, rows}
		if err := writeJSON(env.stdout, doc); err != nil {
			return reportErr(env.stderr, err)
		}
		return 0
	}
	var b strings.Builder
	fmt.Fprintf(&b, "rc.service=%s enabled=%s active=%s\n", rc.UnitName, enabled, active)
	for _, row := range rows {
		running := "unknown"
		if procOK {
			running = strconv.FormatBool(len(row.Running) > 0)
		}
		fmt.Fprintf(&b, "rc.repo=%s path=%s exists=%t running=%s", row.Name, row.Path, row.Exists, running)
		if len(row.Running) > 0 {
			fmt.Fprintf(&b, " pids=%s", joinInts(row.Running))
		}
		b.WriteByte('\n')
	}
	if _, err := io.WriteString(env.stdout, b.String()); err != nil {
		return reportErr(env.stderr, err)
	}
	return 0
}
