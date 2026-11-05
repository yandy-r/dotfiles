package rc

import "fmt"

// DownResult is the outcome of Down for one repo.
type DownResult struct {
	Stopped []int  // server PIDs stopped
	Tab     string // the rc tab closed ("" when none was found)
}

// Down brings one repo's Remote Control server down: it stops every server
// found in the repo (Stop re-checks each PID before signalling) and then
// closes the repo's rc tab, which also ends a server /proc could not see. A
// nil-runner Herdr (herdrOK=false, e.g. no Herdr server running) skips the
// tab step: there is no tab to close without a server.
func Down(p Plan, h Herdr, herdrOK bool) (DownResult, error) {
	var res DownResult
	for _, srv := range p.Running {
		if err := Stop(srv, StopGrace); err != nil {
			return res, fmt.Errorf("stop pid %d: %w", srv.PID, err)
		}
		res.Stopped = append(res.Stopped, srv.PID)
	}
	if !herdrOK {
		return res, nil
	}
	ws, ok, err := h.FindWorkspace(p.Repo.Name, p.Repo.Path)
	if err != nil || !ok {
		return res, err
	}
	tab, ok, err := h.RepoTab(ws.ID, p.Repo.Path)
	if err != nil || !ok {
		return res, err
	}
	if err := h.CloseTab(tab.ID); err != nil {
		return res, err
	}
	res.Tab = tab.ID
	return res, nil
}
