# Graceful Ctrl+C / Esc Exit — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a user abort any interactive step of `dotf` (the huh wizard in `plan`/`apply`, and the `sysconfig` y/N confirm) with Ctrl+C — and Esc in the wizard — producing one clean `dotf: aborted` line and exit code 130.

**Architecture:** One shared abort vocabulary in `cmd/dotf` (`abortExitCode=130`, `reportAbort`, `wizardExit`). The huh wizard gains an Esc→Quit keymap and re-exports huh's abort sentinel as `tui.ErrAborted`; callers classify it via `errors.Is`. The `sysconfig` confirm read becomes interrupt-aware with a `signal.NotifyContext` **scoped to the read only**, so the execution phase keeps default SIGINT behavior.

**Tech Stack:** Go 1.26; `charm.land/huh/v2` v2.0.3; `charm.land/bubbles/v2/key` v2.0.0 (currently indirect — promoted to direct).

## Global Constraints

- Exit code for a user abort (Ctrl+C / Esc) = **130** (128 + SIGINT). A deliberate "No"/EOF at a confirm stays **1** ("not confirmed"). Never conflate the two.
- Abort message is exactly one line: `dotf: aborted\n` on **stderr**. No `progName: <err>` wrapping, no stack.
- Scope is interactive prompt/wizard steps only. Do NOT add interrupt handling to execution phases (installs, sysconfig apply, symlink writes) — they keep default SIGINT. Do NOT add a process-wide signal handler.
- `signal.NotifyContext` in `sysconfig` MUST be scoped to the confirm read (its `stop()` runs before the execution phase), never to all of `runSysconfig`.
- Only `os.Interrupt` is registered (portable, no `syscall` import).
- CLI flag/exit-code contract is load-bearing — 130 is additive, nothing existing changes.
- Verification gate (run before "done"): `go build ./...`, `go vet ./...`, `go test -race ./...`, `golangci-lint run ./...` (or `bin/lint.sh --go`).

---

## File Structure

- `internal/tui/plan.go` — MODIFY: add `ErrAborted` sentinel, `abortKeyMap()`, wire `.WithKeyMap` into `configureWizard`.
- `internal/tui/tui_test.go` — MODIFY: add `TestAbortKeyMap`, `TestErrAbortedAliasesHuh`.
- `cmd/dotf/abort.go` — CREATE: `abortExitCode`, `reportAbort`, `wizardExit`.
- `cmd/dotf/abort_test.go` — CREATE: tests for `reportAbort` and `wizardExit`.
- `cmd/dotf/plan.go` — MODIFY: route `tui.Run` error through `wizardExit`; import `errors`.
- `cmd/dotf/apply.go` — MODIFY: route `tui.Run` error through `wizardExit`; update exit-code doc comment.
- `cmd/dotf/sysconfig.go` — MODIFY: `confirmDecision` enum, interrupt-aware `confirmApply` + pure `confirmApplyCtx`; switch at the call site.
- `cmd/dotf/sysconfig_test.go` — MODIFY: add `TestConfirmApplyCtx`.
- `CLAUDE.md` — MODIFY: one-line note that interactive steps abort with Ctrl+C/Esc → exit 130.

---

### Task 1: Wizard Esc keymap + `tui.ErrAborted` sentinel

**Files:**
- Modify: `internal/tui/plan.go`
- Test: `internal/tui/tui_test.go`

**Interfaces:**
- Produces: `tui.ErrAborted` (`error`, == `huh.ErrUserAborted`); `abortKeyMap() *huh.KeyMap` (unexported, package-internal).

- [ ] **Step 1: Write the failing tests**

Append to `internal/tui/tui_test.go` (add `"errors"` and `"slices"` to its import block, and `"charm.land/bubbles/v2/key"` is NOT needed here — we read via `Keys()`):

```go
func TestAbortKeyMap(t *testing.T) {
	got := abortKeyMap().Quit.Keys()
	for _, want := range []string{"ctrl+c", "esc"} {
		if !slices.Contains(got, want) {
			t.Errorf("abortKeyMap Quit keys = %v, want to contain %q", got, want)
		}
	}
}

func TestErrAbortedAliasesHuh(t *testing.T) {
	if !errors.Is(ErrAborted, huh.ErrUserAborted) {
		t.Errorf("tui.ErrAborted must match huh.ErrUserAborted so callers can classify a wizard cancellation")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/ -run 'TestAbortKeyMap|TestErrAbortedAliasesHuh' -v`
Expected: FAIL — `undefined: abortKeyMap` / `undefined: ErrAborted`.

- [ ] **Step 3: Implement in `internal/tui/plan.go`**

Add `"charm.land/bubbles/v2/key"` to the import block. Add the sentinel just below the `Selection` type:

```go
// ErrAborted is returned by Run when the user cancels the wizard with Ctrl+C or
// Esc. It aliases huh's sentinel so callers in cmd/dotf can classify a
// cancellation via errors.Is(err, tui.ErrAborted) without importing huh directly.
var ErrAborted = huh.ErrUserAborted
```

Add the keymap builder (near `configureWizard`):

```go
// abortKeyMap returns huh's default keybindings with the form-level Quit binding
// extended to also fire on Esc, so the user can abort the wizard from any step
// with either Ctrl+C or Esc. huh binds Quit to ctrl+c only by default; the form
// checks Quit before delegating the key to the active field, and the wizard's
// Select/MultiSelect fields never enable filtering, so Esc has no competing
// binding to lose.
func abortKeyMap() *huh.KeyMap {
	km := huh.NewDefaultKeyMap()
	km.Quit = key.NewBinding(
		key.WithKeys("ctrl+c", "esc"),
		key.WithHelp("esc", "quit"),
	)
	return km
}
```

Wire it into `configureWizard`:

```go
func configureWizard(form *huh.Form, accessible bool) *huh.Form {
	return form.WithAccessible(accessible).WithOutput(os.Stderr).WithKeyMap(abortKeyMap())
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/ -run 'TestAbortKeyMap|TestErrAbortedAliasesHuh' -v`
Expected: PASS.

- [ ] **Step 5: Tidy modules (promote `key` to direct) and build**

Run: `go mod tidy && go build ./...`
Expected: `go.mod` change limited to the `charm.land/bubbles/v2` line losing its `// indirect` comment; build OK.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/plan.go internal/tui/tui_test.go go.mod go.sum
git commit -m "feat(tui): abort the wizard on Esc and export the abort sentinel"
```

---

### Task 2: Shared abort vocabulary (`cmd/dotf/abort.go`)

**Files:**
- Create: `cmd/dotf/abort.go`
- Test: `cmd/dotf/abort_test.go`

**Interfaces:**
- Consumes: `tui.ErrAborted` (Task 1); existing `reportErr(io.Writer, error) int`, `progName`.
- Produces: `abortExitCode` (`int` = 130); `reportAbort(stderr io.Writer) int`; `wizardExit(stderr io.Writer, err error) int`.

- [ ] **Step 1: Write the failing tests**

Create `cmd/dotf/abort_test.go`:

```go
package main

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/yandy-r/dotfiles/internal/tui"
)

func TestReportAbort(t *testing.T) {
	var stderr bytes.Buffer
	code := reportAbort(&stderr)

	if code != 130 {
		t.Errorf("reportAbort code = %d, want 130", code)
	}
	if got, want := stderr.String(), "dotf: aborted\n"; got != want {
		t.Errorf("reportAbort stderr = %q, want %q", got, want)
	}
}

func TestWizardExit(t *testing.T) {
	t.Run("abort maps to 130 and clean line", func(t *testing.T) {
		var stderr bytes.Buffer
		code := wizardExit(&stderr, tui.ErrAborted)
		if code != 130 {
			t.Errorf("code = %d, want 130", code)
		}
		if got, want := stderr.String(), "dotf: aborted\n"; got != want {
			t.Errorf("stderr = %q, want %q", got, want)
		}
	})

	t.Run("wrapped abort still classified", func(t *testing.T) {
		var stderr bytes.Buffer
		code := wizardExit(&stderr, fmt.Errorf("run wizard: %w", tui.ErrAborted))
		if code != 130 {
			t.Errorf("code = %d, want 130", code)
		}
	})

	t.Run("real error maps to 1 with diagnostic", func(t *testing.T) {
		var stderr bytes.Buffer
		code := wizardExit(&stderr, errors.New("boom"))
		if code != 1 {
			t.Errorf("code = %d, want 1", code)
		}
		if got, want := stderr.String(), "dotf: boom\n"; got != want {
			t.Errorf("stderr = %q, want %q", got, want)
		}
	})
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/dotf/ -run 'TestReportAbort|TestWizardExit' -v`
Expected: FAIL — `undefined: reportAbort` / `undefined: wizardExit`.

- [ ] **Step 3: Implement `cmd/dotf/abort.go`**

```go
package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/yandy-r/dotfiles/internal/tui"
)

// abortExitCode is the process exit code for a user-initiated abort of an
// interactive step (Ctrl+C or Esc). It follows the Unix convention of
// 128 + SIGINT(2) = 130, kept deliberately distinct from the repo's 0/1/2/3
// result codes so a cancellation is never mistaken for a soft failure (1), a
// usage error (2), or a hard precondition abort (3).
const abortExitCode = 130

// reportAbort writes a single clean line to stderr and returns abortExitCode.
// Unlike reportErr it wraps no error: a user cancellation is not a failure, so
// it gets no "progName: <err>" diagnostic and no stack.
func reportAbort(stderr io.Writer) int {
	_, _ = fmt.Fprintf(stderr, "%s: aborted\n", progName)
	return abortExitCode
}

// wizardExit maps a tui.Run error to an exit code: a user cancellation
// (Ctrl+C/Esc, surfaced as tui.ErrAborted) becomes a clean abort (130), while
// any other form failure keeps the standard reportErr diagnostic (1). It is the
// single decision point shared by the plan and apply guided paths.
func wizardExit(stderr io.Writer, err error) int {
	if errors.Is(err, tui.ErrAborted) {
		return reportAbort(stderr)
	}
	return reportErr(stderr, err)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/dotf/ -run 'TestReportAbort|TestWizardExit' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/dotf/abort.go cmd/dotf/abort_test.go
git commit -m "feat(cli): add shared abort exit code and wizard-error classifier"
```

---

### Task 3: Route `plan` and `apply` wizard errors through `wizardExit`

**Files:**
- Modify: `cmd/dotf/plan.go` (the `tui.Run` error branch), `cmd/dotf/apply.go` (the `tui.Run` error branch + exit-code doc).

**Interfaces:**
- Consumes: `wizardExit` (Task 2), `tui.ErrAborted` (Task 1).

- [ ] **Step 1: Modify `cmd/dotf/plan.go`**

Add `"errors"` to the import block. Replace the current branch:

```go
	sel, err := tui.Run(m, info, accessible)
	if err != nil {
		return reportErr(stderr, err)
	}
```

with:

```go
	sel, err := tui.Run(m, info, accessible)
	if err != nil {
		return wizardExit(stderr, err)
	}
```

(`errors` may end up unused in plan.go — if `go build` reports it unused, drop the import. `wizardExit` owns the `errors.Is`.)

- [ ] **Step 2: Modify `cmd/dotf/apply.go`**

Replace:

```go
		sel, serr := tui.Run(m, info, accessible)
		if serr != nil {
			return reportErr(stderr, serr)
		}
```

with:

```go
		sel, serr := tui.Run(m, info, accessible)
		if serr != nil {
			return wizardExit(stderr, serr)
		}
```

Update the `runApply` doc comment's exit-code line to record 130:

```
// Exit codes: 0 = converged/all-applied; 1 = completed-with-failures (soft);
// 2 = usage error (flag parse / invalid --homebrew or firewall value); 3 = hard
// abort (manifest/home/repo resolve, sudo prime, or non-TTY refusal); 130 = user
// aborted the guided wizard (Ctrl+C/Esc). Code 3 is an apply-only additive
// extension over the sibling 0/1/2 convention (via the local reportFatal), so CI
// can distinguish a hard abort from a soft failure.
```

- [ ] **Step 3: Build and run the full package test + vet**

Run: `go build ./... && go vet ./... && go test ./cmd/dotf/ ./internal/tui/`
Expected: builds clean (no unused-import error — remove `errors` from plan.go if flagged); tests PASS.

- [ ] **Step 4: Commit**

```bash
git add cmd/dotf/plan.go cmd/dotf/apply.go
git commit -m "feat(cli): exit 130 with a clean message when the wizard is aborted"
```

---

### Task 4: Interrupt-aware `sysconfig` confirm

**Files:**
- Modify: `cmd/dotf/sysconfig.go`
- Test: `cmd/dotf/sysconfig_test.go`

**Interfaces:**
- Consumes: `reportAbort` (Task 2), existing `progName`, `stdoutIsTTY`.
- Produces: `confirmDecision` (`confirmNo|confirmYes|confirmAborted`); `confirmApply(stdout io.Writer, stdin io.Reader) confirmDecision`; `confirmApplyCtx(ctx context.Context, stdout io.Writer, stdin io.Reader) confirmDecision`.

- [ ] **Step 1: Write the failing tests**

Append to `cmd/dotf/sysconfig_test.go` (ensure imports include `"context"`, `"io"`, `"strings"`, `"testing"`):

```go
func TestConfirmApplyCtx(t *testing.T) {
	t.Run("cancelled context aborts without consuming stdin", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		// A blocking reader that never yields — proves the abort path does not
		// depend on stdin producing anything.
		pr, pw := io.Pipe()
		defer pw.Close()

		var stdout strings.Builder
		if got := confirmApplyCtx(ctx, &stdout, pr); got != confirmAborted {
			t.Errorf("confirmApplyCtx(cancelled) = %v, want confirmAborted", got)
		}
	})

	cases := []struct {
		name string
		in   string
		want confirmDecision
	}{
		{"yes", "y\n", confirmYes},
		{"yes word", "yes\n", confirmYes},
		{"no", "n\n", confirmNo},
		{"empty defaults no", "\n", confirmNo},
		{"eof defaults no", "", confirmNo},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout strings.Builder
			got := confirmApplyCtx(context.Background(), &stdout, strings.NewReader(tc.in))
			if got != tc.want {
				t.Errorf("confirmApplyCtx(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/dotf/ -run TestConfirmApplyCtx -v`
Expected: FAIL — `undefined: confirmApplyCtx` / `undefined: confirmAborted`.

- [ ] **Step 3: Implement in `cmd/dotf/sysconfig.go`**

Add `"os/signal"` to the import block (`bufio`, `context`, `fmt`, `io`, `os`, `strings` are already imported). Replace the existing `confirmApply` function with the enum + wrapper + pure core:

```go
// confirmDecision is the outcome of the interactive sysconfig apply prompt.
type confirmDecision int

const (
	confirmNo confirmDecision = iota // declined ("n", empty, EOF, or read error)
	confirmYes                       // explicit "y"/"yes"
	confirmAborted                   // Ctrl+C while the prompt was waiting
)

// confirmApply prompts for the sysconfig apply and reads one line from stdin.
// SIGINT handling is scoped to this read: signal.NotifyContext is installed
// before the prompt and stopped (restoring Go's default SIGINT behavior) the
// moment it returns, so a Ctrl+C during any later execution phase keeps its
// normal terminate semantics. Only os.Interrupt is registered.
func confirmApply(stdout io.Writer, stdin io.Reader) confirmDecision {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return confirmApplyCtx(ctx, stdout, stdin)
}

// confirmApplyCtx is the pure, context-injected core of confirmApply. It prompts
// on stdout, reads a line from stdin in a goroutine, and returns confirmAborted
// as soon as ctx is done (Ctrl+C), confirmYes for an explicit yes, or confirmNo
// for any other answer, EOF, or read error. On abort the read goroutine is left
// blocked on stdin; the process is exiting, so that is harmless.
func confirmApplyCtx(ctx context.Context, stdout io.Writer, stdin io.Reader) confirmDecision {
	_, _ = fmt.Fprint(stdout, "apply system configuration? [y/N] ")

	type readResult struct {
		line string
		err  error
	}
	resultCh := make(chan readResult, 1)
	go func() {
		line, err := bufio.NewReader(stdin).ReadString('\n')
		resultCh <- readResult{line: line, err: err}
	}()

	select {
	case <-ctx.Done():
		// Break the line so the caller's "dotf: aborted" starts fresh, not
		// appended to the dangling prompt.
		_, _ = fmt.Fprintln(stdout)
		return confirmAborted
	case res := <-resultCh:
		if res.err != nil && res.line == "" {
			return confirmNo
		}
		switch strings.TrimSpace(strings.ToLower(res.line)) {
		case "y", "yes":
			return confirmYes
		default:
			return confirmNo
		}
	}
}
```

Update the call site inside `runSysconfig` — replace:

```go
		if !confirmApply(stdout, os.Stdin) {
			_, _ = fmt.Fprintf(stderr, "%s: sysconfig not confirmed\n", progName)
			return 1
		}
```

with:

```go
		switch confirmApply(stdout, os.Stdin) {
		case confirmAborted:
			return reportAbort(stderr)
		case confirmNo:
			_, _ = fmt.Fprintf(stderr, "%s: sysconfig not confirmed\n", progName)
			return 1
		case confirmYes:
			// proceed to apply
		}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/dotf/ -run TestConfirmApplyCtx -v`
Expected: PASS.

- [ ] **Step 5: Full package test + vet**

Run: `go build ./... && go vet ./... && go test ./cmd/dotf/`
Expected: all PASS (existing `TestRunSysconfig_*` unaffected — none call `confirmApply` directly).

- [ ] **Step 6: Commit**

```bash
git add cmd/dotf/sysconfig.go cmd/dotf/sysconfig_test.go
git commit -m "feat(cli): abort the sysconfig confirm on Ctrl+C with exit 130"
```

---

### Task 5: Docs + full verification gate

**Files:**
- Modify: `CLAUDE.md` (Manual Post-Setup / Troubleshooting area or the `dotf` subcommand description — wherever interactive behavior is documented).

- [ ] **Step 1: Add a one-line note to `CLAUDE.md`**

In the section describing `dotf apply`/`plan` interactive behavior, add:

```markdown
- Interactive steps (the `plan`/`apply` guided wizard and the `sysconfig` confirm) abort cleanly on **Ctrl+C**, and the wizard also on **Esc**, printing `dotf: aborted` and exiting **130**. Answering "No" at a confirm is a decline (exit 1), not an abort.
```

- [ ] **Step 2: Run the full verification gate**

Run:
```bash
go build ./... && go vet ./... && go test -race ./... && bin/lint.sh --go
```
Expected: build OK; vet clean; all tests PASS under `-race`; linters clean.

- [ ] **Step 3: Manual smoke checklist (TTY-only, document results in the PR)**

These require a real terminal and are not automated:
- `dotf plan` → press **Esc** at the OS step; repeat pressing **Ctrl+C** at the intents/packages/confirm steps → each prints `dotf: aborted`, `echo $?` = 130.
- `dotf apply` (guided, no `--headless`) → **Esc**/**Ctrl+C** at any step → `dotf: aborted`, exit 130.
- `dotf sysconfig` with a pending change → **Ctrl+C** at the `[y/N]` prompt → `dotf: aborted`, exit 130; run again and answer **n** → `dotf: sysconfig not confirmed`, exit 1; confirm a Ctrl+C during the *execution* phase still terminates (default behavior preserved).

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: note Ctrl+C/Esc graceful abort (exit 130) for interactive dotf"
```

---

## Self-Review

- **Spec coverage:** Esc binding (Task 1) ✓; ErrAborted sentinel (Task 1) ✓; shared 130/reportAbort (Task 2) ✓; plan+apply mapping (Task 3) ✓; sysconfig ctx-scoped confirm (Task 4) ✓; execution-phase default-SIGINT non-goal preserved by scoping `stop()` (Task 4) ✓; docs (Task 5) ✓.
- **Placeholder scan:** none — every step has concrete code/commands.
- **Type consistency:** `tui.ErrAborted`, `wizardExit(io.Writer, error) int`, `reportAbort(io.Writer) int`, `confirmDecision` + `confirmApply`/`confirmApplyCtx` names used identically across tasks.
