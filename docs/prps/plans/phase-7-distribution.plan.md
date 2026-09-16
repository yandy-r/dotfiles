# Plan: Distribution + `bootstrap.sh` shim + conservative cutover (Phase 7)

## Summary

Turn `dotf` into a **distributed, self-installing binary** and replace the 1,848-line
`bootstrap.sh` with a ~45-line POSIX shim. Three net-new artifacts do the work:

1. **`.goreleaser.yml`** — builds `dotf` for linux/darwin/windows × amd64/arm64 with
   version-free archive names (`dotf_{{ .Os }}_{{ .Arch }}`) and a `checksums.txt`, injecting
   `internal/version.Version`/`.Commit` via `-ldflags`.
2. **`.github/workflows/release.yml`** — a GitHub Actions release-on-tag (`v*`) workflow that
   runs GoReleaser and publishes to GitHub Releases.
3. **`bootstrap.sh` (shim)** — detects OS/arch, downloads the matching release asset from
   `…/releases/latest/download/`, sha256-verifies it against `checksums.txt`, extracts, and
   `exec`s `dotf apply` (injecting the mandatory `apply` subcommand). A safe fallback prints an
   actionable message + a local-build escape hatch when no release exists yet or the platform
   is unsupported — a fresh clone never hard-crashes.

This is the **conservative "Option A" cutover**. The shim replaces the bash provisioning body
wholesale, but **no `bin/install-*.sh` script is deleted**: `manifest/packages.toml` is still a
7-package skeleton, so nothing meets the SAFE_TO_DELETE bar ("100% absorbed into the manifest
via a pure-data backend **and** referenced nowhere"). Phase 7 removes ~1.8k bash LOC (~12%);
the PRD's ≥80% bash-LOC-retired metric is explicitly **deferred** to the manifest-population /
script-retirement follow-up, not claimed here.

## User Story

> As the maintainer on a brand-new machine, I run `git clone … && ./bootstrap.sh`. The shim
> downloads the checksum-verified `dotf` binary for my OS/arch and hands off to `dotf apply`'s
> guided TUI. `./bootstrap.sh --force` runs headless (`dotf apply --force`). `./bootstrap.sh
> version` passes straight through. Before any release exists, the same clone still provisions:
> the shim tells me exactly how to tag a release **or** build from source, instead of dumping a
> raw `curl: (22)` 404.

## Problem → Solution

`bootstrap.sh` is a 1,848-line bash script that shells out to ~21 `bin/install-*.sh` installers
and the Ansible symlink playbook. Phases 1–6 rebuilt provisioning natively in Go
(`dotf apply` orchestrates sysconfig → packages → symlinks → firewall with full flag/env
parity), but `dotf` is **not distributed** — there is no `.goreleaser.yml`, no release
workflow, and no prebuilt binary, so a fresh machine cannot run it without a Go toolchain.
Phase 7 closes that gap: it ships the release pipeline and rewrites `bootstrap.sh` into a shim
that downloads and execs `dotf apply`. Bash provisioning is retired as the **entry point**;
the individual installer scripts are retained (still reachable via `dotf`'s `custom-script`
backend and for the not-yet-migrated tools), per the conservative scope.

## Metadata

| Field            | Value                                                                       |
| ---------------- | --------------------------------------------------------------------------- |
| Issue            | Forgejo **#15** (distribution / shim) and **#22** (cutover / LOC) — `Closes #15`, `Closes #22` in the PR body |
| PRD              | `docs/prps/prds/provisioning-cli.prd.md` — Phase 7 (L158, L199–203), Distribution decision (L221), Success Metric "Bash LOC retired" (L39) |
| Module / binary  | `github.com/yandy-r/dotfiles`, Go 1.26, entrypoint `./cmd/dotf`, binary `dotf` |
| Commit scope     | `feat(dist): …` / `build(release): …` for the pipeline; `refactor(bootstrap): …` for the shim; docs under `docs/prps/**` → `docs(internal): …`, root docs → `docs: …` |
| Depends on       | Phase 6 (`dotf apply`, merged — flag/env parity, snapper, failure summary)   |
| New artifacts    | `.goreleaser.yml`, `.github/workflows/release.yml`, rewritten `bootstrap.sh` |
| Distribution target | GitHub remote (`github` = `git@github.com:yandy-r/dotfiles.git`) — GoReleaser publishes there |
| CI split         | **Forgejo Actions** runs `.github/workflows/go.yml` (test/vet/lint on `origin` PRs); **GitHub Actions** runs `.github/workflows/release.yml` (release on tag pushed to `github`) |

## Batches

| Batch | Tasks                | Can run in parallel | Rationale                                        |
| ----- | -------------------- | ------------------- | ------------------------------------------------ |
| B1    | 1.1, 1.2             | yes (disjoint files) | GoReleaser config + release workflow are independent files |
| B2    | 2.1                  | —                   | Shim rewrite; verify it maps to the B1 asset names |
| B3    | 3.1                  | —                   | Docs — depends on the shim/pipeline being final   |

---

## Scope

### In scope (Option A — conservative cutover)

- **`.goreleaser.yml`** (net-new): 6 asset archives + `checksums.txt`, version-free names, `-ldflags` version injection.
- **`.github/workflows/release.yml`** (net-new): release-on-tag `v*`, GitHub Actions, GoReleaser v2.
- **`bootstrap.sh` rewrite** (full replacement): ~45-line POSIX `sh` shim that downloads →
  checksum-verifies → execs `dotf apply`, injecting the `apply` subcommand and translating away
  `--interactive`, with a graceful no-release / unsupported-platform guard and local-build
  escape hatch.
- **Doc cutover**: flip all "bootstrap still shells to bash / Phase 7 pending" language to the
  shim reality across `CLAUDE.md`, `AGENTS.md`, `README.md`, `playbooks/README.md`,
  `docs/README.md`, `bin/README.md`, `.ai/rules/project.md`, `.cursor/rules/project.mdc`, and
  the PRD phase-7 row + detail block.

### Explicitly out of scope / deferred

- **VM dogfood** — the PRD success signal (clone → provisioned on a clean Arch/Debian/macOS VM)
  is a **manual maintainer gate**, not automatable in this PR (no release asset exists to fetch
  until the first tag is cut). Listed under Manual Follow-ups.
- **Tagging / publishing the first release** — this PR lands the *pipeline*; cutting `v0.1.0`
  and pushing it to the `github` remote is a deliberate maintainer action after the shim/asset
  names are locked (the shim verifies against real assets). Manual Follow-up.
- **Aggressive script deletion** — per brief R2, **SAFE_TO_DELETE is empty**: the manifest is a
  7-package skeleton, so no `bin/install-*.sh` is fully absorbed. No installer is deleted here.
  Growing the manifest to real per-tool pure-data backends (the PRD's "~18 scripts → data",
  L82/L177) and cutting the corresponding `bootstrap.sh` call-sites is a separate, larger
  effort that the ≥80% metric tracks.
- **`bin/install-cursor-agent.sh` orphan** (604 LOC, zero references) — a genuine dead-code
  candidate, but it was never migrated into the manifest, so it does **not** qualify as
  "manifest absorption." Flagged for a **separate maintainer decision** (Manual Follow-up), not
  bundled into this cutover.
- **`-rc`/snapshot release testing** — `prerelease: auto` means a prerelease tag is skipped by
  `/releases/latest/download`; test only with a stable tag (noted so a maintainer does not hit
  the guard on an `-rc`).

---

## Mandatory Reading (ground truth for this plan)

- **Brief R1** (bootstrap anatomy + shim contract) — the full `bootstrap.sh` flag/env surface,
  the `dotf apply` invocation contract (mandatory `apply` subcommand, `--interactive` has no
  equivalent, `DOTFILES_ROOT`/`resolveRepoRoot` note), exit codes, and the Option-A fallback
  requirement.
- **Brief R2** (script retirement) — SAFE_TO_DELETE = empty; KEEP all 25; the cursor-agent
  orphan finding.
- **Brief R3** (distribution artifacts) — the **verbatim** contents of the three files below and
  the cross-file consistency checklist (asset name ≡ template, checksum format, extracted-binary
  path, `apply` injection, ldflags var paths, action SHAs).
- **Brief R4** (docs / LOC / follow-ups) — the exact doc edits, the LOC math, the manual gates,
  and the `Closes #15` / `Closes #22` PR footer.
- `cmd/dotf/main.go` (`:34-37` no default subcommand → exit 1; `:39-63` dispatch; `:58-59`
  `apply` → `runApply`), `cmd/dotf/apply.go` (`:42-51` flags; `:53-55` parse-error → exit 2;
  `:64` `DOTF_MANIFEST`; `:75` `resolveRepoRoot`; `:145` `symlink.NewEngine(repoRoot, home)`),
  `cmd/dotf/applyopts.go` (`:36` `envTrue`; `:48-61` `resolveApplyOptions`),
  `internal/version/version.go` (`Version`/`Commit` in `github.com/yandy-r/dotfiles/internal/version`),
  `.github/workflows/go.yml` (pinned `checkout@…v4`, `setup-go@…v5` SHAs to reuse).

---

## Patterns to Mirror

- **Pinned action SHAs**: reuse the exact SHAs already in `go.yml` for `actions/checkout` (v4)
  and `actions/setup-go` (v5); pin `goreleaser/goreleaser-action` to the verified v7.2.3 SHA.
  Never float a tag.
- **POSIX `sh`, `set -eu`**: the shim is `#!/bin/sh`, not bash — it must run on a machine with
  nothing but coreutils + curl + tar. Guard every external tool with `command -v`.
- **Fail-fast at boundaries with an actionable message**: `die`/`guard` helpers print
  `dotf bootstrap: …` context, never a raw curl/tar error. A 404 (no release) is an *expected*
  Option-A path, so it routes to the multi-line `guard` with both a "cut a release" and a
  "build from source" instruction — not a hard crash.
- **`apply` injection**: `dotf` has no default subcommand (exit 1 on none). The shim prepends
  `apply` only when there are no args or the first token is a flag, so `dotf version` /
  `dotf apply --force` pass through unchanged.

---

## The three new artifacts (write verbatim)

> Write these three files **exactly** as below. They are internally cross-consistent (asset
> name ≡ GoReleaser `name_template`; checksum filename/format; extracted `dotf` at archive root;
> `apply` injection; ldflags var paths; action SHAs). Do not "improve" names or templates — the
> shim resolves `…/releases/latest/download/dotf_${OS}_${ARCH}.tar.gz` by exact string match.

### File 1 — `.goreleaser.yml`

```yaml
# GoReleaser configuration for the dotf CLI.
# Distribution target: github.com/yandy-r/dotfiles (Releases).
# Docs: https://goreleaser.com  —  schema version 2.
version: 2

project_name: dotf

before:
  hooks:
    - go mod tidy

builds:
  - id: dotf
    main: ./cmd/dotf
    binary: dotf
    env:
      - CGO_ENABLED=0
    goos:
      - linux
      - darwin
      - windows
    goarch:
      - amd64
      - arm64
    flags:
      - -trimpath
    mod_timestamp: "{{ .CommitTimestamp }}"
    ldflags:
      - -s -w -X github.com/yandy-r/dotfiles/internal/version.Version={{.Version}} -X github.com/yandy-r/dotfiles/internal/version.Commit={{.ShortCommit}}

archives:
  - id: dotf
    ids:
      - dotf
    # Deterministic, version-free name so the bootstrap shim can resolve
    # /releases/latest/download/<name> without parsing a version.
    name_template: "dotf_{{ .Os }}_{{ .Arch }}"
    formats:
      - tar.gz
    format_overrides:
      - goos: windows
        formats:
          - zip
    files:
      - LICENSE*
      - README*

checksum:
  name_template: "checksums.txt"
  algorithm: sha256

snapshot:
  version_template: "{{ incpatch .Version }}-snapshot-{{ .ShortCommit }}"

changelog:
  use: git
  sort: asc
  filters:
    exclude:
      - "^docs:"
      - "^test:"
      - "^chore:"
      - "^ci:"
      - "^style:"
      - Merge pull request
      - Merge branch

release:
  github:
    owner: yandy-r
    name: dotfiles
  draft: false
  prerelease: auto
```

**Resulting assets** (what the shim depends on): `dotf_linux_amd64.tar.gz`,
`dotf_linux_arm64.tar.gz`, `dotf_darwin_amd64.tar.gz`, `dotf_darwin_arm64.tar.gz`,
`dotf_windows_amd64.zip`, `dotf_windows_arm64.zip`, `checksums.txt`. Each archive holds `dotf`
at its root (GoReleaser default, no `wrap_in_directory`).

**Schema notes (v2 correctness):** `snapshot.version_template` is the v2 key (older
`name_template` under `snapshot` is deprecated); `formats` (list) + `format_overrides[].formats`
is v2 (singular `format` is deprecated); `archives.ids` scopes the archive to the `dotf` build.
`release.github.owner/name` is pinned so GoReleaser never mistakes the Forgejo `origin` for the
release repo.

### File 2 — `.github/workflows/release.yml`

```yaml
name: Release

on:
  push:
    tags:
      - "v*"

permissions:
  contents: write

jobs:
  goreleaser:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5 # v4
        with:
          fetch-depth: 0

      - name: Set up Go
        uses: actions/setup-go@40f1582b2485089dde7abd97c1529aa768e1baff # v5
        with:
          go-version-file: go.mod
          cache: true

      - name: Run GoReleaser
        uses: goreleaser/goreleaser-action@f06c13b6b1a9625abc9e6e439d9c05a8f2190e94 # v7.2.3
        with:
          distribution: goreleaser
          version: "~> v2"
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

**Notes:** `fetch-depth: 0` gives GoReleaser full tag/commit history for the changelog.
`version: "~> v2"` pins the GoReleaser binary to the v2 line (matches `version: 2` in the
config). `contents: write` is required to publish the release. This runs on **GitHub Actions**
(tag pushed to the `github` remote); Forgejo keeps running `go.yml`. The `checkout`/`setup-go`
SHAs are the exact pins from `.github/workflows/go.yml:29,32`.

### File 3 — `bootstrap.sh` (shim, full replacement)

```sh
#!/bin/sh
# ==============================================================================
# dotfiles bootstrap shim (Phase 7, Option A)
# ------------------------------------------------------------------------------
# Downloads the prebuilt `dotf` binary from GitHub Releases, verifies its
# sha256 checksum, and execs it — forwarding all arguments. With no subcommand
# (or a leading flag) it injects `apply`, the headless-capable orchestrator.
#
# Safe fallback: if the platform is unsupported OR no release asset is found
# (e.g. no release has been tagged yet), it prints the exact manual step and a
# local-build escape hatch, then exits non-zero. It never hard-breaks silently.
# ==============================================================================
set -eu

REPO="yandy-r/dotfiles"
BINARY="dotf"

die() {
  printf '%s\n' "dotf bootstrap: $*" >&2
  exit 1
}

guard() {
  # $1 = human-readable reason
  cat >&2 <<EOF
dotf bootstrap: $1

No prebuilt binary could be used. Do ONE of the following:

  1. Tag a release so prebuilt binaries exist (maintainer, on the GitHub remote):
       git tag v0.1.0 && git push github v0.1.0
     then re-run this script.

  2. Build locally from a clone of https://github.com/${REPO} (requires Go 1.26+):
       go build -o ${BINARY} ./cmd/dotf
       ./${BINARY} apply --headless
EOF
  exit 1
}

# --- Detect OS -----------------------------------------------------------------
case "$(uname -s)" in
  Linux)  OS="linux" ;;
  Darwin) OS="darwin" ;;
  *)      guard "unsupported OS '$(uname -s)' (only linux and darwin have prebuilt binaries)" ;;
esac

# --- Detect ARCH ---------------------------------------------------------------
case "$(uname -m)" in
  x86_64 | amd64)   ARCH="amd64" ;;
  aarch64 | arm64)  ARCH="arm64" ;;
  *)                guard "unsupported architecture '$(uname -m)'" ;;
esac

command -v curl >/dev/null 2>&1 || die "curl is required but was not found"
command -v tar  >/dev/null 2>&1 || die "tar is required but was not found"

# Must match .goreleaser.yml archives.name_template: "dotf_{{ .Os }}_{{ .Arch }}"
ASSET="${BINARY}_${OS}_${ARCH}.tar.gz"
BASE="https://github.com/${REPO}/releases/latest/download"

TMP="$(mktemp -d "${TMPDIR:-/tmp}/dotf-boot.XXXXXX")" || die "failed to create temp dir"

# --- Download archive + checksums (latest release redirect; 404 -> guard) -----
curl -fsSL "${BASE}/${ASSET}"        -o "${TMP}/${ASSET}" \
  || guard "could not download '${ASSET}' from the latest release of ${REPO}"
curl -fsSL "${BASE}/checksums.txt"   -o "${TMP}/checksums.txt" \
  || guard "could not download 'checksums.txt' from the latest release of ${REPO}"

# --- Verify sha256 (Linux: sha256sum, macOS: shasum -a 256) --------------------
(
  cd "${TMP}"
  if command -v sha256sum >/dev/null 2>&1; then
    grep " ${ASSET}\$" checksums.txt | sha256sum -c -
  elif command -v shasum >/dev/null 2>&1; then
    grep " ${ASSET}\$" checksums.txt | shasum -a 256 -c -
  else
    echo "dotf bootstrap: neither sha256sum nor shasum found; cannot verify checksum" >&2
    exit 1
  fi
) || die "checksum verification failed for ${ASSET}"

# --- Extract -------------------------------------------------------------------
tar -xzf "${TMP}/${ASSET}" -C "${TMP}" || die "failed to extract ${ASSET}"
[ -x "${TMP}/${BINARY}" ] || die "extracted archive did not contain an executable '${BINARY}'"

# --- Inject default subcommand & exec -----------------------------------------
# No args, or a leading flag -> prepend `apply` (the orchestrator).
# `dotf version`, `dotf apply --force`, etc. pass through unchanged.
if [ "$#" -eq 0 ] || [ "${1#-}" != "$1" ]; then
  set -- apply "$@"
fi

exec "${TMP}/${BINARY}" "$@"
```

### Cross-file consistency (verify after writing)

- Shim asset `dotf_${OS}_${ARCH}.tar.gz` ≡ GoReleaser `name_template: "dotf_{{ .Os }}_{{ .Arch }}"` + `formats: [tar.gz]`.
- Shim `checksums.txt` ≡ GoReleaser `checksum.name_template: "checksums.txt"` (sha256).
- `grep " ${ASSET}$"` matches GoReleaser's two-space `<hash>  <name>` line; piped to `-c -` from inside `${TMP}` so both `sha256sum -c` and `shasum -a 256 -c` resolve the file.
- Extracted `dotf` at archive root ≡ shim `"${TMP}/${BINARY}"`.
- Default subcommand `apply` ≡ `cmd/dotf/main.go:58`; `--force`/`--headless` forward to `apply` per `cmd/dotf/apply.go:42-43`.
- ldflags var paths ≡ `internal/version/version.go:12,15`.
- `release.yml` action SHAs ≡ `go.yml` (checkout v4, setup-go v5) + verified goreleaser-action v7.2.3.

### Shim decisions to surface (from R1/R3)

- **`--interactive` has no `dotf apply` equivalent.** Bootstrap accepted `--interactive`;
  forwarding it raw to `dotf apply` is an undefined flag → `flag.Parse` error → exit 2. The
  guided intent TUI is already the default on a TTY-without-`--headless`, so **the shim as
  written simply does not special-case `--interactive`** — a user passing it will hit exit 2.
  If a maintainer wants `./bootstrap.sh --interactive` to keep working as a silent alias for the
  default TUI, add a one-line pre-filter that drops a leading `--interactive` before the
  `apply`-injection block. This is documented, not required, for Option A (the flag is
  maintainer-facing).
- **Repo-root resolution.** `runApply` → `resolveRepoRoot()` walks up from CWD for
  `manifest/packages.toml`, else `DOTFILES_ROOT`. The manifest is `go:embed`'d, but the symlink
  engine sources real files from `repoRoot` (`apply.go:145`). Because `./bootstrap.sh` runs with
  CWD = the clone root, `resolveRepoRoot` finds `manifest/packages.toml` there and resolution
  succeeds. If a future invocation runs the shim from outside the clone, export
  `DOTFILES_ROOT="$(dirname "$0")"` before the `exec`. Not needed for the documented
  `git clone … && ./bootstrap.sh` path; noted for robustness.
- **`exec` leaves `${TMP}` behind by design** — no EXIT trap can clean a tmpdir holding the
  running binary. One `${TMPDIR:-/tmp}/dotf-boot.XXXXXX` dir per run is the accepted trade-off
  (the task specified `exec … "$@"`). To reclaim it, switch `exec` to a
  run-then-cleanup-then-`exit $?` pattern.
- **`prerelease: auto`** — a `-rc`/snapshot tag is flagged prerelease and skipped by
  `/releases/latest/download`; the shim only ever pulls a stable release.

---

## Script retirement (Option A)

### SAFE_TO_DELETE — **EMPTY (0 scripts, 0 LOC)**

`manifest/packages.toml` is a **minimal worked example, not a full port**: 7 `[[package]]`
entries (one per backend), and the only `bin/install-*.sh` it references at all is
`manifest/packages.toml:93` → `script = "bin/install-atuin.sh"` (the `custom-script` backend).
No script meets the bar "100% absorbed into `packages.toml` via a pure-data backend **and**
referenced nowhere":

- **`install-atuin.sh`** — tool *is* in the manifest, but via `custom-script`, which *invokes
  the script itself* (not pure data), and it is referenced by bootstrap, `CLAUDE.md`,
  `common/tools.sh`, and Go tests. **KEEP.**
- **`install-docker.sh`** — `docker-ce` uses the pure-data `apt-repo` backend
  (`manifest/packages.toml:76`), but the entry is Ubuntu-apt only (`names = { apt = "docker-ce" }`
  + a hardcoded Ubuntu repo at L80) — it covers none of Arch/Fedora/macOS the script handles, so
  it is **partially absorbed only**, and it is still functionally referenced at
  `bootstrap.sh:1404-1405`. **KEEP.**

**Zero-reference proof for the absorbing entry:** the only manifest → script edge in the entire
`manifest/` tree is the single `atuin` `custom-script` line (L93); grepping `bin/install-` under
`manifest/` returns exactly that one hit. Every other script's tool is absent from the manifest,
so by the rule "not covered by the manifest at all" they are automatic KEEP.

### KEEP — all 25 `bin/install-*.sh`

**Invoked directly by `bootstrap.sh` (22)** — live functional dependencies of the (now-retired
as an entry point, but still-referenced-by-the-body-logic) provisioning path; their tools are
absorbed by `dotf apply`'s package phase only where a manifest entry exists (currently 7):

`install-lua.sh`, `install-neovim.sh`, `install-claude.sh`, `install-opencode.sh`,
`install-codex.sh`, `install-fabric.sh`, `install-atuin.sh`, `install-turso.sh`,
`install-tea.sh`, `install-coderabbit.sh`, `install-lefthook.sh`, `install-docker.sh`,
`install-tailscale.sh`, `install-native-packages.sh`, `install-aws-tools.sh`,
`install-azure-cli.sh`, `install-hashicorp-tools.sh`, `install-container-tools.sh`,
`install-modern-cli.sh`, `install-arch-pkgs.sh`, `install-brew.sh`, `install-mise.sh`.

**Not invoked by bootstrap but still referenced / not manifest-covered (2):**
`install-smartsheet-cli.sh` (referenced by `bin/smartsheet-*.md`), `install-yazi.sh`
(referenced by `bin/README.md`).

**Orphan, flagged separately (1):** `install-cursor-agent.sh` (604 LOC) has **zero references
anywhere** in the repo. It is genuine dead code, but because it was never migrated into the
manifest it does **not** qualify as SAFE_TO_DELETE under the conservative rule. → **Manual
Follow-up** for a maintainer decision (orphan removal), not part of this cutover.

> All 25 scripts total ~12,967 LOC; **none** are eligible for deletion under Option A.

---

## Doc edits (exact)

> `CLAUDE.md` is the source of truth; `AGENTS.md`/`.cursor/rules/project.mdc`/`.ai/rules/project.md`
> are sync-locked to it. Keep the four in lockstep. Root docs (`README.md`, `docs/README.md`,
> `bin/README.md`, `playbooks/README.md`) commit as `docs: …`; PRD/plan under `docs/prps/**` as
> `docs(internal): …`.

### `docs/prps/prds/provisioning-cli.prd.md`

1. **Phase table, L158** — flip `Status` `pending` → `complete`, and replace the trailing PRP-Plan
   `-` with `[plan](../plans/phase-7-distribution.plan.md)` (matches the `phase-4-…`/`phase-5-…`
   naming).
2. **After L203** (end of the Phase 7 detail block), add a `- **Status**` bullet mirroring the
   Phase 6 block at L197:
   > `- **Status**: **complete** — delivered as `.goreleaser.yml` (`CGO_ENABLED=0`, linux/darwin/windows × amd64/arm64, `-ldflags -X …/internal/version.Version`/`.Commit`, version-free archive names + `checksums.txt`), a GitHub Actions release-on-tag workflow (`.github/workflows/release.yml`, GoReleaser v2), and a ~45-line POSIX `bootstrap.sh` shim (detect OS/arch → download `…/releases/latest/download/<asset>` → sha256-verify → `exec dotf apply`, injecting the mandatory `apply` subcommand). A safe no-release fallback prints an actionable message + a local-build escape hatch (`go build -o dotf ./cmd/dotf`); a fresh clone never hard-crashes. **Conservative (Option A) cutover:** the shim replaces the bash body, but no `bin/install-*.sh` is retired (the manifest is still a 7-package skeleton — SAFE_TO_DELETE is empty), so only ~1.8k bash LOC (~12%) are removed here. VM dogfood and the first `v0.1.0` tag are maintainer gates; the ≥80% bash-LOC-retired metric is deferred to the manifest-population / script-retirement follow-up. See [plan](../plans/phase-7-distribution.plan.md).`
3. **Success Metrics, L39** ("Bash LOC retired") — annotate that this is a **full-cutover target
   not satisfied at Phase 7 landing** (e.g. append "— measured at cutover; ~12% removed at Phase 7
   landing (shim only), ≥80% tracked to manifest-population / script-retirement follow-up").

### `CLAUDE.md`

- **L28** ("Accuracy note: `bootstrap.sh` still shells to the bash provisioning path until the
  Phase 7 cutover; only the **symlink mechanism** …") → replace with: *"As of Phase 7,
  `bootstrap.sh` is a ~45-line POSIX shim that downloads the prebuilt, checksum-verified `dotf`
  binary from GitHub Releases and runs `dotf apply`; the bash provisioning path is retired as the
  entry point (the `bin/install-*.sh` installers are retained — the manifest is still a skeleton)."*
- **Stack Overview, L745-751** — rewrite the paragraph so `bootstrap.sh` is the shim and
  `dotf apply` is *the* entry point: drop "still shells to the bash provisioning path until the
  Phase 7 cutover" and "dogfooded but not yet wired into `bootstrap.sh` (that is Phase 7)"; state
  that Phase 7 shipped `.goreleaser.yml` + `release.yml` + the shim and `bootstrap.sh` now execs
  `dotf apply`.
- **Infrastructure table row, L759** — change "Ansible (retired for symlinks) … `bootstrap.sh`
  still shells to the bash provisioning path until the Phase 7 cutover" to note bootstrap is now
  the shim → `dotf apply`; Ansible remains historical-rollback-only.
- **L431** ("`bootstrap.sh`: Cross-platform setup script with intelligent detection") and **L446**
  ("The `bootstrap.sh` script handles initial setup and tool installation") → reframe both as the
  shim that downloads `dotf` and runs `dotf apply`.
- **L383** ("Update `playbooks/symlinks.yml` to create necessary symlinks") in *Adding New
  Applications* → replace with adding a `[[symlink]]` entry to `manifest/packages.toml` and
  running `dotf link` (this is actively wrong post-Phase-4).
- **L714-719 / L424 / L546** (validation & debug commands that say
  `ansible-playbook playbooks/symlinks.yml --extra-vars "dryrun=true"`) → de-emphasize; replace
  with `dotf link --dry-run` guidance (Ansible is no longer in provisioning). Leave the already
  commented rollback command at L184.

### `AGENTS.md`

- **L21** ("`bootstrap.sh` still shells to the bash provisioning path until the Phase 7 cutover.")
  → same flip as `CLAUDE.md` L28. Keep in lockstep with `.cursor/rules/project.mdc` per the sync
  policy (AGENTS.md L24-28). Do not duplicate `CLAUDE.md` prose — the one-line reference is enough.

### `README.md`

- **L78-79** ("`dotf apply` is dogfooded but not yet the `bootstrap.sh` entry point — the shim
  cutover is Phase 7.") → *"As of Phase 7, `./bootstrap.sh` is a ~45-line POSIX shim that downloads
  the checksum-verified `dotf` binary from GitHub Releases and execs `dotf apply` (booleans still
  resolve flag-OR-env, truthy `"true"`)."*
- **Add a top-level Installation / Quick-start section** (near the top): `git clone … &&
  ./bootstrap.sh` (shim path → guided TUI); `./bootstrap.sh --force` (headless); the no-release
  behavior (clear actionable error, not a raw curl failure); and the local-build escape hatch
  (`go build -o dotf ./cmd/dotf && ./dotf apply`).
- **L25** ("The Ansible playbook … **retired but retained** for one release …") → soften to
  historical-rollback-only.

### `playbooks/README.md`

- **Banner (L3)** currently "RETIRED (retained for rollback) … slated for deletion next release."
  Phase 7 is the "one release" boundary → update the banner to state the `dotf` CLI (shim →
  `dotf apply`, `dotf link` for symlinks) is now the entry point and this directory is
  **historical-rollback-only**. Keep the file (Option A "everything else stays").

### `docs/README.md`

- **L72** (`[bootstrap.sh](../bootstrap.sh) | Initial setup and installation`) → "~45-line shim:
  downloads `dotf`, runs `dotf apply`".
- **L105 / L108** (`ansible-playbook playbooks/symlinks.yml --extra-vars "dryrun=true"` /
  `"backup=true"`) → replace with `dotf link --dry-run` and `dotf link --backup`.

### `bin/README.md`

- **L10** (Setup row: `bootstrap.sh | Initial environment setup`) and **L22-31** (the
  `### bootstrap.sh` section with `./bootstrap.sh` / `NON_INTERACTIVE=true ./bootstrap.sh`
  examples) → reframe as the shim. `NON_INTERACTIVE=true` still passes through to `dotf apply` via
  env parity, so the second example stays valid with a reword (it now means "headless `dotf apply`
  via the shim").

### `.ai/rules/project.md` (L21) and `.cursor/rules/project.mdc` (L29)

- Identical stale line: `Ansible changes: validate with `ansible-playbook
  playbooks/symlinks.yml --extra-vars "dryrun=true"`.` → replace both with
  `Symlink changes: validate with `dotf link --dry-run`.` (and keep the existing Go-validation
  line in the `.mdc`). Update both together.

> **Out of scope:** the many `.docs/plans/**` / `docs/plans/**` historical planning artifacts that
> mention `bootstrap.sh`/Ansible — leave as history.

---

## Step-by-Step Tasks

### Task 1.1: `.goreleaser.yml` — Depends on [none]

- [ ] **BATCH B1.** Create `.goreleaser.yml` **verbatim** from *File 1* above.
- [ ] **VERIFY** locally without cutting a release: `goreleaser check` (schema-validates the
      config) and `goreleaser release --snapshot --clean --skip=publish` (builds all 6 archives +
      `checksums.txt` into `dist/`). Confirm `dist/` contains `dotf_linux_amd64.tar.gz` … and that
      each tar.gz extracts a `dotf` at its root.
- **GOTCHA**: use the v2 keys exactly (`snapshot.version_template`, `formats`,
  `format_overrides[].formats`, `archives.ids`) — singular `format`/snapshot `name_template` are
  deprecated and `goreleaser check` will warn/fail. `release.github.owner/name` must stay pinned.

### Task 1.2: `.github/workflows/release.yml` — Depends on [none]

- [ ] **BATCH B1.** Create `.github/workflows/release.yml` **verbatim** from *File 2* above.
- [ ] **VERIFY** the three action SHAs: `checkout`/`setup-go` match `.github/workflows/go.yml`;
      `goreleaser-action` is the v7.2.3 SHA `f06c13b6b1a9625abc9e6e439d9c05a8f2190e94`. Confirm
      `permissions: contents: write` and `on.push.tags: ["v*"]`.
- **GOTCHA**: this workflow runs on **GitHub Actions**, triggered by a tag pushed to the `github`
  remote — not Forgejo. Do not add it to any Forgejo-only path filter.

### Task 2.1: `bootstrap.sh` shim rewrite — Depends on [1.1] (asset names must be final)

- [ ] **BATCH B2.** Replace the entire `bootstrap.sh` (1,848 lines) with the shim **verbatim**
      from *File 3* above.
- [ ] **VERIFY** the shim is `#!/bin/sh`, `set -eu`, `shellcheck`-clean, and that the `ASSET`
      string matches the GoReleaser `name_template` character-for-character.
- [ ] **VERIFY the no-release path is graceful** (this is the live path until a tag exists): run
      `./bootstrap.sh` on this repo and confirm it prints the `guard` message (tag-a-release +
      local-build escape hatch) and exits non-zero — **not** a raw `curl: (22)` 404.
- [ ] **VERIFY apply injection**: `sh -x` trace (or a dry read) that no-args and a leading-flag
      invocation prepend `apply`, while `./bootstrap.sh version` does not.
- **GOTCHA**: `exec` leaves `${TMP}` behind by design (accepted). `--interactive` is **not**
  translated — document it (see "Shim decisions"). Keep `command -v` guards for `curl`/`tar` and
  the `sha256sum`/`shasum` branch.

### Task 3.1: Docs cutover — Depends on [2.1]

- [ ] **BATCH B3.** Apply every edit in the **Doc edits (exact)** section: PRD (3 edits),
      `CLAUDE.md`, `AGENTS.md`, `README.md` (+ new Installation section), `playbooks/README.md`,
      `docs/README.md`, `bin/README.md`, `.ai/rules/project.md`, `.cursor/rules/project.mdc`.
- [ ] **VERIFY** no remaining "bootstrap still shells to bash / Phase 7 pending / dogfooded but
      not wired" language in the source-of-truth docs:
      `grep -rn "shells to the bash provisioning" CLAUDE.md AGENTS.md README.md` → empty;
      `grep -rn "Phase 7 cutover" CLAUDE.md AGENTS.md` → empty (except historical `docs/plans/**`).
- [ ] **VERIFY** the four sync-locked files (`CLAUDE.md`/`AGENTS.md`/`.cursor/rules/project.mdc`/
      `.ai/rules/project.md`) are consistent.
- **GOTCHA**: do not duplicate `CLAUDE.md` prose into `AGENTS.md` (its sync policy forbids it) —
  update the single reference line only. Leave `docs/plans/**` history untouched.

---

## Validation commands (the full local gate)

Run from the repo root. All must be clean before "done."

```bash
# Go build/vet/test/lint (dotf is unchanged in Phase 7, but the release build path is exercised).
go build ./...
go vet ./...
go test -race ./...
golangci-lint run ./...            # or: bin/lint.sh --go

# Format — must produce NO diff.
gofmt -l .                         # EXPECT: empty
test -z "$(gofmt -l .)"

# Shim lint (the shim is the only shell file changed).
shellcheck bootstrap.sh

# GoReleaser config + a publish-less snapshot build (proves the pipeline before any tag).
goreleaser check
goreleaser release --snapshot --clean --skip=publish
ls dist/                           # EXPECT: dotf_{linux,darwin,windows}_{amd64,arm64}.* + checksums.txt

# No-release shim guard (live Option-A path today).
./bootstrap.sh ; echo "exit=$?"    # EXPECT: guard message to stderr, non-zero exit (no raw curl error)

# Full mixed-language lint (pre-push).
npm run lint
```

**CI note:** the PR gate is **Forgejo Actions running `.github/workflows/go.yml`** (test / vet /
golangci-lint on the `origin` PR). The new `.github/workflows/release.yml` runs on **GitHub
Actions** only, triggered by a `v*` tag pushed to the `github` remote — it does not run on the
Forgejo PR. Verify the release workflow by pushing the first tag (Manual Follow-up), not in this
PR's CI.

---

## LOC accounting + the ≥80% metric (honest)

**Baseline (confirmed by `wc -l`):**

| Component                       | Lines      |
| ------------------------------- | ---------- |
| `bootstrap.sh`                  | 1,848      |
| `bin/install-*.sh` (25 files)   | 12,967     |
| **Grand total**                 | **14,815** |

**Removed in THIS PR (Option A):**

| Item                                       | Lines removed |
| ------------------------------------------ | ------------- |
| `bootstrap.sh` 1,848 → ~45-line shim       | ~1,803        |
| SAFE_TO_DELETE install scripts             | 0             |
| **Total**                                  | **~1,803**    |

**~1,803 / 14,815 ≈ 12.2%.**

**Verdict — the ≥80% Success Metric is NOT met in this PR and must not be claimed here.** To
reach 80% you must remove ≥11,852 of 14,815 lines — i.e. all of `bootstrap.sh` **plus** ~10,000
of the 12,967 install-script lines. That requires migrating the PRD's "~18/25 scripts → manifest
data" (`prd:L82,L177`) and cutting the corresponding `bootstrap.sh` call-sites over to
`dotf apply`'s package phase — deliberately outside Option A's conservative scope.

State it in the PR exactly as: *"Phase 7 lands the shim + distribution and removes ~1.8k bash
LOC (~12%); the ≥80% metric (PRD Success Metrics 'Bash LOC retired' row, measured `wc -l`
before/after at cutover, L39) remains an open target tracked to the manifest-population /
script-retirement work that follows."*

**Caveat to surface:** the Option-A shim execs `dotf apply`, which currently provisions only the
7 manifest packages — far less than today's `bootstrap.sh`. This is *why* the cutover is
conservative and *why* the no-release safe-fallback + local-build escape hatch matter: a fresh
clone stays fully provisionable (guided or from source) while the manifest is still a skeleton.

---

## Manual follow-ups (maintainer)

1. **VM dogfood gate** (PRD success signal, `prd:L40,L203`) on **Arch/CachyOS + Debian/Ubuntu +
   macOS**: `git clone … && ./bootstrap.sh` (guided) and `NON_INTERACTIVE=true ./bootstrap.sh` /
   `dotf apply --headless` (unattended); confirm each reaches the expected end state with no
   pre-installed runtime beyond curl/coreutils. **Cannot run until the first release exists** (the
   shim needs a real asset); until then the safe-fallback + local-build path is what a fresh clone
   exercises.
2. **Publish the first GitHub release** (only after the shim/checksum asset names are locked,
   since the shim verifies against release assets):

   ```bash
   git tag v0.1.0
   git push github v0.1.0     # push to the GitHub remote (NOT origin/Forgejo) to trigger GoReleaser
   ```

3. **Re-run the fresh-clone test** after `v0.1.0` exists to confirm the shim downloads +
   checksum-verifies + execs the real release (the safe-fallback path is exercised only until the
   release is live). Do **not** test with a `-rc`/prerelease tag — `prerelease: auto` makes
   `/releases/latest/download` skip it and the shim will hit the guard.
4. **`bin/install-cursor-agent.sh` orphan decision** (604 LOC, zero references) — decide whether
   to delete it as dead-code cleanup. It is *not* part of this cutover (never migrated to the
   manifest), so it is a standalone maintainer call.
5. **Optional `--interactive` alias** — if `./bootstrap.sh --interactive` should keep working,
   add the one-line pre-filter that drops a leading `--interactive` before the `apply`-injection
   block (see "Shim decisions"). Otherwise it exits 2.
6. **Verify Forgejo #15 / #22 titles** map to "distribution/shim" and "cutover/LOC" before
   finalizing the `Closes #15` / `Closes #22` lines in the PR body (and/or the squashed commit
   footer). Suggested PR footer:

   ```
   ## Related issues

   Closes #15
   Closes #22

   Phase 7 (Distribution + shim) lands GoReleaser config, a GitHub Actions
   release-on-tag workflow, and a ~45-line `bootstrap.sh` shim that downloads the
   checksum-verified `dotf` binary from GitHub Releases and execs `dotf apply`,
   with a safe no-release fallback (clear actionable error + local-build escape
   hatch). Conservative cutover (Option A): only fully-absorbed, unreferenced
   install scripts are retired (currently none — the manifest remains a skeleton),
   so ~1.8k bash LOC (~12% of 14,815) are removed here; the ≥80% bash-LOC-retired
   metric remains a tracked target for the manifest-population / script-retirement
   follow-up.
   ```

---

## Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
| --- | --- | --- | --- |
| Fresh clone hits the shim before any release exists → hard crash / cryptic curl error | High (until v0.1.0) | Med | The `guard` path is the **designed** live path: a 404 routes to a multi-line actionable message (tag-a-release **and** `go build -o dotf ./cmd/dotf` escape hatch), non-zero exit — never a raw curl error. Validated by `./bootstrap.sh; echo $?`. |
| Shim asset name drifts from GoReleaser `name_template` → every download 404s | Low | High | Both derive from the literal `dotf_<os>_<arch>`; Task 2.1 depends on Task 1.1 (names finalized first) + the cross-file consistency check. `goreleaser release --snapshot` proves the produced names locally. |
| `--interactive` forwarded → `dotf apply` exits 2 | Med | Low | Documented: the guided TUI is already the default on a TTY-without-`--headless`; the flag is maintainer-facing. Optional one-line pre-filter if kept (Manual Follow-up 5). |
| Release published from the wrong remote (Forgejo `origin` vs GitHub `github`) | Med | Med | `release.github.owner/name: yandy-r/dotfiles` pins the target; the tag is pushed explicitly to `github` (Manual Follow-up 2); `release.yml` runs on GitHub Actions only. |
| ≥80% LOC metric assumed met by reviewers | Med | Low | Plan + PR body state plainly ~12% removed, ≥80% deferred; PRD L39 annotated as a full-cutover target. |
| `${TMP}` tmpdir leaks each run (`exec` prevents cleanup) | High | Low | Accepted trade-off (task specified `exec`); one small dir under `${TMPDIR:-/tmp}` per run. Documented; switchable to run-then-cleanup if undesirable. |
| Prebuilt binaries only for linux/darwin, but archives also build windows | Low | Low | GoReleaser builds windows (zip) for completeness; the shim `guard`s any non-linux/darwin `uname -s` with a clear message. |
| goreleaser-action / GoReleaser v2 schema mismatch | Low | Med | `version: 2` config ↔ `version: "~> v2"` action ↔ v7.2.3 action pin (v7 requires GoReleaser v2); `goreleaser check` in the local gate catches drift. |
| Retiring bootstrap body removes tool coverage `dotf apply` doesn't yet provide (manifest = 7 pkgs) | High | Med | Conservative scope + no-release fallback keep a fresh clone provisionable (guided/from source); the coverage gap and ≥80% metric are the tracked follow-up, not a Phase-7 regression claim. VM dogfood (Follow-up 1) is the gate before relying on the shim in anger. |

## Notes

- **Confidence: 8/10.** High on the three files (verbatim, cross-checked in brief R3) and the
  retirement analysis (brief R2, grep-proven). The −2 is residual on the live release: the shim's
  download/checksum/exec path can only be end-to-end-proven **after** `v0.1.0` is published
  (Manual Follow-up 3), and the VM dogfood is a manual gate. The snapshot build
  (`goreleaser release --snapshot`) de-risks the pipeline locally without a tag.
- **Parallel-then-cutover ends here for the entry point.** Phase 7 flips `bootstrap.sh` from the
  bash body to the shim; the `bin/install-*.sh` installers remain on disk (KEEP list) and the
  manifest stays a skeleton, so the deeper "bash retired" claim (≥80%) is intentionally the next
  phase of work, not this PR.
