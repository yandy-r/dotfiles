# Implementation Report: SSH-Server Sysconfig Domain

## Summary

Added an `ssh-server` domain to `internal/sysconfig` so `dotf apply` and `dotf sysconfig --ssh-server on` install, key, enable, and firewall-open OpenSSH on Linux/WSL — mirroring legacy `bootstrap.sh configure_ssh_server` behavior. The domain follows the existing firewall-domain pattern (plan-first, idempotent, additive-only), integrates with manifest/CLI flag resolution, and is a no-op on macOS.

## Assessment vs Reality

| Metric        | Predicted (Plan) | Actual         |
| ------------- | ---------------- | -------------- |
| Complexity    | Medium           | Medium         |
| Confidence    | High             | High           |
| Files Changed | 15               | 18             |

## Tasks Completed

| #   | Task                              | Status          | Notes                                      |
| --- | --------------------------------- | --------------- | ------------------------------------------ |
| 1.1 | SSHServerMode enum + Config       | [done] Complete |                                            |
| 1.2 | manifest ssh_server + validation  | [done] Complete |                                            |
| 2.1 | Linux planSSHServer + helpers     | [done] Complete |                                            |
| 2.2 | macOS no-op skip                  | [done] Complete |                                            |
| 3.1 | dotf sysconfig command wiring     | [done] Complete | identityConfig caller updated early        |
| 4.1 | dotf apply pipeline pass          | [done] Complete | variadic mergeSysconfigResults             |
| 5.1 | help text + completion            | [done] Complete |                                            |
| 5.2 | CLAUDE.md docs                    | [done] Complete | macOS no-op noted                          |
| 6.1 | sysconfig domain tests            | [done] Complete | 8 Linux + 1 macOS tests                    |
| 6.2 | manifest validation tests         | [done] Complete | 5 subtests                                 |
| 6.3 | command-layer tests               | [done] Complete | help_test updated for new flag             |

## Validation Results

| Level           | Status      | Notes                                    |
| --------------- | ----------- | ---------------------------------------- |
| Static Analysis | [done] Pass | go build, go vet, golangci-lint clean    |
| Unit Tests      | [done] Pass | go test -race ./... all green            |
| Build           | [done] Pass | go build ./...                           |
| Integration     | N/A         | CLI-only feature; no server integration  |
| Edge Cases      | [done] Pass | Covered in table-driven unit tests       |

## Files Changed

| File                              | Action  | Lines        |
| --------------------------------- | ------- | ------------ |
| `internal/sysconfig/sysconfig.go` | UPDATED | +42          |
| `internal/sysconfig/linux.go`     | UPDATED | +136         |
| `internal/sysconfig/darwin.go`    | UPDATED | +1           |
| `internal/manifest/types.go`      | UPDATED | +4           |
| `internal/manifest/validate.go`   | UPDATED | +13          |
| `cmd/dotf/sysconfig.go`           | UPDATED | +31          |
| `cmd/dotf/apply.go`               | UPDATED | +20          |
| `cmd/dotf/applyphases.go`         | UPDATED | +91          |
| `cmd/dotf/helptext.go`            | UPDATED | +13          |
| `CLAUDE.md`                       | UPDATED | +4           |
| `internal/sysconfig/linux_test.go`| UPDATED | +303         |
| `internal/sysconfig/darwin_test.go`| CREATED | +24          |
| `internal/manifest/validate_test.go`| UPDATED | +70        |
| `cmd/dotf/sysconfig_test.go`      | UPDATED | +107         |
| `cmd/dotf/apply_test.go`          | UPDATED | +91          |
| `cmd/dotf/apply_helpers_test.go`  | UPDATED | +25          |
| `cmd/dotf/completion_test.go`     | UPDATED | +4           |
| `cmd/dotf/help_test.go`           | UPDATED | +7           |

## Deviations from Plan

- Task 3.1 updated `identityConfig` caller early (planned for 4.1) so `go build ./cmd/dotf/` stayed green between batches.
- Task 6.3 updated `help_test.go` (not listed in plan) to include `--ssh-server` in expected flag lists.
- `bin/style.sh --go` in plan validation uses `bin/style.sh lint --go` (actual script interface).

## Issues Encountered

None — all batches passed between-batch validation on first attempt.

## Tests Written

| Test File                         | Tests   | Coverage                          |
| --------------------------------- | ------- | --------------------------------- |
| `internal/sysconfig/linux_test.go`| 8 tests | install, unit, keygen, enable, fw |
| `internal/sysconfig/darwin_test.go`| 1 test | macOS no-op                       |
| `internal/manifest/validate_test.go`| 5 subtests | enum validation              |
| `cmd/dotf/sysconfig_test.go`      | 3 tests | flag, precedence, dry-run         |
| `cmd/dotf/apply_test.go`          | 2 tests | apply ordering, dry-run           |
| `cmd/dotf/completion_test.go`     | 1 case  | on\|off completion                |

## Next Steps

- [ ] Code review via `/code-review`
- [ ] Create PR via `/prp-pr`
