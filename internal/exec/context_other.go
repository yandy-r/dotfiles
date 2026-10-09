//go:build !unix

package exec

import (
	"context"
	osexec "os/exec"
)

// commandContext is the portable fallback: CommandContext kills only the
// direct child on cancellation (no process-group support off unix).
func commandContext(ctx context.Context, name string, args ...string) *osexec.Cmd {
	return osexec.CommandContext(ctx, name, args...)
}
