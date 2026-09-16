# Fedora Asahi Remix first-run checklist

> **This is an ACCEPTANCE GATE, not a write-up.** It is walked end to end, in order, on the physical device before
> the release tag is cut. Every step below is either read-only or names the exact observation that lets the next step
> run. If an observation does not match, stop and report it — do not continue to the next step.

The target device is an Apple M1 MacBook Pro running Fedora Asahi Remix: KDE Plasma 6 on Wayland, dnf5, firewalld,
SELinux enforcing, btrfs with a root subvolume literally named `root`, and `uname -m` = `aarch64`. It is a populated
daily driver, not a scratch VM — there is real work in `~/.config`. Everything here is therefore ordered by blast
radius: read-only capture first, then read-only previews, then exactly one mutating domain at a time with a
verification between each.

For what can be rehearsed off-device beforehand, see [the Fedora container rehearsal](./fedora-rehearsal.md). This
document covers only what no container can prove.

## Step 0 — device ground truth capture (read-only)

Run this first. It mutates nothing, and it runs **before** any code that depends on a device fact is finalised — the
operator pastes the raw output back into the PR verbatim, unedited and untrimmed.

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
ls -ld ~/.config/systemd/user/tmux.service ~/.ssh/config ~/.ansible.cfg \
  ~/.branchlet/settings.json ~/.config/atuin/config.toml \
  ~/.config/lazygit/config.yml ~/.config/zed/{keymap,settings}.json 2>&1
```

What each capture settles:

| Capture                                | Settles                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| -------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `cat /etc/os-release`                  | The verbatim fixture behind the Fedora/Asahi `TestDetectOS` rows. Paste the whole block; `ID`, `ID_LIKE` and `VARIANT_ID` are all load-bearing.                                                                                                                                                                                                                                                                                                                                  |
| `uname -m -r`                          | Confirms `aarch64` and records the Asahi kernel flavour behind the 16 KiB page-size risk below.                                                                                                                                                                                                                                                                                                                                                                                  |
| `dnf --version`                        | Records dnf5 versus a dnf4 compatibility shim, so the package-manager detection result can be read against a known binary.                                                                                                                                                                                                                                                                                                                                                       |
| `rpm -qf "$(command -v chsh)"`         | Settles whether `chsh` ships on this release, and from which package — the `LookPath` / `usermod -s` fallback question for the shell sysconfig domain.                                                                                                                                                                                                                                                                                                                           |
| `command -v qdbus qdbus6 …`            | Settles which KWin reconfigure binary actually exists here, and therefore which `dotf plasma` reload argv is reachable.                                                                                                                                                                                                                                                                                                                                                          |
| `command -v gh python3`                | Records whether release **discovery** is possible at all. The shim itself needs only `curl` and `tar` (both in `@core`), but `github_download_release_file` resolves a tag _first_, and `github_resolve_latest_tag` returns 1 when neither `gh` nor `python3` exists — so with both absent the expected, documented outcome is a clean **exit 1** and the `./bootstrap.sh --install-go` backstop, not a successful download. At least one of the two must be present for Step 1. |
| `getenforce`                           | The SELinux baseline every later step is compared against.                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `sudo firewall-cmd --get-default-zone` | The pre-existing firewalld zone on a roaming laptop, before any firewall domain runs.                                                                                                                                                                                                                                                                                                                                                                                            |
| `systemctl is-enabled sshd`            | Whether the OpenSSH server is already enabled, so the `ssh-server` domain's effect is attributable.                                                                                                                                                                                                                                                                                                                                                                              |
| `findmnt -no FSTYPE /`                 | Confirms btrfs, which is the premise of the snapper discussion below.                                                                                                                                                                                                                                                                                                                                                                                                            |
| `command -v snapper`                   | Expected to find nothing. That absence is exactly why `--snapper` is on the DO NOT USE list.                                                                                                                                                                                                                                                                                                                                                                                     |
| `printenv XDG_CURRENT_DESKTOP …`       | Confirms a real Plasma 6 Wayland session, the gate `dotf plasma` and the apply Plasma phase both key off.                                                                                                                                                                                                                                                                                                                                                                        |
| `dnf list --available …`               | Settles the package mapping table's release header — which of the named packages Fedora's own repos actually carry on this release.                                                                                                                                                                                                                                                                                                                                              |
| `ls -d ~/.config/… ~/.bin`             | Records which managed destinations already exist as real directories. `dotf link` deletes each of these recursively, and as of ADR-11 `--backup` copies every one of them first.                                                                                                                                                                                                                                                                                                 |
| `ls -ld ~/.config/systemd/… ~/.ssh/…`  | Records the destinations `--backup` does **not** cover, plus the newly-managed tmux user unit. A `cannot access` line is the good outcome; a real file is something Step 4 must account for and the pre-flight tarball must capture. See the uncovered table below.                                                                                                                                                                                                              |

## DO NOT USE (read before running anything)

Three flags are wrong on this host for reasons that are not obvious from their help text.

- **`--hyprland`** — and its deprecated alias `--linux-desktop` — on `dotf link` or `dotf apply`. It deploys the
  Hyprland/wlroots desktop set (`internal/symlink/table.go`, facet `FacetHyprland`) onto a KDE host. The
  CachyOS-branded `arch-update.desktop` is no longer part of that set — it moved to `FacetArchDesktop` — but the rest
  of it (hypr, waybar, swayosd, mako, wlogout, fuzzel, satty) is categorically wrong on Plasma 6. Use `dotf plasma`
  instead.
- **`--snapper`** on `dotf apply`. `cmd/dotf/applyphases.go` states verbatim that snapshot failures are "NEVER
  recorded in the failure collector", and `internal/snapper/snapper.go` gates only on the Linux family plus
  `LookPath("snapper")` — there is no btrfs check and no snapper-config check, and the emitted command is
  `sudo snapper create` with no `-c <config>`. Fedora ships neither snapper nor a config, so the run exits **0**
  having protected nothing. A green exit code here means the opposite of what it looks like.
- **`dotf plasma apply --reload`** on the first pass. Run the printed `plasma.reload.<n>=` commands by hand instead:
  the visible result on a live Plasma 6 session — panels, KWin, look-and-feel — cannot be judged headlessly, and a
  component restart mid-session is not something to discover by surprise.

## Backup behaviour — what `dotf link --backup` now covers

As of ADR-11, `dotf link --backup` **does** back up directory destinations before replacing them.
`internal/symlink/symlink.go` still removes a real file or directory at a managed destination recursively, but
`BackupEligible` now covers **30** of the table's 41 entries: the 14 shell/common/prompt single-file entries, **all
15** directory entries, and the `.config/systemd/user/tmux.service` user unit. The plan orders `ActionBackup` before
`ActionRemove`, and the backup lands under `$HOME/.config/dotfiles_backup_<epoch>` preserving the `$HOME`-relative
path.

Every `IsDir` entry is now eligible whatever its facet, so there is no longer any directory that `--backup` fails to
copy before deleting. The remaining **11** uncovered entries are all single files:

| Uncovered destination                               | Applies here?                        |
| --------------------------------------------------- | ------------------------------------ |
| `.ssh/config`                                       | **yes** — the one that matters most  |
| `.ansible.cfg`                                      | yes                                  |
| `.branchlet/settings.json`                          | yes                                  |
| `.config/atuin/config.toml`                         | yes                                  |
| `.config/lazygit/config.yml`                        | yes                                  |
| `.config/zed/keymap.json`                           | yes                                  |
| `.config/zed/settings.json`                         | yes (the copy exception, not a link) |
| `.local/share/applications/alacritty-float.desktop` | no — `FacetHyprland`, not used here  |
| `.local/share/applications/gedit-float.desktop`     | no — `FacetHyprland`, not used here  |
| `.local/share/applications/wifi-tui.desktop`        | no — `FacetHyprland`, not used here  |
| `.local/share/applications/arch-update.desktop`     | no — `FacetArchDesktop`              |

Therefore: **always pass `--backup`**, and read every destination on the `link.remove=` line of the dry run against
that 11-row table. Anything on `link.remove=` that appears there is deleted with no copy kept; everything else is
copied first.

`--backup` is what makes the archive below optional for the directories. It is still **mandatory** for the seven
uncovered files that apply here, which is exactly what this archive now captures:

```bash
set --
for p in .ssh/config .ansible.cfg .branchlet/settings.json \
  .config/atuin/config.toml .config/lazygit/config.yml \
  .config/zed/keymap.json .config/zed/settings.json; do
  [ -e "$HOME/$p" ] && set -- "$@" "$p"
done
if [ "$#" -gt 0 ]; then
  tar czf ~/dotfiles-preflight-"$(date +%Y%m%d%H%M)".tar.gz -C "$HOME" "$@"
  echo "archived $# path(s)"
else
  echo "nothing to archive"
fi
```

The `[ -e ]` loop, rather than a glob or an `ls`, is deliberate: it exits 0 whether or not a given path exists on
this host, and it is immune to the `ls`→`eza` alias **this repo itself installs** — which would otherwise feed
colour escapes into `tar`'s argument list.

That tarball, plus the `$HOME/.config/dotfiles_backup_<epoch>` directory, is the rollback. **Never** `--snapper`.

## Ordered checklist

1. **Download the binary with `GH_TOKEN` deliberately unset.**

   ```bash
   env -u GH_TOKEN -u GITHUB_TOKEN ./bootstrap.sh --dry-run --headless
   ```

   Scope of this step: **`gh` absent, `python3` present** — confirm against the Step 0 `command -v gh python3`
   capture before running it. This must obtain `dotf_linux_arm64.tar.gz` with `gh` absent and no token in the
   environment, using `python3` to parse the release JSON. If Step 0 showed `python3` absent too, this step cannot
   pass and is not a failure of the shim — skip to the `--install-go` backstop below. If the release origin is
   private, or the newest release is a prerelease **and** you need an asset-id lookup, `GH_TOKEN`/`GITHUB_TOKEN` is
   additionally required; a public prerelease resolves through the tag-pinned URL with no token.

   Note the shim consumes only `--repo` and `--install-go` and forwards everything else to `dotf`, injecting `apply`
   when the first remaining argument starts with a dash — so the command above runs
   `dotf apply --dry-run --headless`. Record whether the download succeeded: this is the one claim no CI job can
   make. Documented backstop on failure, from a clone, which installs official Go under `~/.local/go` with no sudo
   and builds from source:

   ```bash
   ./bootstrap.sh --install-go
   ```

2. **Detection.** `dotf platform` prints `os=<os> arch=<arch> pkg=<pkg> container=<c> distro=<distro>`
   (`cmd/dotf/main.go`). Expect `os=redhat arch=arm64 pkg=dnf` and record `distro=` verbatim. On aarch64 Linux the
   native package manager now wins over an incidental Linuxbrew, so `pkg=dnf` is expected **even with brew on
   `PATH`**. If `pkg=` is anything other than `dnf`, stop and report it — that is a detection bug, not an expected
   condition.

3. **Doctor.** `dotf doctor` — read-only. It now emits an informational `check.selinux.mode=ok` row whose detail
   (visible with `--verbose`) is `selinux=<mode>`. Record `getenforce` before and after every mutation from here on,
   and treat a flip from Enforcing to Permissive as stop-the-line.

4. **`dotf link --dry-run --backup`** — still read-only. The `--backup` on a dry run is what makes the `link.backup=`
   line appear, so the two lines can be compared directly. Do **not** pass `--hyprland`.

   Read `link.remove=` against `link.backup=`: every entry on `link.remove=` that is _missing_ from `link.backup=` is
   deleted with no copy kept, and must appear in the 11-row uncovered table above.

   `.config/systemd/user/tmux.service` is newly managed here — ADR-12 retagged it from `FacetArch` to `FacetLinux`,
   so on Fedora it was untouched before this change. If Step 0 showed a real file at that path it must appear on
   **both** `link.remove=` and `link.backup=`; if the path did not exist it appears on `link.create=` instead. It
   must never appear on `link.remove=` alone.

5. **`dotf apply --dry-run --headless`** — must exit 0. Among the `plan.*` lines, expect these two verbatim:

   ```text
   plan.repo=docker-ce
   plan.not-applicable=lazygit,yazi
   ```

   `docker-ce` moved to the family-dispatching `repo` backend with a `dnf_repo` sub-table, so on a dnf host it **is**
   planned and it appears under `plan.repo=`. No `plan.apt-repo=` line is emitted anywhere — `printApplyDryRun` keys
   the line on the backend name, and `apt-repo` is not this host's backend. A grep for `plan.apt-repo=` therefore
   passes vacuously and proves nothing; check for `plan.repo=docker-ce` instead.

   `plan.not-applicable=` is the honest statement of what this host will skip; read it in full and expect only
   `lazygit,yazi` (both are served by the `lazygit-bin` / `yazi-bin` github-release siblings).

6. **First mutation, one domain at a time.** Never a single combined run:
   1. `dotf sysconfig` per domain — `--hostname`, `--timezone`, `--locale`, `--shell`, `--firewall`, `--ssh-server` —
      one invocation each, each preceded by its own `--dry-run`.
   2. Packages. **This phase adds a third-party vendor repository to the system**, which is the largest blast radius
      in the whole run and the only step here that writes outside `$HOME` and outside systemd's own config. Because
      `docker-ce` resolves to the `repo` backend, `dotf` will:

      ```text
      curl -fsSL https://download.docker.com/linux/fedora/docker-ce.repo
      sudo install -m 644 <tmp> /etc/yum.repos.d/docker-ce.repo
      sudo dnf install -y docker-ce
      ```

      `sudo install` (rather than a `mv` from `/tmp`) is deliberate — the file is created at the destination and so
      inherits the correct SELinux type label, where a move would carry `tmp_t` across and leave dnf unable to read
      it. Re-run `getenforce` afterwards, and confirm the label with `command ls -Z /etc/yum.repos.d/docker-ce.repo`
      — `command` because by this point `ls` may already be aliased to `eza`, which has no `-Z`. To defer Docker
      entirely and keep the rest of the package phase:

      ```bash
      dotf apply --skip docker-ce
      ```

      Verified on this branch: `--skip docker-ce` drops it from the `packages=` line and emits no `plan.repo=` line
      at all.

   3. Symlinks: `dotf link --backup`.

   Re-run `getenforce` after each of those, and re-run `sudo firewall-cmd --get-default-zone` after the firewall
   domain. Confirm the `dotfiles_backup_<epoch>` directory exists and contains what `link.backup=` promised before
   opening a new shell.

## Residual risks only the device can settle

Every row here is something no container, no CI job and no emulator can answer. Record the observed result against
each one during the walk-through.

| Risk                                   | Why only the device settles it                                                                                                                                                                                                                                                                                                                                                                                                                          |
| -------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Verbatim `ID` / `VARIANT_ID`           | The Asahi Remix `os-release` is not the stock Fedora one, and the detection fixtures are transcribed from whatever Step 0 prints.                                                                                                                                                                                                                                                                                                                       |
| `chsh` presence and owning package     | Whether the shell domain takes the `chsh` path or the `usermod -s` fallback depends on what this release actually ships.                                                                                                                                                                                                                                                                                                                                |
| The download itself                    | A prerelease origin with `gh` absent and no token — resolved through `python3` and the tag-pinned public URL — is exactly the case CI never exercises. (With `gh` **and** `python3` both absent the shim cannot discover a tag at all; that is a documented exit 1, not a device finding.)                                                                                                                                                              |
| SELinux under real provisioning        | Enforcing mode against real writes into `$HOME` and into existing system inodes. Containers run with a different policy or none.                                                                                                                                                                                                                                                                                                                        |
| 16 KiB page size                       | Asahi's kernel uses 16 KiB pages. Any C-linked third-party binary installed during provisioning may assume 4 KiB; nothing off-device reproduces this.                                                                                                                                                                                                                                                                                                   |
| A live Plasma 6 session                | `dotf plasma` writes are judged by what the running session looks like afterwards, and by what the `--reload` component restarts do to it.                                                                                                                                                                                                                                                                                                              |
| `hostnamectl set-hostname`             | Needs a real, writable systemd-hostnamed on a machine whose hostname matters.                                                                                                                                                                                                                                                                                                                                                                           |
| btrfs and snapper                      | A btrfs root subvolume literally named `root`, with no snapper and no snapper config — the exact combination that makes `--snapper` a silent no-op.                                                                                                                                                                                                                                                                                                     |
| arm64 **plus** systemd                 | Unreachable under `qemu-user`: `dbus-broker` dies `226/NAMESPACE`. A full `qemu-system-aarch64` TCG VM is out of scope, so systemd-on-arm64 is a device-only observation.                                                                                                                                                                                                                                                                               |
| The real NetworkManager/firewalld zone | A roaming laptop's active zone depends on the network it is on when the firewall domain runs; the captured default zone is only the starting point.                                                                                                                                                                                                                                                                                                     |
| The Fedora `/etc/bashrc` severance     | Fedora's bash is not built with `-DSYS_BASHRC`, so `/etc/bashrc` — and therefore `/etc/profile.d/*.sh` — is pulled in only by the skel `~/.bashrc` that `dotf link` replaces, for exactly the interactive non-login shell a Konsole tab starts. This was deliberately **not** changed: adding a guarded source is a behaviour change to the login path on four platforms. Record whether anything in `/etc/profile.d` turns out to matter on this host. |
