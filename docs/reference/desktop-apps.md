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

AUR entries carry no `arch` gate, so on an Arch Linux arm64 host the AUR siblings (`1password`, `slack-arch`, `zoom-arch`, `winboat-arch` and the rest) are still applicable. dotf does not read a PKGBUILD's `arch` array, so an x86_64-only AUR package fails at build time there instead of being skipped.

OBS Studio, GIMP, darktable, Kdenlive, Meld, Pinta, VLC, Qalculate, Fedora Media Writer and LibreOffice use distro packages on Linux and Homebrew on macOS where available. Fedora Pinta can lag upstream. VLC may need RPM Fusion on older Fedora releases.

## Native artifact installs

`app-artifact` installs Linux AppImages/tarballs under `$XDG_DATA_HOME/dotf/apps/<name>` (default `~/.local/share/dotf/apps`) and writes a dotf-owned desktop entry. No root is needed. Updates download and verify a new copy before replacing the old one. Removal touches dotf-owned paths only. AppImage entries require `appimage-runtime`, so `--only` also installs FUSE 2 support. Do not assume every app's internal updater replaces its full runtime: use `dotf install --update --only <app>`. Zed's own updater remains enabled; a later `dotf install --update --only zed-preview` replaces it with the latest upstream preview release.

The GitHub-hosted artifacts (Zed Preview, Obsidian, LocalSend, Teams for Linux, Beekeeper Studio and WinBoat) are `class = "rolling"` with no `tag`: every install or update resolves the latest GitHub release (the newest prerelease for Zed Preview) and verifies the download against the SHA256 digest GitHub reports for that asset. That digest provides integrity, not a vendor signing identity, and no version is pinned in the manifest. Rolling LM Studio and Discord downloads trust vendor HTTPS and have no published SHA256.

`pkg-url` verifies RPM/DEB magic, package name and architecture before invoking the host package manager. Signed RPMs require a verified key fingerprint and a valid RPM signature. Upstream unsigned RPMs require explicit `allow_unsigned` metadata plus a checksum (a manifest `sha256`, or the GitHub asset digest on the rolling route); this exception is documented per package. Imported signing keys remain after uninstall because other packages may share them.

## Virtualization and exclusions

VMware was replaced by `virt-manager`. Selecting it also installs QEMU/KVM and libvirt runtime packages. dotf does not enable services, change group membership, firmware or networking. Run `sudo systemctl enable --now libvirtd` (or distro equivalent) and add your user to the `libvirt` group when appropriate. WinBoat likewise needs its container/virtualization prerequisites and a valid Windows license; installation does not create a Windows VM.

DaVinci Resolve is excluded by choice. Linux WhatsApp wrappers and snapd are excluded; use WhatsApp Web. Bundled KDE/CachyOS tools were not added during inventory review.

## Verification limits

Manifest selection and fixture-based installer tests do not prove an app starts on every distro. Fedora, Arch and macOS device acceptance remains separate from Go tests. Remote inventory of `cachy-g7` was read-only.
