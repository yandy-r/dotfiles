# Plan: Foundation & Platform Core (`dotf` Go skeleton + unified platform detection)

## Summary

Stand up the greenfield Go module that the provisioning-CLI rewrite builds on: `go.mod`
(`github.com/yandy-r/dotfiles`), a thin `cmd/dotf/main.go`, a canonical `internal/platform`
detector that ports `common/platform.sh` and retires the three drifted OS/arch/pkg-mgr copies,
and a mockable exec seam for table-driven tests. This is also the shell-only → Go+shell
**mono-repo flip**, so it wires Go into CI (`.github/workflows/go.yml`), the lefthook
pre-commit branch, and the existing `bin/style.sh` formatter bundle, and refreshes the
AI-agent doc trio for the mixed stack.

## User Story

As the maintainer, I want a single source of truth for platform detection so that adding or
fixing a tool never requires editing three places, and `amd64`-vs-`x86_64` style drift cannot recur.

## Problem → Solution

OS/arch/pkg-mgr detection lives in three drifted copies (`common/platform.sh`,
`bootstrap.sh:286-368`, and 13/25 `bin/install-*.sh` local `detect_*` — `install-tea.sh:69`
already returns `amd64` where the canonical value is `x86_64`) with zero Go, CI, or tests →
one canonical, table-tested `internal/platform` Go package behind a mockable exec seam, with CI
and pre-commit enforcing both Go and shell, leaving the bash system untouched and still default.

## Metadata

- **Complexity**: Large (greenfield Go module + CI + lefthook/formatter wiring + doc refresh)
- **Source PRD**: docs/prps/prds/provisioning-cli.prd.md
- **PRD Phase**: Phase 1 — Foundation & platform core
- **Tracking**: Forgejo issue #4 (Unified platform detection), tracker #16
- **Estimated Files**: ~16 (10 Go new, 6 config/doc updated)

## Batches

Tasks grouped by dependency for parallel execution. Tasks within the same batch run concurrently; batches run in order.

| Batch | Tasks              | Depends On | Parallel Width |
| ----- | ------------------ | ---------- | -------------- |
| B1    | 1.1                | —          | 1              |
| B2    | 2.1, 2.2, 2.3, 2.4 | B1         | 4              |
| B3    | 3.1, 4.1, 4.2      | B2         | 3              |
| B4    | 5.1                | B3         | 1              |
| B5    | 5.2                | B4         | 1              |

- **Total tasks**: 7
- **Total batches**: 5
- **Max parallel width**: 4

---

## UX Design

### Before

```
$ source common/platform.sh && echo "$DOTFILES_OS $DOTFILES_ARCH"   # shell-only debug path
arch x86_64
# No `dotf` binary exists. No Go. No CI. lint = shell + python only.
```

### After

```
$ dotf version            # new binary exists, prints version (ldflags-injected)
dotf dev (none)
$ dotf platform           # canonical detector, same vocab as common/platform.sh
os=arch arch=x86_64 pkg=pacman container=none distro=cachyos
# Pre-commit now lints Go AND shell; CI runs go test/vet/golangci-lint.
```

### Interaction Changes

| Touchpoint            | Before                                  | After                                            | Notes                                                            |
| --------------------- | --------------------------------------- | ------------------------------------------------ | ---------------------------------------------------------------- |
| `dotf` binary         | does not exist                          | builds; `version` + `platform` debug subcommands | Phase-1 surface only; intent TUI/install engine are later phases |
| Pre-commit (lefthook) | `bin/lint.sh --staged --shell --python` | `… --shell --python --go`                        | one consistent config; no second dispatcher                      |
| CI                    | none (`.github/workflows/` absent)      | `go.yml`: test/vet/golangci-lint on PR           | shell flow remains unaffected                                    |
| Maintainer workflow   | edit detection in 3 places              | edit `internal/platform` once                    | drift cannot recur                                               |

---

## Mandatory Reading

Files that MUST be read before implementing:

| Priority       | File                                     | Lines                                  | Why                                                                  |
| -------------- | ---------------------------------------- | -------------------------------------- | -------------------------------------------------------------------- |
| P0 (critical)  | `common/platform.sh`                     | 48-150                                 | Canonical OS/arch/pkg-mgr/container vocab to port verbatim into Go   |
| P0 (critical)  | `docs/prps/prds/provisioning-cli.prd.md` | 112-127                                | Architecture Notes: layout, module path, binary name, CI/lefthook    |
| P0 (critical)  | `bin/style.sh`                           | 1-50, 430-470, 640-690, 780-800        | Existing Go-aware formatter bundle (lint/format/init `--go`)         |
| P1 (important) | `bin/install-tea.sh`                     | 60-73                                  | Concrete arch drift (`amd64` vs canonical `x86_64`) to canonicalize  |
| P1 (important) | `bootstrap.sh`                           | 286-368                                | Second drifted detection copy (do NOT modify; parallel-then-cutover) |
| P1 (important) | `lefthook.yml`                           | all                                    | Hook wiring (pre-commit, commit-msg, pre-push) to extend for Go      |
| P1 (important) | `package.json`                           | all                                    | Lint and format npm scripts that wrap the bin dispatchers            |
| P2 (reference) | `bin/go-tools.sh`                        | 67-128                                 | `.golangci.yml` v2 generator + Go task idioms already in-repo        |
| P2 (reference) | `bin/lib/modified-files.sh`              | all                                    | Git-scope helper the bundle shares; bundle manifest constants        |
| P2 (reference) | `CLAUDE.md`                              | "Scoping style/lint/format operations" | Style-dispatcher contract + git-scope flags                          |

## External Documentation

| Topic                | Source                                           | Key Takeaway                                                                                       |
| -------------------- | ------------------------------------------------ | -------------------------------------------------------------------------------------------------- |
| Go project layout    | https://go.dev/doc/modules/layout                | `cmd/<bin>/main.go` thin entry + `internal/*` packages; keep `main` tiny                           |
| Version via ldflags  | https://pkg.go.dev/cmd/go#hdr-Compile_packages   | `-ldflags "-X github.com/yandy-r/dotfiles/internal/version.Version=…"` sets a package var at build |
| golangci-lint v2     | https://golangci-lint.run/usage/configuration/   | `.golangci.yml` uses `version: "2"` schema; `bin/go-tools.sh:98` already emits a v2 config         |
| golangci-lint-action | https://github.com/golangci/golangci-lint-action | Pin by tag (`@v6`); manages its own lint cache; pass `version:` input                              |
| setup-go cache       | https://github.com/actions/setup-go              | `actions/setup-go@v5` with `cache: true` caches module + build cache by `go.sum`                   |
| Exec seam idiom      | https://pkg.go.dev/os/exec                       | Wrap `exec.Command` behind a small interface; inject a fake in tests — no mocking framework needed |
| os-release parsing   | std-lib only                                     | Read `/etc/os-release` `ID`/`ID_LIKE` with `bufio.Scanner`; no dependency warranted                |

---

## Patterns to Mirror

Code patterns to follow. There is **no existing Go code** in the repo, so naming/test patterns
follow standard Go idiom; the detection logic and vocabulary are ported verbatim from
`common/platform.sh` (the canonical shell source).

### NAMING_CONVENTION

```go
// SOURCE: standard Go idiom (no prior Go in repo); package == lowercase dir name, no stutter.
// internal/platform/platform.go
package platform

// Exported funcs read like docs; return typed values, not strings.
func Detect(r exec.Runner) Info // Info{OS, Arch, PkgMgr, Container, Distro}
```

### ERROR_HANDLING

```sh
# SOURCE: common/platform.sh:50-86 — detection falls back through ID_LIKE, then a final
# "unknown" sentinel rather than erroring. Mirror the fall-through; return "unknown"/zero-value,
# never panic. Repo policy (CLAUDE.md): fail-fast at *boundaries*, but detection degrades gracefully.
case "$ID" in ubuntu|debian) echo "debian" ;; *) case "${ID_LIKE:-}" in *debian*) echo "debian";; *) echo "linux";; esac ;; esac
```

### LOGGING_PATTERN

```sh
# SOURCE: common/platform.sh:357-367 print_platform_info — plain "key: value" lines to stdout.
# Mirror for `dotf platform`: emit "os=… arch=… pkg=… container=… distro=…" (machine-greppable).
echo "  OS: $(detect_os)"
```

### SERVICE_PATTERN

```go
// SOURCE: idiomatic Go exec seam (no in-repo precedent). Interface lives in internal/exec,
// real impl wraps os/exec, fake impl returns canned output for table tests.
type Runner interface {
    Output(name string, args ...string) ([]byte, error)
    LookPath(name string) (string, error) // mirrors `command -v` in detect_package_manager
}
```

### REPOSITORY_PATTERN

```go
// SOURCE: common/platform.sh:50-98 reads /etc/os-release. Mirror: parse ID/ID_LIKE from the
// file (not via exec) so it is host-injectable in tests via a configurable path.
// internal/platform — osReleasePath defaults to "/etc/os-release", overridable in tests.
func parseOSRelease(path string) (id, idLike string)
```

### TEST_STRUCTURE

```go
// SOURCE: standard Go table-driven test; PRD requires "assert exact argv per (distro × backend)".
// Phase 1 establishes the fake-Runner + table harness the backend phases reuse.
func TestDetectArch(t *testing.T) {
    for _, tc := range []struct{ uname, want string }{
        {"x86_64", "x86_64"}, {"amd64", "x86_64"}, {"aarch64", "arm64"},
    } { t.Run(tc.uname, func(t *testing.T) { /* fake Runner returns tc.uname */ }) }
}
```

---

## Files to Change

| File                                 | Action | Justification                                                            |
| ------------------------------------ | ------ | ------------------------------------------------------------------------ |
| `go.mod`                             | CREATE | Module `github.com/yandy-r/dotfiles`, Go 1.26 — gates all Go compilation |
| `go.sum`                             | CREATE | Generated by `go mod tidy` (likely empty/minimal — std-lib only target)  |
| `cmd/dotf/main.go`                   | CREATE | Thin entrypoint; dispatch `version`/`platform` debug subcommands         |
| `internal/version/version.go`        | CREATE | `Version` var set via `-ldflags -X`; default `"dev"`                     |
| `internal/version/version_test.go`   | CREATE | Assert default + injected version formatting                             |
| `internal/exec/exec.go`              | CREATE | `Runner` interface + real `os/exec` impl (no `sh -c`)                    |
| `internal/exec/fake.go`              | CREATE | Test fake returning canned output keyed by command                       |
| `internal/exec/exec_test.go`         | CREATE | Verify real runner argv + fake routing                                   |
| `internal/platform/platform.go`      | CREATE | Canonical `Detect()` porting `common/platform.sh` vocab                  |
| `internal/platform/types.go`         | CREATE | Typed `OS`/`Arch`/`PkgMgr`/`Container` enums + `Info` struct             |
| `internal/platform/platform_test.go` | CREATE | Table-driven per (uname/os-release/pkg-mgr) → canonical value            |
| `.golangci.yml`                      | CREATE | Generated via `bin/style.sh init --go` (v2 schema)                       |
| `.github/workflows/go.yml`           | CREATE | PR CI: `go test ./...` + `go vet` + golangci-lint                        |
| `.gitignore`                         | UPDATE | Ignore Go build output (`/dotf`, `dotf`, `*.test`, coverage)             |
| `lefthook.yml`                       | UPDATE | Add `--go` to pre-commit `lint` and pre-push `lint-full`                 |
| `package.json`                       | UPDATE | Add `--go` to `lint*`/`format*` scripts; add `go:test`/`go:build`        |
| `CLAUDE.md`                          | UPDATE | Reflect Go+shell mono-repo (via `ycc:init --update --profile=mixed`)     |
| `AGENTS.md`                          | UPDATE | Same mono-repo refresh                                                   |
| `.cursor/rules/project.mdc`          | UPDATE | Same mono-repo refresh                                                   |

## NOT Building

- **Manifest schema / TOML parser / `go:embed`** — Phase 2 (issue beyond #4). Phase 1 ships no manifest.
- **Install backends, privilege model, drift report** — Phase 3.
- **Native symlink engine** — Phase 4 (Ansible stays in place).
- **System config (hostname/timezone/locale/shell/firewall)** — Phase 5.
- **Headless mode, flag parity, snapper, failure summary** — Phase 6.
- **GoReleaser, `bootstrap.sh` shim, script retirement, dockertest matrix** — Phase 7.
- **TUI (`charmbracelet/huh`) or any third-party runtime dependency** — Phase 1 is std-lib only; keep `go.sum` minimal.
- **Modifying `bootstrap.sh` / `bin/install-*.sh` detection** — parallel-then-cutover invariant: the bash system stays default and untouched this phase. The Go detector is added _alongside_, not wired into bash.
- **Migrating the formatter bundle to `scripts/style.sh`** — see Decision below; the repo keeps the single `bin/` dispatcher.
- **A dedicated `lint.yml`** — Go lint runs inside `go.yml`; a separate workflow is optional and deferred unless a split is later wanted.

---

## Step-by-Step Tasks

### Task 1.1: Initialize Go module & project layout — Depends on [none]

- **BATCH**: B1
- **ACTION**: Create the Go module, thin `cmd/dotf` entrypoint, and `internal/version` package so the repo compiles and `dotf version` runs.
- **IMPLEMENT**: `go mod init github.com/yandy-r/dotfiles` (Go 1.26). Write `internal/version/version.go` with `var Version = "dev"` and a `String()` helper. Write `cmd/dotf/main.go` as a minimal flag/subcommand dispatcher (`version`, `platform` — `platform` stubbed until 3.1) printing to stdout, returning non-zero on unknown subcommand. Run `go mod tidy`.
- **MIRROR**: NAMING_CONVENTION (package == dir, no stutter); keep `main` tiny.
- **IMPORTS**: `flag`/`os`/`fmt`; `github.com/yandy-r/dotfiles/internal/version`.
- **GOTCHA**: Module path MUST be `github.com/yandy-r/dotfiles` (matches `github` remote, not the `origin` Forgejo remote) per Decisions Log. Binary dir is `cmd/dotf` (not `cmd/dotfiles`) — Decisions Log locks `dotf`. **No Go version pin exists in-repo today** (`mise/config.toml` `[tools]` is empty; no `.tool-versions`/`.go-version`) — the `go 1.26` directive in `go.mod` becomes the authoritative pin, and `go.yml` consumes it via `go-version-file: go.mod`. Optionally also add `go = "1.26"` to `mise/config.toml` for local parity, but go.mod stays the source of truth.
- **VALIDATE**: `go build ./... && go run ./cmd/dotf version` prints `dotf dev (none)` (or similar); `go vet ./...` clean.

### Task 2.1: Mockable exec seam — Depends on [1.1]

- **BATCH**: B2
- **ACTION**: Add `internal/exec` with a `Runner` interface, a real `os/exec` implementation, and a test fake.
- **IMPLEMENT**: Define `Runner` with `Output(name string, args ...string) ([]byte, error)` and `LookPath(name string) (string, error)`. Real impl calls `exec.Command(name, args...).Output()` and `exec.LookPath` — **never** `sh -c` with interpolated strings. Fake impl matches on `name`+`args` and returns canned bytes/err. Add `exec_test.go` asserting argv passthrough and fake routing.
- **MIRROR**: SERVICE_PATTERN (interface in `internal/exec`, real wraps `os/exec`).
- **IMPORTS**: `os/exec`.
- **GOTCHA**: Establish the no-shell-interpolation rule now — every later backend reuses this seam; a `sh -c` shortcut here becomes a repo-wide injection footgun.
- **VALIDATE**: `go test ./internal/exec/...` passes.

### Task 2.2: Go CI workflow — Depends on [1.1]

- **BATCH**: B2
- **ACTION**: Create `.github/workflows/go.yml` running test/vet/lint on PRs touching Go.
- **IMPLEMENT**: Trigger on `pull_request` and `push` to `main` with `paths: ['**.go', 'go.mod', 'go.sum', '.github/workflows/go.yml', '.golangci.yml']`. Steps: `actions/checkout@v4`, `actions/setup-go@v5` (`go-version-file: go.mod`, `cache: true`), `go vet ./...`, `go test -race ./...`, `golangci/golangci-lint-action@v6`. Set `permissions: contents: read`.
- **MIRROR**: External Documentation (setup-go cache; golangci-lint-action pin).
- **GOTCHA**: `.golangci.yml` (Task 2.3) must exist for the lint step to pass — both land in B2; note the logical ordering. Pin actions by major tag at minimum.
- **VALIDATE**: `yamllint`/`actionlint` clean if available; workflow parses. (CI runs on the GitHub mirror, where issues are tracked on Forgejo — workflow targets `github` remote.)

### Task 2.3: golangci-lint config — Depends on [1.1]

- **BATCH**: B2
- **ACTION**: Generate `.golangci.yml` using the in-repo generator so config matches the bundle's expectations.
- **IMPLEMENT**: Run `bin/style.sh init --go` (which emits the v2-schema config via `bin/go-tools.sh:68 generate_golangci_config`). Verify it is `version: "2"`. Do not hand-roll a competing config.
- **MIRROR**: `bin/go-tools.sh:98-127` (v2 config template).
- **GOTCHA**: golangci-lint v2 config schema differs from v1; the generator already targets v2 — don't downgrade. Confirm the installed `golangci-lint` is v2.
- **VALIDATE**: `golangci-lint run ./...` executes without config-parse errors.

### Task 2.4: Ignore Go build artifacts — Depends on [1.1]

- **BATCH**: B2
- **ACTION**: Extend `.gitignore` for Go output.
- **IMPLEMENT**: Append a `# Go` block: `/dotf`, `dotf`, `*.test`, `*.out`, `coverage.*`. Keep `go.mod`/`go.sum` tracked (do NOT ignore). Leave existing entries intact.
- **MIRROR**: existing `.gitignore` section-comment style.
- **GOTCHA**: The compiled `dotf` binary must not be committed (Distribution ships via Releases). Don't ignore `cmd/dotf/` (source).
- **VALIDATE**: `git status --porcelain` shows no built binary after `go build -o dotf ./cmd/dotf`.

### Task 3.1: Canonical platform detector — Depends on [2.1]

- **BATCH**: B3
- **ACTION**: Implement `internal/platform` porting `common/platform.sh` detection into typed Go behind the exec seam, with table-driven tests.
- **IMPLEMENT**: `types.go`: typed `OS`/`Arch`/`PkgMgr`/`Container` string enums + `Info` struct + `Distro` raw-ID field. `platform.go`: `Detect(r exec.Runner) Info` composing `detectOS` (uname + `/etc/os-release` `ID`/`ID_LIKE` mapping → `macos|debian|redhat|arch|suse|linux|wsl|windows|unknown`), `detectArch` (`x86_64|amd64→x86_64`, `aarch64|arm64→arm64`, `armv7l|armv6l→arm`), `detectPkgMgr` (brew>apt>dnf>yum>pacman>zypper via `LookPath`), `detectContainer`, `distroName`. Wire `cmd/dotf platform` to print `os=… arch=… pkg=… container=… distro=…`.
- **MIRROR**: `common/platform.sh:50-150` vocab verbatim; REPOSITORY_PATTERN (injectable os-release path); TEST_STRUCTURE (table-driven).
- **IMPORTS**: `bufio`/`os`/`runtime`; `internal/exec`.
- **GOTCHA**: Vocabulary must match the shell **exactly** (e.g., `redhat` not `fedora`; `x86_64` not `amd64` — this is the drift that #4 exists to kill). `cachyos`/`manjaro`/`endeavouros`→`arch`. Prefer parsing `/etc/os-release` from an injectable path over exec so tests need no real host. Splitting os-release parsing into a small `internal/platform/osrelease.go` (mirroring `common/platform.sh:56-73,91-98`, incl. a raw `Distro` accessor like `get_distro_name`) is encouraged for testability. The drift to canonicalize spans **multiple** copies, not just `install-tea.sh`: `install-mise.sh:106-111` emits `x64`, `install-codex.sh:114-118` emits `amd64`, `bootstrap.sh:325-368` returns `none` (not `unknown`) and lacks global brew priority. Capture the worst offenders as named test cases.
- **VALIDATE**: `go test ./internal/platform/...` passes; on this host `go run ./cmd/dotf platform` prints `os=arch arch=x86_64 pkg=pacman …` matching `source common/platform.sh; echo $DOTFILES_OS $DOTFILES_ARCH $DOTFILES_PKG_MGR`.

### Task 4.1: Lefthook Go branch — Depends on [1.1, 2.3]

- **BATCH**: B3
- **ACTION**: Extend `lefthook.yml` so Go is linted alongside shell/python in one consistent config.
- **IMPLEMENT**: In `pre-commit.lint`, change `bin/lint.sh --staged --shell --python` → `… --shell --python --go`. In `pre-push.lint-full`, ensure the full pass includes Go (via `npm run lint`, updated in 4.2). Do not add a competing Go-only hook command.
- **MIRROR**: existing `lefthook.yml` command structure + `fail_text` style.
- **GOTCHA**: `bin/style.sh` skips Go gracefully when no `go.mod`/`*.go` exists, so adding `--go` is safe even on shell-only commits. The lefthook Go branch (AC #5) and the formatter wiring must be **one** config — don't let `ycc:formatters --hooks` and this edit fight over managed commands.
- **VALIDATE**: `lefthook run pre-commit` lints staged Go without error on a clean tree.

### Task 4.2: package.json Go scripts — Depends on [1.1, 2.3]

- **BATCH**: B3
- **ACTION**: Wire `--go` into the npm lint/format aliases and add Go convenience scripts.
- **IMPLEMENT**: Append `--go` to `lint`, `lint:*`, `lint:fix*`, `format`, `format:*` script bodies (they wrap `bin/lint.sh`/`bin/format.sh`). Add `"go:test": "go test ./..."`, `"go:vet": "go vet ./..."`, `"go:build": "go build -o dotf ./cmd/dotf"`.
- **MIRROR**: existing `package.json` script naming (`lint:staged` etc.).
- **GOTCHA**: Keep `--shell --python` flags present — only _add_ `--go`. `format` currently omits `--shell`; preserve its existing flag set, just add `--go`.
- **VALIDATE**: `npm run go:test` and `npm run lint` run end-to-end.

### Task 5.1: Mono-repo doc & tooling refresh — Depends on [4.1, 4.2]

- **BATCH**: B4
- **ACTION**: Refresh the AI-agent doc trio and confirm the formatter bundle is integrated (not duplicated) for the Go+shell mono-repo.
- **IMPLEMENT**: Run `ycc:init --update --profile=mixed --formatters` to refresh `CLAUDE.md`/`AGENTS.md`/`.cursor/rules/project.mdc` for Go+shell (diff-and-ask on conflicts; preserve customizations — do NOT `--force`). Then `ycc:formatters --sync --go --shell --docs --hooks` to reconcile the existing `bin/` bundle (reject `--sync --copy` together; do NOT drop a competing `scripts/style.sh`). Update CLAUDE.md "Stack Overview" to add Go and note `bin/style.sh` now covers Go.
- **MIRROR**: CLAUDE.md "Scoping style/lint/format operations" wording; existing `.cursor/rules/project.mdc` structure.
- **GOTCHA**: The formatter bundle is already installed under `bin/` (manages `go-tools.sh`, `lib/modified-files.sh`, `templates/`). `--sync` must target `bin/`, not create `scripts/`. See **Decision** in Notes.
- **VALIDATE**: `git diff --stat` shows only doc/config updates (no second dispatcher, no `scripts/style.sh`); CLAUDE.md/AGENTS.md mention Go.

### Task 5.2: Full validation & acceptance — Depends on [5.1]

- **BATCH**: B5
- **ACTION**: Run the complete validation gate proving every acceptance criterion.
- **IMPLEMENT**: `go build ./...`, `go vet ./...`, `go test -race ./...`, `golangci-lint run ./...`, `bin/style.sh lint --go --shell`, `lefthook run pre-commit`. Diff `dotf platform` output against `source common/platform.sh` exports on this host. Confirm `grep -rn 'detect_os\|detect_arch' --include=*.go` shows the single canonical definition.
- **MIRROR**: Validation Commands section below.
- **GOTCHA**: Bash detection copies still exist (intentionally — parallel-then-cutover); the "1 copy" metric applies to the **Go** side. Don't delete shell copies in this phase.
- **VALIDATE**: All commands green; outputs match; Acceptance Criteria all checked.

---

## Testing Strategy

### Unit Tests

| Test               | Input                                               | Expected Output         | Edge Case?              |
| ------------------ | --------------------------------------------------- | ----------------------- | ----------------------- |
| `detectArch`       | `uname -m` = `amd64`                                | `x86_64` (canonical)    | Yes — the drift case    |
| `detectArch`       | `aarch64` / `arm64`                                 | `arm64`                 | Yes                     |
| `detectArch`       | `armv7l`                                            | `arm`                   | Yes                     |
| `detectOS`         | os-release `ID=cachyos`                             | `arch`                  | Yes — distro family map |
| `detectOS`         | `ID=rocky`                                          | `redhat`                | Yes                     |
| `detectOS`         | unknown `ID`, `ID_LIKE=debian`                      | `debian`                | Yes — fallback path     |
| `detectOS`         | no `/etc/os-release`, `/proc/version` has microsoft | `wsl`                   | Yes                     |
| `detectPkgMgr`     | fake `LookPath` finds `pacman` only                 | `pacman`                | No                      |
| `detectPkgMgr`     | fake finds `brew` + `apt`                           | `homebrew` (priority)   | Yes — precedence        |
| `version.String`   | `Version="1.2.3"`                                   | contains `1.2.3`        | No                      |
| exec real `Output` | `echo hi`                                           | `hi\n` argv passthrough | No                      |

### Edge Cases Checklist

- [ ] Empty/missing `/etc/os-release` → `unknown`/`linux` fallback (not a crash)
- [ ] `uname -m` unrecognized → raw value returned (mirror shell `*) echo $(uname -m)`)
- [ ] No package manager found → `unknown`
- [ ] Invalid types — N/A (typed enums)
- [ ] Concurrent access — detector is pure/stateless; safe
- [ ] Network failure — N/A (no network in Phase 1)
- [ ] Permission denied reading os-release → fallback, no panic

---

## Validation Commands

### Static Analysis

```bash
go vet ./...
golangci-lint run ./...
```

EXPECT: Zero vet/lint errors.

### Unit Tests

```bash
go test -race ./...
```

EXPECT: All tests pass (platform, exec, version).

### Full Test Suite

```bash
go build ./... && bin/style.sh lint --go --shell && lefthook run pre-commit
```

EXPECT: Build clean; Go+shell lint clean; pre-commit hook passes.

### Behavior Parity Check

```bash
go run ./cmd/dotf platform
# compare against:
( . common/platform.sh; echo "os=$DOTFILES_OS arch=$DOTFILES_ARCH pkg=$DOTFILES_PKG_MGR container=$DOTFILES_CONTAINER" )
```

EXPECT: `os`/`arch`/`pkg`/`container` values match the shell exports on this host.

### Manual Validation

- [ ] `go run ./cmd/dotf version` prints an ldflags-overridable version
- [ ] `git status` shows no committed `dotf` binary after a build
- [ ] CLAUDE.md/AGENTS.md describe the Go+shell mono-repo; only one style dispatcher exists (`bin/style.sh`)

---

## Acceptance Criteria

- [ ] `go test ./...` + `go vet` + `golangci-lint` green in CI (`.github/workflows/go.yml`)
- [ ] Detector returns canonical OS / arch / pkg-mgr matching `common/platform.sh` vocabulary, asserted in table-driven tests
- [ ] Exec interface allows mocking external commands for unit tests
- [ ] `go.mod` (`github.com/yandy-r/dotfiles`), `cmd/dotf/main.go`, and project layout build cleanly
- [ ] lefthook Go branch wired (gofmt/go vet/golangci-lint with `glob: "*.go"` behavior via `bin/style.sh --go`)
- [ ] Project-bootstrap tooling refreshed: CLAUDE.md/AGENTS.md/.cursor rules reflect Go+shell; `bin/style.sh` lints both Go and shell
- [ ] Success Metric: Go-side platform-detection definitions = 1 (single canonical `internal/platform`)
- [ ] Style-bundle path decision resolved inline (see Notes → Decision) — repo ends with one dispatcher + one lefthook config

## Completion Checklist

- [ ] Code follows discovered patterns (typed enums, exec seam, table tests)
- [ ] Error handling degrades gracefully to `unknown` (no panics at detection boundary)
- [ ] Logging follows `print_platform_info` key=value style for `dotf platform`
- [ ] Tests follow Go table-driven structure with the fake Runner
- [ ] No hardcoded host paths (os-release path is injectable)
- [ ] Documentation updated (CLAUDE.md/AGENTS.md/.cursor) via `ycc:init --update`
- [ ] No unnecessary scope additions (Phases 2–7 explicitly excluded)
- [ ] `bootstrap.sh` and `bin/install-*.sh` detection left untouched (parallel-then-cutover)
- [ ] `go.sum` minimal (std-lib only; no third-party runtime deps added)
- [ ] Self-contained — no questions needed during implementation

## Risks

| Risk                                                                                                     | Likelihood | Impact | Mitigation                                                                                     |
| -------------------------------------------------------------------------------------------------------- | ---------- | ------ | ---------------------------------------------------------------------------------------------- |
| Go detector vocab diverges from `common/platform.sh` (reintroducing drift the issue exists to kill)      | Med        | High   | Table tests assert every mapping verbatim against the shell; parity check task 5.2             |
| Exec seam designed with `sh -c`/string interpolation → injection footgun inherited by all later backends | Low        | High   | Establish arg-slice-only `Runner` now; no `sh -c`; reviewed in 2.1                             |
| `ycc:formatters --sync` drops a competing `scripts/style.sh` next to the live `bin/style.sh`             | Med        | Med    | Decision = keep `bin/`; `--sync` (never `--copy`); verify no `scripts/` created (5.1)          |
| lefthook Go branch and `--formatters --hooks` fight over managed pre-commit commands                     | Med        | Med    | Land both as one config; 4.1 + 5.1 reconciled; single `bin/lint.sh … --go` command             |
| golangci-lint v1 vs v2 config-schema mismatch breaks CI                                                  | Low        | Med    | Use in-repo v2 generator (`bin/go-tools.sh`); confirm installed binary is v2                   |
| CI runs on GitHub mirror while issues live on Forgejo `origin` — drift between forges                    | Low        | Low    | `go.yml` targets the `github` remote per Distribution decision; Forgejo is issue-tracking only |

## Notes

**Decision — style-bundle path (resolved inline per issue #4 comment): KEEP `bin/style.sh`.**
The repo's live dispatcher is `bin/style.sh` (34k), already Go-aware — it runs `golangci-lint run [--fix] ./...` (`bin/style.sh:442-463`), `gofmt -w` + `goimports -w` (`:655-688`), and `init --go` generates a v2 `.golangci.yml` (`bin/go-tools.sh:68`). `bin/{lint,format}.sh` wrappers, `bin/lib/modified-files.sh`, `bin/templates/`, lefthook, and the 12 `package.json` scripts all already point at `bin/`. There is **no** `scripts/style.sh`. Migrating to `scripts/` would churn lefthook.yml, package.json, CLAUDE.md, `.cursor/rules`, and both wrappers for zero functional gain and would violate the minimal-impact / parallel-then-cutover ethos. Therefore: integrate the formatters bundle in place at `bin/`, enable Go by adding `--go` to the existing invocations, and run `ycc:formatters --sync` (never `--copy`) so the managed `bin/` files stay current. One dispatcher, one lefthook config.

**Orientation for the implementor:**

- Module path is the **GitHub** slug (`github.com/yandy-r/dotfiles`) even though issues/this checkout's `origin` are on Forgejo (`git.example-tailnet.ts.net`). Both remotes exist; CI/releases target GitHub.
- Binary/dir is `dotf` / `cmd/dotf` (Decisions Log), not the `cmd/dotfiles` that appears in one older PRD prose line — the Decisions Log + issue #4 are authoritative.
- This phase is the **gate** for PRD Phases 2–5 (issue #16 tracker); keep the exec seam and platform `Info` contract clean and stable — later phases import them heavily.
- Go 1.26.4 resolves via mise shims, but **no version is pinned in-repo** — `go.mod`'s `go 1.26` introduces the pin; `go-version-file: go.mod` keeps CI on the declared version. `.github/workflows/` does not exist yet — `go.yml` is the repo's first workflow.
- **Asset-arch mapper (forward-looking, NOT required this phase)**: the install scripts each need a *release-asset* arch string (`amd64`/`x64`/`arm64`) that differs from the canonical detection enum. Design the `Arch` enum so a future `ReleaseArch()` mapper (the actual consumer is the Phase 3 `github-release` backend) can be added without reshaping the type — but do not build the mapper in Phase 1.
- **Commit convention**: this plan file lives under `docs/prps/plans/`, so commit it as `docs(internal): …`. The Go code/CI/tooling commits use `feat(go):` / `feat(tool):` per the PRD; `go` is already a valid commitlint scope (no `commitlint.config.cjs` change needed).

**Confidence Score: 9/10** — greenfield with no `go.mod`/CI to untangle, exact canonical vocab in hand, the formatter bundle already supports Go, and the one open decision (bundle path) is resolved by repo reality. The single point of residual uncertainty is exact `ycc:init`/`ycc:formatters` interactive diff behavior during the doc refresh (Task 5.1), which is interactive and low-risk.
