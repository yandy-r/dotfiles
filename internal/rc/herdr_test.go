package rc

import (
	"strings"
	"syscall"
	"testing"
	"time"
)

func fastPolls(t *testing.T) {
	t.Helper()
	p, rp, rt, vt, vs := pollInterval, ReadyPoll, ReadyTimeout, VerifyTimeout, VerifySettle
	t.Cleanup(func() { pollInterval, ReadyPoll, ReadyTimeout, VerifyTimeout, VerifySettle = p, rp, rt, vt, vs })
	VerifySettle = time.Millisecond
	pollInterval, ReadyPoll, ReadyTimeout, VerifyTimeout = time.Millisecond, time.Millisecond, 50*time.Millisecond, 5*time.Millisecond
}

func repo() Resolved {
	return Resolved{Name: "app", Path: "/p/app", PermissionMode: "auto", Spawn: "worktree", AllowBypass: true, Claude: "claude"}
}

func TestEnsureServer(t *testing.T) {
	fastPolls(t)
	f := NewFakeHerdr()
	if started, err := (Herdr{R: f}).EnsureServer(time.Second); started || err != nil {
		t.Fatalf("running server: started=%v err=%v", started, err)
	}

	orig := StartDetached
	t.Cleanup(func() { StartDetached = orig })
	f.Running = false
	StartDetached = func(bin string) error {
		if bin != "/fake/bin/herdr" {
			t.Errorf("StartDetached(%q)", bin)
		}
		f.Running = true
		return nil
	}
	if started, err := (Herdr{R: f}).EnsureServer(time.Second); !started || err != nil {
		t.Fatalf("stopped server: started=%v err=%v", started, err)
	}

	f.Running = false
	StartDetached = func(string) error { return nil } // never comes up
	if _, err := (Herdr{R: f}).EnsureServer(5 * time.Millisecond); err == nil {
		t.Fatal("EnsureServer did not time out")
	}
}

func TestStarter_FreshRepo(t *testing.T) {
	fastPolls(t)
	f := NewFakeHerdr()
	s := Starter{H: Herdr{R: f}}
	p := Plan{Repo: repo()}
	ws, created, err := s.Prepare(&p)
	if err != nil || !created || ws.Label != "app" || p.TabExists {
		t.Fatalf("Prepare = %+v created=%v err=%v tabExists=%v", ws, created, err, p.TabExists)
	}
	res, err := s.Apply(p, ws, false)
	if err != nil || !res.TabCreated || res.Tab.Label != DefaultTabLabel {
		t.Fatalf("Apply = %+v, %v", res, err)
	}
	pane, _ := s.H.TabPane(res.Tab)
	if typed := f.Typed[pane.ID]; len(typed) != 1 || typed[0] != p.Repo.ShellLine() {
		t.Errorf("typed into %s = %q", pane.ID, typed)
	}
	if !f.Called("herdr pane read " + pane.ID) {
		t.Error("a new tab was not waited on before typing")
	}
}

func TestStarter_ReusesWorkspaceByCwdAndIdleTab(t *testing.T) {
	fastPolls(t)
	f := NewFakeHerdr()
	f.Workspaces = []Workspace{{ID: "w9", Label: "renamed"}}
	f.addTab("w9", "1", "/p/app")
	f.addTab("w9", DefaultTabLabel, "/p/app")
	s := Starter{H: Herdr{R: f}}
	p := Plan{Repo: repo()}
	ws, created, err := s.Prepare(&p)
	if err != nil || created || ws.ID != "w9" || !p.TabExists {
		t.Fatalf("Prepare = %+v created=%v err=%v tabExists=%v", ws, created, err, p.TabExists)
	}
	res, err := s.Apply(p, ws, false)
	if err != nil || res.TabCreated || f.Called("herdr tab close") || f.Called("herdr workspace create") {
		t.Fatalf("idle tab not reused: %+v, %v, calls=%v", res, err, f.Calls)
	}
}

func TestStarter_RestartStopsClosesAndRecreates(t *testing.T) {
	fastPolls(t)
	origSig, origIs := Signal, isServer
	t.Cleanup(func() { Signal, isServer = origSig, origIs })
	var killed []int
	gone := map[int]bool{}
	Signal = func(pid int, _ syscall.Signal) error { killed = append(killed, pid); gone[pid] = true; return nil }
	isServer = func(s Server) bool { return !gone[s.PID] }

	f := NewFakeHerdr()
	f.Workspaces = []Workspace{{ID: "w1", Label: "app"}}
	f.addTab("w1", DefaultTabLabel, "/p/app")
	oldTab := f.Tabs[0].ID
	s := Starter{H: Herdr{R: f}}
	p := Plan{Repo: repo(), Running: []Server{{PID: 41, Dir: "/p/app"}, {PID: 42, Dir: "/p/app"}}}
	ws, _, err := s.Prepare(&p)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Busy(true) {
		t.Fatal("plan with running servers is not Busy")
	}
	res, err := s.Apply(p, ws, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(killed) != 2 || len(res.Stopped) != 2 {
		t.Errorf("killed=%v stopped=%v", killed, res.Stopped)
	}
	if !f.Called("herdr tab close "+oldTab) || !res.TabCreated || res.Tab.ID == oldTab {
		t.Errorf("old tab not replaced: res=%+v calls=%v", res, f.Calls)
	}
}

func TestCreateTab_PicksTheNewTabNotAnOldSameLabel(t *testing.T) {
	f := NewFakeHerdr()
	f.Workspaces = []Workspace{{ID: "w1", Label: "app"}}
	f.addTab("w1", DefaultTabLabel, "/p/app") // a user-made tab with the same label
	old := f.Tabs[0].ID
	got, err := (Herdr{R: f}).CreateTab("w1", "/p/app", DefaultTabLabel)
	if err != nil || got.ID == old {
		t.Fatalf("CreateTab = %+v, %v; must not return the pre-existing %s", got, err, old)
	}
}

func TestRepoTab_RequiresThePaneInTheRepo(t *testing.T) {
	f := NewFakeHerdr()
	f.Workspaces = []Workspace{{ID: "w1", Label: "app"}}
	f.addTab("w1", DefaultTabLabel, "/elsewhere")
	h := Herdr{R: f}
	if _, ok, _ := h.RepoTab("w1", "/p/app"); ok {
		t.Fatal("RepoTab matched an rc tab whose pane is not in the repo")
	}
	f.Panes[0].ForegroundCwd = "/p/app"
	if _, ok, _ := h.RepoTab("w1", "/p/app"); !ok {
		t.Fatal("RepoTab ignored a pane whose foreground cwd is the repo")
	}
}

func TestPlan_Busy(t *testing.T) {
	if (Plan{TabExists: true}).Busy(true) {
		t.Error("an idle rc tab on a procfs host must not need a restart decision")
	}
	if !(Plan{TabExists: true}).Busy(false) {
		t.Error("without procfs an existing rc tab must be treated as possibly running")
	}
}

func TestHerdr_ErrorsPropagate(t *testing.T) {
	f := NewFakeHerdr()
	f.FailOn = "herdr workspace list"
	if _, _, err := (Herdr{R: f}).EnsureWorkspace("a", "/a"); err == nil || !strings.Contains(err.Error(), "workspace list") {
		t.Fatalf("EnsureWorkspace err = %v", err)
	}
}

func TestPlanRepos_MissingDir(t *testing.T) {
	dir := t.TempDir()
	plans := PlanRepos([]Resolved{{Name: "ok", Path: dir}, {Name: "gone", Path: dir + "/nope"}}, nil)
	if plans[0].Reason != "" || plans[1].Reason == "" {
		t.Errorf("reasons = %q, %q", plans[0].Reason, plans[1].Reason)
	}
}

func TestWaitForServer_RequiresItToStayUp(t *testing.T) {
	fastPolls(t)
	ProcRoot = fakeProc(t, map[int][2]any{3: {[]string{"claude", "rc"}, "/p/app"}})
	t.Cleanup(func() { ProcRoot = "/proc" })
	origIs := isServer
	t.Cleanup(func() { isServer = origIs })

	if !waitForServer("/p/app", 20*time.Millisecond) {
		t.Error("a server that stays up was not verified")
	}
	isServer = func(Server) bool { return false } // exits right after it is seen
	if waitForServer("/p/app", 20*time.Millisecond) {
		t.Error("a server that exited right after starting was verified")
	}
	// The first process exits but a replacement keeps running: still verified.
	first := true
	isServer = func(Server) bool {
		if first {
			first = false
			return false
		}
		return true
	}
	if !waitForServer("/p/app", 50*time.Millisecond) {
		t.Error("a server replaced after its first process exited was not verified")
	}
	if waitForServer("/p/none", 5*time.Millisecond) {
		t.Error("verified a directory with no server")
	}
}
