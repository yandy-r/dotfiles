# Testing Guide

Verification in this repository is three-tiered: **automated CI** on every pull request, a **local container
rehearsal** run by hand before opening one, and **physical-device acceptance** walked end to end on the target
machine. The third tier is a **gate**, not a post-hoc report — the device checklist is walked before a release tag is
cut, and a claim that only the device can settle stays unproven until it is.

Each tier exists because the one below it cannot reach the claim. Do not treat a green CI run as evidence for
anything in Tier 2 or Tier 3.

## Tier 1 — Automated (every PR)

### Go

The Go gate is four commands. CI runs the last three; `go build` is what you run locally first.

```bash
go build ./...
go vet ./...
go test -race ./...
golangci-lint run ./...
```

`golangci-lint` is pinned to **v2.12.2** in both Go workflows — install that exact version locally, because a newer
release will surface findings CI does not have and an older one will miss findings CI does.

`internal/backend/route_matrix_test.go` checks the real manifest offline. Each package family with two or more
entries that `--only <name>` expands (`<name>` plus any `manifest.PlatformSiblingSuffixes` sibling) must
have exactly one applicable route on every {debian, redhat, arch, macos} × {x86_64, arm64} host. A
host with no route must be listed in `unsupportedRoutes` with a reason. A gate edit that adds,
drops or collides a route fails the test until that list changes in the same diff.

There is **no `npm test` script and no `npm run build` script** in `package.json`; do not prescribe them. The npm
aliases that do exist are thin wrappers and are not identical to what CI runs:

| Script             | Actual command                | Difference from CI                              |
| ------------------ | ----------------------------- | ----------------------------------------------- |
| `npm run go:build` | `go build -o dotf ./cmd/dotf` | Builds only the CLI binary, not every package   |
| `npm run go:vet`   | `go vet ./...`                | None                                            |
| `npm run go:test`  | `go test ./...`               | **No `-race`** — CI's race detector is stricter |

### Lint bundle

`bin/style.sh` is the single style dispatcher; `bin/lint.sh` and `bin/format.sh` are wrappers over it.

```bash
npm run lint          # bin/lint.sh --python --shell --go
npm run lint:fix
```

Three mutually exclusive git-scope flags narrow the file set: `--modified` (staged + unstaged + untracked),
`--staged` (exactly what `git commit` will record), and `--unstaged` (dirty but not yet staged). Omitting all three
runs against the full repo tree. `npm run lint` is what `pre-push` runs.

The lefthook `pre-commit` hook runs `bin/lint.sh --staged --shell --python --go --docs`. Note the trailing `--docs`:
`npm run lint:staged` is otherwise identical but **omits** it, so the npm script never invokes markdownlint. To
reproduce the hook exactly before committing, run both:

```bash
npm run lint:staged      # shell + python + go
npm run lint:md:staged   # the --docs half the hook adds
```

### Markdown

```bash
npm run lint:md       # bin/lint.sh --docs
npm run format:md     # markdownlint-cli2 --fix, then prettier --write
```

`--docs` is **opt-in**. It is excluded from the no-flag default _and_ from `--all`, deliberately, so that
`npm run lint` and the pre-push hook stay green against the repository's existing markdown backlog. The pre-commit
hook lints **staged** Markdown only, for the same reason. A new or heavily edited Markdown file should nonetheless
pass both `markdownlint-cli2` and `prettier --check` before it is committed.

### Shell smoke

```bash
bash -l -c 'echo "Bash config loaded successfully"'
zsh  -l -c 'echo "Zsh config loaded successfully"'
dotf link --dry-run
```

### Shell regression suites

Two hermetic suites cover the shell config; neither depends on the host OS:

```bash
bash scripts/test-shell-startup-smoke.sh   # rc syntax, quiet startup, login DOTFILES_OS, dash via ~/.profile, startup time budget
bats tests/shell                           # platform detection, PATH cleanup, rc reload guards
```

The smoke driver runs every shell under `env -i` with a temp `HOME` (rc files
symlinked from the checkout, fake no-op tools on `PATH`), so a failure is
always a dotfiles regression and never host noise. The dash case
(skipped when dash is absent) keeps `bash/profile` POSIX, since `/bin/sh`
login shells read it as `~/.profile`; its `dash -l` check is compared against
an empty-`HOME` baseline so host `/etc/profile` output cancels out. `TEST_BASH=/bin/bash`
points the bash under test at another binary; `SHELL_STARTUP_BUDGET_MS`
(default 1000) and `SHELL_STARTUP_RUNS` (default 5) tune the time budget.

bats-core is not vendored: install the pinned release locally under `/tmp`
(the same commands CI runs) with
`git clone --depth 1 --branch v1.11.1 https://github.com/bats-core/bats-core /tmp/bats-core && /tmp/bats-core/install.sh /tmp/bats`
and run `/tmp/bats/bin/bats tests/shell`. The bats cases drive platform
behaviour through `DOTFILES_OS` and the `DOTFILES_OS_RELEASE_FILE` /
`DOTFILES_PROC_VERSION_FILE` seams in `common/platform.sh`, so they never
read the host's `/etc/os-release`.

### CI jobs

Every CI edit is a **paired** edit: Forgejo reads workflows from `.forgejo/workflows/` in preference to
`.github/workflows/` and never merges the two directories, so each workflow exists twice and the pair must stay in
sync.

| Workflow                       | Jobs                       | Path filters                                                                                                                            |
| ------------------------------ | -------------------------- | --------------------------------------------------------------------------------------------------------------------------------------- |
| `.github/workflows/go.yml`     | `test`, `fedora`           | `**.go`, `go.mod`, `go.sum`, `manifest/**`, `bootstrap.sh`, `.goreleaser.yml`, `.goreleaser.forgejo.yml`, its own path, `.golangci.yml` |
| `.forgejo/workflows/go.yml`    | `test`, `fedora`           | Identical, with `.forgejo/workflows/go.yml` in place of the `.github` path                                                              |
| `.github/workflows/shell.yml`  | `shellcheck`, `regression` | `**.sh`, `common/**`, `bin/**`, `bash/**`, `zsh/**`, `shellcheck/**`, `tests/**`, its own path                                          |
| `.forgejo/workflows/shell.yml` | `shellcheck`, `regression` | Identical, with `.forgejo/workflows/shell.yml` in place of the `.github` path                                                           |

The Forgejo twin **cannot use third-party actions**: only `actions/*` and `forgejo/*` resolve from its
`DEFAULT_ACTIONS_URL` (`data.forgejo.org`). That is why `golangci/golangci-lint-action` is replaced there by a raw
shell install of the pinned upstream `install.sh`, and why the Forgejo `fedora` job installs one extra package —
`nodejs` — since `act_runner` needs node _inside_ the container image before any JavaScript action can run.

#### The Fedora container job

The `fedora` job runs on `ubuntu-latest` inside `container: fedora:latest`. It is **the only thing in the repository
that ever executes against a real Fedora userspace** — a real `/etc/os-release`, real dnf5 and a real rpm database.
Every other Fedora assertion in the tree (detection, manifest applicability, backend argv, sysconfig planning) runs
against `exec.Fake`.

It installs `git gcc tar gzip findutils diffutils` first, because `fedora:latest` ships dnf5, rpm, bash, curl, tar
and sudo but **not** git or gcc: without git `actions/checkout` silently degrades to a REST tarball, and without gcc
`go test -race` fails with `-race requires cgo`. It then runs `go vet ./...`, `go test -race ./...`, and a smoke
step that asserts:

- `dotf platform` output contains `os=redhat`
- `dotf platform` output contains `pkg=dnf`
- `dotf apply --dry-run --headless` exits 0
- `dotf link --dry-run` exits 0

No arm64 runner is bought — architecture variance is rehearsed locally instead (Tier 2).

#### The shellcheck workflow and its one known gap

`shell.yml` runs `bin/lint.sh --shell` — the same dispatcher `npm run lint` uses — so CI and local linting cannot
drift apart. shellcheck is pinned to `v0.9.0` and installed from its upstream tarball under `$HOME` (no sudo, no
third-party action, so the Forgejo twin needs no divergence here).

Its `regression` job runs every `scripts/test-*.sh` driver plus the bats-core
suite in `tests/shell` (bats-core v1.11.1 installed from its pinned upstream
release tag under `$HOME` — again no sudo and no third-party action, so the
Forgejo twin runs the identical install). The GitHub job also covers macOS
with Homebrew bash 5 and re-runs the local-hook, parity, smoke and bats steps
against the system `/bin/bash` 3.2; the Forgejo twin is Linux-only.

The gap: `bin/lint.sh --shell` discovers files by the **`.sh` extension only**. The extensionless shell files are
therefore never linted by CI or by `npm run lint`:

- `bash/bashrc`
- `bash/profile` (and `bash/bash_profile`)
- `zsh/zshrc`
- `zsh/zprofile`

Run `shellcheck` on those by hand when you edit them.

## Tier 2 — Local container rehearsal (pre-PR)

Two containers cover what CI cannot, and [`docs/fedora-rehearsal.md`](../fedora-rehearsal.md) is their full recipe —
exact `docker run` invocations, measured behaviour, and the traps. It is not duplicated here.

In summary: **Container A** is a native amd64, privileged, `--cgroupns=host` Fedora container with `systemd` as PID
1, and it is the only place a real `dotf sysconfig --timezone … --firewall firewalld --ssh-server on --yes` can be
driven to `failed=0`. **Container B** is an emulated `linux/arm64` `fedora:latest` under `qemu-user`, and it is the
only place a real multi-package `dnf` transaction and an `rpm -q` cross-check run on aarch64 (expect roughly an 8–9×
wall-clock penalty on the RPM layer and effectively none on the Go binary). Run both before opening a
Fedora-touching pull request, not instead of the checklist.

The two cannot be combined: arm64 **plus** systemd is unreachable under `qemu-user` — `dbus-broker` dies
`226/NAMESPACE`. A full `qemu-system-aarch64` TCG VM is explicitly out of scope, so that combination is a Tier 3
claim.

## Tier 3 — Physical-device acceptance

[`docs/fedora-first-run.md`](../fedora-first-run.md) is the permanent home of the device checklist. It is **walked
end to end on the actual machine before a release tag is cut** — a gate, not a report. Do not copy its steps
anywhere else; edit it in place.

Claims that only the physical device can settle, each with its reason:

- **The verbatim `/etc/os-release` `ID` and `VARIANT_ID`** — four sources contradict each other; only the machine
  settles it.
- **Whether `chsh` is present, and from which package** — the same contradiction; the fallback is correct either
  way, but the answer should be recorded.
- **`bootstrap.sh` downloading the binary from a private prerelease origin** — no CI job exercises the shim against
  one. Run it with `GH_TOKEN` deliberately unset so success or failure is observed in isolation.
- **SELinux behaviour under real provisioning** — no container on an x86 host exposes `/sys/fs/selinux`; record
  `getenforce` before and after.
- **16 KiB page-size interaction with any C-linked third-party binary** — not modelled by any x86 container.
- **A live KDE Plasma 6 session** — `dotf plasma diff` runs headless, but the merge's effect on a Retina display and
  what `--reload`'s component restarts do to a running desktop cannot be judged headlessly.
- **`hostnamectl set-hostname`** — `/etc/hostname` is owned by the container runtime, so it fails in every
  container with `Device or resource busy`.
- **btrfs and snapper behaviour** — containers are overlayfs; the snapshot path is exercised only through the fake.
- **arm64 plus systemd together** — unreachable under `qemu-user`, as above.
- **The real NetworkManager/firewalld zone on a roaming laptop** — a container's zone is not a laptop's zone.

### Two standing DO-NOT-USE warnings

- **`--hyprland`** (and its deprecated alias `--linux-desktop`) on `dotf link` or `dotf apply`. It deploys the
  Hyprland/wlroots desktop set, which is categorically wrong on a KDE host. Use `dotf plasma` instead.
- **`--snapper`** on `dotf apply`. `cmd/dotf/applyphases.go` states verbatim that snapshot failures are **never**
  recorded in the failure collector, so a host with no snapper and no snapper config exits **0** having protected
  nothing. A green exit code here means the opposite of what it looks like.

## Adding a test

Two rules.

1. **Prefer a row in an existing table-driven test over a new test file.** Nearly every seam in this repository is
   already covered by a table; adding a case to it is cheaper to read, cheaper to maintain, and keeps the
   assertion next to its siblings. A new test file needs a reason a table row cannot give — a brand-new subsystem,
   or behaviour that writes outside the test's own `t.TempDir()`.
2. **A golden or lock table changing _is_ the intentionality signal.** When a change moves a golden, that churn is
   the review surface: it forces someone to look at exactly what moved and agree it should have. Never "fix" a
   golden by loosening the assertion — update the expected value, and explain the move in the commit message.

Live tripwires, so nobody trips one by accident:

- **`internal/manifest/ratchet_test.go`** — `customScriptBaseline` is an **exact match in both directions**
  (currently 23). Adding a `custom-script` entry fails the build, and so does removing one without ratcheting the
  constant down. Reach for a native backend first; see [`../reference/manifest-backends.md`](../reference/manifest-backends.md).
- **`internal/selfupdate/selfupdate_test.go`** — `TestAssetNaming_MatchesBootstrap` string-matches literals inside
  `bootstrap.sh`. Renaming a release asset means editing the goreleaser config, the shim and `selfupdate` together.
- **`internal/symlink/table_test.go`** — three golden locks over the symlink table: `TestTable_Count`,
  `TestTable_FacetCounts` and `TestTable_MatchesPlaybook`. Any entry added, removed, or re-faceted moves at least one
  of them.

## See Also

- [Documentation index](../README.md)
- [Platform Detection](../architecture/platform-detection.md)
- [Fedora First Run](../fedora-first-run.md) — the device acceptance checklist
- [Fedora Rehearsal](../fedora-rehearsal.md) — the two local containers
- [Manifest Backends](../reference/manifest-backends.md) — the backend ladder and the `custom-script` fence
