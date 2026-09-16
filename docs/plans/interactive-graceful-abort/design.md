# Design: Graceful Ctrl+C / Esc Exit for dotf's Interactive Workflow

Status: approved (decisions locked 2026-07-03)
Scope: prompts & wizard only (Approach A)

## Problem

`dotf`'s interactive (non-headless) commands leave the user with no clean way to
back out of a prompt:

- The huh-based **wizard** (`dotf plan`, `dotf apply` guided path) returns
  `huh.ErrUserAborted` on **Ctrl+C**, but callers funnel it through the generic
  `reportErr`, so it prints `dotf: user aborted` and exits 1 — indistinguishable
  from a real form failure. **Esc is unbound** (huh's default `Quit` binding is
  `ctrl+c` only), so pressing Esc does nothing.
- The **`sysconfig` y/N confirm** prompt (`confirmApply`) reads a line with
  `bufio` in cooked mode and has **no interrupt handling**. Ctrl+C hits Go's
  default SIGINT behavior: abrupt termination, no message, deferred cleanup
  skipped.

The user wants Ctrl+C (and Esc where feasible) to produce a *graceful* exit from
any interactive step: one clean message, a consistent exit code, terminal left
usable, distinct from a deliberate "No".

## Decisions (locked)

1. **Abort exit code = `130`** (128 + SIGINT). The Unix convention for Ctrl+C
   termination. Distinct from the repo's `0/1/2/3` contract (no collision), so
   scripts/CI can tell "user cancelled" from soft-fail (1) / usage (2) / hard
   precondition abort (3). A deliberate **"No"** at a confirm is *not* an abort —
   it stays exit 1 ("not confirmed").
2. **Scope = interactive prompt/wizard steps only** (Approach A). Explicit
   non-goals for this change:
   - Interrupting the post-confirmation **execution phases** (package installs,
     `sysconfig` apply, symlink writes) — those keep default SIGINT behavior.
   - Graceful Ctrl+C in huh's **accessible mode** (a blocking cooked read we do
     not own) — documented limitation. Esc is likewise unavailable there.

## Architecture (Approach A: layered & minimal)

One shared notion of "abort" that both interactive surfaces funnel into.

### 1. Shared abort vocabulary (`cmd/dotf`)

- A single constant `abortExitCode = 130`.
- A single helper `reportAbort(stderr io.Writer) int` that writes exactly one
  line — `dotf: aborted` — to stderr and returns `abortExitCode`.
- Both the wizard-error mapping and the confirm-prompt interrupt path call it, so
  the message and code live in exactly one place (DRY).

### 2. Wizard: Esc binding + abort mapping

- **`internal/tui/plan.go`**: build a keymap from `huh.NewDefaultKeyMap()`,
  extend `Quit` to `key.WithKeys("ctrl+c", "esc")`, and apply it with
  `form.WithKeyMap(...)`. The form checks `keymap.Quit` at the form level before
  delegating the key to the active field (huh `form.go` Update), and our
  Select/MultiSelect fields never enable filtering, so `esc`'s only other default
  bindings (`Back`, `SetFilter`, `ClearFilter`) are disabled and nothing is lost.
  Esc now aborts exactly like Ctrl+C, yielding `huh.ErrUserAborted`.
- **`cmd/dotf/plan.go` and `cmd/dotf/apply.go`**: after `tui.Run(...)`, if the
  error `errors.Is(err, huh.ErrUserAborted)`, return `reportAbort(stderr)`
  instead of `reportErr`. Any other form error keeps its current handling
  (`reportErr` in plan, `reportErr` in apply). `apply` maps the abort to `130`
  (an additive extension over its existing `0/1/2/3`); `plan` likewise.

  This requires `internal/tui` to keep exporting/forwarding huh's sentinel so
  callers can match it without importing huh directly. Preferred: re-export
  `var ErrAborted = huh.ErrUserAborted` from `internal/tui`, and have callers
  match `errors.Is(err, tui.ErrAborted)`. Keeps `cmd/dotf` free of a direct huh
  dependency and gives us a stable internal sentinel.

### 3. Confirm prompt: context-aware read

- **`cmd/dotf/sysconfig.go`**: make the confirm read interrupt-aware, with the
  signal registration **scoped to the read itself** (not the whole command).
  Inside the confirm helper: `ctx, stop := signal.NotifyContext(context.Background(),
  os.Interrupt); defer stop()`, run the blocking `bufio` read in a goroutine, and
  `select` on `ctx.Done()` vs the read result.
  - On `ctx.Done()` (Ctrl+C): return an `aborted=true` result → `runSysconfig`
    returns `reportAbort(stderr)` (130). The read goroutine is left blocked on
    stdin; the process is exiting, so this is harmless.
  - On a normal line: parse y/N as today.
  - **Critical:** `stop()` runs the moment the confirm returns, restoring the Go
    runtime's default SIGINT behavior *before* the execution phase. This is
    deliberate: a process-wide `NotifyContext` for all of `runSysconfig` would keep
    `signal.Notify` registered through `engine.Execute`, swallowing Ctrl+C during
    execution and making the (intentionally default-behavior) execution phase
    **un-interruptible** — a regression. Scoping the registration to the prompt
    avoids that entirely.
  - Only `os.Interrupt` (SIGINT) is registered — portable across macOS/Linux with
    no `syscall` import, and it is exactly the Ctrl+C signal in scope.
- The existing keep-alive `context.Background()` calls in `sysconfig.go` stay as
  they are — they are unrelated to interrupt handling and are cancelled via their
  own `cancel()` on the normal return path.
- **`cmd/dotf/apply.go`** needs no `NotifyContext`: its guided confirmation *is*
  the raw-mode huh wizard (Ctrl+C/Esc handled by huh → `tui.ErrAborted`). There is
  no cooked-mode prompt in `apply`, and its execution phases intentionally keep
  default SIGINT behavior.

### Why not a global signal handler (Approach B)?

A process-wide `signal.Notify` + `os.Exit(130)` would cover accessible mode and
execution phases too, but: (a) `os.Exit` in a handler skips deferred cleanup, and
(b) if it ever fired while the raw-mode bubbletea form was active, it would race
the form's terminal-restore and could leave the terminal corrupted. Approach A
sidesteps both by letting huh own Ctrl+C in raw mode and only adding a *scoped*,
non-`os.Exit` interrupt wait for the one cooked-mode read we own.

### Why not full context threading (Approach C)?

Threading `ctx` through every backend/sysconfig/symlink engine to abort in-flight
execution is the most complete answer but a large multi-file surface change,
disproportionate to a goal whose "steps" are the prompts. Deferred as a possible
follow-up.

## Data flow

```
dotf plan / dotf apply (guided, TTY, non-accessible)
  tui.Run → form.Run()
    Ctrl+C or Esc → keymap.Quit → StateAborted → ErrUserAborted (== tui.ErrAborted)
  caller: errors.Is(err, tui.ErrAborted) → reportAbort(stderr) → exit 130

dotf sysconfig (apply path, TTY)
  runSysconfig: ctx = signal.NotifyContext(..., os.Interrupt, SIGTERM); defer stop()
  confirmApply(ctx, stdout, stdin):
    Ctrl+C → ctx.Done() → abort sentinel → reportAbort(stderr) → exit 130
    "y"/"yes" → true (proceed)
    "n"/""/EOF → false → "sysconfig not confirmed" → exit 1 (unchanged)
```

## Error handling

- Abort is *not* an error condition: it prints a single clean line, no
  `progName: <err>` wrapping, no stack.
- A genuine form failure (non-abort huh error) still routes through `reportErr`
  (exit 1) unchanged.
- Declining a confirm ("No"/EOF) stays exit 1 with the existing "not confirmed"
  message — semantically distinct from an abort.

## Testing strategy

- **`internal/tui`**: unit test that the wizard keymap's `Quit` binding includes
  both `ctrl+c` and `esc` (pure, no TTY — assert on the keymap the builder
  produces). Assert `tui.ErrAborted` is identical to `huh.ErrUserAborted`.
- **`cmd/dotf`**: unit test `reportAbort` writes `dotf: aborted\n` and returns
  `130`. Table-test the abort-vs-real-error branch of the plan/apply wizard-result
  handling by injecting each sentinel (refactor the mapping into a pure helper
  that takes an error and returns an exit code + message, so it is testable
  without a TTY).
- **`cmd/dotf` sysconfig**: test the context-aware `confirmApply` with a
  pre-cancelled context (returns the abort sentinel without consuming stdin) and
  with normal y/N/EOF inputs (existing behavior preserved). Keep the read
  goroutine leak-free-enough for tests (cancelled-ctx path returns immediately).
- Full suite: `go build ./...`, `go vet ./...`, `go test -race ./...`,
  `golangci-lint run ./...`.
- Manual smoke (documented, not automated — needs a TTY): `dotf plan`, press Esc
  at each step and Ctrl+C at each step → `dotf: aborted`, exit 130; `dotf apply`
  guided likewise; `dotf sysconfig` (with a pending change) Ctrl+C at the confirm
  → `dotf: aborted`, exit 130; answer "n" → "not confirmed", exit 1.

## Files touched (estimate)

- `internal/tui/plan.go` — keymap with Esc; `ErrAborted` re-export.
- `internal/tui/*_test.go` — keymap + sentinel tests.
- `cmd/dotf/abort.go` (new) — `abortExitCode`, `reportAbort`, pure abort-mapping helper.
- `cmd/dotf/abort_test.go` (new) — helper tests.
- `cmd/dotf/plan.go` — map `tui.ErrAborted` → `reportAbort`.
- `cmd/dotf/apply.go` — map `tui.ErrAborted` → `reportAbort`.
- `cmd/dotf/sysconfig.go` — `signal.NotifyContext`; context-aware `confirmApply`.
- `cmd/dotf/sysconfig_test.go` — context-aware confirm tests.
- Docs: `CLAUDE.md` / help text note that Ctrl+C/Esc abort with exit 130 (if a
  natural spot exists).
```
