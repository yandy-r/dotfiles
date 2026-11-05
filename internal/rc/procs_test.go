package rc

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// fakeProc builds a /proc-like tree: pid -> (argv, cwd).
func fakeProc(t *testing.T, procs map[int][2]any) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "self"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "self", "cmdline"), []byte("x\x00"), 0o644); err != nil {
		t.Fatal(err)
	}
	for pid, pc := range procs {
		dir := filepath.Join(root, strconv.Itoa(pid))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		argv := pc[0].([]string)
		if err := os.WriteFile(filepath.Join(dir, "cmdline"), []byte(strings.Join(argv, "\x00")+"\x00"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(pc[1].(string), filepath.Join(dir, "cwd")); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestFindServers(t *testing.T) {
	ProcRoot = fakeProc(t, map[int][2]any{
		10: {[]string{"claude", "--allow-dangerously-skip-permissions", "rc"}, "/p/a"},
		11: {[]string{"/home/u/.local/bin/claude", "remote-control", "--name", "a"}, "/p/a"},
		12: {[]string{"claude", "--resume"}, "/p/a"},                                   // interactive, not a server
		13: {[]string{"claude", "chat", "rc"}, "/p/a"},                                 // first positional is not rc
		14: {[]string{"node", "claude", "rc"}, "/p/a"},                                 // different executable
		15: {[]string{"claude", "rc"}, "/p/b"},                                         // other dir
		16: {[]string{"claude", "--debug-file", "/tmp/x", "remote-control"}, "/p/c"},   // value flag first
		17: {[]string{"claude", "--name", "rc"}, "/p/c"},                               // session NAMED rc
		18: {[]string{"claude", "--resume", "remote-control"}, "/p/c"},                 // resuming a session id
		19: {[]string{"claude", "--debug=api", "--verbose", "remote-control"}, "/p/d"}, // allowed leading flags
	})
	t.Cleanup(func() { ProcRoot = "/proc" })
	if !ProcSupported() {
		t.Fatal("ProcSupported = false on the fake tree")
	}
	got, err := FindServers()
	if err != nil {
		t.Fatal(err)
	}
	if len(got["/p/a"]) != 2 || got["/p/a"][0].PID != 10 || got["/p/a"][1].PID != 11 {
		t.Errorf("/p/a servers = %+v; want pids 10, 11", got["/p/a"])
	}
	if len(got["/p/b"]) != 1 {
		t.Errorf("/p/b servers = %+v", got["/p/b"])
	}
	// Any flag outside the allowlist before the verb means no match, so an
	// interactive session named or resuming "rc" is never mistaken for (and
	// killed as) a server.
	if len(got["/p/c"]) != 0 {
		t.Errorf("/p/c servers = %+v; want none", got["/p/c"])
	}
	if len(got["/p/d"]) != 1 {
		t.Errorf("/p/d servers = %+v; want pid 19", got["/p/d"])
	}
}

func TestStop(t *testing.T) {
	origSig, origIs, origPoll := Signal, isServer, pollInterval
	t.Cleanup(func() { Signal, isServer, pollInterval = origSig, origIs, origPoll })
	pollInterval = time.Millisecond
	srv := Server{PID: 1, Dir: "/p"}

	var sent []syscall.Signal
	exited := false
	Signal = func(_ int, s syscall.Signal) error { sent = append(sent, s); exited = true; return nil }
	isServer = func(Server) bool { return !exited }
	if err := Stop(srv, time.Second); err != nil || len(sent) != 1 || sent[0] != syscall.SIGTERM {
		t.Fatalf("graceful Stop: err=%v sent=%v", err, sent)
	}

	sent = nil
	Signal = func(_ int, s syscall.Signal) error { sent = append(sent, s); return nil }
	isServer = func(Server) bool { return true }
	if err := Stop(srv, 5*time.Millisecond); err != nil || len(sent) != 2 || sent[1] != syscall.SIGKILL {
		t.Fatalf("forced Stop: err=%v sent=%v", err, sent)
	}

	// The PID is no longer a server in that dir (it exited and was reused):
	// nothing is signalled.
	sent = nil
	isServer = func(Server) bool { return false }
	if err := Stop(srv, time.Second); err != nil || len(sent) != 0 {
		t.Fatalf("Stop of a reused PID: err=%v sent=%v", err, sent)
	}

	isServer = func(Server) bool { return true }
	Signal = func(int, syscall.Signal) error { return syscall.ESRCH }
	if err := Stop(srv, time.Second); err != nil {
		t.Fatalf("Stop of a gone process = %v", err)
	}
	Signal = func(int, syscall.Signal) error { return syscall.EPERM }
	if err := Stop(srv, time.Second); !errors.Is(err, syscall.EPERM) {
		t.Fatalf("Stop with EPERM = %v", err)
	}
}

func TestStop_RechecksIdentityViaProc(t *testing.T) {
	ProcRoot = fakeProc(t, map[int][2]any{
		7: {[]string{"claude", "rc"}, "/p/a"},
		8: {[]string{"vim"}, "/p/a"},
	})
	t.Cleanup(func() { ProcRoot = "/proc" })
	if !isServer(Server{PID: 7, Dir: "/p/a"}) {
		t.Error("pid 7 should still be the server in /p/a")
	}
	if isServer(Server{PID: 8, Dir: "/p/a"}) || isServer(Server{PID: 7, Dir: "/p/b"}) || isServer(Server{PID: 99, Dir: "/p/a"}) {
		t.Error("isServer matched a non-server, wrong dir, or missing pid")
	}
}
