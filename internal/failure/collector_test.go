package failure

import (
	"errors"
	"strings"
	"testing"
)

func TestCollector_AllSuccess(t *testing.T) {
	var c Collector
	c.Record("a", nil)
	c.Record("b", nil)
	c.Record("c", nil)

	if c.Failed() {
		t.Fatalf("Failed() = true, want false")
	}
	if got := c.ExitCode(); got != 0 {
		t.Fatalf("ExitCode() = %d, want 0", got)
	}
	if got := c.failures; len(got) != 0 {
		t.Fatalf("failures = %v, want empty", got)
	}

	var sb strings.Builder
	c.WriteSummary(&sb, "dotf")
	got := sb.String()

	want := "dotf: apply summary: steps=3 ok=3 failed=0\n"
	if got != want {
		t.Fatalf("WriteSummary() = %q, want %q", got, want)
	}
}

func TestCollector_AccumulatesAll(t *testing.T) {
	errB := errors.New("boom b")
	errC := errors.New("boom c")

	var c Collector
	c.Record("a", nil)
	c.Record("b", errB)
	c.Record("c", errC)

	if !c.Failed() {
		t.Fatalf("Failed() = false, want true")
	}
	if got := c.ExitCode(); got != 1 {
		t.Fatalf("ExitCode() = %d, want 1", got)
	}

	failures := c.failures
	want := []Failure{{Step: "b", Err: errB}, {Step: "c", Err: errC}}
	if len(failures) != len(want) {
		t.Fatalf("failures len = %d, want %d", len(failures), len(want))
	}
	for i, f := range failures {
		if f.Step != want[i].Step || f.Err != want[i].Err {
			t.Fatalf("failures[%d] = %+v, want %+v", i, f, want[i])
		}
	}

	var sb strings.Builder
	c.WriteSummary(&sb, "dotf")
	got := sb.String()

	wantSummary := "dotf: apply summary: steps=3 ok=1 failed=2\n" +
		"dotf: b: boom b\n" +
		"dotf: c: boom c\n"
	if got != wantSummary {
		t.Fatalf("WriteSummary() = %q, want %q", got, wantSummary)
	}
}

func TestCollector_NilErrIgnored(t *testing.T) {
	var c Collector
	c.Record("only", nil)

	if c.Failed() {
		t.Fatalf("Failed() = true, want false")
	}
	if got := c.ExitCode(); got != 0 {
		t.Fatalf("ExitCode() = %d, want 0", got)
	}
	if got := c.failures; len(got) != 0 {
		t.Fatalf("failures = %v, want empty", got)
	}

	var sb strings.Builder
	c.WriteSummary(&sb, "dotf")
	if got, want := sb.String(), "dotf: apply summary: steps=1 ok=1 failed=0\n"; got != want {
		t.Fatalf("WriteSummary() = %q, want %q", got, want)
	}
}
