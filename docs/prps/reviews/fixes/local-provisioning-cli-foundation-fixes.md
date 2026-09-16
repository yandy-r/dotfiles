# Review Fix Report — Provisioning CLI Foundation (local)

- **Source review**: `docs/prps/reviews/local-provisioning-cli-foundation-review.md`
- **Branch**: `feat/provisioning-cli-foundation`
- **Date**: 2026-06-30
- **Mode**: `--parallel --no-worktree --severity LOW`
- **Execution**: 5 `ycc:review-fixer` sub-agents in a single parallel batch (no file overlap between agents)

## Summary

| Metric | Count |
| --- | --- |
| Findings at/above `LOW` threshold | 13 (F1–F13) |
| Fixed | 13 |
| Failed | 0 |
| Skipped (below threshold: `[Nit]`) | 2 (F14, F15 → remain `Open`) |

## Validation (post-fix, full repo)

| Check | Result |
| --- | --- |
| `go build ./...` | ✅ pass |
| `go vet ./...` | ✅ pass |
| `go test -race ./...` | ✅ pass (cmd/dotf, internal/exec, internal/platform, internal/version) |
| `golangci-lint run ./...` | ✅ 0 issues |

## Batch layout

Findings were grouped by file so no two parallel agents touched the same file. F2
(new `main_test.go`) was folded into the same agent as F1 because it depends on F1's
`ContinueOnError` refactor.

| Agent | Files | Findings |
| --- | --- | --- |
| 1 | `cmd/dotf/main.go` + new `cmd/dotf/main_test.go` | F1, F7, F2 |
| 2 | `internal/platform/{platform,osrelease,types}.go` | F5, F6, F9, F12, F13 |
| 3 | `internal/platform/platform_test.go` | F3, F8, F10 |
| 4 | `internal/exec/exec.go` | F11 |
| 5 | `.github/workflows/go.yml` | F4 |

## Per-finding outcomes

- **F1 [High] — Fixed**. `run()` now builds a local `flag.NewFlagSet(progName, flag.ContinueOnError)`
  with `fs.SetOutput(io.Discard)` and parses `args` through it. The previously dead
  `if err != nil { … return 2 }` branch is reachable and `run()` is a pure function of `args`.
- **F2 [High] — Fixed**. Added `cmd/dotf/main_test.go` with table/scenario coverage: no-args → exit 1 +
  stderr; unknown subcommand → exit 1 + stderr; `version` → exit 0 + `dotf <version>`; `platform` →
  exit 0 + `os=… arch=… pkg=… container=… distro=…`; `-badflag` → exit 2 (no process exit, proving F1).
- **F3 [Medium] — Fixed**. Added ID_LIKE-fallback cases forcing redhat/arch/suse via unrecognized `ID`.
- **F4 [Medium] — Fixed**. Pinned third-party CI actions to full commit SHAs (resolved via `gh api`):
  - `actions/checkout` → `34e114876b0b11c390a56381ad16ebd13914f8d5` # v4
  - `actions/setup-go` → `40f1582b2485089dde7abd97c1529aa768e1baff` # v5
  - `golangci/golangci-lint-action` → `55c2c1448f86e01eaae002a5a3a9624417608d84` # v6
- **F5 [Medium] — Fixed**. Replaced ~9 fragile exact-line-number shell citations (both `platform.go`
  and `osrelease.go`) with function-name citations (`common/platform.sh:detect_os`, etc.), which also
  moots the `58-72`→`58-73` off-by-one.
- **F6 [Low] — Fixed**. Softened `detectPkgMgr` doc to note `command -v` (resolves functions/aliases)
  vs `LookPath` (PATH executables only).
- **F7 [Low] — Fixed**. Reworded `runPlatform` doc to "machine-greppable analogue of
  `print_platform_info`", dropping the false line-format parity claim.
- **F8 [Low] — Fixed**. Added `MINGW64_NT-10.0` → `OSWindows` and `FreeBSD` → `OSUnknown` cases.
- **F9 [Low] — Fixed**. `parseOSRelease` now checks `scanner.Err()` after the loop and degrades to
  `"", ""` (matching the missing/unreadable-file style) instead of silently returning partial results.
- **F10 [Low] — Fixed**. Added a single-quoted `ID='cachyos'` fixture to `TestDistroName`.
- **F11 [Low] — Fixed**. Added a doc note that `Runner` is intentionally exported as a shared test seam.
- **F12 [Low] — Fixed**. Added a `unameM` helper mirroring `unameS`; `detectArch` now uses it.
- **F13 [Low] — Fixed**. Documented the deliberate underscore in `ArchX86_64` with a comment and a
  `//nolint:revive,stylecheck` directive (verified not flagged as an unused-nolint).

## Skipped (below `LOW` threshold — remain `Open`)

- **F14 [Nit]** — PATH-based `uname` resolution (not exploitable for this unprivileged CLI).
- **F15 [Nit]** — CI/tooling polish (single-OS matrix, minimal linter set, per-call `candidates` slice).

## Follow-ups noted by agents

- **Dependabot/Renovate** for the `github-actions` ecosystem to auto-bump the new SHA pins (F4) —
  deferred as optional scope; no `.github/dependabot.yml` was added.

## Next steps

- Re-run `/ycc:code-review` on the same target to confirm findings resolved.
- `/ycc:git-workflow --commit` to commit the fixes when ready.
