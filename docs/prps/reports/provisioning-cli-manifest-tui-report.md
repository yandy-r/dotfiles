# Implementation Report: Manifest schema + intent-selection TUI (dotf Phase 2)

## Summary

Implemented the declarative TOML manifest — the single source of truth for the
`dotf` rewrite — as a dependency-free typed schema in `internal/manifest` (parse,
strict-validate, `go:embed` default, `--manifest`/`$DOTF_MANIFEST` override), and
rendered it through a `charm.land/huh/v2` multi-step TUI (OS → pre-checked intents →
editable package plan → confirm), surfaced via a new `dotf plan` subcommand with a
machine-greppable dry-run preview. Covers Forgejo tracker **#17** = issue **#5**
(manifest) + issue **#6** (TUI). The manifest reserves a data-only `[[symlink]]`
table (ported from `playbooks/vars/symlinks.yml`) so Phase 4 reuses one parser and
one embed.

Executed in **parallel sub-agent** mode (Path B) across 5 dependency-ordered batches,
`--no-worktree` (direct on `feat/provisioning-cli-manifest-tui`).

## Assessment vs Reality

| Metric        | Predicted (Plan)         | Actual                                              |
| ------------- | ------------------------ | -------------------------------------------------- |
| Complexity    | Large                    | Large — as predicted                               |
| Confidence    | 8/10                     | Realized; only deviation was the huh v2 module path |
| Files Changed | 13 (11 create, 2 update) | 16 create + 3 update (testdata split into 10 files; go.sum added) |

## Tasks Completed

| #   | Task                                   | Status        | Notes                                                |
| --- | -------------------------------------- | ------------- | ---------------------------------------------------- |
| 1.1 | Frozen manifest schema types           | ✅ Complete   | `internal/manifest/types.go`, dependency-free        |
| 1.2 | Add TOML + huh dependencies            | ✅ Complete   | Deviated — huh v2 module path is `charm.land/huh/v2` |
| 1.3 | Embedded default manifest              | ✅ Complete   | `embed.go` + `manifest/packages.toml` (7 backends)   |
| 2.1 | Manifest loader + strict validation    | ✅ Complete   | `load.go`, `validate.go`, tests + 10 fixtures        |
| 2.2 | TUI option mappers                     | ✅ Complete   | `internal/tui/options.go`, pure/TTY-free             |
| 3.1 | huh multi-step form runner             | ✅ Complete   | `internal/tui/plan.go`, 4-group wizard               |
| 4.1 | `dotf plan` subcommand                 | ✅ Complete   | `cmd/dotf/plan.go`, key=value dry-run preview        |
| 5.1 | Wire `plan` into main + dispatch test  | ✅ Complete   | `main.go`/`main_test.go` + `go mod tidy`             |

## Validation Results

| Level           | Status  | Notes                                                            |
| --------------- | ------- | --------------------------------------------------------------- |
| Static Analysis | ✅ Pass | `go build`, `go vet`, `gofmt -l` clean, `golangci-lint` 0 issues |
| Unit Tests      | ✅ Pass | 85 test cases, `-race` clean                                    |
| Build           | ✅ Pass | `go build ./...`                                                 |
| Integration     | N/A     | CLI — no server component                                       |
| Edge Cases      | ✅ Pass | strict decode, no partial manifest, override-errors, dep isolation `clean` |

Additional gates from the plan:

- **Dependency-isolation**: `go list -deps ./internal/manifest/ | grep -E 'charmbracelet|charm.land|bubbletea'` → `clean` (TUI dep does not leak into `internal/manifest`).
- **`go mod verify`**: all modules verified.

## Files Changed

| File                                       | Action  | Lines |
| ------------------------------------------ | ------- | ----- |
| `internal/manifest/types.go`               | CREATE  | 141   |
| `internal/manifest/load.go`                | CREATE  | 91    |
| `internal/manifest/validate.go`            | CREATE  | 145   |
| `internal/manifest/manifest_test.go`       | CREATE  | 206   |
| `internal/manifest/testdata/valid.toml`    | CREATE  | ~65   |
| `internal/manifest/testdata/invalid_*.toml`| CREATE  | 7 files |
| `internal/manifest/testdata/resolve_*.toml`| CREATE  | 2 files |
| `internal/tui/options.go`                  | CREATE  | 78    |
| `internal/tui/plan.go`                     | CREATE  | 142   |
| `internal/tui/tui_test.go`                 | CREATE  | 152   |
| `cmd/dotf/plan.go`                         | CREATE  | 134   |
| `embed.go`                                 | CREATE  | 20    |
| `manifest/packages.toml`                   | CREATE  | 118   |
| `go.sum`                                   | CREATE  | (deps)|
| `cmd/dotf/main.go`                         | UPDATE  | +10/-2 |
| `cmd/dotf/main_test.go`                    | UPDATE  | +32   |
| `go.mod`                                   | UPDATE  | +2 direct deps + transitives |

## Deviations from Plan

1. **huh v2 module path moved to `charm.land/huh/v2`** (WHAT). The plan specified
   importing `github.com/charmbracelet/huh`. Upstream renamed the v2+ module to the
   `charm.land` domain; `github.com/charmbracelet/huh@latest` resolves only to v1.0.0,
   and `github.com/charmbracelet/huh/v2` fails to resolve. (WHY) To get the v2 line on
   Bubble Tea v2 as the plan requires, the correct module is `charm.land/huh/v2 v2.0.3`.
   All TUI imports use `charm.land/huh/v2`. Pinned: `pelletier/go-toml/v2 v2.4.2`,
   `charm.land/huh/v2 v2.0.3`.

2. **`testdata` split into 10 fixture files** (WHAT) — one `valid.toml`, seven
   `invalid_*.toml` (one per failure mode), and two `resolve_*.toml` for precedence,
   rather than the plan's shorthand "`valid.toml` + `invalid_*.toml`". (WHY) One fixture
   per failure mode keeps each table-driven case unambiguous.

3. **Preview write hardened for errcheck** (WHAT) — `printPlanPreview` assembles the
   whole preview in memory and emits it with a single checked `io.WriteString`, returning
   an error that `runPlan` surfaces via `reportErr`. (WHY) `runPlan` writes to injected
   `io.Writer` params (for testability), which are not covered by errcheck's
   `os.Stdout`/`os.Stderr` default exclusion — honoring the repo's fail-fast policy
   rather than blank-discarding write errors on the output path.

## Issues Encountered

- **`go mod tidy` timing (B1)** — at batch B1 no package imported the new deps yet, so a
  premature `go mod tidy` would strip them. Task 1.2 was directed to `go get` only and
  defer `tidy` to Task 5.1 (after imports exist). Resolved cleanly; final `go mod tidy`
  left `go.mod` requires unchanged and only added `go.sum` checksums.
- **Transient v1 huh dependency graph** — the initial `huh@latest` pulled a v1 graph that
  was dropped once the correct `charm.land/huh/v2` path was identified. Final `go.mod` is
  clean.
- **Pre-existing lint findings on generated files** — surfaced by the final gate (6
  errcheck in `plan.go`, 1 revive package-comment in `internal/tui/plan.go`, 1 staticcheck
  `SA9009` in `embed.go`); all fixed by the orchestrator (see Deviation 3, package-comment
  reworded to the `Package tui …` form, and the `go:embed` token removed from a
  comment-line start). Final `golangci-lint run ./...` → **0 issues**.

## Tests Written

| Test File                            | Tests | Coverage                                                      |
| ------------------------------------ | ----- | ------------------------------------------------------------ |
| `internal/manifest/manifest_test.go` | 8 fns / 12+ cases | Parse valid, strict unknown-field, all Validate failure modes, Resolve precedence + unreadable-override error, embedded parse |
| `internal/tui/tui_test.go`           | 4 fns / 9 cases | `IntentOptions` default-selection, `PackagesForIntents` dedup/sort/edge cases |
| `cmd/dotf/main_test.go`              | +2 dispatch tests | `plan` manifest-resolution error (exit 1, `dotf:` prefix), bad-flag (exit 2) |

Module total: **85** `go test` run cases, all passing race-clean.

## Manual Validation (pending — requires a real TTY)

The interactive form cannot be driven from a non-TTY harness. The following remain for a
human to confirm in a real terminal (logic is covered by unit tests + non-interactive
dispatch):

- [ ] `go run ./cmd/dotf plan` launches the 4-step huh form (OS → intents → packages → confirm)
- [ ] `default=true` intents in `manifest/packages.toml` render pre-checked
- [ ] Toggling a package updates the confirm-step preview
- [ ] The confirm step prints a dry-run plan and applies nothing
- [ ] `DOTF_MANIFEST=/path go run ./cmd/dotf plan` loads the override

Non-interactive paths verified: unknown flag → exit 2; unreadable `--manifest` → exit 1 with
`dotf:` prefix and **no** embedded fallback; usage lists `plan`.

## Next Steps

- [ ] Manual TTY smoke of the interactive wizard (above)
- [ ] Code review via `/ycc:code-review`
- [ ] Create PR via `/ycc:prp-pr`
