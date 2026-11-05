package rc

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/yandy-r/dotfiles/internal/exec"
)

// FakeHerdr is a stateful exec.Runner test double for the herdr CLI (plus
// optional systemctl): workspace/tab create and tab close change what later
// list calls return, which exec.Fake's fixed canned outputs cannot model.
// Exported, like exec.Fake, so cmd/dotf tests drive the same double.
type FakeHerdr struct {
	mu         sync.Mutex
	Running    bool              // herdr status server reports running
	Workspaces []Workspace       // current workspaces
	Tabs       []Tab             // current tabs
	Panes      []Pane            // current panes
	Screens    map[string]string // pane id -> `pane read` text (default "$ ")
	Paths      map[string]string // LookPath results; herdr is always found
	Typed      map[string][]string
	Calls      []string // every invocation, space-joined
	FailOn     string   // a call prefix that returns an error
	next       int
}

// NewFakeHerdr returns a FakeHerdr with a running server and no workspaces.
func NewFakeHerdr() *FakeHerdr {
	return &FakeHerdr{Running: true, Screens: map[string]string{}, Paths: map[string]string{}, Typed: map[string][]string{}}
}

var _ exec.Runner = (*FakeHerdr)(nil)

// LookPath resolves herdr always, and any name registered in Paths.
func (f *FakeHerdr) LookPath(name string) (string, error) {
	if p, ok := f.Paths[name]; ok {
		return p, nil
	}
	if name == "herdr" {
		return "/fake/bin/herdr", nil
	}
	return "", errors.New("not found")
}

// Run and RunDir are unused by rc.
func (f *FakeHerdr) Run(string, ...string) error { return errors.New("FakeHerdr: Run unsupported") }

// RunDir is unused by rc.
func (f *FakeHerdr) RunDir(string, string, ...string) error {
	return errors.New("FakeHerdr: RunDir unsupported")
}

func envelope(v any) []byte {
	b, _ := json.Marshal(map[string]any{"result": v})
	return b
}

// flagVal returns the value following --name in args.
func flagVal(args []string, name string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--"+name {
			return args[i+1]
		}
	}
	return ""
}

// Output implements the herdr subcommands rc uses.
func (f *FakeHerdr) Output(name string, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	call := strings.Join(append([]string{name}, args...), " ")
	f.Calls = append(f.Calls, call)
	if f.FailOn != "" && strings.HasPrefix(call, f.FailOn) {
		return nil, fmt.Errorf("fake failure: %s", call)
	}
	if name == "systemctl" {
		return nil, nil
	}
	if len(args) < 2 {
		return nil, fmt.Errorf("FakeHerdr: unexpected %q", call)
	}
	switch args[0] + " " + args[1] {
	case "status server":
		return json.Marshal(map[string]bool{"running": f.Running})
	case "workspace list":
		return envelope(map[string]any{"workspaces": f.Workspaces}), nil
	case "workspace create":
		f.next++
		ws := Workspace{ID: fmt.Sprintf("w%d", f.next), Label: flagVal(args, "label")}
		f.Workspaces = append(f.Workspaces, ws)
		f.addTab(ws.ID, "1", flagVal(args, "cwd"))
		return envelope(map[string]any{}), nil
	case "tab list":
		var tabs []Tab
		for _, t := range f.Tabs {
			if t.WorkspaceID == flagVal(args, "workspace") {
				tabs = append(tabs, t)
			}
		}
		return envelope(map[string]any{"tabs": tabs}), nil
	case "tab create":
		f.addTab(flagVal(args, "workspace"), flagVal(args, "label"), flagVal(args, "cwd"))
		return envelope(map[string]any{}), nil
	case "tab close":
		f.closeTab(args[2])
		return envelope(map[string]any{}), nil
	case "pane list":
		var panes []Pane
		for _, p := range f.Panes {
			if p.WorkspaceID == flagVal(args, "workspace") {
				panes = append(panes, p)
			}
		}
		return envelope(map[string]any{"panes": panes}), nil
	case "pane read":
		if s, ok := f.Screens[args[2]]; ok {
			return []byte(s), nil
		}
		return []byte("$ "), nil
	case "pane run":
		f.Typed[args[2]] = append(f.Typed[args[2]], args[3])
		return envelope(map[string]any{}), nil
	}
	return nil, fmt.Errorf("FakeHerdr: unexpected %q", call)
}

func (f *FakeHerdr) addTab(wsID, label, cwd string) {
	f.next++
	t := Tab{ID: fmt.Sprintf("%s:t%d", wsID, f.next), Label: label, WorkspaceID: wsID}
	f.Tabs = append(f.Tabs, t)
	f.Panes = append(f.Panes, Pane{ID: fmt.Sprintf("%s:p%d", wsID, f.next), TabID: t.ID, WorkspaceID: wsID, Cwd: cwd})
}

func (f *FakeHerdr) closeTab(id string) {
	var tabs []Tab
	for _, t := range f.Tabs {
		if t.ID != id {
			tabs = append(tabs, t)
		}
	}
	f.Tabs = tabs
	var panes []Pane
	for _, p := range f.Panes {
		if p.TabID != id {
			panes = append(panes, p)
		}
	}
	f.Panes = panes
}

// Called reports whether any recorded call starts with prefix.
func (f *FakeHerdr) Called(prefix string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.Calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}
