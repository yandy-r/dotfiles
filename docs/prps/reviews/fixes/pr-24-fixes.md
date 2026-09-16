# Fix Report: pr-24-review

**Source**: docs/prps/reviews/pr-24-review.md
**Applied**: 2026-07-01T00:00:00Z
**Mode**: Parallel sub-agents (3 batches, max width 3) — applied sequentially in-repo with `--no-worktree`
**Severity threshold**: LOW

## Summary

- **Total findings in source**: 10
- **Already processed before this run**:
  - Fixed: 0
  - Failed: 0
- **Eligible this run**: 10
- **Applied this run**:
  - Fixed: 10
  - Failed: 0
- **Skipped this run**:
  - Below severity threshold: 0
  - No suggested fix: 0
  - Missing file: 0

## Fixes Applied

| ID   | Severity | File                        | Line | Status | Notes                                              |
| ---- | -------- | --------------------------- | ---- | ------ | -------------------------------------------------- |
| F001 | HIGH     | internal/tui/plan.go        | 121  | Fixed  | Added `Selection.Confirmed`; `runPlan` short-circuits when declined |
| F002 | MEDIUM   | cmd/dotf/plan.go            | 79   | Fixed  | Added `cmd/dotf/plan_test.go` for preview helpers  |
| F003 | MEDIUM   | internal/manifest/validate.go | 65 | Fixed  | Added `validateSymlink` with src/dest/`..`/abs checks |
| F004 | MEDIUM   | internal/manifest/validate.go | 94 | Fixed  | Reject packages with empty `Name`                  |
| F005 | MEDIUM   | internal/manifest/validate.go | 104 | Fixed | Require `SHA256` for download backends           |
| F006 | MEDIUM   | internal/tui/options.go     | 1    | Fixed  | Removed duplicate package doc from `plan.go`       |
| F007 | MEDIUM   | internal/tui/options.go     | 56   | Fixed  | Extracted shared `dedupeSorted` helper             |
| F008 | LOW      | cmd/dotf/plan.go            | 66   | Fixed  | TTY check uses injected `*os.File` when available  |
| F009 | LOW      | internal/manifest/validate.go | 112 | Fixed | Restrict `script-url` URLs to `https://`           |
| F010 | LOW      | internal/tui/plan.go        | 39   | Fixed  | Added per-field doc comments on `Selection`        |

## Files Changed

- `cmd/dotf/plan.go` (Fixed F001, F008)
- `cmd/dotf/plan_test.go` (Fixed F002 — added)
- `internal/manifest/validate.go` (Fixed F003, F004, F005, F009)
- `internal/manifest/manifest_test.go` (new invalid fixture cases)
- `internal/manifest/testdata/invalid_*.toml` (4 fixtures — added)
- `internal/tui/plan.go` (Fixed F001, F007, F010)
- `internal/tui/options.go` (Fixed F006, F007)
- `manifest/packages.toml` (added missing `eza` SHA256 for F005)

## Failed Fixes

_None._

## Validation Results

| Check      | Result |
| ---------- | ------ |
| Type check | Pass (`go vet ./...`) |
| Tests      | Pass (`go test -race ./...`) |

## Next Steps

- Re-run `/code-review 24` to verify the remaining open findings and confirm fixes resolved the issues
- Address failed fixes manually using the Blocker + Recommendation notes above
- Run `/git-workflow --commit` to commit the changes when satisfied
