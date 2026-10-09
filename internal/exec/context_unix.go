//go:build unix

package exec

import (
	"context"
	osexec "os/exec"
	"syscall"
	"time"
)

// commandContext builds an exec.Cmd bound to ctx. The child runs in its own
// process group; on cancellation the whole group is SIGKILLed so shell
// grandchildren die too, and WaitDelay bounds how long Wait blocks on pipes
// still held open by dying descendants before the call returns.
func commandContext(ctx context.Context, name string, args ...string) *osexec.Cmd {
	cmd := osexec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 5 * time.Second
	return cmd
}
