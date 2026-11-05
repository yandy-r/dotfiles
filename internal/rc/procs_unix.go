//go:build unix

package rc

import "syscall"

func signalProcess(pid int, sig syscall.Signal) error { return syscall.Kill(pid, sig) }

// detachedAttr starts a child in a new session, detached from the caller's
// terminal and process group.
func detachedAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }
