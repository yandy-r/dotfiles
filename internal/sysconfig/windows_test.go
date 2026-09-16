package sysconfig

import (
	"errors"
	"testing"

	"github.com/yandy-r/dotfiles/internal/platform"
)

func TestWindows_Plan_NotImplemented(t *testing.T) {
	c := newWindowsConfigurer()

	plan, err := c.Plan(Config{Hostname: "win01", Timezone: "America/New_York"}, platform.Info{OS: platform.OSWindows})
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("Plan() error = %v, want ErrNotImplemented", err)
	}
	if len(plan.Steps) != 0 {
		t.Errorf("Plan() steps = %d, want 0 for the deferred Windows stub", len(plan.Steps))
	}
}
