# PR Review #24 — feat(dotf): manifest schema and intent-selection TUI (Phase 2)

**Reviewed**: 2026-07-01T13:27:02Z
**Mode**: PR (Forgejo — `git.example-tailnet.ts.net`, reviewed via `tea`)
**Author**: yandy
**Branch**: feat/provisioning-cli-manifest-tui → main
**Decision**: REQUEST CHANGES
**Review dispatch**: `--parallel` (3 standalone `ycc:code-reviewer` sub-agents: correctness, security, quality) · `--no-worktree`

## Summary

Phase 2 delivers a clean, dependency-disciplined manifest schema, a well-factored
pure/TTY-split TUI, and a `plan` dry-run command; the manifest parser and validator
are sound and thoroughly test-covered, and all validation (build, vet, `test -race`,
golangci-lint) passes. One HIGH logic defect — the interactive "Apply this selection?"
answer is silently discarded — plus a cluster of MEDIUM schema-completeness and
convention gaps warrant changes before merge (several are forward-looking guards that
Phase 6's apply step will depend on).

## Findings

### CRITICAL

_None._

### HIGH

- **[F001]** `internal/tui/plan.go:121` — The `huh.Confirm` "Apply this selection?" answer is bound to a local `confirmed` var (line 121-124) that is never surfaced: `tui.Selection` (line 39-43) has no confirmation field, and `Run` returns the collected OS/intents/packages unconditionally (line 137-141). `cmd/dotf/plan.go:42-51` then prints the preview and returns 0 regardless of the answer, so declining has zero observable effect — discarded user input, and a latent bug that will propagate directly into the Phase 6 apply flow built on this same `Selection`.
  - **Status**: Fixed
  - **Category**: Correctness
  - **Suggested fix**: Add `Confirmed bool` to `tui.Selection`, populate it from the local `confirmed` var, and in `runPlan` short-circuit (distinct exit / "not confirmed" message) when `!sel.Confirmed` instead of always printing the preview. For a dry-run `plan` this may mean dropping the confirm step entirely and reintroducing it in the apply command — either way, do not collect an answer the code ignores.

### MEDIUM

- **[F002]** `cmd/dotf/plan.go:79` — `printPlanPreview`, `groupPackagesByBackend`, and `sortedBackendKeys` are pure, TTY-free functions with zero direct unit-test coverage (no `plan_test.go`), while the sibling pure mappers in `internal/tui/options.go` get full table-driven tests. `cmd/dotf/main_test.go` only exercises the two `plan` error paths, never the successful multi-backend preview shape. (Flagged HIGH by the correctness reviewer and MEDIUM by the quality reviewer; recorded MEDIUM — it is a coverage/completeness gap, not a live bug.)
  - **Status**: Fixed
  - **Category**: Completeness
  - **Suggested fix**: Add `cmd/dotf/plan_test.go` calling these unexported functions directly with a fixture `*manifest.Manifest` and `tui.Selection`, asserting key=value line shape and deterministic backend sort order.
- **[F003]** `internal/manifest/validate.go:65` — `Manifest.Symlinks` entries are never validated (`Validate` has no `validateSymlink`). A `[[symlink]]` whose `dest` contains `..` or is absolute passes silently. Not exploitable in Phase 2 (no symlink materialization exists yet), but it is a path-traversal foothold the moment a future apply step joins `Dest` onto `$HOME`. The manifest is operator-authored, so the trust boundary is low — but fail-fast validation of the reserved data is cheap and matches the schema's own strictness elsewhere.
  - **Status**: Fixed
  - **Category**: Security
  - **Suggested fix**: Add a `validateSymlink` pass that rejects empty `src`/`dest`, absolute `dest`, and any `dest` containing a `..` path element; call it from `Validate` alongside `validatePackage`.
- **[F004]** `internal/manifest/validate.go:94` — `validatePackage` never checks that `Package.Name` is non-empty. A single `[[package]]` with no `name` decodes cleanly and passes `Validate` (only a *second* empty-named package trips the duplicate-name check). Since `Name` is the canonical package identifier and intent references key off it, an unnamed package is a silent schema hole.
  - **Status**: Fixed
  - **Category**: Correctness
  - **Suggested fix**: At the top of `validatePackage`, add `if p.Name == "" { return append(errs, &ValidationError{Msg: "package missing required name"}) }` before the backend switch.
- **[F005]** `internal/manifest/validate.go:104` — `SHA256` is only format-checked when present (line 137); it is never *required*, even for the download backends `script-url` and `github-release`. A future apply step that fetches and executes an artifact would have no integrity guarantee for any package that simply omits the field.
  - **Status**: Fixed
  - **Category**: Security
  - **Suggested fix**: Require a non-empty `SHA256` for `BackendScriptURL` and `BackendGitHubRelease` in the backend switch (in addition to the existing shape check), so the integrity contract is enforced at manifest-load time rather than assumed by the apply phase.
- **[F006]** `internal/tui/options.go:1` — Both `options.go` and `plan.go` carry a full `// Package tui …` doc comment, producing a duplicated/concatenated `go doc ./internal/tui` rendering. Go convention (and this repo's own pattern in `cmd/dotf` and `internal/manifest`) is a single package-doc comment per package.
  - **Status**: Fixed
  - **Category**: Pattern Compliance
  - **Suggested fix**: Keep the package doc on one file (e.g. `options.go`) and replace the `plan.go` header with a plain file comment that does not begin with "Package tui".
- **[F007]** `internal/tui/options.go:56` — `PackagesForIntents` (options.go:56-77) and `dedupePackages` (`internal/tui/plan.go:68-83`) implement the same seen-map dedup + `sort.Strings` almost verbatim, a copy-paste duplication the project's DRY rule explicitly forbids.
  - **Status**: Fixed
  - **Category**: Maintainability
  - **Suggested fix**: Extract a shared `dedupeSorted(items []string) []string` helper and call it from both sites.

### LOW

- **[F008]** `cmd/dotf/plan.go:66` — `runPlan` threads an injected `stdout io.Writer` (and an `exec.Runner`) for testability, but the TTY check at line 40 calls `isTTY(os.Stdout)` against the global rather than the injected writer, so the accessibility decision can't be exercised in isolation.
  - **Status**: Fixed
  - **Category**: Maintainability
  - **Suggested fix**: Accept a stat-capable `*os.File` alongside `stdout`, or document the intentional coupling at the call site.
- **[F009]** `internal/manifest/validate.go:112` — `script-url`'s `URL` is only checked for non-emptiness; its scheme is unrestricted. Combined with F005, a plaintext `http://` download with no checksum would carry zero integrity/transport guarantee once apply is built.
  - **Status**: Fixed
  - **Category**: Security
  - **Suggested fix**: Restrict `URL` to `https://` (parse with `net/url` and reject non-`https` schemes) when validating `BackendScriptURL`.
- **[F010]** `internal/tui/plan.go:39` — `Selection`'s exported fields (`OS`, `Intents`, `Packages`) lack per-field doc comments, unlike every other exported struct added in this PR (`manifest.Manifest/Intent/Package/Symlink` document every field).
  - **Status**: Fixed
  - **Category**: Pattern Compliance
  - **Suggested fix**: Add a one-line doc comment to each exported field.

## Validation Results

| Check      | Result |
| ---------- | ------ |
| Type check | Pass (`go build ./...`) |
| Lint       | Pass (`go vet ./...`, `golangci-lint run ./...` — 0 issues) |
| Tests      | Pass (`go test -race ./...`) |
| Build      | Pass (`go build ./...`) |

## Files Reviewed

- `cmd/dotf/main.go` (Modified)
- `cmd/dotf/main_test.go` (Modified)
- `cmd/dotf/plan.go` (Added)
- `embed.go` (Added)
- `go.mod` (Modified)
- `go.sum` (Added)
- `internal/manifest/load.go` (Added)
- `internal/manifest/types.go` (Added)
- `internal/manifest/validate.go` (Added)
- `internal/manifest/manifest_test.go` (Added)
- `internal/manifest/testdata/*.toml` (Added — 11 fixtures)
- `internal/tui/options.go` (Added)
- `internal/tui/plan.go` (Added)
- `internal/tui/tui_test.go` (Added)
- `manifest/packages.toml` (Added)
- `docs/prps/plans/completed/provisioning-cli-manifest-tui.plan.md` (Renamed)
- `docs/prps/reports/provisioning-cli-manifest-tui-report.md` (Added)
