# Plan: Install Backend Engine + Privilege Model + Drift Report (Phase 3)

## Summary

Build the `dotf` install layer: a 7-backend install engine (`internal/backend/{pkg,github_release,aur,toolchain,apt_repo,script_url,custom_script}`) driven entirely by the existing manifest `[[package]]` data, a shared privilege model (`internal/privilege`: `sudo -v` prime + `sudo -n -v` keep-alive goroutine, drop to `$SUDO_USER` via `syscall.Credential{NoSetGroups:true}` for AUR builds), and a read-only drift report (`dotf status` / `dotf diff`). Every backend dispatches external commands through the existing `internal/exec.Runner` seam so table-driven tests can assert the **exact argv** per (distro × backend). Scope is the engine + one representative `[[package]]` per backend — not the full port of all 25 `bin/install-*.sh` scripts (that is a follow-on data task).

## User Story

As the dotfiles maintainer (and anyone cloning the repo) provisioning a fresh machine across Arch/CachyOS, macOS, Debian/Ubuntu, and WSL, I want every package the 25 imperative `bin/install-*.sh` scripts install to come from declarative manifest data through a small backend taxonomy that escalates privilege correctly and reports drift read-only, so that adding a tool is one TOML entry with zero control-flow edits, installs are single-password and never run AUR builds as root, and I can preview installed-vs-manifest deltas without any removal blast radius.

## Problem → Solution

Package installs live in 25 `bin/install-*.sh` scripts (`bootstrap.sh` + installers ≈ 14,815 lines) that hand-roll OS/backend dispatch, fallback chains, and privilege handling with no tests and no drift visibility. → A data-driven `internal/backend` engine + `internal/privilege` (prime-once/keep-alive, drop to `$SUDO_USER`) + read-only `status`/`diff`, where ~18 scripts collapse to manifest data and each backend's exact argv is asserted via the existing `internal/exec.Fake` seam.

## Metadata

- **Complexity**: Large
- **Source PRD**: docs/prps/prds/provisioning-cli.prd.md
- **PRD Phase**: Phase 3 — Install backend engine (Forgejo tracker #18: issues #7 engine, #8 privilege, #9 drift)
- **Estimated Files**: ~26 (18 CREATE, 8 UPDATE)

## Batches

Tasks grouped by dependency for parallel execution. Tasks within the same batch run concurrently; batches run in order. No two tasks in the same batch touch the same file.

| Batch | Tasks                        | Depends On | Parallel Width |
| ----- | ---------------------------- | ---------- | -------------- |
| B1    | 1.1, 1.2, 1.3, 1.4, 1.5      | —          | 5              |
| B2    | 2.1                          | B1         | 1              |
| B3    | 3.1, 3.2, 3.3, 3.4, 3.5, 3.6, 3.7 | B1, B2 | 7              |
| B4    | 4.1                          | B3         | 1              |
| B5    | 5.1, 5.2                     | B4         | 2              |
| B6    | 6.1                          | B5         | 1              |

- **Total tasks**: 17
- **Total batches**: 6
- **Max parallel width**: 7

---

## UX Design

Phase 3 is partly user-facing: it adds the `dotf install`, `dotf status`, and `dotf diff` subcommands and the single-sudo-prompt install UX. Output convention follows the existing CLI (`cmd/dotf/main.go:62-66`, `plan.go:88-102`): **machine-greppable `key=value` on stdout, diagnostics on stderr**.

### Before

- Entry is `./bootstrap.sh`; all progress is `[BOOTSTRAP]`-prefixed, colorized, on **stderr** (`bootstrap.sh:35-51`).
- Per-package lines are category-organized, not backend-attributed (`bin/install-native-packages.sh:185,202`).
- **Multiple sudo prompts**: each installer runs its own `sudo -v` (`bin/install-native-packages.sh:664`) with no keep-alive → re-prompts on timeout.
- No drift/status/diff concept exists anywhere.

### After

- `dotf install` — per-package result attributed to its backend, non-fatal failure accumulation + end-of-run summary (parity with `BOOTSTRAP_FAILURES`).
- `dotf status` — read-only: every manifest package with installed/missing state.
- `dotf diff` — read-only: only the drift (declared-not-installed; never proposes removal).
- **Single sudo prompt**: `sudo -v` prime + `sudo -n -v` keep-alive; AUR builds drop to `$SUDO_USER`.

### Interaction Changes

| Touchpoint          | Before                                             | After                                                            | Notes                                              |
| ------------------- | -------------------------------------------------- | --------------------------------------------------------------- | -------------------------------------------------- |
| Drift check         | none — state unknowable without re-running install | `dotf status` / `dotf diff`, read-only key=value                | NET-NEW; additive-only, report never mutates       |
| Per-package install | `Installing $pkg (category: …)`                    | line attributed to backend (`pkg`/`github-release`/…)           | grouping shifts from apt/pacman category → backend |
| Sudo prompt         | multiple prompts, re-prompt on timeout             | one prompt: `sudo -v` prime + `sudo -n -v` keep-alive           | also correctness: makepkg refuses root             |
| Failure summary     | `print_bootstrap_summary` `⚠` lines                | non-fatal accumulation + end-of-run summary                     | preserve `BOOTSTRAP_FAILURES` UX                   |

---

## Mandatory Reading

Files that MUST be read before implementing:

| Priority       | File                                   | Lines   | Why                                                                       |
| -------------- | -------------------------------------- | ------- | ------------------------------------------------------------------------- |
| P0 (critical)  | `internal/exec/exec.go`                | 1-47    | The `Runner` seam every backend/privilege call routes through; the `Run` extension lands here |
| P0 (critical)  | `internal/exec/fake.go`                | 1-101   | `Fake.Calls`/`AddCommand`/`AddPath` — the exact-argv assertion mechanism  |
| P0 (critical)  | `internal/manifest/types.go`           | 43-141  | `Package` fields + 7 `Backend` consts the engine dispatches on            |
| P0 (critical)  | `internal/manifest/validate.go`        | 100-190 | Per-backend required-field validation to extend for apt-repo; error style |
| P1 (important) | `internal/platform/types.go`           | 1-110   | `Info`, `Arch` (`ReleaseArch()` reserved at :34-37), `PkgMgr` consts      |
| P1 (important) | `internal/platform/platform.go`        | 170-195 | `LookPath` capability-probe pattern to mirror for `Installed()`           |
| P1 (important) | `cmd/dotf/main.go`                      | 1-66    | Subcommand dispatch + `runX(args, stdout, stderr, exec.New())` wiring     |
| P1 (important) | `cmd/dotf/plan.go`                      | 1-60    | `manifest.Resolve` + `platform.Detect` + greppable stdout pattern         |
| P2 (reference) | `internal/platform/platform_test.go`   | 22-47   | Table-driven + `exec.Fake` test harness to copy for backend tests         |
| P2 (reference) | `internal/manifest/manifest_test.go`   | 20-103  | `testdata/*.toml` golden + `errors.As(*ValidationError)` assertion        |
| P2 (reference) | `bin/install-hashicorp-tools.sh`       | 120-150 | Canonical apt-repo keyring/sources.list steps (schema source of truth)    |
| P2 (reference) | `bin/install-turso.sh`                  | 155-193 | github-release asset/arch templating + per-asset `.sha256`                |
| P2 (reference) | `bin/install-claude.sh`                 | 405-435 | Verify-before-execute checksum gate to mirror in `download.go`            |
| P2 (reference) | `docs/prps/prds/provisioning-cli.prd.md` | 112-137 | Technical Approach: backend taxonomy, privilege, reconciliation, risks    |

## External Documentation

| Topic                              | Source                                                                                     | Key Takeaway                                                                                                          |
| ---------------------------------- | ------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------- |
| GitHub release download (no API)   | https://docs.github.com/en/repositories/releasing-projects-on-github/about-releases        | `https://github.com/OWNER/REPO/releases/latest/download/<ASSET>` 302s to the CDN — never hits `api.github.com`, so **no 60/hr rate limit**. Prefer over the REST API. |
| GitHub REST rate limit             | https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api            | Unauth `api.github.com` = 60 req/hr/IP; a `latest`-tag API call burns quota and 403s look like "missing asset". Avoid. |
| Stream download + verify sha256    | https://pkg.go.dev/crypto/sha256 · https://pkg.go.dev/io#TeeReader                         | `io.TeeReader(resp.Body, sha256.New())`; `io.Copy(io.Discard, tee)` after extract so the hasher sees every byte; compare with `crypto/subtle.ConstantTimeCompare`. |
| tar.gz streams, zip does not       | https://pkg.go.dev/archive/tar · https://pkg.go.dev/archive/zip                            | `compress/gzip`+`archive/tar` chain as `io.Reader` (constant memory); `zip.NewReader` needs `io.ReaderAt`+size → buffer `.zip` to a temp file first. |
| Path-traversal (tar/zip slip)      | https://pkg.go.dev/path/filepath#IsLocal                                                   | Validate `header.Name` against destDir with `filepath.Clean`+prefix check; `tar.ErrInsecurePath` only fires under `GODEBUG=tarinsecurepath=0`. Chmod extracted binary `0o755`. |
| Non-interactive pkg-manager flags  | codebase `bin/install-arch-pkgs.sh`, `bin/install-hashicorp-tools.sh`, `bin/install-azure-cli.sh` | `pacman -S --noconfirm --needed`; `apt-get install -y` + `DEBIAN_FRONTEND=noninteractive`; `dnf install -y`; `zypper --non-interactive install`; `brew install` (never root, `HOMEBREW_NO_AUTO_UPDATE=1`). |
| yay refuses root                   | https://github.com/Jguer/yay                                                               | `yay -S --noconfirm`; must run as `$SUDO_USER` with real `$HOME` — matches PRD privilege decision.                   |
| sudo timestamp priming             | codebase `bin/install-native-packages.sh:664`                                              | `sudo -v` primes the timestamp (interactive TTY); `sudo -n -v` refreshes non-interactively for keep-alive.          |
| syscall privilege drop             | https://pkg.go.dev/syscall#Credential · https://pkg.go.dev/os/exec#Cmd                     | `Cmd.SysProcAttr.Credential{Uid,Gid,NoSetGroups:true}` + explicit `Cmd.Env` (`HOME`,`USER`); never inherit root env for the dropped child. |

---

## Patterns to Mirror

Code patterns discovered in the codebase. Follow these exactly.

### NAMING_CONVENTION

```go
// SOURCE: internal/manifest/types.go:100-141 — typed-string enum + Valid() switch.
// Mirror this for any new closed set; dispatch keys off these exact consts.
type Backend string
const ( BackendPkg Backend = "pkg"; BackendGitHubRelease Backend = "github-release" /* … */ )
func (b Backend) Valid() bool { switch b { case BackendPkg /* … */: return true; default: return false } }
```

```go
// SOURCE: internal/exec/exec.go:35 + fake.go:42 — constructor naming.
// New() returns the interface; New<Thing>() returns the concrete pointer.
func New() Runner { return osRunner{} }   // real
func NewFake() *Fake { /* … */ }          // test double
// Package layout: types.go (data) · <pkg>.go (impl+New) · fake.go (double). One file per backend.
```

### ERROR_HANDLING

```go
// SOURCE: internal/manifest/validate.go:17-40,96 — typed error struct + aggregate.
type ValidationError struct{ Package, Intent, Msg string }
func (e *ValidationError) Error() string { /* package-prefixed message */ }
return errors.Join(errs...)   // collect all failures, don't stop at first
// Boundary wrap: fmt.Errorf("manifest: decode: %w", err)  (load.go:25-31)
```

```go
// SOURCE: internal/exec/exec.go:7-9 — argv-safety contract to honor in every backend.
// NEVER shell out via `sh -c` with interpolated strings. Always pass args as a slice.
return osexec.Command(name, args...).Output()   // exec.go:40-42
```

### REPOSITORY_PATTERN

```go
// SOURCE: internal/platform/platform.go:187-192 — read-only capability probe.
// Mirror for backend Installed(): LookPath (or pacman -Q / dpkg -s / rpm -q / zypper se -i / brew list).
for _, c := range candidates {
    if _, err := r.LookPath(c.bin); err == nil { return c.mgr }
}
```

```go
// SOURCE: cmd/dotf/plan.go:33 — embedded-manifest resolution reused verbatim by install/status.
m, err := manifest.Resolve(flagManifest, os.Getenv("DOTF_MANIFEST"), dotfiles.ManifestFS)
info := platform.Detect(r)
```

### SERVICE_PATTERN

```go
// SOURCE: internal/exec/exec.go:21-29 + fake.go:81-89 — injected seam + argv recording.
type Runner interface {
    Output(name string, args ...string) ([]byte, error)
    LookPath(name string) (string, error)
}
// Fake.Output records FakeCall{Name, Args} into f.Calls in order → assert exact per-backend argv.
```

```go
// SOURCE: cmd/dotf/main.go:39-51 + plan.go:22 — subcommand dispatch + thin runX.
switch cmd := fs.Arg(0); cmd {
case "plan": return runPlan(fs.Args()[1:], os.Stdout, os.Stderr, exec.New())
// add: case "install"/"status"/"diff": return runInstall/runStatus/runDiff(…, exec.New())
}
```

### TEST_STRUCTURE

```go
// SOURCE: internal/platform/platform_test.go:22-47 — table-driven + exec.Fake canned argv.
tests := []struct{ name, unameM, want string }{ /* … */ }
for _, tt := range tests { t.Run(tt.name, func(t *testing.T) {
    r := exec.NewFake(); r.AddCommand("uname", []string{"-m"}, []byte(tt.unameM+"\n"), nil)
    if got := detectArch(r); got != tt.want { t.Errorf("got %q want %q", got, tt.want) }
}) }
```

```go
// SOURCE: internal/exec/exec_test.go:53-58 — exact-argv assertion on recorded calls.
if f.Calls[0].Name != "git" || f.Calls[0].Args[0] != "status" { t.Fatalf("wrong argv: %+v", f.Calls) }
// SOURCE: internal/manifest/manifest_test.go:97-100 — typed-error match, not string compare.
var valErr *manifest.ValidationError; if !errors.As(err, &valErr) { t.Fatalf("want ValidationError") }
```

---

## Files to Change

| File                                        | Action | Justification                                                                                          |
| ------------------------------------------- | ------ | ----------------------------------------------------------------------------------------------------- |
| `internal/exec/exec.go`                     | UPDATE | Add `Run(name, args...) error` (TTY-attached stdio) to `Runner` + `osRunner`; keep `Output`/`LookPath` |
| `internal/exec/fake.go`                     | UPDATE | Implement `Fake.Run` (record in `Calls`, canned error via `RunErrors`); keep `var _ Runner` assertion  |
| `internal/exec/exec_test.go`                | UPDATE | Cover `Run` argv recording + canned-error propagation                                                  |
| `internal/platform/types.go`                | UPDATE | Add `func (a Arch) ReleaseArch() string` (`x86_64→amd64`, `arm64→arm64`, passthrough) — reserved at :34-37 |
| `internal/platform/platform_test.go`        | UPDATE | Table test for `ReleaseArch()` mapping                                                                  |
| `internal/manifest/types.go`                | UPDATE | Add nested `AptRepo` struct + `Package.AptRepo` field (additive; frozen fields untouched)               |
| `internal/manifest/validate.go`             | UPDATE | Add `BackendAPTRepo` required-field checks (key url / keyring / repo line / sources path)               |
| `internal/manifest/testdata/valid.toml`     | UPDATE | Add a valid apt-repo package example so the loader round-trips it                                       |
| `internal/manifest/testdata/invalid_apt_repo_missing_field.toml` | CREATE | Golden fixture: apt-repo missing a required field → `ValidationError`             |
| `internal/manifest/manifest_test.go`        | UPDATE | Register the new invalid apt-repo fixture in the table                                                  |
| `manifest/packages.toml`                    | UPDATE | Add one representative `[[package]]` per backend (data example only; additive)                          |
| `internal/backend/backend.go`               | CREATE | `Backend` interface (`Install`/`Installed`) + `Engine` (`map[manifest.Backend]Backend`) + typed errors  |
| `internal/backend/download.go`              | CREATE | Shared stdlib helper: GET → 0600 temp → sha256 gate → tar.gz/zip extract (traversal-guarded) → chmod 0755 |
| `internal/backend/download_test.go`         | CREATE | `httptest.Server` cases: checksum match/mismatch, tar.gz + zip extract, traversal rejection             |
| `internal/backend/pkg.go`                   | CREATE | Resolve `Names[info.PkgMgr]`; per-manager install argv + read-only `Installed` query                    |
| `internal/backend/github_release.go`        | CREATE | Template asset via `info.OS`+`Arch.ReleaseArch()`, `releases/latest/download/`, `download.go`, verify   |
| `internal/backend/aur.go`                   | CREATE | `yay -S --noconfirm` (bootstrap yay via `makepkg` if absent) run through `privilege.Dropper`            |
| `internal/backend/toolchain.go`             | CREATE | Map `Tool`→cmd (`cargo install`, `npm i -g`); `Installed` via `LookPath`                                |
| `internal/backend/apt_repo.go`              | CREATE | Fetch key → dearmor → keyring; write sources line; `apt-get update`; install `Names[apt]`               |
| `internal/backend/script_url.go`            | CREATE | Download `URL` (https), optional sha256 verify, execute; `Installed` via `LookPath`                     |
| `internal/backend/custom_script.go`         | CREATE | Resolve repo-relative `Script` (reject abs/`..`), execute; `Installed` via `LookPath`                   |
| `internal/backend/*_test.go` (per backend)  | CREATE | Table-driven (distro × backend) exact-argv assertions + `Installed` cases via `exec.Fake`               |
| `internal/backend/register.go`              | CREATE | `Default(r, sudo, drop) *Engine` wiring all 7 concrete backends into the dispatch map                   |
| `internal/privilege/privilege.go`           | CREATE | `Sudo{Prime,KeepAlive(ctx),Prefix}` + real-user resolution from `SUDO_USER`/`SUDO_UID`/`SUDO_GID`       |
| `internal/privilege/drop.go`                | CREATE | (`//go:build unix`) `Dropper` via `SysProcAttr.Credential{NoSetGroups:true}` + explicit `HOME`/`USER`   |
| `internal/privilege/drop_other.go`          | CREATE | (`//go:build !unix`) stub returning an unsupported-platform error (GoReleaser targets windows/darwin)   |
| `internal/privilege/fake.go`                | CREATE | Fake `Dropper` recording dropped-command argv/uid for tests                                             |
| `internal/privilege/privilege_test.go`      | CREATE | Prime/KeepAlive emit `sudo -v`/`sudo -n -v`; user resolution set/unset; drop no-ops when euid≠0          |
| `cmd/dotf/install.go`                        | CREATE | `runInstall`: Resolve+Detect, `priv.Prime`+`KeepAlive`, iterate `engine.Install`, failure summary       |
| `cmd/dotf/install_test.go`                   | CREATE | Dispatch + failure-summary behavior via `exec.Fake`                                                     |
| `cmd/dotf/status.go`                         | CREATE | `runStatus`+`runDiff` share `driftReport()`; read-only `engine.Installed`; greppable + sorted output     |
| `cmd/dotf/status_test.go`                    | CREATE | `status`/`diff` output + zero-mutation assertion (`Fake` records no install/write argv)                  |
| `cmd/dotf/main.go`                           | UPDATE | Add `case "install"/"status"/"diff"` to the switch; update `usage`                                      |
| `cmd/dotf/main_test.go`                      | UPDATE | Dispatch tests for the three new subcommands                                                            |

## NOT Building

- **No package removal / reconcile / `--prune`** — additive-install + read-only drift only (PRD L29, L93, L125). `status`/`diff` never propose or perform a removal.
- **No secrets orchestration** — 1Password/Bitwarden wiring deferred (PRD L28). `dotenvx` may still be an ordinary manifest package.
- **No full port of all 25 scripts** — Phase 3 ships the engine + one representative `[[package]]` per backend proving each path (PRD L178). The exhaustive script→data migration is a follow-on data task gated behind the engine.
- **No rpm-repo backend variant** — `install-azure-cli.sh` Fedora uses `rpm --import` + prod `.rpm`, which the 7-backend taxonomy has no slot for; azure Debian (`aka.ms | bash`) maps to `script-url`, aws (vendor zip) to `script-url`/`custom-script`. Only Debian keyring apt-repos (hashicorp/docker) use the `apt-repo` backend. rpm-repo deferred.
- **No native re-implementation of vendor installers** — `script-url`/`custom-script` stay data-driven shell-outs (the shrinking escape hatch), not Go rewrites (PRD L82, L120).
- **No streaming/live-progress renderer, snapper, failure-summary flags, or `--headless`/`--force` parity** — those are Phase 6. Phase 3 adds only the `Run` seam and per-package pass/fail accumulation those phases build on.
- **No sysconfig/symlink/firewall execution** — Phases 4/5. Phase 3 only produces the shared `internal/privilege` those phases consume.
- **No plugin/registry/reflection abstraction** — 7 backends are a closed enum; a small interface + a static dispatch map is sufficient (no `init()` self-registration).
- **No `testscript`/dockertest dependency** — the established `exec.Fake.Calls` argv + `testdata/*.toml` pattern meets the "exact argv per (distro × backend)" signal; dockertest matrix is deferred to nightly CI.
- **No changes to `bootstrap.sh` / `bin/install-*.sh`** — parallel-then-cutover; the bash system stays authoritative and runnable throughout Phase 3.

---

## Step-by-Step Tasks

> Backends are structs that hold their injected dependencies (`exec.Runner`, and `*privilege.Sudo`/`privilege.Dropper` where needed) via constructors; the `Engine` wires them in `register.go`. This keeps `Install(pkg, info)` clean and matches the seam-injection idiom (`platform.Detect(r)`, `manifest.Resolve(...)`).

### Task 1.1: Extend the exec seam with `Run` — Depends on [none]

- **BATCH**: B1
- **ACTION**: Add a `Run(name string, args ...string) error` method to `internal/exec` `Runner`, `osRunner`, and `Fake`.
- **IMPLEMENT**: `osRunner.Run` builds `osexec.Command(name, args...)`, wires `cmd.Stdin/Stdout/Stderr = os.Stdin/os.Stdout/os.Stderr` (needed for the interactive `sudo -v` prompt and live install output), and returns `cmd.Run()`. `Fake.Run` appends `FakeCall{Name,Args}` to `f.Calls` and returns a canned error from a new `RunErrors map[string]error` (nil when unregistered = success). This is **additive** — `Output`/`LookPath` are unchanged, so `platform`, `tui`, and `cmd/dotf` keep compiling.
- **IMPLEMENT (why)**: `Output` buffers stdout and detaches the TTY, so it can neither prompt for a sudo password nor stream a long install; `Run` is the streamed/interactive counterpart.
- **MIRROR**: SERVICE_PATTERN (`internal/exec/exec.go:21-42`, `fake.go:81-100`).
- **IMPORTS**: `os`, `osexec "os/exec"` (already present).
- **GOTCHA**: Keep the `var _ Runner = (*Fake)(nil)` and `var _ Runner = osRunner{}` assertions passing — the interface grew, both impls must satisfy it. Do NOT change `Output`'s signature.
- **VALIDATE**: `go build ./... && go test ./internal/exec/ -run Run` — argv recorded in `Calls`, canned error via `errors.Is`.

### Task 1.2: Add `Arch.ReleaseArch()` — Depends on [none]

- **BATCH**: B1
- **ACTION**: Add `func (a Arch) ReleaseArch() string` to `internal/platform/types.go`.
- **IMPLEMENT**: Map `ArchX86_64 → "amd64"`, `ArchARM64 → "arm64"`, `ArchARM → "arm"`, default → `string(a)` (passthrough for unrecognized). This backs github-release asset templating (`{{.Arch}}` → `amd64`), the reservation documented at types.go:34-37.
- **MIRROR**: NAMING_CONVENTION (typed-string method on `Arch`, `internal/platform/types.go:38-58`).
- **IMPORTS**: none new.
- **GOTCHA**: Do NOT rename or "fix" the existing `ArchX86_64` const (the underscore is intentional, mirrors raw `x86_64`). `ReleaseArch` is a new mapper, not a value change.
- **VALIDATE**: `go test ./internal/platform/ -run ReleaseArch` — table asserts each mapping incl. passthrough.

### Task 1.3: Extend the manifest schema for apt-repo — Depends on [none]

- **BATCH**: B1
- **ACTION**: Add a nested `AptRepo` struct and a `Package.AptRepo *AptRepo` field; add validation; add fixtures + one data example.
- **IMPLEMENT**: `type AptRepo struct { KeyURL, Keyring, RepoLine, SourcesPath string }` with toml tags (`key_url`,`keyring`,`repo_line`,`sources_path`). In `validate.go`'s `case BackendAPTRepo`, require `AptRepo` non-nil with all four fields set (grounded in `bin/install-hashicorp-tools.sh:120-150`). Add a valid apt-repo package to `testdata/valid.toml`, create `invalid_apt_repo_missing_field.toml`, register it in `manifest_test.go`, and add one representative apt-repo `[[package]]` to `manifest/packages.toml`. **Additive only** — no frozen field renamed/removed.
- **MIRROR**: ERROR_HANDLING (`internal/manifest/validate.go:115-145` per-backend `switch` + `ValidationError`).
- **IMPORTS**: none new.
- **GOTCHA**: `Package` is a superset with a frozen field set (Phase 2 contract: additions safe, renames/removals forbidden). Add fields; never reorder/remove. Keep the `sha256Pattern` and `knownPkgMgrs` checks intact.
- **VALIDATE**: `go test ./internal/manifest/` — valid.toml round-trips, the new invalid fixture yields `*ValidationError`.

### Task 1.4: Backend interface + Engine dispatch — Depends on [none]

- **BATCH**: B1
- **ACTION**: Create `internal/backend/backend.go` with the `Backend` interface, the `Engine` dispatcher, and typed errors.
- **IMPLEMENT**: `type Backend interface { Install(pkg manifest.Package, info platform.Info) error; Installed(pkg manifest.Package, info platform.Info) (bool, error) }`. `type Engine struct { backends map[manifest.Backend]Backend }` with `NewEngine(backends map[manifest.Backend]Backend) *Engine`, and `Install`/`Installed` methods that look up `pkg.Backend`, returning a typed `*UnknownBackendError` when absent. Defines only the interface + dispatch — NO concrete backend references (those are wired in `register.go`, Task 4.1), so this file compiles standalone against `manifest`+`platform`.
- **MIRROR**: NAMING_CONVENTION (`internal/manifest/types.go:100-141` enum-keyed dispatch) + ERROR_HANDLING (typed error struct).
- **IMPORTS**: `github.com/yandy-r/dotfiles/internal/manifest`, `.../internal/platform`, `fmt`.
- **GOTCHA**: Do NOT import `internal/privilege` or reference concrete backends here — that would couple the interface file to Task 2.1/Batch 3 and break B1 compilation. Keep dispatch data-driven off the map.
- **VALIDATE**: `go build ./internal/backend/ && go test ./internal/backend/ -run Engine` — unknown backend returns the typed error.

### Task 1.5: Shared download/verify/extract helper — Depends on [none]

- **BATCH**: B1
- **ACTION**: Create `internal/backend/download.go` (+ `download_test.go`) — the stdlib fetch+verify+extract helper reused by github-release and script-url.
- **IMPLEMENT**: `fetchVerifyExtract(url, wantSHA256, destDir string) error`: `http.Get` (follows the GitHub 302), stream to a `0600` temp file through `io.TeeReader(resp.Body, sha256.New())`, `crypto/subtle.ConstantTimeCompare` the hex digest vs `wantSHA256` (skip only if `wantSHA256==""`), then branch extract — `.tar.gz` via `gzip`+`tar` (stream), `.zip` via buffered `zip.NewReader`. Sanitize each entry name with `filepath.Clean`+destDir-prefix check (reject `..`/absolute); `chmod 0o755` extracted binaries.
- **MIRROR**: ERROR_HANDLING (verify-before-execute gate, `bin/install-claude.sh:405-435`).
- **IMPORTS**: `net/http`, `crypto/sha256`, `crypto/subtle`, `encoding/hex`, `archive/tar`, `archive/zip`, `compress/gzip`, `io`, `os`, `path/filepath`.
- **GOTCHA**: After extraction, `io.Copy(io.Discard, tee)` so the hasher consumes every byte before comparison (gzip/tar may stop before EOF). `zip` cannot stream — buffer to temp first. No new go.mod dependency (stdlib only).
- **VALIDATE**: `go test ./internal/backend/ -run Download` — `httptest.Server` covers match, mismatch (aborts, no extract), tar.gz + zip, and a traversal entry rejection.

### Task 2.1: Privilege model (sudo keep-alive + drop) — Depends on [1.1]

- **BATCH**: B2
- **ACTION**: Create `internal/privilege` — `Sudo` (prime + keep-alive + prefix + real-user resolution) and `Dropper` (credential drop for AUR), with unix/non-unix split and a fake.
- **IMPLEMENT**: `Sudo.Prime(r exec.Runner) error` = `r.Run("sudo","-v")` (interactive, once). `KeepAlive(ctx, r) func()` starts a goroutine ticking `r.Run("sudo","-n","-v")` on a `<5m` `time.Ticker`, stopping on `ctx.Done()` and returning a cancel func; surface the first `sudo -n -v` failure. `RealUser()` resolves `$SUDO_USER` (+ numeric `$SUDO_UID`/`$SUDO_GID`), erroring when running as root with `SUDO_USER` empty/`root`. `drop.go` (`//go:build unix`): `Dropper.Run(argv []string, home string) error` builds `osexec.Cmd` with `SysProcAttr.Credential{Uid,Gid,NoSetGroups:true}` and explicit `Env` (`HOME`,`USER`,`LOGNAME`, sanitized `PATH`), asserting `Uid != 0` before spawn; active only when `os.Geteuid()==0`, else runs directly. `drop_other.go` (`//go:build !unix`): stub returning an unsupported-platform error. `fake.go`: records dropped argv/uid.
- **MIRROR**: ERROR_HANDLING (typed errors + fail-fast) + SERVICE_PATTERN (Runner injection).
- **IMPORTS**: `context`, `os`, `os/user`, `osexec "os/exec"`, `strconv`, `syscall`, `time`, `github.com/yandy-r/dotfiles/internal/exec`.
- **GOTCHA**: Never inherit root's env into the dropped child (`HOME=/root` breaks makepkg/gpg) — set `Cmd.Env` explicitly. Stop the keep-alive ticker on both success and error paths (no goroutine leak). Never capture/log the password — rely solely on the `sudo` timestamp. `syscall.Credential` is unix-only → keep it behind the build tag.
- **VALIDATE**: `go test ./internal/privilege/` — Prime emits `sudo -v`; KeepAlive emits `sudo -n -v` then stops; `RealUser` set/unset; drop no-ops when euid≠0 and refuses `Uid==0`.

### Task 3.1: `pkg` backend — Depends on [1.1, 1.4]

- **BATCH**: B3
- **ACTION**: Create `internal/backend/pkg.go` (+ test) dispatching the host package manager.
- **IMPLEMENT**: Resolve the manager-specific name via `pkg.Names[string(info.PkgMgr)]`; build install argv per `info.PkgMgr` — `sudo pacman -S --noconfirm --needed <n>`, `sudo apt-get install -y <n>` (env `DEBIAN_FRONTEND=noninteractive`), `sudo dnf install -y <n>`, `sudo zypper --non-interactive install <n>`, `brew install <n>` (no sudo) — dispatched through `r.Run`. `Installed` uses read-only queries (`pacman -Q`, `dpkg -s`, `rpm -q`, `zypper se -i`, `brew list`) via `r.Output`/`r.LookPath`.
- **MIRROR**: REPOSITORY_PATTERN (`internal/platform/platform.go:187-192` LookPath probe) + TEST_STRUCTURE.
- **IMPORTS**: `internal/exec`, `internal/manifest`, `internal/platform`, `fmt`.
- **GOTCHA**: `brew` must never run under sudo. Keep the file under the ~500-line cap — if per-manager argv builders bloat it, split them into small helpers within the same file. Missing `Names[mgr]` → typed error, not a panic.
- **VALIDATE**: `go test ./internal/backend/ -run Pkg` — table asserts exact argv for each of the 5 managers via `Fake.Calls`.

### Task 3.2: `github-release` backend — Depends on [1.1, 1.2, 1.4, 1.5]

- **BATCH**: B3
- **ACTION**: Create `internal/backend/github_release.go` (+ test).
- **IMPLEMENT**: Substitute `{{.OS}}`/`{{.Arch}}`/version tokens in `pkg.Asset` using `info.OS` + `info.Arch.ReleaseArch()`; build the download URL as `https://github.com/<Repo>/releases/latest/download/<asset>` (no API call → no rate limit); delegate to `fetchVerifyExtract(url, pkg.SHA256, destDir)`; place the binary on `PATH`. `Installed` via `r.LookPath` of the resolved binary name.
- **MIRROR**: REPOSITORY_PATTERN (download helper) + TEST_STRUCTURE.
- **IMPORTS**: `internal/exec`, `internal/manifest`, `internal/platform`, `fmt`, `strings`.
- **GOTCHA**: Validate `Repo` matches `owner/repo` and URL-encode the asset before concatenating — never string-concat unvalidated manifest data into a URL/path. `SHA256` is required by the manifest validator for this backend; a mismatch must abort before any file is placed.
- **VALIDATE**: `go test ./internal/backend/ -run GitHubRelease` — asset templating per (os×arch) + a mismatched-sha aborts with zero install step.

### Task 3.3: `aur` backend — Depends on [1.1, 1.4, 2.1]

- **BATCH**: B3
- **ACTION**: Create `internal/backend/aur.go` (+ test) installing from the AUR, never as root.
- **IMPLEMENT**: If `yay` is absent (`r.LookPath`), bootstrap it: `git clone https://aur.archlinux.org/yay.git` then `makepkg -si` — run through `privilege.Dropper` (to `$SUDO_USER`, `NoSetGroups:true`, real `$HOME`). Otherwise `yay -S --noconfirm <name>` as the real user. `Installed` via `pacman -Q <name>`.
- **MIRROR**: SERVICE_PATTERN (privilege injection) + REPOSITORY_PATTERN (`pacman -Q`).
- **IMPORTS**: `internal/exec`, `internal/manifest`, `internal/platform`, `internal/privilege`, `fmt`.
- **GOTCHA**: makepkg/yay REFUSE to run as root — when `euid==0` you MUST drop to `$SUDO_USER`; if `SUDO_USER` is empty/`root`, error rather than run as root. Only meaningful on `info.OS == OSArch`; return a typed "unsupported on this OS" error elsewhere.
- **VALIDATE**: `go test ./internal/backend/ -run AUR` — asserts drop path used (never uid 0), yay bootstrap when absent, and OS guard.

### Task 3.4: `toolchain` backend — Depends on [1.1, 1.4]

- **BATCH**: B3
- **ACTION**: Create `internal/backend/toolchain.go` (+ test).
- **IMPLEMENT**: Map `pkg.Tool` to its command: `cargo` → `cargo install <name>`, `npm` → `npm install -g <name>` (extend the small map as needed), dispatched via `r.Run`. `Installed` via `r.LookPath` (or `cargo install --list`).
- **MIRROR**: SERVICE_PATTERN + TEST_STRUCTURE.
- **IMPORTS**: `internal/exec`, `internal/manifest`, `internal/platform`, `fmt`.
- **GOTCHA**: Toolchain installs run as the user, never sudo. Unknown `Tool` → typed error. The `Name` used for the toolchain command may differ from `pkg.Name`; use `Names`/`Name` consistently and document which.
- **VALIDATE**: `go test ./internal/backend/ -run Toolchain` — exact argv for cargo/npm.

### Task 3.5: `apt-repo` backend — Depends on [1.1, 1.3, 1.4]

- **BATCH**: B3
- **ACTION**: Create `internal/backend/apt_repo.go` (+ test).
- **IMPLEMENT**: From `pkg.AptRepo`: fetch `KeyURL`, `gpg --dearmor` into `Keyring`, write `RepoLine` (`deb [signed-by=<Keyring>] … <codename> main`) to `SourcesPath`, `sudo apt-get update`, then `sudo apt-get install -y <Names["apt"]>` — all through `r.Run`. `Installed` via `dpkg -s`.
- **MIRROR**: ERROR_HANDLING + REPOSITORY_PATTERN, grounded in `bin/install-hashicorp-tools.sh:120-150`.
- **IMPORTS**: `internal/exec`, `internal/manifest`, `internal/platform`, `fmt`.
- **GOTCHA**: Debian/Ubuntu only — guard on `info.OS`/`info.PkgMgr == apt` and return a typed unsupported error elsewhere. The codename must come from the host, not be hard-coded. Do NOT attempt rpm-repo here (out of scope).
- **VALIDATE**: `go test ./internal/backend/ -run AptRepo` — asserts the keyring→sources→update→install argv sequence.

### Task 3.6: `script-url` backend — Depends on [1.1, 1.4, 1.5]

- **BATCH**: B3
- **ACTION**: Create `internal/backend/script_url.go` (+ test).
- **IMPLEMENT**: Download `pkg.URL` (validator guarantees https + sha256), verify via the `download.go` helper, then execute the fetched installer through `r.Run`. `Installed` via `r.LookPath`.
- **MIRROR**: REPOSITORY_PATTERN (download helper) + ERROR_HANDLING (checksum gate).
- **IMPORTS**: `internal/exec`, `internal/manifest`, `internal/platform`, `fmt`.
- **GOTCHA**: Keep sha256 verification mandatory for script-url (the manifest validator already requires it — do NOT relax to match the PRD's "optional pinned" wording). Never pipe an unverified `curl | bash`.
- **VALIDATE**: `go test ./internal/backend/ -run ScriptURL` — verified-then-execute; mismatch aborts before execute.

### Task 3.7: `custom-script` backend — Depends on [1.1, 1.4]

- **BATCH**: B3
- **ACTION**: Create `internal/backend/custom_script.go` (+ test).
- **IMPLEMENT**: Resolve `pkg.Script` relative to the repo root, reject absolute paths and `..` traversal (mirror `validate.go:178-188`), then execute via `r.Run`. `Installed` via `r.LookPath`. The shrinking escape hatch for scripts not yet expressible as data.
- **MIRROR**: ERROR_HANDLING (path-safety guard, `internal/manifest/validate.go:178-188`).
- **IMPORTS**: `internal/exec`, `internal/manifest`, `internal/platform`, `path/filepath`, `fmt`.
- **GOTCHA**: `Script` currently passes only a non-empty check in validation — this backend MUST re-validate containment inside the repo before shelling out. Do NOT interpolate `Script` into a shell string.
- **VALIDATE**: `go test ./internal/backend/ -run CustomScript` — rejects abs/`..`, executes a repo-relative script.

### Task 4.1: Engine registration (`Default`) — Depends on [3.1, 3.2, 3.3, 3.4, 3.5, 3.6, 3.7, 2.1]

- **BATCH**: B4
- **ACTION**: Create `internal/backend/register.go` wiring all 7 concrete backends into the dispatch map.
- **IMPLEMENT**: `func Default(r exec.Runner, sudo *privilege.Sudo, drop privilege.Dropper) *Engine` constructs each backend (injecting `r`, and `sudo`/`drop` for `aur`) and returns `NewEngine(map[manifest.Backend]Backend{ BackendPkg: newPkg(r), … BackendCustomScript: newCustomScript(r) })`. This is the only file coupling the interface to concrete backends + privilege.
- **MIRROR**: NAMING_CONVENTION (`New`/constructor pattern) + SERVICE_PATTERN (dependency injection).
- **IMPORTS**: `internal/exec`, `internal/manifest`, `internal/privilege`.
- **GOTCHA**: Every one of the 7 `Backend` consts must have a map entry or dispatch will return `UnknownBackendError` at runtime — add a test asserting the map covers all `Backend.Valid()` values.
- **VALIDATE**: `go test ./internal/backend/ -run Default` — map has all 7 backends; `go build ./...`.

### Task 5.1: `dotf install` subcommand — Depends on [4.1]

- **BATCH**: B5
- **ACTION**: Create `cmd/dotf/install.go` (+ test) — `runInstall(args, stdout, stderr, r)`.
- **IMPLEMENT**: Own `flag.FlagSet` with `--manifest` and `--dry-run` (mirror `runPlan`). `manifest.Resolve(...)` + `platform.Detect(r)`; construct `privilege.Sudo`, `priv.Prime(r)` + `cancel := priv.KeepAlive(ctx, r)` (deferred cancel); `backend.Default(r, sudo, drop)`; iterate the selected packages calling `engine.Install`, accumulating failures non-fatally and printing a per-backend result line + an end-of-run summary. On `--dry-run`, print the plan without executing.
- **MIRROR**: SERVICE_PATTERN (`cmd/dotf/plan.go:22-33` runX + Resolve/Detect).
- **IMPORTS**: `dotfiles`, `internal/exec`, `internal/manifest`, `internal/platform`, `internal/backend`, `internal/privilege`, `context`, `flag`, `io`, `os`.
- **GOTCHA**: Always `defer cancel()` the keep-alive so the ticker never outlives the run. Collect failures instead of aborting on the first (parity with `BOOTSTRAP_FAILURES`); return non-zero exit only if ≥1 failed.
- **VALIDATE**: `go test ./cmd/dotf/ -run Install` — one sudo prime, per-package dispatch, failure summary, non-zero exit on failure.

### Task 5.2: `dotf status` / `dotf diff` drift report — Depends on [4.1]

- **BATCH**: B5
- **ACTION**: Create `cmd/dotf/status.go` (+ test) — `runStatus` and `runDiff` sharing a `driftReport` helper.
- **IMPLEMENT**: `driftReport(stdout, m, engine, info)` enumerates `m.Packages`, calls `engine.Installed(pkg, info)` for each (READ-ONLY), and prints sorted greppable lines. `runStatus` prints every package with `installed=true|false`; `runDiff` prints only `declared-not-installed` drift. Additive-only — never emit or perform a removal.
- **MIRROR**: REPOSITORY_PATTERN (`engine.Installed`) + greppable stdout (`cmd/dotf/plan.go:88-102`).
- **IMPORTS**: `dotfiles`, `internal/exec`, `internal/manifest`, `internal/platform`, `internal/backend`, `flag`, `io`, `os`, `sort`.
- **GOTCHA**: `status`/`diff` must invoke ZERO mutating commands — assert in the test that the `Fake` records only query/`LookPath` argv, never an install/write. Sort output for deterministic goldens.
- **VALIDATE**: `go test ./cmd/dotf/ -run "Status|Diff"` — read-only assertion + correct drift set.

### Task 6.1: Wire subcommands into `main` — Depends on [5.1, 5.2]

- **BATCH**: B6
- **ACTION**: Update `cmd/dotf/main.go` (switch + `usage`) and `cmd/dotf/main_test.go` (dispatch tests).
- **IMPLEMENT**: Add `case "install": return runInstall(fs.Args()[1:], os.Stdout, os.Stderr, exec.New())` and the `status`/`diff` equivalents to the `switch` at main.go:39-51; update the `usage` const to list them. Add dispatch tests mirroring `TestRun_Platform`.
- **MIRROR**: SERVICE_PATTERN (`cmd/dotf/main.go:39-51` dispatch).
- **IMPORTS**: none new (functions already in package `main`).
- **GOTCHA**: This is the only task touching `main.go`/`main_test.go` — keep it in its own batch after 5.1/5.2 so the referenced `runInstall`/`runStatus`/`runDiff` exist. Update `usage` or the unknown-subcommand help drifts.
- **VALIDATE**: `go test ./cmd/dotf/ && go vet ./... && go build ./...` — all three subcommands dispatch.

---

## Testing Strategy

### Unit Tests

| Test                          | Input                                          | Expected Output                                        | Edge Case? |
| ----------------------------- | ---------------------------------------------- | ------------------------------------------------------ | ---------- |
| pkg argv per manager          | pkg package + each `info.PkgMgr`               | exact `Fake.Calls` argv (`pacman -S --noconfirm`, …)   | No         |
| pkg brew never sudo           | pkg package, `PkgMgr=homebrew`                  | argv[0] == `brew`, no `sudo` prefix                    | Yes        |
| github asset templating       | Repo/Asset with `{{.OS}}/{{.Arch}}`, x86_64/arm64 | resolved URL uses `amd64`/`arm64`                    | No         |
| github sha256 mismatch        | asset whose digest ≠ `SHA256`                  | error, and NO install/place step recorded              | Yes        |
| download traversal rejection  | tar entry named `../evil`                      | error, nothing written outside destDir                 | Yes        |
| aur drop, never root          | aur package, `euid=0`, `SUDO_USER=yandy`       | dropped argv recorded, uid ≠ 0                          | Yes        |
| aur missing SUDO_USER as root | aur package, `euid=0`, `SUDO_USER=""`          | typed error, zero exec calls                            | Yes        |
| sudo single prime             | install run with N privileged pkgs             | exactly one `sudo -v`; keep-alive `sudo -n -v`; cancel | Yes        |
| apt-repo sequence             | apt-repo package on apt host                    | keyring → sources → `apt-get update` → install argv    | No         |
| script-url verify-then-exec   | url + matching sha256                          | verify precedes execute; mismatch aborts               | Yes        |
| custom-script containment     | `Script` with `..`                             | rejected before shell-out                              | Yes        |
| drift read-only               | manifest vs mixed installed state              | correct `status`/`diff`; Fake records no mutation      | Yes        |
| engine unknown backend        | package with unmapped backend                   | `*UnknownBackendError`                                 | Yes        |

### Edge Cases Checklist

- [ ] Empty `Names[mgr]` for the host's package manager (typed error, no panic)
- [ ] Unrecognized `Arch` falls through `ReleaseArch()` (passthrough, not empty)
- [ ] `.zip` asset (buffered) vs `.tar.gz` asset (streamed) both extract
- [ ] Checksum mismatch aborts before any file is placed / executed
- [ ] Running without sudo (`euid≠0`) — privileged backends still function or fail cleanly
- [ ] Keep-alive goroutine stops on both success and error (no leak)
- [ ] Backend invoked on the wrong OS (aur off-Arch, apt-repo off-Debian) → typed unsupported error
- [ ] `status`/`diff` invoke zero mutating commands

---

## Validation Commands

### Static Analysis

```bash
go build ./... && go vet ./...
```

EXPECT: Zero build/vet errors.

### Unit Tests

```bash
go test -race ./internal/backend/ ./internal/privilege/ ./internal/exec/ ./cmd/dotf/
```

EXPECT: All tests pass, including exact-argv assertions per (distro × backend).

### Full Test Suite

```bash
go test -race ./...
```

EXPECT: No regressions across manifest/platform/tui/exec/version.

### Lint

```bash
golangci-lint run ./...   # or: bin/lint.sh --go
gofmt -l . ; test -z "$(gofmt -l .)"
```

EXPECT: Clean (v2 `default: standard` + `revive`; `_test.go` exempt from errcheck).

### Bash-untouched guard

```bash
git diff --stat -- bootstrap.sh 'bin/install-*.sh'
```

EXPECT: empty — the bash bootstrap system is unchanged (parallel-then-cutover).

### Manual Validation

- [ ] `go run ./cmd/dotf status` — prints installed/missing per package, mutates nothing.
- [ ] `go run ./cmd/dotf diff` — prints only drift; exits 0; system untouched.
- [ ] `go run ./cmd/dotf install --dry-run` — prints plan without executing.
- [ ] On a scratch VM: `dotf install` a single representative tool per backend succeeds (Phase 3 VM signal).

---

## Acceptance Criteria

- [ ] All 17 tasks completed
- [ ] All validation commands pass
- [ ] For every (distro × backend), a table-driven test asserts the EXACT argv (`FakeCall.Name`+`Args`) — no substring/wildcard matches (#7; Success Metric)
- [ ] All 7 backends implemented one file each under `internal/backend/`, dispatched purely from `pkg.Backend` (no per-tool `switch`) (#7)
- [ ] Adding a tool = 1 new `[[package]]` entry + 0 `.go` control-flow edits (Success Metric)
- [ ] `github-release` templates the asset (os/arch), verifies `SHA256`, and extracts; a mismatch aborts before placing the binary (#7)
- [ ] `internal/privilege`: a full run primes sudo at most once; keep-alive holds the timestamp; goroutine stops on exit (#8)
- [ ] `aur` runs `yay`/`makepkg` as `$SUDO_USER` (never uid 0) via `NoSetGroups:true` + explicit `$HOME`; errors if `SUDO_USER` unset while root (#8)
- [ ] Privileged commands escalate per-command; process default stays unprivileged (#8)
- [ ] `dotf status`/`dotf diff` print installed-vs-manifest deltas with zero mutation; read-only/additive-only — never a removal (#9)
- [ ] Drift report reuses the engine's `Installed` enumeration, not a parallel probe (#9)
- [ ] One representative `[[package]]` per backend present in `manifest/packages.toml`; the rest deferred (NOT Building)
- [ ] No type errors, no lint errors
- [ ] Matches UX design (install/status/diff subcommands, single sudo prompt)

## Completion Checklist

- [ ] Code follows discovered patterns (Runner injection, typed errors, enum dispatch)
- [ ] Error handling matches codebase style (`*ValidationError`-like typed errors, `errors.Join`, `%w` wrapping, fail-fast)
- [ ] Tests follow the table-driven + `exec.Fake.Calls` pattern; goldens in `testdata/`
- [ ] No hardcoded values (codename, arch, asset resolved from `platform.Info`/manifest)
- [ ] `go build ./... && go vet ./... && go test -race ./...` all green
- [ ] `golangci-lint run ./...` clean; `gofmt -l .` empty
- [ ] `bin/style.sh lint --go` clean; lefthook pre-commit passes on staged Go
- [ ] Bash untouched: `git diff` shows no change to `bootstrap.sh`/`bin/install-*.sh`; bootstrap still runnable
- [ ] `internal/manifest/types.go` + `manifest/packages.toml` changed additively only (no rename/removal of frozen fields or the 7 `Backend` values)
- [ ] apt-repo schema decision recorded; aws/azure/rpm-repo reconciled or deferred in NOT Building
- [ ] Forgejo #7/#8/#9 acceptance criteria each mapped to a task (#7 → 1.2–1.5, 3.1–3.7, 4.1; #8 → 1.1, 2.1, 3.3; #9 → 5.2)
- [ ] `internal/privilege` declared the single shared package (consumed by #7 + Phase 5); makepkg/yay proven never root (tested)
- [ ] Drift proven read-only: `status`/`diff` test asserts `Fake` runs zero install/write commands
- [ ] `Runner` extension is additive: `Output`/`LookPath` unchanged, `Fake` updated, `platform`/`tui`/`cmd/dotf` still build
- [ ] Self-contained — no questions needed during implementation

## Risks

| Risk                                                                                          | Likelihood | Impact | Mitigation                                                                                                       |
| --------------------------------------------------------------------------------------------- | ---------- | ------ | --------------------------------------------------------------------------------------------------------------- |
| makepkg/yay run as root (refuses, or builds in `/root` with leaked env)                        | Med        | High   | Drop via `Credential{NoSetGroups:true}` from numeric `SUDO_UID`/`SUDO_GID`; assert `Uid != 0`; explicit `Cmd.Env` (`HOME`/`USER`), never inherit root env |
| `SUDO_USER` absent/spoofed → drop target wrong or root                                         | Med        | High   | Fail fast for aur when `SUDO_USER` empty/`root`; prefer numeric ids; treat as untrusted input                    |
| Remote asset executed without checksum gate (github-release/script-url)                         | Low        | High   | `download.go` verifies sha256 (constant-time) BEFORE extract/execute; validator already requires sha256; never relax |
| `exec.Runner` extension (`Run`) subtly breaks existing `Output`/`LookPath` consumers            | Low        | Med    | Additive only — new method, unchanged signatures; `var _ Runner` assertions; `go build ./...` gate               |
| apt-repo schema wrong for hashicorp/aws/azure                                                   | Med        | Med    | Line-read `install-hashicorp-tools.sh:120-150`; only Debian keyring repos use apt-repo; aws/azure→script-url; rpm-repo deferred |
| Keep-alive goroutine leaks past run end                                                        | Med        | Med    | `ctx`-scoped ticker + returned cancel; `defer cancel()` in `runInstall`; test proves it stops                    |
| Phase 3 accidentally touches/breaks the live bash bootstrap                                     | Low        | High   | Add-only Go; `git diff` guard on `bootstrap.sh`/`bin/install-*.sh`; completion-checklist gate                    |
| `internal/privilege` also consumed by Phase 5 (sysconfig) → surface churn                       | Med        | Med    | Declare it Phase-3-owned with a stable public surface; Phase 5 depends on it (correct the PRD dependency table)  |
| "18 scripts → data" misread as 18 packages when one script fans to several backends            | Med        | Low    | Scope by backend coverage (7 worked examples), not script count; full port deferred (NOT Building)               |

## Notes

- **Greenfield inside the Go tree**: `internal/backend/` and `internal/privilege/` do not exist yet — this is net-new, not a bash port. The sudo keep-alive + `$SUDO_USER` drop has no in-repo Go precedent; follow the platform/exec idioms + the three line-read installers.
- **The pivotal design decision is the exec seam.** `exec.Runner` (exec.go:21-29) exposes only buffered `Output`+`LookPath`. That cannot (a) prompt for a sudo password on a TTY, nor (b) stream a long install. Task 1.1 adds an **additive** `Run` method (stdio attached). The privilege *drop* (credential + custom env) does NOT fit the generic seam, so it lives in a narrow `privilege.Dropper` (unix-gated) with its own fake. Do not try to force credential-drop through `exec.Runner`.
- **github-release version**: v1 uses the `releases/latest/download/<asset>` redirect (no GitHub API call → no 60/hr unauth rate limit). A pinned tag/version field can be added later; not required now.
- **Scope discipline**: this phase delivers the engine + one representative `[[package]]` per backend, not a migration of all 25 scripts. That framing is deliberate (PRD L178) — the full data port is a follow-on task gated behind a working engine.
- **Dependency-table correction**: the PRD lists Phase 5 as depending only on Phase 1, but sysconfig will consume `internal/privilege` produced here. Treat `internal/privilege` as Phase-3-owned with a stable surface; Phase 5 should depend on it.
- **Confidence Score: 8/10.** High on the backend taxonomy, exec/fake test harness, and privilege mechanics (all grounded in existing code + the three line-read installers). The −2 is residual: the exact per-script→backend mapping beyond the representative sample, and whether `Run` (vs a richer streaming API) fully satisfies real installs, are best confirmed during implementation of the first backend.
- **Research backstops**: `docs/prps/plans/.prp-research/provisioning-cli-install-engine/{api,business-analyzer,tech-designer,ux,security,practices,recommendations-agent}.md`.

