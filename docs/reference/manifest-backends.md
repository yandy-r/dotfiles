# Choosing a manifest backend: native before `custom-script`

`dotf` installs every package through one of the backends declared on a
`[[package]]` entry in `manifest/packages.toml`. All but one are **native**:
they describe _what_ to install in typed manifest fields, so the engine can probe
installed-state, pin checksums, and update/remove without shelling out to
bespoke logic. The exception — `custom-script` — is the **escape hatch**: it runs
a hardened `bin/install-*.sh`, which is powerful but opaque to the engine.

This document is the decision order. **Reach for `custom-script` last**, and only
after none of the native backends can express the install.

## The backend ladder

Try these in order; the first that fits is the right one:

1. **`pkg`** — the tool ships in a host package manager (apt/dnf/pacman/zypper/brew).
   Map the per-manager name in `names`.
2. **`aur`** — Arch-only, available in the AUR.
3. **`repo`** — the tool ships from a third-party OS repository. ONE entry per tool;
   add the sub-table(s) for the families you support and the backend dispatches on
   the host's package manager:
   - `apt_repo` — Debian/Ubuntu: GPG key + `sources.list` line, then a normal install.
     `repo_line` templates `{{.Codename}}` and `{{.Arch}}` from the host. Optional
     `key_fingerprint` (40 hex) is checked with `gpg --show-keys` before any key is
     trusted; a mismatch aborts before any `sudo` step.
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

4. **`toolchain`** — installable via a language toolchain (`cargo`, `npm`). Set `tool`.
5. **`github-release`** — a GitHub release ships a checksummed binary/asset. Supports
   asset auto-detection and a shared `checksums.txt` sidecar (`checksums_asset`).
6. **`font`** — the package IS a font family the configs pin by name. Set `font.family`
   (the fontconfig name, and the installed-state probe) plus either per-manager `names`
   (Arch `ttf-*-nerd`, a Homebrew cask) or `font.archive` + `repo`/`tag` to extract the
   checksum-verified upstream release archive into the XDG fonts directory. A downloadable
   archive REQUIRES a checksum source — either `sha256` or `checksums_asset`, and the
   backend fails closed without one. The per-target `checksums` map is **not** accepted
   here: it is keyed `<os>/<arch>`, which is meaningless for a single arch-independent
   Nerd Font zip, and `fontChecksum` never reads it. Validation rejects a font entry
   pinned only that way rather than letting it pass CI and hard-fail at install.
7. **`script-url`** — a vendor HTTPS install script that cannot be pinned to a single
   artifact (a rolling installer). Verified against `sha256` when the vendor offers one.
8. **`git-clone`** — the install _is_ a clone into `$HOME` (e.g. tpm), with optional
   typed `post_install` activation steps.
9. **`custom-script`** — **last resort**: a hardened `bin/install-*.sh` for install logic
   none of the above expresses. Governed (see below).

Any backend accepts `requires_desktop = true` for GUI apps: the package is
`applicable=false` unless dotf sees a graphical session (macOS, or Linux with
`XDG_SESSION_TYPE=x11|wayland` or `WAYLAND_DISPLAY`). It applies to `--only` too.

## How to add a native entry

Each backend needs only its own fields. Worked examples:

```toml
# 1. pkg — native package manager, per-manager name map
[[package]]
name = "ripgrep"
backend = "pkg"
names = { apt = "ripgrep", dnf = "ripgrep", pacman = "ripgrep", brew = "ripgrep" }

# 1b. pkg with a distro-bug workaround — dnf_exclude is passed as --exclude=<glob> on
#     dnf install/upgrade (other managers ignore it; pkg backend only). Document the
#     upstream bug in a comment and drop the entry once it is fixed.
[[package]]
name = "vlc"
backend = "pkg"
names = { pacman = "vlc", apt = "vlc", dnf = "vlc" }
dnf_exclude = ["kf6-kmime*"]

# 3. repo — one tool, one entry; the sub-table matching the host's package manager wins
[[package]]
name = "docker-ce"
backend = "repo"
names = { apt = "docker-ce", dnf = "docker-ce" }
apt_repo = { key_url = "…/gpg", keyring = "/etc/apt/keyrings/docker.gpg", repo_line = "deb [arch={{.Arch}} signed-by=/etc/apt/keyrings/docker.gpg] …/ubuntu {{.Codename}} stable", sources_path = "/etc/apt/sources.list.d/docker.list" }
dnf_repo = { id = "docker-ce", repofile = "https://download.docker.com/linux/fedora/docker-ce.repo" }

# 4. toolchain — a cargo/npm-installed tool
[[package]]
name = "eza"
backend = "toolchain"
tool = "cargo"

# 5. github-release — checksummed release asset (auto-detected when `asset` is empty)
[[package]]
name = "hadolint"
backend = "github-release"
repo = "hadolint/hadolint"
binary = "hadolint"                 # installed PATH name when it differs from the asset
tag = "v2.12.0"                     # required whenever sha256/checksums are pinned
checksums_asset = "checksums.txt"   # read the asset's sha256 from a shared sidecar

# 6. font — a patched font family; package manager where one exists, archive otherwise
[[package]]
name = "jetbrains-mono-nerd-font"
backend = "font"
names = { pacman = "ttf-jetbrains-mono-nerd", homebrew = "font-jetbrains-mono-nerd-font" }
repo = "ryanoasis/nerd-fonts"
tag = "v3.4.0"                      # required whenever font.archive is set
checksums_asset = "SHA-256.txt"     # required whenever font.archive is set
font = { family = "JetBrainsMono Nerd Font", archive = "JetBrainsMono" }

# 7. script-url — a rolling vendor install script, run headlessly
[[package]]
name = "starship"
backend = "script-url"
url = "https://starship.rs/install.sh"
args = ["--yes"]                    # literal argv appended to the installer

# 8. git-clone — the install is a clone into $HOME
[[package]]
name = "tpm"
backend = "git-clone"
url = "https://github.com/tmux-plugins/tpm"
dest = ".tmux/plugins/tpm"          # doubles as the installed-state probe
```

A `script-url` installer runs under `sh`, not `bash` — these are POSIX vendor
scripts, and some (starship's) refuse a non-POSIX shell outright. Anything the
installer needs in order to run **non-interactively** goes in `args`, which is
passed as literal argv and never through a shell. Without it a prompting
installer reads `/dev/tty`, which a headless `dotf apply` cannot answer, and the
package fails on every machine. `args` is rejected on any other backend.

A font family is a native `font` entry, **never** a `custom-script` one: the
escape-hatch budget is frozen by the ratchet below, and cosmetics do not spend it.

See `manifest/packages.toml` for live examples of every backend, and
`internal/manifest/validate.go` for the exact per-backend required-field rules.

### A COPR-only tool

Reach for `dnf_repo = { copr = "owner/project" }` **only** when the tool has no
upstream release asset. A `github-release` entry is lock-pinnable and independent of
a third party's build queue, so it wins whenever one is available: `lazygit` on
Fedora is the `lazygit-bin` `github-release` entry, **not** `copr:atim/lazygit`,
whose newest build had failed.

`copr` is the only `dnf_repo` mode that shells out to dnf — the file modes write
`/etc/yum.repos.d/<id>.repo` directly. It is also the only mode with a plugin
prerequisite, and dotf installs the right package for the host's dnf major
(`dnf5-plugins` vs `dnf-plugins-core`) automatically. Never run `dnf config-manager`
by hand from a manifest entry.

## The `custom-script` governance fence

`custom-script` is deliberately fenced so the escape hatch cannot quietly grow:

- **Every `custom-script` entry MUST declare a `class`** (`pinned` | `rolling` |
  `managed`) **and a `justification`.** This is enforced at parse time by
  `validateClass` (`internal/manifest/validate.go`) — an unclassified or
  unjustified entry fails `go test` (and CI). Use `rolling` for a vendor installer
  that tracks latest, `managed` when versioning is delegated to a package/toolchain
  manager or a governed setup step, `pinned` only when a single checksummed
  artifact is involved.

- **Adding or removing a `custom-script` entry moves a committed baseline.** The
  count of `custom-script` packages is pinned by `customScriptBaseline` in
  `internal/manifest/ratchet_test.go`. `TestCustomScriptRatchet` fails unless the
  live count equals the baseline exactly, so every change to the escape-hatch
  population is a visible, reviewed, deliberate act: an addition forces you to
  justify why no native backend fit, and a migration ratchets the baseline down.

This is the "freeze growth after the native/setup-step tiers exist" governance
from the Phase-4 research: the native path is the expected one, and the hatch
stays flat unless a reviewer signs off on a bump.

Growing the ladder is priced honestly against that fence. Generalising `apt-repo`
into the family-dispatching `repo` backend added one enum value plus one deprecated
alias (ADR-5), and it cost **nothing** against `customScriptBaseline` — still 16 —
because `repo` is native.

## Reading the backend/class histogram

`dotf status --counts` (and `dotf diff --counts`) append an aggregate governance
histogram over the manifest:

```console
$ dotf status --counts | grep '^count\.'
count.total=NN
count.backend.custom-script=NN
count.backend.github-release=NN
count.backend.pkg=NN
count.class.managed=NN
count.class.pinned=NN
count.class.rolling=NN
```

`count.backend.custom-script` is the live escape-hatch population — the number the
ratchet pins. With `--json`, the same aggregate is attached as a `Summary` object.

## Native desktop artifacts

`app-artifact` installs a Linux AppImage or tarball per user, with a dotf-owned
menu entry. Declare `url` (HTTPS) or `repo`/`tag`/`asset`, plus `app` metadata
(`icon`, `desktop_name`, optional `desktop` and `categories`). Archives need
`binary` relative to their root. SHA256 pins immutable artifacts; rolling
vendor URLs rely on HTTPS. ELF architecture, archive paths and desktop values
are checked before activation. Install/update stages the full artifact before
replacing the old copy; remove touches only dotf-owned paths.

Both `app-artifact` and `pkg-url` also take a rolling GitHub route: `repo` +
`asset` with `class = "rolling"` and no `tag`. `asset` is then a glob (Go
`filepath.Match`, not a `{{.Arch}}` template) matched against the asset names of
the repo's latest stable release. `preview = true` selects the newest non-draft
prerelease among the 20 most recent releases instead. Assets whose name carries
a foreign-arch token (`arm64`/`aarch64` on an x86_64 host; `x86_64`/`amd64`/`x64`
on an arm64 host; 32-bit `arm` variants on both) are skipped, so one glob
resolves a multi-arch release. An asset with no arch token is never skipped, so
a release that leaves one arch unlabelled needs per-arch sibling entries with
explicit globs. Exactly one asset must match, with an HTTPS download URL and a
GitHub-reported `sha256:` digest, which is the checksum the download is
verified against; anything else fails closed. No manifest checksum or
`dotf lock` entry is involved.

`pkg-url` installs an official downloaded RPM/DEB through dnf/apt. Declare
`url` or GitHub coordinates (pinned `repo`/`tag`/`asset`, or the rolling route
above), `format` and `pkg_name`. RPM signatures use
`gpg_key_url` plus a full `gpg_fingerprint`; only that key is imported. Unsigned
upstream packages require `allow_unsigned = true`, a `justification` and
SHA256. Debian packages require SHA256. Package name, magic and architecture
are checked before privileged installation.

Choose these native backends before `custom-script` when the vendor supplies
an artifact but no usable package repository. See [desktop apps](desktop-apps.md)
for source routes and platform limits.

`arch = ["x86_64"]` restricts package applicability. `requires = ["package"]`
selects prerequisites alongside install (including `--only`); update/remove do
not implicitly operate on shared prerequisites. `--skip` cannot omit a prerequisite while retaining its dependent.
Prerequisites install first; cycles are rejected during selection.
