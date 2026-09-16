# Code Review — Provisioning CLI Foundation (local, uncommitted)

- **Mode**: Local (uncommitted changes)
- **Branch**: `feat/provisioning-cli-foundation`
- **Date**: 2026-06-30
- **Reviewers**: 3 parallel sub-agents (correctness, security, best-practices) + orchestrator verification
- **Decision**: 🔶 **REQUEST CHANGES** (2 must-fix before commit; rest are follow-ups)

## Scope

```
cmd/dotf/main.go
internal/platform/{platform,osrelease,types}.go
internal/exec/{exec,fake}.go
internal/version/version.go
internal/{platform,exec,version}/*_test.go
go.mod  .golangci.yml  .github/workflows/go.yml  lefthook.yml  package.json
docs (report, CLAUDE.md, AGENTS.md, .cursor/rules)
```

## Validation (all green)

| Check | Result |
| --- | --- |
| `go build ./...` | ✅ pass |
| `go vet ./...` | ✅ pass |
| `go test -race ./...` | ✅ pass (exec, platform, version) |
| `golangci-lint run ./...` | ✅ 0 issues |

## Parity verdict (`common/platform.sh` ↔ `internal/platform`)

**Confirmed exact** by two independent passes (orchestrator + correctness reviewer). OS family
mapping (ID + ID_LIKE fallback sets and priority), arch mapping (+ raw passthrough), package-manager
priority (brew→apt→dnf→yum→pacman→zypper), container detection, and `distroName` "unknown" fallback
all match the shell string-for-string. The "WSL only when `/etc/os-release` is absent" quirk is
faithfully reproduced. The Go implementation additionally degrades gracefully (uname→`runtime.GOOS`)
where the shell would simply fail — a superset, not a divergence.

---

## Findings

Status legend: `Open` / `Fixed` / `Failed`. IDs are stable for `/ycc:review-fix`.

### F1 — [High] `flag.CommandLine` (ExitOnError) makes `run()`'s error branch dead & untestable
- **File**: `cmd/dotf/main.go:22-25`
- **Status**: Fixed
- **Verified**: CONFIRMED (reproduced: `flag.CommandLine.Parse([]string{"-badflag"})` prints usage and
  `os.Exit(2)` — the following statement never executes).
- **Defect**: `flag.CommandLine` is constructed by the stdlib with `ExitOnError`. `Parse` therefore
  never returns a non-nil error to `run()`; on a bad flag it prints its own usage (keyed on
  `os.Args[0]`, **not** `progName`) and calls `os.Exit(2)` directly. Consequences:
  1. The `if err != nil { … return 2 }` block is unreachable dead code.
  2. `run([]string{"-x"})` kills the test process instead of returning `2`, so `run()` cannot be
     unit-tested against flag errors.
  3. The error message for bad flags is inconsistent with the `dotf: …` prefix used elsewhere in the
     same function.
- **Fix**: Build a local `fs := flag.NewFlagSet(progName, flag.ContinueOnError)` inside `run()` (optionally
  `fs.SetOutput(io.Discard)` or an injected writer) and parse `args` through it. Makes the existing
  error branch reachable and `run()` a pure function of `args`.

### F2 — [High] No tests for `cmd/dotf` (policy: tests ship with the change)
- **File**: `cmd/dotf/main.go` (whole package)
- **Status**: Fixed
- **Defect**: `run()`/`runVersion()`/`runPlatform()` and subcommand dispatch (missing, unknown,
  `version`, `platform`) have zero coverage, unlike every other new package here. Repo policy requires
  tests alongside changes; the entrypoint's exit-code logic is exactly what a cheap table test guards.
- **Fix**: Add `cmd/dotf/main_test.go` (depends on F1): no args → exit 1 + stderr; unknown subcommand
  → exit 1 + stderr; `version` → exit 0 + stdout contains version; `platform` → exit 0 + stdout matches
  `os=… arch=… pkg=… container=… distro=…`.

### F3 — [Medium] ID_LIKE fallback arms for redhat/arch/suse are untested
- **File**: `internal/platform/platform_test.go` (vs `platform.go:100-107`)
- **Status**: Fixed
- **Defect**: Only the debian ID_LIKE-fallback path is exercised. redhat/arch/suse are only hit via
  direct-`ID` matches, so a regression in those specific fallback arms would not be caught.
- **Fix**: Add cases with an unrecognized `ID` forcing the fallback, e.g.
  `ID=somedistro\nID_LIKE=rhel` → `OSRedHat`; likewise `arch` and `suse`.

### F4 — [Medium] Third-party CI actions pinned to mutable tags
- **File**: `.github/workflows/go.yml:29,32,44`
- **Status**: Fixed
- **Defect**: `actions/checkout@v4`, `actions/setup-go@v5`, `golangci/golangci-lint-action@v6` are
  floating tags. A hijacked/re-pointed tag would run arbitrary code in CI (the third-party
  golangci-lint action is the higher-value target). Workflow is otherwise well-scoped
  (`permissions: contents: read`, `pull_request` not `pull_request_target`, no secret/`${{ }}`
  interpolation in `run:` steps).
- **Fix**: Pin to full commit SHAs with a version comment; add Dependabot/Renovate to bump pins.

### F5 — [Medium] Doc comments cite exact shell line numbers (fragile) + one off-by-one
- **File**: `internal/platform/platform.go` (~9 citations), `osrelease.go:13-16`
- **Status**: Fixed
- **Defect**: Citations like "lines 50-87" / "line 114" are currently accurate **except** the
  `osFamilyFromID` "lines 58-72" note, which should be 58-73 (excludes the closing `esac`). More
  broadly, any unrelated edit to `common/platform.sh` silently invalidates every citation with no
  compiler/lint signal.
- **Fix**: Cite function names only (`mirrors common/platform.sh:detect_os`), or add a tiny CI check
  asserting the cited function still starts near the referenced line. At minimum correct the off-by-one.

### F6 — [Low] `detectPkgMgr` doc overclaims "exactly" vs shell `command -v brew`
- **File**: `internal/platform/platform.go:158-179`
- **Status**: Fixed
- **Defect**: Shell `command -v brew` also resolves shell functions/aliases; Go `LookPath` resolves
  PATH executables only. Edge case (a `brew` shell-function shim with no PATH binary) where the two
  disagree, contradicting the "exactly" claim.
- **Fix**: Soften the doc comment to note the `command -v` vs `LookPath` distinction.

### F7 — [Low] `runPlatform` doc falsely claims to mirror `print_platform_info`
- **File**: `cmd/dotf/main.go:50-51`
- **Status**: Fixed
- **Defect**: `print_platform_info` prints a multi-line human block (incl. Homebrew/copy/paste/open,
  none detected in Go); `runPlatform` prints a single-line `key=value`. Different formats/purposes.
- **Fix**: Reword to "machine-greppable analogue of …" or drop the parity claim.

### F8 — [Low] `detectOS` Windows and "unknown" branches untested
- **File**: `internal/platform/platform_test.go` (vs `platform.go:41-44`)
- **Status**: Fixed
- **Fix**: Feed `uname -s` = `MINGW64_NT-10.0` → `OSWindows` and e.g. `FreeBSD` → `OSUnknown`.

### F9 — [Low] `bufio.Scanner.Err()` unchecked after os-release scan
- **File**: `internal/platform/osrelease.go:29-49`
- **Status**: Fixed
- **Defect**: A line exceeding the 64KB default token limit would stop `Scan()` early and silently
  return partial results (degrades to wrong/unknown distro; no security impact).
- **Fix**: Check `scanner.Err()` after the loop.

### F10 — [Low] `unquote` single-quote path untested
- **File**: `internal/platform/osrelease.go:55-62` (vs `TestDistroName`)
- **Status**: Fixed
- **Fix**: Add a single-quoted `ID` fixture to lock in behavior.

### F11 — [Low] `Runner` interface defined at producer, not consumer
- **File**: `internal/exec/exec.go:16-24`
- **Status**: Fixed
- **Note**: Defensible as a deliberately shared test seam (`fake.go` is meant for other packages),
  but worth a one-line package-doc note so it doesn't read as an oversight.

### F12 — [Low] `detectArch` inlines uname fallback while `detectOS` extracts `unameS`
- **File**: `internal/platform/platform.go:135-140` vs `48-66`
- **Status**: Fixed
- **Fix**: One-line comment or a matching `unameM` helper to remove the "why is this asymmetric" question.

### F13 — [Low] `ArchX86_64` underscore lacks a "deliberate exception" comment
- **File**: `internal/platform/types.go:46`
- **Status**: Fixed
- **Note**: Intentional (mirrors raw `x86_64`) and lint-clean today, but a `//nolint`/comment guards
  against a future stricter linter or well-meaning "fix".

### F14 — [Nit] PATH-based `uname` resolution
- **File**: `internal/platform/platform.go:52,136`
- **Status**: Open
- **Note**: Not exploitable for this unprivileged, non-setuid CLI. Only relevant if ever invoked from
  an elevated context with attacker-influenced `$PATH`.

### F15 — [Nit] CI/tooling polish
- **Status**: Open
- `.github/workflows/go.yml`: single-OS matrix (`ubuntu-latest`) — add `macos-latest` once there's
  Darwin real-exec code; `cache: true` is a no-op with no `go.sum` (fine as forward-compat).
- `.golangci.yml`: minimal `standard` + `revive` set — `gosec`/`unparam` deferred (a decision, not a bug).
- `candidates` slice in `detectPkgMgr` is rebuilt per call (moot — `Detect()` runs once/process).

---

## Passed checks (no issues)

- **Security**: no `sh -c`/shell interpolation; the exec package's "never shell out" contract is
  honored; no external input reaches a command/format-string/injection sink; stdlib-only `go.mod`
  (no `go.sum`, no new npm deps); CI uses least-privilege `permissions` and the safe `pull_request`
  trigger.
- **Docs consistency**: `CLAUDE.md`/`AGENTS.md`/`.cursor/rules` documented commands match
  `go.yml`/`lefthook.yml`/`package.json`/`.golangci.yml`. `.gitignore` `/dotf` anchoring does not
  shadow `cmd/dotf/`.
- **Test structure**: table-driven, hermetic (Fake-based), explicit no-panic/graceful-degradation
  coverage, and `var _ Runner = …` compile-time assertions. Strongest part of the change.

## Recommended order

1. **F1** (unblocks F2) → **F2**, then **F3** — closes the correctness bug + test-policy gap.
2. **F4**, **F5** — CI hardening + doc-fragility.
3. Remaining Low/Nit as convenient.
