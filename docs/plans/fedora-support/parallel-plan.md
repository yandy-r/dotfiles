<!-- markdownlint-disable MD010 MD038 -->
<!-- MD010: the Go blocks below are gofmt output and MUST use tabs; an implementer copy-pastes them verbatim. -->
<!-- MD038: several code spans quote TOML/YAML anchor lines whose leading indentation is load-bearing. -->

# Fedora Support Implementation Plan

`fedora-support` brings Fedora — specifically Fedora Asahi Remix (KDE Plasma 6) on an Apple M1 — to first-class
parity with the existing macOS / Arch / Debian support, through the identical `bootstrap.sh` → `dotf apply`
pipeline. The authority on _what_ and _why_ is [`design.md`](./design.md); this file is the executable _how_.

The plan is ordered by **what stops the user**, not by architectural tidiness. Two things must land before the
device is touched at all: the binary must actually download (`bootstrap.sh` never tries the tag-pinned URL it just
resolved), and `dotf link` must stop recursively deleting a populated `~/.config/nvim` with no backup. Everything
else — package correctness, the repo backend, sysconfig's Fedora domains, docs — follows behind those two.

## Settled user decisions (do not re-open)

1. **Scope is all nine workstreams** (WS0–WS8) plus the shell workstream the design references but never defines
   (see correction C36). Full Fedora parity in one PR.
2. **`docker-ce` on Fedora is the vendor `.repo`** via the new family-dispatching `repo` backend (ADR-5 / ADR-6 /
   WS7). Not `moby-engine`. Not skipped.
3. **`lutris` gets the `lutris-pkg` Fedora sibling** (ADR-4). Included, with its aarch64 caveat recorded.

## Hard constraints

- CI gate: `go vet ./...`, `go test -race ./...`, `golangci-lint v2.12.2 run ./...` all clean.
- Shell: shellcheck-clean; `[[ ]]` not `[ ]` (except `bootstrap.sh`, which is POSIX `sh`); quote every expansion;
  `$(...)` not backticks; guard optional tools with `command -v`.
- `customScriptBaseline` in `internal/manifest/ratchet_test.go:22` stays at **exactly 16** — an exact-match
  tripwire in both directions. Verified live count today: 16.
- No unnecessary tests. Design §7 defines the critical set of six; five are rows in existing tables. Where a
  golden/lock table must move, that movement **is** the signal — never loosen an assertion to make it pass.
- Conventional Commits. Soft ~500-line file cap.
- Must not regress macOS / Arch / Debian / openSUSE.

---

## Corrections applied after fact-checking

Every row below is a place the design (or the first-pass task breakdown) asserted something the worktree
contradicts. **The plan uses the "Reality" column.** Do not follow a design citation that appears here.

| #   | Design's claim                                                                              | Reality (verified in this worktree)                                                                                                                                              |
| --- | ------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| C1  | WS1 read range `/bootstrap.sh (:243-250)` for asset naming                                  | The anchoring comment starts at `bootstrap.sh:242`; the block is `:242-253`. `ASSET=` at `:245` is correct as stated.                                                             |
| C2  | WS1 inserts "an unauthenticated tag-pinned attempt" (helper unnamed)                        | Must use `try_auth_curl` (`bootstrap.sh:266-276`), which suppresses the expected 404. `auth_curl` (`:255-264`) does **not** suppress stderr and would leak a curl banner.         |
| C3  | design:9 / :140 — `github_download_release_file` "returns 1 at `bootstrap.sh:401`"           | `:401` is the `if [ -z "$GITHUB_AUTH_TOKEN" ]` test; `return 1` is on `:402`; `fi` on `:403`. The design's own WS1 body cites `:401-403` correctly.                               |
| C4  | design:139 — `.goreleaser.yml` "Nothing to do"                                              | True for the Fedora/arm64 build matrix. It also emits `dotf_windows_*.zip` (no `ignore:` block). Out of scope — do **not** "fix" the matrix.                                      |
| C5  | design:709 — "lift `installAPT` **verbatim**"                                                | No identifier `installAPT` exists anywhere in `internal/backend`. The body is `func (b *aptRepoBackend) Install` at `apt_repo.go:52-115`; the liftable body is `:61-114`.         |
| C6  | design:390 — github-release "pinned check at `:88-90`"                                      | `internal/backend/github_release.go:89-91` (`:88` is blank). Conclusion (sidecar-resolved `wantSHA` passes the check on a lock miss) is correct.                                  |
| C7  | design:33 — `driftReport` probe abort at "`:105-107`"                                       | `cmd/dotf/status.go:105` is the call; the abort guard is `:106-108`. The second citation `:117-120` is exact and is the one ADR-2 actually fixes.                                 |
| C8  | design:327 — `status.go:110-113` emits one `applicable=false` line                          | The branch is `:109-115`; the emitting statements are `:111-112`. Behavioural claim is correct.                                                                                  |
| C9  | design:399 — `packages.toml:548-563` ships `[sysconfig]` commented out                      | The commented table proper is `:559-563`; `:547-558` is banner + prose. Uncommenting the design's range verbatim would uncomment the banner. Also: no `# ssh_server` line exists. |
| C10 | design WS3 — "update the **three** golden locks"                                             | **Six** assertion sites break: `table_test.go` ×4 (Count, FacetCounts, **BackupEligibleSet**, MatchesPlaybook), `symlink_test.go` ×2 (IdenticalSet_PerOS, BackupAction), `apply_test.go` ×1. |
| C11 | design ADR-11 — `symlink.go:42` "recursive for dirs"                                        | The comment text is on `symlink.go:41`; `:42` is the bare `ActionRemove` identifier.                                                                                             |
| C12 | ADR-14 — one builtin-shadowing `alias history="dnf history"`                                | **Two**: `common/linux/aliases.sh:50` (dnf arm) and `:70` (yum arm). Fixing only the first leaves the builtin shadowed on a yum host.                                             |
| C13 | ADR-14 — the loader is three copy-pasted dispatch blocks                                    | Also `common/README.md:233-244`, which documents an `is_macos`/`is_linux` loader **the code has never had** — the likely origin of the bug. `:210-212` needs the overlay slot.    |
| C14 | ADR-12 — `FacetLinux` "resolving via the local `isLinuxFamily`"                             | Correct and load-bearing. The local predicate (`table.go:143-150`) excludes `platform.OSLinux`; `platform.IsLinuxFamily` includes it. Two golden rows pin the exclusion.          |
| C15 | WS5 — "prefer the Qt-free `busctl …` as the leading candidate" in `qdbusCandidates`         | `resolveArgv` (`reload.go:158-176`) substitutes **argv[0] only**. A `busctl` candidate would emit `busctl org.kde.KWin /KWin reconfigure` — invalid. Needs a full-argv table.     |
| C16 | WS1 — filter `pkgs` before `printApplyDryRun` (`:429`)                                      | `applyphases.go:418-427` also builds `names` from unfiltered `pkgs` and emits `packages=` at `:426`. Filter at the **call site** (`:385`) so both lines are honest.               |
| C17 | WS5 — mimeapps pins `google-chrome.desktop` for `text/html` + `http`/`https`                | **Five** chrome keys: `plasma/files/mimeapps.list` lines 7, 8, 13, 14, 15 (`text/html`, `about`, `http`, `https`, `unknown`). Lines 9–12 are four separate AI-app desktop ids.    |
| C18 | WS5 — "seed the system colour-scheme directory **list** from `$XDG_DATA_DIRS`"              | `systemColorSchemes` is a singular `string` (`plasma.go:303-305`) assigned as a plain string by the shared helper `internal/plasma/helpers_test.go:30`. Widening changes the type.|
| C19 | WS6 — the `*PlanNeedsSudo` flips' "test churn is itself the intentionality signal"          | Asymmetric. `FirewallPlanNeedsSudo` has golden rows (`linux_test.go:486-487`). `SSHServerPlanNeedsSudo` has **zero** tests repo-wide — its flip produces zero churn.               |
| C20 | WS6 — "`linux_test.go` — RedHat rows for locale, shell and `ufwInstallArgv`"                | No dedicated `ufwInstallArgv` test exists. `TestLinux_Firewall_UfwSelection` uses `FirewallAuto` and asserts exactly 3 steps; RedHat under `auto` routes to firewalld.            |
| C21 | design §3 / WS6 — `sshInstallArgv` "`:498` `dnf install -y openssh-server`"                 | `:498` is `case platform.OSRedHat:`; the argv literal is `:499`. Function span `:494-505` (doc `:493`) is right.                                                                  |
| C22 | design — `planLocale` `:115-153`                                                            | `:115-155`. `:153` is the `localectl set-locale` argv line; the closing brace is `:155`.                                                                                          |
| C23 | design §3 — "the sshd unit at `:484-490`"                                                   | `sshServiceName` is `:484-491` (doc `:483`).                                                                                                                                     |
| C24 | design — `printApplyDryRun` `:417-431`                                                      | `:417-453` (doc `:412-416`). The load-bearing `:429` anchor is correct; the range is not.                                                                                         |
| C25 | design §3 — CI is "One `ubuntu-latest` job"                                                 | **Two** Go workflows: `.github/workflows/go.yml` (48 lines) and `.forgejo/workflows/go.yml` (60 lines), each with a `release.yml` sibling. Every CI edit is a paired edit.         |
| C26 | design §7 test #3 lives in `internal/backend/pkg_test.go` "(existing table)"                | `pkg_test.go` has no applicability table. The table for this exact seam is `TestEngine_Applicable` in `internal/backend/applicable_test.go:24-53`, which the change breaks anyway. |
| C27 | ADR-9 — `dotf lock --platform redhat/arm64 --platform redhat/amd64`                         | `amd64` is the **ReleaseArch** spelling; the canonical `platform.Arch` is `x86_64` (`types.go:49`). Measured: `redhat/amd64` exits 1. Also must name `arch/x86_64` (see C28).     |
| C28 | ADR-9 — implicit that `dotf lock` merges with the committed file                            | `resolveLock` builds a **fresh** lock and `lockTargets` always prepends the detected host. Omitting `arch/x86_64` silently drops the committed `turso@arch/amd64` pin.            |
| C29 | ADR-3 — `yazi` uses `checksums_asset = "checksums.txt"`                                     | `sxyazi/yazi` publishes **no** combined sidecar — only per-asset `.sha256` bare-hash files, which `checksumFor` cannot parse (`len(fields) < 2`). Pin inline per-target `checksums`. |
| C30 | WS7 — "gate the `fingerprint` module `os = "arch"`"                                         | `manifest.Module` (`internal/manifest/module.go:15-28`) has **no `os` field**. The gate needs a schema field, its validation, and a `dotf setup` consumer.                        |
| C31 | design §1 Non-Goals lists `ROADMAP.md` as out of scope                                      | The orchestrator brief mandates it (WS8). Deliberate override, recorded here so it reads as a decision.                                                                           |
| C32 | WS5 — `/internal/backend/download.go` needs "(xz branch)"                                   | `ryanoasis/nerd-fonts` publishes a `.zip` beside every `.tar.xz`; `download.go` already extracts zip by magic bytes and Go's stdlib has no xz decoder. Use `.zip`; no new dep.     |
| C33 | (task breakdown) — "`bootstrap.sh` passes `LINUX_DESKTOP` as an env twin"                   | `bootstrap.sh` contains **zero** occurrences of `LINUX_DESKTOP` or `hypr`. The alias survives for ADR-12, `apply_test.go:717-748`, and README/CHANGELOG — not for the shim.       |
| C34 | (task breakdown) — `checkPlasma` has "no `home` in scope"                                   | `home, _ := os.UserHomeDir()` is at `cmd/dotf/doctor.go:60`, four lines above the `checkPlasma` call at `:64`.                                                                    |
| C35 | ADR-13 — the `font` backend as specified                                                    | Would download **unverified**: `VerifySHA256` (`download.go:117-120`) returns `nil` on an empty want. `validateGitHubRelease` enforces a checksum source (`validate.go:409-411`); the design's `validateFont` omits it. |
| C36 | design §6 — the workstream list                                                             | design.md:690 references a **"WS-shell"** that §6 never defines. ADR-14 (the loader fix that is user requirement #2) had no owner. This plan adds **WS-SHELL**.                    |
| C37 | `CLAUDE.md:28,458,778` + `AGENTS.md:22` — `bootstrap.sh` is "~307 lines"                    | It is **593 lines** today, and WS1-T1 adds to it. Compute `wc -l` at edit time; do not hardcode 593.                                                                              |

---

## Batch table

Batches are sequential; tasks **within** a batch are file-disjoint and may run in parallel.

| #   | Name                                            | Tasks                                                                              | Unblocks next                                                                                          |
| --- | ----------------------------------------------- | ---------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------ |
| 1   | First-boot unblock + symlink data safety        | WS0-DOC, WS1-T1, WS1-T2, WS1-T3, WS1-T4, WS3-SAFETY, WS3-T6, WS4-T2, WS6-T7        | A downloadable binary, a non-destructive `dotf link`, and CI path filters WS4-T1 builds on.            |
| 2   | ADR-2 applicability + shell loader + Fedora CI  | WS2-CORE, WS-SHELL, WS4-T1, WS4-T3, WS5-T1, WS5-T3                                 | `names` becomes the applicability declaration; `internal/manifest` and `internal/symlink` freed.       |
| 3   | Facet rename, WS2 follow-ups, sysconfig split   | WS2-T4, WS2-T5, WS3-FACETS, WS5-T4, WS6-T1                                         | `--hyprland` exists; `internal/manifest/{types,validate,class}.go` handed to WS7; `linux.go` under cap. |
| 4   | Desktop XDG paths, font backend, brew demotion  | WS5-T2, WS5-T5, WS6-T2, WS6-T9                                                     | `platform.Detect` settled for WS7-T1; `pkgInstallArgv` exists for WS6-T3/T5.                           |
| 5   | Fonts, locale, brew decline, os-release scan    | WS1-T5, WS5-T6, WS6-T3, WS6-T8, WS6-T10, WS7-T1                                    | `Info.VersionID` exists for the `repo` backend; the device checklist is written against final flags.   |
| 6   | chsh fallback, font docs, repo schema           | WS5-T7, WS6-T4, WS7-T2                                                             | `BackendRepo` + `DNFRepo` schema ready for the backend.                                                |
| 7   | firewalld install + the `repo` backend          | WS6-T5, WS7-REPO                                                                   | `repo` is registered and argv-tested; `manifest/packages.toml` can name it.                            |
| 8   | Privileged firewalld probe + Fedora manifest    | WS6-T6, WS7-MANIFEST                                                               | All code is final; documentation may transcribe it.                                                    |
| 9   | Documentation pass                              | WS7-T7, WS8-T1, WS8-T2, WS8-T3, WS8-T4                                             | `manifest-backends.md` final; docs index repaired.                                                     |
| 10  | Roadmap                                         | WS8-T5                                                                             | ROADMAP.md exists for the release notes to source.                                                     |
| 11  | Release-notes gate                              | WS8-T6                                                                             | The tag can be cut.                                                                                    |

### Why WS1-T5 is in batch 5, not batch 1

The device checklist is the **physical acceptance gate**. Written in batch 1 it would ship describing three bugs
fixed later in the same PR: the seven un-backed-up directories (fixed by WS3-SAFETY, batch 1), the brew-shadows-dnf
warning (fixed by WS6-T9, batch 4), and `--linux-desktop` as the primary flag name (renamed by WS3-FACETS,
batch 3). Batch 5 is the first batch where all three are settled.

---

## Primary owner / serialized writer chains

There is no file two tasks may write **concurrently**. Some files have a serialized chain of writers across
batches; the chain order is the invariant, and it is listed in full so it is checkable from this table alone.

| File                                     | Writer chain (batch order)                                       |
| ---------------------------------------- | ---------------------------------------------------------------- |
| `manifest/packages.toml`                 | WS2-CORE (2) → WS5-T6 (5) → WS7-MANIFEST (8)                      |
| `internal/backend/backend.go`            | WS2-CORE (2) — **sole writer**                                    |
| `internal/backend/pkg.go`                | WS2-CORE (2) — **sole writer**                                    |
| `internal/backend/applicable_test.go`    | WS2-CORE (2) → WS7-REPO (7)                                       |
| `internal/backend/register.go`           | WS5-T5 (4) → WS7-REPO (7)                                         |
| `internal/backend/register_test.go`      | WS5-T5 (4) → WS7-REPO (7)                                         |
| `internal/manifest/manifest_test.go`     | WS2-CORE (2) — **sole writer**                                    |
| `internal/manifest/types.go`             | WS5-T4 (3) → WS7-T2 (6)                                           |
| `internal/manifest/validate.go`          | WS5-T4 (3) → WS7-T2 (6)                                           |
| `internal/manifest/class.go`             | WS5-T4 (3) → WS7-T2 (6)                                           |
| `internal/symlink/table.go`              | WS3-SAFETY (1) → WS3-FACETS (3)                                   |
| `internal/symlink/table_test.go`         | WS3-SAFETY (1) → WS3-FACETS (3)                                   |
| `internal/symlink/symlink_test.go`       | WS3-SAFETY (1) → WS3-FACETS (3)                                   |
| `internal/symlink/apply.go`              | WS3-SAFETY (1) — **sole writer**                                  |
| `internal/symlink/apply_test.go`         | WS3-SAFETY (1) — **sole writer**                                  |
| `internal/sysconfig/linux.go`            | WS6-T1 (3) → WS6-T2 (4) → WS6-T4 (6) → WS6-T5 (7) → WS6-T6 (8)    |
| `internal/sysconfig/linux_firewall.go`   | WS6-T1 (3) → WS6-T2 (4) → WS6-T5 (7) → WS6-T6 (8)                 |
| `internal/sysconfig/linux_locale.go`     | WS6-T1 (3) → WS6-T3 (5)                                           |
| `internal/sysconfig/linux_test.go`       | WS6-T3 (5) → WS6-T4 (6) → WS6-T5 (7) → WS6-T6 (8)                 |
| `internal/platform/platform.go`          | WS1-T2 (1) → WS6-T9 (4) → WS7-T1 (5) → WS7-MANIFEST (8)           |
| `internal/platform/platform_test.go`     | WS1-T2 (1) → WS6-T9 (4)                                           |
| `cmd/dotf/applyphases.go`                | WS1-T3 (1) → WS3-FACETS (3) → WS5-T2 (4) → WS6-T8 (5)             |
| `cmd/dotf/apply.go`                      | WS6-T8 (5) — **sole writer**                                      |
| `cmd/dotf/link.go`                       | WS3-FACETS (3) — **sole writer**                                  |
| `cmd/dotf/applyopts.go`                  | WS3-FACETS (3) → WS5-T6 (5)                                       |
| `cmd/dotf/doctor.go`                     | WS5-T2 (4) → WS6-T10 (5)                                          |
| `cmd/dotf/doctor_test.go`                | WS2-CORE (2) → WS5-T2 (4)                                         |
| `.github/workflows/go.yml`               | WS1-T4 (1) → WS4-T1 (2)                                           |
| `.forgejo/workflows/go.yml`              | WS1-T4 (1) → WS4-T1 (2)                                           |
| `common/platform.sh`                     | WS1-T2 (1) → WS-SHELL (2)                                         |
| `common/exports.sh`                      | WS-SHELL (2) — **sole writer**                                    |
| `common/linux/aliases.sh`                | WS-SHELL (2) — **sole writer**                                    |
| `docs/reference/manifest-backends.md`    | WS5-T7 (6) → WS7-T7 (9)                                           |
| `docs/fedora-first-run.md`               | WS1-T5 (5) — **sole writer**                                      |

---

## Batch 1 — First-boot unblock and symlink data safety

Everything here is dependency-free and file-disjoint. It removes the two things that stop the user: the binary
never downloading, and `dotf link` recursively deleting a populated `~/.config/nvim` with no backup.

### WS0-DOC — Commit the design document Depends on [none]

**Files to Create** — none (the file exists, untracked).

**Files to Modify** — none.

**Files to Commit**

- `/docs/plans/fedora-support/design.md`
- `/docs/plans/fedora-support/parallel-plan.md`

**Instructions**

`git status` shows `?? docs/plans/fedora-support/`. The authoritative design the whole PR derives from is untracked,
and WS8-T5/WS8-T6 are instructed to source content from it. Commit both files with type `docs(internal):` — repo
policy (CLAUDE.md) requires that type for anything under `docs/plans`, so it stays out of release notes.

Change no other file. Do not edit `design.md` itself; the corrections table above is the errata surface.

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support && git status --porcelain docs/plans/fedora-support/ | wc -l   # must print 0 after committing
```

### WS1-T1 — `bootstrap.sh`: tag-pinned release-asset fallback + Fedora prerequisite hint Depends on [none]

**READ THESE BEFORE TASK**

- `/bootstrap.sh` (`:242-253`, `:255-276`, `:378-421`, `:442-453`)
- `/internal/selfupdate/selfupdate_test.go` (`:590-650`)
- `/.goreleaser.yml`

**Files to Modify**

- `/bootstrap.sh`

**Instructions**

`bootstrap.sh` is POSIX `sh` with `set -eu` and uses plain global assignments with **no `local`** — follow that
style exactly.

PROBLEM (verified): `github_download_release_file` (`:382-421`) resolves the newest release tag at `:387`
(`tag="$(github_resolve_latest_tag || true)"`), optionally tries `gh release download` at `:395-399`, then at
`:401-403` bails with `return 1` whenever `$GITHUB_AUTH_TOKEN` is empty — **without ever trying the plain
tag-pinned asset URL it just resolved**. Combined with `.goreleaser.yml:73`'s `prerelease: auto`, `BASE`
(`/releases/latest/download`, `:246`) 404s on a prerelease-only repo, and `ALT_BASE` (`:247`) points at a literal
tag named `latest` that GoReleaser never creates. Net effect on a fresh Fedora box with no `gh` and no token: the
binary never downloads.

**EDIT 1 — insert the tag-pinned attempt.** Match this exact block (`:395-403`, unique in the file):

```sh
  if command -v gh >/dev/null 2>&1; then
    if gh release download "$tag" --repo "$REPO_SLUG" --pattern "$name" -D "$dir" >/dev/null 2>&1; then
      [ -f "$dest" ] && return 0
    fi
  fi

  if [ -z "$GITHUB_AUTH_TOKEN" ]; then
    return 1
  fi
```

Insert between the `fi` closing the `gh` block and the `if [ -z "$GITHUB_AUTH_TOKEN" ]` bail:

```sh
  # Tag-pinned public asset URL. This is the ONLY path that works with neither
  # `gh` nor a token: BASE (/releases/latest/download) 404s whenever the newest
  # release is a prerelease (.goreleaser.yml sets `prerelease: auto`), and
  # ALT_BASE points at a literal tag named "latest" that GoReleaser never
  # creates. try_auth_curl (NOT auth_curl) so the expected 404 on a private
  # repo stays silent and falls through to the token+asset-id path below.
  if try_auth_curl "${REPO_ORIGIN}/releases/download/${tag}/${name}" -o "$dest"; then
    return 0
  fi

```

Use `try_auth_curl` (`:266-276` — it suppresses stderr on the expected 404), **not** `auth_curl` (`:255-264`, which
leaks a curl banner). `name` and `dest` are the in-scope globals set at `:383-384`; introduce no new variables and
do not use `local`.

**EDIT 2 — update the function doc comment** (`:378-381`) so it names the new middle step: resolve the tag, then try
in order — download by tag (`gh`), the tag-pinned public asset URL (no `gh` and no token needed), and finally the
asset id from `GET …/releases/tags/$tag` (token + python3). Keep the closing "No sed over the full /releases list."

**EDIT 3 — extend the guard hint.** The heredoc inside `github_guard_hint` (`:442-453`) uses an **unquoted** `<<EOF`,
so `$` expands and backticks execute (existing text escapes them as `` \` ``). Append one short paragraph before the
closing `EOF`, avoiding both by using single quotes:

```
On Fedora/RHEL the shim itself needs only curl and tar (both in @core), but
release discovery needs gh or python3: 'sudo dnf install -y gh' (or
'sudo dnf install -y python3').
```

**HARD CONSTRAINTS**

- Do **not** touch `ASSET="${BINARY}_${OS}_${ARCH}.tar.gz"` at `:245` — `internal/selfupdate/selfupdate_test.go:598`
  string-matches it verbatim.
- Do **not** remove `ALT_BASE` (`:247` — still the correct probe for older Gitea/Forgejo instances), the
  `--install-go` path (`INSTALL_GO=0` `:32`, `GO_BOOTSTRAP_VERSION="1.26.5"` `:34`, `install_go_toolchain()` `:456`,
  `offer_source_build_or_exit()` `:500`), or the `releases/tags/${tag}` API call at `:410`.
- These six substrings MUST survive (pinned by `selfupdate_test.go:632-639`): `github_resolve_latest_tag`,
  `releases/tags/`, `--install-go`, `.local/go`, `GO_BOOTSTRAP_VERSION`, `offer_source_build_or_exit`. The **new**
  URL uses `releases/download/${tag}`, which does **not** satisfy the `releases/tags/` marker — that marker survives
  via `:410` and the reworded comment.
- Do **not** change the OS/ARCH detection block at `:157-169`; it is already correct for Fedora Asahi
  (`aarch64` → `arm64` → `dotf_linux_arm64.tar.gz`).

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && sh -n bootstrap.sh \
  && shellcheck --severity=warning bootstrap.sh \
  && go test ./internal/selfupdate/... \
  && for m in github_resolve_latest_tag 'releases/tags/' '--install-go' '.local/go' GO_BOOTSTRAP_VERSION offer_source_build_or_exit; do \
       grep -qF -- "$m" bootstrap.sh || { echo "FAIL: missing marker $m"; exit 1; }; done \
  && grep -qF 'releases/download/${tag}/${name}' bootstrap.sh \
  && grep -qF 'ASSET="${BINARY}_${OS}_${ARCH}.tar.gz"' bootstrap.sh \
  && echo WS1-T1-OK
```

> Correction applied (critique minor): the first-pass verify used a `for … do :; done` loop whose body was a no-op,
> and had `--install-go` word-split into `--` and `install-go`. The loop above actually asserts, with `-F` and `--`
> so the flag-shaped literal is not parsed as a `grep` option.

### WS1-T2 — Asahi os-release ID tokens in the Go detector + its shell mirror (critical test #1) Depends on [none]

**READ THESE BEFORE TASK**

- `/internal/platform/platform.go` (`:84-112`)
- `/internal/platform/platform_test.go` (`:81-153`, `:266-278`)
- `/common/platform.sh` (`:50-87`, `:102-109`)

**Files to Modify**

- `/internal/platform/platform.go`
- `/common/platform.sh`
- `/internal/platform/platform_test.go`

**Instructions**

This is **critical test #1** and its production change. All three files MUST land in **one commit** — the shell file
is a string-locked mirror of the Go detector (`internal/platform/types.go:6-16`), so it must never drift, and the
test rows are the only thing making the new literals load-bearing.

Context (ADR-1): Fedora stays `platform.OSRedHat`; do **not** add `OSFedora`/`OSAsahi`. Four surveys disagree on
Fedora Asahi Remix's os-release `ID`: `fedora`, `fedora-asahi-remix`, or the archived prototype spelling
`asahi-fedora-remix`. Under the last two, detection today succeeds only by accident through the
`strings.Contains(idLike, "fedora")` fallback at `platform.go:103`.

**EDIT 1** — `internal/platform/platform.go`, inside `osFamilyFromID` (declared `:87`, closes `:112`). Match `:91`
exactly (tabs, not spaces):

```go
	case "fedora", "rhel", "centos", "rocky", "almalinux":
```

replace with:

```go
	case "fedora", "fedora-asahi-remix", "asahi-fedora-remix", "rhel", "centos", "rocky", "almalinux":
```

Change nothing else; in particular leave the `ID_LIKE` switch at `:100-111` alone.

**EDIT 2** — `common/platform.sh`, inside `detect_os()` (`:50-87`). Match `:60` exactly (20 spaces of indent):

```sh
                    fedora|rhel|centos|rocky|almalinux) echo "redhat" ;;
```

replace with:

```sh
                    fedora|fedora-asahi-remix|asahi-fedora-remix|rhel|centos|rocky|almalinux) echo "redhat" ;;
```

Leave the `ID_LIKE` fallback (`:65-71`) and `detect_arch()` (`:102-109`) unchanged — arm64 is already correct.

**EDIT 3** — `internal/platform/platform_test.go`, `TestDetectOS` (func `:81`, table `:82-153`). Insert three rows
immediately after the existing `"rocky maps to redhat"` row (`:95-99`):

```go
		{
			name:      "fedora maps to redhat",
			osRelease: "ID=fedora\n",
			want:      OSRedHat,
		},
		{
			name:      "fedora-asahi-remix maps to redhat without ID_LIKE",
			osRelease: "ID=fedora-asahi-remix\n",
			want:      OSRedHat,
		},
		{
			name:      "asahi-fedora-remix maps to redhat without ID_LIKE",
			osRelease: "ID=asahi-fedora-remix\n",
			want:      OSRedHat,
		},
```

**DELIBERATE STRENGTHENING — do not "correct" it back.** Design §7 row 1 words the two Asahi rows as carrying
`ID_LIKE=fedora`. Such rows pass **before** the `platform.go` change (the `ID_LIKE` fallback at `:103` already
returns `OSRedHat`), so they would pin nothing. Written **without** `ID_LIKE`, each new literal is independently
load-bearing: delete either token from `:91` and a named row fails. The bare `ID=fedora` row is design row 1
verbatim. The `ID_LIKE=fedora` path stays covered by the existing `"rocky maps to redhat"`
(`ID_LIKE="rhel centos fedora"`) and `"unknown ID falls back to ID_LIKE rhel"` rows.

**EDIT 4** — `TestDistroName` (func `:266`, table `:267-278`). One row after `"single-quoted ID"` (`:275`):

```go
		{name: "fedora-asahi-remix", osRelease: "ID=fedora-asahi-remix\n", want: "fedora-asahi-remix"},
```

Add no other test. Do not add a shell-side test harness (none exists). Do not touch
`internal/platform/types.go` (`ReleaseArchAliases` `:76-87` already covers `{aarch64, arm64}`) and do **not**
reorder `detectPkgMgr`'s candidate slice — the brew demotion is WS6-T9.

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && gofmt -l internal/platform \
  && go vet ./internal/platform/... \
  && go test -race -run 'TestDetectOS|TestDistroName' -v ./internal/platform/... \
  && go test -race ./internal/platform/... \
  && shellcheck --severity=warning common/platform.sh \
  && golangci-lint run ./internal/platform/...
```

### WS1-T3 — Make `dotf apply --dry-run` honest Depends on [none]

**READ THESE BEFORE TASK**

- `/cmd/dotf/applyphases.go` (`:340-360`, `:380-390`, `:412-453`)
- `/internal/backend/backend.go` (`:141-157`), `/internal/backend/register.go`
- `/cmd/dotf/apply_helpers_test.go`

**Files to Modify**

- `/cmd/dotf/applyphases.go`

**Instructions**

PROBLEM (verified): `printApplyDryRun` (signature `:417`, body to `:453`) builds `names` from the **unfiltered**
`pkgs` at `:418-421`, emits `packages=` at `:426`, then groups the same unfiltered slice at `:429`. Nothing in the
dry-run path ever calls `engine.Applicable`. On Fedora this prints `plan.apt-repo=docker-ce` for a package the real
apply silently skips (`aptRepoBackend.applicable`, `internal/backend/apt_repo.go:48-50`, gates on
`info.PkgMgr == platform.PkgMgrAPT`). `dotf apply --dry-run` is the safety surface the whole device-first rollout
rests on, so it must be honest.

Per correction **C16**: filtering only at `:429` is not enough — `packages=` at `:426` reads the same slice. Filter
`pkgs` at the **call site** so both lines are corrected at once.

**EDIT 1 — imports** (`:3-21`): add `"github.com/yandy-r/dotfiles/internal/privilege"`, keeping the existing
grouping/order. `backend` is already imported at `:11`.

**EDIT 2 — `applyDryRun`.** Match (`:353-357`):

```go
	pkgs, ferr := filterPackages(m, pkgs, opts.Only, opts.Skip)
	if ferr != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", progName, ferr)
		return 2
	}
```

Insert immediately after:

```go

	// Applicability filter: the preview must reflect what a real apply would
	// actually do on THIS host, not what the manifest declares. Without it the
	// dry run promises installs the real run silently skips (e.g.
	// plan.apt-repo=docker-ce on a dnf host). Applicable is a pure read-only
	// gate, so this engine never runs a command and never primes sudo —
	// the same construction status.go uses for its probe engine.
	engine := backend.Default(r, privilege.NewSudo(), privilege.NewFakeDropper(), repoRoot, nil)
	pkgs, notApplicable := partitionApplicableForPreview(stderr, engine, pkgs, info)
```

**EDIT 3 — new helper**, immediately before `printApplyDryRun`'s doc comment (`:412`):

```go
// partitionApplicableForPreview splits the previewed package set into the
// packages whose backend can actually run on this host and the names of the
// ones that cannot. A probe error is a stderr warning that KEEPS the package
// in the applicable set: a preview must never hide planned work because a
// probe was flaky, and hiding is the dangerous direction for a dry run.
func partitionApplicableForPreview(stderr io.Writer, engine *backend.Engine, pkgs []manifest.Package, info platform.Info) ([]manifest.Package, []string) {
	applicable := make([]manifest.Package, 0, len(pkgs))
	var notApplicable []string
	for _, pkg := range pkgs {
		ok, err := engine.Applicable(pkg, info)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: package %q: %v; previewing as applicable\n", progName, pkg.Name, err)
			applicable = append(applicable, pkg)
			continue
		}
		if !ok {
			notApplicable = append(notApplicable, pkg.Name)
			continue
		}
		applicable = append(applicable, pkg)
	}
	return applicable, notApplicable
}
```

**EDIT 4 — signature + the new line.** Change `:417` to take `notApplicable []string` immediately after
`pkgs []manifest.Package`:

```go
func printApplyDryRun(stdout io.Writer, intents []string, pkgs []manifest.Package, notApplicable []string, idPlan, fwPlan, sshPlan sysconfig.Plan, linkPlan *symlink.Plan, plasmaPlan *plasma.Plan) error {
```

After the existing `byBackend` loop (`:430-432`), append:

```go
	if len(notApplicable) > 0 {
		lines = append(lines, "plan.not-applicable="+strings.Join(notApplicable, ","))
	}
```

Order is manifest-declaration order, matching how `packages=` is built. Emit only when non-empty, matching how
`plan.<backend>=` lines are emitted only for backends that have packages.

**EDIT 5 — call site** at `:385`:

```go
	if err := printApplyDryRun(stdout, intents, pkgs, notApplicable, idPlan, fwPlan, sshPlan, linkPlan, plasmaPlan); err != nil {
```

**EDIT 6** — extend `printApplyDryRun`'s doc comment (`:412-416`) with one sentence: `pkgs` is already
applicability-filtered, and the excluded names are reported on the single `plan.not-applicable=` line.

**CONSTRAINTS**

- **No new test.** Existing dry-run tests use only `toolchain` and `custom-script` backends
  (`cmd/dotf/apply_helpers_test.go:34,47,59,72,87,94`), neither of which implements `platformGated`, so they stay
  applicable and every existing assertion must pass unchanged. If one fails, the filter is wrong — fix the filter.
- The dry-run path must still never invoke sudo: `backend.Default` (`register.go:14-26`) is pure construction
  (`_ = sudo` at `:15`) and `Engine.Applicable` only consults `osGateAllows` plus the optional interfaces. Do not
  call `sudo.Prime`.
- `applyphases.go` is 454 lines today; these edits land it near 485, inside the ~500-line soft cap. Do not split.

**Probe-error policy — deliberate asymmetry.** This filter is **fail-open** (keep the package, warn on stderr),
the opposite of `cmd/dotf/status.go:106-108` and `cmd/dotf/doctor.go:211-214`, which abort with exit 1. Rationale:
hiding planned work is the dangerous direction for a preview, and manifest validation already rejects unknown
backends, so the error is unreachable in practice.

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && gofmt -l cmd/dotf && go build ./... && go vet ./... \
  && go test -race ./cmd/dotf/... && golangci-lint run ./cmd/dotf/...
```

### WS1-T4 — CI path filters: trigger the Go workflows on `bootstrap.sh` and the goreleaser configs Depends on [none]

**READ THESE BEFORE TASK**

- `/.github/workflows/go.yml`, `/.forgejo/workflows/go.yml`
- `/internal/selfupdate/selfupdate_test.go` (`:590-650`)

**Files to Modify**

- `/.github/workflows/go.yml`
- `/.forgejo/workflows/go.yml`

**Instructions**

PROBLEM (verified): both Go workflows filter on `**.go`, `go.mod`, `go.sum`, `manifest/**`, the workflow itself and
`.golangci.yml`. `bootstrap.sh` and the goreleaser configs are excluded, so `TestAssetNaming_MatchesBootstrap`
(`selfupdate_test.go:590-650`) — which reads `../../bootstrap.sh` and pins the three-way asset-name contract plus six
markers — never runs on a PR that touches only `bootstrap.sh`. WS1-T1 changes exactly that file.

**EDIT A — `.github/workflows/go.yml`.** The 6-entry `paths:` list appears **twice** identically (pull_request
`:5-11`, push `:15-21`). Replace-all on this two-line fragment (which occurs exactly twice):

```yaml
      - 'manifest/**'
      - '.github/workflows/go.yml'
```

with:

```yaml
      - 'manifest/**'
      - 'bootstrap.sh'
      - '.goreleaser.yml'
      - '.goreleaser.forgejo.yml'
      - '.github/workflows/go.yml'
```

**EDIT B — `.forgejo/workflows/go.yml`.** Same shape, but the self-reference is `.forgejo/workflows/go.yml`
(pull_request paths `:13-19`, push paths `:23-29`). Do **not** copy the GitHub block wholesale. Replace-all on:

```yaml
      - 'manifest/**'
      - '.forgejo/workflows/go.yml'
```

with:

```yaml
      - 'manifest/**'
      - 'bootstrap.sh'
      - '.goreleaser.yml'
      - '.goreleaser.forgejo.yml'
      - '.forgejo/workflows/go.yml'
```

**CONSTRAINTS**

- Both `.goreleaser.yml` and `.goreleaser.forgejo.yml` exist at the repo root.
- Change nothing else: not the job matrix, not `runs-on: ubuntu-latest`, not the pinned action SHAs, and not the
  Forgejo Lint step's raw-shell golangci-lint v2.12.2 install (its header comment `:3-9` documents why it cannot use
  `golangci-lint-action`).
- Do **not** add the Fedora container job or `**.sh`/`common/**`/`bin/**` here — WS4-T1 and WS4-T2 own those, and
  keeping this change minimal is what makes WS4-T1 apply cleanly on top.
- 6-space indent for list items under `paths:`; single-quoted scalars; keep prettier-clean.

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && npx --no-install prettier --check .github/workflows/go.yml .forgejo/workflows/go.yml \
  && for f in .github/workflows/go.yml .forgejo/workflows/go.yml; do \
       for p in "- 'bootstrap.sh'" "- '.goreleaser.yml'" "- '.goreleaser.forgejo.yml'"; do \
         n=$(grep -cF "$p" "$f"); [ "$n" = 2 ] || { echo "FAIL $f [$p] = $n, want 2"; exit 1; }; done; done \
  && echo WS1-T4-OK
```

### WS3-SAFETY — ADR-11: directory backup executor + `BackupEligible` on seven dirs + critical test #4 Depends on [none]

**Merged from WS3-T1 + WS3-T2.** Mandated atomic: WS3-T2 sets `BackupEligible` on seven `IsDir` entries, and without
WS3-T1's `copyTree`, `doBackup` → `copyFile` → `os.ReadFile` returns `EISDIR` and `Engine.Apply` **hard-fails** on
any host that already has a real `~/.config/nvim`. Committing the flag alone converts a silent data-loss bug into a
loud apply failure on the user's daily driver.

**READ THESE BEFORE TASK**

- `/internal/symlink/apply.go` (`:69-82`, `:143-158`)
- `/internal/symlink/symlink.go` (`:195-215`; note the "recursive for dirs" comment is on `:41`, per C11)
- `/internal/symlink/table.go` (`:53-55`, `:88-133`)
- `/internal/symlink/table_test.go`, `/internal/symlink/symlink_test.go`, `/internal/symlink/apply_test.go`,
  `/internal/symlink/helpers_test.go`

**Files to Modify**

- `/internal/symlink/apply.go`
- `/internal/symlink/table.go`
- `/internal/symlink/helpers_test.go`
- `/internal/symlink/table_test.go`
- `/internal/symlink/symlink_test.go`
- `/internal/symlink/apply_test.go`

**Instructions**

ADR-11 / design R8: `symlink.go:205-209` emits `ActionBackup` only `if spec.BackupEligible && opts.Backup`, then an
**unconditional** `ActionRemove` documented at `symlink.go:41` as "recursive for dirs". Only the 14 single-file
entries carry the flag, so `dotf link --backup` today `rm -rf`s a populated `~/.config/nvim` with no backup.

**PART 1 — `internal/symlink/apply.go` (pure add; every existing test must still pass unchanged).**

Import `"io/fs"`. Rewrite `doBackup` (`:69-82`) so that after `dst := filepath.Join(backupDir, act.Spec.Dest)` it
branches on `info.IsDir()` and delegates to a new `copyTree(act.DestAbs, dst)`, wrapping its error with the
**identical** `"symlink: backup %q: %w"` message so the error surface is unchanged. Update the doc comment to say it
copies a real file **or a directory tree**, preserving the `$HOME`-relative path and permission bits, cite ADR-11,
and note that `ActionBackup` is only ever planned from `symlink.go`'s `default:` Lstat arm, so `DestAbs` is never a
symlink and `os.Stat` following links is not a hazard.

Add a package-level `copyTree(src, dst string) error` immediately after `copyFile` (`:146-158`, the last function).
Implement with `filepath.WalkDir`:

- propagate a non-nil walk `err` immediately;
- `rel, rerr := filepath.Rel(src, p)`; propagate; `target := filepath.Join(dst, rel)`;
- `info, ierr := d.Info()` (WalkDir does not follow symlinks, so this is Lstat info); propagate;
- `d.IsDir()` → `os.MkdirAll(target, dirPerm)`. Use the package const `dirPerm` (0o755), **not** the source dir's
  own mode — WalkDir visits a parent before its children, so recreating a restrictive mode (e.g. 0o500) would make
  the copy of its children fail. Comment that.
- `info.Mode()&os.ModeSymlink != 0` → `os.Readlink(p)`, `os.MkdirAll(filepath.Dir(target), dirPerm)`,
  `os.Symlink(link, target)`. Recreate rather than follow, so a plugin-link-heavy config dir cannot fan out into an
  unbounded copy. Comment the rationale.
- `info.Mode().IsRegular()` → `copyFile(p, target, info.Mode().Perm())` (copyFile already MkdirAlls the parent).
- `default:` → `return nil` (sockets/fifos/devices skipped, commented).

Doc-comment `copyTree` in house style. Do not change `copyFile`, `doLink`, `doCopy`, `applyAction`, `Apply`, or
`requiredDirs`. `apply.go` goes ~158 → ~205 lines, inside the cap.

**PART 2 — `internal/symlink/table.go`.** Append `, BackupEligible: true` (field order in this file is Src, Dest,
IsDir, Copy, Facet, BackupEligible — append after Facet) to exactly these **seven** entries and nothing else:

| Line   | Entry                                                    |
| ------ | -------------------------------------------------------- |
| `:91`  | `{Src: "nvim", Dest: ".config/nvim", …}`                 |
| `:92`  | `{Src: "wezterm", Dest: ".config/wezterm", …}`           |
| `:93`  | `{Src: "ghostty", Dest: ".config/ghostty", …}`           |
| `:94`  | `{Src: "mise", Dest: ".config/mise", …}`                 |
| `:95`  | `{Src: "alacritty", Dest: ".config/alacritty", …}`       |
| `:119` | `{Src: "bin", Dest: ".bin", …}`                          |
| `:124` | `{Src: "dotenv", Dest: ".config/direnv", …}`             |

Update the `BackupEligible` field doc (`:53-55`, currently "true only for the 14 shell/common/prompt file entries
that the playbook's backup step covers") to say **21** entries: the 14 single-file entries plus the seven managed
app-config/utility **directories** added by ADR-11, and record that this is a **deliberate divergence** from the
retired playbook's backup step — a safety flag that does not cover the majority of what it removes is a broken
contract. Do **not** touch `aerospace` (`:98`, FacetMacOS) or the hypr/waybar desktop dirs (flag-gated and already
forced to `Backup: true` at `cmd/dotf/applyphases.go:300`).

**PART 3 — `internal/symlink/helpers_test.go`.** Add beside `countKind`:

```go
// planIndexOf returns the position of the first action in p matching dest and
// kind, so phase ordering (backup before remove) is assertable.
func planIndexOf(p *Plan, dest string, kind ActionKind) (int, bool) {
	for i, a := range p.Actions {
		if a.Spec.Dest == dest && a.Kind == kind {
			return i, true
		}
	}
	return 0, false
}
```

**PART 4 — `internal/symlink/table_test.go` (two golden locks).**

1. `TestTable_BackupEligibleSet`: append the seven new dests to `want` (`:43-48`) — `.config/nvim`,
   `.config/wezterm`, `.config/ghostty`, `.config/mise`, `.config/alacritty`, `.bin`, `.config/direnv`. The
   `len(got) != len(want)` check at `:50-52` goes 14 → 21. Comment the new group with ADR-11.
2. `TestTable_MatchesPlaybook` inline golden (`:64-106`): add `BackupEligible: true` to the same seven rows
   (`:79-83`, `:97`, `:100`). Comparison at `:113` is struct equality — byte-identical to `table.go`.

**PART 5 — `internal/symlink/symlink_test.go`, `TestPlan_BackupAction` (`:187-210`).** The setup comment at `:191`
("Real file at a backup-eligible dest and a non-eligible dest.") and the negative assertion at `:204-206`
(`unexpected ActionBackup for non-eligible .config/nvim`) are both now wrong. Rewrite the body so it:

- seeds three real files: `.bashrc` (file-eligible), `.config/nvim` (now dir-entry-eligible), `.ansible.cfg`
  (a genuinely still-non-eligible `FacetAll` entry, `table.go:123`);
- asserts `ActionBackup` **is** planned for `.bashrc`;
- asserts `ActionBackup` **is** planned for `.config/nvim` (the inverted assertion — ADR-11);
- asserts `ActionBackup` is **not** planned for `.ansible.cfg` (preserves the negative case);
- keeps the existing `ActionRemove for .config/nvim` assertion;
- updates the `:191` comment to describe the three-dest setup.

**PART 6 — `internal/symlink/apply_test.go`.** Replace the single subtest
`non-eligible-real-file-removed-without-backup` (`:120-140`) — whose name **and** whose `:134` assertion both invert
— with **two** subtests in the same `TestApply_Backup` table:

**(6a) CRITICAL TEST #4** — `t.Run("eligible-directory-backed-up-then-linked", …)`: with `home := t.TempDir()`, seed
a populated real directory via `writeFile(t, filepath.Join(home, ".config/nvim/lua/user/init.lua"), []byte("sentinel"))`.
Plan `DefaultTable()` for `platform.Info{OS: platform.OSDebian}` with `Options{Backup: true}`. Assert, in order:

- `bi, ok := planIndexOf(plan, ".config/nvim", ActionBackup)` → ok (t.Fatalf otherwise);
- `ri, ok := planIndexOf(plan, ".config/nvim", ActionRemove)` → ok;
- **`bi < ri`** — the backup must be ordered before the recursive remove that destroys the tree; this ordering is
  the entire point of the test;

then `eng.Apply(plan, opts)` and assert the sentinel survives:
`os.ReadFile(filepath.Join(plan.BackupDir, ".config/nvim/lua/user/init.lua"))` equals `sentinel` (`bytes.Equal`), and
`isSymlink(t, filepath.Join(home, ".config/nvim"))` is true. Doc-comment the subtest with ADR-11 / design §7 test 4
and the fact that the first host to exercise the directory-copy path is a populated daily driver.

**(6b)** — keep `non-eligible-real-file-removed-without-backup` but repoint it at `.ansible.cfg`: seed
`writeFile(t, filepath.Join(home, ".ansible.cfg"), []byte("user ansible cfg"))`, apply with `Options{Backup: true}`,
assert `os.Stat(filepath.Join(plan.BackupDir, ".ansible.cfg"))` `IsNotExist` and that `~/.ansible.cfg` is a symlink.

Add no other test. Do not touch `cmd/dotf` in this task.

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && gofmt -l internal/symlink && go build ./... && go vet ./internal/symlink/... \
  && go test -race ./internal/symlink/... ./cmd/dotf/...
```

> The worktree is clean and `go test ./...` is green at HEAD, so **every** test must pass. The first-pass
> `| grep -v 'TestCheckPackages'` filter was based on a stale "worktree is dirty" claim and is removed (critique
> minor): under this batch plan WS2-CORE has not run yet, so there is no pre-existing failure to mask.

### WS3-T6 — Remove the hardcoded `/home/yandy` path from `wifi-tui.desktop` Depends on [none]

**READ THESE BEFORE TASK**

- `/launchers/applications/wifi-tui.desktop`, `/launchers/applications/alacritty-float.desktop`
- `/common/paths.sh` (`:309`), `/hypr/bindings.lua` (`:185`)

**Files to Modify**

- `/launchers/applications/wifi-tui.desktop`

**Instructions**

`wifi-tui.desktop:8` hardcodes the author's home directory, so the launcher is dead for every other user. It is the
only remaining `/home/yandy` literal outside docs.

Match this exact 3-line block (`:6-8`):

```ini
TryExec=alacritty
Terminal=false
Exec=alacritty --class wifi-tui -e /home/yandy/.bin/wifi-tui.sh
```

replace with:

```ini
TryExec=alacritty
Terminal=false
Exec=alacritty --class wifi-tui -e wifi-tui.sh
```

Leave `TryExec=alacritty` exactly as it is — it already gates the entry on the terminal being installed, matching
`alacritty-float.desktop`. Do **not** set `TryExec=wifi-tui.sh`: a desktop session's PATH is not the shell's, and a
failing `TryExec` **hides** the entry entirely.

Why a bare PATH lookup and not a `$HOME` expansion: the Desktop Entry spec does not expand `~` or `$HOME` in `Exec`
and reserves both `$` and the double-quote character, so an `sh -lc` wrapper needs escaping that is easy to get
subtly wrong. `~/.bin` is prepended to PATH by `common/paths.sh:309` and the `bin` → `.bin` symlink is a `FacetAll`
table entry, so `wifi-tui.sh` resolves for any user. The Hyprland keybinding at `hypr/bindings.lua:185` keeps its own
`sh -lc 'exec alacritty --class wifi-tui -e "$HOME/.bin/wifi-tui.sh"'` form — that path runs through a login shell
and is unaffected; do not edit it.

Do not touch `arch-update.desktop` — its CachyOS branding is handled by WS3-FACETS' `FacetArchDesktop` retag.

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && ! grep -q '/home/yandy' launchers/applications/wifi-tui.desktop \
  && grep -qx 'Exec=alacritty --class wifi-tui -e wifi-tui.sh' launchers/applications/wifi-tui.desktop \
  && grep -qx 'TryExec=alacritty' launchers/applications/wifi-tui.desktop \
  && { command -v desktop-file-validate >/dev/null && desktop-file-validate launchers/applications/wifi-tui.desktop || true; } \
  && echo WS3-T6-OK
```

### WS4-T2 — Create the shellcheck workflow on both forges Depends on [none]

**READ THESE BEFORE TASK**

- `/bin/style.sh` (`:336-360`), `/bin/lint.sh`, `/lefthook.yml`, `/package.json`, `/shellcheck/shellcheckrc`
- `/.github/workflows/go.yml`, `/.forgejo/workflows/go.yml` (for the header-comment style)

**Files to Create**

- `/.github/workflows/shell.yml`
- `/.forgejo/workflows/shell.yml`

**Instructions**

No workflow gates shell today, yet Fedora's provisioning correctness lives as much in `common/**`, `bin/**` and
`bootstrap.sh` as in Go. Create one new workflow per CI system that runs shellcheck through the repo's **single**
style dispatcher, so CI and `npm run lint` can never diverge (CLAUDE.md: "There is a single style dispatcher at
`bin/`; do not introduce a competing `scripts/style.sh`").

MEASURED FACTS (do not re-litigate): `bin/lint.sh --shell` → `run_shell_lint` (`bin/style.sh:336-360`) →
`shellcheck --severity=warning <every tracked *.sh>`; it needs only bash, git and shellcheck, and currently exits 0
over all 102 tracked `*.sh` files, **with or without** a `.shellcheckrc` in scope (the repo's rc lives at
`shellcheck/shellcheckrc` and is only symlinked to `~/.shellcheckrc` on a provisioned host — do **not** copy it into
CI). shellcheck v0.9.0 is the version the tree is verified clean against; its asset
`https://github.com/koalaman/shellcheck/releases/download/v0.9.0/shellcheck-v0.9.0.linux.x86_64.tar.xz` extracts to
`shellcheck-v0.9.0/shellcheck` (HTTP 200 confirmed).

**CREATE `.github/workflows/shell.yml`:**

```yaml
name: Shell

# Shell axis. Runs shellcheck through the repo's single style dispatcher
# (bin/lint.sh --shell -> `shellcheck --severity=warning` over every tracked
# *.sh), so CI and `npm run lint` cannot drift apart. Fedora's provisioning
# correctness lives as much in common/ and bin/ as it does in Go, and neither
# was gated by any workflow before.

on:
  pull_request:
    paths:
      - '**.sh'
      - 'common/**'
      - 'bin/**'
      - 'bash/**'
      - 'zsh/**'
      - 'shellcheck/**'
      - '.github/workflows/shell.yml'
  push:
    branches:
      - main
    paths:
      - '**.sh'
      - 'common/**'
      - 'bin/**'
      - 'bash/**'
      - 'zsh/**'
      - 'shellcheck/**'
      - '.github/workflows/shell.yml'

permissions:
  contents: read

env:
  # Pinned to the version the tree is verified clean against at
  # --severity=warning. Bumping it is a deliberate change that may surface new
  # findings across common/ and bin/, so it does not ride along with unrelated
  # work.
  SHELLCHECK_VERSION: v0.9.0

jobs:
  shellcheck:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5 # v4

      - name: Install shellcheck
        # Installed from the pinned upstream tarball rather than the runner's
        # preinstalled copy or a third-party action, so both CI systems run the
        # identical binary. Installed under $HOME so no sudo is required.
        run: |
          set -euo pipefail
          mkdir -p "$HOME/.local/bin"
          curl -fsSL "https://github.com/koalaman/shellcheck/releases/download/${SHELLCHECK_VERSION}/shellcheck-${SHELLCHECK_VERSION}.linux.x86_64.tar.xz" \
            | tar -xJf - -C /tmp
          install -m 0755 "/tmp/shellcheck-${SHELLCHECK_VERSION}/shellcheck" "$HOME/.local/bin/shellcheck"
          echo "$HOME/.local/bin" >> "$GITHUB_PATH"

      - name: Shellcheck
        run: |
          shellcheck --version
          bin/lint.sh --shell
```

**CREATE `.forgejo/workflows/shell.yml`** — byte-identical **except**: (1) the two
`- '.github/workflows/shell.yml'` entries become `- '.forgejo/workflows/shell.yml'`; (2) a header comment
immediately after `name: Shell` (blank line either side), mirroring `.forgejo/workflows/go.yml:3-9`:

```yaml
# Forgejo-native shell CI. Forgejo reads workflows from .forgejo/workflows/ in
# preference to .github/workflows/ (the two directories are never merged), so
# this mirrors .github/workflows/shell.yml for the self-hosted origin. There is
# no divergence: shellcheck is installed from its pinned upstream tarball, which
# needs no third-party action and so is unaffected by Forgejo's
# DEFAULT_ACTIONS_URL constraint. Keep this in sync with
# .github/workflows/shell.yml.
```

**CONSTRAINTS**

- Do **not** add a shell job to `go.yml`, and do not add `'**.sh'`/`'common/**'`/`'bin/**'` to `go.yml`'s path
  filters — a shell-only PR must not spin up the Fedora container job.
- Do not introduce a bespoke shellcheck file list; go through `bin/lint.sh --shell`.
- Both files land in the same commit (sync contract).

**Known gap (recorded, not fixed here):** `bin/lint.sh --shell` only lints files with a `.sh` extension, so
extension-less shell files (`bash/bashrc`, `zsh/zshrc`, `zsh/zprofile`, `bash/profile`) remain unlinted by CI and by
the pre-commit hook. Widening the dispatcher's file set is a change to `bin/style.sh`, a shared cross-workstream
file, and is deliberately out of scope. WS8-T5 records it in ROADMAP.md's near-term list.

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && actionlint .github/workflows/shell.yml && actionlint .forgejo/workflows/shell.yml \
  && python3 -c "import yaml; g=yaml.safe_load(open('.github/workflows/shell.yml')); f=yaml.safe_load(open('.forgejo/workflows/shell.yml')); assert 'shellcheck' in g['jobs'] and 'shellcheck' in f['jobs']; assert 'common/**' in g[True]['pull_request']['paths']; print('shell workflows ok')" \
  && rm -rf /tmp/sc-pin && mkdir -p /tmp/sc-pin \
  && curl -fsSL https://github.com/koalaman/shellcheck/releases/download/v0.9.0/shellcheck-v0.9.0.linux.x86_64.tar.xz | tar -xJf - -C /tmp/sc-pin \
  && PATH=/tmp/sc-pin/shellcheck-v0.9.0:$PATH bin/lint.sh --shell \
  && echo SHELL_JOB_OK
```

### WS6-T7 — Fix the silent `SSHServer` drop in `mergeSysconfig` Depends on [none]

**READ THESE BEFORE TASK**

- `/internal/manifest/merge.go` (`:244-265`)
- `/internal/manifest/merge_test.go` (`:277+`)
- `/internal/manifest/types.go` (`:58`), `/internal/manifest/validate.go` (`:173`)

**Files to Modify**

- `/internal/manifest/merge.go`
- `/internal/manifest/merge_test.go`

**Instructions**

`mergeSysconfig` (`:244-265`) merges Timezone, Locale, Shell and Firewall but **not** SSHServer. The field exists
(`types.go:58`, `SSHServer string \`toml:"ssh_server"\``) and is validated (`validate.go:173`), so an overlay's
`ssh_server` is silently dropped whenever the base manifest already carries any `[sysconfig]` table. The nil-base
path adopts the whole struct and hides the bug.

**(1)** Append a branch before `mergeSysconfig`'s closing brace. Current tail:

```go
	if overlay.Firewall != "" {
		merged.Sysconfig.Firewall = overlay.Firewall
	}
}
```

becomes:

```go
	if overlay.Firewall != "" {
		merged.Sysconfig.Firewall = overlay.Firewall
	}
	if overlay.SSHServer != "" {
		merged.Sysconfig.SSHServer = overlay.SSHServer
	}
}
```

If the doc comment enumerates merged fields, update it; if it only says "non-empty fields" (it does), leave it.

**(2)** Extend the **existing** test rather than adding one. In `TestMerge_SysconfigFieldWiseOverride` (`:277+`):

- give the base `Sysconfig` an `SSHServer: "off"` field;
- give the overlay `SSHServer: "on"`;
- add one assertion beside the existing four:

```go
	if got.SSHServer != "on" {
		t.Errorf("ssh_server = %q, want on (overlay non-empty wins; it was silently dropped before)", got.SSHServer)
	}
```

Read `internal/manifest/validate.go:173` first and use whatever enum tokens that switch accepts, since `Merge` runs
`Validate` on the merged result (`TestMerge_ValidatesMergedResult`).

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && gofmt -l internal/manifest && go build ./... && go vet ./... \
  && go test -race -run 'Merge' -v ./internal/manifest/... \
  && go test -race ./internal/manifest/...
```

---

## Batch 2 — ADR-2 applicability contract, the shell loader, and the Fedora CI job

WS2-CORE is the highest-risk change in the plan (design R1) and is given a quiet Go tree: it is the only writer in
`internal/backend`, the only writer of `manifest/packages.toml`, and the only writer in `internal/manifest` this
batch, so `go test -race ./...` is an unambiguous verdict on it.

### WS2-CORE — `names` becomes the pkg applicability declaration + Fedora siblings + critical tests #2 and #3 Depends on [none]

**Merged from WS2-T1 + WS2-T2 + WS2-T3.** Design R1 names this the single riskiest change in the plan, and the
design's Advice makes it a hard requirement: ADR-2's `applicableFor` and the rewritten coverage test **must land in
one commit**. Any two-of-three split ships either a red CI (T2 without T3: the retired presence test fails on
lazygit/yazi/lutris-pkg) or — worse — a silent-regression window on macOS and Arch where a dropped `names` key
becomes a quiet `applicable=false` row with no guard.

**READ THESE BEFORE TASK**

- `/internal/backend/backend.go` (`:88-157`), `/internal/backend/pkg.go` (`:104-190`, `:214-218`)
- `/internal/backend/applicable_test.go` (`:24-60`), `/cmd/dotf/doctor_test.go` (`:40-60`, `:230-250`)
- `/internal/manifest/manifest_test.go` (`:355-472`), `/internal/manifest/validate.go` (`:64-71`, `:106-127`)
- `/manifest/packages.toml`, `/docs/reference/manifest-backends.md`, `/internal/manifest/ratchet_test.go`

**Files to Modify**

- `/internal/backend/backend.go`
- `/internal/backend/pkg.go`
- `/internal/backend/applicable_test.go`
- `/cmd/dotf/doctor_test.go`
- `/manifest/packages.toml`
- `/internal/manifest/manifest_test.go`

**Instructions — ONE commit, three steps, verify once at the end.**

#### Step 1 (was WS2-T1) — the applicability seam

Today `pkgBackend` implements no applicability gate (repo-wide, only `apt_repo.go:48` and `aur.go:36` have an
`applicable(info)` method), so `Engine.Applicable` returns true unconditionally for every `pkg` package. A missing
`names[<host mgr>]` key therefore surfaces as `*MissingPkgNameError` from `engine.Installed`, and
`cmd/dotf/status.go:117-120` turns it into `return 0, fmt.Errorf("package %q: %w", …)` — aborting the **whole**
`dotf status`/`diff` run rather than skipping one row.

**MEASURED**: applying this change breaks exactly three test sites — two rows in `applicable_test.go` and one
subtest in `cmd/dotf/doctor_test.go` (both repaired here), plus
`TestEmbeddedManifest_PkgNamesCoverSupportedManagers`, which Step 3 replaces.

**1a — `internal/backend/backend.go`.** Match (`:103-108`):

```go
// platformGated is implemented by backends that can only run on certain
// hosts (apt-repo requires apt, aur requires Arch). Backends without this
// method are applicable on every host.
type platformGated interface {
	applicable(info platform.Info) bool
}
```

Append immediately after it (before the `// lockable is implemented by …` comment at `:110`), blank line then:

```go
// packageGated is implemented by backends whose applicability depends on the
// PACKAGE as well as the host: the pkg backend can only install an entry that
// declares a name for the host's package manager. Backends without this method
// are package-agnostic. Same optional-interface shape as platformGated.
type packageGated interface {
	applicableFor(pkg manifest.Package, info platform.Info) bool
}
```

`manifest` and `platform` are already imported. Tabs, not spaces.

**1b — wire it into `Engine.Applicable`.** Match `:150-157` (unique because of the trailing `platformGated`
assertion — the bare `if !osGateAllows(...)` pair also appears at `:75-77` inside `Installed`):

```go
	if !osGateAllows(pkg.OS, info.OS) {
		return false, nil
	}
	if g, ok := b.(platformGated); ok {
		return g.applicable(info), nil
	}
	return true, nil
}
```

Replace with:

```go
	if !osGateAllows(pkg.OS, info.OS) {
		return false, nil
	}
	if pg, ok := b.(packageGated); ok && !pg.applicableFor(pkg, info) {
		return false, nil
	}
	if g, ok := b.(platformGated); ok {
		return g.applicable(info), nil
	}
	return true, nil
}
```

The `packageGated` check MUST sit **after** `osGateAllows` and **before** the `platformGated` branch (ADR-2). Extend
`Engine.Applicable`'s doc comment (`:141-144`) with one sentence.

**ALSO** (absorbed from WS5-T5 so `backend.go` stays single-owner): reword `backend.go:15`
(`// value. Each of the eight canonical backends implements this interface.`) to be **count-free** — WS5 adds a
`font` value and WS7 adds `repo`, and no numeral in this file may be left to rot. Say "every canonical backend".

**1c — `internal/backend/pkg.go`.** Match `:116-122`:

```go
func resolvePkgName(pkg manifest.Package, info platform.Info) (string, error) {
	name, ok := pkg.Names[string(info.PkgMgr)]
	if !ok {
		return "", &MissingPkgNameError{Package: pkg.Name, PkgMgr: info.PkgMgr}
	}
	return name, nil
}
```

Insert immediately after it (before `func installArgv(` at `:124`), blank line then:

```go
// applicableFor reports whether pkg can be installed on info through the host's
// native package manager: the entry must declare a name for info.PkgMgr. It uses
// the SAME pkg.Names[string(info.PkgMgr)] lookup as resolvePkgName so
// applicability and the MissingPkgNameError path can never disagree. A manifest
// entry is applicable exactly where it has a name, which turns a genuinely
// unpackaged tool from a fail-late probe error (aborting the entire dotf status
// run at cmd/dotf/status.go:117-120) into one clean applicable=false row.
func (b *pkgBackend) applicableFor(pkg manifest.Package, info platform.Info) bool {
	_, ok := pkg.Names[string(info.PkgMgr)]
	return ok
}
```

Add `_ packageGated = (*pkgBackend)(nil)` to the trailing var block (`:214-218`) and run `gofmt -w` (it realigns
the `=` column). Do **not** touch the four `PkgMgr` argv switches (installArgv `:130`, upgradeArgv `:149`,
removeArgv `:168`, installedQuery `:186`) — they are dnf5-correct (design R5).

**1d — `internal/backend/applicable_test.go`.** The table in `TestEngine_Applicable` (`:24-53`) is a **positional**
struct literal (`name, backend, os, info, want`) and the package under test is built at `:59` as
`manifest.Package{Name: "example", Backend: tt.backend, OS: tt.os}` — with no `Names` map. Under the new gate the
two `BackendPkg` rows at `:35-36` evaluate false and fail.

1. Add a `names map[string]string` field between `os string` and `info platform.Info`, commented
   `// pkg-backend name map (nil for backends that are not package-gated)`.
2. Because rows are positional, insert `nil,` as the new 4th element on **every** existing row except the two pkg
   rows.
3. Replace the two pkg rows with three (the third is **CRITICAL TEST #3**; per correction **C26** it lives here, not
   in `pkg_test.go`, which has no applicability table):

```go
		{"pkg on pacman host", manifest.BackendPkg, "", map[string]string{"pacman": "example"}, archPacman, true},
		{"pkg on apt host", manifest.BackendPkg, "", map[string]string{"apt": "example"}, debianAPT, true},
		// ADR-2: a pkg entry with no name for the HOST's package manager is a clean
		// not-applicable, not a MissingPkgNameError that aborts the whole
		// status/diff run. Pins the exact seam that changed.
		{"pkg with no name for host manager", manifest.BackendPkg, "", map[string]string{"pacman": "example"}, debianAPT, false},
```

4. Thread the field into the fixture at `:59`: `…, OS: tt.os, Names: tt.names}`.
5. `gofmt -w internal/backend/applicable_test.go`.

**1e — `cmd/dotf/doctor_test.go`.** The `probe-error-fails` subtest inside `TestCheckPackages` (~`:239-247`) reads:

```go
	t.Run("probe-error-fails", func(t *testing.T) {
		// PkgMgrUnknown => resolvePkgName finds no name => Installed returns a
		// real probe error, exercising the fail branch.
		engine := testDriftEngine(t, exec.NewFake())
		info := platform.Info{OS: platform.OSArch, Arch: platform.ArchX86_64, PkgMgr: platform.PkgMgrUnknown}
		c := checkPackages(parsePkgManifest(t, singlePkgManifest), engine, info)
		if c.Status != checkFail {
			t.Errorf("checkPackages = %q, want fail (detail=%q)", c.Status, c.Detail)
		}
	})
```

Its premise is now invalid: with `PkgMgrUnknown` the package is simply not applicable, so `checkPackages` reports
`ok` with `all 0 applicable packages installed` (the exact observed failure). Preserve the subtest's **purpose** by
choosing a manager that IS declared in `names` but that `internal/backend/pkg.go`'s switches do not handle — `yum`.
`validate.go:64-71` admits `yum` as a `names` key, and `installedQuery` has no `PkgMgrYUM` case, so it returns
`*UnsupportedPkgMgrError`: a real probe error that survives the applicability gate.

1. Extend `singlePkgManifest` (`:48-52`) — change its names line to
   `names = { pacman = "git", apt = "git", dnf = "git", yum = "git", zypper = "git", homebrew = "git" }`
   (harmless for the other subtests, which all run on pacman hosts).
2. Change the subtest's `info` to
   `platform.Info{OS: platform.OSRedHat, Arch: platform.ArchX86_64, PkgMgr: platform.PkgMgrYUM}`.
3. Replace the two-line comment with: the manifest declares a yum name so the entry IS applicable, but the pkg
   backend has no yum argv, so `Installed` returns `*UnsupportedPkgMgrError` — a real probe error, exercising the
   fail branch. (Post-ADR-2, `PkgMgrUnknown` no longer reaches the probe at all — it is a clean not-applicable.)

Change no other subtest.

#### Step 2 (was WS2-T2) — the manifest

Fedora ships **no** `lazygit` and **no** `yazi` RPM in any release/updates branch, yet `packages.toml:157` and
`:176` assert `dnf = "lazygit"` / `dnf = "yazi"`. On a dnf host this is what makes `dotf apply` exit non-zero. Keep
the shared pkg entries for pacman/apt/homebrew/zypper (converting them wholesale would silently downgrade Arch and
macOS from distro-managed packages to unmanaged `~/.local/bin` binaries) and add `os = "redhat"` github-release
siblings. This is only **safe** because Step 1's `applicableFor` turns a dnf-less pkg entry into a clean skip.

**2a — remove the two fictional dnf keys.** At `:157` replace

```toml
names = { pacman = "lazygit", apt = "lazygit", dnf = "lazygit", homebrew = "lazygit", zypper = "lazygit" }
```

with

```toml
names = { pacman = "lazygit", apt = "lazygit", homebrew = "lazygit", zypper = "lazygit" }
```

At `:176` replace

```toml
names = { pacman = "yazi", apt = "yazi", dnf = "yazi", homebrew = "yazi", zypper = "yazi" }
```

with

```toml
names = { pacman = "yazi", apt = "yazi", homebrew = "yazi", zypper = "yazi" }
```

Both original strings are unique in the file.

**2b — rewrite the pkg-section header comment** (`:103-108`) so it states that the `names` table **is** the
applicability declaration: `internal/backend/pkg.go`'s `applicableFor` makes an entry applicable exactly where it
declares a name for the host's package manager, so a manager whose distro genuinely does not package the tool is
**omitted, never invented** — the entry reports `applicable=false` there instead of aborting the run. Any tool
omitted for some manager MUST be covered by a sibling entry on another backend (see `lazygit-bin` / `yazi-bin`
below); that reachability contract is enforced by `TestEmbeddedManifest_ToolsReachableOnSupportedManagers`. Keep the
existing sentence about apt's `fdfind`/`batcat` renames being irrelevant (installs are detected via
package-manager query, not a PATH probe).

**2c — add the two github-release siblings.** Match the unique anchor line
`# --- aur: installed from the Arch User Repository ----------------------` (`:226`) and insert **before** it,
followed by a blank line:

```toml
# Fedora ships no `lazygit` RPM in any release/updates branch, so the dnf key was
# removed from the shared pkg entry above and this os = "redhat" sibling supplies
# the Fedora install path instead (ADR-3). Arch/Debian/macOS/openSUSE keep their
# distro-managed package; only RedHat-family hosts resolve this entry. The asset
# is auto-detected (lazygit_<ver>_linux_<arch>.tar.gz) and verified against the
# release's own checksums.txt sidecar. `binary` names both the extracted
# executable and the PATH probe, so the -bin package name never leaks onto PATH.
# copr:atim/lazygit was rejected: its chroots are live but its newest build
# FAILED, and a github-release is lock-pinnable and independent of a third
# party's build queue.
[[package]]
name = "lazygit-bin"
backend = "github-release"
os = "redhat"
repo = "jesseduffield/lazygit"
tag = "v0.64.1"
binary = "lazygit"
checksums_asset = "checksums.txt"

# Fedora ships no `yazi` RPM either; same os = "redhat" sibling pattern (ADR-3).
# Upstream publishes NO combined checksums sidecar — only per-asset .sha256 files,
# which carry a bare hash with no filename column and so cannot be read by
# checksums_asset — so the shas are pinned inline per target. Keys are
# "<ReleaseOS>/<canonical-arch>" (internal/backend/github_release.go targetKey),
# hence x86_64 rather than amd64. CAVEAT: the upstream archive also ships `ya`,
# yazi's plugin manager; the single-binary github-release backend installs only
# `yazi`.
[[package]]
name = "yazi-bin"
backend = "github-release"
os = "redhat"
repo = "sxyazi/yazi"
tag = "v26.8.15"
binary = "yazi"
checksums = { "linux/arm64" = "f5a85771f06bb0e8c488136ae0aedaec8d341a7cee995549df391d7d852fe8d1", "linux/x86_64" = "cc67eb7991550c2f9407cda52d3f5af0937627aa6884e7de99a04fcf059807e0" }
```

> **Correction C29 applied.** The design specified `checksums_asset = "checksums.txt"` for yazi; upstream publishes
> no combined sidecar, only per-asset `.sha256` bare-hash files that `checksumFor` cannot parse. The two sha256
> values above were computed in this worktree from the v26.8.15
> `yazi-aarch64-unknown-linux-gnu.zip` / `yazi-x86_64-unknown-linux-gnu.zip` assets. **If you change the tag you
> MUST recompute them** (`curl -fsSL <url> | sha256sum`) — a stale sha fails the install closed. lazygit **does**
> publish `checksums.txt`, so it keeps `checksums_asset`.

Asset auto-detection was verified against the real release asset lists with `matchAsset`: lazygit resolves
`lazygit_0.64.1_linux_arm64.tar.gz` / `…_linux_x86_64.tar.gz`; yazi resolves `yazi-aarch64-unknown-linux-gnu.zip` /
`yazi-x86_64-unknown-linux-gnu.zip` (gnu beats musl via `libcScore`). No `asset` and no `match` block is needed — do
not add one.

**2d — add `lutris-pkg`.** After the aur `lutris` block (`:226-229`), blank line first:

```toml
# Fedora ships `lutris` in its official updates repo, but the aur backend gates on
# OSArch (internal/backend/aur.go:36), so the whole gaming intent is a silent
# no-op on Fedora without this sibling (ADR-4). Package names must be unique
# (internal/manifest/validate.go:113), hence the distinct lutris-pkg name.
# Deliberate low-value inclusion: on aarch64 Asahi there is no x86 Wine/Proton
# without the muvm+FEX 4 KiB-page microVM. gaming is opt-in.
[[package]]
name = "lutris-pkg"
backend = "pkg"
os = "redhat"
names = { dnf = "lutris" }
```

**2e — intents.** In `[intents.dev]` (`:40-56`) insert `    "lazygit-bin",` and `    "yazi-bin",` immediately after
`    "direnv",` (match the two-line anchor `    "direnv",\n    "atuin",`). In `[intents.gaming]` (`:61`) change
`packages = ["lutris"]` to `packages = ["lutris", "lutris-pkg"]`. `validate.go:118-127` requires every
intent-referenced name to exist as a `[[package]]`, so all three must be added first.

**2f — audit (no edit expected).** Re-confirm the remaining 11 pkg entries' `dnf` names against Fedora's
release/updates repos and, when it lands, the WS0 device capture. Researched result: ripgrep, eza, bat, fzf, zoxide,
jq, gh, tmux, direnv are correct as-is; `fd` correctly maps to **`fd-find`** (Fedora's `fd-find` ships
`/usr/bin/fd`); `delta` correctly maps to **`git-delta`** — Fedora also ships an unrelated package literally named
`delta`, so do **not** shorten it. Change nothing unless the capture contradicts one of these.

Add or convert **zero** `custom-script` entries.

#### Step 3 (was WS2-T3) — CRITICAL TEST #2, rewritten to stay exhaustive

This is the sole guard on ADR-2's one real regression risk (R1): with `names` now the applicability declaration, a
dropped key on macOS or Arch becomes a **silent** `applicable=false` row instead of a loud probe error. It is also
the fix for the root cause of the two fictional dnf names — the old test's contract ("every intent-referenced pkg
entry carries a name for all five managers") is what pressured an author into inventing a plausible RPM name to keep
CI green, and the same trap is armed for zypper today and for every future distro.

> **BLOCKER FIX (critique).** The first-pass rewrite iterated a hand-written `coreTools` literal. The retired test
> derived its subject set **from the manifest** (`manifest_test.go:460-462`), so every intent-referenced pkg entry —
> present and future — was covered automatically. A hand-maintained allowlist is not the guard R1 requires: the
> moment a 14th pkg tool joins `intents.dev` it is checked by nothing. The spec below keeps the subject set
> **manifest-derived** and uses the provider map only to group per-platform siblings.

**3a — imports** (`manifest_test.go:2-17`). Add, in the same grouped block, gofmt-sorted:

```go
	"github.com/yandy-r/dotfiles/internal/backend"
	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/platform"
	"github.com/yandy-r/dotfiles/internal/privilege"
```

(No import cycle: `manifest_test` is an **external** test package, so it may import `internal/backend`, which
imports `internal/manifest`. Verified by prototype.)

**3b — retire `supportedPkgMgrs`, add the fixtures.** Delete `manifest_test.go:360-364` and put in its place:

```go
// supportedManagerHosts is the representative host for each package manager the
// pkg backend can execute installs for (internal/backend/pkg.go installArgv),
// paired with the distro family that manager belongs to. Reachability is
// evaluated per HOST, not per manager string, because a package's `os` gate and
// its backend's own platform gate both key off platform.Info.OS.
var supportedManagerHosts = []struct {
	mgr  string
	info platform.Info
}{
	{"apt", platform.Info{OS: platform.OSDebian, Arch: platform.ArchX86_64, PkgMgr: platform.PkgMgrAPT}},
	{"dnf", platform.Info{OS: platform.OSRedHat, Arch: platform.ArchARM64, PkgMgr: platform.PkgMgrDNF}},
	{"homebrew", platform.Info{OS: platform.OSMacOS, Arch: platform.ArchARM64, PkgMgr: platform.PkgMgrHomebrew}},
	{"pacman", platform.Info{OS: platform.OSArch, Arch: platform.ArchX86_64, PkgMgr: platform.PkgMgrPacman}},
	{"zypper", platform.Info{OS: platform.OSSUSE, Arch: platform.ArchX86_64, PkgMgr: platform.PkgMgrZypper}},
}

// siblingGroups lists the per-platform sibling sets (ADR-3/ADR-4): entries that
// are alternative install routes for ONE logical tool, whose `os` gates and
// `names` tables must TOGETHER cover every supportedManagerHosts entry. Any
// package not named here is its own sole provider. This map is the only
// hand-maintained half of the contract — adding a sibling entry without listing
// it here fails the test, which is the intended review prompt.
var siblingGroups = [][]string{
	{"lazygit", "lazygit-bin"},
	{"yazi", "yazi-bin"},
	{"lutris", "lutris-pkg"},
}

// coreTools are the modern-CLI tools the shared shell configuration
// (common/tools.sh, common/aliases.sh) integrates. Used by
// TestEmbeddedManifest_CoreToolsetProvisioned; the reachability test derives its
// own subject set from the manifest instead, so a new pkg entry is covered
// automatically.
var coreTools = []string{
	"ripgrep", "eza", "bat", "fd", "fzf", "delta", "zoxide", "jq",
	"gh", "lazygit", "tmux", "neovim", "yazi", "direnv",
	"starship", "atuin",
}
```

**3c — hoist `coreTools`.** In `TestEmbeddedManifest_CoreToolsetProvisioned` (`:371-405`) delete its local
`coreTools := []string{…}` literal so the loop uses the new package-level slice. Change nothing else there.

**3d — replace the coverage test.** Delete `manifest_test.go:446-472` (doc comment through closing brace) and put
this in its place:

```go
// TestEmbeddedManifest_ToolsReachableOnSupportedManagers replaces the retired
// TestEmbeddedManifest_PkgNamesCoverSupportedManagers, which asserted per-manager
// name PRESENCE on every intent-referenced pkg entry. That contract was the
// direct cause of two fictional dnf names (`lazygit`, `yazi` — RPMs that exist in
// no Fedora branch): it pressured an author facing a tool a distro does not
// package into inventing a plausible name to keep CI green, and the same trap was
// armed for zypper and for every future distro.
//
// The replacement contract is per-manager reachable INSTALL PATH: for every
// intent-referenced pkg package (the SAME manifest-derived subject set the
// retired test used, so a future pkg entry is covered automatically) and every
// supported manager host, at least one member of that entry's sibling group must
// be Applicable there, evaluated through the REAL backend.Engine.Applicable — the
// same predicate `dotf status`, `dotf diff` and `dotf install` use, so the test
// cannot drift from the engine. This is also the sole guard on ADR-2's one real
// regression risk: now that `names` is the applicability declaration, a dropped
// key is a silent applicable=false row rather than a loud probe error, and this
// test is what makes that silence safe.
func TestEmbeddedManifest_ToolsReachableOnSupportedManagers(t *testing.T) {
	m := loadEmbeddedManifest(t)

	referenced := make(map[string]bool)
	for _, intent := range m.Intents {
		for _, name := range intent.Packages {
			referenced[name] = true
		}
	}
	byName := make(map[string]manifest.Package, len(m.Packages))
	for _, pkg := range m.Packages {
		byName[pkg.Name] = pkg
	}

	// Reverse-index the sibling groups so a group is evaluated once, as a unit,
	// no matter which member the manifest scan reaches first.
	groupOf := make(map[string][]string)
	for _, group := range siblingGroups {
		for _, name := range group {
			if _, ok := byName[name]; !ok {
				t.Fatalf("siblingGroups names %q, which is not a [[package]] in the embedded manifest", name)
			}
			groupOf[name] = group
		}
	}

	engine := backend.Default(exec.NewFake(), privilege.NewSudo(), privilege.NewFakeDropper(), t.TempDir(), nil)

	seen := make(map[string]bool)
	for _, pkg := range m.Packages {
		if pkg.Backend != manifest.BackendPkg || !referenced[pkg.Name] {
			continue
		}
		providers, grouped := groupOf[pkg.Name]
		if !grouped {
			providers = []string{pkg.Name}
		}
		key := strings.Join(providers, "+")
		if seen[key] {
			continue
		}
		seen[key] = true

		t.Run(key, func(t *testing.T) {
			for _, host := range supportedManagerHosts {
				var reachable []string
				for _, name := range providers {
					ok, err := engine.Applicable(byName[name], host.info)
					if err != nil {
						t.Fatalf("Applicable(%q, %s host) error = %v", name, host.mgr, err)
					}
					if ok {
						reachable = append(reachable, name)
					}
				}
				if len(reachable) == 0 {
					t.Errorf("tool %q has no reachable install path on a %s host (os=%s): providers %v are all applicable=false; add a %s name or an os-gated sibling entry",
						key, host.mgr, host.info.OS, providers, host.mgr)
				}
			}
		})
	}
}
```

`strings` is already imported by `manifest_test.go`; confirm before adding. Add **no** other test, and do **not**
keep a vestigial name-presence assertion — the presence contract is exactly what is being retired.

**NEGATIVE CONTROL (run it, then undo by hand).** Prove the guard bites: temporarily delete `dnf = "ripgrep"` from
`manifest/packages.toml:112`, run
`go test ./internal/manifest/ -run TestEmbeddedManifest_ToolsReachableOnSupportedManagers`, and confirm it fails with
`tool "ripgrep" has no reachable install path on a dnf host`. Then restore the key **by hand** — `git checkout --`
would also revert Step 2.

**Verify (whole task, one run)**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && gofmt -l internal/backend cmd/dotf internal/manifest \
  && go build ./... && go vet ./... && go test -race ./... && golangci-lint run ./... \
  && test "$(grep -c '^backend = \"custom-script\"' manifest/packages.toml)" = 16 && echo WS2-CORE-OK
```

`gofmt -l` must print nothing; the whole suite must be green at the end of this one commit;
`TestCustomScriptRatchet` must still report exactly 16.

### WS-SHELL — ADR-14: the per-OS module loader becomes a search path, plus the regressions that go live Depends on [WS1-T2]

> **BLOCKER FIX (critique).** ADR-14 is user requirement #2 — the design's Goal calls "dotfiles support" _"fixing a
> loader bug that silently disables `common/linux/aliases.sh` on Fedora, Debian and Arch alike"_ — and design.md:690
> even references a **"WS-shell"** that §6 never defines (correction **C36**). No task in the first-pass breakdown
> owned any of `common/aliases.sh`, `common/functions.sh`, `common/exports.sh`, `common/linux/aliases.sh` or
> `common/README.md`. This task owns them. Without it, `npm run lint` and WS4-T2's new shellcheck workflow gate work
> nobody performs, and WS8's parity claim is unbacked on **every** distro.

**READ THESE BEFORE TASK**

- `/common/platform.sh` (`:50-87`, `:309`)
- `/common/aliases.sh` (`:854-858`), `/common/functions.sh` (`:828-836`), `/common/exports.sh` (`:230-256`, `:296-312`, `:594-600`)
- `/common/linux/aliases.sh` (`:38-71`, `:320-350`), `/common/macos/`
- `/common/README.md` (`:208-246`)
- `/internal/sysconfig/linux.go` (firewalld routing, for alias parity)

**Files to Modify**

- `/common/platform.sh`
- `/common/aliases.sh`
- `/common/functions.sh`
- `/common/exports.sh`
- `/common/linux/aliases.sh`
- `/common/README.md`

**Instructions**

VERIFIED PROBLEM: all three loaders dispatch on `"$SCRIPT_DIR/$DOTFILES_OS/<file>.sh"`
(`common/aliases.sh:856`, `common/functions.sh:834`, `common/exports.sh:598`), while `detect_os()`
(`common/platform.sh:50-87`, assigned to `DOTFILES_OS` at `:309`) emits `macos|debian|redhat|arch|suse|linux|wsl|…`
— it emits the literal `linux` **only** when `/etc/os-release` is absent or neither `ID` nor `ID_LIKE` matches. Only
`common/linux/` and `common/macos/` exist. So all **495 lines** of `common/linux/aliases.sh` are dead on Fedora,
Debian, Arch **and** openSUSE alike.

**PART A — `common/platform.sh`: add the search-path resolver.** Place `os_module_dirs()` near `detect_os()`
(`:50`), where `has_command`, `is_macos` and `is_linux` already live. It prints, one per line, most-generic-first:
the **family** directory then the **distro overlay** directory, both relative names (`macos`; or `linux` then the
distro token). Mapping: `macos` → `macos`; `debian|redhat|arch|suse|wsl|linux` → `linux` (plus the distro token as
an overlay when it differs from `linux`). Anything else prints nothing. Doc-comment it as the single source of the
per-OS module search path, and note the overlay slot is genuinely usable (`common/redhat/aliases.sh` would layer on
top of `common/linux/aliases.sh`, not replace it).

**PART B — the three loaders.** Replace each dispatch block with a loop over `os_module_dirs()` that sources every
existing match, in order, so the family file loads first and a distro overlay can refine it. Keep the exact existing
`SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"` line and the surrounding banner comments; keep the
`[[ ]]` / quoting / `$(...)` conventions.

- `common/aliases.sh:854-858` (file tail, 858 lines) — `aliases.sh`
- `common/functions.sh:832-836` (file tail, 836 lines; banner at `:828-830`) — `functions.sh`. Note no
  `common/linux/functions.sh` exists today; the loop must tolerate that.
- `common/exports.sh:596-600` — `exports.sh`. **Not** at the file tail (626 lines); a `CLEANUP EMPTY VARIABLES`
  section follows at `:602-626`. Neither `common/linux/exports.sh` nor `common/macos/exports.sh` exists today.

**PART C — the regressions the loader makes live** (ADR-14 requires these in the same change, because ~495
previously-dead lines go live on four distro families at once).

> **Amendment (PR #36 review) — this list is NOT exhaustive.** It originally read "the three regressions" and
> asserted an exhaustive audit. The review reproduced four more in the same newly-live file, all of which are
> fixed alongside items 1-3 below: `alias mount="mount | column -t"` swallowed every argument and is **deleted**
> (`mounts` already provides the pretty listing); `alias suspend=` shadowed a shell builtin and is renamed
> `syssuspend`, as are `enable`/`disable` → `sysenable`/`sysdisable`; and `updateall` / `cleanup-packages` were
> hardcoded to apt, so they move into the `case "$DOTFILES_PKG_MGR"` dispatch with one correct body per manager
> (snap/flatpak appended only when installed). A further minor sweep renames `netstat` away, renames `gcc`/`g++`
> to `gccw`/`gxxw`, and renames Arch's `info` to `pkginfo` in both the yay and paru arms. See ADR-14 in
> `design.md` for the audited classes and the named residual risk (`install` still shadows coreutils
> `install(1)` by design).

1. **Two** builtin-shadowing `history` aliases, not one (correction **C12**): `common/linux/aliases.sh:50`
   (`alias history="dnf history"`, inside the `dnf)` arm `:38-57`) and `:70` (`alias history="yum history"`, inside
   the `yum)` arm `:59-71`). `history` is a builtin in both bash and zsh and aliases take precedence. Rename both to
   a non-shadowing name (e.g. `pkghistory`), consistently across both arms.
2. **Firewalld aliases.** The firewall block is ufw-only: `:333-342`, guarded `if has_command ufw; then`, defining
   `firewall`/`fwenable`/`fwdisable`/`fwallow`/`fwdeny`/`fwdelete`/`fwreset`. Add a sibling
   `elif has_command firewall-cmd; then` branch defining the same alias names against firewalld
   (`firewall-cmd --state` / `--list-all`, `--add-service`, `--reload`, and the `--permanent` forms where
   appropriate). `internal/sysconfig/linux.go` already routes RedHat to firewalld, so the shell layer must match.
   Leave the iptables block (`:344-349`) alone.
3. **Plasma 6 settings binary.** `:324` is `alias settings="systemsettings5"` inside the
   `elif [[ "$XDG_CURRENT_DESKTOP" == *"KDE"* ]]; then` branch (`:323`) of the DE dispatch (`:320-327`). Plasma 6
   ships `systemsettings`. Prefer a guard over a bare rename:
   `if has_command systemsettings; then alias settings="systemsettings"; elif has_command systemsettings5; then …`.

**PART D — the locale codeset bug (design §3 "Shell layer (locale)").** `locale_exists()`
(`common/exports.sh:236-255`) lowercases via `dotfiles_to_lower` (`:232-234`) and strips whitespace (`:244`) but
never collapses the codeset separator, so a glibc `locale -a` entry `de_DE.utf8` can never match a requested
`de_DE.UTF-8`. `pick_dotfiles_locale` (`:296-304`) then falls through and `:306-311` **unconditionally**
force-exports `LANG`, `LC_ALL` and `LC_CTYPE`, silently rewriting a non-`en_US` user's locale to `en_US.UTF-8` or
`C.UTF-8`. Two fixes:

1. Collapse the codeset in a shared normalizer applied to **both** `target_lower` (`:241`) and `entry_lower`
   (`:246`) — treat `utf-8` and `utf8` as equal. **Note this region uses 2-space indent**, unlike the 4-space indent
   elsewhere in `common/`.
2. Guard the `LC_ALL` force-export at `:310` on `[[ -n "${LC_ALL:-}" ]]` so an unset `LC_ALL` is not invented. Keep
   the `C.UTF-8` fallback — it is load-bearing on minimal images. Leave `LANG` and `LC_CTYPE` behaviour as-is.

This makes `internal/sysconfig/locale_resolve.go`'s parity claim true; WS6-T3 writes the doc comment accordingly.

**PART E — `common/README.md`** (correction **C13**). `:233-244` documents an `is_macos`/`is_linux` loader the code
has **never** had — the likely origin of the bug. Replace that fenced example with the real `os_module_dirs()`
search path, and extend the `### linux/aliases.sh` section (`:210-212`) to document the distro-overlay slot.

**Explicitly NOT done here** (recorded so the omission is a decision): the Fedora `/etc/bashrc` severance. Fedora's
bash is not built with `-DSYS_BASHRC`, so `/etc/bashrc` — and therefore `/etc/profile.d/*.sh` — is pulled in only by
the skel `~/.bashrc` that `dotf link` replaces, for exactly the interactive non-login shell a Konsole tab starts.
`bash/bashrc` (verified) never sources it. Adding a guarded source is a behaviour change to the login path on four
platforms and is out of this task's blast radius. **WS1-T5 must list it in the device checklist's residual risks,
and WS8-T5 in ROADMAP.md's "Known risks carried forward".**

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && shellcheck --severity=warning common/platform.sh common/aliases.sh common/functions.sh common/exports.sh common/linux/aliases.sh \
  && bash -l -c 'echo "Bash config loaded successfully"' \
  && zsh  -l -c 'echo "Zsh config loaded successfully"' \
  && bash -c 'source common/platform.sh; os_module_dirs' \
  && bash -ic 'type history' | head -1 \
  && ! grep -q 'alias history=' common/linux/aliases.sh \
  && bin/lint.sh --shell \
  && echo WS-SHELL-OK
```

`type history` must report the **builtin**, not an alias.

### WS4-T1 — Fedora container job on both Go workflows (critical test #6) Depends on [WS1-T4]

**READ THESE BEFORE TASK**

- `/.github/workflows/go.yml`, `/.forgejo/workflows/go.yml` (header comment `:3-9`)
- `/go.mod`

**Files to Modify**

- `/.github/workflows/go.yml`
- `/.forgejo/workflows/go.yml`

**Instructions**

Add a second CI job named `fedora` to **both** Go workflows. It is the only thing in the repo that ever executes
`dotf` against a real Fedora userspace (real `/etc/os-release`, real dnf5, real `rpm`). Its final smoke step **is**
critical test #6.

This recipe was rehearsed end-to-end locally against `fedora:latest` (Fedora 44, `ID=fedora`, no `ID_LIKE`) on the
current tree. Measured: `go vet ./...` OK, `go test -race ./...` exit 0,
`dotf platform` → `os=redhat arch=x86_64 pkg=dnf container=docker distro=fedora`,
`dotf apply --dry-run --headless` exit 0, `dotf link --dry-run` exit 0. **Do not "improve" the recipe.**

**Step A — path filters (idempotent).** WS1-T4 landed in batch 1, so both blocks should already contain
`- 'bootstrap.sh'`, `- '.goreleaser.yml'` and `- '.goreleaser.forgejo.yml'`. Confirm; if present, change nothing
here. Do **not** add `'**.sh'`/`'common/**'`/`'bin/**'` — those live on WS4-T2's `shell.yml`, so a shell-only PR does
not spin up the Fedora container.

**Step B — append the `fedora` job** after the existing `test` job's last step (the `Lint` step), separated by one
blank line, at the same 2-space indentation as `  test:`. Do not modify the `test` job at all.

```yaml
  fedora:
    # Distro axis (ADR-15). This is the only job that runs the Go suite and dotf
    # itself against a real Fedora userspace -- a real /etc/os-release, real dnf5
    # and a real rpm database. Every other Fedora assertion in the repo runs
    # against exec.Fake. No arm64 runner is bought: architecture variance is
    # rehearsed locally instead (see docs/fedora-rehearsal.md).
    runs-on: ubuntu-latest
    container: fedora:latest
    steps:
      - name: Install container prerequisites
        # fedora:latest ships dnf5, rpm, bash, curl, tar and sudo but NOT git or
        # gcc: actions/checkout silently degrades to a REST tarball without git,
        # and `go test -race` fails with "-race requires cgo" without gcc.
        run: dnf -y install --setopt=install_weak_deps=False git gcc tar gzip findutils diffutils

      - name: Checkout
        uses: actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5 # v4

      - name: Set up Go
        # cache is off deliberately: the toolchain cache buys little for a
        # secondary job and is the most likely thing to misbehave inside a
        # distro container.
        uses: actions/setup-go@40f1582b2485089dde7abd97c1529aa768e1baff # v5
        with:
          go-version-file: go.mod
          cache: false

      - name: Vet
        run: go vet ./...

      - name: Test
        run: go test -race ./...

      - name: Smoke
        # Critical test #6 (design section 7): the platform contract and both
        # dry-run surfaces, asserted against a real Fedora host rather than a
        # fake. Deliberately does NOT assert the absence of plan.apt-repo -- that
        # is WS1's verification, so this job stays green independently.
        run: |
          set -euo pipefail
          go build -o /tmp/dotf ./cmd/dotf
          out="$(/tmp/dotf platform)"
          echo "dotf platform: $out"
          case "$out" in
            *os=redhat*) ;;
            *) echo "expected os=redhat in: $out" >&2; exit 1 ;;
          esac
          case "$out" in
            *pkg=dnf*) ;;
            *) echo "expected pkg=dnf in: $out" >&2; exit 1 ;;
          esac
          /tmp/dotf apply --dry-run --headless
          /tmp/dotf link --dry-run
```

**Step C — `.forgejo/workflows/go.yml`**: the same job with exactly **two** deliberate divergences.

1. Add `nodejs` to the dnf install list:
   `run: dnf -y install --setopt=install_weak_deps=False git gcc tar gzip findutils diffutils nodejs`
   Put the reason in the step comment: GitHub mounts the runner's own node into a container job, so `fedora:latest`
   needs none there; Forgejo's `act_runner` requires node **inside** the image before any JavaScript action
   (`actions/checkout`, `actions/setup-go`) can run.
2. Add this comment above the `Set up Go` step:

```yaml
        # actions/setup-go is an actions/* action and so IS resolvable from
        # Forgejo's DEFAULT_ACTIONS_URL. If it ever proves flaky inside this
        # container, replace this step with a raw-shell Go tarball install that
        # parses the version from go.mod -- note go.mod pins `go 1.26` with no
        # patch, so a raw install must resolve a concrete patch release (e.g.
        # via https://go.dev/VERSION?m=text) rather than fetching go1.26.tar.gz,
        # which does not exist.
```

Everything else (runs-on, container, step order, vet/test/smoke bodies) must be byte-identical to the GitHub copy.

**Step D — update the `.forgejo` header comment** (`:3-9`), which currently ends "The only substantive difference is
the Lint step … Keep this in sync with .github/workflows/go.yml." Reword to record **both** divergences: the Lint
step's raw-shell golangci-lint install, and the fedora job's extra `nodejs` package. Keep the closing sync sentence.

**CONSTRAINTS**: do not touch the existing `test` job; reuse the exact pinned action SHAs already in the files; both
files land in the same commit.

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && actionlint .github/workflows/*.yml && actionlint .forgejo/workflows/*.yml \
  && python3 -c "import yaml; g=yaml.safe_load(open('.github/workflows/go.yml')); f=yaml.safe_load(open('.forgejo/workflows/go.yml')); assert 'fedora' in g['jobs'] and 'fedora' in f['jobs']; assert g['jobs']['fedora']['container']=='fedora:latest'; print('fedora job present in both')" \
  && docker run --rm -v "$PWD":/w:ro -v "$(go env GOROOT)":/usr/local/go:ro \
       -e PATH=/usr/local/go/bin:/usr/local/sbin:/usr/sbin:/sbin:/usr/local/bin:/usr/bin:/bin \
       fedora:latest bash -c 'set -euo pipefail; dnf -y install --setopt=install_weak_deps=False git gcc tar gzip findutils diffutils >/dev/null 2>&1; cp -a /w /work; cd /work; go vet ./...; go test -race ./...; go build -o /tmp/dotf ./cmd/dotf; out="$(/tmp/dotf platform)"; echo "$out"; case "$out" in *os=redhat*) ;; *) exit 1 ;; esac; case "$out" in *pkg=dnf*) ;; *) exit 1 ;; esac; /tmp/dotf apply --dry-run --headless >/dev/null; /tmp/dotf link --dry-run >/dev/null; echo FEDORA_JOB_OK'
```

### WS4-T3 — `docs/fedora-rehearsal.md`: the two local containers and what they cannot prove Depends on [none]

**READ THESE BEFORE TASK**

- `/docs/plans/fedora-support/design.md` (§9), `/.github/workflows/go.yml`, `/.markdownlint.json`
- `/internal/sysconfig/linux.go`

**Files to Create**

- `/docs/fedora-rehearsal.md`

**Instructions**

Per ADR-15 this exists because distro variance is cheap to automate and architecture variance is not, and because
arm64 + systemd together is unreachable under emulation. **Every command below was executed and its output observed
against `fedora:latest` (Fedora Linux 44, `ID=fedora`, no `ID_LIKE`) with Docker on an x86_64 host.** Record them
verbatim; do not paraphrase into untested variants.

Required sections:

1. `# Fedora Rehearsal` + a purpose paragraph citing ADR-15: distro variance is automated (the `fedora` job in both
   Go workflows); architecture variance and anything needing PID 1 is rehearsed locally; no arm64 runner is bought.
2. `## What CI already covers` — `go vet`, `go test -race`, and the smoke step (`dotf platform` contains
   `os=redhat` and `pkg=dnf`; `dotf apply --dry-run --headless` and `dotf link --dry-run` exit 0) inside
   `container: fedora:latest`; plus shellcheck over every tracked `*.sh` via `shell.yml`. State that nothing else in
   the repo executes against a real Fedora userspace.
3. `## Container A — native amd64, privileged, systemd` (for sysconfig: firewalld, sshd, localectl):

   ```bash
   docker run -d --name dotf-fedora-systemd \
     --privileged --cgroupns=host \
     -v /sys/fs/cgroup:/sys/fs/cgroup:rw \
     fedora:latest \
     bash -c 'dnf -y install --setopt=install_weak_deps=False systemd dbus-broker firewalld openssh-server && exec /usr/lib/systemd/systemd'

   # give PID 1 ~45s, then:
   docker exec dotf-fedora-systemd systemctl enable --now dbus-broker
   docker exec dotf-fedora-systemd systemctl restart firewalld
   docker exec dotf-fedora-systemd firewall-cmd --state
   ```

   Record as prose these **measured** facts: `systemctl is-system-running` reports `degraded` and that is expected —
   do not chase it. Without `dbus-broker` installed and started, `firewall-cmd --state` fails
   `DBUS_ERROR: Failed to connect to socket /run/dbus/system_bus_socket`; `fedora:latest` does not pull it in
   transitively, and this is the single most likely thing to waste an hour. After `dbus-broker`,
   `firewall-cmd --state` prints `running`, `--get-default-zone` prints `public`,
   `systemctl enable --now sshd` yields `active`, and `firewall-cmd --permanent --query-service=ssh` prints `yes`.
   `hostnamectl set-hostname rehearsal` **FAILS** with `Could not set static hostname: … Device or resource busy` —
   `/etc/hostname` is owned by the container runtime, so that sysconfig domain is a physical-device-only claim.
   A container's firewalld zone is not a roaming laptop's zone. Cleanup: `docker rm -f dotf-fedora-systemd`.
4. `## Container B — emulated aarch64` (package availability and asset matching):

   ```bash
   # one-time, per boot
   docker run --privileged --rm tonistiigi/binfmt --install arm64

   docker run --rm --platform linux/arm64 fedora:latest \
     sh -c 'uname -m; cat /etc/os-release | head -3'
   # -> aarch64 / NAME="Fedora Linux" / VERSION="44 (Container Image)"
   ```

   Expect roughly an 8–9× wall-clock penalty on the RPM layer and effectively none on the Go binary (CGO-free
   cross-compile, no arch-specific codegen).
5. `## What no container can prove` — cross-reference `docs/fedora-first-run.md` (WS1-T5) as the device checklist:
   arm64 **plus** systemd (measured: `dbus-broker` dies `226/NAMESPACE` under `qemu-user`; a full
   `qemu-system-aarch64` TCG VM is explicitly out of scope per ADR-15); SELinux; `hostnamectl set-hostname`;
   btrfs/snapper; 16 KiB page-size interaction with C-linked third-party binaries; a live KDE Plasma 6 session and
   `--reload`'s component restarts; `bootstrap.sh` against a private prerelease origin.
6. `## Reproducing the CI Fedora job locally` — the exact one-liner from WS4-T1's verify block, verified to exit 0.
   Record the observed output line `os=redhat arch=x86_64 pkg=dnf container=docker distro=fedora` so drift is
   visible, and explain the `-v /w:ro` + `cp -a` dance: the worktree is mounted read-only so the rehearsal can never
   mutate the developer's checkout.

**MARKDOWN CONSTRAINTS** (lefthook runs markdownlint-cli2 on staged Markdown; config `.markdownlint.json`): MD014 is
ON — never prefix a fenced command with `$`. MD022/MD031/MD032: blank lines around every heading, fence and list.
MD047: exactly one trailing newline. MD010: no hard tabs. MD049 italics `_underscores_`; MD050 bold `**asterisks**`.
MD034: no bare URLs outside code — wrap in `<…>` or a link. MD024 siblings_only. MD013 is OFF, so long commands may
stay on one line.

**SCOPE FENCE**: do not edit `docs/README.md`, `CLAUDE.md` or `AGENTS.md` (WS8 owns those); do not create
`docs/fedora-first-run.md` (WS1-T5 owns it) — only link to it.

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && npx --no-install markdownlint-cli2 docs/fedora-rehearsal.md \
  && grep -q 'dbus-broker' docs/fedora-rehearsal.md \
  && grep -q '226/NAMESPACE' docs/fedora-rehearsal.md \
  && grep -q 'FEDORA_JOB_OK' docs/fedora-rehearsal.md \
  && echo DOCS_OK
```

### WS5-T1 — Plasma reload: busctl-first KWin reconfigure candidate **table** Depends on [none]

**READ THESE BEFORE TASK**

- `/internal/plasma/reload.go` (`:41-43`, `:80-95`, `:158-176`)
- `/internal/plasma/reload_test.go` (`:140-190`, `:240-250`), `/internal/plasma/apply.go` (`:118-125`)
- `/cmd/dotf/plasmaverbs.go` (`:310-340`, `:365-374`), `/cmd/dotf/plasmadiag_test.go` (`:153-172`)

**Files to Modify**

- `/internal/plasma/reload.go`
- `/internal/plasma/reload_test.go`
- `/cmd/dotf/plasmaverbs.go`

**Instructions**

The KWin `reconfigure` reload must resolve on a Fedora KDE host. Today `reload.go:43` probes only
`{"qdbus6", "qdbus"}`; Fedora's `qt6-qttools` ships `/usr/bin/qdbus-qt6` and is not installed by default, so the
reload silently resolves nothing. `busctl` ships with systemd on every Linux host and needs no Qt tooling.

> **Correction C15 applied.** `busctl` **cannot** be added to `qdbusCandidates`: `resolveArgv` (`:158-176`)
> substitutes **only** `resolved[0]`, so a `busctl` candidate would emit the invalid
> `busctl org.kde.KWin /KWin reconfigure`. Replace the argv[0]-substitution for this one command with a **candidate
> argv table**.

**EDIT 1 — `reload.go`.** Replace `:41-43`:

```go
// qdbusCandidates are probed in order, because the binary name varies across
// distributions.
var qdbusCandidates = []string{"qdbus6", "qdbus"}
```

with:

```go
// kwinReconfigureCandidates are COMPLETE argv vectors for "make KWin re-read
// its config", probed in order and substituted wholesale for the generic
// placeholder ReloadPlan emits. busctl leads because it ships with systemd on
// every Linux host and needs no Qt tooling at all; the qdbus variants follow
// because the binary name varies per distribution (only qdbus6 on a Plasma 6
// Arch/Debian host, only qdbus-qt6 in Fedora's qt6-qttools). A full-argv table
// rather than an argv[0] candidate list is load-bearing: busctl's call syntax
// takes the interface as its own argument, so the ARITY differs.
var kwinReconfigureCandidates = [][]string{
	{"busctl", "--user", "call", "org.kde.KWin", "/KWin", "org.kde.KWin", "reconfigure"},
	{"qdbus6", "org.kde.KWin", "/KWin", "reconfigure"},
	{"qdbus-qt6", "org.kde.KWin", "/KWin", "reconfigure"},
	{"qdbus", "org.kde.KWin", "/KWin", "reconfigure"},
}
```

**EDIT 2 — `resolveArgv`** (`:158-176`). Keep the signature `func (e *Engine) resolveArgv(argv []string) ([]string, bool)`
and the empty-argv guard. When `argv[0] == "qdbus"` (the generic placeholder `ReloadPlan` emits at `:88`), walk
`kwinReconfigureCandidates` and return the **first whole vector** whose `[0]` resolves via `e.r.LookPath`, returning
a freshly copied slice (never aliasing the package-level table). Every other argv keeps today's behaviour: LookPath
`argv[0]`, return `argv` unchanged on success, `(nil, false)` on failure. Update the doc comment to say the kwin
command is resolved from a candidate **table** (whole argv, not just argv[0]).

Do **not** change `ReloadPlan`'s emitted placeholder at `:88` — it must stay
`[]string{"qdbus", "org.kde.KWin", "/KWin", "reconfigure"}`. It is pure (no Runner) and `reload_test.go:246` pins
that an unresolvable host still prints a legible `qdbus` line.

**EDIT 3 — `reload_test.go`**, `TestReload_ProbesQdbusVariants` table (`:148-152`). Add **one row** to the existing
table (a row, not a new test):

```go
{"busctl preferred over every qdbus variant", map[string]string{"busctl": "/usr/bin/busctl", "qdbus6": "/usr/bin/qdbus6"}, "busctl", true},
```

The existing rows (`qdbus6 preferred`, `qdbus fallback`, `neither resolves`) must stay green **unchanged** — they
register no `busctl` in the fake, so behaviour is identical. The assertion body only inspects `ran[0].Argv[0]` and
the recorded call names (`:176-188`), so a 7-element busctl argv fits the existing shape.

**EDIT 4 — `cmd/dotf/plasmaverbs.go`, comments only.** Two stale statements: (a) `~:318` inside `reload`'s doc
comment ("it probes qdbus6 before qdbus, and qdbus does not exist on a Plasma 6 …") → reword to say the engine
resolves the whole kwin argv from a candidate table (busctl first, then the qdbus variants); (b) `:330-334`, the
`plasmaSubstitutedBinary` doc → reword to: it is the **placeholder** argv[0] the plan emits for the kwin
reconfigure, the one command whose entire argv the engine may replace. **Keep** `const plasmaSubstitutedBinary = "qdbus"`.

Do **not** change `plasmaSameReload` (`:365-374`). It still pairs correctly: `Engine.PlanApply` calls
`resolveReloads` (`internal/plasma/apply.go:118-125`), which replaces the whole argv at **plan** time, so
`plan.Reloads` already carries the busctl vector and `Engine.Reload`'s re-resolution is the identity. Prove it by
running cmd/dotf's plasma tests. Do not touch `internal/plasma/apply.go`.

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && gofmt -l internal/plasma cmd/dotf && go build ./... \
  && go vet ./internal/plasma/... ./cmd/dotf/... \
  && go test -race ./internal/plasma/... ./cmd/dotf/...
```

### WS5-T3 — `mimeapps.list`: make the five chrome pins volatile, and re-strip the snapshot Depends on [none]

**READ THESE BEFORE TASK**

- `/internal/plasma/table.go` (`:303-331`), `/internal/plasma/strip.go` (`:78-92`)
- `/internal/plasma/table_glob_test.go` (`:336-366`), `/internal/plasma/payload_test.go` (`:102`, `:187-220`)
- `/plasma/files/mimeapps.list`

**Files to Modify**

- `/internal/plasma/table.go`
- `/internal/plasma/table_glob_test.go`
- `/plasma/files/mimeapps.list`

**Instructions**

`plasma/files/mimeapps.list` pins `google-chrome.desktop` on **five** keys — lines 7 (`text/html`), 8
(`x-scheme-handler/about`), 13 (`http`), 14 (`https`), 15 (`unknown`) — a desktop id with **no aarch64 Linux build**.
`dotf plasma apply` would merge a broken default browser into the live session: the `mimeapps.list` spec
(`table.go:303-331`) does not cover them, because `VolatileKeys` names only mail/calendar keys and `VolatileValues`
matches only per-machine `userapp-*` ids.

> **MAJOR FIX (critique).** The first-pass spec used a blanket `[*]/x-scheme-handler/*` rule, which would also
> de-manage the four AI-app handlers on lines 9–12 (antigravity, claude, claude-cli, codex) — working, managed
> desktop state on the author's Arch daily driver, and a separate policy question the design's rationale (no
> aarch64 chrome build) does not reach. **Enumerate the five chrome keys instead.**

**EDIT 1 — `internal/plasma/table.go`.** In the `mimeapps.list` FileSpec, append to the **end** of the
`VolatileKeys` slice (which starts at `:310` with the `mailto` entry):

```go
				// The default browser is host-local: google-chrome.desktop has no
				// aarch64 Linux build at all, so merging these pins onto an Asahi
				// KDE host sets a default that cannot launch. Enumerated rather
				// than a blanket x-scheme-handler/* rule, which would also
				// de-manage the AI-app handlers (antigravity/claude/claude-cli/
				// codex) that DO work and are deliberately still managed.
				"[*]/text/html",
				"[*]/x-scheme-handler/about",
				"[*]/x-scheme-handler/http",
				"[*]/x-scheme-handler/https",
				"[*]/x-scheme-handler/unknown",
```

Rule grammar (`parseKeyRule`, `strip.go:78-91`): `<group-glob>/<key-glob>`; `[*]` is the any-depth group wildcard,
and the key glob may itself contain `/` because the group run is consumed by the leading bracket parse (the existing
`x-scheme-handler/mailto` entry already demonstrates this). `TestDefaultFiles_RulesParse` (`table_test.go:145-163`)
confirms they parse. Remove no existing entry; do not change `ApplyOrder` (2) or `SessionOwned` (false) —
`TestDefaultFiles_MatchesGolden` pins `{"mimeapps.list", 2, false}`.

**EDIT 2 — `internal/plasma/table_glob_test.go`**, `TestStripTable_MimeappsList` (`:336-366`). **One** existing case
is necessarily inverted (an existing-test update, not a new test):

```go
		{group: []string{"Default Applications"}, key: "text/html", value: "google-chrome.desktop", strip: false,
			why: "an ordinary portable default"},
```

becomes `strip: true` with `why: "the default browser is host-local; google-chrome has no aarch64 Linux build"`.

Leave every other case **unchanged** — in particular the `x-scheme-handler/claude` case must stay `strip: false`
(the AI-app handlers remain managed), and `[Added Associations]/image/webp` stays `strip: false`.

**EDIT 3 — `plasma/files/mimeapps.list`.** `TestCommittedSnapshot_SurvivesItsOwnStrip` (`payload_test.go:187-220`)
fails when the committed snapshot carries a key its own strip rules now delete, so the five newly-volatile lines
MUST be removed. Rewrite the file to exactly:

```ini
[Added Associations]
application/x-pcapng=org.wireshark.Wireshark.desktop;
image/webp=com.github.PintaProject.Pinta.desktop;

[Default Applications]
image/webp=com.github.PintaProject.Pinta.desktop
x-scheme-handler/antigravity=antigravity.desktop
x-scheme-handler/claude=com.anthropic.Claude.desktop
x-scheme-handler/claude-cli=claude-code-url-handler.desktop
x-scheme-handler/codex=chatgpt.desktop
```

(one trailing newline, no trailing whitespace). That deletes lines 7, 8, 13, 14, 15 and keeps everything else.
`TestCommittedPayload_ParsesAndIsCanonical` (`payload_test.go:102`) asserts the bytes are exactly what the codec
would write — if it reports a canonicality mismatch, adjust the file to the codec's rendering rather than changing
the codec.

Do not touch `plasma/recommended/` — it ships no `mimeapps.list`.

**Note for WS8-T6:** this is a user-visible behaviour change on **Arch too** (the repo stops managing the default
browser everywhere), so it belongs in the release notes' behaviour-changes section.

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && gofmt -l internal/plasma \
  && go test -race ./internal/plasma/... \
  && go test -race -run 'TestStripTable_MimeappsList|TestCommittedSnapshot_SurvivesItsOwnStrip|TestCommittedPayload_ParsesAndIsCanonical|TestDefaultFiles_RulesParse|TestDefaultFiles_MatchesGolden' ./internal/plasma/...
```

---

## Batch 3 — Facet rename, WS2 follow-ups, and the sysconfig split

### WS3-FACETS — ADR-12: `FacetLinux` / `FacetArchDesktop` / `FacetHyprland`, the `--hyprland` flag, and the playbook mirror Depends on [WS3-SAFETY]

**Merged from WS3-T3 + WS3-T4 + WS3-T5.** T3+T5 are mandated atomic (`table.go:62-65` declares `DefaultTable` a
frozen 1:1 transcription of `playbooks/vars/symlinks.yml`, so retagging `tmux.service` in Go while leaving it in
`archlinux_configs` breaks the stated SYNC contract). T4 is folded in because it writes the same `cmd/dotf/link.go`,
`applyopts.go` and `applyphases.go` that T3 must touch to keep the build compiling — merging makes `cmd/dotf/link.go`
genuinely single-owner and makes ADR-12 one reviewable change.

**READ THESE BEFORE TASK**

- `/internal/symlink/table.go` (`:22-35`, `:88-133`, `:143-169`), `/internal/symlink/symlink.go` (`:27-29`, `:121-129`, `:165`)
- `/internal/symlink/table_test.go`, `/internal/symlink/symlink_test.go`
- `/cmd/dotf/link.go`, `/cmd/dotf/applyopts.go`, `/cmd/dotf/applyphases.go`, `/cmd/dotf/helptext.go`,
  `/cmd/dotf/help_test.go`, `/cmd/dotf/applyopts_test.go`, `/cmd/dotf/apply_test.go` (`:717-748`)
- `/playbooks/vars/symlinks.yml` (`:57-58`, `:80-85`, `:112`), `/playbooks/tasks/archlinux.yml` (`:8`)

**Files to Modify**

- `/internal/symlink/table.go`, `/internal/symlink/symlink.go`
- `/internal/symlink/table_test.go`, `/internal/symlink/symlink_test.go`
- `/cmd/dotf/link.go`, `/cmd/dotf/applyopts.go`, `/cmd/dotf/applyphases.go`, `/cmd/dotf/helptext.go`
- `/cmd/dotf/help_test.go`, `/cmd/dotf/applyopts_test.go`
- `/playbooks/vars/symlinks.yml`

**Instructions**

ADR-12: facet names describe **capabilities**, not distros. `common/systemd/user/tmux.service` orders on
`graphical-session-pre.target` and runs a repo script — nothing Arch-specific — yet sits behind `FacetArch`, so
Fedora silently loses it. Meanwhile `arch-update.desktop` is CachyOS-branded (`Name=Cachy-Update`,
`Exec=… arch-update`) yet installs on **any** Linux host under `--linux-desktop`, and the whole `FacetLinuxDesktop`
set is the Hyprland/wlroots stack — categorically wrong on a KDE host.

**PART A — `internal/symlink/table.go`**

- **A1** Facet const block (`:22-35`): rename `FacetLinuxDesktop` → `FacetHyprland` and **append** two values after
  it (append, do not renumber the existing iota order):
  - `FacetLinux` — every Linux-family host, resolved through the **local** `isLinuxFamily` (`:143-150`), **never**
    `platform.IsLinuxFamily`. Load-bearing (correction **C14**): the local predicate deliberately excludes
    `platform.OSLinux`, and that exclusion is pinned by `table_test.go:139` and `symlink_test.go`'s `generic-linux`
    row.
  - `FacetArchDesktop` — Arch-family **and** the opt-in desktop flag.

  Doc-comment each in the existing style. Update `FacetHyprland`'s comment to say it is the Hyprland/wlroots stack
  (hypr, waybar, swayosd, mako, wlogout, fuzzel, satty + the generic launchers) and is wrong for a KDE host. **Keep**
  `FacetArch` even though its entry count drops to 0 — it is still referenced by `applies` and by `TestApplies` rows,
  and `FacetWindows` already sits at 0 as precedent.
- **A2** Retag `:116` `tmux.service` `FacetArch` → `FacetLinux`, and change its group comment on `:115` from
  `// archlinux_configs (Arch family only)` to `// linux_configs (Linux family — ADR-12: the unit is not Arch-specific)`.
- **A3** Retag `:111` `arch-update.desktop` → `FacetArchDesktop`. (The identical spec string also exists at
  `table_test.go:93` — disambiguate edits by file.)
- **A4** Rename the remaining **ten** `FacetLinuxDesktop` occurrences to `FacetHyprland`: `:101` hypr, `:102` waybar,
  `:103` swayosd, `:104` mako, `:105` wlogout, `:106` fuzzel, `:107` satty, `:110` alacritty-float.desktop, `:112`
  gedit-float.desktop, `:113` wifi-tui.desktop.
- **A5** `applies` switch (`:154-169`): rename `case FacetLinuxDesktop:` → `case FacetHyprland:` with body
  `return isLinuxFamily(info.OS) && opts.ManageHyprland`, and insert before `default:`:

  ```go
  	case FacetLinux:
  		return isLinuxFamily(info.OS)
  	case FacetArchDesktop:
  		return info.OS == platform.OSArch && opts.ManageHyprland
  ```

- **A6** Update the SYNC comment (`:62-65`) to record the two deliberate, now-permanent divergences from
  `playbooks/vars/symlinks.yml`: (i) the tmux user unit moves from `archlinux_configs` to `linux_configs` and **is**
  mirrored in the YAML (Part D); (ii) `arch-update.desktop` becomes Arch-gated in Go and is deliberately **not**
  mirrored, because doing so on a retired rollback-only playbook would need a new launcher group plus a new
  `tasks/*.yml` for zero rollback value.

**PART B — `internal/symlink/symlink.go`.** Rename `Options.ManageLinuxDesktop` → `ManageHyprland` (declaration and
doc at `:27-29`; note the `--linux-desktop` flag is retained as a back-compat alias) and its one use in `Plan`'s
guard at `:165`. Do **not** rename the exported `LinuxDesktopGuardError` (`:121-129`) — `internal/plasma/errors.go:162`
references it by name and the flag survives, so the name stays accurate.

**PART C — `cmd/dotf`: the `--hyprland` flag surface.** `--hyprland` becomes the primary flag on both `link` and
`apply`; `--linux-desktop` and `LINUX_DESKTOP` stay registered as deprecated aliases, OR'd with the new names.

> **Correction C33.** The first-pass justification claimed "`bootstrap.sh` passes `LINUX_DESKTOP` as an env twin".
> It does not — `bootstrap.sh` contains **zero** occurrences of `LINUX_DESKTOP` or `hypr`. The alias is retained
> because ADR-12 mandates it, because `cmd/dotf/apply_test.go:717-748` passes the literal string, and because
> README.md (`:59`, `:64`, `:146`, `:170`, `:234-235`, `:329`, `:388-394`) and CHANGELOG.md:61 document it by name.

CLI convention reminder (settled — do not "fix"): Go stdlib `flag`, GNU-style double-dash long flags only, no short
flags, no Cobra/pflag.

- `cmd/dotf/link.go`: add a `hyprland bool` field to `linkOpts` (`:127-130`), keep `linuxDesktop`; in `setLinkFlags`
  (`:135-143`) keep `--linux-desktop` but change its usage string to `"deprecated alias for --hyprland"`, and register
  `--hyprland` with a real description of the Hyprland/wlroots set; `runLink` (`:57`) becomes
  `ManageHyprland: o.hyprland || o.linuxDesktop,`.
- `cmd/dotf/applyopts.go`: in `applyFlags` (`:12-22`) rename the field `LinuxDesktop` → `Hyprland` **and add** a
  separate `LinuxDesktop bool` for the alias (both must parse independently); in `setApplyFlags` (`:37`) keep
  `--linux-desktop` with a deprecated-alias usage string and add `--hyprland`; in `applyOptions` (`:58-68`) rename
  `LinuxDesktop` → `Hyprland` (one resolved field); in `resolveApplyOptions` (`:104`)
  `Hyprland: f.Hyprland || f.LinuxDesktop || envTrue(getenv("HYPRLAND")) || envTrue(getenv("LINUX_DESKTOP")),`.
  Update the `applyOptions` doc (`:52-57`) and the env-twin comment block (`:106-111`) to name both HYPRLAND and the
  retained LINUX_DESKTOP alias, keeping the "every apply twin is unprefixed" rationale.
- `cmd/dotf/applyphases.go` — **four lines only** (this is the second of four serialized writers of this file):
  the doc comment at `:219`, and inside `applySymlinkOptions` (`:294-305`) both `Backup: opts.LinuxDesktop` (`:300`)
  and the struct key `ManageLinuxDesktop:` (`:303`), which becomes `ManageHyprland: opts.Hyprland`. Update
  `applySymlinkOptions`' doc comment (`:294-297`) to say `--hyprland` (aliased by `--linux-desktop`). Re-gofmt the
  literal's key alignment; reflow nothing else.
- `cmd/dotf/helptext.go`: `envHelp` (`:36-41`) gains HYPRLAND with LINUX_DESKTOP noted as its deprecated alias; the
  `link` entry (`:182-197`) changes its example to `dotf link --dry-run --hyprland` and gains one sentence that the
  set is the Hyprland/wlroots stack and is **not** what a KDE/Plasma host wants (that is `dotf plasma`); the `apply`
  entry's `long` (`:222-227`) names HYPRLAND, keeping LINUX_DESKTOP as the deprecated alias.
  `TestCommandHelp_DRY_NoDrift` (`help_test.go:475-499`) mechanically requires every registered flag usage string to
  appear verbatim in rendered help — automatic via `renderFlags`; do **not** hand-duplicate usage text into `long`.
- `cmd/dotf/help_test.go` — pinned golden updates: `:455` add `"--hyprland"`; `:518`
  `{"--backup", "--dry-run", "--force", "--hyprland", "--linux-desktop", "--no-validate"}`; `:595-596` add
  `o.hyprland` to the "unexpected true flag" guard; `:643`/`:652` add `"--hyprland"` to the parsed args and
  `!f.Hyprland` to the assertion (keep `!f.LinuxDesktop` — both flags are passed, both must be true); `:695`
  `{"backup", "dry-run", "force", "hyprland", "linux-desktop", "no-validate"}`; `:721` insert `"hyprland"` in sorted
  position. Search the whole file for any other `linux-desktop`/`LinuxDesktop`; `go build` + `go test` is ground
  truth.
- `cmd/dotf/applyopts_test.go` — rows at `:67-74` use `applyFlags{LinuxDesktop: true}` / `applyOptions{LinuxDesktop: true}`
  and env `LINUX_DESKTOP`. Update to the new field names (`applyOptions{Hyprland: true}` on the want side for **both**
  the flag row and the env row, since the alias resolves into Hyprland) and **add** two sibling rows proving the new
  names work. At `:174` add `"HYPRLAND": v` alongside `"LINUX_DESKTOP": v`; at `:180` change `got.LinuxDesktop` to
  `got.Hyprland`.
- `cmd/dotf/apply_test.go` needs **no** edit: `TestRunApply_LinuxDesktopFlagPassthrough` (`:717-748`) passes the
  literal `--linux-desktop` and asserts `.config/hypr` becomes a symlink — that is now exactly the back-compat proof
  and must stay green unmodified. If it fails, the alias wiring is wrong.

**PART D — `playbooks/vars/symlinks.yml`** (data-file edit only; the playbook is historical-rollback-only — do
**not** run it). Replace `:57-58`:

```yaml
# Linux-specific configurations
linux_configs: []
```

with:

```yaml
# Linux-specific configurations (systemd user services; ADR-12 — the tmux unit
# orders on graphical-session-pre.target and runs a repo script, so it is not
# Arch-specific)
linux_configs:
  - { src: 'common/systemd/user/tmux.service', dest: '.config/systemd/user/tmux.service' }
```

and replace `:80-82`:

```yaml
# Arch Linux-specific configurations (systemd user services)
archlinux_configs:
  - { src: 'common/systemd/user/tmux.service', dest: '.config/systemd/user/tmux.service' }
```

with:

```yaml
# Arch Linux-specific configurations (none; the tmux user unit moved to
# linux_configs — ADR-12)
archlinux_configs: []
```

The explicit `[]` is **required**: an empty key parses as null and `playbooks/tasks/archlinux.yml:8`
(`loop: '{{ archlinux_configs }}'`) would fail on null. `windows_configs: []` at `:85` is the in-file precedent.
`playbooks/symlinks.yml:112` already folds `linux_configs` in under `is_linux_family`, so no task file changes.
Leave `launcher_configs` (`:70-78`) untouched. 2-space indent, single-quoted scalars, inline flow maps.

**PART E — test tables**

- `table_test.go` `TestTable_Count` (`:10`): **no change** — retagging does not change the entry count (41).
- `TestTable_FacetCounts` `want` (`:21-27`) becomes `FacetAll: 28, FacetMacOS: 1, FacetArch: 0, FacetWindows: 0,
  FacetLinux: 1, FacetHyprland: 10, FacetArchDesktop: 1`. Re-gofmt (map values are column-aligned).
- `TestTable_MatchesPlaybook` golden (`:64-106`): mirror A2/A3/A4 exactly — `:85-92` and `:94-95` → FacetHyprland,
  `:93` → FacetArchDesktop, `:96` → FacetLinux. Struct equality at `:113`, so byte-identical to `table.go`
  (including the `BackupEligible` fields WS3-SAFETY added).
- `TestApplies` (`:119-152`): rename the five `FacetLinuxDesktop` rows (`:135-139`) to `FacetHyprland`, keeping
  `{"desktop-generic-linux-ld-on", FacetHyprland, platform.OSLinux, true, false}` at `:139` **unchanged in
  expectation** (it pins the OSLinux exclusion). Append these rows (rows in an existing table, not new tests):

  ```go
  		{"linux-on-redhat", FacetLinux, platform.OSRedHat, false, true},
  		{"linux-on-arch", FacetLinux, platform.OSArch, false, true},
  		{"linux-on-macos", FacetLinux, platform.OSMacOS, false, false},
  		{"linux-on-generic-linux", FacetLinux, platform.OSLinux, false, false},
  		{"hyprland-on-redhat-ld-on", FacetHyprland, platform.OSRedHat, true, true},
  		{"archdesktop-on-arch-ld-on", FacetArchDesktop, platform.OSArch, true, true},
  		{"archdesktop-on-arch-ld-off", FacetArchDesktop, platform.OSArch, false, false},
  		{"archdesktop-on-redhat-ld-on", FacetArchDesktop, platform.OSRedHat, true, false},
  ```

- `TestFaceting_WSL` (`:156-165`): `FacetLinuxDesktop` → `FacetHyprland`; `Options{ManageLinuxDesktop: true}` →
  `Options{ManageHyprland: true}`; update the error string on `:163`.
- `symlink_test.go` `:70`: `Options{ManageHyprland: tt.ld, DryRun: true}`.
- `symlink_test.go` `TestPlan_IdenticalSet_PerOS` (`:34-62`) — recompute against the retag:
  - `macos` (`:35-38`): unchanged, 29.
  - `arch-no-desktop` (`:40-43`): unchanged, 29 (28 + FacetLinux 1, previously 28 + FacetArch 1).
  - `arch-desktop` (`:45-48`): stays 40; **add** `".local/share/applications/arch-update.desktop"` to `mustHave`
    (proves FacetArchDesktop still applies on Arch).
  - `debian-no-desktop` (`:50-53`): 28 → **29**; move `".config/systemd/user/tmux.service"` from `mustNotHave` to
    `mustHave`.
  - `debian-desktop` (`:55-58`): **stays 39** — leave a comment reasoning it out: tmux.service is gained (+1)
    exactly as arch-update.desktop is lost (−1), a coincidental cancellation that would otherwise read as a false
    pass. Move tmux.service to `mustHave` and add `".local/share/applications/arch-update.desktop"` to `mustNotHave`.
  - `generic-linux` (`:60-62`): unchanged, 28 (`isLinuxFamily` excludes OSLinux).
  - Append the Fedora row:

    ```go
    		{
    			name: "redhat-no-desktop", os: platform.OSRedHat, wantCreated: 29,
    			mustHave:    []string{".bashrc", ".config/nvim", ".config/systemd/user/tmux.service"},
    			mustNotHave: []string{".config/aerospace", ".config/hypr", ".local/share/applications/arch-update.desktop"},
    		},
    ```

- `TestPlan_LinuxDesktopGuard` (`:241-244`) and `TestPlan_LinuxDesktopGuard_NonLinuxUnaffected` (`:277`): rename every
  `Options{ManageLinuxDesktop: …}` to `ManageHyprland`. Keep the test function names and the
  `LinuxDesktopGuardError` assertions.

Do not add a new test file. Do **not** edit README.md, CLAUDE.md, CHANGELOG.md, plasma/README.md or `docs/**` —
WS8 owns documentation and would clobber it. Record the new flag in the commit body instead.

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && gofmt -l internal/symlink cmd/dotf && go build ./... && go vet ./... \
  && go test -race ./... \
  && go run ./cmd/dotf link --dry-run --hyprland >/dev/null \
  && go run ./cmd/dotf link --dry-run --linux-desktop >/dev/null \
  && go run ./cmd/dotf help link | grep -e --hyprland -e --linux-desktop
```

### WS2-T4 — Regenerate `manifest/dotf.lock` with the redhat targets Depends on [WS2-CORE]

**READ THESE BEFORE TASK**

- `/manifest/dotf.lock`, `/cmd/dotf/lock.go` (`:193-212`), `/internal/platform/types.go` (`:49`), `/embed.go` (`:21`)

**Files to Modify**

- `/manifest/dotf.lock`

**Instructions**

ADR-9: widen the lock for **reproducibility**. (The earlier framing that installs would otherwise fail closed is an
overstatement — `internal/backend/github_release.go:79-87` falls through to `resolveAsset` on a lock miss and the
sidecar/inline checksum still populates `wantSHA`, so the pinned check at `:89-91` passes. The real cost of a miss is
that the Asahi box would be the only host installing off a live GitHub API call.) The lock is `go:embed`-ed
(`embed.go:21`), so it must ship in the release the device downloads.

**Two corrections to the design's command — measured; do not use the design's literal invocation:**

- **C27**: the design says `--platform redhat/amd64`. `parsePlatform` (`cmd/dotf/lock.go:206-212`) constructs
  `platform.Arch(parts[1])` verbatim, and the **canonical** arch value is `x86_64` (`internal/platform/types.go:49`,
  whose comment says it MUST match `common/platform.sh`'s `detect_arch` output exactly, "never `amd64`"). `amd64` is
  only the RELEASE arch produced by `ReleaseArch()`, which is why the committed lock records `arch = 'amd64'`.
  Measured: `--platform redhat/amd64` produces `failed=3` / exit 1 for turso, lazygit-bin and yazi-bin.
- **C28**: `lockTargets` (`:193-203`) always prepends the detected **host** target, and `resolveLock` builds a
  **fresh** lock — it does not merge. Omitting `arch/x86_64` would DROP the existing `turso` @ `os = 'arch'
  arch = 'amd64'` artifact and regress Arch reproducibility.

Run from the repo root:

```bash
go run ./cmd/dotf lock --no-user-config \
  --manifest manifest/packages.toml \
  --lock manifest/dotf.lock \
  --platform arch/x86_64 --platform redhat/arm64 --platform redhat/x86_64
```

Network is required (GitHub REST + the `checksums.txt` sidecar); set `GH_TOKEN`/`GITHUB_TOKEN` if you hit the
unauthenticated rate limit (`internal/backend/ghapi.go:githubToken`).

**EXPECTED RESULT** (measured via `--dry-run`: `packages=3 artifacts=12 failed=0`, exit 0): three locked packages —
`lazygit-bin`, `turso`, `yazi-bin` — each with artifacts for `arch/amd64`, `redhat/amd64`, `redhat/arm64`, plus one
extra row for whatever family the machine you run on detects as. That extra host row is **expected**:
`Engine.Resolve` (`internal/backend/backend.go:122-131`) is os-gate-agnostic, so an `os = "redhat"` package still
resolves for the host target. It is inert (Install short-circuits on the os gate) and is a legitimate pin for that
family. Call it out in the commit body; do **not** hand-trim it.

**Verify these three things in `git diff manifest/dotf.lock`:**

- the pre-existing `turso` artifact `os = 'arch'` / `arch = 'amd64'` /
  `sha256 = '0eae140b030b11ad98540ed85ceb62575100135e51038009064e50675822c0f5'` is present and unchanged;
- `yazi-bin`'s `redhat/arm64` sha equals the inline `checksums["linux/arm64"]` in `manifest/packages.toml`
  (`f5a8577…`) and its `*/amd64` shas equal `checksums["linux/x86_64"]` (`cc67eb7…`) — a mismatch means the tag or
  the inline pin is wrong;
- `lazygit-bin`'s `redhat/arm64` sha is `8b7ca3b344e60340ad1f89f29b9868ee39bcaba5bb92ee818bbe65476bb8b6e7` and its
  amd64 shas are `f8ea237c41f194cd799b48505518bfdaae4edf5a2ad6bd3d898e939785ee4532` (from v0.64.1's `checksums.txt`).

**NEVER** hand-edit `manifest/dotf.lock`; it is machine-written canonical TOML.

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && go run ./cmd/dotf lock --no-user-config --manifest manifest/packages.toml --lock manifest/dotf.lock \
       --check --platform arch/x86_64 --platform redhat/arm64 --platform redhat/x86_64 \
  && go build ./... && go test -race ./...
```

### WS2-T5 — `.docs/fedora-package-mapping.md` Depends on [WS2-CORE]

**READ THESE BEFORE TASK**

- `/.docs/arch-package-mapping.md` (fixes the column shape this doc must mirror — read it first)
- `/manifest/packages.toml`, `/docs/plans/fedora-support/design.md` (§4), `/.markdownlint-cli2.jsonc`

**Files to Create**

- `/.docs/fedora-package-mapping.md`

**Instructions**

Mirror `.docs/arch-package-mapping.md`'s column shape
(`| Homebrew Package | Arch Package | Repository | Notes/Alternative Method |`, with a `Research completed: <date>`
line under the H1). `.docs/` is tracked in git, not ignored.

1. `# Fedora Package Mapping` + a provenance block: the table commits to package **names**, not versions (two
   research datasets disagreed — one reported `fc43` NEVRs, another `fc44`); plus the exact Fedora release and date
   it was verified against, taken from the WS0 device capture's `dnf --version` + `cat /etc/os-release` +
   `dnf list --available lazygit yazi lutris docker-ce moby-engine` output. If the capture has not landed, write
   `Verified against: Fedora <release> (aarch64), <date> — PENDING WS0 device capture` and leave a visible TODO.
2. A legend for the **Fedora path** column: `official` = Fedora release/updates repos · `copr:<owner>/<project>` ·
   `github-release` = upstream release asset via the existing backend · `vendor .repo` = third-party dnf repository ·
   `toolchain` = language package manager · `n/a` = correctly not-applicable.
3. **pkg backend (13 entries)** table, columns `| Package | Manifest line | dnf name | Fedora path | aarch64 | Verdict |`,
   transcribed from design §4 and re-confirmed against the post-WS2-CORE manifest. Key rows: `fd` → **`fd-find`**
   (ships `/usr/bin/fd`, so no Debian-style rename shim); `bat` → **`bat`** (Fedora's binary is `bat`, not `batcat`);
   `delta` → **`git-delta`** (Fedora **also** ships an unrelated package literally named `delta` — do **not**
   shorten); `gh` → `gh` (Arch uses `github-cli`); **lazygit** → no dnf key, `lazygit-bin` github-release
   (`jesseduffield/lazygit`, `v0.64.1`, `checksums.txt` sidecar); **yazi** → no dnf key, `yazi-bin` github-release
   (`sxyazi/yazi`, `v26.8.15`, **inline per-target `checksums`** because upstream ships no combined sidecar; note the
   `ya` plugin-manager binary is **not** installed).
4. **Non-pkg backends** table from design §4's second table (neovim, atuin, mise, rust, lua, claude, codex,
   opencode, fabric, coderabbit, aws-tools, azure-cli, hashicorp-tools, container-tools, turso, tea, lefthook, tpm,
   tmux-sessionizer, gnu-utilities, lutris/lutris-pkg, docker-ce, starship), preserving the design's caveats in
   spirit: `coderabbit` is an **arm64 risk** (opt-in `ai` only); `aws-tools`' `ecs-cli` is x86_64-only and EOL so the
   all-present probe can never satisfy; `azure-cli`'s RHEL-9 Microsoft repo path has no aarch64 RPM;
   `hashicorp-tools`' `yum-config-manager` is absent on dnf5; `codex`/`lefthook`/`tmux-sessionizer`/`fabric` are
   pre-existing cross-platform holes (non-goal); `gnu-utilities` is `os = "macos"`, correctly not-applicable;
   `docker-ce` is apt-gated today and is WS7's `dnf_repo` work.
5. **Summary**: 11 of 13 pkg entries were Fedora-correct already; 2 (`lazygit`, `yazi`) were outright wrong and
   caused a non-zero exit on every Fedora `dotf apply`; 2 (`docker-ce`, `lutris`) were structurally unreachable and
   vanished silently as `not-applicable` — `lutris` fixed here by `lutris-pkg`, `docker-ce` deferred to WS7.
6. Closing note: **no version is asserted**, and RPM Fusion is deliberately **not** enabled (nothing in the manifest
   needs it; every resolving dnf name comes from Fedora's own release/updates repos).

Do not restate this content elsewhere; do not touch `docs/reference/manifest-backends.md` (WS5-T7/WS7-T7 own it).

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && npx --no-install markdownlint-cli2 '.docs/fedora-package-mapping.md' \
  && npx --no-install prettier --check '.docs/fedora-package-mapping.md'
```

(Scope to the single new file — `bin/lint.sh --docs` runs against a pre-existing repo-wide markdown backlog.)

### WS5-T4 — Manifest schema: the `font` backend enum, `[font]` sub-table, validation, class Depends on [none]

**READ THESE BEFORE TASK**

- `/internal/manifest/types.go` (`:78`, `:165-175`, `:285-305`, `:320-368`)
- `/internal/manifest/validate.go` (`:200-247`, `:390-420`), `/internal/manifest/class.go` (`:39-103`)
- `/internal/manifest/class_test.go`

**Files to Modify**

- `/internal/manifest/types.go`, `/internal/manifest/validate.go`, `/internal/manifest/class.go`

**Instructions**

Give ADR-13's native `font` backend its typed manifest contract. Schema-only: it must compile and pass every
existing test on its own (no manifest entry uses the backend yet).

**EDIT 1 — `types.go` Package field**, immediately after `AptRepo *AptRepo \`toml:"apt_repo"\`` (`:171-173`):

```go
	// Font configures the "font" backend: the fontconfig family name that is
	// the installed-state probe, and the upstream release archive base name
	// used when no host package manager ships a patched build.
	Font *Font `toml:"font"`
```

**EDIT 2 — the `Font` struct**, next to `AptRepo` (`:289-303`):

```go
// Font describes one font family for the "font" backend. The repo's terminal
// and editor configs pin patched family names by exact string, so the family
// name — not a package name — is the thing that must end up installed.
type Font struct {
	// Family is the fontconfig family name the configs pin (e.g.
	// "JetBrainsMono Nerd Font"). It is the fc-list installed-state probe.
	Family string `toml:"family"`

	// Archive is the base name of the upstream release archive (e.g.
	// "JetBrainsMono"). The backend downloads "<Archive>.zip" from Repo@Tag
	// and extracts it under the XDG fonts directory. Empty means this family
	// is only installable through a host package manager (Names).
	Archive string `toml:"archive"`
}
```

**EDIT 3 — the Backend enum**, after `BackendGitClone` (`:354-357`):

```go
	// BackendFont installs a font family: via the host package manager when it
	// ships a patched build (Names), otherwise by extracting the checksum-
	// verified upstream release archive named by Font.Archive into the XDG
	// fonts directory.
	BackendFont Backend = "font"
```

Add `BackendFont` to `Valid()`'s case list (`:363`).

**EDIT 4 — comment hygiene.** Lines `:78`, `:325` and `:360` say "eight". WS7-T2 adds a ninth value to this same
file, so do **not** write a new numeral — reword count-free: `:78` → "required across every supported backend (see
Backend)"; `:325-326` → "Canonical Backend values. These are the only installation mechanisms the provisioning CLI
supports."; `:360` → "Valid reports whether b is one of the canonical Backend values."

**EDIT 5 — `validate.go`.** Add an arm to `validatePackage`'s backend switch (`:200-247`) alongside
`case BackendGitClone:`:

```go
	case BackendFont:
		errs = append(errs, validateFont(p)...)
```

and add `validateFont` beside the other per-backend validators. Rules, each emitting
`&ValidationError{Package: p.Name, Msg: …}`:

- `p.Font == nil` → `"font backend requires font"` (return early).
- `p.Font.Family == ""` → `"font backend requires font.family"`.
- `p.Font.Archive == "" && len(p.Names) == 0` → `"font backend requires font.archive or names"`.
- `p.Font.Archive != ""` and `p.Repo` does not match the repo-slug shape →
  `"font backend requires repo owner/repo when font.archive is set"`. Use the same shape check style as
  `validateGitHubRelease`; if that helper is not reachable, add a local regexp
  `^[a-zA-Z0-9][a-zA-Z0-9._-]*/[a-zA-Z0-9][a-zA-Z0-9._-]*$`.
- `p.Font.Archive != "" && p.Tag == ""` → `"font backend requires tag when font.archive is set"`.
- **BLOCKER FIX (critique) — the checksum-source rule.** `p.Font.Archive != ""` and
  `p.SHA256 == "" && p.ChecksumsAsset == ""` →
  `"font backend requires a checksum source (sha256 or checksums_asset) when font.archive is set"`.
  **Amendment (PR #36 review):** this originally also accepted the per-target `Checksums` map and advertised all
  three sources, mirroring `validateGitHubRelease`. That was wrong in the font case — `backend.fontChecksum`
  reads only `SHA256` then `ChecksumsAsset`, so an entry pinned via `checksums` validated clean and then
  hard-failed at install with `FontNoChecksumError` on every host. A `<os>/<arch>` key has no meaning for a single
  arch-independent Nerd Font zip anyway, so the schema is narrowed to the two sources the backend actually reads.
  **It is not optional**:
  `VerifySHA256` (`internal/backend/download.go:117-120`) returns `nil` on an **empty** want, and its own doc warns
  that "callers that must fail closed MUST reject an empty want BEFORE calling this". Without this rule the `font`
  backend can download and extract a remote archive with **no verification at all**, on the default-intent path.

No change is needed to `knownPkgMgrs` (`:64-71`, already pacman/homebrew/dnf/apt/yum/zypper) or `knownPackageOS`
(`:475-483`, already contains `"redhat"`).

**EDIT 6 — `class.go`.** Add `BackendFont: {ClassManaged: true},` to `classByBackend` (`:45-54`) and add
`BackendFont` to the `case BackendPkg, BackendAUR, BackendAPTRepo, BackendToolchain, BackendGitClone:` arm of
`EffectiveClass` (`:91`) so it infers `ClassManaged`. This is **required**: `TestPartitionByClass` demands every
embedded-manifest package resolve to a non-empty class, and WS5-T6 adds three font packages. Update the
`classByBackend` doc (`:39-44`) to name font among the authority-managed backends. Managed (not pinned) is
deliberate: the pacman/Homebrew arms defer to the package authority and the download arm verifies its checksum at
install time rather than through the lockfile — so `lockfile.Pinnable` stays false for font and `dotf lock` never
tries to resolve it.

Add **no** new test file. `class_test.go`'s `TestPackage_EffectiveClass` and `TestLegalClasses` are curated (not
exhaustive) tables and stay green unchanged.

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && gofmt -l internal/manifest && go build ./... && go vet ./internal/manifest/... \
  && go test -race ./internal/manifest/...
```

### WS6-T1 — Phase-one split: carve `linux.go` into three files with ZERO behaviour change Depends on [none]

**READ THESE BEFORE TASK**

- `/internal/sysconfig/linux.go` (all 554 lines), `/internal/sysconfig/sysconfig.go`, `/internal/sysconfig/locale_resolve.go`

**Files to Create**

- `/internal/sysconfig/linux_locale.go`
- `/internal/sysconfig/linux_firewall.go`

**Files to Modify**

- `/internal/sysconfig/linux.go`

**Instructions**

**PURE CODE MOVE.** Do not rename, reorder within a function, change a signature, change an argv, or edit a doc
comment. `linux.go` is 554 lines against the ~500 soft cap and every subsequent WS6 task lands in it, so it is split
**first, in its own commit**, as the behaviour-preservation proof (design R10).

**CREATE `internal/sysconfig/linux_locale.go`:**

```go
// Locale planning for the systemd Linux family: locale-gen / localectl. Split
// out of linux.go, which exceeded the repo's ~500-line soft cap.
package sysconfig

import (
	"strings"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/platform"
)
```

Move into it, verbatim and in this order, cutting them out of `linux.go`: `planLocale` (`:111-155` including its doc
comment — note **C22**: the span is `:115-155`, doc from `:111`); `localeGenSteps` (`:157-184`); `currentLocaleLinux`
(`:521-537`).

**CREATE `internal/sysconfig/linux_firewall.go`:**

```go
// Firewall planning for the systemd Linux family: ufw (debian/arch/wsl) and
// firewalld (redhat/suse). Split out of linux.go, which exceeded the repo's
// ~500-line soft cap.
package sysconfig

import (
	"strings"

	"github.com/yandy-r/dotfiles/internal/platform"
)
```

Move into it, verbatim and in this order: `planFirewall` (`:240-263`), `planUfw` (`:265-306`), `ufwStatus`
(`:308-323`), `FirewallPlanNeedsSudo` (`:325-343`), `planFirewalld` (`:345-370`), `ufwInstallArgv` (`:372-378`),
`ufwHasSSHRule` (`:380-383`), `ufwActive` (`:385-388`).

`linux.go` **retains**, in existing order: package doc + imports, `type linuxConfigurer`, `newLinuxConfigurer`,
`Plan`, `planHostname`, `planTimezone`, `planShell`, `planSSHServer`, `planSSHServerFirewallOpen`, `sshServiceName`
(`:484-491`, per **C23**), `sshInstallArgv` (`:494-505`), `SSHServerPlanNeedsSudo`, `currentShellLinux`, and the
trailing `var _ Configurer = (*linuxConfigurer)(nil)`.

Fix `linux.go`'s import block afterwards: it still needs `fmt`, `strings`, `exec` and `platform`. Run
gofmt/goimports. **Do not touch `internal/sysconfig/linux_test.go`** — its remaining unmodified state is this task's
whole point.

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && gofmt -l internal/sysconfig && go build ./... && go vet ./... \
  && go test -race ./internal/sysconfig/... \
  && test -z "$(git diff --name-only -- internal/sysconfig/linux_test.go)" || { echo 'FAIL: linux_test.go was modified'; exit 1; } \
  && wc -l internal/sysconfig/linux.go internal/sysconfig/linux_locale.go internal/sysconfig/linux_firewall.go
```

---

## Batch 4 — Desktop XDG paths, the font backend, and package-manager ordering

### WS5-T2 — Plasma engine: honour `$XDG_DATA_HOME` and `$XDG_DATA_DIRS` Depends on [none]

**READ THESE BEFORE TASK**

- `/internal/plasma/plasma.go` (`:45`, `:300-330`), `/internal/plasma/strip.go` (`:330-347`)
- `/internal/plasma/helpers_test.go` (`:25-35`), `/internal/plasma/export_test.go` (`:12`)
- `/cmd/dotf/completion.go` (`:196-213`), `/cmd/dotf/plasma.go` (`:145-165`)
- `/cmd/dotf/applyphases.go` (`:180-190`, `:398-408`), `/cmd/dotf/doctorplasma.go` (`:45-60`), `/cmd/dotf/doctor.go` (`:55-70`)
- `/cmd/dotf/doctor_test.go`, `/cmd/dotf/apply_test.go` (`:975-1035`)

**Files to Modify**

- `/internal/plasma/plasma.go`, `/internal/plasma/strip.go`, `/internal/plasma/helpers_test.go`, `/internal/plasma/export_test.go`
- `/cmd/dotf/completion.go`, `/cmd/dotf/plasma.go`, `/cmd/dotf/applyphases.go`, `/cmd/dotf/doctorplasma.go`,
  `/cmd/dotf/doctor.go`, `/cmd/dotf/doctor_test.go`, `/cmd/dotf/apply_test.go`

**Instructions**

`internal/plasma`'s colour-scheme guard hard-derives the XDG data home from `configDir` (`plasma.go:325`) and
hard-codes **one** system directory (`systemColorSchemesDir = "/usr/share/color-schemes"`, `:45`, assigned at `:326`),
so it misreads a host with `$XDG_DATA_HOME` set and misses Fedora's `/usr/local/share` entry of `$XDG_DATA_DIRS`.
Sole consumer: `strip.go:332-335` inside `hasColorSchemeFile`. `internal/plasma` deliberately performs **no**
environment lookups (see `DetectSession(getenv)`'s doc at `plasma.go:456-459`); env resolution belongs in `cmd/dotf`,
which already has `xdgDataHome(home)` (`completion.go:196-205`) and `xdgConfigHome(home)` (`:207-213`).

**EDIT 1 — `plasma.go`.** Widen `systemColorSchemes string` to `[]string` (doc: "the ORDERED distribution
colour-scheme directories") and change the constructor to:

```go
func NewEngine(repoRoot, configDir, dataHome string, systemColorSchemes []string, r exec.Runner) *Engine
```

Document the semantics: an **empty** `dataHome` falls back to the historical
`filepath.Join(filepath.Dir(configDir), ".local", "share")` derivation, so a caller that does not care keeps today's
behaviour; a nil/empty `systemColorSchemes` falls back to `[]string{systemColorSchemesDir}`. Copy the caller's slice
into the Engine (do not retain the caller's backing array). Keep the const at `:45`.

**EDIT 2 — `strip.go:332-335`.** Replace the two-element literal with a list built as: the user dir
`filepath.Join(e.dataHome, "color-schemes")` **first**, then every entry of `e.systemColorSchemes` in order. Keep the
rest of `hasColorSchemeFile` (case-insensitive `<scheme>.colors` basename match, `continue` on ReadDir error)
byte-identical.

**EDIT 3 — `helpers_test.go:29-30`.** Currently:

```go
	e := NewEngine(repoRoot, configDir, exec.NewFake())
	e.systemColorSchemes = filepath.Join(t.TempDir(), "absent")
```

Replace with a single hermetic construction
`e := NewEngine(repoRoot, configDir, "", []string{filepath.Join(t.TempDir(), "absent")}, exec.NewFake())` and
**delete** the field-assignment line. The whole plasma test suite depends on this helper never reading the host's
`/usr/share`.

**EDIT 4 — `export_test.go:12`**: `NewEngine(filepath.Join(t.TempDir(), "absent"), t.TempDir(), "", nil, nil)`.

**EDIT 5 — `cmd/dotf/completion.go`**, beside `xdgDataHome`/`xdgConfigHome`:

```go
// xdgDataDirs returns the absolute entries of $XDG_DATA_DIRS in order,
// defaulting to the spec's /usr/local/share:/usr/share when unset or when no
// entry is absolute. Per the XDG spec a relative entry is invalid and dropped
// (same rule as xdgDataHome).
func xdgDataDirs() []string
```

Split on `os.PathListSeparator`, keep only `filepath.IsAbs` entries, return `[]string{"/usr/local/share", "/usr/share"}`
when the result is empty.

**EDIT 6 — `cmd/dotf/plasma.go`.** Add:

```go
// plasmaSystemColorSchemeDirs is the ordered system colour-scheme search path:
// "color-schemes" under every $XDG_DATA_DIRS entry. On a default host this is
// /usr/local/share/color-schemes then /usr/share/color-schemes — a superset of
// the single hard-coded directory it replaces.
func plasmaSystemColorSchemeDirs() []string
```

Then change `:159` from `eng: plasma.NewEngine(repoRoot, configDir, r),` to
`eng: plasma.NewEngine(repoRoot, configDir, xdgDataHome(home), plasmaSystemColorSchemeDirs(), r),`. `home` is already
in scope (`:150` computes `configDir := xdgConfigHome(home)`).

**EDIT 7 — `cmd/dotf/applyphases.go`, BOTH call sites** (`:182` in `applyPlasmaPhase`, `:403` in the dry-run plan
builder), currently `pe := plasma.NewEngine(repoRoot, xdgConfigHome(home), r)` → add
`xdgDataHome(home), plasmaSystemColorSchemeDirs(),`. `home` is a parameter at both sites. This is the **third** of
four serialized writers of this file — touch only these two lines.

**EDIT 8 — `cmd/dotf/doctorplasma.go` + `cmd/dotf/doctor.go` — MAJOR FIX (critique).** The first-pass spec passed an
empty `dataHome` in doctor on the premise that `checkPlasma` has "no `home` in scope". **That premise is false**
(correction **C34**): `home, _ := os.UserHomeDir()` is at `cmd/dotf/doctor.go:60`, four lines above the `checkPlasma`
call at `:64`. Passing an empty dataHome would make `dotf doctor` and `dotf plasma diff` resolve the colour-scheme
guard differently on **any** host with `$XDG_DATA_HOME` set (Arch and Debian included) — directly contradicting
`plasmaDriftCheck`'s own doc (`doctorplasma.go:56-60`): _"doctor and `dotf plasma diff` cannot disagree."_

So: widen `checkPlasma` to `checkPlasma(repoRoot, configDir, dataHome string, info platform.Info, r exec.Runner)`,
pass `xdgDataHome(home)` from `doctor.go:64`, and at `doctorplasma.go:51` construct
`plasma.NewEngine(repoRoot, configDir, dataHome, plasmaSystemColorSchemeDirs(), r)`. Update the six `checkPlasma`
call sites in `cmd/dotf/doctor_test.go` (`:395`, `:402`, `:409`, `:434`, `:457`, `:485`) with an extra
`t.TempDir()`/`""` argument — the same mechanical churn this task already accepts for `helpers_test.go` and
`apply_test.go`.

**EDIT 9 — `cmd/dotf/apply_test.go`**, two construction sites (`:983`, `:1030`): add `"", nil` as the third and
fourth arguments, preserving today's derived behaviour.

Before finishing, confirm the complete caller set with
`grep -rn 'plasma.NewEngine(\|= NewEngine(' --include '*.go' internal/plasma cmd`.

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && gofmt -l internal/plasma cmd/dotf && go build ./... && go vet ./... \
  && go test -race ./internal/plasma/... ./cmd/dotf/...
```

### WS5-T5 — Native `font` backend implementation and registration Depends on [WS5-T4]

**READ THESE BEFORE TASK**

- `/internal/backend/github_release.go` (`:25`, `:35-41`, `:151-230`, `:289`), `/internal/backend/download.go`
  (`:33-70`, `:110-125`, `:141-165`), `/internal/backend/ghapi.go` (`:65`)
- `/internal/backend/pkg.go` (`:104-135`, `:180-190`, `:214-218`), `/internal/backend/apt_repo.go` (`:14-25`, `:46-50`)
- `/internal/backend/register.go`, `/internal/backend/register_test.go`, `/internal/manifest/types.go`

**Files to Create**

- `/internal/backend/font.go`

**Files to Modify**

- `/internal/backend/register.go`, `/internal/backend/register_test.go`

**Instructions**

Create the native `font` backend ADR-13 specifies: delegate to the host package manager where a patched package
exists, otherwise extract the **checksum-verified** upstream release archive into the XDG fonts directory. It must
**not** be a `custom-script` entry — `customScriptBaseline` stays exactly 16 and this task adds none.

> **DESIGN DEVIATION (C32) — do NOT implement the design's `download.go` xz branch.** `ryanoasis/nerd-fonts`
> publishes both `<Family>.tar.xz` and `<Family>.zip` per release; `download.go` already classifies and extracts zip
> by magic bytes (`detectFormat` `:141-165`, `extractZip`), and Go's stdlib has no xz decoder — adding one means a
> new module dependency for zero capability gain. Use the `.zip`. **`internal/backend/download.go` is NOT modified.**
>
> **Do NOT edit `internal/backend/backend.go`.** Its count-free comment reword is absorbed into WS2-CORE so that file
> stays single-owner.

**CREATE `internal/backend/font.go`** (target ≤ 220 lines):

1. `type fontBackend struct { r exec.Runner; fontsDir func(platform.Info) (string, error) }` and
   `func newFontBackend(r exec.Runner) *fontBackend` seeding `fontsDir` with the default resolver. The injectable
   field mirrors `githubReleaseBackend.installDir` (`github_release.go:25`, `:35-41`) so tests stay hermetic.
2. `func defaultFontsDir(info platform.Info) (string, error)`: on `platform.OSMacOS` return `<home>/Library/Fonts`;
   otherwise `$XDG_DATA_HOME/fonts` when `$XDG_DATA_HOME` is absolute, else `<home>/.local/share/fonts`. Resolve home
   with `os.UserHomeDir()`, wrapping its error as `fmt.Errorf("resolve home directory: %w", err)` exactly as
   `github_release.go:37-39` does.
3. An install-route classifier used by **both** `Install` and `Installed` so they can never disagree:

   ```go
   type fontRoute int

   const (
   	fontRouteNone fontRoute = iota
   	fontRoutePacman
   	fontRouteBrewCask
   	fontRouteDownload
   )

   // fontRouteFor picks how THIS host installs pkg, and the name/archive that
   // route needs. Package-manager routes win over the download route because a
   // distro-patched build is updated by the package authority.
   func fontRouteFor(pkg manifest.Package, info platform.Info) (fontRoute, string)
   ```

   Order: (a) `info.PkgMgr == platform.PkgMgrPacman` and `pkg.Names["pacman"] != ""` → `fontRoutePacman` + that name;
   (b) `info.OS == platform.OSMacOS` and `pkg.Names["homebrew"] != ""` → `fontRouteBrewCask` + that name;
   (c) `pkg.Font != nil && pkg.Font.Archive != "" && pkg.Repo != ""` → `fontRouteDownload` + `pkg.Font.Archive`;
   (d) otherwise `fontRouteNone, ""`.
4. `func (b *fontBackend) applicable(info platform.Info) bool { return info.OS != platform.OSWindows }` — implements
   the existing `platformGated` optional interface (`backend.go:103-108`), mirroring `aptRepoBackend.applicable`. Do
   **not** implement `packageGated`/`applicableFor`: every manifest font entry carries a download route, so a
   package-level gate would add cross-workstream coupling for no benefit.
5. `func (b *fontBackend) Install(pkg manifest.Package, info platform.Info) error`:
   - `fontRouteNone` → `&UnsupportedPlatformError{Backend: manifest.BackendFont, OS: info.OS, PkgMgr: info.PkgMgr}`
     (type exists at `apt_repo.go:14`).
   - `fontRoutePacman` → `b.r.Run("sudo", "pacman", "-S", "--noconfirm", "--needed", name)` (identical argv to
     `installArgv`'s pacman arm, `pkg.go:131`).
   - `fontRouteBrewCask` → `b.r.Run("brew", "install", "--cask", name)`. `--cask` is deliberate and is why this does
     not reuse `installArgv`'s bare `brew install`.
   - `fontRouteDownload` → `archive := pkg.Font.Archive + ".zip"`;
     `downloadURL := githubReleaseURL(pkg.Repo, pkg.Tag, archive)` (`github_release.go:289`); resolve the expected
     checksum as `pkg.SHA256` when non-empty, else `checksumFor(pkg.Repo, pkg.Tag, pkg.ChecksumsAsset, archive)`
     when `pkg.ChecksumsAsset != ""` (`ghapi.go:65` — a package var, so injectable), else `""`. A `checksumFor`
     **error is FAIL-CLOSED**: return it; never fall through to an unverified download.

     **BLOCKER FIX (critique) — reject an empty want before downloading.** `VerifySHA256` (`download.go:117-120`)
     returns `nil` when `wantSHA256 == ""`, and its own doc says callers that must fail closed have to reject an
     empty want **before** calling it. So: if the resolved `wantSHA` is empty, return a typed error naming the
     package and the missing checksum source — do **not** call `fetchVerifyExtract`. (WS5-T4's `validateFont` rule
     makes this unreachable from a valid manifest; this is the defence-in-depth half, mirroring
     `github_release.go:89-91`'s `pinnedNoChecksumErr`.)

     Then resolve `dir, err := b.fontsDir(info)`, compute `dest := filepath.Join(dir, pkg.Font.Archive)`, and call
     `fetchVerifyExtract(downloadURL, wantSHA, dest, archive)` (`download.go:33`).

     **MINOR FIX (critique) — do NOT `os.MkdirAll(dest, …)` before extracting.** `extractZip` already MkdirAlls each
     entry's parent, and pre-creating the directory means a failed download (checksum mismatch, 404, network error)
     leaves an **empty directory** behind that the `Installed` probe then reads as "installed forever".

     Finally, best-effort refresh the font cache: if `b.r.LookPath("fc-cache")` succeeds,
     `_ = b.r.Run("fc-cache", "-f", dir)` — a missing or failing `fc-cache` is never an install failure (the fonts
     are on disk; the cache rebuilds at next login).
6. `func (b *fontBackend) Installed(pkg manifest.Package, info platform.Info) (bool, error)` — route through the
   **same** `fontRouteFor`:
   - `fontRoutePacman` → `b.r.Output("pacman", "-Q", name)`; error ⇒ `(false, nil)` (same swallow-error convention as
     `pkgBackend.Installed`, `pkg.go:106-113`).
   - `fontRouteBrewCask` → `b.r.Output("brew", "list", "--cask", name)`; error ⇒ `(false, nil)`.
   - `fontRouteDownload` → when `b.r.LookPath("fc-list")` succeeds, run `b.r.Output("fc-list", ":", "family")` and
     report `strings.Contains(string(out), pkg.Font.Family)` on success. Otherwise (or on Output error) fall back to
     reading the directory: `entries, err := os.ReadDir(filepath.Join(dir, pkg.Font.Archive))` and report
     `err == nil && len(entries) > 0` — **not** a bare `os.Stat` on the directory (minor fix above: an empty
     directory left by a failed install must not read as installed).
   - `fontRouteNone` → `(false, nil)`.

   Never return a non-nil error from a probe unless the fonts dir cannot be resolved at all.
7. End the file with the interface assertions, matching `pkg.go`'s shape:

   ```go
   var (
   	_ Backend       = (*fontBackend)(nil)
   	_ platformGated = (*fontBackend)(nil)
   )
   ```

   Do **not** implement `Updater`, `Remover`, or the `lockable` `resolve` method: `EffectiveClass` for font is
   `managed`, so `lockfile.Pinnable` is false and `dotf lock` never asks.

**EDIT — `register.go`:** add `manifest.BackendFont: newFontBackend(r),` to `Default`'s map (`:16-25`), after the
`BackendGitClone` entry. Reword the `:10` doc comment count-**free** ("every canonical backend") — WS7-REPO adds
another entry to this same map, so no numeral survives.

**EDIT — `register_test.go`:** add `manifest.BackendFont,` to the `validBackends` slice (`:23-32`) of
`TestDefault_CoversAllValidBackends`. This is completing an existing exhaustive list, not a new test.

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && gofmt -l internal/backend && go build ./... && go vet ./internal/backend/... \
  && go test -race ./internal/backend/... && golangci-lint run ./internal/backend/...
```

### WS6-T2 — Extract `pkgInstallArgv(info, name)`; fix apt-get-on-Fedora Depends on [WS6-T1]

**READ THESE BEFORE TASK**

- `/internal/sysconfig/linux.go` (`sshInstallArgv`), `/internal/sysconfig/linux_firewall.go` (`ufwInstallArgv`)
- `/internal/sysconfig/linux_test.go` (`:339-365`, `:553-590`), `/internal/sysconfig/register_test.go` (`:57-80`)

**Files to Modify**

- `/internal/sysconfig/linux.go`, `/internal/sysconfig/linux_firewall.go`

**Instructions**

`ufwInstallArgv` returns `{"sudo","apt-get","install","-y","ufw"}` for **every** non-Arch host, including OSRedHat
and OSSUSE — `apt-get` does not exist on Fedora. `sshInstallArgv` already spells out the correct four-way family
switch. Extract that switch once and make both a caller.

In `linux.go`, add a package-level helper immediately **above** `sshInstallArgv`:

```go
// pkgInstallArgv builds the non-interactive install argv for name using the
// native package manager of info's distro family. It is the single source of
// truth for every sysconfig-owned package install (ufw, firewalld, openssh),
// so a new family is added in exactly one place.
func pkgInstallArgv(info platform.Info, name string) []string {
	switch info.OS {
	case platform.OSArch:
		return []string{"sudo", "pacman", "-S", "--noconfirm", "--needed", name}
	case platform.OSRedHat:
		return []string{"sudo", "dnf", "install", "-y", name}
	case platform.OSSUSE:
		return []string{"sudo", "zypper", "--non-interactive", "install", name}
	default:
		return []string{"sudo", "apt-get", "install", "-y", name}
	}
}
```

Replace `sshInstallArgv`'s whole body with:

```go
// sshInstallArgv builds the OpenSSH server install argv for the distro family.
// Arch's package is `openssh`; every other family ships it as `openssh-server`.
func sshInstallArgv(info platform.Info) []string {
	name := "openssh-server"
	if info.OS == platform.OSArch {
		name = "openssh"
	}
	return pkgInstallArgv(info, name)
}
```

Byte-identical output for all four families — `TestLinux_SSHServer_InstallArgvPerFamily` (`linux_test.go:553-590`)
must keep passing **unmodified**.

In `linux_firewall.go`, replace `ufwInstallArgv` with:

```go
// ufwInstallArgv builds the ufw install argv for the distro family. RedHat and
// SUSE reach this only under an explicit `--firewall ufw`; under `auto` they
// route to firewalld (see planFirewall).
func ufwInstallArgv(info platform.Info) []string {
	return pkgInstallArgv(info, "ufw")
}
```

**No test file changes** — this is still phase one of the R10 gate. Verified non-breaking: the only two places ufw
install argv is pinned are `TestLinux_Firewall_UfwSelection` (`linux_test.go:344-345`, rows OSArch + OSDebian only)
and `TestDefault_LinuxFamilySharesInstance` (`register_test.go:61-65`, OSArch/OSDebian/OSWSL only).

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && gofmt -l internal/sysconfig && go build ./... && go vet ./... \
  && go test -race ./internal/sysconfig/... \
  && test -z "$(git diff --name-only -- internal/sysconfig/linux_test.go internal/sysconfig/register_test.go)" \
     || { echo 'FAIL: a sysconfig test file was modified in phase one'; exit 1; }
```

### WS6-T9 — R3b: demote Homebrew below the native manager on **arm64** Linux Depends on [WS1-T2]

**READ THESE BEFORE TASK**

- `/internal/platform/platform.go` (`:19-28`, `:175-194`), `/internal/platform/platform_test.go` (`:224-270`)
- `/internal/brew/brew.go`, `/cmd/dotf/apply.go` (`:169-175`), `/common/platform.sh` (`:134-150`)

**Files to Modify**

- `/internal/platform/platform.go`, `/internal/platform/platform_test.go`

**Instructions**

`detectPkgMgr` (`:175-194`) probes `{"brew", PkgMgrHomebrew}` **first** (`:180`) with no OS or arch gate, so an
Apple-Silicon Fedora box with Linuxbrew silently routes every package through `brew install` and never consults the
manifest's `dnf` names — on the exact architecture where Homebrew publishes **no bottles at all**.

> **MAJOR FIX (critique) — scope the demotion to arm64 Linux, not all Linux.** The first-pass spec demoted brew on
> **every** Linux-family host, which silently guts the documented `--homebrew=true` contract on Debian, Arch,
> openSUSE and WSL. `cmd/dotf/apply.go:169-172` states the purpose in so many words — bootstrap Homebrew before
> packages "so the pkg backend's `brew install` has a working brew … and the platform re-detected so
> PkgMgrHomebrew is selected for the packages phase" — and `applyBrew` (`applyphases.go:45-59`) exists only to do
> that re-detect. A blanket demotion makes `dotf apply --homebrew=true` on Debian install Homebrew and then never
> use it. Gating on arm64 fixes the Fedora Asahi bypass (where brew genuinely cannot work) and leaves x86_64
> `--homebrew=true` working as documented.

**(1)** Change `Detect` (`:19-28`) to compute the OS and arch once and pass them down. Replace:

```go
	return Info{
		OS:        detectOS(r, defaultOSReleasePath, defaultProcVersionPath),
		Arch:      detectArch(r),
		PkgMgr:    detectPkgMgr(r),
```

with:

```go
	os := detectOS(r, defaultOSReleasePath, defaultProcVersionPath)
	arch := detectArch(r)
	return Info{
		OS:        os,
		Arch:      arch,
		PkgMgr:    detectPkgMgr(r, os, arch),
```

keeping `Container`, `Distro` and `Codename` exactly as they are.

**(2)** Replace `detectPkgMgr`'s doc comment and signature/body with:

```go
// detectPkgMgr determines the canonical primary package manager. On macOS
// Homebrew IS the package manager, so it is probed first. On x86_64 Linux it is
// still probed first, because `--homebrew=true` deliberately bootstraps
// linuxbrew and expects the packages phase to use it (see cmd/dotf/apply.go's
// applyBrew). On aarch64 Linux the NATIVE manager wins instead: Homebrew
// publishes no aarch64-Linux bottles at all, so an incidental linuxbrew install
// must not silently reroute every package away from the manifest's dnf names.
// Linuxbrew is still selected there when no native manager is present.
//
// NOTE: this deliberately DIVERGES from common/platform.sh:detect_package_manager,
// which still prioritizes brew unconditionally. Reconcile the shell side
// separately; do not "fix" this back.
//
// Note: the shell implementation checks `command -v brew`, which also resolves
// shell functions and aliases, not just PATH executables. r's LookPath (like
// Go's exec.LookPath) only resolves executables on PATH, so the two can diverge
// in the edge case of a brew shim defined as a shell function/alias with no
// corresponding PATH binary.
func detectPkgMgr(r exec.Runner, os OS, arch Arch) PkgMgr {
	type candidate struct {
		bin string
		mgr PkgMgr
	}
	brew := candidate{"brew", PkgMgrHomebrew}
	native := []candidate{
		{"apt", PkgMgrAPT},
		{"dnf", PkgMgrDNF},
		{"yum", PkgMgrYUM},
		{"pacman", PkgMgrPacman},
		{"zypper", PkgMgrZypper},
	}

	candidates := append([]candidate{brew}, native...)
	if IsLinuxFamily(os) && arch == ArchARM64 {
		candidates = append(append([]candidate{}, native...), brew)
	}

	for _, c := range candidates {
		if _, err := r.LookPath(c.bin); err == nil {
			return c.mgr
		}
	}
	return PkgMgrUnknown
}
```

**(3)** `internal/platform/platform_test.go`'s `TestDetectPkgMgr` (`:224+`) calls `detectPkgMgr(r)` at `:259` and will
not compile. Add `os OS` and `arch Arch` fields to the table struct, pass them at the call site
(`detectPkgMgr(r, tt.os, tt.arch)`), and set them per row: give the existing `"pacman only"`, `"dnf only"` and
`"none found -> unknown"` rows an explicit Linux-family OS and `ArchX86_64`. Rename the existing
`"brew and apt both present -> homebrew priority"` row to `"macos: brew wins over any native manager"`
(`os: OSMacOS`, `want: PkgMgrHomebrew`) and **add** three rows (rows in an existing table, not new tests):

```go
		{
			name: "linux x86_64: brew still wins so --homebrew=true works as documented",
			os:   OSDebian, arch: ArchX86_64,
			setup: func(r *exec.Fake) {
				r.AddPath("brew", "/home/linuxbrew/.linuxbrew/bin/brew")
				r.AddPath("apt", "/usr/bin/apt")
			},
			want: PkgMgrHomebrew,
		},
		{
			name: "linux arm64: native dnf wins over an incidental linuxbrew (no aarch64 bottles)",
			os:   OSRedHat, arch: ArchARM64,
			setup: func(r *exec.Fake) {
				r.AddPath("brew", "/home/linuxbrew/.linuxbrew/bin/brew")
				r.AddPath("dnf", "/usr/bin/dnf")
			},
			want: PkgMgrDNF,
		},
		{
			name: "linux arm64: linuxbrew still selected when no native manager exists",
			os:   OSLinux, arch: ArchARM64,
			setup: func(r *exec.Fake) {
				r.AddPath("brew", "/home/linuxbrew/.linuxbrew/bin/brew")
			},
			want: PkgMgrHomebrew,
		},
```

Cross-workstream note: WS1-T2 already edited `osFamilyFromID` and added `TestDetectOS` rows in this same file pair —
different functions and tables, so rebase and re-run the whole package. Do **not** touch `common/platform.sh` here;
the divergence is recorded in the doc comment and in ROADMAP.md's near-term list (WS8-T5).

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && gofmt -l internal/platform && go build ./... && go vet ./... \
  && go test -race -v ./internal/platform/... 2>&1 | grep -E '^(---|ok|FAIL)' \
  && go test -race ./... && golangci-lint run ./internal/platform/...
```

---

## Batch 5 — Fonts, locale, brew decline, single-scan os-release, and the device checklist

### WS1-T5 — `docs/fedora-first-run.md`: the WS0 capture block plus the ordered M1 checklist Depends on [WS3-SAFETY, WS3-FACETS, WS6-T9, WS1-T1, WS1-T3]

> **MAJOR FIX (critique) — this task moved from batch 1 to batch 5.** It is the physical acceptance gate: the one
> document a human follows while running destructive operations on a populated daily driver. Written in batch 1 it
> would ship describing three bugs fixed later in the **same PR** — the seven un-backed-up directories
> (WS3-SAFETY), the brew-shadows-dnf warning (WS6-T9), and `--linux-desktop` as the primary flag name
> (WS3-FACETS). Batch 5 is the first batch where all three are settled, and this file is written **once, against
> the final behaviour**.

**READ THESE BEFORE TASK**

- `/monitor-auto-switch/testing.md` (the repo's only precedent for a hand-written hardware verification doc — read
  it for tone and shape)
- `/bootstrap.sh`, `/cmd/dotf/main.go` (`:109`), `/cmd/dotf/applyphases.go` (`:23-26`, `:294-305`)
- `/internal/symlink/table.go` (post-WS3-FACETS), `/internal/symlink/symlink.go` (`:205-210`)
- `/internal/snapper/snapper.go`, `/internal/platform/platform.go` (post-WS6-T9), `/.markdownlint-cli2.jsonc`

**Files to Create**

- `/docs/fedora-first-run.md`

**Instructions**

State up front that this checklist is an **ACCEPTANCE GATE**: it is walked end to end on the physical M1 before the
release tag is cut, not written up afterwards.

1. **Title + scope.** `# Fedora Asahi Remix first-run checklist`. One paragraph: target device is an Apple M1
   MacBook Pro running Fedora Asahi Remix (KDE Plasma 6 / Wayland, dnf5, firewalld, SELinux enforcing, btrfs with a
   root subvolume literally named `root`, `uname -m` = `aarch64`). The device is a populated daily driver, so every
   step is ordered by blast radius: read-only first, one mutating domain at a time.

2. **Step 0 — device ground truth capture (read-only).** Runs BEFORE any code depending on a device fact is
   finalised; the operator pastes raw output back into the PR. One fenced `bash` block, exactly these lines in this
   order (no `$` prompt prefixes — MD014 is on):

   ```bash
   cat /etc/os-release
   uname -m -r
   dnf --version
   rpm -qf "$(command -v chsh)" 2>&1
   command -v qdbus qdbus6 qdbus-qt6 busctl gdbus
   command -v gh python3
   getenforce
   sudo firewall-cmd --get-default-zone
   systemctl is-enabled sshd
   findmnt -no FSTYPE /
   command -v snapper
   printenv XDG_CURRENT_DESKTOP KDE_FULL_SESSION XDG_SESSION_TYPE
   dnf list --available lazygit yazi lutris docker-ce moby-engine 2>&1 | tail -20
   ls -d ~/.config/{nvim,wezterm,ghostty,mise,alacritty,direnv} ~/.bin 2>&1
   ```

   Follow with a short `Capture | Settles` table: the os-release block is the verbatim fixture behind the Fedora/Asahi
   `TestDetectOS` rows; `rpm -qf "$(command -v chsh)"` settles whether `chsh` ships on this release (the
   `LookPath`/`usermod -s` question); `command -v qdbus*`/`busctl` settles which KWin reconfigure binary exists;
   `dnf list --available` settles the mapping table's release header; the final `ls` records which managed
   directories already exist.

3. **DO NOT USE (read before running anything).** Each item with its verified reason:
   - **`--hyprland`** (and its deprecated alias `--linux-desktop`) on `dotf link` / `dotf apply`: it deploys the
     Hyprland/wlroots desktop set (`internal/symlink/table.go`, facet `FacetHyprland`) onto a KDE host. The
     CachyOS-branded `arch-update.desktop` is no longer part of that set (it moved to `FacetArchDesktop`), but the
     rest of it — hypr, waybar, swayosd, mako, wlogout, fuzzel, satty — is categorically wrong on Plasma 6. Use
     `dotf plasma` instead.
   - **`--snapper`** on `dotf apply`: `cmd/dotf/applyphases.go:23-26` states verbatim that snapshot failures are
     "NEVER recorded in the failure collector", and `internal/snapper/snapper.go` gates only on Linux-family +
     `LookPath("snapper")` with no btrfs and no snapper-config check, emitting `sudo snapper create` with no
     `-c <config>`. Fedora ships neither snapper nor a config, so the run exits **0** having protected nothing.
   - **`dotf plasma apply --reload`** on the first pass: run the printed reload commands by hand instead; the visible
     result on a live Plasma 6 session cannot be judged headlessly.

4. **Backup behaviour — what `dotf link --backup` now covers.**

   > **Rewritten for the post-ADR-11 world.** State the _current_ behaviour, not the bug that was fixed in the same
   > PR: as of ADR-11 (critical test #4), `dotf link --backup` **does** back up directory destinations before
   > replacing them. `internal/symlink/symlink.go:205-210` still removes a real file or directory at a managed
   > destination recursively, but `BackupEligible` now covers **30** of the 41 entries — the 14 single-file
   > entries, **all 15** directory entries, and the tmux user unit. The plan orders `ActionBackup` before
   > `ActionRemove`, and the backup lands under `$HOME/.config/dotfiles_backup_<epoch>` preserving the
   > `$HOME`-relative path.
   >
   > **Amendment (PR #36 review):** this originally said 21 entries — the 14 single files plus only the seven
   > `FacetAll` directories. That left the seven `FacetHyprland` directories, the `FacetMacOS` `aerospace`
   > directory and the newly-widened tmux unit recursively deletable under `--backup`, which is precisely what
   > `LinuxDesktopGuardError` points the operator at `--backup` to prevent. Every `IsDir` entry is eligible now,
   > regardless of facet. Since the complement is the shorter list, the operator-facing enumeration in
   > `docs/fedora-first-run.md` is written as the **11 uncovered single-file destinations** instead.

   Then: **always pass `--backup`**, and read every `link.remove=` line in the dry run against that list. Note that
   `--backup` is what makes the tarball below optional rather than mandatory. Keep the manual archive as a
   belt-and-braces step for anyone who wants a single restorable artifact:

   ```bash
   tar czf ~/dotfiles-preflight-$(date +%Y%m%d%H%M).tar.gz \
     -C "$HOME" .config/nvim .config/wezterm .config/ghostty \
     .config/mise .config/alacritty .bin .config/direnv
   ```

   Say explicitly that this tarball or the `--backup` directory — **never** `--snapper` — is the rollback.

5. **Ordered checklist.** Numbered steps, each with the exact command and the exact expected observation:
   1. **Download the binary with `GH_TOKEN` deliberately unset.** `env -u GH_TOKEN -u GITHUB_TOKEN ./bootstrap.sh …`
      obtains `dotf_linux_arm64.tar.gz` with `gh` absent and no token. Note the shim consumes only
      `--repo`/`--install-go` and forwards everything else to `dotf`, injecting `apply` when the first argument is a
      flag — so `./bootstrap.sh --dry-run --headless` runs `dotf apply --dry-run --headless`. Record whether the
      download succeeded; this is the one claim no CI job can make. Documented backstop on failure:
      `./bootstrap.sh --install-go` from a clone, which installs Go under `~/.local/go` with no sudo and builds from
      source.
   2. **Detection.** `dotf platform` prints `os=<os> arch=<arch> pkg=<pkg> container=<c> distro=<distro>`
      (`cmd/dotf/main.go:109`). Expect `os=redhat arch=arm64 pkg=dnf` and record `distro=` verbatim.
      **Post-WS6-T9 note:** on aarch64 Linux the native manager now wins over an incidental Linuxbrew, so `pkg=dnf`
      is expected even with brew on PATH. If `pkg=` is anything other than `dnf`, stop and report it — that is a
      detection bug, not an expected condition.
   3. **Doctor.** `dotf doctor` — read-only. It now emits an informational `selinux mode=` row (WS6-T10); record
      `getenforce` before and after every mutation and treat a flip to Permissive as stop-the-line.
   4. **`dotf link --dry-run`** — read every `link.remove=` destination against section 4 before proceeding. Do not
      pass `--hyprland`. Confirm `.config/systemd/user/tmux.service` **is** listed (ADR-12 retagged it to
      `FacetLinux`; it was absent before this PR).
   5. **`dotf apply --dry-run --headless`** — must exit 0 and must **not** list `docker-ce` under `plan.apt-repo=`.
      Any package this host cannot install appears on the single `plan.not-applicable=` line instead.
   6. **First mutation, one domain at a time** — sysconfig domains individually, then packages, then symlinks
      (`dotf link --backup`); re-run `getenforce` after each.

6. **Residual risks only the device can settle.** A table: the verbatim os-release `ID`/`VARIANT_ID`; whether `chsh`
   is present and from which package; the private/prerelease download itself; SELinux behaviour under real
   provisioning; 16 KiB page-size interaction with any C-linked third-party binary; the live Plasma 6 session;
   `hostnamectl set-hostname`; btrfs/snapper behaviour; arm64 + systemd together (unreachable under `qemu-user` —
   `dbus-broker` dies `226/NAMESPACE`); the real NetworkManager/firewalld zone on a roaming laptop; **and** the
   Fedora `/etc/bashrc` severance — Fedora's bash is not built with `-DSYS_BASHRC`, so `/etc/bashrc` (and therefore
   `/etc/profile.d/*.sh`) is pulled in only by the skel `~/.bashrc` that `dotf link` replaces, for exactly the
   interactive non-login shell a Konsole tab starts. WS-SHELL deliberately did **not** change that; record whether
   anything in `/etc/profile.d` turns out to matter on this host.

**FORMATTING CONSTRAINTS** (`.markdownlint-cli2.jsonc`): no `$`-prefixed commands in fenced blocks (MD014); fenced
blocks only, surrounded by blank lines (MD031/MD046); lists surrounded by blank lines (MD032); 2-space list indent
(MD007); `_underscores_` for italics and `**asterisks**` for bold (MD049/MD050); no bare URLs (MD034); no hard tabs
(MD010); exactly one trailing newline (MD047). MD013 is disabled, so long table rows are fine.

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && npx --no-install prettier --write docs/fedora-first-run.md \
  && npx --no-install prettier --check docs/fedora-first-run.md \
  && npx --no-install markdownlint-cli2 docs/fedora-first-run.md \
  && grep -q 'DO NOT USE' docs/fedora-first-run.md \
  && grep -q 'plan.not-applicable=' docs/fedora-first-run.md \
  && grep -q 'config/direnv' docs/fedora-first-run.md \
  && grep -q 'GH_TOKEN' docs/fedora-first-run.md \
  && grep -q -- '--hyprland' docs/fedora-first-run.md \
  && grep -q 'etc/bashrc' docs/fedora-first-run.md \
  && echo WS1-T5-OK
```

### WS5-T6 — Manifest: three Nerd Font packages plus the `fonts` / `desktop-fonts` intents Depends on [WS5-T4, WS5-T5]

**READ THESE BEFORE TASK**

- `/manifest/packages.toml` (re-read immediately before editing — WS2-CORE already changed it)
- `/cmd/dotf/applyopts.go` (`:74-140`), `/internal/manifest/ratchet_test.go`
- `/alacritty/alacritty.toml` (`:50-56`), `/hypr/looknfeel.lua` (`:30`), `/hypr/hyprlock.conf` (`:77`),
  `/bin/install-arch-pkgs.sh` (`:474-488`)

**Files to Modify**

- `/manifest/packages.toml`, `/cmd/dotf/applyopts.go`

**Instructions**

Ship the three patched Nerd Font families the repo's configs pin by exact string. Today they are installed only by
`bin/install-arch-pkgs.sh:474-488` (`install_fonts`), a function referenced nowhere in the manifest, so Fedora and
macOS get tofu.

Families and their pins (verified in-repo): `JetBrainsMono Nerd Font` — `alacritty/alacritty.toml:50-56`,
`wezterm/appearance.lua:65`, `zed/settings.json:162/164/237`, `hypr/show-keybindings.lua:91`,
`iterm2/Default.json:219`; used everywhere ⇒ the **default** intent. `CaskaydiaMono Nerd Font` —
`hypr/looknfeel.lua:30`, `hypr/hyprlock.conf:77`; Hyprland only ⇒ the **opt-in** intent. `CaskaydiaCove Nerd Font` —
the third family `bin/install-arch-pkgs.sh:488` installs (`ttf-cascadia-code-nerd`); Hyprland set ⇒ opt-in.

**EDIT 1 — intents.** Place `[intents.fonts]` immediately after the `[intents.dev]` block and
`[intents.desktop-fonts]` immediately after `[intents.gaming]`:

```toml
[intents.fonts]
description = "Patched Nerd Font families the terminal and editor configs pin"
default = true
packages = ["jetbrains-mono-nerd-font"]

[intents.desktop-fonts]
description = "Additional Nerd Font families used by the Hyprland desktop set"
default = false
packages = ["caskaydia-mono-nerd-font", "caskaydia-cove-nerd-font"]
```

**EDIT 2 — packages.** Insert immediately **before** the `# --- aur:` header:

```toml
# --- font: patched Nerd Font families ----------------------------------
# 22 config sites pin these family names by exact string. Arch ships patched
# packages (ttf-*-nerd) and Homebrew ships casks; every other host — Fedora
# included, whose only COPR is symbols-only and never creates these family
# names — extracts the checksum-verified upstream archive into the XDG fonts
# directory. See docs/reference/manifest-backends.md.
[[package]]
name = "jetbrains-mono-nerd-font"
backend = "font"
names = { pacman = "ttf-jetbrains-mono-nerd", homebrew = "font-jetbrains-mono-nerd-font" }
repo = "ryanoasis/nerd-fonts"
tag = "v3.4.0"
checksums_asset = "SHA-256.txt"
font = { family = "JetBrainsMono Nerd Font", archive = "JetBrainsMono" }

[[package]]
name = "caskaydia-mono-nerd-font"
backend = "font"
names = { pacman = "ttf-cascadia-mono-nerd", homebrew = "font-caskaydia-mono-nerd-font" }
repo = "ryanoasis/nerd-fonts"
tag = "v3.4.0"
checksums_asset = "SHA-256.txt"
font = { family = "CaskaydiaMono Nerd Font", archive = "CascadiaMono" }

[[package]]
name = "caskaydia-cove-nerd-font"
backend = "font"
names = { pacman = "ttf-cascadia-code-nerd", homebrew = "font-caskaydia-cove-nerd-font" }
repo = "ryanoasis/nerd-fonts"
tag = "v3.4.0"
checksums_asset = "SHA-256.txt"
font = { family = "CaskaydiaCove Nerd Font", archive = "CascadiaCode" }
```

**BEFORE COMMITTING, verify the pins against the live release** — `tag = "v3.4.0"` and `checksums_asset` are pinned
from memory:

```bash
gh release view v3.4.0 --repo ryanoasis/nerd-fonts --json assets --jq '.assets[].name' \
  | grep -E '^(JetBrainsMono|CascadiaMono|CascadiaCode)\.zip$|SHA-256'
```

> **BLOCKER FIX (critique) — the "drop `checksums_asset`" escape is DELETED.** The first-pass spec said that if the
> sidecar does not exist, drop the `checksums_asset` line from all three entries. That authorises shipping an
> **unverified** download on a `default = true` intent. If the sidecar is named differently, **correct**
> `checksums_asset`. If no combined sidecar exists at all, pin the archive's own `sha256` computed from the real
> asset — `curl -fsSL <url> | sha256sum`. Do **not** ship a font entry with no checksum source: WS5-T4's
> `validateFont` rule rejects it, and WS5-T5's `fontRouteDownload` fails closed on it.
>
> **Amendment (PR #36 review):** this originally said to pin per-target `checksums`. That instruction now steers
> straight into a hard validation failure — `validateFont` accepts only `sha256` or `checksums_asset`, because
> `backend.fontChecksum` reads only those two and a `<os>/<arch>` key is meaningless for a single
> arch-independent zip. Use `sha256` for the single-asset case.

**EDIT 3 — header comment** (`~:14-17`). It names "the eight Backend values (pkg, github-release, aur, toolchain,
apt-repo, script-url, custom-script, git-clone)". Reword **count-free** and append `font`: "… the frozen source of
truth for field names and the Backend values (pkg, github-release, aur, toolchain, apt-repo, script-url,
custom-script, git-clone, font)." WS7-MANIFEST appends `repo` to this same list — leave the numeral out so the two
edits do not fight.

**EDIT 4 — `cmd/dotf/applyopts.go`**, `optInIntents` map (`:79-87`): add `"desktop-fonts": true,` and note in the
map's doc (`:74-78`) that it is the Hyprland-only font set. Do **not** add `fonts` — it is intentionally in the
headless default set (`resolveIntents` at `:123-140` selects every intent **not** in `optInIntents`). This is the
**second** of two serialized writers of this file (WS3-FACETS was first) — re-read it before editing.

**CONSTRAINTS AND EXPECTED CHURN**

- `customScriptBaseline` stays exactly **16**; this task adds zero custom-script entries.
- Adding a `default = true` intent changes the headless default intent set. Run the full `./cmd/dotf/...` suite; if a
  test pins the embedded manifest's resolved default intents, update that expectation — that churn is the intended
  signal.
- `cmd/dotf/applyopts_test.go`'s `testIntentsManifest` (`:187-200`) is synthetic and needs no change.
- **Open question for the reviewer** (recorded, not silently decided): with `fonts` default, a bare `dotf apply` on a
  non-Arch, non-macOS host downloads a Nerd Font archive over the network on first boot. If that is unwanted, flip
  `fonts` to `default = false` — a one-line change here, nowhere else.

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && test "$(grep -c '^backend = \"custom-script\"' manifest/packages.toml)" = 16 \
  && go build ./... && go test -race ./internal/manifest/... ./cmd/dotf/... \
  && go run ./cmd/dotf status --json >/dev/null
```

### WS6-T3 — Locale: OS-first strategy, the RedHat glibc-langpack arm, `langpackFor`, doc corrections Depends on [WS6-T2]

**READ THESE BEFORE TASK**

- `/internal/sysconfig/linux_locale.go`, `/internal/sysconfig/locale_resolve.go` (all 80 lines)
- `/internal/sysconfig/linux_test.go` (`:63-250`), `/common/exports.sh` (post-WS-SHELL)

**Files to Modify**

- `/internal/sysconfig/linux_locale.go`, `/internal/sysconfig/locale_resolve.go`, `/internal/sysconfig/linux_test.go`

**Instructions**

On Fedora a requested locale that `locale -a` does not list silently degrades to `C`: `planLocale` gates purely on
`LookPath("locale-gen")`, Fedora ships no `locale-gen` and no `/etc/locale.gen`, so it falls to `resolveLocale`,
whose last resort is `"C"` (`locale_resolve.go:52`). Fedora's mechanism is `dnf install glibc-langpack-<lang>`.

**(1) `locale_resolve.go` — add `langpackFor`** at end of file:

```go
// langpackFor returns the Fedora/RHEL glibc langpack package name for locale
// ("de_DE.UTF-8" -> "glibc-langpack-de"), or "" when locale has no language
// prefix or is a built-in (C / C.UTF-8 / POSIX) that needs no package.
func langpackFor(locale string) string {
	if isBuiltinLocale(locale) {
		return ""
	}
	n := normalizeLocale(locale)
	lang := n
	if i := strings.IndexAny(lang, "_.@"); i >= 0 {
		lang = lang[:i]
	}
	if lang == "" {
		return ""
	}
	for _, ch := range lang {
		if ch < 'a' || ch > 'z' {
			return ""
		}
	}
	return "glibc-langpack-" + lang
}
```

**(2) `locale_resolve.go` — correct two stale citations.** Replace `normalizeLocale`'s doc comment (`:5-8`) so it
states the parity **truthfully**. WS-SHELL (batch 2) fixed `common/exports.sh:locale_exists` to collapse the codeset,
so the parity now **does** exist:

```go
// normalizeLocale canonicalizes a locale string for comparison: it lowercases,
// strips all whitespace, and treats "utf-8" == "utf8", so
// "en_us.utf-8" == "en_US.UTF-8". This mirrors the case/hyphen-insensitive
// matching common/exports.sh:locale_exists performs (see WS-SHELL / ADR-14,
// which fixed the shell side to collapse the codeset separator the same way).
```

In `resolveLocale`'s doc comment, replace `from common/exports.sh:231-312 (pick_dotfiles_locale)` with
`mirroring the fallback precedence of common/exports.sh's pick_dotfiles_locale`.

**(3) `linux_locale.go` — OS-first dispatch.** Replace this block inside `planLocale`:

```go
	if !localeAvailable(avail, cfg.Locale) {
		_, lookErr := c.r.LookPath("locale-gen")
		switch {
		case lookErr == nil && !isBuiltinLocale(cfg.Locale):
			genSteps = c.localeGenSteps(cfg.Locale, info)
		default:
			target = resolveLocale(cfg.Locale, avail)
		}
	}
```

with:

```go
	if !localeAvailable(avail, cfg.Locale) && !isBuiltinLocale(cfg.Locale) {
		genSteps = c.localeGenSteps(cfg.Locale, info)
	}
	if len(genSteps) == 0 && !localeAvailable(avail, cfg.Locale) {
		target = resolveLocale(cfg.Locale, avail)
	}
```

and move the `LookPath` decision **into** `localeGenSteps`, so the strategy is picked by family first:

```go
// localeGenSteps builds the generate Steps for target, choosing the strategy by
// distro family FIRST and tool presence second:
//   - RedHat: no locale-gen exists; glibc ships pre-generated locales in
//     `glibc-langpack-<lang>` RPMs, so install that.
//   - Arch: uncomment the entry in /etc/locale.gen (a file step) then run
//     bare `locale-gen`.
//   - everything else: `locale-gen <target>` directly.
//
// Returns nil when no strategy applies (no locale-gen on PATH, or no derivable
// langpack); the caller then falls back through resolveLocale.
func (c *linuxConfigurer) localeGenSteps(target string, info platform.Info) []Step {
	if info.OS == platform.OSRedHat {
		pkg := langpackFor(target)
		if pkg == "" {
			return nil
		}
		return []Step{{
			Domain: "locale",
			Desc:   "install " + pkg,
			Argv:   pkgInstallArgv(info, pkg),
		}}
	}

	if _, err := c.r.LookPath("locale-gen"); err != nil {
		return nil
	}

	if info.OS == platform.OSArch {
		var steps []Step
		content, _ := c.fs.ReadFile("/etc/locale.gen")
		if newContent, changed := uncommentLocaleGen(content, target); changed {
			steps = append(steps, Step{
				Domain: "locale",
				Desc:   "uncomment " + target + " in /etc/locale.gen",
				File:   &FileInstall{Dest: "/etc/locale.gen", Content: newContent, Backup: true},
			})
		}
		steps = append(steps, Step{
			Domain: "locale",
			Desc:   "locale-gen",
			Argv:   []string{"sudo", "locale-gen"},
		})
		return steps
	}

	return []Step{{
		Domain: "locale",
		Desc:   "locale-gen " + target,
		Argv:   []string{"sudo", "locale-gen", target},
	}}
}
```

Also update `planLocale`'s own doc comment to state the OS-first dispatch and the RedHat langpack arm.

**BEHAVIOUR-PRESERVATION CHECK** — these must pass **unmodified**, confirming Debian/Arch are untouched:
`TestLinux_Locale_GenBeforeSet` (`:155`), `TestLinux_Locale_ArchUncommentThenGen` (`:181`),
`TestLinux_Locale_FallbackWhenNoLocaleGen` (`:206`), `TestLinux_Locale_NormalizedAvailableNoGen` (`:229`), and
`TestLinux_Plan_AllDomains` (`:63`, step 3 `sudo locale-gen en_US.UTF-8`).

**(4) ONE new test** in `linux_test.go`, immediately after `TestLinux_Locale_NormalizedAvailableNoGen` (~`:250`).
This is the design-mandated RedHat locale row, not speculative — the langpack arm has no other coverage and its
absence **is** the silent LANG=C downgrade being fixed:

```go
// TestLinux_Locale_RedHatLangpack proves Fedora installs the glibc langpack
// rather than silently downgrading LANG to C: it has no locale-gen and no
// /etc/locale.gen, so the pre-fix code fell through resolveLocale to "C".
func TestLinux_Locale_RedHatLangpack(t *testing.T) {
	fake := exec.NewFake()
	fake.AddCommand("locale", []string{"-a"}, []byte("C\nC.UTF-8\nPOSIX\n"), nil)
	// locale-gen intentionally NOT on PATH (Fedora ships none).
	fake.AddCommand("localectl", []string{"status"}, []byte("System Locale: LANG=C\n"), nil)

	cfg := Config{Locale: "de_DE.UTF-8", User: "yandy"}
	plan, err := newLinuxTest(fake, t.TempDir()).Plan(cfg, platform.Info{OS: platform.OSRedHat})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(plan.Steps) != 2 {
		t.Fatalf("Plan() steps = %d, want 2 (langpack install, set-locale):\n%s", len(plan.Steps), formatSteps(plan.Steps))
	}
	assertStepArgv(t, plan.Steps[0], []string{"sudo", "dnf", "install", "-y", "glibc-langpack-de"})
	assertStepArgv(t, plan.Steps[1], []string{"sudo", "localectl", "set-locale", "LANG=de_DE.UTF-8"})
}
```

This is the **first** task to modify `linux_test.go`; phase two of the R10 gate begins here, and its churn from now
on is the intentionality signal.

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && gofmt -l internal/sysconfig && go build ./... && go vet ./... \
  && go test -race -run 'Locale' -v ./internal/sysconfig/... \
  && go test -race ./internal/sysconfig/...
```

### WS6-T8 — R3a: decline the Homebrew bootstrap on arm64 Linux, with an explicit stderr note Depends on [WS6-T9]

**READ THESE BEFORE TASK**

- `/internal/brew/brew.go` (`:32-41`, `:74-83`), `/internal/brew/brew_test.go` (`:25-40`, `:86`)
- `/cmd/dotf/applyphases.go` (`:38-59`), `/cmd/dotf/apply.go` (`:41`, `:169-175`)

**Files to Modify**

- `/internal/brew/brew.go`, `/internal/brew/brew_test.go`, `/cmd/dotf/applyphases.go`, `/cmd/dotf/apply.go`

> **MAJOR FIX (critique) — `cmd/dotf/apply.go` added to the write set.** Step (3) changes `applyBrew`'s signature,
> and its **only** non-test caller is `cmd/dotf/apply.go:173` (`info = applyBrew(r, info, opts, &coll)`). The
> first-pass write set omitted it while the prose said "update every call site", so an agent scoped to the declared
> files would leave the tree broken.

**Instructions**

`brew.Applicable` (`brew.go:32-41`) gates on OS family + the `--homebrew` tristate with **no** `info.Arch` reference,
and `prefix()` (`:74-83`) returns `/home/linuxbrew/.linuxbrew` for every non-macOS host including arm64. Homebrew
publishes no aarch64-Linux bottles and its installer refuses on that host, so `--homebrew=true` on an Apple-Silicon
Fedora box runs a doomed installer whose failure is recorded by `applyBrew` and flips the whole run to exit 1.

**(1) `internal/brew/brew.go`** — replace `Applicable` (doc + func) with:

```go
// Applicable reports whether a Homebrew bootstrap should run on info given the
// --homebrew tristate. macOS always bootstraps brew (it is the canonical
// package manager); a Linux-family host bootstraps linuxbrew only when the
// caller opted in with --homebrew=true AND the architecture is one Homebrew
// actually supports. Homebrew publishes no aarch64-Linux bottles and its
// installer refuses to run there, so an arm64 Linux host is declined rather
// than handed a doomed installer whose failure would fail the whole run.
// Everything else is a no-op.
func Applicable(info platform.Info, homebrew string) bool {
	switch {
	case info.OS == platform.OSMacOS:
		return true
	case platform.IsLinuxFamily(info.OS) && homebrew == "true":
		return info.Arch != platform.ArchARM64
	default:
		return false
	}
}

// DeclinedUnsupportedArch reports whether info+homebrew asked for a Linux
// Homebrew bootstrap that Applicable refuses purely because of the
// architecture, so the caller can say so on stderr instead of silently doing
// nothing with an opt-in flag the user explicitly passed.
func DeclinedUnsupportedArch(info platform.Info, homebrew string) bool {
	return platform.IsLinuxFamily(info.OS) && homebrew == "true" && info.Arch == platform.ArchARM64
}
```

**(2) `internal/brew/brew_test.go`** — add two rows to the existing `TestApplicable` table (`~:29-35`), no new test
function:

```go
		{"linux opt-in on arm64 is declined (no aarch64 bottles)", platform.Info{OS: platform.OSRedHat, Arch: platform.ArchARM64}, "true", false},
		{"linux opt-in on x86_64 still bootstraps", platform.Info{OS: platform.OSRedHat, Arch: platform.ArchX86_64}, "true", true},
```

Check the existing row `{"linux opt-in bootstraps linuxbrew", platform.Info{OS: platform.OSDebian}, "true", true}` —
its zero-value `Arch` must not equal `platform.ArchARM64`; confirm by reading `internal/platform/types.go` and leave
it as-is if the zero value is distinct. Same check for `TestBootstrap_LinuxOptInInstallsLinuxbrew` (`:86`); if the
zero Arch is not distinct, set `Arch: platform.ArchX86_64` on both.

**(3) `cmd/dotf/applyphases.go`** — `applyBrew` currently has signature
`func applyBrew(r exec.Runner, info platform.Info, opts applyOptions, coll *failure.Collector) platform.Info` and no
writer. Add an `stderr io.Writer` parameter (place it after `r`, matching whatever convention the neighbouring phase
funcs use — read them and copy it), and at the **top** of the body:

```go
	if brew.DeclinedUnsupportedArch(info, opts.Homebrew) {
		_, _ = fmt.Fprintf(stderr, "%s: --homebrew declined: Homebrew publishes no aarch64-Linux bottles; using the native package manager instead\n", progName)
	}
```

This is the **fourth and final** serialized writer of `applyphases.go`. Also correct `applyBrew`'s doc comment,
which says the re-detect happens "so PkgMgrHomebrew is selected for the packages phase" — after WS6-T9 that is no
longer true on **arm64** Linux. Reword to: "then returns a possibly re-detected platform.Info (macOS and x86_64
Linux select PkgMgrHomebrew; on aarch64 Linux the native manager wins — see platform.detectPkgMgr)".

**(4) `cmd/dotf/apply.go:173`** — update the single call site to pass the run's stderr:
`info = applyBrew(r, stderr, info, opts, &coll)` (argument order per step 3). `stderr` is in scope —
`runApply(args []string, stdout, stderr io.Writer, r exec.Runner)` at `:41`. Confirm `progName` is the identifier
this package uses for the program-name prefix (it is, at `apply.go:93`/`:99`) and that `fmt` and `io` are imported in
`applyphases.go`.

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && gofmt -l internal/brew cmd/dotf && go build ./... && go vet ./... \
  && go test -race -v ./internal/brew/... && go test -race ./cmd/... \
  && golangci-lint run ./internal/brew/... ./cmd/dotf/...
```

### WS6-T10 — R4: informational SELinux reading in `dotf doctor` Depends on [WS5-T2]

**READ THESE BEFORE TASK**

- `/cmd/dotf/doctor.go` (`:30-80`, `:200-230`), `/cmd/dotf/doctor_test.go` (`:560-580`)

**Files to Modify**

- `/cmd/dotf/doctor.go`

**Instructions**

R4 assigns WS6 an **informational** SELinux reading so the device checklist can record `getenforce` before and after
every mutation and treat a flip to Permissive as stop-the-line. It must never fail the run: current exposure is
genuinely low (symlinks write under `$HOME` and inherit context; sysconfig's `installFile` writes into an existing
inode; plasma's `writeAtomic` renames within the destination directory), and `semanage` port labelling is an explicit
non-goal.

Add:

```go
// checkSELinux reports the host's SELinux mode as an INFORMATIONAL row. It is
// never a fail and never a warn: dotf's current write surface (symlinks under
// $HOME, sysconfig writes into existing inodes, plasma's same-directory atomic
// rename) all inherit or preserve context, so no mode is a fault. The row
// exists so a provisioning session can record the mode before and after a
// mutation and notice a flip. Hosts without getenforce (macOS, most
// non-RedHat Linux) emit no row at all.
func checkSELinux(r exec.Runner, info platform.Info) []doctorCheck {
	if !platform.IsLinuxFamily(info.OS) {
		return nil
	}
	if _, err := r.LookPath("getenforce"); err != nil {
		return nil
	}
	mode := "unknown"
	if out, err := r.Output("getenforce"); err == nil {
		if v := strings.TrimSpace(string(out)); v != "" {
			mode = strings.ToLower(v)
		}
	}
	return []doctorCheck{{
		Category: "selinux", Name: "mode", Status: checkOK,
		Detail: "selinux=" + mode,
	}}
}
```

Wire it into `runDoctor` immediately after the `checkSysconfig(info)` append:

```go
	checks = append(checks, checkSysconfig(info))
	checks = append(checks, checkSELinux(r, info)...)
```

**CONSTRAINTS**

- Status MUST be `checkOK` in every branch — `doctorExitCode` returns 1 on any fail and
  `TestRunDoctor_Healthy` (`doctor_test.go:567`) asserts exit 0.
- Emit **no** row when `getenforce` is absent, so macOS/Debian/Arch output is byte-identical and
  `TestRunDoctor_Healthy`'s `Contains`-based key list and its "summary line is last" assertion still hold
  (`healthyDoctorFake()` does not register `getenforce`).
- Do not set a `Remedy` — a remedy implies an action, and there is none.
- Confirm `strings` is imported; `exec` and `platform` already are.
- **No new test.** If an existing doctor test regresses, that is a signal the row is not actually inert — fix the
  code, not the test.

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && gofmt -l cmd/dotf && go build ./... && go vet ./... \
  && go test -race -v ./cmd/... 2>&1 | grep -E '^(---|ok|FAIL)' \
  && golangci-lint run ./cmd/dotf/...
```

### WS7-T1 — platform: single-scan `/etc/os-release` + `Info.VersionID` Depends on [WS6-T9]

**READ THESE BEFORE TASK**

- `/internal/platform/osrelease.go` (all), `/internal/platform/platform.go` (`:19-112`, post-WS6-T9)
- `/internal/platform/types.go` (`:160-180`), `/internal/platform/platform_test.go` (`:173-216`, `:287`, `:326`, `:344`)
- `/cmd/dotf/main.go` (`:109`), `/cmd/dotf/main_test.go` (`:119`)

**Files to Modify**

- `/internal/platform/osrelease.go`, `/internal/platform/platform.go`, `/internal/platform/types.go`

**Instructions**

`platform.Detect` scans `/etc/os-release` **three** times (`distroName`, `osReleaseCodename`, and `parseOSRelease`
inside `detectLinuxFamily`). Collapse to one scan and add `Info.VersionID`, which WS7-REPO's `dnf_repo`
`{{.ReleaseVer}}` templating and the dnf-major fallback consume.

**(1) `osrelease.go`** — add a parsed-record type and one reader, then turn the three existing path-taking helpers
into thin wrappers so `platform_test.go` compiles and passes **unchanged** (it calls `parseOSRelease(path)` at
`:344`, `osReleaseCodename(path)` at `:326`, `distroName(path)` at `:287`, and
`detectOS(r, osReleasePath, procVersionPath)` at `:173`/`:184`/`:193`/`:202`/`:216`).

Add after `scanOSRelease` (ends `:54`):

```go
// osRelease is the parsed subset of an os-release file dotf consumes: the
// distro family IDs, the Debian/Ubuntu codename, and the release version. It
// exists so Detect scans /etc/os-release exactly ONCE per run instead of once
// per consumed field.
type osRelease struct {
	// Present reports whether the file existed at scan time, mirroring the
	// shell's `[[ -f /etc/os-release ]]` test. It is what distinguishes a
	// missing file (WSL/generic-linux fallback) from a present-but-empty one.
	Present bool
	// ID is the os-release ID field (e.g. "fedora", "ubuntu").
	ID string
	// IDLike is the os-release ID_LIKE field (e.g. "fedora rhel").
	IDLike string
	// Codename is VERSION_CODENAME, falling back to UBUNTU_CODENAME.
	Codename string
	// VersionID is the raw VERSION_ID field (e.g. "42", "24.04"). Empty on
	// rolling distros that set none.
	VersionID string
}

// readOSRelease scans the os-release file at path once and returns every field
// dotf consumes. It degrades to the zero value (Present false, empty fields)
// when the file is missing or unreadable; it never panics.
func readOSRelease(path string) osRelease {
	fields := scanOSRelease(path)
	rel := osRelease{
		Present:   fileExists(path),
		ID:        fields["ID"],
		IDLike:    fields["ID_LIKE"],
		Codename:  fields["VERSION_CODENAME"],
		VersionID: fields["VERSION_ID"],
	}
	if rel.Codename == "" {
		rel.Codename = fields["UBUNTU_CODENAME"]
	}
	return rel
}

// distro returns the raw ID, or "unknown" when os-release defines none,
// mirroring common/platform.sh:get_distro_name.
func (o osRelease) distro() string {
	if o.ID == "" {
		return "unknown"
	}
	return o.ID
}
```

Rewrite `parseOSRelease` / `osReleaseCodename` / `distroName` as wrappers over `readOSRelease`, **keeping their exact
current signatures**, and add one sentence to each doc comment noting they re-scan and exist for the path-taking test
seam. Leave `scanOSRelease` and `unquote` untouched.

**(2) `platform.go`** — replace `Detect` with a version that scans once.

> **MAJOR FIX (critique / correction to the first-pass spec).** WS6-T9 (batch 4) already changed the signature to
> `detectPkgMgr(r exec.Runner, os OS, arch Arch) PkgMgr`. **Do NOT restore the one-argument call** — the obvious way
> to make a stale code block compile is to revert that signature, which would silently undo the R3 brew demotion.

```go
func Detect(r exec.Runner) Info {
	rel := readOSRelease(defaultOSReleasePath)
	os := detectOSFrom(r, rel, defaultProcVersionPath)
	arch := detectArch(r)
	return Info{
		OS:        os,
		Arch:      arch,
		PkgMgr:    detectPkgMgr(r, os, arch),
		Container: detectContainer(),
		Distro:    rel.distro(),
		Codename:  rel.Codename,
		VersionID: rel.VersionID,
	}
}
```

Keep `detectOS(r exec.Runner, osReleasePath, procVersionPath string) OS` as a thin wrapper
(`return detectOSFrom(r, readOSRelease(osReleasePath), procVersionPath)`) so the five existing test call sites are
untouched; move its current uname switch body (`:37-46`) into the new
`detectOSFrom(r exec.Runner, rel osRelease, procVersionPath string) OS`. Change `detectLinuxFamily` (`:72-82`) to
take `(rel osRelease, procVersionPath string)`: `if !rel.Present` replaces the `fileExists` test, and
`return osFamilyFromID(rel.ID, rel.IDLike)` replaces the `parseOSRelease` call.

Do **not** touch `osFamilyFromID` (`:87-112` — WS1-T2's Asahi arm), `detectPkgMgr`'s body (WS6-T9's), `detectArch`,
`unameM`, `archFromUname`, `detectContainer` or `IsLinuxFamily`.

**(3) `types.go`** — one field at the end of `Info`, after `Codename` (`:176`):

```go
	// VersionID is the raw /etc/os-release VERSION_ID (e.g. "42" on Fedora 42,
	// "24.04" on Ubuntu). It is the $releasever the repo backend's dnf_repo
	// templating consumes, and the fallback signal for the dnf4/dnf5 probe.
	// Empty on rolling distros that set no VERSION_ID, and on macOS/Windows.
	VersionID string
```

**HARD CONSTRAINT** — do **not** print `VersionID` from `dotf platform`: `cmd/dotf/main.go:109` emits
`os=%s arch=%s pkg=%s container=%s distro=%s` and `cmd/dotf/main_test.go:119` anchors that exact shape with
`^os=\S+ arch=\S+ pkg=\S+ container=\S+ distro=\S+\n$`. `Codename` is already unprinted; `VersionID` follows the
same rule. That contract is also asserted by critical test #6 (the Fedora CI smoke step) and documented by WS8-T1.

Add **no** test in this task — the existing `internal/platform` tests passing unmodified **is** the
behaviour-preservation proof.

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && gofmt -l internal/platform && go build ./... && go vet ./internal/platform/... \
  && go test -race ./internal/platform/... ./cmd/dotf/... \
  && golangci-lint run ./internal/platform/...
```

---

## Batch 6 — chsh fallback, font docs, and the repo backend schema

### WS5-T7 — Document the `font` rung on the backend ladder Depends on [WS5-T6]

**READ THESE BEFORE TASK**

- `/docs/reference/manifest-backends.md` (all 109 lines), `/manifest/packages.toml`, `/internal/manifest/validate.go`

**Files to Modify**

- `/docs/reference/manifest-backends.md`

**Instructions**

This file is the decision ladder that governs the `custom-script` budget. A native backend that is not on the ladder
will be bypassed for a `custom-script` entry — exactly what ADR-13 exists to prevent.

**EDIT 1 — opening paragraph (`:3-8`).** It reads "one of eight backends … Seven of them are **native** … The
eighth — `custom-script` — is the **escape hatch**". Reword **count-free** so WS7-T7's `repo` rung does not conflict:
"`dotf` installs every package through one of the backends declared on a `[[package]]` entry in
`manifest/packages.toml`. All but one are **native**: they describe _what_ to install in typed manifest fields …
The exception — `custom-script` — is the **escape hatch** …".

**EDIT 2 — "## The backend ladder" (`:13-30`).** The list is numbered 1–8 with `custom-script` last. Insert a `font`
rung between the current item 5 (`github-release`) and item 6 (`script-url`), renumbering the tail and keeping
`custom-script` as the final rung:

```
6. **`font`** — the package IS a font family the configs pin by name. Set `font.family`
   (the fontconfig name, and the installed-state probe) plus either per-manager `names`
   (Arch `ttf-*-nerd`, a Homebrew cask) or `font.archive` + `repo`/`tag` to extract the
   checksum-verified upstream release archive into the XDG fonts directory. A downloadable
   archive REQUIRES a checksum source — either `sha256` or `checksums_asset`, and the
   backend fails closed without one. The per-target `checksums` map is **not** accepted
   here: it is keyed `<os>/<arch>`, which is meaningless for a single arch-independent
   Nerd Font zip, and `fontChecksum` never reads it.
```

**EDIT 3 — "## How to add a native entry" (`:32-67`).** Add a worked example to the fenced TOML block, after the
`github-release` example and before `git-clone`:

```toml
# 6. font — a patched font family; package manager where one exists, archive otherwise
[[package]]
name = "jetbrains-mono-nerd-font"
backend = "font"
names = { pacman = "ttf-jetbrains-mono-nerd", homebrew = "font-jetbrains-mono-nerd-font" }
repo = "ryanoasis/nerd-fonts"
tag = "v3.4.0"                      # required whenever font.archive is set
checksums_asset = "SHA-256.txt"     # required whenever font.archive is set
font = { family = "JetBrainsMono Nerd Font", archive = "JetBrainsMono" }
```

Add one sentence after the fence recording the governance point: a font family is a native `font` entry, **never** a
`custom-script` one — the escape-hatch budget is frozen at 16 and cosmetics do not spend it.

Keep the file markdownlint-clean. Do **not** renumber or reword rung 3 (`apt-repo`) — WS7-T7 owns that edit and will
insert its `repo` rung relative to whatever is present.

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && npx --no-install markdownlint-cli2 docs/reference/manifest-backends.md
```

### WS6-T4 — ADR-8: guard `chsh` with `LookPath`, fall back to `usermod -s` (Linux only) Depends on [WS6-T3]

**READ THESE BEFORE TASK**

- `/internal/sysconfig/linux.go` (`planShell`, `:225-232`), `/internal/sysconfig/darwin.go` (`:163-170`)
- `/internal/sysconfig/linux_test.go` (`:63-110`, `:252-300`)

**Files to Modify**

- `/internal/sysconfig/linux.go`, `/internal/sysconfig/linux_test.go`

**Instructions**

ADR-8: two device surveys contradict each other on whether Fedora still ships `chsh` (base `util-linux` vs the
retired `util-linux-user` subpackage). A `LookPath` guard with a `usermod -s` fallback is correct under **both**
readings and costs four lines. `usermod` comes from `shadow-utils`, present on Fedora, Debian, Arch and SUSE base
installs.

**(1)** Inside `planShell`, replace:

```go
	if currentShellLinux(c.r, cfg.User) != path {
		p.Steps = append(p.Steps, Step{
			Domain: "shell",
			Desc:   "chsh " + cfg.User + " " + path,
			Argv:   []string{"sudo", "chsh", "-s", path, cfg.User},
		})
		changed = true
	}
```

with:

```go
	if currentShellLinux(c.r, cfg.User) != path {
		p.Steps = append(p.Steps, loginShellStep(c.r, cfg.User, path))
		changed = true
	}
```

and add, as a package-level func immediately after `planShell`:

```go
// loginShellStep builds the login-shell change Step for user. chsh is
// preferred, but it is not universally present: on some Fedora releases it
// lives in the separate util-linux-user subpackage, and on others it is folded
// back into base util-linux. When chsh is absent, fall back to `usermod -s`
// from shadow-utils, which is present on Fedora, Debian, Arch and SUSE base
// installs. (macOS keeps chsh unconditionally — see darwin.go — because
// usermod does not exist there.)
func loginShellStep(r exec.Runner, user, path string) Step {
	if _, err := r.LookPath("chsh"); err == nil {
		return Step{
			Domain: "shell",
			Desc:   "chsh " + user + " " + path,
			Argv:   []string{"sudo", "chsh", "-s", path, user},
		}
	}
	return Step{
		Domain: "shell",
		Desc:   "usermod -s " + path + " " + user,
		Argv:   []string{"sudo", "usermod", "-s", path, user},
	}
}
```

**Do NOT touch `internal/sysconfig/darwin.go`** — its near-identical block at `:163-170` (`currentShellDarwin`,
`Argv: []string{"sudo", "chsh", "-s", path, cfg.User}`) stays unconditional by design.

**(2) MANDATORY test churn** — existing fakes never register `chsh` on PATH, so without this the guard silently flips
three passing assertions from `chsh` to `usermod`, a green-looking false pass:

- **(a)** `TestLinux_Plan_AllDomains` (`:63`): add `fake.AddPath("chsh", "/usr/bin/chsh")` immediately after the
  existing `fake.AddPath("zsh", "/usr/bin/zsh")`. The step count (9) and the `assertStepArgv(t, plan.Steps[6], …chsh…)`
  assertion then hold unchanged.
- **(b)** `TestLinux_Shell_EtcShellsGuard` (`:252`): add `fake.AddPath("chsh", "/usr/bin/chsh")` immediately after
  the existing `fake.AddPath("zsh", …)` inside the subtest body. Every other assertion stays as-is.
- **(c)** `TestLinux_Plan_Idempotent` (`:109`): it seeds `currentShell` already equal to the target so no shell step
  is emitted; verify it still passes and add `AddPath("chsh", …)` only if it regresses.
- **(d)** ONE new test immediately after `TestLinux_Shell_EtcShellsGuard`:

```go
// TestLinux_Shell_UsermodFallbackWhenNoChsh proves ADR-8: on a host with no
// chsh (some Fedora releases ship it only in util-linux-user), the login-shell
// change is planned via `usermod -s` from shadow-utils rather than emitting an
// argv whose binary does not exist.
func TestLinux_Shell_UsermodFallbackWhenNoChsh(t *testing.T) {
	root := t.TempDir()
	seedFile(t, root, "etc/shells", []byte("/bin/bash\n/usr/bin/zsh\n"))

	fake := exec.NewFake()
	fake.AddPath("zsh", "/usr/bin/zsh")
	// chsh intentionally NOT on PATH.
	fake.AddCommand("getent", []string{"passwd", "yandy"}, []byte("yandy:x:1000:1000::/home/yandy:/bin/bash\n"), nil)

	cfg := Config{Shell: "zsh", User: "yandy"}
	plan, err := newLinuxTest(fake, root).Plan(cfg, platform.Info{OS: platform.OSRedHat})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("Plan() steps = %d, want 1:\n%s", len(plan.Steps), formatSteps(plan.Steps))
	}
	assertStepArgv(t, plan.Steps[0], []string{"sudo", "usermod", "-s", "/usr/bin/zsh", "yandy"})
}
```

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && gofmt -l internal/sysconfig && go build ./... && go vet ./... \
  && go test -race -run 'Shell|AllDomains|Idempotent' -v ./internal/sysconfig/... \
  && go test -race ./internal/sysconfig/...
```

### WS7-T2 — Manifest schema: `BackendRepo` + `DNFRepo` + validation Depends on [WS5-T4]

**READ THESE BEFORE TASK**

- `/internal/manifest/types.go` (post-WS5-T4), `/internal/manifest/class.go`, `/internal/manifest/validate.go`
  (`:226-242`, `:326-340`, `:475-507`), `/internal/manifest/class_test.go`
- `/internal/manifest/testdata/invalid_apt_repo_missing_field.toml`

**Files to Create**

- `/internal/manifest/validate_repo.go`

**Files to Modify**

- `/internal/manifest/types.go`, `/internal/manifest/class.go`, `/internal/manifest/validate.go`,
  `/internal/manifest/class_test.go`

**Instructions**

ADR-5: add the `repo` backend value and the `dnf_repo` sub-table, keeping `apt-repo` as a deprecated alias that
validates identically. Schema/validation only — no backend implementation (WS7-REPO), no manifest data
(WS7-MANIFEST).

**(1) `types.go`** — in `Package`, immediately after `AptRepo *AptRepo \`toml:"apt_repo"\``:

```go
	// DNFRepo configures the dnf repository to add before installing, for the
	// "repo" backend on a RedHat-family host. It is the dnf sibling of AptRepo:
	// a package may carry either, or both (one logical tool, one entry), and the
	// backend dispatches on the host's package manager.
	DNFRepo *DNFRepo `toml:"dnf_repo"`
```

Immediately after the `AptRepo` struct, add `DNFRepo` with exactly **three** mutually exclusive modes (ADR-6):

```go
// DNFRepo describes a third-party dnf repository to configure before installing
// a package via the "repo" backend. Exactly one of three modes is used:
//
//	copr     — a COPR project enabled via `dnf -y copr enable <owner>/<project>`
//	repofile — a vendor-published .repo file curl'd and installed verbatim
//	baseurl  — an explicit repo body rendered by dotf
//
// The two file modes write /etc/yum.repos.d/<id>.repo directly with
// `sudo install -m 644` and never shell out to `dnf config-manager`, whose repo
// verbs and plugin package differ between dnf4 and dnf5 (see ADR-6).
type DNFRepo struct {
	// COPR is an "owner/project" COPR slug. Mutually exclusive with the file
	// modes; it is the only mode that shells out to dnf to add the repo.
	COPR string `toml:"copr"`

	// ID is the repository id. It names the file written to
	// /etc/yum.repos.d/<id>.repo and is REQUIRED for both file modes. It must be
	// a single safe path element (no "/", not "." or "..").
	ID string `toml:"id"`

	// RepoFile is the HTTPS URL of a vendor-published .repo file, installed
	// verbatim. Requires ID.
	RepoFile string `toml:"repofile"`

	// BaseURL is the repository base URL for the explicit mode. It may use the
	// {{.ReleaseVer}} (os-release VERSION_ID) and {{.Arch}} placeholders, the dnf
	// analogue of AptRepo.RepoLine's {{.Codename}}/{{.Arch}}. Requires ID.
	BaseURL string `toml:"baseurl"`

	// GPGKey is the HTTPS URL of the repo's signing key, for the explicit mode.
	// Optional: an empty key renders gpgcheck=0. Supports the same placeholders
	// as BaseURL.
	GPGKey string `toml:"gpgkey"`
}
```

Add the enum value after `BackendAPTRepo`:

```go
	// BackendRepo adds a third-party OS repository before installing: an APT
	// sources entry via apt_repo on a Debian-family host, a dnf repository via
	// dnf_repo on a RedHat-family host. One logical tool is ONE entry; the
	// backend dispatches on the host's package manager and is not applicable on a
	// host with no matching sub-table.
	BackendRepo Backend = "repo"
```

and extend `BackendAPTRepo`'s doc with:
`// Deprecated: apt-repo is a compatibility alias for BackendRepo, retained one release. New entries use "repo" with an apt_repo sub-table.`
Add `BackendRepo` to `Valid()`'s case list.

> **Correction (critique).** The first-pass spec said to update three "eight" comments to "nine declared values /
> eight mechanisms". **Do not write a count.** WS5-T4 (batch 3) already made `:78`, `:325` and `:360` count-free
> precisely so they stop rotting, and `repo` would make a fresh numeral wrong again on the next backend. **Confirm**
> those three comments are count-free; if any numeral survives, remove it.

**(2) `class.go`** — add `BackendRepo: {ClassManaged: true},` to `classByBackend` next to the `BackendAPTRepo` row,
update the map's doc to name `repo` among the authority-managed backends, and add `BackendRepo` to the
`case BackendPkg, BackendAUR, BackendAPTRepo, BackendToolchain, BackendGitClone:` arm of `EffectiveClass` and to that
function's doc backend list.

**(3) `validate.go`** — replace the whole `case BackendAPTRepo:` arm (`:226-242`, the nil check plus four
required-field checks) with:

```go
	case BackendRepo, BackendAPTRepo:
		errs = append(errs, validateRepo(p)...)
```

Make no other change to this file (it is already 602 lines; the new rules go in a new file).

**(4) CREATE `internal/manifest/validate_repo.go`** (package `manifest`) implementing `validateRepo(p Package) []error`
plus helpers:

- Sub-table presence: `p.AptRepo == nil && p.DNFRepo == nil` → preserve the legacy vocabulary for the alias —
  `"apt-repo backend requires apt_repo"` when `p.Backend == BackendAPTRepo`, otherwise
  `"repo backend requires at least one of apt_repo or dnf_repo"`.
- When `p.AptRepo != nil`, keep the four existing required-field checks with their **current message text verbatim**
  (`"apt-repo backend requires apt_repo.key_url"`, `…apt_repo.keyring`, `…apt_repo.repo_line`,
  `…apt_repo.sources_path`). `internal/manifest/testdata/invalid_apt_repo_missing_field.toml` must keep failing
  exactly as it does today.
- When `p.DNFRepo != nil`, validate the three-mode discriminator:
  - count set modes (copr / repofile / baseurl); zero →
    `"repo backend dnf_repo requires one of copr, repofile, or baseurl"`; more than one →
    `"repo backend dnf_repo modes copr, repofile and baseurl are mutually exclusive"`.
  - **copr**: the slug must match a package-level `coprSlugPattern` regexp
    `^[A-Za-z0-9._@+-]+/[A-Za-z0-9._+-]+$` → `fmt.Sprintf("repo backend dnf_repo copr %q must be \"owner/project\"", …)`;
    `id`, `baseurl`, `gpgkey` must be empty → `"repo backend dnf_repo copr mode takes no id, baseurl or gpgkey"`.
  - **file modes**: `id` is REQUIRED → `"repo backend dnf_repo requires id"`; and `id` must be a single safe path
    element — reject when it contains `/` or equals `.`/`..` (it is joined into `/etc/yum.repos.d/<id>.repo`) →
    `fmt.Sprintf("repo backend dnf_repo id %q must be a bare filename", id)`.
  - `repofile`, `baseurl` and `gpgkey`, when set, must parse and carry the **https** scheme, mirroring
    `validateGitClone`'s check at `validate.go:334` (`u, parseErr := url.Parse(v); parseErr != nil || u.Scheme != "https"`).
    Messages: `"repo backend dnf_repo requires https repofile"` / `"… https baseurl"` / `"… https gpgkey"`. A
    templated URL such as `https://example.com/{{.ReleaseVer}}/x86_64` still parses with scheme https — do not reject
    placeholders.
- Every error is `&ValidationError{Package: p.Name, Msg: …}`.

Do **not** add `BackendRepo` to `pathInstallBackends` (`:502-507`): like pkg/apt-repo/aur it queries the package
authority, so `provides`/`installed_when` stay rejected on it. `knownPkgMgrs` (`:64-71`) and `knownPackageOS`
(`:475-483`) already contain everything needed.

**(5) `class_test.go`** — two mirror rows only (keeping the existing tables exhaustive over the Backend const block;
not new tests): beside the apt-repo row in `TestPackage_EffectiveClass`,
`{"repo infers managed", manifest.Package{Backend: manifest.BackendRepo}, manifest.ClassManaged},`; and beside the
apt-repo row in `TestLegalClasses`, `{manifest.BackendRepo, []manifest.Class{manifest.ClassManaged}},`. Add nothing
else and no new testdata fixture.

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && gofmt -l internal/manifest && go build ./... && go vet ./internal/manifest/... \
  && go test -race ./internal/manifest/... && golangci-lint run ./internal/manifest/...
```

---

## Batch 7 — firewalld install and the family-dispatching `repo` backend

### WS6-T5 — `planFirewalld`: LookPath-gated install step + hardened `--state` test Depends on [WS6-T4]

**READ THESE BEFORE TASK**

- `/internal/sysconfig/linux_firewall.go` (`planUfw`, `planFirewalld`), `/internal/sysconfig/linux.go`
  (`planSSHServerFirewallOpen`, `:449`), `/internal/sysconfig/linux_test.go` (`:455-472`)

**Files to Modify**

- `/internal/sysconfig/linux_firewall.go`
- `/internal/sysconfig/linux.go` ← **added to the write set (critique major)**: step (3) below de-duplicates
  `planSSHServerFirewallOpen`'s RedHat/SUSE arm, which lives in `linux.go`, against the shared helper this task adds
  to `linux_firewall.go`. The first-pass write set omitted it while the prose required the edit.
- `/internal/sysconfig/linux_test.go`

**Instructions**

`planFirewalld` today assumes firewalld is already installed and conflates three distinct states in one test
(`if out, err := c.r.Output("firewall-cmd", "--state"); err != nil || !strings.Contains(string(out), "running")`). On
a Fedora KDE spin without firewalld it plans `systemctl enable --now firewalld` for a unit that does not exist.
Mirror `planUfw`'s shape: install when absent, **then** interpret state.

**(1) `linux_firewall.go`** — widen the signature (`planFirewalld` takes only `(p *Plan)` today; it needs `info` for
`pkgInstallArgv`) and replace the whole function:

```go
// planFirewalld installs firewalld when absent, ensures it is running, and
// ensures the ssh service is permanently allowed (permanent + reload). It
// mirrors planUfw's shape: presence is decided by LookPath FIRST, so a host
// without firewall-cmd is not asked to enable a unit that does not exist, and
// the --state read is only interpreted once the binary is known to be present.
func (c *linuxConfigurer) planFirewalld(info platform.Info, p *Plan) {
	_, lookErr := c.r.LookPath("firewall-cmd")
	installed := lookErr == nil
	if !installed {
		p.Steps = append(p.Steps, Step{
			Domain: "firewall",
			Desc:   "install firewalld",
			Argv:   pkgInstallArgv(info, "firewalld"),
		})
	}

	if !installed || !firewalldRunning(c.r) {
		p.Steps = append(p.Steps, Step{
			Domain: "firewall",
			Desc:   "firewalld enable",
			Argv:   []string{"sudo", "systemctl", "enable", "--now", "firewalld"},
		})
	}

	if _, err := c.r.Output("firewall-cmd", "--permanent", "--query-service=ssh"); err != nil {
		p.Steps = append(p.Steps, firewalldAllowSSHSteps("firewall")...)
	}
}

// firewalldRunning reports whether `firewall-cmd --state` says running. It is
// only meaningful once firewall-cmd is known to be on PATH; a probe error on a
// present binary means not-running.
func firewalldRunning(r exec.Runner) bool {
	out, err := r.Output("firewall-cmd", "--state")
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "running")
}

// firewalldAllowSSHSteps returns the permanent ssh-allow + reload pair, tagged
// with domain so both the firewall and ssh-server planners emit identical argv.
func firewalldAllowSSHSteps(domain string) []Step {
	return []Step{
		{
			Domain: domain,
			Desc:   "firewalld allow ssh",
			Argv:   []string{"sudo", "firewall-cmd", "--permanent", "--add-service=ssh"},
		},
		{
			Domain: domain,
			Desc:   "firewalld reload",
			Argv:   []string{"sudo", "firewall-cmd", "--reload"},
		},
	}
}
```

Add `"github.com/yandy-r/dotfiles/internal/exec"` to `linux_firewall.go`'s imports (`firewalldRunning` takes an
`exec.Runner`).

**(2)** Update **both** call sites in the same file's `planFirewall`: `case FirewallFirewalld: c.planFirewalld(info, p)`
and, inside `case FirewallAuto:`, `c.planFirewalld(info, p)`.

**(3) `linux.go`** — refactor `planSSHServerFirewallOpen`'s RedHat/SUSE arm to reuse the shared helper so the two
planners cannot drift: replace its inline two-Step literal with
`p.Steps = append(p.Steps, firewalldAllowSSHSteps("ssh-server")...)`. The emitted argv are byte-identical, so
`TestLinux_SSHServer_FirewallOpenPresent`'s redhat row (`linux_test.go:736-746`) still passes.

**(4) MANDATORY golden churn** in `linux_test.go` — `TestLinux_Firewall_Firewalld` (~`:455`) uses a bare
`exec.NewFake()`, so `firewall-cmd` is now **absent** and an install step is prepended, moving the count 3 → 4.
Rewrite that test's table so it asserts the install argv per family:

```go
func TestLinux_Firewall_Firewalld(t *testing.T) {
	tests := []struct {
		os          platform.OS
		wantInstall []string
	}{
		{os: platform.OSRedHat, wantInstall: []string{"sudo", "dnf", "install", "-y", "firewalld"}},
		{os: platform.OSSUSE, wantInstall: []string{"sudo", "zypper", "--non-interactive", "install", "firewalld"}},
	}
	for _, tt := range tests {
		t.Run(string(tt.os), func(t *testing.T) {
			fake := exec.NewFake() // firewall-cmd absent → install, enable, allow, reload

			cfg := Config{Firewall: FirewallAuto, User: "yandy"}
			plan, err := newLinuxTest(fake, t.TempDir()).Plan(cfg, platform.Info{OS: tt.os})
			if err != nil {
				t.Fatalf("Plan() error = %v", err)
			}
			if len(plan.Steps) != 4 {
				t.Fatalf("Plan() firewalld steps = %d, want 4:\n%s", len(plan.Steps), formatSteps(plan.Steps))
			}
			assertStepArgv(t, plan.Steps[0], tt.wantInstall)
			assertStepArgv(t, plan.Steps[1], []string{"sudo", "systemctl", "enable", "--now", "firewalld"})
			assertStepArgv(t, plan.Steps[2], []string{"sudo", "firewall-cmd", "--permanent", "--add-service=ssh"})
			assertStepArgv(t, plan.Steps[3], []string{"sudo", "firewall-cmd", "--reload"})
		})
	}
}
```

Read `:455-472` first and **preserve** whatever surrounding assertions the original carried (e.g. any
`assertReadOnlyPlanCalls` call). Then re-run the whole package: any other test that plans the firewall domain on
RedHat/SUSE with a bare fake now sees the extra install step and needs its count adjusted the same way. Do **not**
adjust anything on Debian/Arch/WSL — if a ufw-path test moves, that is a regression, not churn.

> Note on **C20**: no dedicated `ufwInstallArgv` RedHat row is added. `TestLinux_Firewall_UfwSelection` uses
> `Firewall: FirewallAuto` and asserts exactly 3 steps, but RedHat under `auto` routes to firewalld — a RedHat row
> there would need an explicit `FirewallUfw` and a different step expectation. WS6-T2's extraction is instead
> covered by this task's firewalld install-argv assertions, which exercise the same `pkgInstallArgv`.

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && gofmt -l internal/sysconfig && go build ./... && go vet ./... \
  && go test -race -v ./internal/sysconfig/... 2>&1 | grep -E '^(---|ok|FAIL)' \
  && go test -race ./cmd/... ./internal/...
```

### WS7-REPO — Generalise `apt-repo` into the family-dispatching `repo` backend + critical test #5 Depends on [WS7-T2, WS7-T1, WS2-CORE]

**Merged from WS7-T3 + WS7-T4.** Mandated atomic by design §7: a new backend that writes files under `/etc` with
`sudo` is not shippable untested, and critical test #5's apt regression case is the only proof that generalising
`apt-repo` did not break the live `docker-ce` path in Debian's headless default set.

**READ THESE BEFORE TASK — mandatory**

- `/internal/backend/apt_repo.go` **in full** and `/internal/backend/apt_repo_test.go` **in full** (414 lines).
  `apt_repo_test.go` stays **unchanged** and is the ADR-5 regression guard; it also defines the shared helpers
  `assertCall` (`:362`), `assertSudoCall` (`:377`), `assertSudoGpgDearmor`, `assertSudoInstallSources` (`:400`) that
  `repo_test.go` reuses and must **not** redeclare.
- `/internal/backend/backend.go` (WS2-CORE's `packageGated`), `/internal/backend/pkg.go` (`:180-190`),
  `/internal/backend/register.go`, `/internal/backend/register_test.go`, `/internal/backend/applicable_test.go`
- `/internal/manifest/types.go` (WS7-T2's `DNFRepo`), `/internal/platform/types.go` (WS7-T1's `VersionID`)

**Files to Create**

- `/internal/backend/repo.go`, `/internal/backend/repo_dnf.go`, `/internal/backend/repo_test.go`

**Files to Delete**

- `/internal/backend/apt_repo.go`

**Files to Modify**

- `/internal/backend/register.go`, `/internal/backend/register_test.go`, `/internal/backend/applicable_test.go`

**Instructions**

**(A) `repo.go`.** Move here, **verbatim**, from `apt_repo.go`: `UnsupportedPlatformError` + `Error` (`:14-25`),
`MissingPackageNameError` + `Error` (`:27-36`), `renderRepoLine` (`:139-150`), `writeTempFile` (`:152-176`). Update
`UnsupportedPlatformError`'s doc to say the repo backend cannot run on the detected host (no matching sub-table for
its package manager). Then:

```go
// repoBackend adds a third-party OS repository before installing, dispatching on
// the host's package manager: apt_repo drives Debian/Ubuntu, dnf_repo drives
// Fedora/RHEL. One logical tool is one manifest entry (see ADR-5); a host with
// no matching sub-table reports not-applicable rather than failing.
type repoBackend struct {
	r exec.Runner
}

func newRepoBackend(r exec.Runner) *repoBackend { return &repoBackend{r: r} }

// newAptRepoBackend constructs the repo backend under its deprecated apt-repo
// alias, retained one release so existing manifests keep working.
//
// Deprecated: use newRepoBackend.
func newAptRepoBackend(r exec.Runner) *repoBackend { return newRepoBackend(r) }
```

`applicableFor` — implement WS2-CORE's `packageGated` interface (`backend.go:110-116`) with its **exact** declared
name and signature `applicableFor(pkg manifest.Package, info platform.Info) bool`. Body: true only when
(`info.PkgMgr == platform.PkgMgrAPT && pkg.AptRepo != nil`) or (`info.PkgMgr == platform.PkgMgrDNF && pkg.DNFRepo != nil`).
Add `var _ packageGated = (*repoBackend)(nil)` beside `var _ Backend = (*repoBackend)(nil)`. **CRITICAL:** do **not**
give `repoBackend` an `applicable(info platform.Info) bool` method — implementing `platformGated` would shadow the
package-aware gate in `Engine.Applicable`'s assertion order.

`Install(pkg, info)`: guard first with
`if !b.applicableFor(pkg, info) { return &UnsupportedPlatformError{Backend: backendLabel(pkg), OS: info.OS, PkgMgr: info.PkgMgr} }`.
Add `func backendLabel(pkg manifest.Package) manifest.Backend` returning `pkg.Backend` when non-empty and
`manifest.BackendRepo` otherwise — this keeps `apt_repo_test.go:135-137` passing (that fixture sets
`Backend: manifest.BackendAPTRepo` and asserts the error carries it). Then switch on `info.PkgMgr`: `PkgMgrAPT` →
`b.installAPT(pkg, info)`; `PkgMgrDNF` → `b.installDNF(pkg, info)`.

> **Correction C5.** The design says "lift `installAPT` verbatim". **No such identifier exists.** The body is
> `apt_repo.go:61-114` — everything after the guard, up to the final `return nil`. Lift it with **no** reordering,
> renaming or rewording: curl the key, `writeTempFile("dotf-apt-key-*")`, `sudo install -d -m 0755 <keyring dir>`,
> `sudo gpg --batch --yes --dearmor -o <keyring> <tmp>`, `renderRepoLine`, `writeTempFile("dotf-apt-sources-*")`,
> `sudo install -m 644 <tmp> <sources_path>`, `sudo apt-get update`, `sudo apt-get install -y <aptName>`. Keep the
> `repo := pkg.AptRepo; if repo == nil` nil-check (`:66-69`) as defence-in-depth even though it is now unreachable.

`Installed(pkg, info) (bool, error)` — gate on the package **MANAGER ONLY**, never on the sub-table
(`apt_repo_test.go:140-214` probes installed-state with fixtures that carry `Names` but no `AptRepo`; requiring the
sub-table here would break three passing tests):

- `PkgMgrAPT`: unchanged from `apt_repo.go:126-136` — name from `pkg.Names["apt"]` (missing/empty →
  `&MissingPackageNameError{Package: pkg.Name, PkgMgr: platform.PkgMgrAPT}`), then `dpkgInstalledQuery(name)` via
  `b.r.Output`, error → `(false, nil)`, else `dpkgStatusInstalled(out)`.
- `PkgMgrDNF`: name from `pkg.Names["dnf"]` (missing/empty → MissingPackageNameError with PkgMgrDNF), then
  `b.r.Output("rpm", "-q", name)`; error → `(false, nil)`, success → `(true, nil)`. Matches `pkg.go:186-187` exactly.
- any other manager → `(false, &UnsupportedPlatformError{Backend: backendLabel(pkg), OS: info.OS, PkgMgr: info.PkgMgr})`,
  preserving `apt_repo_test.go:216-238`.

**(B) `repo_dnf.go`** — the RedHat arm, kept separate so `repo.go` stays well under the ~500-line cap.

- `func (b *repoBackend) installDNF(pkg manifest.Package, info platform.Info) error`: resolve
  `dnfName := pkg.Names["dnf"]` (missing/empty → MissingPackageNameError{PkgMgr: platform.PkgMgrDNF});
  `repo := pkg.DNFRepo` (nil → `fmt.Errorf("backend: package %q missing dnf_repo configuration", pkg.Name)`);
  dispatch the three modes; finish **every** mode with `b.r.Run("sudo", "dnf", "install", "-y", dnfName)`.
- **repofile** mode: `body, err := b.r.Output("curl", "-fsSL", <rendered repofile>)`;
  `writeTempFile("dotf-dnf-repo-*", body)` (defer `os.Remove`);
  `b.r.Run("sudo", "install", "-m", "644", tmp, "/etc/yum.repos.d/"+repo.ID+".repo")`. Deliberately the same
  `sudo install -m 644` pattern as `apt_repo.go:102` — it is the SELinux-correct one (the file is created at the
  destination and inherits the right type label, where a `mv` from `/tmp` would carry `tmp_t`). **NEVER** call
  `dnf config-manager`.
- **explicit** (`baseurl`) mode: render the body in Go, then the same writeTempFile + `sudo install -m 644` +
  install (no curl in this mode):

  ```
  [<id>]
  name=<id>
  baseurl=<rendered baseurl>
  enabled=1
  gpgcheck=<1 when GPGKey set, else 0>
  gpgkey=<rendered gpgkey>   (line omitted entirely when GPGKey is empty)
  ```

  (trailing newline).
- **copr** mode: probe the dnf major, then `b.r.Run("sudo", "dnf", "install", "-y", plugin)` where plugin is
  `"dnf5-plugins"` when major ≥ 5 and `"dnf-plugins-core"` otherwise, then
  `b.r.Run("sudo", "dnf", "-y", "copr", "enable", repo.COPR)`, then the install.
- `func (b *repoBackend) dnfMajor(info platform.Info) int` (ADR-7): parse `b.r.Output("dnf", "--version")` — take the
  first line, trim; prefix `dnf5` → 5; otherwise scan for the first token beginning with a digit and return its
  leading integer. On any error or unparseable output, fall back to os-release: parse the leading integer of
  `info.VersionID` (WS7-T1) and return 5 when ≥ 41, else 4. Do **NOT** probe `LookPath("dnf5")` — on Fedora 40 dnf5
  can be installed while `/usr/bin/dnf` is still dnf4, so that probe lies. Write **no** dedicated unit test for this
  helper (design §7 explicitly excludes `TestDetectDNFMajor`); the copr case exercises it.
- `func renderRepoTemplate(s string, info platform.Info) (string, error)`: substitute `{{.ReleaseVer}}` →
  `info.VersionID` and `{{.Arch}}` → `info.Arch.ReleaseArch()`, mirroring `renderRepoLine`. Error when the string
  contains `{{.ReleaseVer}}` and `info.VersionID` is empty — the message must mention `releasever`, mirroring
  `renderRepoLine`'s codename error. Apply it to RepoFile, BaseURL and GPGKey.
- **Scope**: `platform.PkgMgrDNF` only. `PkgMgrYUM` is deliberately unsupported (no live manifest entry, and every
  current RedHat target ships dnf); a yum host falls through to not-applicable / `UnsupportedPlatformError`.

**(C)** `git rm internal/backend/apt_repo.go` — every declaration has been lifted into `repo.go` verbatim.
`apt_repo_test.go` survives untouched as the alias regression suite.

**(D) `register.go`** — build one instance and map both keys to it, so `apt-repo` and `repo` share exactly one
implementation:

```go
	repo := newRepoBackend(r)
	…
		manifest.BackendRepo:          repo,
		manifest.BackendAPTRepo:       repo, // deprecated alias, retained one release
```

replacing line `:21`. **Do not write a backend count** in the `:10` doc comment — WS5-T5 already made it count-free
("every canonical backend"); leave it that way.

**(E) `register_test.go`** — add `manifest.BackendRepo,` to `validBackends` (`:23-32`), beside
`manifest.BackendAPTRepo`.

**(F) `applicable_test.go`** — its **second and final** write (WS2-CORE owned it in batch 2). The two apt-repo rows
(`:31-32`) are **broken by this change**: the fixture builds a package with no sub-table, so a package-aware gate now
reports false on the apt host too. Extend the table struct with two bools (`aptRepo` / `dnfRepo`), set
`pkg.AptRepo = &manifest.AptRepo{…}` / `pkg.DNFRepo = &manifest.DNFRepo{…}` when true, add a
`fedoraDNF := platform.Info{OS: platform.OSRedHat, PkgMgr: platform.PkgMgrDNF}` fixture beside the existing three,
and replace the two rows with these five:

- `{"apt-repo alias with apt_repo on pacman host", manifest.BackendAPTRepo, …, archPacman, false}`
- `{"apt-repo alias with apt_repo on apt host", manifest.BackendAPTRepo, …, debianAPT, true}`
- `{"repo with apt_repo on apt host", manifest.BackendRepo, …, debianAPT, true}`
- `{"repo with apt_repo only on dnf host", manifest.BackendRepo, …, fedoraDNF, false}`
- `{"repo with dnf_repo on dnf host", manifest.BackendRepo, …, fedoraDNF, true}`

Change nothing else in that table — WS2-CORE owns the `pkg on …` rows.

**(G) CREATE `repo_test.go` — CRITICAL TEST #5.** Package `backend` (**internal**, same as `apt_repo_test.go` — it
needs `newRepoBackend`). Reuse `assertCall` / `assertSudoCall` / `assertSudoGpgDearmor` / `assertSudoInstallSources`
from `apt_repo_test.go`; do **not** redeclare them (duplicate declarations will not compile).

One table-driven test `TestRepoBackend_Install_ExactArgv` with exactly **four** cases. Each builds a fresh
`exec.NewFake()`, registers stubs, calls `newRepoBackend(r).Install(pkg, info)`, then asserts the **full** call
sequence from `r.CallsSnapshot()` — exact length first, then each call.

1. **dnf repofile** (the live docker-ce Fedora path). info
   `{OS: OSRedHat, PkgMgr: PkgMgrDNF, Arch: ArchARM64, VersionID: "42"}`; pkg `BackendRepo`,
   `Names: {"dnf": "docker-ce"}`, `DNFRepo: {ID: "docker-ce", RepoFile: "https://download.docker.com/linux/fedora/docker-ce.repo"}`;
   stub the curl. Want, in order: `curl -fsSL <url>`;
   `sudo install -m 644 <tempfile> /etc/yum.repos.d/docker-ce.repo` (assert argv[0..2] and argv[4], argv[3] non-empty
   — reuse `assertSudoInstallSources`); `sudo dnf install -y docker-ce`. **Exactly 3 calls.**
2. **dnf explicit baseurl with `{{.ReleaseVer}}`**. info same but `VersionID: "42"`, `Arch: ArchX86_64`; pkg
   `DNFRepo: {ID: "example", BaseURL: "https://example.com/fedora/{{.ReleaseVer}}/{{.Arch}}", GPGKey: "https://example.com/RPM-GPG-KEY"}`,
   `Names: {"dnf": "example"}`. Want: `sudo install -m 644 <tempfile> /etc/yum.repos.d/example.repo`;
   `sudo dnf install -y example`. **Exactly 2 calls** — assert **no** curl. In the same case, assert the rendering
   directly: `renderRepoTemplate("https://example.com/fedora/{{.ReleaseVer}}/{{.Arch}}", info)` ==
   `"https://example.com/fedora/42/amd64"`, and that it **errors** when `info.VersionID` is empty and the string uses
   `{{.ReleaseVer}}`.
3. **dnf copr on dnf5**. info `{OS: OSRedHat, PkgMgr: PkgMgrDNF, VersionID: "42"}`; pkg
   `DNFRepo: {COPR: "atim/lazygit"}`, `Names: {"dnf": "lazygit"}`; stub
   `r.AddCommand("dnf", []string{"--version"}, []byte("dnf5 version 5.2.6.2\n"), nil)`. Want: `dnf --version`;
   `sudo dnf install -y dnf5-plugins`; `sudo dnf -y copr enable atim/lazygit`; `sudo dnf install -y lazygit`.
   **Exactly 4 calls.**
4. **apt regression** — the ADR-5 no-regression proof. info `{OS: OSDebian, PkgMgr: PkgMgrAPT, Codename: "noble", Arch: ArchX86_64}`;
   pkg `BackendRepo`, `Names: {"apt": "docker-ce", "dnf": "docker-ce"}`, and the exact `AptRepo` sub-table shipped in
   `manifest/packages.toml`; stub the curl key fetch as `apt_repo_test.go:25` does. Want the **same six calls**
   `apt_repo_test.go:50-55` asserts, in the same order: `curl -fsSL <keyURL>`; `sudo install -d -m 0755 /etc/apt/keyrings`;
   `sudo gpg --batch --yes --dearmor -o /etc/apt/keyrings/docker.gpg <tmp>` (reuse `assertSudoGpgDearmor`);
   `sudo install -m 644 <tmp> /etc/apt/sources.list.d/docker.list`; `sudo apt-get update`;
   `sudo apt-get install -y docker-ce`. **Exactly 6 calls.** Assert argv, not just "no error".

Plus **one** short negative test `TestRepoBackend_Install_NotApplicable`: a package carrying only `dnf_repo` on a
Debian/apt host returns `*UnsupportedPlatformError`, and the mirror (only `apt_repo` on a Fedora/dnf host). Nothing
else — no speculative cases, no yum case, no `dnfMajor` unit test.

`t.Parallel()` on the outer function and every subtest, matching `apt_repo_test.go`'s style.

Convert no manifest entry to or from `custom-script`.

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && gofmt -l internal/backend && go build ./... && go vet ./internal/backend/... \
  && go test -race ./internal/backend/... ./cmd/dotf/... \
  && go test -race -run 'AptRepo|TestRepoBackend' -v ./internal/backend/ \
  && golangci-lint run ./internal/backend/...
```

---

## Batch 8 — Privileged firewalld probe and the Fedora manifest data

### WS6-T6 — `sudo -n` tri-state firewalld ssh probe + both `*PlanNeedsSudo` flips Depends on [WS6-T5]

**READ THESE BEFORE TASK**

- `/internal/sysconfig/linux_firewall.go` (`ufwStatus`, `planFirewalld`, `FirewallPlanNeedsSudo`)
- `/internal/sysconfig/linux.go` (`planSSHServerFirewallOpen`, `SSHServerPlanNeedsSudo`)
- `/internal/sysconfig/linux_test.go` (`:30-45`, `:475-504`, `~:536`, `~:575`, `~:615`, `~:740`)
- `/cmd/dotf/sysconfig.go` (`:76`)

**Files to Modify**

- `/internal/sysconfig/linux_firewall.go`, `/internal/sysconfig/linux.go`, `/internal/sysconfig/linux_test.go`

**Instructions**

`firewall-cmd --permanent --query-service=ssh` is polkit-gated on Fedora and exits non-zero **both** when ssh is not
allowed and when the caller lacks authorization — so today an unprivileged denial is misread as "no rule", and
`dotf sysconfig --firewall auto --yes` run twice never prints `sysconfig=converged`. Model it on `ufwStatus`'s
tri-state and run it under `sudo -n` (non-interactive: it never prompts, it fails closed).

**This is ONE atomic unit.** Landing the probe change without the sudo-priming flip leaves `--firewall auto` unable
to converge on Fedora; landing the flip without the new `SSHServerPlanNeedsSudo` test produces **zero** test churn
and therefore zero review signal (correction **C19**).

**(1) `linux_firewall.go`** — add next to `ufwStatus`:

```go
// firewalldSSHStatus reports whether firewalld permanently allows the ssh
// service, and whether that is KNOWN. `--permanent --query-service=ssh` is
// polkit-gated on Fedora and exits 1 both when the service is absent and when
// the caller is unauthorized, so the exit status alone is ambiguous: the
// verdict is read from stdout ("yes"/"no") under a non-interactive `sudo -n`,
// and anything else is UNKNOWN. A not-yet-installed firewalld definitionally
// has no rule, so it returns (false, true) without probing — same contract as
// ufwStatus.
func (c *linuxConfigurer) firewalldSSHStatus(installed bool) (allowed, known bool) {
	if !installed {
		return false, true
	}
	out, _ := c.r.Output("sudo", "-n", "firewall-cmd", "--permanent", "--query-service=ssh")
	switch strings.TrimSpace(string(out)) {
	case "yes":
		return true, true
	case "no":
		return false, true
	default:
		return false, false
	}
}
```

**(2)** In `planFirewalld`, replace the tail:

```go
	if _, err := c.r.Output("firewall-cmd", "--permanent", "--query-service=ssh"); err != nil {
		p.Steps = append(p.Steps, firewalldAllowSSHSteps("firewall")...)
	}
```

with:

```go
	allowed, known := c.firewalldSSHStatus(installed)
	if !known {
		// Rule state is unreadable without privilege on this read-only path.
		// Do not fabricate a missing rule — that would re-add + reload on
		// every run and break idempotency (same contract as planUfw).
		p.Skipped = append(p.Skipped, "firewall")
		return
	}
	if !allowed {
		p.Steps = append(p.Steps, firewalldAllowSSHSteps("firewall")...)
	}
```

**(3) `linux.go`**, `planSSHServerFirewallOpen`'s RedHat/SUSE arm — same tri-state, mirroring the ufw arm just below
it:

```go
	if info.OS == platform.OSRedHat || info.OS == platform.OSSUSE {
		_, lookErr := c.r.LookPath("firewall-cmd")
		allowed, known := c.firewalldSSHStatus(lookErr == nil)
		if !known || allowed {
			return false
		}
		p.Steps = append(p.Steps, firewalldAllowSSHSteps("ssh-server")...)
		return true
	}
```

**(4)** The probes are now privileged, so **flip both gates**. `FirewallPlanNeedsSudo` (in `linux_firewall.go`):

```go
// FirewallPlanNeedsSudo reports whether planning the firewall domain for mode on
// info issues a privileged read, so the command layer can prime sudo before
// Plan on the apply path. BOTH Linux backends do: ufw's `sudo -n ufw status`
// and firewalld's `sudo -n firewall-cmd --permanent --query-service=ssh` (which
// is polkit-gated on Fedora). The macOS Application Firewall reads its state
// unprivileged, and off/unset skip the domain entirely.
func FirewallPlanNeedsSudo(mode FirewallMode, info platform.Info) bool {
	if info.OS == platform.OSMacOS || info.OS == platform.OSWindows {
		return false
	}
	switch mode {
	case FirewallUfw, FirewallFirewalld, FirewallAuto:
		return true
	default:
		return false
	}
}
```

`SSHServerPlanNeedsSudo` (in `linux.go`):

```go
// SSHServerPlanNeedsSudo reports whether planning the ssh-server domain for mode
// on info issues a privileged read, so the command layer can prime sudo before
// Plan on the apply path. Every Linux family does: the ufw SSH-allow probe
// (`sudo -n ufw status`) and the firewalld one
// (`sudo -n firewall-cmd --permanent --query-service=ssh`) both need root.
func SSHServerPlanNeedsSudo(mode SSHServerMode, info platform.Info) bool {
	if info.OS == platform.OSMacOS || info.OS == platform.OSWindows {
		return false
	}
	return mode == SSHServerOn
}
```

Do **not** change `cmd/dotf/sysconfig.go:76` — its `FirewallPlanNeedsSudo(…) || SSHServerPlanNeedsSudo(…)` condition
already routes correctly.

**(5) MANDATORY test churn** in `linux_test.go`, all of it in this same commit:

- **(a)** `assertReadOnlyPlanCalls` (`~:30-45`): widen the allowlist and its doc comment to permit the second
  privileged READ. Replace the single
  `if len(call.Args) >= 3 && call.Args[0] == "-n" && call.Args[1] == "ufw" && call.Args[2] == "status" { continue }`
  with a check that also skips `sudo -n firewall-cmd --permanent --query-service=ssh`, and update the comment to name
  both. Do **not** weaken it further: any other `sudo` call recorded during Plan is still a failure.
- **(b)** Every registration of the old unprivileged probe becomes the `sudo -n` form with a `yes`/`no` **stdout**
  verdict: `~:536` in `seedLinuxSSHConverged` (verdict `yes`); `~:575` in
  `TestLinux_SSHServer_InstallArgvPerFamily` (verdict `no`); `~:615` in `TestLinux_SSHServer_ServiceUnitPerOS`
  (verdict `yes`); `~:740` in `TestLinux_SSHServer_FirewallOpenPresent`'s `redhat firewalld without ssh service` row
  (verdict `no`). Each becomes `fake.AddPath("firewall-cmd", "/usr/bin/firewall-cmd")` plus
  `fake.AddCommand("sudo", []string{"-n", "firewall-cmd", "--permanent", "--query-service=ssh"}, []byte("<verdict>\n"), nil)`.
- **(c)** `TestLinux_Firewall_Firewalld` (rewritten by WS6-T5): its bare `exec.NewFake()` means `firewall-cmd` is
  absent → `firewalldSSHStatus(false)` = `(false, true)` = known-no-rule → the 4-step expectation still holds.
  **Confirm; do not change.**
- **(d)** `TestFirewallPlanNeedsSudo` (`:475-504`): three rows flip, and rename the third so its name is not a lie —
  `:486` `{name: "auto on redhat resolves to firewalld", …, want: false}` → `want: true`, renamed
  `"auto on redhat resolves to firewalld (polkit-gated probe)"`; `:487` the OSSUSE twin, same shape;
  `:488` `{name: "firewalld explicit reads unprivileged", mode: FirewallFirewalld, os: OSDebian, want: false}` →
  `{name: "firewalld explicit needs sudo for the query-service probe", …, want: true}`. Leave the off/unset/macos/
  windows rows alone.
- **(e)** ONE new test immediately after `TestFirewallPlanNeedsSudo`. `SSHServerPlanNeedsSudo` has **zero** coverage
  today (verified: `rg SSHServerPlanNeedsSudo` hits only `linux.go` and `cmd/dotf/sysconfig.go:76`), so its flip
  would otherwise produce **no** intentionality signal at all — this is required, not speculative:

```go
func TestSSHServerPlanNeedsSudo(t *testing.T) {
	tests := []struct {
		name string
		mode SSHServerMode
		os   platform.OS
		want bool
	}{
		{name: "on debian probes ufw under sudo", mode: SSHServerOn, os: platform.OSDebian, want: true},
		{name: "on arch probes ufw under sudo", mode: SSHServerOn, os: platform.OSArch, want: true},
		{name: "on redhat probes firewalld under sudo", mode: SSHServerOn, os: platform.OSRedHat, want: true},
		{name: "on suse probes firewalld under sudo", mode: SSHServerOn, os: platform.OSSUSE, want: true},
		{name: "off skips", mode: SSHServerOff, os: platform.OSRedHat, want: false},
		{name: "unset skips", mode: SSHServerUnset, os: platform.OSRedHat, want: false},
		{name: "macos never", mode: SSHServerOn, os: platform.OSMacOS, want: false},
		{name: "windows never", mode: SSHServerOn, os: platform.OSWindows, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SSHServerPlanNeedsSudo(tt.mode, platform.Info{OS: tt.os}); got != tt.want {
				t.Errorf("SSHServerPlanNeedsSudo(%q, %q) = %v, want %v", tt.mode, tt.os, got, tt.want)
			}
		})
	}
}
```

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && gofmt -l internal/sysconfig && go build ./... && go vet ./... \
  && go test -race -v ./internal/sysconfig/... 2>&1 | grep -E '^(---|ok|FAIL)' \
  && go test -race ./... && golangci-lint run ./internal/sysconfig/...
```

**Recorded, not fixed here** (raise as follow-up): `cmd/dotf/apply.go`'s sysconfig phases never consult
`FirewallPlanNeedsSudo`/`SSHServerPlanNeedsSudo` — only `cmd/dotf/sysconfig.go:76` does. That is pre-existing, but
this flip widens its blast radius: `dotf apply --firewall auto` on Fedora now plans against an unprimed sudo and
defers the domain as UNKNOWN. WS8-T5 records it in ROADMAP.md's near-term list.

### WS7-MANIFEST — `docker-ce` becomes one `repo` entry; the `fingerprint` module gets an os gate Depends on [WS7-REPO, WS5-T6]

**Merged from WS7-T5 + WS7-T6.** Both write `manifest/packages.toml` and T6 depends on T5, so merging makes WS7 a
single `packages.toml` editor and reduces that file's writer chain to three across the whole plan.

**READ THESE BEFORE TASK**

- `/manifest/packages.toml` (**re-read immediately before editing** — WS2-CORE and WS5-T6 already changed it)
- `/internal/manifest/validate_repo.go`, `/internal/backend/repo_dnf.go`, `/internal/manifest/ratchet_test.go`
- `/internal/manifest/module.go` (`:15-28`, `:133-148`), `/internal/manifest/validate.go` (`:475-483`)
- `/internal/platform/platform.go` (`:225-240`), `/cmd/dotf/setup.go` (`:85-110`), `/cmd/dotf/setup_test.go` (`:16-18`)

**Files to Modify**

- `/manifest/packages.toml`, `/internal/manifest/module.go`, `/internal/platform/platform.go`, `/cmd/dotf/setup.go`

**Instructions**

You are the **third and final** writer of `manifest/packages.toml` and the **fourth and final** writer of
`internal/platform/platform.go`. Re-read both immediately before editing and **append** rather than rewriting
neighbouring blocks.

**PART 1 — `docker-ce` becomes one `repo` entry.** Rewrite the block at `~:241-253`. Re-head the group
`# --- repo: adds a third-party OS repository before installing ------------`, keep the existing apt rationale
paragraph about `{{.Codename}}`/`{{.Arch}}` templating and the Ubuntu dists path **verbatim**, and add prose stating
(a) one logical tool is one entry — the sub-table matching the host's package manager decides applicability, so the
entry is silently not-applicable on Arch, macOS and openSUSE instead of failing; and (b) the dnf side installs
docker's ready-made Fedora `.repo` at `/etc/yum.repos.d/docker-ce.repo` rather than calling `dnf config-manager`,
whose repo verbs and plugin package differ between dnf4 and dnf5. The entry becomes:

```toml
[[package]]
name = "docker-ce"
backend = "repo"
names = { apt = "docker-ce", dnf = "docker-ce" }
apt_repo = { key_url = "https://download.docker.com/linux/ubuntu/gpg", keyring = "/etc/apt/keyrings/docker.gpg", repo_line = "deb [arch={{.Arch}} signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu {{.Codename}} stable", sources_path = "/etc/apt/sources.list.d/docker.list" }
dnf_repo = { id = "docker-ce", repofile = "https://download.docker.com/linux/fedora/docker-ce.repo" }
```

The `apt_repo` line must be **byte-identical** to today's; only `backend`, `names` and the new `dnf_repo` line
change. Add **no** `os` gate — the sub-table pair **is** the gate.

Update the backend inventory comment (`~:16`) and the native-backend reminder (`~:279`) to list `repo` in place of
`apt-repo`, **without** writing a count (WS5-T6 already made the header list count-free).

**PART 2 — the module os gate.**

> **Correction C30.** The design's instruction assumed `manifest.Module` already has an `os` field. It does not
> (`internal/manifest/module.go:15-28`). This part adds the field, its validation, and the `dotf setup` consumer,
> plus a canonical `platform.GateAllows` so a third copy of the gate switch is not hand-rolled.

- `internal/platform/platform.go` — add directly after `IsLinuxFamily` (ends `:240`):

  ```go
  // GateAllows reports whether a manifest os token ("", "macos", "windows",
  // "linux", or a distro family) permits the host OS. An empty want matches every
  // host; "linux" matches every Linux-family host via IsLinuxFamily; every other
  // token matches its OS value exactly.
  //
  // It is the canonical form of the gate that internal/backend's osGateAllows and
  // internal/manifest's knownPackageOS are kept in lockstep with.
  func GateAllows(want string, host OS) bool {
  	switch want {
  	case "":
  		return true
  	case "macos":
  		return host == OSMacOS
  	case "windows":
  		return host == OSWindows
  	case "linux":
  		return IsLinuxFamily(host)
  	default:
  		return string(host) == want
  	}
  }
  ```

  Do **NOT** edit `internal/backend/backend.go`'s `osGateAllows` to delegate — that file is WS2-CORE's single-owner
  surface; the lockstep note in the doc comment above is the record.

- `internal/manifest/module.go` — add to `Module`, after `Description` (`:22`):

  ```go
  	// OS restricts this module to a platform, using the same token vocabulary as
  	// Package.OS ("macos", "windows", "linux", or a distro family). Empty means
  	// every platform. `dotf setup` refuses an off-platform module rather than
  	// running its steps.
  	OS string `toml:"os"`
  ```

  and in `validateModule` (`:133-148`), after the duplicate-name check (`:139-142`):

  ```go
  	if mod.OS != "" && !knownPackageOS[mod.OS] {
  		errs = append(errs, &ValidationError{Module: mod.Name, Msg: fmt.Sprintf("os %q must be one of: arch, debian, linux, macos, redhat, suse, windows", mod.OS)})
  	}
  ```

  (`knownPackageOS` is `validate.go:475-483`, same package; `fmt` is already imported.)

- `cmd/dotf/setup.go` — in `runSetup`, after `info := platform.Detect(r)` (`:92`) and **before** the
  `if o.dryRun {` branch (`:94`), so both dry-run and real runs refuse:

  ```go
  	if !platform.GateAllows(mod.OS, info.OS) {
  		_, _ = fmt.Fprintf(stderr, "%s: module %q requires os=%s; this host is os=%s\n", progName, mod.Name, mod.OS, info.OS)
  		return 1
  	}
  ```

  Exit 1 matches the sibling refusal at `:105`. `cmd/dotf/setup_test.go`'s fixture manifest (`:16-18`) declares no
  `os`, so every existing setup test is unaffected — verify that and add **no** new test.

- `manifest/packages.toml` — add `os = "arch"` to the `fingerprint` module block (`~:537-540`), between
  `description` and `steps`, with a one-line comment above the block: the PAM fingerprint setup targets the
  Arch/CachyOS host, and there is no supported Linux fingerprint stack on Apple Silicon. Leave the `onedrive` module
  ungated. Change nothing else in this file.

**HARD CONSTRAINT**: `grep -c '^backend = "custom-script"' manifest/packages.toml` must still print exactly **16**.

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && test "$(grep -c '^backend = \"custom-script\"' manifest/packages.toml)" = 16 \
  && gofmt -l internal/platform internal/manifest cmd/dotf && go build ./... && go vet ./... \
  && go test -race ./... \
  && golangci-lint run ./internal/manifest/... ./internal/platform/... ./cmd/dotf/... \
  && go run ./cmd/dotf status --counts | grep -E '^count\.backend\.(repo|custom-script)='
```

---

## Batch 9 — Documentation pass

Every WS8 task transcribes source files **verbatim**, so all of them run after WS1, WS-SHELL, WS6 and WS7 have
settled `internal/platform`, `common/platform.sh`, `cmd/dotf`'s flag surface, the manifest schema and the workflows.

### WS7-T7 — `manifest-backends.md`: rung 3 becomes the family-dispatching `repo` backend Depends on [WS7-MANIFEST, WS5-T7]

**READ THESE BEFORE TASK**

- `/docs/reference/manifest-backends.md` (**post-WS5-T7** — it already has a `font` rung and a count-free opening)
- `/internal/manifest/types.go`, `/internal/manifest/validate_repo.go`, `/manifest/packages.toml`

**Files to Modify**

- `/docs/reference/manifest-backends.md`

**Instructions**

> **Correction (critique).** The first-pass spec anchored on "`dotf` installs through **eight** backends" and on
> "rungs 4-8 keep their numbers". WS5-T7 (batch 6) already reworded the opening **count-free** and inserted a `font`
> rung at position 6, so those anchors no longer exist. Re-anchor on the **post-WS5-T7** file, insert `repo` by
> replacing the existing `apt-repo` rung **in place**, and renumber only relative to whatever rungs are present.
> **Write no backend count anywhere.**

**EDIT 1 — the `apt-repo` rung (currently rung 3).** Replace:

```
3. **`apt-repo`** — Debian/Ubuntu third-party APT repo (GPG key + `sources.list` line),
   then a normal package install.
```

with a `repo` rung carrying the sub-table decision ladder:

```
3. **`repo`** — the tool ships from a third-party OS repository. ONE entry per tool;
   add the sub-table(s) for the families you support and the backend dispatches on
   the host's package manager:
   - `apt_repo` — Debian/Ubuntu: GPG key + `sources.list` line, then a normal install.
     `repo_line` templates `{{.Codename}}` and `{{.Arch}}` from the host.
   - `dnf_repo` — Fedora/RHEL, in exactly one of three modes:
     - `repofile = "<https url>"` (+ `id`) — the vendor publishes a ready-made `.repo`;
       **prefer this.** It is installed verbatim at `/etc/yum.repos.d/<id>.repo`.
     - `baseurl = "…"` (+ `id`, optional `gpgkey`) — no vendor `.repo` exists; dotf renders
       the repo body. `baseurl`/`gpgkey` template `{{.ReleaseVer}}` (os-release `VERSION_ID`)
       and `{{.Arch}}`.
     - `copr = "owner/project"` — the tool exists only in COPR (see below).
   A host whose package manager matches no sub-table reports `applicable=false` — it is
   silent, not an error. `apt-repo` is the deprecated alias of this backend; do not use it
   for new entries.
```

Leave every other rung's number and text as WS5-T7 left them.

**EDIT 2 — worked examples.** In the fenced TOML block, replace/augment the `apt-repo` example with the live
docker-ce shape (both sub-tables under one name), numbered to match the ladder:

```toml
# 3. repo — one tool, one entry; the sub-table matching the host's package manager wins
[[package]]
name = "docker-ce"
backend = "repo"
names = { apt = "docker-ce", dnf = "docker-ce" }
apt_repo = { key_url = "…/gpg", keyring = "/etc/apt/keyrings/docker.gpg", repo_line = "deb [arch={{.Arch}} signed-by=/etc/apt/keyrings/docker.gpg] …/ubuntu {{.Codename}} stable", sources_path = "/etc/apt/sources.list.d/docker.list" }
dnf_repo = { id = "docker-ce", repofile = "https://download.docker.com/linux/fedora/docker-ce.repo" }
```

**EDIT 3 — a COPR subsection.** After the worked examples and before "The `custom-script` governance fence", add
`### A COPR-only tool` with the explicit answer WS7 owes: use `dnf_repo = { copr = "owner/project" }` **only** when
the tool has no upstream release asset — a `github-release` entry is lock-pinnable and independent of a third party's
build queue, so it wins whenever available (worked example: `lazygit` takes `github-release`, **not**
`copr:atim/lazygit`, whose newest build failed). Note that copr is the only `dnf_repo` mode that shells out to dnf,
and that dotf installs the right plugin package for the host's dnf major (`dnf5-plugins` vs `dnf-plugins-core`)
automatically — never run `dnf config-manager` by hand in a manifest.

**EDIT 4 — the governance fence section.** Add one sentence recording ADR-5's honest accounting: generalising
`apt-repo` into `repo` added **one enum value plus one deprecated alias**, and it costs **nothing** against
`customScriptBaseline` (still 16) because `repo` is native.

Do **not** touch `CLAUDE.md:411`, `README.md:44`, or `AGENTS.md` — those also enumerate the ladder and belong to
WS8-T4/WS8-T5. Keep markdownlint-clean.

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && npx --no-install markdownlint-cli2 docs/reference/manifest-backends.md \
  && ! grep -Ei 'eight backends|nine (declared )?backends' docs/reference/manifest-backends.md \
  && grep -q 'dnf_repo' docs/reference/manifest-backends.md
```

### WS8-T1 — Rewrite `docs/architecture/platform-detection.md` into two labelled halves Depends on [WS1-T2, WS-SHELL, WS6-T9, WS7-T1]

**READ THESE BEFORE TASK — in their post-batch-8 state**

- `/internal/platform/platform.go`, `/internal/platform/types.go`, `/internal/platform/osrelease.go`
- `/common/platform.sh`, `/cmd/dotf/main.go` (`:109`), `/internal/symlink/table.go` (`:143-150`)
- `/docs/fedora-first-run.md`, `/docs/plans/fedora-support/design.md`

**Files to Modify**

- `/docs/architecture/platform-detection.md`

**Instructions**

The file (399 lines) documents **only** the shell detector, and documents it **inaccurately**: it omits
`rocky`/`almalinux`, every Arch derivative (`cachyos`/`manjaro`/`endeavouros`), the entire `suse` arm
(`opensuse*`/`sles`), and the whole `ID_LIKE` fallback block. It never mentions the canonical Go detector, which is
what `dotf` actually uses. **Full-file rewrite, not a patch.**

**BEFORE WRITING**, re-read the four source files **as they exist after batch 8**. Do not copy snippets from this
plan or from `design.md` — transcribe from the actual files, because WS1-T2 (Asahi ID tokens), WS6-T9 (arm64-scoped
brew demotion, `detectPkgMgr(r, os, arch)`), WS7-T1 (`readOSRelease`, `Info.VersionID`) and WS-SHELL
(`os_module_dirs()`) all mutate them.

Required structure:

1. `# Platform Detection System` (keep the H1 verbatim — other docs link by path, and the filename must not change).
   Replace the current line-7 claim ("implemented in `common/platform.sh`") with the truth: there are **two**
   detectors in deliberate lockstep — the canonical Go detector in `internal/platform` (every `dotf` subcommand) and
   the shell mirror in `common/platform.sh` (bash/zsh startup). State that the Go one is canonical, that its doc
   comments assert the mirroring, and that an edit to one **requires** the mirrored edit to the other.
2. `## Part 1 — Canonical Go detector (internal/platform)`. Tables, not code dumps, for: `Detect(r exec.Runner) Info`
   and every `Info` field it populates (include `VersionID` — WS7-T1 added it); `detectOS` (`uname -s` → Darwin /
   Linux / CYGWIN|MINGW|MSYS / default, plus the `runtime.GOOS` fallback in `unameS`); `detectLinuxFamily`
   (os-release present → ID/ID_LIKE; absent → `/proc/version` contains "microsoft" → `OSWSL`; else `OSLinux`);
   `osFamilyFromID` as a table with **every** accepted ID literal per family — Debian: ubuntu, debian · RedHat:
   fedora, **fedora-asahi-remix, asahi-fedora-remix**, rhel, centos, rocky, almalinux · Arch: arch, cachyos, manjaro,
   endeavouros · SUSE: sles plus any ID with the `opensuse` prefix; the `ID_LIKE` fallback as a second table, stating
   explicitly that ID is checked first; `detectArch`/`archFromUname`; `detectPkgMgr` with its **post-WS6-T9** probe
   ordering (macOS and x86_64 Linux probe brew first; aarch64 Linux probes the native manager first, because Homebrew
   publishes no aarch64-Linux bottles), plus the source's own note that the shell uses `command -v` while Go uses
   `LookPath`, so a brew shell function with no PATH binary diverges; `detectContainer`.
3. `### Two Linux-family predicates` — `platform.IsLinuxFamily` **includes** `OSLinux` while `internal/symlink`'s
   private `isLinuxFamily` (`table.go:143-150`) deliberately **excludes** it; the divergence is intentional, pinned by
   a golden test row, and the two are not interchangeable. Name `FacetLinux` as the consumer that must use the local
   predicate.
4. `### dotf platform output contract` — the exact key=value line from `cmd/dotf/main.go:109`, with a Fedora example.
   Note that `Codename` and `VersionID` are deliberately **not** printed, and that
   `cmd/dotf/main_test.go:119` plus critical test #6 both anchor the shape.
5. `## Part 2 — Shell mirror (common/platform.sh)`. Keep the useful existing material but **correct** it: replace the
   stale `detect_os()` code block (currently `:63-92`, whose arms read `fedora|rhel|centos`, `arch`, `*) echo "linux"`)
   with the real post-WS1-T2 body including the `ID_LIKE` fallback; replace the "Supported Operating Systems" table
   (`:96-105`) so the `redhat` row lists Fedora (incl. Fedora Asahi Remix), RHEL, CentOS, Rocky, AlmaLinux, the `arch`
   row lists Arch/CachyOS/Manjaro/EndeavourOS, and a missing **`suse`** row (openSUSE, SLES) is added, with a
   footnote for the ID_LIKE path. **Document `os_module_dirs()`** (WS-SHELL / ADR-14) as the per-OS module search
   path, and note that `detect_package_manager` still prioritises brew unconditionally — a deliberate, recorded
   divergence from the Go side after WS6-T9. Keep `get_distro_name()`, `detect_arch()`, `detect_container()`,
   `get_homebrew_path()`, the predicate tables, SSH + OSC52 detection, PATH helpers, exported `DOTFILES_*` variables,
   XDG setup, `DOTFILES_NO_AUTO_SETUP` and the debugging section — verify each against the file first.
   Update the mermaid flowchart (`:11-57`) so its Linux branch shows the rocky/almalinux, Arch-derivative and suse
   arms and an explicit ID_LIKE fallback node.
6. `## Fedora family (including Fedora Asahi Remix)` — every candidate os-release `ID` resolves to `redhat` /
   `platform.OSRedHat`, either through the explicit ID case or the `ID_LIKE=fedora` fallback; `pkg` is `dnf` (which
   **is** dnf5 on Fedora 41+, and the install/upgrade/remove/`rpm -q` argv are source-compatible across dnf4 and dnf5
   — **do not "fix" them**); `uname -m` is `aarch64` → arch `arm64` → release asset `dotf_linux_arm64.tar.gz`.
   Homebrew publishes no aarch64-Linux bottles, which is why `detectPkgMgr` demotes brew there. Cite
   `docs/fedora-first-run.md` as the provenance for device-captured os-release values — read it and do **not** restate
   an ID as fact if that file records a different one.
7. `## See Also` — keep the links to overview.md and common-system.md, replace the bare `/common/platform.sh` bullet
   with links to **both** source files, and add `docs/development/testing.md` and `docs/fedora-first-run.md`.

**CONSTRAINTS**: markdownlint bans hard tabs (MD010), requires blank lines around headings/fences/lists, a single
trailing newline, `_underscore_` italics and `**asterisk**` bold. MD013 is off. Keep the file **at or under 520
lines** — if Part 1 pushes it over, trim Part 2's verbose "Common Patterns"/"Usage Examples" prose (which duplicates
`docs/reference/aliases.md` and `functions.md`) rather than splitting the file; splitting would break the
`docs/README.md` tree and inbound links.

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && npx --no-install markdownlint-cli2 docs/architecture/platform-detection.md \
  && npx --no-install prettier --check docs/architecture/platform-detection.md \
  && for s in internal/platform almalinux endeavouros ID_LIKE fedora-asahi-remix opensuse os_module_dirs VersionID; do \
       grep -q "$s" docs/architecture/platform-detection.md || { echo "FAIL: missing $s"; exit 1; }; done \
  && test "$(wc -l < docs/architecture/platform-detection.md)" -le 520 \
  && echo WS8-T1-OK
```

### WS8-T2 — Create `docs/development/testing.md` and repair the docs index Depends on [WS1-T5, WS4-T1, WS4-T2]

**READ THESE BEFORE TASK**

- `/docs/README.md` (`:12`, `:44-50`, `:140-143`, `:146-155`), `/monitor-auto-switch/testing.md`
- `/docs/fedora-first-run.md`, `/docs/fedora-rehearsal.md`, `/package.json`
- `/.github/workflows/go.yml`, `/.forgejo/workflows/go.yml`, `/.github/workflows/shell.yml`

**Files to Create**

- `/docs/development/testing.md` (create the directory too — it does not exist)

**Files to Modify**

- `/docs/README.md`

**Instructions — PART A: create the guide**

`docs/README.md` has advertised a Development section (`:12`), three docs (`:44-50`) and a file tree (`:140-143`)
through two feature cycles; **none of those files exists**. This lands the one the design commits to. Use
`monitor-auto-switch/testing.md` as the house precedent for tone and structure.

1. `# Testing Guide` + one paragraph: verification is three-tiered — automated CI, local container rehearsal, and
   physical-device acceptance — and the third tier is a **gate**, not a post-hoc report.
2. `## Tier 1 — Automated (every PR)`. Transcribe the **real** commands from `package.json` and the workflows.
   **There is no `npm test` script and no `npm run build` script** — do not prescribe them. Go: `go build ./...`,
   `go vet ./...`, `go test -race ./...`, `golangci-lint run ./...` (read `.github/workflows/go.yml` for the pinned
   version). Lint bundle: `npm run lint` (= `bin/lint.sh --python --shell --go`), plus the git-scope flags
   `--modified` / `--staged` / `--unstaged`. Markdown: `npm run lint:md` / `npm run format:md`, stating that
   `--docs` is **opt-in**, excluded from the no-flag default and from `--all`, so `npm run lint` stays green against
   the existing backlog. Shell smoke: `bash -l -c …`, `zsh -l -c …`, `dotf link --dry-run`. Then list each CI job
   with its path filters, from **both** `.github/` and `.forgejo/`, and record that the Forgejo twin cannot use
   third-party actions (only `actions/*` and `forgejo/*` resolve from its `DEFAULT_ACTIONS_URL`), which is why it
   installs golangci-lint via raw shell. Record the Fedora container job and what its smoke step asserts
   (`dotf platform` contains `os=redhat` and `pkg=dnf`; `dotf apply --dry-run --headless` and `dotf link --dry-run`
   exit 0), noting it is the only thing in the repo that ever executes against a real Fedora userspace — every other
   Fedora assertion runs against `exec.Fake`. Record the shellcheck workflow and its one known gap: `bin/lint.sh
   --shell` lints only `*.sh`, so `bash/bashrc`, `zsh/zshrc`, `zsh/zprofile` and `bash/profile` stay unlinted.
3. `## Tier 2 — Local container rehearsal (pre-PR)`. Do **not** duplicate `docs/fedora-rehearsal.md` — summarise in
   three sentences and link it. Record what is only provable here: a privileged systemd Fedora container for
   `dotf sysconfig` reaching `failed=0`, and an emulated linux/arm64 `fedora:latest` for a real multi-package dnf
   transaction and `rpm -q`. State plainly that arm64 **plus** systemd is unreachable under `qemu-user`
   (`dbus-broker` dies `226/NAMESPACE`).
4. `## Tier 3 — Physical-device acceptance`. Do **not** duplicate the checklist — state that
   `docs/fedora-first-run.md` is its permanent home and that it is walked end to end **before** a release tag is cut.
   Reproduce design §9's "provable only on the physical M1" table as a compact list with one-line reasons. Include the
   two standing DO-NOT-USE warnings: `--hyprland`/`--linux-desktop` (deploys the Hyprland/wlroots set, categorically
   wrong on a KDE host) and `--snapper` (`cmd/dotf/applyphases.go:23-26` states failures are **never** recorded in the
   failure collector, so the run exits 0 having protected nothing).
5. `## Adding a test`. Two rules only: (a) prefer a row in an existing table-driven test over a new test file;
   (b) a golden/lock table changing **is** the intentionality signal — never "fix" a golden by loosening the
   assertion. Name the live tripwires: `internal/manifest/ratchet_test.go`'s `customScriptBaseline` (exact-match in
   **both** directions, currently 16); `internal/selfupdate/selfupdate_test.go`'s `TestAssetNaming_MatchesBootstrap`
   (string-matches literals in `bootstrap.sh`); `internal/symlink/table_test.go`'s four golden locks.
6. `## See Also` — `../README.md`, `../architecture/platform-detection.md`, `../fedora-first-run.md`,
   `../fedora-rehearsal.md`, `../reference/manifest-backends.md`.

Target 150–250 lines.

**Instructions — PART B: repair `docs/README.md`** so the index stops advertising files that do not exist.

- `:12` — change the Development row's description to "Testing tiers and verification workflow"; the
  `[Development](development/)` link now resolves.
- `:46-50` — delete the **Contributing** and **Adding Tools** bullets (neither file exists) and replace the Testing
  bullet with `- [**Testing**](development/testing.md) - CI, local container rehearsal, and device acceptance tiers`.
- `:140-143` — replace the three-file `development/` tree with a single `└── development/` /
  `    └── testing.md               # Testing tiers and verification`.
- Add `fedora-first-run.md` and `fedora-rehearsal.md` (both at `docs/` root) to the tree and/or the Related
  Documentation list (`:146-151`).
- Update `Last updated:` (`:155`) to today's date in the existing `YYYY-MM-DD` format.
- Do **not** add a ROADMAP.md link here — WS8-T5 owns that link and places it in the root README.

**CONSTRAINTS**: `docs/README.md` currently **fails** `prettier --check` (pre-existing) — do not gate on prettier for
it and do not reformat it wholesale; make surgical edits only. `docs/development/testing.md` is new and must pass
**both** markdownlint and prettier.

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && test -f docs/development/testing.md \
  && npx --no-install markdownlint-cli2 docs/development/testing.md docs/README.md \
  && npx --no-install prettier --check docs/development/testing.md \
  && for s in fedora-first-run.md fedora-rehearsal.md customScriptBaseline 'npm run go:build' 'npm run lint'; do \
       grep -qF "$s" docs/development/testing.md || { echo "FAIL: missing $s"; exit 1; }; done \
  && grep -q 'development/testing.md' docs/README.md \
  && ! grep -q 'contributing.md' docs/README.md \
  && ! grep -q 'adding-tools.md' docs/README.md \
  && echo WS8-T2-OK
```

> **Minor fix applied (critique).** The first-pass verify asserted `! grep -q 'npm test'` over the whole file, which
> would reject correct prose such as "there is no `npm test` script; use `npm run go:test`". The assertion above is
> positive-only (`npm run go:build`, `npm run lint` present) and no longer punishes an accurate sentence.

### WS8-T3 — Fedora `dnf` install blocks in the feature guides Depends on [WS2-CORE, WS2-T5]

**READ THESE BEFORE TASK**

- `/docs/features/modern-tools.md`, `/docs/features/environment-management.md`
- `/.docs/fedora-package-mapping.md` (**the verified source of truth once WS2-T5 lands — read it and use its names
  if they differ from those below**)

**Files to Modify**

- `/docs/features/modern-tools.md`, `/docs/features/environment-management.md`

**Instructions**

Every `### Installation` fence in these two guides shows a `# macOS` / `# Ubuntu/Debian` / `# Arch Linux` triple with
no Fedora arm. In **every** case insert the Fedora block **after** `# Ubuntu/Debian` and **before** `# Arch Linux`,
separated by one blank line, matching the existing spacing exactly. Do not reflow or reindent anything else.

`docs/features/modern-tools.md` — **7** insertions:

| Tool    | Fence     | Insert                                                                                              |
| ------- | --------- | --------------------------------------------------------------------------------------------------- |
| eza     | `:27-37`  | `# Fedora` / `dnf install eza`                                                                      |
| bat     | `:92-102` | `# Fedora (ships /usr/bin/bat — no rename shim needed)` / `dnf install bat`                         |
| ripgrep | `:158-167`| `# Fedora` / `dnf install ripgrep`                                                                  |
| fd      | `:238-248`| `# Fedora (fd-find ships /usr/bin/fd — no rename shim needed)` / `dnf install fd-find`               |
| fzf     | `:317-327`| `# Fedora` / `dnf install fzf`                                                                      |
| delta   | `:380-389`| `# Fedora (the RPM is git-delta; an unrelated package is named 'delta' — do not shorten)` / `dnf install git-delta` |
| zoxide  | `:442-451`| `# Fedora` / `dnf install zoxide`                                                                   |

`docs/features/environment-management.md` — **2** insertions:

- direnv (`:18-28`): `# Fedora` / `dnf install direnv`.
- atuin (`:252-262`): this fence has **no** Ubuntu/Arch pair — its arms are `# macOS` + `brew install atuin`,
  `# Using installer script` + `./bin/install-atuin.sh`, `# Or manual installation` + `curl …`. Insert
  `# Fedora` / `dnf install atuin` immediately after `brew install atuin` and its blank separator. (Fedora ships an
  official `atuin`; the manifest still installs it via the vendor script, so leave the installer-script arm in place.)

Do **not** change `modern-tools.md`'s `## Tool Summary` table (`:9-20`) — it has no per-distro column. Change nothing
else in either file. All nine `# Fedora` comments start at column 0 inside their fence, matching the existing
`# macOS` / `# Arch Linux` comments.

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && npx --no-install markdownlint-cli2 docs/features/modern-tools.md docs/features/environment-management.md \
  && npx --no-install prettier --check docs/features/modern-tools.md docs/features/environment-management.md \
  && test "$(grep -c '^# Fedora' docs/features/modern-tools.md)" -eq 7 \
  && test "$(grep -c '^# Fedora' docs/features/environment-management.md)" -eq 2 \
  && grep -q 'dnf install fd-find' docs/features/modern-tools.md \
  && grep -q 'dnf install git-delta' docs/features/modern-tools.md \
  && grep -q 'dnf install direnv' docs/features/environment-management.md \
  && echo WS8-T3-OK
```

### WS8-T4 — Correct `CLAUDE.md` and `AGENTS.md`: the stale line count and the non-existent npm scripts Depends on [WS1-T1]

**READ THESE BEFORE TASK**

- `/CLAUDE.md` (`:28`, `:458`, `:734-761`, `:778`), `/AGENTS.md` (`:22`, `:28-31`), `/bootstrap.sh`, `/package.json`

**Files to Modify**

- `/CLAUDE.md`, `/AGENTS.md`

**Instructions**

**PART A — the `bootstrap.sh` line count (correction C37).** The literal `(~307 lines)` appears **four** times and is
wrong: `bootstrap.sh` was 593 lines before this branch and WS1-T1 added to it. **Compute the real count first** —
`N=$(wc -l < bootstrap.sh)` — then replace every occurrence of the exact substring `(~307 lines)` with
`(~${N} lines)`, substituting the literal number. The four sites are `CLAUDE.md:28`, `CLAUDE.md:458`,
`CLAUDE.md:778`, `AGENTS.md:22`. A repo-wide replace-all of that substring in these two files is safe: it occurs
nowhere else in either, and nowhere else in the repo except `docs/plans/fedora-support/` — **do not edit the plan or
design docs**.

Note on `AGENTS.md`: its own Sync policy (`:28-31`) says CLAUDE.md is the source of truth and this file should not be
edited except for agent-runtime notes. Correcting a duplicated factual error is in scope and explicitly mandated by
the design's WS8 and its closing Advice; do not restructure `AGENTS.md` otherwise.

**PART B — the Testing & Verification block in `CLAUDE.md`** (`:734+`). It opens:

````
Run the following before marking any task complete:

```bash
npm test
npm run lint
npm run build
```
````

`package.json` defines **no** `test` script and **no** `build` script (verified: the scripts map has smartsheet,
install-deps, lint\*, format\*, go:test, go:vet, go:build), so both commands fail. Replace that **first** fenced block
with:

````
```bash
npm run lint          # shell + python + go
npm run lint:md       # markdown (opt-in; excluded from the default lint)
npm run go:build      # go build -o dotf ./cmd/dotf
```
````

Leave the two following fenced blocks in that section (the shell/symlink block at `:745-749` and the Go block at
`:753-758`) **unchanged** — they are already correct — and leave the closing sentence at `:761` unchanged. Add one
sentence after the new block:
`See [docs/development/testing.md](docs/development/testing.md) for the three verification tiers (CI, local container rehearsal, device acceptance).`

**CONSTRAINTS**: `CLAUDE.md` currently **fails** `prettier --check` (pre-existing) — do **not** run
`prettier --write` on it and do not gate on prettier. Both files currently **pass** markdownlint; keep them passing.
Surgical edits only.

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && N="$(wc -l < bootstrap.sh)" \
  && ! grep -q '307 lines' CLAUDE.md AGENTS.md \
  && test "$(grep -cF "(~${N} lines)" CLAUDE.md)" -eq 3 \
  && test "$(grep -cF "(~${N} lines)" AGENTS.md)" -eq 1 \
  && ! grep -qx 'npm test' CLAUDE.md && ! grep -qx 'npm run build' CLAUDE.md \
  && grep -q 'npm run go:build' CLAUDE.md \
  && grep -q 'docs/development/testing.md' CLAUDE.md \
  && npx --no-install markdownlint-cli2 CLAUDE.md AGENTS.md \
  && echo WS8-T4-OK
```

---

## Batch 10 — Roadmap

### WS8-T5 — Create `/ROADMAP.md` and link it from `README.md`; correct the desktop-flag section Depends on [WS7-T7, WS3-FACETS, all code batches]

**READ THESE BEFORE TASK**

- `/README.md` (`:383-397`, `:518`), `/docs/plans/fedora-support/design.md` (§1 Non-Goals, §5, §8)
- `/docs/reference/manifest-backends.md` (post-WS7-T7), `/internal/manifest/ratchet_test.go`
- `/cmd/dotf/link.go`, `/cmd/dotf/applyopts.go` (post-WS3-FACETS), `/internal/backend/register.go`
- `/bin/standardize-md-names.sh` (`:33` — `ROADMAP` is already allowlisted as an uppercase root doc)

**Files to Create**

- `/ROADMAP.md`

**Files to Modify**

- `/README.md`

**Instructions — PART A: create `/ROADMAP.md`** (target 150–220 lines).

> **Recorded override (C31).** design §1's Non-Goals table lists `ROADMAP.md` as out of scope ("Worth doing; not on
> the first-boot path"). The orchestrator brief mandates it. This is a deliberate override, not an accident.

Source content from `design.md` §1 Non-Goals, §5 ADRs and §8 risks. **Every deferred item carries the design's own
reason**, not an invented one.

1. `# Roadmap` — one paragraph: what this repo is and the supported platform matrix after this change (macOS,
   Debian/Ubuntu, RedHat/Fedora incl. Fedora Asahi Remix, Arch family, openSUSE, WSL; amd64 and arm64). One sentence
   saying this file records direction only, and that authoritative per-feature specs live under `docs/plans/`.
2. `## Shipped` — Phases 4–7 (native `dotf link`; `dotf sysconfig`; `dotf apply`; the `bootstrap.sh` shim +
   GoReleaser pipeline), `dotf plasma`, then Fedora first-class support with one line per workstream that actually
   landed — **read the working tree to confirm each before claiming it**: binary-download fallback + Asahi ID tokens
   (WS1); `pkg` applicability via `names` + the lazygit/yazi github-release siblings + `lutris-pkg` (WS2); symlink
   backup safety for directory destinations + capability-named facets and the `--hyprland` flag (WS3); **the per-OS
   shell module loader fixed as a search path, plus the `history`/firewalld/`systemsettings` regressions and the
   locale codeset match (WS-SHELL / ADR-14)**; Fedora container CI + shellcheck CI (WS4); Plasma/desktop fixes + the
   native `font` backend (WS5); sysconfig Fedora domains — langpacks, firewalld install, chsh guard, `pkgInstallArgv`
   (WS6); the family-dispatching `repo` backend + docker-ce on Fedora (WS7).
3. `## Near-term` — worth doing, off the first-boot path:
   - `CHANGELOG.md` backfill and the full multi-file documentation drift pass.
   - Ratchet-**down** candidates for the `custom-script` budget: `atuin` and `azure-cli` both have official Fedora
     packages. State the rule plainly: `customScriptBaseline` (`internal/manifest/ratchet_test.go`) is an exact-match
     tripwire in **both** directions, currently **16** — moving an entry off `custom-script` requires lowering the
     baseline in the same commit.
   - openSUSE third-party repos as a `zypper_repo` sub-table on the existing `repo` backend — explicitly **not** a
     new backend (ADR-5).
   - Retiring or wiring the orphaned `bin/install-*.sh` scripts, unreachable from `bootstrap.sh → dotf apply`.
   - Wiring the `sysconfig` `[[module]]` step kind and its `ports` list (`dotf setup` is not reachable from the
     bootstrap pipeline).
   - Threading `privilege.Sudo` into the backends — `internal/backend/register.go:15` is literally `_ = sudo`; new
     code follows the literal `"sudo"` argv convention so a future fix stays one refactor.
   - A dependency graph on `Package`: the npm/cargo toolchain entries (`codex`, `lefthook`, `tmux-sessionizer`,
     `fabric`) have no provisioned node/cargo/Go on **any** distro — a pre-existing cross-platform hole, not a Fedora
     one.
   - **Widening `bin/style.sh`'s shell file set** so extension-less shell files (`bash/bashrc`, `zsh/zshrc`,
     `zsh/zprofile`, `bash/profile`) are linted by CI and the pre-commit hook.
   - **Priming sudo in `dotf apply`'s sysconfig phases.** `cmd/dotf/apply.go` never consults
     `FirewallPlanNeedsSudo`/`SSHServerPlanNeedsSudo` — only `cmd/dotf/sysconfig.go:76` does. Pre-existing, but WS6-T6
     widened its blast radius: `dotf apply --firewall auto` on Fedora now plans against an unprimed sudo and defers
     the domain as UNKNOWN.
   - **Reconciling `common/platform.sh:detect_package_manager` with the Go detector**, which after WS6-T9 demotes
     brew below the native manager on **aarch64** Linux; the shell side still prioritises brew unconditionally.
4. `## Explicitly deferred` — a two-column table (Item | Why deferred) transcribing design §1's Non-Goals in
   substance: a distinct `platform.OSFedora`/`OSAsahi`; rpm-ostree / Silverblue / Kinoite / Bazzite; implementing
   **or** removing `platform.PkgMgrYUM`; a `platform.Info` capability table (`Caps`); aarch64 CI and an arm64+systemd
   emulated environment (ADR-15 — hosted arm64 runners are paid on a private repo, and a self-hosted runner on the
   target laptop would invert the safety property); porting `bin/setup-fingerprint.sh` to Fedora/authselect (Touch ID
   sits in Asahi's "TBA — no active work" tier); RPM Fusion enablement; unifying `internal/symlink`'s private
   `isLinuxFamily` with `platform.IsLinuxFamily` (the `OSLinux` exclusion is deliberate and pinned by a golden test
   row); SELinux `semanage port` labelling; **and the Fedora `/etc/bashrc` severance** — Fedora's bash is not built
   with `-DSYS_BASHRC`, so replacing `~/.bashrc` severs `/etc/profile.d` for the interactive non-login shell a
   Konsole tab starts. WS-SHELL deliberately did not change the login path; adding a guarded source is a behaviour
   change on four platforms and needs its own review.
5. `## Known risks carried forward` — three entries with mitigations, from design §8: **R3** Homebrew on arm64 Linux
   (no aarch64 bottles; `--homebrew=true` is now declined there with an explicit stderr note, and `detectPkgMgr`
   demotes brew on aarch64 Linux only, so an x86_64 `--homebrew=true` still works as documented); **R6** rpm-ostree
   variants (`/usr/bin/dnf` absent by design → `PkgMgrUnknown` → a wall of `UnsupportedPkgMgrError`; the canonical
   detection signal if it ever enters scope is `/run/ostree-booted`, and the right behaviour is to skip the packages
   phase with one message, not abort); **R7** snapper giving false rollback confidence on Fedora (no snapper, no
   config, no `-c` argument, and snapshot failures are never recorded in the failure collector, so `--snapper` exits
   0 having protected nothing).
6. `## Proposing a change` — point at `.github/ISSUE_TEMPLATE/` (bug_report.yml, feature_request.yml,
   compatibility_report.yml), the colon-prefixed label taxonomy, and `docs/plans/` as the home for design docs.
7. Footer `Last updated: <YYYY-MM-DD>`.

**Instructions — PART B: two surgical edits to `/README.md`** (541 lines).

1. Insert a new section immediately **before** `## 🤝 Contributing` (`:518`):

   ```markdown
   ## 🗺️ Roadmap

   See [ROADMAP.md](ROADMAP.md) for what has shipped, what is planned, and what is explicitly deferred (with reasons).
   ```

2. Correct `### Optional: Enable Generic Linux Desktop Configs` (`:383-397`), whose body says "For CachyOS or another
   Arch-family Hyprland install…". **Read `cmd/dotf/link.go` and `cmd/dotf/applyopts.go` first** to learn the exact
   final flag surface after WS3-FACETS (`--hyprland` primary, `--linux-desktop` retained alias) and document the
   **real** one. Rename the heading to name Hyprland explicitly (e.g. `### Optional: Enable the Hyprland desktop config set`)
   and add one **bold** warning sentence: this set is the Hyprland/wlroots stack and is categorically wrong on a KDE
   Plasma host (including Fedora KDE / Fedora Asahi Remix) — do not pass it there. Keep the existing
   dry-run-then-backup command pair and the closing sentence naming `~/.config/hypr`, `~/.config/waybar`,
   `~/.config/fuzzel`.

**CONSTRAINTS**: `README.md` currently **fails** `prettier --check` (pre-existing) — do not run `prettier --write` on
it and do not gate on prettier for it; it currently **passes** markdownlint, keep it passing. `ROADMAP.md` is new and
must pass both. No hard tabs; `_underscore_` italics, `**asterisk**` bold; exactly one trailing newline.

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && test -f ROADMAP.md \
  && npx --no-install markdownlint-cli2 ROADMAP.md README.md \
  && npx --no-install prettier --check ROADMAP.md \
  && for s in customScriptBaseline ostree-booted OSFedora etc/bashrc os_module_dirs; do \
       grep -qF "$s" ROADMAP.md || { echo "FAIL: missing $s"; exit 1; }; done \
  && grep -q '(ROADMAP.md)' README.md \
  && grep -qi 'hyprland' README.md \
  && test "$(grep -c 'ROADMAP' bin/standardize-md-names.sh)" -ge 1 \
  && echo WS8-T5-OK
```

---

## Batch 11 — Release-notes gate

### WS8-T6 — `docs/releases/v0.3.0-beta.md` Depends on [WS8-T5, all code batches]

**READ THESE BEFORE TASK**

- `/docs/releases/v0.2.3-beta.md` and `/docs/releases/v0.2.2-beta.md` (the structural models)
- `/.github/workflows/release.yml` (`:34`), `/.forgejo/workflows/release.yml` (`:54`), `/.goreleaser.yml`
- `/bootstrap.sh`, `/ROADMAP.md`

**Files to Create**

- `/docs/releases/v0.3.0-beta.md`

**Instructions**

**WHY THIS FILE IS MANDATORY**: `.github/workflows/release.yml:34` passes
`--release-notes=docs/releases/${{ github.ref_name }}.md` to GoReleaser, and `.forgejo/workflows/release.yml:54` does
the same. Both fail loudly if the file is absent, so **the tag cannot be cut without it.**

**TAG**: `v0.3.0-beta`. Existing tags are v0.1.0-beta … v0.2.3-beta (confirm with `git tag --sort=-v:refname`), and
Fedora support is a `feat` (minor bump). If the release owner picks a different tag, **rename the file to match
`docs/releases/<tag>.md` exactly** — the filename must equal the git tag.

**MODEL**: copy the structure of `docs/releases/v0.2.3-beta.md` — H1 `# v0.3.0-beta`, an intro paragraph, a
`> **Beta scope.**` blockquote, `## Highlights`, a maintenance/changes section, `## Install`, and a
`**Full changelog:**` compare link. Read `v0.2.2-beta.md` too for a second sample of the section vocabulary.

**CONTENT** — write from what actually landed; verify each claim against the working tree before asserting it.

- Intro: this release brings Fedora (including Fedora Asahi Remix on Apple Silicon) to first-class parity with the
  existing macOS / Arch / Debian support, through the identical `bootstrap.sh` → `dotf apply` pipeline — no bespoke
  side path.
- `## Highlights`:
  - **Fedora / RedHat family support.** Detection covers `fedora`, `fedora-asahi-remix`, `asahi-fedora-remix`,
    `rhel`, `centos`, `rocky`, `almalinux`; `pkg=dnf` argv are dnf5-compatible.
  - **Binary download fixed for prerelease-only origins.** `bootstrap.sh` now tries the tag-pinned
    `releases/download/<tag>/<asset>` URL it just resolved before giving up. State plainly that the dominant cause was
    `.goreleaser.yml`'s `prerelease: auto` combined with `-beta` tags — GitHub's `/releases/latest` excludes
    prereleases, so `/releases/latest/download` 404s on a **public** repo too. **Do not** attribute it to repository
    visibility.
  - **Shell configuration now actually loads on every distro.** The per-OS module loader was dispatching on
    `common/$DOTFILES_OS/…`, a directory that exists for no named distro family, so ~495 lines of
    `common/linux/aliases.sh` were dead on Fedora, Debian, Arch **and** openSUSE. It is now a family search path
    (`os_module_dirs()`), with the regressions that go live fixed in the same change.
  - **Symlink backup safety (behaviour change).** `dotf link --backup` now backs up **directory** destinations
    (`~/.config/nvim`, `wezterm`, `ghostty`, `mise`, `alacritty`, `~/.bin`, `~/.config/direnv`) before replacing them;
    previously these were removed recursively even with `--backup` passed. Backup directories will grow.
  - **Facet rename.** `--hyprland` is the new primary flag for the Hyprland/wlroots desktop set;
    `--linux-desktop`/`LINUX_DESKTOP` remain as back-compat aliases. Do not use it on a KDE host.
  - **Package manifest.** The fictional `dnf` names for `lazygit` and `yazi` are gone, replaced by `os = "redhat"`
    github-release siblings; `lutris` gains a Fedora `pkg` sibling; `docker-ce` installs on Fedora via the vendor
    `.repo` through the new family-dispatching `repo` backend.
  - **Nerd Fonts** now install through a native `font` backend on every platform (previously only via an
    Arch-only script referenced nowhere in the manifest).
- `## Breaking / behaviour changes` — surface each user-visible default change exactly once (do not duplicate from
  Highlights):
  - directory destinations are now backed up (see above);
  - `--linux-desktop` is deprecated in favour of `--hyprland`;
  - **`dotf plasma` no longer manages the default browser.** `text/html` and the `about`/`http`/`https`/`unknown`
    URL-scheme handlers are now stripped as host-local, on **every** platform including Arch — the pinned
    `google-chrome.desktop` has no aarch64 Linux build. The AI-app scheme handlers stay managed;
  - `--homebrew=true` is declined on aarch64 Linux with an explicit stderr note, and on aarch64 Linux the native
    package manager now wins over an incidental Linuxbrew.
- `## Install` — adapt `v0.2.3-beta.md`'s block, and per the design it MUST state Fedora and aarch64 explicitly: name
  `dotf_linux_arm64.tar.gz` as the asset an Apple-Silicon Fedora host resolves, and add the Fedora prerequisite line
  for the private/prerelease download path (`sudo dnf install -y gh`). Keep the clone + `./bootstrap.sh` snippet.
- Close with
  `**Full changelog:** <https://github.com/yandy-r/dotfiles/compare/v0.2.3-beta...v0.3.0-beta>` (MD034 is on, so keep
  the angle brackets exactly as the existing files do).

Must pass markdownlint and prettier (both existing release notes do). Touch no other `docs/releases/*.md`.

**Verify**

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && test -f docs/releases/v0.3.0-beta.md \
  && npx --no-install markdownlint-cli2 docs/releases/v0.3.0-beta.md \
  && npx --no-install prettier --check docs/releases/v0.3.0-beta.md \
  && grep -qi 'fedora' docs/releases/v0.3.0-beta.md \
  && grep -q 'dotf_linux_arm64.tar.gz' docs/releases/v0.3.0-beta.md \
  && grep -q 'prerelease' docs/releases/v0.3.0-beta.md \
  && grep -q 'Full changelog' docs/releases/v0.3.0-beta.md \
  && echo WS8-T6-OK
```

---

## Commit plan

One commit per task, in batch order. Conventional Commits 1.0.0; titles written as they should read in a changelog.
Files under `docs/plans` use `docs(internal):` per repo policy.

| #   | Commit title                                                                             | Task          |
| --- | ---------------------------------------------------------------------------------------- | ------------- |
| 1   | `docs(internal): add the Fedora support design and implementation plan`                  | WS0-DOC       |
| 2   | `fix(bootstrap): try the tag-pinned release asset before giving up on download`           | WS1-T1        |
| 3   | `feat(platform): map every Fedora Asahi Remix os-release ID to the RedHat family`         | WS1-T2        |
| 4   | `fix(apply): filter the dry-run package preview through backend applicability`            | WS1-T3        |
| 5   | `ci(go): trigger the Go workflows on bootstrap.sh and the goreleaser configs`             | WS1-T4        |
| 6   | `fix(symlink): back up directory destinations before removing them`                       | WS3-SAFETY    |
| 7   | `fix(launchers): resolve wifi-tui.sh from PATH instead of a hardcoded home directory`     | WS3-T6        |
| 8   | `ci(shell): gate every tracked shell script with shellcheck`                              | WS4-T2        |
| 9   | `fix(manifest): merge the ssh_server overlay field instead of dropping it`                | WS6-T7        |
| 10  | `fix(manifest): make names the pkg applicability declaration and add Fedora siblings`     | WS2-CORE      |
| 11  | `fix(common): load per-OS shell modules from a family search path`                        | WS-SHELL      |
| 12  | `ci(go): run the suite and dotf against a real Fedora userspace`                          | WS4-T1        |
| 13  | `docs(fedora): document the local container rehearsal loop`                               | WS4-T3        |
| 14  | `fix(plasma): resolve the KWin reconfigure from a busctl-first candidate table`           | WS5-T1        |
| 15  | `fix(plasma): stop managing the host-local default browser`                               | WS5-T3        |
| 16  | `feat(symlink): name facets by capability and add the --hyprland flag`                    | WS3-FACETS    |
| 17  | `chore(manifest): widen dotf.lock to redhat/arm64 and redhat/x86_64`                      | WS2-T4        |
| 18  | `docs(internal): add the Fedora package mapping table`                                    | WS2-T5        |
| 19  | `feat(manifest): add the font backend schema`                                             | WS5-T4        |
| 20  | `refactor(sysconfig): split linux.go into locale and firewall modules`                    | WS6-T1        |
| 21  | `fix(plasma): honour XDG_DATA_HOME and XDG_DATA_DIRS for colour schemes`                  | WS5-T2        |
| 22  | `feat(backend): add a native font backend`                                                | WS5-T5        |
| 23  | `fix(sysconfig): install ufw with the host family's own package manager`                  | WS6-T2        |
| 24  | `fix(platform): prefer the native package manager over Linuxbrew on arm64 Linux`          | WS6-T9        |
| 25  | `docs(fedora): add the Apple Silicon first-run acceptance checklist`                      | WS1-T5        |
| 26  | `feat(manifest): install the pinned Nerd Font families through the font backend`          | WS5-T6        |
| 27  | `fix(sysconfig): install glibc langpacks on Fedora instead of downgrading LANG`           | WS6-T3        |
| 28  | `fix(brew): decline the Homebrew bootstrap on arm64 Linux`                                | WS6-T8        |
| 29  | `feat(doctor): report the SELinux mode`                                                   | WS6-T10       |
| 30  | `refactor(platform): scan os-release once and expose VersionID`                           | WS7-T1        |
| 31  | `docs(backends): document the font rung on the decision ladder`                           | WS5-T7        |
| 32  | `fix(sysconfig): fall back to usermod when chsh is absent`                                | WS6-T4        |
| 33  | `feat(manifest): add the repo backend schema with apt and dnf sub-tables`                 | WS7-T2        |
| 34  | `fix(sysconfig): install firewalld when it is missing`                                    | WS6-T5        |
| 35  | `feat(backend): dispatch third-party repositories by package manager family`              | WS7-REPO      |
| 36  | `fix(sysconfig): read the firewalld ssh rule under sudo -n as a tri-state`                | WS6-T6        |
| 37  | `feat(manifest): install docker-ce on Fedora and gate the fingerprint module to Arch`     | WS7-MANIFEST  |
| 38  | `docs(backends): make rung 3 the family-dispatching repo backend`                         | WS7-T7        |
| 39  | `docs(architecture): rewrite platform detection around the canonical Go detector`         | WS8-T1        |
| 40  | `docs: add the testing guide and fix the documentation index`                             | WS8-T2        |
| 41  | `docs(features): add Fedora install blocks to the tool guides`                            | WS8-T3        |
| 42  | `docs: correct the bootstrap.sh line count and the verification commands`                 | WS8-T4        |
| 43  | `docs: add ROADMAP.md and link it from the README`                                        | WS8-T5        |
| 44  | `docs(release): add the v0.3.0-beta release notes`                                        | WS8-T6        |

---

## Critical test set (design §7) — exactly six

| #   | Test                                                                                                                          | Task / location                                                                    | Why it is not optional                                                                                                                   |
| --- | ----------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| 1   | Three `TestDetectOS` rows (bare `ID=fedora`; both Asahi IDs **without** `ID_LIKE`) + one `TestDistroName` row                  | **WS1-T2** — `internal/platform/platform_test.go` (existing tables)                | The primary target platform has no test row today, and a misclassification is not a loud failure but a silent loss of every Linux facet.  |
| 2   | `TestEmbeddedManifest_ToolsReachableOnSupportedManagers` (presence → reachable install path, subject set **manifest-derived**) | **WS2-CORE** step 3 — `internal/manifest/manifest_test.go`                         | Sole guard on ADR-2's one real regression risk (R1), **and** the fix for the root cause of the two fictional dnf names.                   |
| 3   | One `TestEngine_Applicable` row: a pkg entry with no name for the host manager is `applicable=false`, not an error             | **WS2-CORE** step 1d — `internal/backend/applicable_test.go` (existing table, C26) | Pins ADR-2's new semantics at the exact seam that changed; without it a refactor silently restores the fail-late abort.                   |
| 4   | `eligible-directory-backed-up-then-linked`: a populated **directory** dest emits `ActionBackup` **ordered before** `ActionRemove`, and the sentinel survives | **WS3-SAFETY** part 6a — `internal/symlink/apply_test.go` (existing table)          | The only test guarding data-destroying behaviour, and the first host to exercise the directory-copy path is a populated daily driver.     |
| 5   | `repo_test.go`: exact-argv table for the three `dnf_repo` modes **plus** an apt regression case                                | **WS7-REPO** part G — `internal/backend/repo_test.go` (**new file**)               | A new backend writing under `/etc` with `sudo` is not shippable untested; the apt case proves the lifted Debian `docker-ce` path is intact. |
| 6   | Fedora CI smoke: `dotf platform` contains `os=redhat`/`pkg=dnf`; `dotf apply --dry-run --headless` and `dotf link --dry-run` exit 0 | **WS4-T1** — `.github/workflows/go.yml` + `.forgejo/workflows/go.yml` (job step)   | The only thing in the repo that ever executes against a real Fedora userspace, a real `/etc/os-release`, real dnf5 and real rpm.          |

### Tests deliberately NOT written

Listed so their absence reads as a decision:

- a `capability_test.go` for a `platform.Info` capability table this plan does not build;
- per-installer tests for the orphaned `bin/install-*.sh` scripts;
- an arm64 **execution** test (no runner; the binary is a CGO-free cross-compile);
- `TestDetectDNFMajor` — its only consumer is WS7-REPO's copr mode, which case 3 of critical test #5 already
  exercises end to end;
- any test for `PkgMgrYUM`, `rpm-ostree`, or the `fingerprint` module;
- a shell-side detection harness (none exists in the repo; WS1-T2's shell edit is covered by `shellcheck` + the
  `bash -l` / `zsh -l` smoke checks);
- a `renderRepoTemplate` unit test separate from critical test #5 case 2, which asserts it directly;
- a `platform.GateAllows` unit test (WS7-MANIFEST) — it is a verbatim copy of `osGateAllows`, already covered
  through `TestEngine_Applicable`, and `cmd/dotf/setup_test.go`'s fixture exercises the ungated path.

---

## Regression guards

For each named risk: the specific guard, and the command that proves it.

### R1 — ADR-2 inverts a loud failure into a silent one on three working platforms (**highest**)

- **Guard**: `TestEmbeddedManifest_ToolsReachableOnSupportedManagers` (critical test #2), whose subject set is derived
  **from the manifest** — every intent-referenced `pkg` package, present and future — and whose predicate is the
  **real** `backend.Engine.Applicable`, so the test cannot drift from the engine. Evaluated against five
  representative hosts (apt/dnf/homebrew/pacman/zypper), with sibling groups resolved as units.
- **Landing rule**: it ships in the **same commit** as `applicableFor` (WS2-CORE). Not negotiable.
- **Proof**: `go test ./internal/manifest/ -run TestEmbeddedManifest_ToolsReachableOnSupportedManagers`, plus the
  **negative control** in WS2-CORE step 3 — delete `dnf = "ripgrep"` from `manifest/packages.toml:112` and confirm
  the failure message names `ripgrep` and the dnf host, then restore by hand.

### R10 — WS6's refactor breadth against a daily driver

- **Guard**: the **two-phase gate**. Phase one is WS6-T1 (pure code move) and WS6-T2 (`pkgInstallArgv` extraction),
  both of which must land with `internal/sysconfig/linux_test.go` and `register_test.go` **byte-identical to HEAD**.
  That unmodified test file is the behaviour-preservation proof for Arch/Debian/SUSE/macOS. Phase two (WS6-T3 →
  WS6-T6) then adds Fedora rows and the deliberate `*PlanNeedsSudo` flips, whose churn is the intentionality signal.
- **Proof**: after WS6-T1 and again after WS6-T2 —

  ```bash
  go test -race ./internal/sysconfig/... \
    && test -z "$(git diff --name-only -- internal/sysconfig/linux_test.go internal/sysconfig/register_test.go)"
  ```

- **Asymmetry note (C19)**: `FirewallPlanNeedsSudo`'s flip breaks golden rows at `linux_test.go:486-488`, but
  `SSHServerPlanNeedsSudo` has **zero** tests repo-wide — so WS6-T6 adds `TestSSHServerPlanNeedsSudo` deliberately
  rather than relying on churn that would not appear.

### ADR-12 — the facet rename

- **Guard**: six golden locks move together, and `cmd/dotf/apply_test.go:717-748`
  (`TestRunApply_LinuxDesktopFlagPassthrough`) stays **unmodified** as the back-compat proof — it passes the literal
  `--linux-desktop` and asserts `.config/hypr` becomes a symlink.
- **Locks**: `table_test.go` `TestTable_Count` (41, unchanged), `TestTable_FacetCounts`, `TestTable_MatchesPlaybook`,
  `TestApplies`; `symlink_test.go` `TestPlan_IdenticalSet_PerOS`, `TestFaceting_WSL`.
- **The trap to reason about**: `debian-desktop`'s `wantCreated` **stays 39** because tmux.service is gained (+1)
  exactly as arch-update.desktop is lost (−1). WS3-FACETS requires a comment stating this, or the row reads as a
  false pass.
- **The predicate that must not slip**: `FacetLinux` resolves through the **local** `isLinuxFamily`
  (`table.go:143-150`), never `platform.IsLinuxFamily`. Pinned by `table_test.go:139`
  (`desktop-generic-linux-ld-on` expects false on `OSLinux`) and by `symlink_test.go`'s `generic-linux` row
  (wantCreated 28).
- **Proof**:
  `go test -race ./internal/symlink/... ./cmd/dotf/... && go run ./cmd/dotf link --dry-run --hyprland && go run ./cmd/dotf link --dry-run --linux-desktop`.

### ADR-5 — generalising `apt-repo` into `repo`

- **Guard**: `internal/backend/apt_repo_test.go` (414 lines) survives **completely unmodified** and now exercises the
  new backend through the deprecated alias constructor; plus critical test #5 case 4, which asserts the **same six
  calls in the same order** for the apt path.
- **Why it matters**: `docker-ce` is in the headless default set, so a regression breaks **Debian** provisioning.
- **Landing rule**: the APT install body is lifted **verbatim** from `apt_repo.go:61-114` — no reordering, no
  renaming, no rewording.
- **Proof**:
  `go test -race -run 'AptRepo|TestRepoBackend' -v ./internal/backend/ && go test -race ./internal/backend/... ./cmd/dotf/...`.

### ADR-14 — the shell loader fix

- **Guard**: the loader change makes ~495 previously-dead lines live on **four** distro families at once, so the
  regressions it exposes are fixed in the same commit — both `alias history=` shadows (dnf `:50` and yum `:70`),
  the ufw-only firewall block (a firewalld branch is added), and the Plasma 5 `systemsettings5` alias (guarded by
  `command -v`). The PR #36 review found this enumeration incomplete: `mount` and `netstat` are deleted,
  `suspend`/`enable`/`disable` are renamed to `sys*` non-shadowing names, `gcc`/`g++` become `gccw`/`gxxw`, Arch's
  `info` becomes `pkginfo`, and `updateall`/`cleanup-packages` move into the package-manager dispatch. Do not
  treat any count in this document as the audited total — see PART C and ADR-14 in `design.md`.
- **Proof**:

  ```bash
  shellcheck --severity=warning common/platform.sh common/aliases.sh common/functions.sh common/exports.sh common/linux/aliases.sh \
    && bash -l -c 'echo ok' && zsh -l -c 'echo ok' \
    && bash -ic 'type history' | head -1   # must report the BUILTIN, not an alias
  ```

  plus the new `.github/workflows/shell.yml` job, which triggers on `common/**` (WS4-T2).

### Custom-script governance

- **Guard**: `customScriptBaseline = 16` in `internal/manifest/ratchet_test.go` — an exact-match tripwire in **both**
  directions. No task in this plan adds, removes, or converts a `custom-script` entry; the two new backends (`font`,
  `repo`) are native and cost nothing against it.
- **Proof**:
  `go test ./internal/manifest/ -run TestCustomScriptRatchet && test "$(grep -c '^backend = \"custom-script\"' manifest/packages.toml)" = 16`.

### Asset-name three-way pin

- **Guard**: `TestAssetNaming_MatchesBootstrap` (`internal/selfupdate/selfupdate_test.go:590-650`) string-matches
  `bootstrap.sh:245`'s `ASSET=` line, `checksums.txt`, the four os/arch combos, and six bootstrap markers. WS1-T4
  makes it actually **trigger** on a `bootstrap.sh`-only PR for the first time.
- **Proof**: `go test ./internal/selfupdate/...`, plus WS1-T1's marker loop.

---

## Verification ladder

### Per task

Each task's own **Verify** block, run to completion before the commit. Every Go task ends with `gofmt -l <dirs>`
printing nothing plus `go build ./... && go vet ./... && go test -race <scope>`; every shell task ends with
`shellcheck --severity=warning`; every docs task ends with `markdownlint-cli2` (and `prettier --check` for **new**
files only — `README.md`, `CLAUDE.md` and `docs/README.md` fail prettier pre-existing and must not be reformatted).

### Per batch

After the last task in a batch, the full gate from the repo root:

```bash
cd /home/yandy/.claude-worktrees/dotfiles-feat-fedora-support \
  && gofmt -l ./cmd ./internal \
  && go build ./... \
  && go vet ./... \
  && go test -race ./... \
  && golangci-lint run ./... \
  && bin/lint.sh --shell \
  && test "$(grep -c '^backend = \"custom-script\"' manifest/packages.toml)" = 16
```

`gofmt -l` must print nothing and every command must exit 0. The worktree is green at HEAD, so **any** failure is
attributable to the batch.

### Before the PR

```bash
bash -l -c 'echo "Bash config loaded successfully"' \
  && zsh  -l -c 'echo "Zsh config loaded successfully"' \
  && go run ./cmd/dotf link --dry-run \
  && go run ./cmd/dotf apply --dry-run --headless \
  && go run ./cmd/dotf doctor \
  && npm run lint \
  && actionlint .github/workflows/*.yml .forgejo/workflows/*.yml
```

Plus the local rehearsal loop from `docs/fedora-rehearsal.md`: the amd64 privileged systemd Fedora container
(`dotf sysconfig … --yes` reaching `failed=0`), the emulated `linux/arm64 fedora:latest` (a real dnf transaction and
`rpm -q`), and the one-liner that reproduces the CI Fedora job locally (`FEDORA_JOB_OK`).

### Only the user can prove it — on the M1

`docs/fedora-first-run.md` is walked **end to end on the physical device before the release tag is cut**. It is a
gate, not a report. The claims no CI job and no container can make:

| Claim                                                                     | Why only the device                                                                            |
| ------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------ |
| The verbatim `/etc/os-release` `ID` and `VARIANT_ID`                      | Four sources contradict each other; only the machine settles it (WS0 capture, step 0).         |
| Whether `chsh` is present, and from which package                         | Same contradiction. ADR-8 is correct either way, but the answer should be recorded.            |
| `bootstrap.sh` downloading against a private prerelease origin            | Step 1, run with `GH_TOKEN` **deliberately unset**, so success or failure is observed alone.   |
| SELinux behaviour under real provisioning                                 | No x86 container exposes a meaningful `/sys/fs/selinux`. `dotf doctor` now reports the mode.   |
| 16 KiB page-size interaction with any C-linked third-party binary         | Not modelled by any x86 container.                                                             |
| A live KDE Plasma 6 session; `--reload`'s component restarts              | `dotf plasma diff` runs headless, but the visible result cannot be judged headlessly.          |
| `hostnamectl set-hostname`                                                | Measured to fail in every container — `/etc/hostname` is owned by the runtime.                 |
| btrfs / snapper behaviour                                                 | Containers are overlayfs; the snapshot path is exercised only through the fake.                |
| arm64 **plus** systemd together                                           | Unreachable under `qemu-user` (`dbus-broker` dies `226/NAMESPACE`); ADR-15 declines a TCG VM.  |
| The real NetworkManager/firewalld zone on a roaming laptop                | A container's zone is not a laptop's zone.                                                     |
| Whether anything in `/etc/profile.d` matters on this host                 | The Fedora `/etc/bashrc` severance is deliberately unaddressed; record the observed impact.    |
| Sentinel survival: `~/.config/nvim/…` after `dotf link --backup`          | Critical test #4 proves the mechanism; only the device proves it against the real tree.        |
| `dotf link --dry-run` now lists `.config/systemd/user/tmux.service`       | ADR-12's retag; it does not today.                                                             |
| `dotf install docker-ce` writes `/etc/yum.repos.d/docker-ce.repo`         | Run on a host with **no** Linuxbrew on PATH, or `pkg=` will not be `dnf` and the result misreads. |

---

## Advice

- Keep `manifest/packages.toml`, `internal/backend/backend.go`, `internal/symlink/table.go` and
  `internal/sysconfig/linux.go` on their declared writer chains. They are the highest-risk coordination surfaces, and
  three of the four have already drifted internally at least once.
- **WS3-SAFETY before anyone runs `dotf link` on the M1.** A Fedora rollout whose first mutating command deletes the
  user's editor config is a failed rollout regardless of how correct the dnf argv is. It is scheduled in batch 1 for
  exactly that reason.
- **Land ADR-2's `applicableFor` and its coverage test in one commit** (WS2-CORE). Separating them ships a silent
  regression window on macOS and Arch. If the test's reachability logic is not convincing in review, **reject the
  commit** — that is the design's own instruction.
- Do **not** "fix" `internal/backend/pkg.go`'s dnf argv. They are dnf5-correct. The dnf5 problem is entirely in the
  **repo** verbs, which ADR-6 routes around by writing `.repo` files directly.
- Do **not** unify `internal/symlink`'s private `isLinuxFamily` with `platform.IsLinuxFamily`. The `OSLinux` exclusion
  is deliberate and pinned by a golden row; `FacetLinux` must use the local predicate.
- Treat `dotf apply --dry-run` as a **safety-critical surface**, not a convenience. The whole device-first plan rests
  on it being honest, which is why WS1-T3 fixes its applicability filtering before any mutation is attempted — and
  why its probe-error policy is deliberately fail-**open**.
- Write **no backend count** in any code comment or doc. Two backends are added in this PR (`font`, `repo`) plus one
  deprecated alias; every numeral that survives will be wrong again on the next one. `docs/reference/manifest-backends.md`
  is the single place a count could live, and even there the ladder is enumerated rather than counted.
- Correct documentation drift **as part of** the implementation, not after. The user's next test is a clean Fedora
  Asahi install, so a `platform-detection.md` that documents only the shell detector and a `CLAUDE.md` that claims
  `bootstrap.sh` is 307 lines will create avoidable false negatives on the exact file the first boot depends on.

