package rc

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"slices"
	"strings"
	"time"

	"github.com/yandy-r/dotfiles/internal/exec"
)

// Herdr drives the Herdr terminal workspace manager through its CLI. Every
// call goes through the injected Runner, so tests substitute exec.Fake.
type Herdr struct {
	R   exec.Runner
	Bin string // herdr executable (default "herdr")
}

// Workspace is a Herdr workspace. Workspace, Tab and Pane carry only the
// fields of Herdr's socket-API objects that rc reads.
type Workspace struct {
	ID    string `json:"workspace_id"`
	Label string `json:"label"`
}

// Tab is a Herdr tab.
type Tab struct {
	ID          string `json:"tab_id"`
	Label       string `json:"label"`
	WorkspaceID string `json:"workspace_id"`
}

// Pane is a Herdr pane.
type Pane struct {
	ID          string `json:"pane_id"`
	TabID       string `json:"tab_id"`
	WorkspaceID string `json:"workspace_id"`
	Cwd         string `json:"cwd"`
}

func (h Herdr) bin() string {
	if h.Bin != "" {
		return h.Bin
	}
	return "herdr"
}

// Available reports whether the herdr executable is on PATH.
func (h Herdr) Available() bool {
	_, err := h.R.LookPath(h.bin())
	return err == nil
}

// ServerRunning reports whether a Herdr server is reachable.
func (h Herdr) ServerRunning() bool {
	out, err := h.R.Output(h.bin(), "status", "server", "--json")
	if err != nil {
		return false
	}
	var st struct {
		Running bool `json:"running"`
	}
	return json.Unmarshal(out, &st) == nil && st.Running
}

// StartDetached launches `herdr server` in its own session so it outlives the
// caller (a shell or a systemd oneshot). A package var so tests never fork.
var StartDetached = func(bin string) error {
	cmd := osexec.Command(bin, "server") //nolint:gosec // fixed argv, resolved binary
	cmd.SysProcAttr = detachedAttr()
	devnull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer func() { _ = devnull.Close() }()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = devnull, devnull, devnull
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// EnsureServer starts the Herdr server when none is running and waits up to
// timeout for it to answer. started reports whether this call launched it.
func (h Herdr) EnsureServer(timeout time.Duration) (started bool, err error) {
	if h.ServerRunning() {
		return false, nil
	}
	bin, err := h.R.LookPath(h.bin())
	if err != nil {
		return false, fmt.Errorf("herdr not found on PATH: %w", err)
	}
	if err := StartDetached(bin); err != nil {
		return false, fmt.Errorf("start herdr server: %w", err)
	}
	deadline := time.Now().Add(timeout)
	for {
		if h.ServerRunning() {
			return true, nil
		}
		if time.Now().After(deadline) {
			return true, fmt.Errorf("herdr server did not come up within %s", timeout)
		}
		time.Sleep(pollInterval)
	}
}

// call runs a herdr API subcommand and decodes its {"result": ...} envelope
// into dst.
func (h Herdr) call(dst any, args ...string) error {
	out, err := h.R.Output(h.bin(), args...)
	if err != nil {
		return fmt.Errorf("herdr %s: %w", strings.Join(args, " "), stderrOf(err))
	}
	if dst == nil && len(strings.TrimSpace(string(out))) == 0 {
		return nil
	}
	var env struct {
		Result json.RawMessage `json:"result"`
		Error  any             `json:"error"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		return fmt.Errorf("herdr %s: decode: %w", strings.Join(args, " "), err)
	}
	if env.Error != nil {
		return fmt.Errorf("herdr %s: %v", strings.Join(args, " "), env.Error)
	}
	if dst == nil || len(env.Result) == 0 {
		return nil
	}
	if err := json.Unmarshal(env.Result, dst); err != nil {
		return fmt.Errorf("herdr %s: decode result: %w", strings.Join(args, " "), err)
	}
	return nil
}

func stderrOf(err error) error {
	var ee *osexec.ExitError
	if errors.As(err, &ee) && len(ee.Stderr) > 0 {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(ee.Stderr)))
	}
	return err
}

// Workspaces lists every workspace.
func (h Herdr) Workspaces() ([]Workspace, error) {
	var res struct {
		Workspaces []Workspace `json:"workspaces"`
	}
	err := h.call(&res, "workspace", "list")
	return res.Workspaces, err
}

// Tabs lists the tabs of one workspace.
func (h Herdr) Tabs(workspaceID string) ([]Tab, error) {
	var res struct {
		Tabs []Tab `json:"tabs"`
	}
	err := h.call(&res, "tab", "list", "--workspace", workspaceID)
	return res.Tabs, err
}

// Panes lists the panes of one workspace.
func (h Herdr) Panes(workspaceID string) ([]Pane, error) {
	var res struct {
		Panes []Pane `json:"panes"`
	}
	err := h.call(&res, "pane", "list", "--workspace", workspaceID)
	return res.Panes, err
}

// FindWorkspace returns the workspace for a repo: the one labelled name, else
// the first one with a pane whose cwd is dir.
func (h Herdr) FindWorkspace(name, dir string) (Workspace, bool, error) {
	wss, err := h.Workspaces()
	if err != nil {
		return Workspace{}, false, err
	}
	for _, ws := range wss {
		if ws.Label == name {
			return ws, true, nil
		}
	}
	for _, ws := range wss {
		panes, err := h.Panes(ws.ID)
		if err != nil {
			return Workspace{}, false, err
		}
		for _, p := range panes {
			if p.Cwd == dir {
				return ws, true, nil
			}
		}
	}
	return Workspace{}, false, nil
}

// EnsureWorkspace returns the repo's workspace, creating it (unfocused, in
// dir, labelled name) when none exists.
func (h Herdr) EnsureWorkspace(name, dir string) (ws Workspace, created bool, err error) {
	if ws, ok, err := h.FindWorkspace(name, dir); err != nil || ok {
		return ws, false, err
	}
	if err := h.call(nil, "workspace", "create", "--cwd", dir, "--label", name, "--no-focus"); err != nil {
		return Workspace{}, false, err
	}
	ws, ok, err := h.FindWorkspace(name, dir)
	if err != nil {
		return Workspace{}, true, err
	}
	if !ok {
		return Workspace{}, true, fmt.Errorf("herdr workspace %q was created but is not listed", name)
	}
	return ws, true, nil
}

// FindTab returns the tab labelled label in workspaceID.
func (h Herdr) FindTab(workspaceID, label string) (Tab, bool, error) {
	tabs, err := h.Tabs(workspaceID)
	if err != nil {
		return Tab{}, false, err
	}
	for _, t := range tabs {
		if t.Label == label {
			return t, true, nil
		}
	}
	return Tab{}, false, nil
}

// CreateTab creates an unfocused tab labelled label in workspaceID, rooted at
// dir. It returns the tab that did not exist before the call, so a second,
// user-made tab with the same label is never mistaken for the new one.
func (h Herdr) CreateTab(workspaceID, dir, label string) (Tab, error) {
	before, err := h.Tabs(workspaceID)
	if err != nil {
		return Tab{}, err
	}
	if err := h.call(nil, "tab", "create", "--workspace", workspaceID, "--cwd", dir, "--label", label, "--no-focus"); err != nil {
		return Tab{}, err
	}
	after, err := h.Tabs(workspaceID)
	if err != nil {
		return Tab{}, err
	}
	for _, t := range after {
		if t.Label == label && !slices.ContainsFunc(before, func(b Tab) bool { return b.ID == t.ID }) {
			return t, nil
		}
	}
	return Tab{}, fmt.Errorf("herdr tab %q was created but is not listed", label)
}

// CloseTab closes a tab and every process running in it.
func (h Herdr) CloseTab(tabID string) error {
	return h.call(nil, "tab", "close", tabID)
}

// TabPane returns the first pane of tab t.
func (h Herdr) TabPane(t Tab) (Pane, error) {
	panes, err := h.Panes(t.WorkspaceID)
	if err != nil {
		return Pane{}, err
	}
	for _, p := range panes {
		if p.TabID == t.ID {
			return p, nil
		}
	}
	return Pane{}, fmt.Errorf("herdr tab %s has no pane", t.ID)
}

// ReadyPoll and ReadyTimeout pace WaitReady; exported so tests run fast.
var (
	ReadyPoll    = 400 * time.Millisecond
	ReadyTimeout = 15 * time.Second
)

// WaitReady waits until paneID's screen is non-empty and unchanged across two
// consecutive polls, i.e. its shell has drawn a prompt and gone idle. Typing
// into a freshly created tab before then loses keystrokes (a slow-starting
// shell such as one loading ble.sh drops the submitting Enter). Timing out is
// not an error: the caller types anyway and verifies the server separately.
func (h Herdr) WaitReady(paneID string) {
	deadline := time.Now().Add(ReadyTimeout)
	prev := ""
	for time.Now().Before(deadline) {
		out, err := h.R.Output(h.bin(), "pane", "read", paneID, "--source", "visible")
		cur := strings.TrimSpace(string(out))
		if err == nil && cur != "" && cur == prev {
			return
		}
		prev = cur
		time.Sleep(ReadyPoll)
	}
}

// Run types line into paneID's shell and submits it.
func (h Herdr) Run(paneID, line string) error {
	return h.call(nil, "pane", "run", paneID, line)
}
