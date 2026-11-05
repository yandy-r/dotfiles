package rc

import (
	"fmt"
	"os"
	"time"
)

// Action is what `dotf rc up` decided for one repo.
type Action string

// Up actions.
const (
	ActionStart   Action = "start"   // no server running: start one
	ActionRestart Action = "restart" // a server is running: stop it, then start
	ActionKeep    Action = "keep"    // a server is running and restart was declined
	ActionSkip    Action = "skip"    // the repo cannot be served (missing dir)
)

// Plan is the per-repo state `up` acts on.
type Plan struct {
	Repo    Resolved
	Running []Server // servers found in Repo.Path (empty when none, or no procfs)
	// TabExists reports a pre-existing rc tab in the repo's Herdr workspace.
	// It is only known once the workspace is resolved (not in a dry run that
	// skipped Herdr).
	TabExists bool
	Reason    string // why the repo is skipped
}

// Busy reports whether a restart decision is needed for p: a server process
// was found, or (on a host without procfs) an rc tab already exists and may
// be hosting one.
func (p Plan) Busy(procOK bool) bool {
	if len(p.Running) > 0 {
		return true
	}
	return !procOK && p.TabExists
}

// PlanRepos inspects every repo: whether its directory exists and which
// servers are already running in it. servers comes from FindServers (nil when
// procfs is unavailable).
func PlanRepos(repos []Resolved, servers map[string][]Server) []Plan {
	plans := make([]Plan, 0, len(repos))
	for _, r := range repos {
		p := Plan{Repo: r, Running: servers[r.Path]}
		if st, err := os.Stat(r.Path); err != nil || !st.IsDir() {
			p.Reason = "directory does not exist"
		}
		plans = append(plans, p)
	}
	return plans
}

// StopGrace is how long Stop waits after SIGTERM before SIGKILL.
var StopGrace = 5 * time.Second

// Starter performs one repo's start or restart inside Herdr.
type Starter struct {
	H Herdr
}

// Result is the outcome of Starter.Apply for one repo.
type Result struct {
	Workspace        Workspace
	WorkspaceCreated bool
	Tab              Tab
	TabCreated       bool
	Stopped          []int
	// Verified reports that a server process was seen in the repo after the
	// command was typed. It stays false when procfs is unavailable.
	Verified bool
}

// VerifyTimeout bounds how long Apply looks for the new server process;
// exported so tests run fast.
var VerifyTimeout = 10 * time.Second

// Prepare resolves (creating if needed) the repo's workspace and reports
// whether an rc tab already exists, so the caller can decide on a restart
// before anything is killed.
func (s Starter) Prepare(p *Plan) (Workspace, bool, error) {
	ws, created, err := s.H.EnsureWorkspace(p.Repo.Name, p.Repo.Path)
	if err != nil {
		return Workspace{}, false, err
	}
	_, ok, err := s.H.FindTab(ws.ID, DefaultTabLabel)
	if err != nil {
		return ws, created, err
	}
	p.TabExists = ok
	return ws, created, nil
}

// Apply starts the repo's server. With restart it first stops every running
// server found in the repo and closes the existing rc tab, so the new server
// gets a fresh shell; without restart an existing, idle rc tab is reused.
func (s Starter) Apply(p Plan, ws Workspace, restart bool) (Result, error) {
	res := Result{Workspace: ws}
	if restart {
		for _, srv := range p.Running {
			if err := Stop(srv, StopGrace); err != nil {
				return res, fmt.Errorf("stop pid %d: %w", srv.PID, err)
			}
			res.Stopped = append(res.Stopped, srv.PID)
		}
	}
	tab, ok, err := s.H.FindTab(ws.ID, DefaultTabLabel)
	if err != nil {
		return res, err
	}
	if ok && restart {
		if err := s.H.CloseTab(tab.ID); err != nil {
			return res, err
		}
		ok = false
	}
	if !ok {
		if tab, err = s.H.CreateTab(ws.ID, p.Repo.Path, DefaultTabLabel); err != nil {
			return res, err
		}
		res.TabCreated = true
	}
	res.Tab = tab
	pane, err := s.H.TabPane(tab)
	if err != nil {
		return res, err
	}
	if res.TabCreated {
		s.H.WaitReady(pane.ID)
	}
	if err := s.H.Run(pane.ID, p.Repo.ShellLine()); err != nil {
		return res, err
	}
	if ProcSupported() {
		res.Verified = waitForServer(p.Repo.Path, VerifyTimeout)
	}
	return res, nil
}

// waitForServer polls until a server is running in dir or timeout elapses.
func waitForServer(dir string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if servers, err := FindServers(); err == nil && len(servers[dir]) > 0 {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(pollInterval)
	}
}
