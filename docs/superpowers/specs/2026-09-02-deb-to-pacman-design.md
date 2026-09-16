# `dotf deb` — Debian package conversion for pacman

Design spec, 2026-09-02.

## Problem

Some upstreams ship only a `.deb`. On Arch the accepted workaround is
[`debtap`](https://github.com/helixarch/debtap), which translates a `.deb` into a
`.pkg.tar.zst` that `pacman -U` can install. Driving it by hand is error-prone:

- `debtap` is interactive by default and its Debian→Arch dependency translation is
  approximate, so it silently produces packages with unsatisfiable dependencies.
- Nothing checks whether a real repo or AUR package already exists — converting a
  `.deb` when a native package is available is the single worst outcome, because the
  converted package will shadow it and never receive updates.
- Downloading a `.deb` from a URL has no integrity story.
- The converted package's real contents are never inspected before `pacman -U` runs
  as root.

## Solution

A `dotf deb` subcommand wrapping the whole pipeline, with verification at every
stage, driven by the repository's existing `exec.Runner` seam.

### Why a subcommand, not a standalone binary

`dotf` already owns the machinery this tool would otherwise duplicate:

| Capability | Source |
| --- | --- |
| bash/zsh/fish completions, `--install`/`--force` | `cmd/dotf/completiongen*.go` derives its model from `helpRegistry` + real flagsets |
| Rich `--help`/`-h` and `dotf help deb` | `cmd/dotf/help.go` + `helptext.go` |
| `key=value` stdout / diagnostics stderr contract | repo-wide convention |
| Exit-code legend `0/1/2` | repo-wide convention |
| Mockable subprocess execution | `internal/exec.Runner` + `Fake` |
| Release packaging, `self-update` | `.goreleaser.yml`, `internal/selfupdate` |

Registering `deb` in `helpRegistry` and `main.go`'s switch makes completions and help
appear with **zero** additional code. A standalone binary would reimplement roughly
800 lines of already-solved completion generation.

## Command surface

```
dotf deb <url|path> [flags]
```

Exactly one positional: an `https://` URL or a filesystem path to a `.deb`.

| Flag | Default | Purpose |
| --- | --- | --- |
| `--install` | false | Install the converted package after verifying it |
| `--yes` | false | Skip the interactive pre-install confirmation |
| `--force` | false | Proceed past the native-package precedence gate |
| `--out-dir <path>` | `.` | Directory the `.pkg.tar.zst` is written to |
| `--sha256 <hex>` | "" | Expected digest of the input `.deb`; fail closed on mismatch |
| `--pkgname <name>` | "" | Override the converted package's name (`debtap -P`) |
| `--pkgver <ver>` | "" | Override the converted package's version (`debtap -p`) |
| `--keep` | false | Keep the downloaded `.deb` and the debtap work directory |
| `--dry-run` | false | Print the plan; execute nothing |
| `--json` | false | Emit a structured document instead of `key=value` lines |

`--install` defaults off: conversion is read-only with respect to the system, while
installation needs root and can claim files owned by real packages. This matches the
posture of `plan`/`status`/`diff`.

## Pipeline

```
resolve → acquire → verify input → precedence gate
        → convert → inspect artifact → dependency check
        → [confirm → install → verify installed]
```

### 1. Resolve

Classify the positional. A value starting with `https://` is a URL; `http://` is
rejected outright (no `--insecure` escape hatch — a plaintext `.deb` that later runs
as root is not a tradeoff worth offering). Anything else is a filesystem path.

### 2. Acquire

The input is downloaded **or copied** into a private, unguessably-named 0700
directory under `$XDG_CACHE_HOME/dotf/deb`, streamed through an `io.TeeReader`
feeding a `sha256.Hash` so the digest covers every byte actually written. Every
later stage reads only that private copy; the caller's path is never re-opened.

This is what makes `--sha256` a **gate** rather than a report. Hashing a path the
caller does not control and then re-opening that same path for validation, control
extraction and conversion leaves a window: anyone with write access to the input's
_directory_ can let the hash read the genuine file and swap in a hostile one before
conversion — and those swapped bytes are what `sudo pacman -U` ultimately runs as
root. Binding every read to one private copy closes it. The user's own file is
copied, never adopted, so it is never modified or deleted.

### 3. Verify input

Two checks, in order, both before anything else touches the file:

1. **Magic bytes.** A `.deb` is an `ar` archive whose first member is
   `debian-binary`. Verify the global header `!<arch>\n` at offset 0, then the
   16-byte name field of the first member header — which begins at offset 8 —
   against `debian-binary`. The file extension is deliberately
   ignored — a mislabeled or extensionless file is classified by its contents,
   consistent with `internal/backend/download.go:detectFormat`.
2. **Digest.** The computed SHA-256 is _always_ printed. When `--sha256` is given it
   is compared via `backend.VerifySHA256`, the repository's existing constant-time
   compare primitive, so the checksum contract stays in one place. A mismatch aborts
   before conversion; no artifact is produced.

### 4. Precedence gate

Read the `.deb`'s own `control` file to get its real `Package` and `Version`, then
check whether an Arch package of that name already exists.

**Reading `control` without new dependencies.** Go's standard library cannot
decompress `control.tar.zst` or `control.tar.xz`, which is what modern `.deb` files
use. Rather than add a zstd/xz module, control extraction shells out to **`bsdtar`**
through `exec.Runner`. `bsdtar` ships with libarchive, which is a hard dependency of
`pacman` itself, so on any host where this command is applicable it is guaranteed
present. Three calls, none of them through a shell:

```
bsdtar -tf <deb>                            # list members; locate control.tar.*
bsdtar -xf <deb> -C <workdir> <member>      # extract that exact member by name
bsdtar -xf <workdir>/<member> -C <workdir>  # yields <workdir>/control
```

Listing first, then extracting an exact member name, avoids guessing the
compression suffix and doubles as a structural validation of the archive.

The `ar` magic check in step 3 stays native Go, since it needs no decompression.

**The gate itself.** Using `pacman -Ssq "^<name>$"` (bare-name output, locale
independent — unlike `pacman -Si`, whose field labels are translated):

- A hit in the **official repos** is a hard gate: exit 1 with an explanatory
  diagnostic, unless `--force`.
- A hit in the **AUR** (`paru -Ssq` or `yay -Ssq`, whichever `LookPath` resolves;
  skipped if neither) is an advisory warning only. AUR name matches are fuzzy enough
  that blocking would be noise.

Because `pacman -Ss` matches descriptions as well as names, results are post-filtered
to those exactly equal to the queried name. The name is regex-quoted, because Debian
package names legitimately contain regex metacharacters (`libstdc++6`, `python3.11`).

**The gate runs twice.** The pre-conversion check reads the `.deb`'s self-declared
`Package` field, but what actually gets installed is the built artifact's `pkgname`,
which debtap (or `--pkgname`) can change. A `.deb` declaring an obscure name would
otherwise slip past the gate and install a package that shadows a real one, so the
gate is re-applied to the artifact's real name after conversion whenever the two
differ.

### Argument-injection guard

The `.deb`'s control data and the built package's `.PKGINFO` are attacker-controlled,
and both feed package and dependency names into pacman argv. pacman parses any operand
beginning with `-` as a **flag**, and offers no reliable `--` end-of-options terminator,
so a crafted `depend = --dbpath=/tmp/evil` would redirect the dependency check at a
database the attacker supplied and make it report that everything is satisfied.

Every value derived from package metadata is therefore screened by `safeOperand` before
it can reach an argv. A rejected dependency is reported as unresolvable without being
queried — no real package can bear such a name, so "broken" is the honest answer.

Commands are never invoked through a shell, so this is the only injection surface;
argument injection, not command injection, is the residual risk that had to be closed.

### 5. Convert

Preconditions, each a soft exit-1 error with an actionable message:

- Host is Arch (`platform.Detect` reports `pacman`), else `UnsupportedOSError`.
- `debtap` resolves on `PATH`, else a message pointing at `dotf install debtap`.
- `/var/cache/debtap` exists and is non-empty, else a message instructing
  `sudo debtap -u` (debtap cannot translate dependencies without its databases).

The `.deb` is staged into a temp work directory and `debtap` is invoked there via
`RunDir`, because debtap writes its output beside its input. The work directory is
removed afterwards unless `--keep`, in which case its path is reported — a kept
directory nobody can name is not actually kept. A `.deb` fetched from a URL is likewise
removed from the cache unless `--keep`; an input the user already had on disk is never
touched.

```
debtap -Q [-P <pkgname>] [-p <pkgver>] <staged.deb>
```

`-Q` is the fully non-interactive mode; the command never leaves a user sitting at an
unexpected prompt. The produced `*.pkg.tar.zst` is located by globbing the work
directory and moved into `--out-dir`.

### 6. Inspect artifact

Read `.PKGINFO` directly out of the produced package:

```
bsdtar -xOf <artifact> .PKGINFO
```

`.PKGINFO` is a plain `key = value` file — authoritative and locale independent,
unlike `pacman -Qip`, whose field labels are translated. It yields `pkgname`,
`pkgver`, `size`, and every `depend`.

### 7. Dependency check

```
pacman -T <depend>...     # provider-aware, version-aware, locale independent
pacman -Ssq "^<dep>$"     # per unsatisfied dep: does a repo supply it at all?
```

`pacman -T` reports which declared dependencies are not satisfied on this system,
correctly handling both version constraints and virtual providers. Each unsatisfied one
is then searched for in the repos; those absent there too are the genuinely broken ones
debtap's translation produced, and are reported as warnings. The unresolvable set can
overreport, because a dependency satisfied by a repo package that _provides_ it under
another name is not found by an exact name search — which is another reason it is
advisory.

This is advisory and never blocks: `pacman -U` is the authority on installability and
produces a better message than a pre-flight guess could.

### 8. Install

Guarded by an interactive confirmation on stdout (following `runPlasma`/`runSysconfig`
— on the interactive path stdout is the terminal, and a prompt on stderr is invisible
to anyone who redirected it). `--yes` skips it; Ctrl+C at the prompt exits 130.

```
sudo pacman -U <artifact>
```

Only this step elevates. Conversion runs entirely as the invoking user.

### 9. Verify installed

```
pacman -Qq <name>    # exit 0 confirms the package is registered
pacman -Qql <name>   # one owned path per line; counted in Go
```

Both are locale independent. The reported file count is what pacman actually owns,
not what the artifact claimed.

## Output contract

`key=value` lines on stdout, diagnostics on stderr, per repo convention:

```
source=file
input=/home/u/slack.deb
sha256=9f2b…
deb_package=slack-desktop
deb_version=4.36.140
precedence=none
artifact=/home/u/slack-desktop-4.36.140-1-x86_64.pkg.tar.zst
package=slack-desktop
version=4.36.140-1
size=112894976
depends=14
depend_unsatisfied=libappindicator-gtk3
depend_unresolvable=libappindicator-gtk3
installed=true
files=1204
```

`--json` emits the same facts as one document, mirroring `plasmajson.go`'s envelope
approach so a machine consumer never gets zero bytes plus a bare exit code.

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | Converted (and installed, when `--install`) successfully |
| 1 | Soft failure: unsupported host, `debtap`/`pacman` missing, running as root, bad `.deb`, digest mismatch, precedence gate, a failed query, conversion or install failure |
| 2 | Usage error (bad flag, wrong positional count) |
| 130 | Ctrl+C at the confirmation prompt |

Exit 3 is **not** used. `cmd/dotf/plasma.go` documents it as reserved for `dotf
apply`'s hard aborts, and `deb` follows plasma's precedent of treating an
inapplicable host as a soft error rather than a usage error — the invocation was
well-formed, the host simply cannot serve it.

## File layout

New:

- `internal/debtap/debtap.go` — `Converter`, pipeline orchestration, options/result types
- `internal/debtap/deb.go` — `ar` magic validation, `control` extraction and parsing
- `internal/debtap/verify.go` — precedence gate, `.PKGINFO` parsing, dependency and post-install checks
- `internal/debtap/*_test.go`
- `cmd/dotf/deb.go` — flag surface, positional parse, confirmation prompt, printing

Touched:

- `cmd/dotf/main.go` — one dispatch case
- `cmd/dotf/helptext.go` — one `helpRegistry` entry (this is what grants completions)
- `manifest/packages.toml` — `debtap` as an `aur` package so `dotf install debtap` works
- `README.md`, `CLAUDE.md` — command surface documentation

## Testing

End-to-end through `runDeb` with `exec.Fake`, asserting real output and the exact
argv sequence. Deliberately not an exhaustive unit matrix:

1. Convert-only from a local file — full `key=value` output and argv sequence
2. URL acquisition via `httptest` with a matching `--sha256`
3. `--sha256` mismatch — fails closed, no artifact written
4. `--install --yes` — asserts `sudo pacman -U` ran and post-install lines are emitted
5. Precedence gate blocks on a repo collision; `--force` proceeds
6. A non-`.deb` file is rejected by magic bytes
7. `--dry-run` records zero mutating calls

The existing `TestHelpRegistry_CoversDispatch` and the completion drift guards extend
to `deb` automatically once it is registered.

## Rejected alternatives

**A standalone binary.** Would duplicate the completion generator, help system, and
release pipeline for no gain; the tool is squarely a provisioning concern.

**A shell script in `bin/`.** Hand-written completions, no test seam, and the
verification logic (digest streaming, `.PKGINFO` parsing, dependency set algebra) is
awkward and fragile in shell.

**Adding a zstd/xz Go module** to read `control.tar.*` natively. `bsdtar` is
guaranteed present via pacman's own dependency on libarchive, so the module would buy
nothing but supply-chain surface.

**Parsing `pacman -Qip` / `pacman -Si` output.** Both are localized. `.PKGINFO`,
`pacman -Ssq`, `pacman -T`, and `pacman -Qql` are not.

### Privilege posture

The command **refuses to run as root**. `sudo dotf deb …` is a natural reflex — it
openly needs sudo for the install, and its preflight suggests `sudo debtap -u` — but
under sudo the download, the `bsdtar` extraction of an attacker-supplied control
tarball, and debtap's whole conversion would execute as uid 0, and root `bsdtar`
additionally restores ownership and setuid bits an unprivileged extraction drops.
Only `pacman -U` elevates, and it prompts for sudo itself.

The install confirmation names the real consequence: debtap carries the `.deb`'s
Debian maintainer scripts into the Arch package's `.INSTALL` scriptlet, so approving
an install runs shell the package author wrote, as root.

### Failing closed

A query that could not run must never look like a query that passed. `pacman -Ssq`
exits 1 with no output for a legitimate no-match, but it also fails when the sync
database was never populated or is locked; `pacman -T` signals "unsatisfied" with
exit 127 specifically. Both are distinguished by exit status, and any other failure
aborts the run rather than degrading to "no native package exists" or "no dependency
problems" — which would silently disable the gate on exactly the hosts most likely
to need it. `pacman` is checked in the preflight for the same reason.

Artifact placement refuses to write through a symlink (`O_NOFOLLOW` on the copy path,
an `Lstat` before the rename), because `--out-dir` may be a directory the user does
not solely control and the artifact's filename is fully predictable from the `.deb`
the attacker authored.

## Rejected alternatives (continued)

**Auto-installing `debtap`.** Silently installing an AUR package as a side effect of
a conversion request is surprising. The command instead fails with the exact
remediation, and `debtap` becomes declarable in the manifest.
