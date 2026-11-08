# Desktop apps

Desktop apps are opt-in and require a graphical session. On macOS they are always applicable; Linux needs a detected X11/Wayland session.

```bash
dotf install --only 1password
dotf install --only chatgpt --only cursor
dotf install --intent desktop
dotf apply --intent desktop
dotf apply --dry-run --intent desktop
dotf install --update --only obsidian
dotf install --remove --only obsidian
```

Use repeated `--only` flags for multiple apps. Bare app names select their exact `-arch`, `-debian`, `-linux`, `-macos`, `-fedora` and `-arm64` siblings. Explicit sibling names select that entry only.

## Sources

| App | Fedora | macOS | Arch |
| --- | --- | --- | --- |
| 1Password | Official vendor repo | Official Homebrew cask | AUR |
| ChatGPT | Official OpenAI repo | Homebrew cask | AUR `chatgpt-desktop-bin` |
| Cursor | Official Anysphere repo | Homebrew cask | AUR `cursor-bin` |
| Grok Bot | Vendor CDN repo | Homebrew cask | AUR `grok-bot-bin` |
| WezTerm | Author's nightly COPR | Homebrew cask | Distro package |
| Ghostty | Community COPR listed by Ghostty | Homebrew cask | Distro package |
| Herdr (`herdr-app`) | Official rolling installer | Official rolling installer (Apple silicon) | Official rolling installer |
| Zed Preview | Official rolling preview tarball | `zed@preview` cask | Official rolling preview tarball |
| LM Studio | Official rolling AppImage | Homebrew cask | Official rolling AppImage |
| Obsidian | Official rolling AppImage | Homebrew cask | Official rolling AppImage |
| LocalSend | Upstream rolling AppImage | Homebrew cask | Upstream rolling AppImage |
| Discord | Official rolling tarball | Homebrew cask | Official rolling tarball |
| Google Chrome | Google repo | Homebrew cask | AUR |
| VS Code Insiders (`code-insiders`) | Microsoft repo | Homebrew cask | AUR |
| Slack | Signed official rolling RPM | Homebrew cask | AUR `slack-desktop` |
| Dropbox | Vendor repo | Homebrew cask | AUR |
| Zoom | Signed vendor RPM | Homebrew cask | AUR |
| Teams for Linux | Upstream rolling RPM (unsigned) | Official Microsoft Teams cask | AUR |
| Beekeeper Studio | Upstream rolling RPM (unsigned) | Homebrew cask | AUR |
| WinBoat | Upstream rolling RPM (unsigned) | Not supported | AUR |
| WhatsApp | Excluded | Official cask (`whatsapp-macos`) | Excluded |

On Linux arm64 (for example Fedora Asahi Remix), ChatGPT and Google Chrome use the same vendor repos (arch-templated), and Zed Preview, LM Studio, Obsidian, Beekeeper Studio and Teams for Linux use `-arm64` sibling entries pointing at upstream's arm64/aarch64 build. The remaining apt/dnf and artifact routes are gated to `x86_64` and are not-applicable there: 1Password (the arm64 vendor repos carry only the CLI), LocalSend (upstream ships an arm64 `.tar.gz` and `.deb` but no arm64 AppImage, and the manifest has no arm64 entry), Discord, Slack, Zoom and WinBoat.

Obsidian's arm64 AppImage runtime links the unversioned `libz.so`, which Fedora and Debian ship only in their zlib development package (Arch's `zlib` includes it). `obsidian-arm64` therefore requires `appimage-libz` (`zlib-ng-compat-devel` on Fedora, `zlib1g-dev` on Debian, `zlib` on Arch). Without it the AppImage exits with `libz.so: cannot open shared object file` before Electron starts.

On Fedora and Fedora remixes, Claude Desktop installs through `bin/install-claude-desktop-rpm.sh` on `x86_64` and `arm64`. The script repacks the signature-verified official `.deb` into a local RPM. Anthropic does not support that RPM.

AUR entries carry no `arch` gate, so on an Arch Linux arm64 host the AUR siblings (`1password`, `slack-arch`, `zoom-arch`, `winboat-arch` and the rest) are still applicable. dotf does not read a PKGBUILD's `arch` array, so an x86_64-only AUR package fails at build time there instead of being skipped.

OBS Studio, GIMP, darktable, Kdenlive, Meld, Pinta, VLC, Qalculate, Fedora Media Writer and LibreOffice use distro packages on Linux and Homebrew on macOS where available. Fedora Pinta can lag upstream. VLC may need RPM Fusion on older Fedora releases.

## Herdr app

Install the GUI with the desktop intent or select it directly:

```bash
dotf install --intent desktop
dotf install --only herdr-app
# Re-run the official installer to install the latest app, even when present:
dotf install --only herdr-app --force
```

`herdr-app` supports Linux `x86_64` and `arm64`; its `herdr-app-macos` sibling
supports macOS `arm64` (Apple silicon). Intel macOS and Windows are not applicable
to these manifest entries. Linux still requires a detected graphical session.
The existing `herdr` and `herdr-macos` CLI packages in the `dev` intent are unchanged.

The [official download page](https://herdr.dev/download/) supplies the
[`script-url` installer](https://releases.herdr.dev/app/install.sh). It installs
the app and `~/.local/bin/herdr-app`, registers the Linux launcher or macOS app,
and attempts to bootstrap `herdr-core` only when absent. If core installation
fails, the app offers it when opened; an existing core is left alone, and the app
keeps it current. dotf probes `herdr-app` on PATH, so keep `~/.local/bin` on PATH.

This is a rolling installer, deliberately without a pinned script hash: dotf
trusts HTTPS for the script, while the installer verifies the app download against
the SHA256 from its release manifest. `script-url` does not support dotf's
`--update` or `--remove`; use the `--force` reinstall above to update this app.
Consequently, `dotf install --update --intent desktop` cannot update every entry.

After launch, the app supplies `~/.config/herdr-app/AGENTS.md`; it is not an
installation input or a repo-managed file. See the
[Herdr app configuration guide](../../herdr-app/README.md) for the files managed
by `dotf link`.

## Native artifact installs

`app-artifact` installs Linux AppImages/tarballs under `$XDG_DATA_HOME/dotf/apps/<name>` (default `~/.local/share/dotf/apps`) and writes a dotf-owned desktop entry. No root is needed. Updates download and verify a new copy before replacing the old one. Removal touches dotf-owned paths only. AppImage entries require `appimage-runtime`, so `--only` also installs FUSE 2 support. Do not assume every app's internal updater replaces its full runtime: use `dotf install --update --only <app>`. Zed's own updater remains enabled; a later `dotf install --update --only zed-preview` replaces it with the latest upstream preview release.

The GitHub-hosted artifacts (Zed Preview, Obsidian, LocalSend, Teams for Linux, Beekeeper Studio and WinBoat) are `class = "rolling"` with no `tag`: every install or update resolves the latest GitHub release (the newest prerelease for Zed Preview) and verifies the download against the SHA256 digest GitHub reports for that asset. That digest provides integrity, not a vendor signing identity, and no version is pinned in the manifest. Rolling LM Studio and Discord downloads trust vendor HTTPS and have no published SHA256.

`pkg-url` verifies RPM/DEB magic, package name and architecture before invoking the host package manager. Signed RPMs require a verified key fingerprint and a valid RPM signature. Upstream unsigned RPMs require explicit `allow_unsigned` metadata plus a checksum (a manifest `sha256`, or the GitHub asset digest on the rolling route); this exception is documented per package. Imported signing keys remain after uninstall because other packages may share them.

## Virtualization and exclusions

VMware was replaced by `virt-manager`. Selecting it also installs QEMU/KVM and libvirt runtime packages. dotf does not enable services, change group membership, firmware or networking. Run `sudo systemctl enable --now libvirtd` (or distro equivalent) and add your user to the `libvirt` group when appropriate. WinBoat likewise needs its container/virtualization prerequisites and a valid Windows license; installation does not create a Windows VM.

DaVinci Resolve is excluded by choice. Linux WhatsApp wrappers and snapd are excluded; use WhatsApp Web. Bundled KDE/CachyOS tools were not added during inventory review.

## Verification limits

Manifest selection and fixture-based installer tests do not prove an app starts on every distro. Fedora, Arch and macOS device acceptance remains separate from Go tests. Remote inventory of `cachy-g7` was read-only.

### Fedora Asahi Remix (16k pages)

Launch-tested on 2026-10-21 on Fedora Asahi Remix 44, kernel `7.1.13-402.asahi.fc44.aarch64+16k` (`getconf PAGESIZE` = 16384), KDE Plasma Wayland. Each app was installed with `dotf install --only <name>` and started from its desktop-entry `Exec` command. An app counts as launched when, 25 s later, its process is still running and KWin has mapped a window for its PID.

| App | Version | Launches | Notes |
| --- | --- | --- | --- |
| Claude Desktop (Fedora repack) | 2.31226.1 | Yes | Window "Claude" |
| ChatGPT | 26.1013.40934 | Yes | Window "ChatGPT" (X11 via Xwayland) |
| Google Chrome | 155.0.8059.84 | Yes | Window "New Tab" |
| Obsidian | 1.14.4 | Yes, with `appimage-libz` | Without `libz.so` the AppImage runtime fails to load; no 16k-page fault |
| LM Studio | 0.4.26 | Yes | Window "LM Studio" |
| Zed Preview | 1.25.1 | Yes | Window "empty project"; harmless xkbcommon Compose warnings |
| Beekeeper Studio | 6.1.6 | Yes | Window "Beekeeper Studio"; the vendor RPM's `%post` key import fails under dnf's RPM lock |
| Teams for Linux | 2.24.0 | Yes | Window "Sign in to your account"; desktop entry forces `--ozone-platform=x11` |

No app failed on 16k pages, so none needs a page-size gate. Electron apps can leave a `SIGTRAP` core dump when they are killed. That happens on shutdown and does not affect launch.
