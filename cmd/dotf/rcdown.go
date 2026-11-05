package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/yandy-r/dotfiles/internal/rc"
)

// This file holds `dotf rc down`: stopping the Remote Control servers of the
// configured repos and closing their rc tabs.

// rcDownJSON is one repo's outcome in `dotf rc down --json`.
type rcDownJSON struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Action  string `json:"action"` // stop, none, or (dry run) would-stop
	Running []int  `json:"running,omitempty"`
	Stopped []int  `json:"stopped,omitempty"`
	Tab     string `json:"tab,omitempty"`
	Error   string `json:"error,omitempty"`
}

// rcDownTarget is a repo with something to bring down.
type rcDownTarget struct {
	plan   rc.Plan
	hasTab bool
}

// rcDown stops every server running in a configured repo (or --only) and
// closes the repo's rc tab. It never starts Herdr and never touches a
// directory that is not a configured repo. Exit codes: 0 done (or nothing to
// do), 1 a repo failed or a refusal (declined, non-interactive without
// --yes), 2 usage, 130 Ctrl+C at the confirmation.
func rcDown(env rcEnv) int {
	repos, code := rcSelect(env)
	if code != 0 {
		return code
	}
	servers, _, err := rcServers()
	if err != nil {
		return reportErr(env.stderr, fmt.Errorf("scan running servers: %w", err))
	}
	h := rc.Herdr{R: env.r}
	herdrOK := h.Available() && h.ServerRunning()

	var results []rcDownJSON
	var targets []rcDownTarget
	for _, p := range rc.PlanRepos(repos, servers) {
		res := rcDownJSON{Name: p.Repo.Name, Path: p.Repo.Path, Action: "none"}
		for _, s := range p.Running {
			res.Running = append(res.Running, s.PID)
		}
		hasTab := false
		if herdrOK {
			if ws, ok, err := h.FindWorkspace(p.Repo.Name, p.Repo.Path); err != nil {
				res.Error = err.Error()
			} else if ok {
				if tab, found, err := h.RepoTab(ws.ID, p.Repo.Path); err != nil {
					res.Error = err.Error()
				} else if found {
					hasTab, res.Tab = true, tab.ID
				}
			}
		}
		if len(p.Running) > 0 || hasTab {
			res.Action = "stop"
			targets = append(targets, rcDownTarget{plan: p, hasTab: hasTab})
		}
		results = append(results, res)
	}

	if env.o.dryRun {
		for i := range results {
			if results[i].Action == "stop" {
				results[i].Action = "would-stop"
			}
		}
		return rcPrintDown(env, results, true)
	}
	if len(targets) == 0 {
		_, _ = fmt.Fprintf(env.stderr, "%s: no Remote Control servers or rc tabs to bring down\n", progName)
		return rcPrintDown(env, results, false)
	}
	if code, ok := rcConfirmDown(env, targets); !ok {
		return code
	}

	for i := range results {
		if results[i].Action != "stop" {
			continue
		}
		t := rcFindTarget(targets, results[i].Name)
		out, err := rc.Down(t.plan, h, herdrOK && t.hasTab)
		results[i].Stopped, results[i].Tab = out.Stopped, out.Tab
		if err != nil {
			results[i].Error = err.Error()
		}
	}
	return rcPrintDown(env, results, false)
}

func rcFindTarget(ts []rcDownTarget, name string) rcDownTarget {
	for _, t := range ts {
		if t.plan.Repo.Name == name {
			return t
		}
	}
	return rcDownTarget{}
}

// rcConfirmDown asks once for the whole run. --yes skips it; without a
// terminal it refuses rather than stopping servers unattended. Ctrl+C is
// intercepted only while the prompt is open.
func rcConfirmDown(env rcEnv, targets []rcDownTarget) (int, bool) {
	if env.o.yes {
		return 0, true
	}
	promptOut := env.stdout
	if env.o.jsonOut {
		promptOut = env.stderr
	}
	if !rcInteractive(env.stdin, promptOut) {
		_, _ = fmt.Fprintf(env.stderr, "%s: refusing to stop Remote Control servers without --yes (non-interactive)\n", progName)
		return 1, false
	}
	for _, t := range targets {
		what := "rc tab"
		if len(t.plan.Running) > 0 {
			what = "server pid " + pidList(t.plan.Running)
			if t.hasTab {
				what += " + rc tab"
			}
		}
		_, _ = fmt.Fprintf(promptOut, "  %s  %s  (%s)\n", t.plan.Repo.Name, t.plan.Repo.Path, what)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	switch confirmPromptCtx(ctx, promptOut, fmt.Sprintf("bring down %d repo(s)? [y/N] ", len(targets)), env.stdin) {
	case confirmAborted:
		return reportAbort(env.stderr), false
	case confirmNo:
		_, _ = fmt.Fprintf(env.stderr, "%s: down not confirmed; nothing stopped\n", progName)
		return 1, false
	default:
		return 0, true
	}
}

// rcPrintDown renders down's results; any per-repo error exits 1.
func rcPrintDown(env rcEnv, results []rcDownJSON, dryRun bool) int {
	failed := 0
	for _, r := range results {
		if r.Error != "" {
			failed++
		}
	}
	if results == nil {
		results = []rcDownJSON{}
	}
	if env.o.jsonOut {
		doc := struct {
			Command string       `json:"command"`
			Verb    string       `json:"verb"`
			DryRun  bool         `json:"dry_run"`
			Repos   []rcDownJSON `json:"repos"`
			Failed  int          `json:"failed"`
		}{"rc", "down", dryRun, results, failed}
		if err := writeJSON(env.stdout, doc); err != nil {
			return reportErr(env.stderr, err)
		}
	} else {
		var b strings.Builder
		if dryRun {
			b.WriteString("dry-run=true\n")
		}
		for _, r := range results {
			fmt.Fprintf(&b, "rc.down=%s action=%s path=%s", r.Name, r.Action, r.Path)
			if len(r.Running) > 0 {
				fmt.Fprintf(&b, " running=%s", joinInts(r.Running))
			}
			if len(r.Stopped) > 0 {
				fmt.Fprintf(&b, " stopped=%s", joinInts(r.Stopped))
			}
			if r.Tab != "" {
				fmt.Fprintf(&b, " tab=%s", r.Tab)
			}
			if r.Error != "" {
				fmt.Fprintf(&b, " error=%q", r.Error)
			}
			b.WriteByte('\n')
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
