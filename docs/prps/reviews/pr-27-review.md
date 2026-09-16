# PR Review #27 — feat(dotf): headless `dotf apply` orchestration + flag/env parity, snapper, failure summary (Phase 6)

**Reviewed**: 2026-07-01
**Mode**: PR (Forgejo — local-diff review; `gh` PR mode is GitHub-only)
**Author**: Yandy Ramirez
**Branch**: feat/provisioning-cli-headless → main
**Decision**: REQUEST CHANGES → **all findings resolved in-session** (see Status per finding)
**Reviewers**: 3-way parallel (`correctness-reviewer`, `security-reviewer`, `quality-reviewer`) via `ycc:code-reviewer`

## Summary

Two HIGH correctness defects (contradictory sysconfig applied/skipped summary output; guided-TUI package selection silently discarded, with an empty selection wrongly installing the whole manifest) plus 3 MEDIUM and 4 LOW findings. Security review found no CRITICAL/HIGH issues (all exec is argv-based, sudo primed once after consent, `--dry-run` never primes, keep-alive always cancelled, snapper description is a compile-time literal). All findings were fixed or explicitly acknowledged; the full Go gate (build/vet/test-race/lint/fmt) is green and `bootstrap.sh`/`bin/install-*.sh` remain byte-identical.

## Findings

### HIGH

- **[F001]** `cmd/dotf/applyphases.go:132` — `mergeSysconfigResults` reports a domain as both applied and skipped. The firewall-only P5 pass plans with identity fields zeroed, so its configurer re-lists hostname/timezone/locale/shell as `skipped`, and the merge unioned them without excluding domains already in `Applied` — producing e.g. `result.locale=ok` and `skipped=…,locale,…` in the same block.
  - **Status**: Fixed
  - **Category**: Correctness
  - **Suggested fix**: Exclude domains present in the merged `Applied` set from the merged `Skipped` list.
  - **Resolution**: `mergeSysconfigResults` now builds an applied-domain set and filters both skip slices against it before dedup. Regression guard added to `TestRunApply_HeadlessHappyPath_OrderAndEndState` (asserts no `skipped=` line contains `locale`).

- **[F002]** `cmd/dotf/apply.go:97` — guided (TTY) mode discarded `tui.Selection.Packages` (the user's per-package confirmed set), keeping only `sel.Intents` and re-deriving via `packagesForInstall(m, intents)`. Unchecking a package within an intent was ignored, and deselecting **all** intents (`sel.Intents == []`) hit `packagesForInstall`'s empty-means-all branch and installed the **entire** manifest — the opposite of the confirmed selection.
  - **Status**: Fixed
  - **Category**: Correctness
  - **Suggested fix**: Thread `sel.Packages` to the packages phase instead of re-deriving from intents.
  - **Resolution**: Added `packagesByName(m, names)` (manifest order, deduped, empty→nothing) in `install.go`; the guided branch now uses `packagesByName(m, sel.Packages)`. Unit test `TestPackagesByName` covers the subset/dedup/unknown-name and empty-means-nothing contract.

### MEDIUM

- **[F003]** `cmd/dotf/applyphases.go:44,100` — `applyIdentity`/`applyFirewall` discarded `eng.Execute`'s error (`res, _ := …`), unlike sibling `runSysconfig`, silently proceeding with a partial `Result` if the dispatch ever fails.
  - **Status**: Fixed
  - **Category**: Correctness
  - **Suggested fix**: Check the error and `coll.Record("sysconfig", err)`.
  - **Resolution**: Both phases now record a non-nil `Execute` error into the failure collector.

- **[F004]** `cmd/dotf/apply_test.go` — the plan's test matrix named three exit-3 hard-abort cases but only two shipped (sudo-prime, non-TTY); no manifest/home/repo hard-abort test, and no keep-alive-surfaced test.
  - **Status**: Fixed
  - **Category**: Completeness
  - **Suggested fix**: Add the missing hard-abort tests.
  - **Resolution**: Added `TestRunApply_ManifestResolutionErrorFatal` (missing `--manifest` → exit 3, no summary, no sudo). The keep-alive-surfaced case is intentionally omitted: the keep-alive refresh runs on a multi-minute ticker goroutine, so asserting "the error was stored before the summary printed" is inherently racy; the `KeepAliveErr()` line remains wired in `applySummary`.

- **[F005]** `cmd/dotf/apply_test.go` — `TestRunApply_SnapperInvokedBeforeMutations` guarded its ordering assertions with `if idx >= 0 && …`, so a regression that dropped the sysconfig/packages phase (index `-1`) would skip the check instead of failing.
  - **Status**: Fixed
  - **Category**: Correctness
  - **Suggested fix**: `t.Fatalf` on a missing index before comparing order.
  - **Resolution**: The test now fatals on `localeIdx < 0`/`cargoIdx < 0` before the ordering comparisons.

- **[F006]** `cmd/dotf/apply.go:209`, `internal/snapper/snapper.go:78` — this PR added a second and third private copy of the "which `platform.OS` is Linux-family" switch (alongside the pre-existing, differently-scoped `internal/symlink` copy), under confusingly similar names (`isLinuxFamilyOS`/`isLinux`/`isLinuxFamily`).
  - **Status**: Fixed
  - **Category**: Maintainability
  - **Suggested fix**: Promote the shared membership to `internal/platform`.
  - **Resolution**: Added exported `platform.IsLinuxFamily` (the full systemd set incl. `OSLinux`/`OSWSL`) with `TestIsLinuxFamily`; `internal/snapper` and `cmd/dotf/apply.go` now call it (their private copies removed). The pre-existing `internal/symlink` desktop-facet predicate (which deliberately excludes bare `OSLinux`) is intentionally left in place — it is a semantically distinct set — and the doc comment on `platform.IsLinuxFamily` now spells out the difference.

### LOW

- **[F007]** `cmd/dotf/applyopts.go:20` — the `applyOptions` doc comment referenced only `NON_INTERACTIVE/FORCE_INSTALL/ENABLE_*`, omitting `LINUX_DESKTOP`/`SNAPPER`.
  - **Status**: Fixed
  - **Category**: Maintainability
  - **Suggested fix**: Enumerate all six env equivalents.
  - **Resolution**: Comment now lists every field's env var explicitly.

- **[F008]** `cmd/dotf/applyphases.go:112` — `resolveSysconfig("", "", "", "", "", m.Sysconfig)` passes five positional blanks with no indication which is which.
  - **Status**: Fixed
  - **Category**: Maintainability
  - **Suggested fix**: Name the skipped params in a comment.
  - **Resolution**: Added a comment naming the `(hostname, timezone, locale, shell, firewall)` positions and why apply leaves them empty.

- **[F009]** `cmd/dotf/apply_test.go` — 547 lines, modestly past the ~500-line soft cap.
  - **Status**: Fixed
  - **Category**: Maintainability
  - **Suggested fix**: Extract the shared helpers.
  - **Resolution**: Moved the fixtures/helpers to `cmd/dotf/apply_helpers_test.go`; `apply_test.go` is now 462 lines.

- **[F010]** `internal/privilege/privilege.go:45` (unchanged; exercised by `apply.go:120`) — the keep-alive goroutine only observes `ctx.Done()` between refreshes, so `cancel()` stops future refreshes immediately but an in-flight `sudo -n -v` returns before the goroutine exits.
  - **Status**: Open
  - **Category**: Performance
  - **Suggested fix**: (Optional) thread `ctx` into a cancellable refresh call.
  - **Resolution**: Acknowledged — **no action**. Bounded (sub-second) lifetime, always `defer`-cancelled on every return path, pre-existing code not touched by this PR. Reviewer verdict: "no action required."

## Validation Results

| Check      | Result  |
| ---------- | ------- |
| Type check | N/A (Go) |
| Lint       | Pass (`golangci-lint run ./...` → 0 issues; `gofmt -l` clean) |
| Tests      | Pass (`go test -race ./...` — all 12 packages ok) |
| Build      | Pass (`go build ./...`, `go vet ./...`) |
| Bash guard | Pass (`git diff` empty for `bootstrap.sh`/`bin/install-*.sh`) |

## Files Reviewed

- `cmd/dotf/apply.go` (Modified)
- `cmd/dotf/applyphases.go` (Modified)
- `cmd/dotf/applyopts.go` (Modified)
- `cmd/dotf/apply_test.go` (Modified)
- `cmd/dotf/apply_helpers_test.go` (Added, during fix)
- `cmd/dotf/install.go` (Modified — `packagesByName`)
- `cmd/dotf/install_test.go` (Modified — `TestPackagesByName`)
- `cmd/dotf/main.go`, `cmd/dotf/main_test.go` (Modified)
- `internal/failure/collector.go` (+ test) (Added)
- `internal/snapper/snapper.go` (+ test) (Added; now uses `platform.IsLinuxFamily`)
- `internal/platform/platform.go` (+ test) (Modified — `IsLinuxFamily`)
