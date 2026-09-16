# Fix Report: pr-38-review

**Source**: `docs/prps/reviews/pr-38-review.md`
**Applied**: 2026-08-29T14:06:01-04:00
**Updated**: 2026-08-29T15:32:46-04:00
**Mode**: Parallel sub-agents (3 severity batches, maximum width 1)
**Severity threshold**: LOW

## Summary

- **Total findings in source**: 8
- **Already processed before this run**:
  - Fixed: 7
  - Failed: 0
- **Eligible this run**: 1
- **Applied this run**:
  - Fixed: 1
  - Failed: 0
- **Skipped this run**:
  - Below severity threshold: 0
  - No suggested fix: 0
  - Missing file: 0

## Fixes Applied

| ID | Severity | File | Line | Status | Notes |
| --- | --- | --- | --- | --- | --- |
| F001 | HIGH | `README.md` | 314 | Fixed | Uses the valid headless full-apply command and matches the design flow. |
| F002 | MEDIUM | `docs/superpowers/plans/2026-08-29-herdr-provisioning.md` | 545 | Fixed | Markdownlint covers all three feature docs; Prettier gates only the new spec and plan. |
| F003 | LOW | `internal/symlink/table.go` | 21 | Fixed | Facet/YAML relationship and divergence count now match the actual table. |
| F004 | CRITICAL | `README.md` | 314 | Fixed | Documents package install plus backup-safe linking and adds a Herdr-specific backup regression test. |
| F005 | MEDIUM | `docs/superpowers/plans/2026-08-29-herdr-provisioning.md` | 425 | Fixed | Uses applicability-aware preview evidence and qualifies guided-plan behavior. |
| F006 | MEDIUM | `docs/superpowers/plans/2026-08-29-herdr-provisioning.md` | 440 | Fixed | Uses an explicit product-doc list and deterministic YAML-compatible range diff check. |
| F007 | LOW | `internal/symlink/table.go` | 101 | Fixed | Distinguishes the synchronized migration from the two actual divergences. |
| F008 | MEDIUM | `docs/superpowers/plans/2026-08-29-herdr-provisioning.md` | 439 | Fixed | Runs deterministic YAML-compatible diff hygiene over the explicit feature base through `HEAD`. |

## Files Changed

- `README.md` (Fixed F001, F004)
- `docs/superpowers/specs/2026-08-29-herdr-provisioning-design.md` (Fixed F001, F002, F004-F006, F008)
- `docs/superpowers/plans/2026-08-29-herdr-provisioning.md` (Fixed F002, F004-F006, F008)
- `internal/symlink/apply_test.go` (Fixed F004)
- `internal/symlink/table.go` (Fixed F003, F007)
- `docs/prps/reviews/pr-38-review.md` (updated F001-F007 statuses to Fixed)

## Failed Fixes

None.

## Validation Results

| Check | Result |
| --- | --- |
| Go vet (`go vet ./...`) | Pass |
| Tests (`go test ./...`) | Pass |
| Feature Markdown lint | Pass |
| Spec/plan Prettier check | Pass |
| Focused symlink tests | Pass |
| Diff hygiene | Pass |

## Next Steps

- Re-run the PR review to verify the fixes and detect regressions.
- Monitor PR #38 CI to green before squash merge.
