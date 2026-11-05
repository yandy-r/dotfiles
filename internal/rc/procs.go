package rc

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Server is a running `claude remote-control` process.
type Server struct {
	PID int
	Dir string
}

// ProcRoot is the procfs mount scanned for running servers. A package var so
// tests can point it at a fake tree.
var ProcRoot = "/proc"

// ProcSupported reports whether server discovery works on this host (it needs
// a Linux-style procfs).
func ProcSupported() bool {
	_, err := os.Stat(filepath.Join(ProcRoot, "self", "cmdline"))
	return err == nil
}

// FindServers returns every running Remote Control server, keyed by its
// working directory. A process qualifies when its executable's base name is
// "claude" and its verb is "remote-control" or "rc" (see isServerCmdline).
// Processes that vanish or cannot be read mid-scan are skipped.
func FindServers() (map[string][]Server, error) {
	entries, err := os.ReadDir(ProcRoot)
	if err != nil {
		return nil, err
	}
	out := map[string][]Server{}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid <= 0 {
			continue
		}
		dir, ok := serverDir(pid)
		if !ok {
			continue
		}
		out[dir] = append(out[dir], Server{PID: pid, Dir: dir})
	}
	for d := range out {
		slices.SortFunc(out[d], func(a, b Server) int { return a.PID - b.PID })
	}
	return out, nil
}

// serverDir reports pid's working directory when pid is a Remote Control
// server.
func serverDir(pid int) (string, bool) {
	base := filepath.Join(ProcRoot, strconv.Itoa(pid))
	raw, err := os.ReadFile(filepath.Join(base, "cmdline")) //nolint:gosec // procfs path
	if err != nil || !isServerCmdline(raw) {
		return "", false
	}
	dir, err := os.Readlink(filepath.Join(base, "cwd"))
	if err != nil {
		return "", false
	}
	return filepath.Clean(dir), true
}

// leadingFlags are the only flags accepted before the verb. They are the
// boolean top-level flags `dotf rc` itself uses (plus --verbose/--debug). Any
// other flag before the verb means no match: a value-taking flag would make
// its value look like the verb (`claude --name rc` is an interactive session
// named rc, not a server), and a missed server is safer than a killed session.
var leadingFlags = []string{"--allow-dangerously-skip-permissions", "--dangerously-skip-permissions", "--verbose"}

// isServerCmdline reports whether a NUL-separated /proc cmdline is a Remote
// Control server: claude, then only leadingFlags (or --debug[=filter]), then
// the verb remote-control or rc.
func isServerCmdline(raw []byte) bool {
	argv := strings.Split(strings.TrimRight(string(raw), "\x00"), "\x00")
	if len(argv) < 2 || filepath.Base(argv[0]) != "claude" {
		return false
	}
	for _, a := range argv[1:] {
		switch {
		case a == "remote-control" || a == "rc":
			return true
		case slices.Contains(leadingFlags, a), a == "--debug", strings.HasPrefix(a, "--debug="):
			continue
		default:
			return false
		}
	}
	return false
}

// Signal is the process-signalling seam Stop uses; tests replace it, and
// isServer (Stop's identity re-check).
var (
	Signal   = signalProcess
	isServer = func(s Server) bool {
		dir, ok := serverDir(s.PID)
		return ok && dir == s.Dir
	}
	pollInterval = 100 * time.Millisecond
)

// Stop sends SIGTERM to s and waits up to grace for it to exit, then sends
// SIGKILL. Before each signal it re-checks that s.PID is still a server in
// s.Dir, so a server that exited (and whose PID was reused) since the scan is
// never signalled. A process that is already gone is not an error.
func Stop(s Server, grace time.Duration) error {
	if !isServer(s) {
		return nil
	}
	if err := Signal(s.PID, syscall.SIGTERM); err != nil {
		if errors.Is(err, syscall.ESRCH) || errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		if !isServer(s) {
			return nil
		}
		time.Sleep(pollInterval)
	}
	if !isServer(s) {
		return nil
	}
	if err := Signal(s.PID, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}
