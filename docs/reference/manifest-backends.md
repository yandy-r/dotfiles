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
     `repo_line` templates host values: `{{.Codename}}` (os-release `VERSION_CODENAME`),
     `{{.UbuntuCodename}}` (`UBUNTU_CODENAME`, so Mint/elementary resolve to their
     Ubuntu base), `{{.BaseDistro}}`/`{{.BaseCodename}}` (`ubuntu`+`noble` on Ubuntu
     and its derivatives, `debian`+`bookworm`/`trixie` on Debian) and `{{.Arch}}`.
     Use `{{.UbuntuCodename}}` for a vendor that publishes Ubuntu dists only, and the
     `BaseDistro`/`BaseCodename` pair for one that publishes both trees; other
     placeholders fail validation. If `apt-get update` fails after the sources entry
     is written, dotf removes the sources file and keyring it created (or restores
     the ones it replaced) before reporting the error. Required
     `key_fingerprint` (40 hex) must name a primary key in the fetched key file,
     checked with `gpg --show-keys` before any `sudo` step; only that key (with its
     subkeys) is exported into the `signed-by` keyring.
   - `dnf_repo` — Fedora/RHEL, in exactly one of three modes:
     - `repofile = "<https url>"` (+ `id`) — the vendor publishes a ready-made `.repo`;
       **prefer this.** It is installed verbatim at `/etc/yum.repos.d/<id>.repo`, below
       dotf's ownership marker line.
     - `baseurl = "…"` (+ `id`, optional `gpgkey`) — no vendor `.repo` exists; dotf renders
       the repo body. `baseurl`/`gpgkey` template `{{.ReleaseVer}}` (os-release `VERSION_ID`)
       and `{{.Arch}}`. Optional `repo_gpgcheck = true` (requires `gpgkey`) adds
       `repo_gpgcheck=1`, so dnf also verifies the signed repository metadata
       (`repodata/repomd.xml.asc`); set it only when the vendor publishes that signature,
       or dnf refuses the repo.
     - `copr = "owner/project"` — the tool exists only in COPR (see below).

     Both file modes write `/etc/yum.repos.d/<id>.repo` with a first line
     `# Managed by dotf (repo backend); …` marking it dotf-owned. On later runs dotf
     rewrites the file only when it is missing, starts with that marker, or (for a
     `baseurl` entry with `repo_gpgcheck = true`) is byte-identical to the unmarked body an
     older dotf rendered for it. Any other existing file (e.g. one a vendor package
     installed) is left untouched — `repofile` is not even fetched — and the package is
     still installed from it. When that file has a section named exactly `[<id>]`, both
     install and update (`dnf install` / `dnf upgrade`) add `--enablerepo=<id>`, so a
     section shipped with `enabled=0` (as Fedora Workstation's `google-chrome.repo` is)
     still works. Other sections are never enabled. Delete the marker line to keep a
     hand-edited file. A failure to read the existing file aborts the install or update.

     Optional `priority = <1..98>` (either file mode) pins the package to this repo when
     another enabled repo also ships it: dnf only considers a package name from the repo
     with the lowest priority value (default 99). Without it, a rebuild in e.g. Terra can
     win on version, or its exact-version subpackages (Terra's `cursor-*-completion`) make
     `dnf upgrade` skip the package ("cannot install the best update candidate"). dotf's
     own `dnf install` / `dnf upgrade` add `--setopt=<id>.priority=<n>`; on dnf5, install
     and update also write the dotf-owned drop-in
     `/etc/dnf/repos.override.d/90-dotf-<id>.repo`, so a plain `dnf upgrade` honours the
     pin even when `/etc/yum.repos.d/<id>.repo` is a kept vendor file. A drop-in without
     the marker line is left untouched. Moving an already-installed package between repos
     is still manual: remove the other repo's version-pinned subpackages first.

     Optional `exclude_from = ["<repo id>", …]` (any mode, including `copr`) hides
     `<name>` and `<name>-*` (`name` = `names.dnf`) in those repos. Reach for it when
     another repo splits the package into differently-named subpackages that collide
     with files the package ships — e.g. Terra's `ghostty-terminfo`,
     `ghostty-shell-integration` and `ghostty-*-completion` against the copr `ghostty`
     1.4.0+, which fails with rpm file conflicts (`priority` cannot stop those, since
     the names differ). dotf's own commands add `--setopt=<repo>.excludepkgs=…` for the
     listed repos the host has configured (dnf5 rejects `--setopt` for an unknown repo
     id). On dnf5, install and update also write the dotf-owned drop-in
     `/etc/dnf/repos.override.d/90-dotf-exclude-<name>.repo` for every listed repo, so a
     repo added later is covered; dnf5 ignores sections for missing repos and appends
     `excludepkgs` across drop-ins, so your own excludes are kept.

   A host whose package manager matches no sub-table reports `applicable=false` — it is
   silent, not an error. The former `apt-repo` alias was removed: set `backend = "repo"`
   (a manifest entry still naming `apt-repo` fails validation).

4. **`toolchain`** — installable via a language toolchain (`cargo`, `npm`). Set `tool`.
5. **`github-release`** — a GitHub release ships a checksummed binary/asset. Supports
   asset auto-detection and a shared `checksums.txt` sidecar (`checksums_asset`).
   `dotf install --update` re-installs that pinned or locked asset into
   `~/.local/bin`. Changing the version means changing the manifest tag or the
   lock, then updating.
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
   `dotf install --update` re-fetches, re-verifies and re-runs the installer (a vendor
   installer installs the current release, so re-running it is the update); `--remove`
   is not supported.
8. **`git-clone`** — the install _is_ a clone into `$HOME` (e.g. tpm), with optional
   typed `post_install` activation steps.
9. **`custom-script`** — **last resort**: a hardened `bin/install-*.sh` for install logic
   none of the above expresses. Governed (see below).

Any backend accepts `requires_desktop = true` for GUI apps: the package is
`applicable=false` unless dotf sees a graphical session (macOS, or Linux with
`XDG_SESSION_TYPE=x11|wayland`, `WAYLAND_DISPLAY`, or a live `wayland-*` socket
in `$XDG_RUNTIME_DIR` — the last covers tmux/herdr/SSH shells on a machine whose
desktop is running). It applies to `--only` too. Override detection with
`DOTF_DESKTOP=1|0` (any command), or pass `--desktop` to `install`/`apply`, e.g.
for an X11 desktop reached from a shell without its session variables.

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
apt_repo = { key_url = "…/gpg", key_fingerprint = "9DC858229FC7DD38854AE2D88D81803C0EBFCD88", keyring = "/etc/apt/keyrings/docker.gpg", repo_line = "deb [arch={{.Arch}} signed-by=/etc/apt/keyrings/docker.gpg] …/{{.BaseDistro}} {{.BaseCodename}} stable", sources_path = "/etc/apt/sources.list.d/docker.list" }
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

### Example: `github-mcp-server`

`github-mcp-server` (in the opt-in `ai` intent) is a native pinned
`github-release`: repo `github/github-mcp-server`, tag `v2.0.2`, binary
`github-mcp-server`, checksums from `github-mcp-server_2.0.2_checksums.txt`. No
`asset` is set — the upstream `Linux`/`Darwin` × `x86_64`/`arm64` flat-binary
`tar.gz` archives are auto-detected for the host, and the sha256 is verified from
the sidecar. It needs no `custom-script` entry or backend change.

```sh
dotf install --intent ai --only github-mcp-server --dry-run
dotf install --intent ai --only github-mcp-server
```

This installs the binary to `~/.local/bin` only. GitHub credentials (e.g. a
personal access token) and MCP client configuration are separate and are not
managed by this entry.

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
(`icon`, `desktop_name`, optional `desktop`, `categories`, `exec_args`,
`startup_wm_class` and `mime_type`; the last three override the same keys from a
vendor `.desktop` file, and `Exec`/`Icon` always stay dotf-owned). Archives need
`binary` relative to their root. A bare AppImage is stored as
`<name>.AppImage` (no upstream version in the filename), so apps that update
themselves in place (electron-updater) keep the dotf launcher valid. SHA256 pins immutable artifacts; rolling
vendor URLs rely on HTTPS. ELF architecture, archive paths and desktop values
are checked before activation. Install/update stages the full artifact before
replacing the old copy; remove touches only dotf-owned paths.

Both `app-artifact` and `pkg-url` also take a rolling GitHub route: `repo` +
`asset` with `class = "rolling"` and no `tag`. `asset` is then a glob (Go
`filepath.Match`, not a `{{.Arch}}` template) matched against the asset names of
the repo's latest stable release; validation rejects an empty, malformed or
`{{`-bearing `asset` on this route. `preview = true` selects the newest non-draft
prerelease among the 20 most recent releases instead, and is rejected anywhere
else. Assets whose name carries a foreign-arch token (`arm64`/`aarch64`/`arm-64`
on an x86_64 host; `x86_64`/`x86-64`/`amd64`/`x64` on an arm64 host; 32-bit `arm`
variants on both) and no host-arch token are skipped, so one glob resolves a
multi-arch release. An asset with no arch token is never skipped, so
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
SHA256. Debian packages require a checksum: a manifest `sha256`, or a
`checksums`/`checksums_asset` source on a repo route. A DEB on the rolling GitHub
route needs no separate manifest checksum source, because that route always
requires and verifies GitHub's `sha256:` asset digest. Pinned, tagged and
non-GitHub DEB routes remain checksummed and fail closed. Package name, magic and
architecture are checked before privileged installation.

Choose these native backends before `custom-script` when the vendor supplies
an artifact but no usable package repository. See [desktop apps](desktop-apps.md)
for source routes and platform limits.

`arch = ["x86_64"]` restricts package applicability. `exclude_os = ["debian"]` is a
static host gate alongside `os`, `arch` and `requires_desktop`: the package is
not applicable (install, update, remove, status) on a host matching ANY listed
token. Tokens and semantics are those of `os` (`linux` matches every Linux-family
host; `wsl` is not accepted); unknown or empty values are rejected on load. The
match is on the OS family only, never the primary package manager, so it lets a
distro-native sibling (such as an official `.deb` on Debian, even with Linuxbrew
primary) take over a host that would otherwise also match a generic artifact
route. `requires = ["package"]`
selects prerequisites alongside install (including `--only`); update/remove do
not implicitly operate on shared prerequisites. `--skip` cannot omit a prerequisite while retaining its dependent.
Prerequisites install first. Requires cycles are rejected when the manifest is
loaded or merged, on every host, even through an `os`/`arch`-gated package.
