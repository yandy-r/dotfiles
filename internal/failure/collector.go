// Package failure accumulates non-fatal step outcomes across a multi-step
// provisioning run so dotf apply reports every failure at the end instead of
// aborting on the first, mirroring bootstrap.sh's BOOTSTRAP_FAILURES.
//
// A Collector records one sub-outcome per step (one per sysconfig domain, one
// per package, one for the symlink phase, one per firewall step) so its steps
// count matches bootstrap's granularity. Its zero value is ready to use. It is
// NOT concurrency-safe; callers must serialize access if used across
// goroutines.
package failure

import (
	"fmt"
	"io"
)

// Failure records a single failed step and its cause. Step is a short,
// colon-scoped identifier such as "packages:rg", "sysconfig:hostname", or
// "symlinks".
type Failure struct {
	Step string
	Err  error
}

// Collector accumulates step outcomes for a provisioning run. The zero value is
// ready to use. It is NOT concurrency-safe.
type Collector struct {
	total    int
	failures []Failure
}

// Record notes one step outcome. A nil err counts as a successful step; a
// non-nil err is accumulated in run order. Every call increments the total
// step count.
func (c *Collector) Record(step string, err error) {
	c.total++
	if err != nil {
		c.failures = append(c.failures, Failure{Step: step, Err: err})
	}
}

// Failed reports whether any recorded step failed.
func (c *Collector) Failed() bool {
	return len(c.failures) > 0
}

// Failures returns the accumulated failures in run order. The returned slice is
// a defensive copy; mutating it does not affect the collector.
func (c *Collector) Failures() []Failure {
	out := make([]Failure, len(c.failures))
	copy(out, c.failures)
	return out
}

// WriteSummary writes the run's aggregate line to w, followed by one line per
// failure in run order. The aggregate line is always written, even when every
// step succeeded. prog is the program name prefix (e.g. "dotf").
func (c *Collector) WriteSummary(w io.Writer, prog string) {
	failed := len(c.failures)
	ok := c.total - failed
	_, _ = fmt.Fprintf(w, "%s: apply summary: steps=%d ok=%d failed=%d\n",
		prog, c.total, ok, failed)
	for _, f := range c.failures {
		_, _ = fmt.Fprintf(w, "%s: %s: %v\n", prog, f.Step, f.Err)
	}
}

// ExitCode returns 1 when any step failed, else 0.
func (c *Collector) ExitCode() int {
	if c.Failed() {
		return 1
	}
	return 0
}
