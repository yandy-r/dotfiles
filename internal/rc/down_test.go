package rc

import (
	"syscall"
	"testing"
)

func TestDown_StopsServersAndClosesTab(t *testing.T) {
	fastPolls(t)
	origSig, origIs := Signal, isServer
	t.Cleanup(func() { Signal, isServer = origSig, origIs })
	gone := map[int]bool{}
	Signal = func(pid int, _ syscall.Signal) error { gone[pid] = true; return nil }
	isServer = func(s Server) bool { return !gone[s.PID] }

	f := NewFakeHerdr()
	f.Workspaces = []Workspace{{ID: "w1", Label: "app"}}
	f.addTab("w1", "1", "/p/app")
	f.addTab("w1", DefaultTabLabel, "/p/app")
	rcTab := f.Tabs[1].ID
	p := Plan{Repo: repo(), Running: []Server{{PID: 5, Dir: "/p/app"}}}

	res, err := Down(p, Herdr{R: f}, true)
	if err != nil || len(res.Stopped) != 1 || res.Tab != rcTab || !gone[5] {
		t.Fatalf("Down = %+v, %v (gone=%v)", res, err, gone)
	}
	if !f.Called("herdr tab close "+rcTab) || len(f.Tabs) != 1 {
		t.Errorf("rc tab not closed / other tab touched: tabs=%v", f.Tabs)
	}
}

func TestDown_WithoutHerdrOnlyStopsProcesses(t *testing.T) {
	origSig, origIs := Signal, isServer
	t.Cleanup(func() { Signal, isServer = origSig, origIs })
	gone := map[int]bool{}
	Signal = func(pid int, _ syscall.Signal) error { gone[pid] = true; return nil }
	isServer = func(s Server) bool { return !gone[s.PID] }

	f := NewFakeHerdr()
	res, err := Down(Plan{Repo: repo(), Running: []Server{{PID: 9, Dir: "/p/app"}}}, Herdr{R: f}, false)
	if err != nil || len(res.Stopped) != 1 || len(f.Calls) != 0 {
		t.Fatalf("Down = %+v, %v; herdr calls %v", res, err, f.Calls)
	}
}

func TestDown_NoWorkspaceIsNotAnError(t *testing.T) {
	res, err := Down(Plan{Repo: repo()}, Herdr{R: NewFakeHerdr()}, true)
	if err != nil || res.Tab != "" || len(res.Stopped) != 0 {
		t.Fatalf("Down = %+v, %v", res, err)
	}
}

func TestDown_LeavesAnRcTabOutsideTheRepo(t *testing.T) {
	f := NewFakeHerdr()
	f.Workspaces = []Workspace{{ID: "w1", Label: "app"}}
	f.addTab("w1", DefaultTabLabel, "/elsewhere") // user tab named rc, not in the repo
	res, err := Down(Plan{Repo: repo()}, Herdr{R: f}, true)
	if err != nil || res.Tab != "" || f.Called("herdr tab close") {
		t.Fatalf("Down closed a foreign rc tab: %+v, %v, calls=%v", res, err, f.Calls)
	}
}
