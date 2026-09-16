# PR Review #38 — feat(provisioning): install Herdr with tmux-compatible config

**Reviewed**: 2026-08-29T15:41:39-04:00
**Mode**: PR
**Author**: yandy-r
**Branch**: feat/herdr-provisioning → main
**Decision**: APPROVE

## Summary

Production behavior, supply-chain pins, platform gating, config validity, native/rollback backup safety, and all eight review findings are verified Fixed. The final deterministic whitespace check runs over the feature base through `HEAD` and passes.

## Findings

### CRITICAL

- **[F004]** `README.md:314` — The documented `dotf apply --headless --intent dev` flow enables native symlink backups only when `--hyprland` is also set. On an ordinary host it can remove a real `~/.config/herdr/config.toml` without preserving its bytes before linking the repository file.
  - **Status**: Fixed
  - **Category**: Security
  - **Suggested fix**: Document the safe two-step flow `dotf install --intent dev` followed by `dotf link --backup`, update the spec/plan data flow to match, and add a focused integration test proving an existing Herdr config is backed up before the link replaces it.

### HIGH

- **[F001]** `README.md:314` — `dotf apply --intent dev` does not reliably select the `dev` intent: interactive apply enters the guided TUI, while a non-TTY invocation without `--headless` aborts. The design repeats the invalid flow, so following the shipped instructions may not install Herdr.
  - **Status**: Fixed
  - **Category**: Correctness
  - **Suggested fix**: Document `dotf apply --headless --intent dev` for a full intent-driven apply (or `dotf install --intent dev` for packages only), and update the design data-flow sentence to use the same valid flow.

### MEDIUM

- **[F002]** `docs/superpowers/plans/2026-08-29-herdr-provisioning.md:545` — The plan says every verification command exits 0, but its exact three-file Prettier command exits 1 on the acknowledged pre-existing README formatting baseline. This makes the completion contract internally contradictory.
  - **Status**: Fixed
  - **Category**: Completeness
  - **Suggested fix**: Require markdownlint for all three feature Markdown files, require Prettier only for the newly authored spec and plan, and state that the unchanged README Prettier baseline is recorded but is not an all-green feature gate.

- **[F005]** `docs/superpowers/plans/2026-08-29-herdr-provisioning.md:425` — The plan still contains the old unsafe apply snippet and asserts that `dotf install --dry-run` or guided `dotf plan` selects exactly one provider, but those previews intentionally report the raw cross-platform provider union before applicability.
  - **Status**: Fixed
  - **Category**: Correctness
  - **Suggested fix**: Update the plan snippet to the safe install/link commands, use `dotf apply --headless --dry-run --intent dev` (or applicability-aware status) for platform-selection evidence, and qualify the design's guided-plan claim so it does not promise an applicability-filtered union.

- **[F006]** `docs/superpowers/plans/2026-08-29-herdr-provisioning.md:440` — The exact Markdown range assertion expects only three files even though committed review artifacts are now part of the PR, and plain `git diff --check` inherits a user-global `indent-with-non-tab` rule that incorrectly rejects repository-compliant YAML space indentation.
  - **Status**: Fixed
  - **Category**: Completeness
  - **Suggested fix**: Scope the product Markdown gate explicitly to README/spec/plan independently of review artifacts, use a deterministic `git -c core.whitespace=blank-at-eol,blank-at-eof,space-before-tab diff --check` command, and rerun the exact documented block at the current PR head.

- **[F008]** `docs/superpowers/plans/2026-08-29-herdr-provisioning.md:439` — The documented deterministic diff check has no base/head range, so it checks only uncommitted changes and becomes a no-op on a clean PR head. The design and full-gate block repeat the same evidence gap.
  - **Status**: Fixed
  - **Category**: Completeness
  - **Suggested fix**: Run the YAML-compatible diff check against the feature merge-base through `HEAD` in every plan/spec occurrence, and record the fresh range-scoped result.

### LOW

- **[F003]** `internal/symlink/table.go:21` — Facet documentation claims a one-to-one YAML-gate mapping even though `FacetUnix` is a native composite, and the divergence comment says “two” while listing three.
  - **Status**: Fixed
  - **Category**: Pattern Compliance
  - **Suggested fix**: Clarify that facets mirror logical platform gates rather than mapping one-to-one to YAML groups, and correct the documented divergence count.

- **[F007]** `internal/symlink/table.go:101` — The comment introduces all three numbered items as divergences even though item 1 explicitly says the YAML and native table agree.
  - **Status**: Fixed
  - **Category**: Pattern Compliance
  - **Suggested fix**: Rename the section to distinguish the synchronized migration from the two actual divergences, without changing behavior.

## Validation Results

| Check | Result |
| --- | --- |
| Go vet | Pass |
| Lint (`npm run lint`) | Pass — golangci-lint 0 issues; ShellCheck clean |
| Tests (`go test ./...`) | Pass |
| Build (`GOFLAGS=-buildvcs=false go build ./...`) | Pass |
| Herdr config (`herdr config check`) | Pass — `config: ok` |
| Ansible syntax | Pass |
| Feature Markdown lint | Pass — all three files |
| Deterministic range diff hygiene | Pass over `783f91a2..HEAD` |

## Files Reviewed

- `README.md` (Modified)
- `docs/superpowers/plans/2026-08-29-herdr-provisioning.md` (Added)
- `docs/superpowers/specs/2026-08-29-herdr-provisioning-design.md` (Added)
- `herdr/config.toml` (Added)
- `internal/manifest/manifest_test.go` (Modified)
- `internal/symlink/symlink_test.go` (Modified)
- `internal/symlink/table.go` (Modified)
- `internal/symlink/table_test.go` (Modified)
- `manifest/packages.toml` (Modified)
- `playbooks/symlinks.yml` (Modified)
- `playbooks/tasks/backup.yml` (Modified)
- `playbooks/vars/symlinks.yml` (Modified)
