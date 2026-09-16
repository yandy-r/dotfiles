# DaVinci Resolve Studio - Manual Install Guide (CachyOS)

This is a manual, read-along runbook. Run each step by hand and confirm it works on
this machine first; once the install is verified end-to-end (and uninstalled again),
these steps get turned into a scripted `bin/install-davinci-resolve.sh` per
[section 9](#9-notes-for-the-future-bininstall-davinci-resolvesh) below. Nothing in
this document is executed automatically — every command block is meant to be
copy-pasted into a terminal by you.

## Why this machine is a special case

- **OS**: CachyOS (Arch-based), kernel `7.1.2-3-cachyos`, running a **Wayland**
  session (`XDG_SESSION_TYPE=wayland`). DaVinci Resolve does not ship a working Qt
  Wayland plugin, so it must be forced onto the `xcb` (X11) Qt platform, otherwise it
  fails to start or misrenders.
- **GPUs**: this machine has three GPUs in play:
  - Intel iGPU (i9-14900HX).
  - **NVIDIA GeForce RTX 5070 Mobile** — the intended compute GPU for Resolve.
    `opencl-nvidia` and `linux-cachyos-nvidia-open` are already installed.
  - **AMD Radeon RX 9070 eGPU** — this is the GPU actually driving the desktop and
    compositor. AMD has no ROCm/OpenCL runtime installed here and RDNA4 compute
    support is still immature, so it is **not** used for Resolve's GPU processing.
- Because the desktop lives on the AMD eGPU while Resolve needs the NVIDIA GPU for
  CUDA/OpenCL, Resolve must be launched with **PRIME render offload** environment
  variables so it renders on NVIDIA instead of picking up the AMD device that owns
  the display. This is handled by the launch wrapper in
  [section 5](#5-create-a-launch-wrapper-with-the-correct-env-nvidia-offload--x11).
- The following prerequisites have already been verified as installed on this
  machine and are **not** reinstalled by this guide: `opencl-nvidia`,
  `lib32-opencl-nvidia`, `libxcrypt-compat`, `lib32-libxcrypt-compat`,
  `pipewire-alsa`, the `en_US.utf8` locale, `paru` (v2.1.0), and `yay` (v13.0.1).
  `clinfo` is the one pre-flight tool that is **not** yet installed; it gets
  installed in [section 1](#1-pre-flight-verification).

## 1. Pre-flight verification

Confirm the prerequisites that are already satisfied on this machine. These checks
should all pass without doing anything — they exist so the state is documented and
reproducible, not because you need to install anything here:

```bash
pacman -Q opencl-nvidia lib32-opencl-nvidia libxcrypt-compat lib32-libxcrypt-compat pipewire-alsa
locale -a | grep -i en_US.utf8
paru --version
yay --version
```

`clinfo` is not currently installed on this machine, so install it now for GPU
validation:

```bash
sudo pacman -S --needed clinfo
```

Optionally, install the community checker (useful on single-GPU systems; see below for
this machine):

```bash
paru -S davinci-resolve-checker-git
```

Run validation:

```bash
clinfo -l
```

If you installed the checker, run it once for diagnostics (expect it to fail on this
machine — see below):

```bash
davinci-resolve-checker
```

`clinfo -l` should list the NVIDIA GPU as an OpenCL platform. Since AMD has no
OpenCL runtime installed on this machine, it should not appear as a broken or
competing platform — if it does, that is worth resolving before continuing, since
Resolve could otherwise try (and fail) to use it.

### What the checker is supposed to do (and why it stops on your machine)

On a **single-vendor** system, `davinci-resolve-checker` is a pass/fail script. It
prints your driver/GPU config, then validates things like OpenCL packages, OpenGL
vendor, AMD codename support, etc., and ends with:

```text
All seems good. You should be able to run DaVinci Resolve successfully.
```

On **your** machine it will never reach that line. The script always scans PCI for
GPUs. If it finds **both** AMD and NVIDIA, it prints:

```text
You have AMD and NVIDIA GPUs. I am confused. Which one do you intend to use?
```

and **exits immediately** (code 1). There is no prompt, no menu, and no way to
answer — it is a hard-coded bail-out in the checker, not a question to you.

**The PRIME offload env vars do not fix this.** They only affect which GPU `glxinfo`
reports (OpenGL). Your output already shows the correct answer before the exit:

```text
OpenGL vendor string: NVIDIA Corporation
OpenGL renderer string: NVIDIA GeForce RTX 5070 Laptop GPU/PCIe/SSE2
clinfo detected platforms and devices:
	NVIDIA CUDA (number of devices: 1)
```

That is the useful part. Everything above the "confused" line is the real
pre-flight; the exit is the checker giving up because it was not written for
dual-vendor rigs like yours (AMD eGPU on display + NVIDIA for compute).

**Verdict for this machine:** pre-flight passes. Proceed with the install. Ignore the
checker's exit code; use the checklist below instead:

| Check | Your expected value |
| --- | --- |
| OpenGL vendor | `NVIDIA Corporation` |
| OpenGL renderer | `GeForce RTX 5070` |
| OpenCL platform | `NVIDIA CUDA`, 1 device |
| Packages | `opencl-nvidia`, `lib32-opencl-nvidia` installed |
| AMD OpenCL | No AMD OpenCL platform listed (good — avoids wrong-GPU crashes) |

Resolve itself will use NVIDIA via the launch wrapper in section 5; the checker cannot
validate that path end-to-end on this hardware layout.

## 2. Download the Studio installer (manual, login-gated)

The Studio ZIP is behind a Blackmagic account login and cannot be scripted:

1. Go to
   [Blackmagic Design support](https://www.blackmagicdesign.com/support/family/davinci-resolve-and-fusion).
2. Choose **DaVinci Resolve Studio → Linux**.
3. Log in with (or register) your Blackmagic account.
4. Download `DaVinci_Resolve_Studio_<version>_Linux.zip` to `~/Downloads/`.

Keep the exact filename — the AUR build step in section 3 needs to match the
version number inside it against the AUR package's expected version.

## 3. Build + install from the AUR (primary: manual clone)

The AUR package `davinci-resolve-studio` (currently `20.3.3-1`) requires the
licensed ZIP from step 2 — it cannot download it automatically because it's
account-gated. Clone the AUR package and inspect the version it expects:

```bash
git clone https://aur.archlinux.org/davinci-resolve-studio.git
cd davinci-resolve-studio
grep -E '^pkgver=' PKGBUILD          # note the exact version it expects
```

If the Blackmagic ZIP version matches `pkgver` exactly, copy it in and build:

```bash
cp ~/Downloads/DaVinci_Resolve_Studio_<ver>_Linux.zip .
makepkg -si
```

If the versions differ, update the PKGBUILD to match the ZIP you downloaded, then
refresh the checksums before building:

```bash
$EDITOR PKGBUILD   # update pkgver=<ver> to match the downloaded ZIP
updpkgsums
makepkg -si
```

The installer binary is large, so packaging (not just building) can take a while.
To skip compression and speed this up:

```bash
PKGEXT='.pkg.tar' makepkg -si
```

**Fallback route** if you'd rather let `paru` drive the AUR build: let the
account-gated download fail first, then drop the ZIP into paru's clone directory and
rerun.

```bash
paru -S davinci-resolve-studio    # this will fail at the download step
cp ~/Downloads/DaVinci_Resolve_Studio_<ver>_Linux.zip ~/.cache/paru/clone/davinci-resolve-studio/
paru -S davinci-resolve-studio    # rerun; it should find the ZIP and continue
```

Note that `davinci-resolve-studio` conflicts with `davinci-resolve`,
`davinci-resolve-beta`, and `davinci-resolve-studio-beta` — only one of these four
can be installed at a time.

## 4. Fix license activation (the #1 Studio gotcha)

The Studio license directory is often owned by `root` after install, which breaks
activation. Before your first launch (or if activation fails after launch), make it
writable by your user:

```bash
sudo mkdir -p /opt/resolve/.license
sudo chown -R "$USER" /opt/resolve/.license
```

Some threads suggest a looser `chmod -R 7777 /opt/resolve/.license` instead — the
`chown` approach above is preferred since it keeps ownership scoped to your user
rather than opening the directory to everyone.

## 5. Create a launch wrapper with the correct env (NVIDIA offload + X11)

Because the desktop is driven by the AMD eGPU and the session is Wayland, launching
`/opt/resolve/bin/resolve` directly will either fail (Qt Wayland plugin error) or
silently try to use the wrong GPU (error 999 / "GPU memory full"). Create a wrapper
at `~/.local/bin/resolve` that forces NVIDIA PRIME render offload and the X11 (`xcb`)
Qt platform:

```bash
mkdir -p ~/.local/bin
cat > ~/.local/bin/resolve <<'EOF'
#!/usr/bin/env bash
export __NV_PRIME_RENDER_OFFLOAD=1
export __GLX_VENDOR_LIBRARY_NAME=nvidia
export __VK_LAYER_NV_optimus=NVIDIA_only
export QT_QPA_PLATFORM=xcb
export QT_AUTO_SCREEN_SCALE_FACTOR=1
exec /opt/resolve/bin/resolve "$@"
EOF
chmod +x ~/.local/bin/resolve
```

Add a matching `.desktop` launcher so the application menu entry uses the same
wrapper instead of Resolve's default entry (which points straight at
`/opt/resolve/bin/resolve` with none of the required env vars):

```bash
mkdir -p ~/.local/share/applications
cat > ~/.local/share/applications/davinci-resolve-studio.desktop <<EOF
[Desktop Entry]
Name=DaVinci Resolve Studio
Comment=Color grading and video editing (NVIDIA offload wrapper)
Exec=$HOME/.local/bin/resolve %U
Icon=/opt/resolve/graphics/DV_Resolve.png
Terminal=false
Type=Application
Categories=AudioVideo;Video;
StartupWMClass=resolve
EOF
```

Inside Resolve, after first launch, confirm the GPU selection explicitly rather than
relying on defaults:

- Go to **Preferences → Memory and GPU**.
- Set **GPU processing mode** to **CUDA**.
- Manually select the NVIDIA GPU only (deselect Intel/AMD if listed).

## 6. First launch + activation

Launch through the wrapper — not the raw `/opt/resolve/bin/resolve` binary — so the
NVIDIA offload and `xcb` env vars are applied:

```bash
~/.local/bin/resolve
```

(Or launch "DaVinci Resolve Studio" from the application menu, which now points at
the same wrapper via the `.desktop` file from section 5.)

Complete the first-run onboarding, then enter your Studio activation key when
prompted. Activation performs a one-time online check against your Blackmagic
account — no key is stored anywhere in this repository or in this document.

## 7. Troubleshooting reference

Curated from the [ArchWiki DaVinci Resolve page](https://wiki.archlinux.org/title/DaVinci_Resolve)
and Blackmagic forum reports, focused on the failures most likely to show up on a
rolling-release, multi-GPU box like this one:

| Symptom / Error | Fix |
| --- | --- |
| `libcrypt.so.1: cannot open shared object file` | Install `libxcrypt-compat` (already installed on this machine — verify with `pacman -Q libxcrypt-compat`). |
| Bundled-library symbol errors on startup (`g_string_free_and_steal`, `g_task_set_static_name`, `bad_function_call`) | Remove or rename the conflicting bundled libs in `/opt/resolve/libs/` (`libglib-2.0`, `libgio-2.0`, `libgmodule-2.0`, `libc++.so.1`) so Resolve falls back to the system versions, e.g. `sudo mv /opt/resolve/libs/libglib-2.0.so.0{,.bak}`; alternatively `LD_PRELOAD` the system libs at launch. |
| Qt "wayland" plugin error / fails to find a Qt platform plugin | Confirm you launched via `~/.local/bin/resolve` (sets `QT_QPA_PLATFORM=xcb`), not the raw `/opt/resolve/bin/resolve` binary. |
| Error 999 / "GPU memory full" on hybrid/multi-GPU systems | The wrapper's `__NV_PRIME_RENDER_OFFLOAD`, `__GLX_VENDOR_LIBRARY_NAME`, and `__VK_LAYER_NV_optimus` vars force PRIME offload onto the NVIDIA GPU. Confirm the wrapper is being used and that these are actually exported (`grep -A5 'export' ~/.local/bin/resolve`). |
| `OpenCL -1001` / no OpenCL platforms found | Driver/OpenCL mismatch. Verify with `clinfo -l` that the NVIDIA platform is listed and healthy. |
| App exits silently, or shows "already running" with no window | Stale lock file from a previous crashed session. Remove it and relaunch: `rm -f /tmp/qtsingleapp-DaVinc-*-lockfile`. |
| Extras / Download Manager fails inside Resolve | Missing legacy TLS compatibility path. Run `sudo mkdir -p /etc/pki && sudo ln -s /etc/ssl /etc/pki/tls`. |
| No preview/playback audio | Install `pipewire-alsa` (already installed on this machine — verify with `pacman -Q pipewire-alsa`). |
| Need more detail on any failure above | Read the app's own debug log: `~/.local/share/DaVinciResolve/logs/ResolveDebug.txt`. |

## 8. Uninstall (so you can hand off to the scripted version cleanly)

If you want to avoid re-doing online activation later, back up the license
directory before removing `/opt/resolve`:

```bash
sudo cp -a /opt/resolve/.license ~/resolve-license-backup
```

Then remove the package and all files created by this guide:

```bash
paru -Rns davinci-resolve-studio
sudo rm -rf /opt/resolve
rm -rf ~/.local/share/DaVinciResolve ~/.local/bin/resolve ~/.local/share/applications/davinci-resolve-studio.desktop
```

These paths match exactly what section 5 created (`~/.local/bin/resolve`,
`~/.local/share/applications/davinci-resolve-studio.desktop`) and what the AUR
package installs to (`/opt/resolve`, `~/.local/share/DaVinciResolve`).

## 9. Notes for the future `bin/install-davinci-resolve.sh`

Once this guide has been verified by hand (install, activate, confirm CUDA GPU
selection, uninstall cleanly), the scripted version should encode the same steps
1:1:

- Install prerequisites idempotently (`clinfo`, and re-check the already-satisfied
  packages from section 1 with `pacman -Q` rather than assuming them).
- Accept a path to the downloaded Studio ZIP as a script argument, or auto-detect it
  in `~/Downloads/`.
- Drive the AUR build non-interactively: clone `davinci-resolve-studio`, reconcile
  `pkgver` against the ZIP filename, run `updpkgsums` if needed, then
  `makepkg -si` (or the `paru` fallback path from section 3).
- Apply the `/opt/resolve/.license` `chown` fix from section 4 unconditionally after
  install.
- Generate the `~/.local/bin/resolve` wrapper and
  `~/.local/share/applications/davinci-resolve-studio.desktop` launcher from
  section 5 idempotently (safe to re-run).
- Guard all NVIDIA/PRIME env vars so the script degrades gracefully on a machine
  without this exact GPU layout, instead of assuming NVIDIA + AMD eGPU + Wayland.

## Verification

The install is considered successful when:

- `clinfo -l` lists the NVIDIA CUDA platform; OpenGL reports `NVIDIA Corporation` /
  `GeForce RTX 5070` (the checker may still exit on dual-vendor systems — see
  section 1).
- `~/.local/bin/resolve` launches into the Resolve UI (not a Qt/Wayland error).
- Studio activation succeeds against your Blackmagic account.
- **Preferences → Memory and GPU** shows the NVIDIA GPU selected under CUDA
  processing mode, not the Intel iGPU or AMD eGPU.

## Assumptions

- The primary install route is the manual `git clone` + `makepkg -si` AUR build,
  since it is the most deterministic path to run by hand and to later mirror 1:1 in
  a script. The `paru -S` route in section 3 is documented as a fallback only.
- This guide targets the NVIDIA RTX 5070 Mobile for all of Resolve's GPU compute
  (CUDA/OpenCL). The AMD RX 9070 eGPU is left as the display/compositor GPU only and
  is intentionally not configured for Resolve processing.
- You already hold a valid DaVinci Resolve Studio activation key and Blackmagic
  account access to download the Linux Studio ZIP; no key or credential is recorded
  anywhere in this document.
