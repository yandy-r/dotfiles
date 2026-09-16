# Implementation Report: Install Backend Engine + Privilege Model + Drift Report (Phase 3)

## Summary

Implemented the `dotf` install layer: a 7-backend install engine under `internal/backend/`, shared privilege model under `internal/privilege/` (sudo prime + keep-alive + unix credential drop for AUR), and read-only drift reporting via `dotf status` / `dotf diff`. Extended the `exec.Runner` seam with additive `Run()` for TTY-attached installs. Added manifest `apt_repo` schema and representative `[[package]]` entries per backend in `manifest/packages.toml`.

## Assessment vs Reality

| Metric        | Predicted (Plan) | Actual         |
| ------------- | ---------------- | -------------- |
| Complexity    | Large            | Large          |
| Confidence    | 8/10             | 8/10           |
| Files Changed | ~26 (18C, 8U)    | ~33 Go files (20 new dirs + 13 updated) |

## Tasks Completed

| #   | Task                          | Status          | Notes                                      |
| --- | ----------------------------- | --------------- | ------------------------------------------ |
| 1.1 | exec Run seam                 | Complete        |                                            |
| 1.2 | Arch.ReleaseArch()            | Complete        |                                            |
| 1.3 | apt-repo manifest schema      | Complete        | docker-ce representative package           |
| 1.4 | Backend interface + Engine    | Complete        |                                            |
| 1.5 | download/verify/extract       | Complete        |                                            |
| 2.1 | Privilege model               | Complete        | Fixed Fake race via CallsSnapshot + mutex  |
| 3.1 | pkg backend                   | Complete        |                                            |
| 3.2 | github-release backend        | Complete        |                                            |
| 3.3 | aur backend                   | Complete        |                                            |
| 3.4 | toolchain backend             | Complete        |                                            |
| 3.5 | apt-repo backend              | Complete        |                                            |
| 3.6 | script-url backend            | Complete        |                                            |
| 3.7 | custom-script backend         | Complete        |                                            |
| 4.1 | Engine registration           | Complete        |                                            |
| 5.1 | dotf install                  | Complete        | --manifest, --dry-run, --intent            |
| 5.2 | dotf status / diff            | Complete        | Read-only drift report                     |
| 6.1 | Wire subcommands in main      | Complete        |                                            |

## Validation Results

| Level           | Status | Notes                                              |
| --------------- | ------ | -------------------------------------------------- |
| Static Analysis | Pass   | `go build ./...`, `go vet ./...`, golangci-lint 0  |
| Unit Tests      | Pass   | `go test -race ./...` all green                    |
| Build           | Pass   |                                                    |
| Integration     | N/A    | Manual VM install deferred                         |
| Edge Cases      | Pass   | Table tests cover argv, traversal, sha mismatch    |

## Files Changed

| File / Area                         | Action  | Notes                          |
| ----------------------------------- | ------- | ------------------------------ |
| `internal/exec/`                    | UPDATED | Added `Run` to Runner + Fake   |
| `internal/platform/types.go`        | UPDATED | `ReleaseArch()`                |
| `internal/manifest/`                | UPDATED | `AptRepo` schema + fixtures    |
| `internal/backend/`                 | CREATED | 7 backends + download + engine |
| `internal/privilege/`               | CREATED | Sudo keep-alive + unix drop    |
| `cmd/dotf/install.go`               | CREATED | install subcommand             |
| `cmd/dotf/status.go`                | CREATED | status + diff subcommands      |
| `cmd/dotf/main.go`                  | UPDATED | dispatch wiring                |
| `manifest/packages.toml`            | UPDATED | one example per backend        |

## Deviations from Plan

- Added `CallsSnapshot()` and mutex to `exec.Fake` to fix data race when `KeepAlive` goroutine records calls concurrently with tests (required for `-race` clean).
- `fetchVerifyScript` split from `fetchVerifyExtract` for raw `.sh` URLs (archives vs scripts).
- `Default()` accepts `repoRoot` parameter; `DOTFILES_ROOT` env fallback documented in custom-script backend.
- `TestResolveRepoRootFromCWD` expects repo root (manifest ancestor), not literal cwd — correct walk-up behavior.

## Issues Encountered

- Batch B2 race detector failure on KeepAlive test — fixed with thread-safe Fake recording.
- Lint errcheck on cleanup defers — wrapped with `_ =` in defer funcs.
- Pre-existing `TestResolveRepoRootFromCWD` expectation corrected.

## Tests Written

| Test Area              | Coverage                                              |
| ---------------------- | ----------------------------------------------------- |
| `internal/backend/*`   | Per-backend exact-argv, Installed probes, download    |
| `internal/privilege/`  | Prime, KeepAlive, RealUser, drop                      |
| `internal/exec/`       | Run recording + canned errors                         |
| `cmd/dotf/`            | install dry-run/sudo/failures, status/diff read-only  |

## Next Steps

- [ ] Code review via `/code-review`
- [ ] Create PR via `/prp-pr`
- [ ] Manual VM validation: `dotf install --dry-run` then single-backend installs
- [ ] Follow-on data task: port remaining `bin/install-*.sh` entries to manifest
