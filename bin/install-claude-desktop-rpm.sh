#!/usr/bin/env bash

# Install, update, or remove Claude Desktop on Fedora by repacking Anthropic's
# official Linux .deb as a local RPM. Anthropic publishes Linux Desktop (beta)
# only as a signed apt repository; there is no official RPM, so this path is
# UNSUPPORTED by Anthropic. Debian/Ubuntu use the native apt repo and macOS the
# Homebrew cask instead (manifest/packages.toml).
#
# Trust chain, fail-closed at every step:
#   key.asc fingerprint == ANTHROPIC_FPR
#   gpgv VALIDSIG ANTHROPIC_FPR over InRelease (only the signed text is used)
#   InRelease SHA256 -> Packages (binary-amd64|arm64) -> chosen .deb size + SHA256
#
# The .deb is unpacked with ar+tar (no-same-owner); its Debian maintainer scripts
# (apt repo, AppArmor) are never run. The RPM keeps the vendor 4755 mode on
# chrome-sandbox only — no other setuid, no --no-sandbox. Built as the invoking
# user; only the final `dnf install` uses sudo.

set -euo pipefail
umask 022

readonly REPO_URL="https://downloads.claude.ai/claude-desktop/apt/stable"
readonly KEY_URL="https://downloads.claude.ai/claude-desktop/key.asc"
readonly ANTHROPIC_FPR="31DDDE24DDFAB679F42D7BD2BAA929FF1A7ECACE"
readonly LOG_PREFIX="[CLAUDE-DESKTOP-RPM]"

ACTION="install"
DRY_RUN=false
BUILD_ONLY=false
OUT_DIR="${CLAUDE_DESKTOP_RPM_OUT:-}"

log() { printf '%s %s\n' "$LOG_PREFIX" "$*" >&2; }
die() {
  log "ERROR: $*"
  exit 1
}

usage() {
  cat <<EOF
Usage: $(basename "$0") [--install|--update|--remove|--uninstall] [--build-only] [--dry-run]

  --install      Build and install the latest signed release (default)
  --update       Rebuild and install when the repo has a newer version
  --remove       sudo dnf remove claude-desktop (alias: --uninstall)
  --build-only   Verify, build the RPM, print it; install nothing
  --dry-run      Print the plan without network access or changes

Environment:
  CLAUDE_DESKTOP_VERSION   Pin a repo version (default: newest)
  CLAUDE_DESKTOP_RPM_OUT   Keep the built RPM in this directory
  NON_INTERACTIVE=true     Install missing build tools without prompting
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --install) ACTION="install" ;;
    --update) ACTION="update" ;;
    --remove | --uninstall) ACTION="remove" ;;
    --build-only) BUILD_ONLY=true ;;
    --dry-run) DRY_RUN=true ;;
    -h | --help)
      usage
      exit 0
      ;;
    *) die "unknown argument: $1" ;;
  esac
  shift
done

[[ $EUID -ne 0 ]] || die "run as your regular user; only dnf uses sudo"

installed_version() {
  local v
  v="$(rpm -q --qf '%{VERSION}' claude-desktop 2>/dev/null)" && printf '%s' "$v"
  return 0
}

if [[ "$ACTION" == "remove" ]]; then
  if [[ -z "$(installed_version)" ]]; then
    log "claude-desktop is not installed"
    exit 0
  fi
  "$DRY_RUN" && {
    log "would run: sudo dnf remove -y claude-desktop"
    exit 0
  }
  exec sudo dnf remove -y claude-desktop
fi

# Fedora and its remixes (ID or ID_LIKE fedora, e.g. fedora-asahi-remix, nobara)
# only: the dependency map below uses Fedora package names and glibc >= 2.34.
# Enterprise Linux rebuilds also list fedora in ID_LIKE but are rejected by ID.
# The repo ships amd64 and arm64. Unsupported hosts fail (non-zero) so dotf
# never reports a skipped install as ok; the manifest os/arch gates keep dotf
# from running the script on non-redhat hosts and other arches at all.
os_release=/etc/os-release
[[ -r "$os_release" ]] || os_release=/usr/lib/os-release
[[ -r "$os_release" ]] || die "cannot read /etc/os-release or /usr/lib/os-release"
os_value() { tr -d '\r' <"$os_release" | sed -n "s/^$1=[\"']\{0,1\}\([^\"']*\)[\"']\{0,1\}\$/\1/p" | head -1; }
os_id="$(os_value ID)"
os_ids=" $os_id $(os_value ID_LIKE) "
case "$os_id" in
  rhel | centos | rocky | almalinux | ol | amzn | circle | eurolinux | navy | cloudlinux)
    die "$os_id is Enterprise Linux, not Fedora; Claude Desktop's Fedora repack is unsupported here"
    ;;
esac
[[ "$os_ids" == *" fedora "* ]] ||
  die "not Fedora; Claude Desktop uses the apt repo on Debian/Ubuntu and is unavailable here"
case "$(uname -m)" in
  x86_64) DEB_ARCH="amd64" RPM_ARCH="x86_64" ;;
  aarch64) DEB_ARCH="arm64" RPM_ARCH="aarch64" ;;
  *) die "unsupported architecture $(uname -m) (repo ships amd64/arm64 only)" ;;
esac

if [[ "$ACTION" == "install" && -n "$(installed_version)" && "$BUILD_ONLY" == false ]]; then
  log "claude-desktop $(installed_version) already installed (use --update)"
  exit 0
fi

if "$DRY_RUN"; then
  log "would verify $KEY_URL ($ANTHROPIC_FPR) -> InRelease -> Packages -> .deb, build RPM, sudo dnf install"
  exit 0
fi

need=()
for t in curl gpg gpgv ar tar xz rpmbuild sha256sum; do
  command -v "$t" >/dev/null 2>&1 || need+=("$t")
done
if ((${#need[@]})); then
  [[ "${NON_INTERACTIVE:-false}" == "true" ]] || die "missing tools: ${need[*]} (install rpm-build gnupg2 binutils xz, or set NON_INTERACTIVE=true)"
  log "installing build tools for: ${need[*]}"
  sudo dnf install -y rpm-build gnupg2 binutils xz curl coreutils
fi

cleanup() {
  # Kill only agents spawned for our private GNUPGHOME; ~/.gnupg is never touched.
  if [[ -n "${GNUPGHOME:-}" && -d "$GNUPGHOME" ]] && command -v gpgconf >/dev/null 2>&1; then
    GNUPGHOME="$GNUPGHOME" gpgconf --kill gpg-agent >/dev/null 2>&1 || :
  fi
  rm -rf "$WORK"
}
WORK="$(mktemp -d)"
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM
chmod 700 "$WORK"
# All scratch (gpg sockets, rpm-tmp.*) stays under WORK; nothing leaks to /tmp or ~/.gnupg.
mkdir -p "$WORK/tmp" "$WORK/gnupg"
chmod 700 "$WORK/tmp" "$WORK/gnupg"
export TMPDIR="$WORK/tmp" GNUPGHOME="$WORK/gnupg"

# 1. Key fingerprint.
curl -fsSL "$KEY_URL" -o "$WORK/key.asc"
fpr="$(gpg --no-autostart --show-keys --with-colons "$WORK/key.asc" | awk -F: '$1=="fpr"{print $10; exit}')"
[[ "${fpr^^}" == "$ANTHROPIC_FPR" ]] || die "signing key fingerprint mismatch: got '$fpr'"
gpg --no-autostart --dearmor <"$WORK/key.asc" >"$WORK/key.gpg"

# 2. Signed InRelease; keep only the verified text.
curl -fsSL "$REPO_URL/dists/stable/InRelease" -o "$WORK/InRelease"
gpgv --status-fd 3 --keyring "$WORK/key.gpg" --output "$WORK/Release" "$WORK/InRelease" 3>"$WORK/status" 2>/dev/null ||
  die "InRelease signature verification failed"
grep -q "^\[GNUPG:\] VALIDSIG $ANTHROPIC_FPR " "$WORK/status" || die "InRelease not signed by $ANTHROPIC_FPR"
until="$(sed -n 's/^Valid-Until: //p' "$WORK/Release" | head -1)"
[[ -n "$until" ]] || die "InRelease has no Valid-Until"
[[ "$(date -u -d "$until" +%s)" -gt "$(date -u +%s)" ]] || die "InRelease expired ($until); refusing stale metadata"

# 3. Packages index hash from the signed SHA256 section.
pkgs_sha="$(awk -v p="main/binary-$DEB_ARCH/Packages" '/^SHA256:/{s=1;next} /^[^ ]/{s=0} s && $3==p{print $1; exit}' "$WORK/Release")"
[[ -n "$pkgs_sha" ]] || die "Packages hash missing from InRelease"
curl -fsSL "$REPO_URL/dists/stable/main/binary-$DEB_ARCH/Packages" -o "$WORK/Packages"
echo "$pkgs_sha  $WORK/Packages" | sha256sum -c --status || die "Packages hash mismatch"

# 4. Pick the version (pinned or newest) and its .deb metadata.
want="${CLAUDE_DESKTOP_VERSION:-$(awk '/^Package: claude-desktop$/{p=1} p&&/^Version:/{print $2; p=0}' "$WORK/Packages" | sort -V | tail -1)}"
read -r filename deb_sha size < <(awk -v v="$want" '
  /^Package: /{p=($2=="claude-desktop")} /^Version: /{ok=p&&($2==v)}
  ok&&/^Filename: /{f=$2} ok&&/^SHA256: /{s=$2} ok&&/^Size: /{z=$2}
  /^$/{if(f&&s&&z){print f, s, z; exit} ok=0}
  END{if(f&&s&&z)print f, s, z}' "$WORK/Packages" | head -1)
[[ -n "${filename:-}" ]] || die "version $want not found in the signed index"
[[ "$filename" =~ ^pool/main/c/claude-desktop/[A-Za-z0-9._+~-]+_${DEB_ARCH}\.deb$ ]] || die "unexpected Filename: $filename"

current="$(installed_version)"
if [[ "$ACTION" == "update" && -n "$current" && "$BUILD_ONLY" == false ]]; then
  [[ "$(printf '%s\n%s\n' "$current" "$want" | sort -V | tail -1)" == "$current" ]] && {
    log "claude-desktop $current is current"
    exit 0
  }
fi

log "key $ANTHROPIC_FPR ok; InRelease valid until $until"
log "claude-desktop $want: $filename sha256=$deb_sha"

# 5. Download and verify the .deb.
curl -fsSL "$REPO_URL/$filename" -o "$WORK/claude-desktop.deb"
[[ "$(stat -c %s "$WORK/claude-desktop.deb")" == "$size" ]] || die ".deb size mismatch"
echo "$deb_sha  $WORK/claude-desktop.deb" | sha256sum -c --status || die ".deb SHA256 mismatch"

# 6. Safe unpack: reject absolute/.. paths and any symlink except the launcher.
(cd "$WORK" && ar x claude-desktop.deb data.tar.xz)
bad="$(tar -tvJf "$WORK/data.tar.xz" | awk '
  { name=$6; for(i=7;i<=NF;i++) name=name" "$i }
  name ~ /^\// || name ~ /(^|\/)\.\.(\/|$)/ { print; next }
  substr($1,1,1)=="l" && name != "./usr/bin/claude-desktop -> ../lib/claude-desktop/claude-desktop" { print }
  substr($1,1,1) ~ /^[hbcp]$/ { print }')"
if [[ -n "$bad" ]]; then
  log "unsafe archive members:"
  printf '%s\n' "$bad" >&2
  die "refusing to unpack"
fi
ROOT="$WORK/root"
mkdir -p "$ROOT"
tar -xJf "$WORK/data.tar.xz" -p --no-same-owner -C "$ROOT"
rm -rf "$ROOT/usr/share/lintian"

# 7. Build the RPM (unprivileged).
TOP="$WORK/rpmbuild"
mkdir -p "$TOP/SPECS"
rpm_version="${want//-/_}"
cat >"$TOP/SPECS/claude-desktop.spec" <<EOF
Name:           claude-desktop
Version:        $rpm_version
Release:        1.dotf%{?dist}
Summary:        Claude Desktop (repacked from Anthropic's official .deb)
License:        Proprietary
URL:            https://claude.ai/download
ExclusiveArch:  $RPM_ARCH
AutoReqProv:    no
Requires:       gtk3 libnotify nss xdg-utils at-spi2-core libdrm mesa-libgbm libxcb libsecret glib2 libXtst libuuid pipewire-libs xdg-desktop-portal alsa-lib glibc >= 2.34
Recommends:     libappindicator-gtk3
%global debug_package %{nil}
%global __strip /bin/true
%global _build_id_links none

%description
Claude Desktop $want, repacked locally by dotf from the signature-verified
official Debian package. Not supported by Anthropic.

%install
cp -a "$ROOT/." %{buildroot}/

%post
update-desktop-database -q /usr/share/applications >/dev/null 2>&1 || :
gtk-update-icon-cache -q /usr/share/icons/hicolor >/dev/null 2>&1 || :

%postun
update-desktop-database -q /usr/share/applications >/dev/null 2>&1 || :
gtk-update-icon-cache -q /usr/share/icons/hicolor >/dev/null 2>&1 || :

%files
%defattr(-,root,root,-)
# tar -p kept the vendor modes (chrome-sandbox 4755); asserted after the build.
/usr/lib/claude-desktop
/usr/bin/claude-desktop
/usr/share/applications/com.anthropic.Claude.desktop
/usr/share/icons/hicolor/*/apps/*
/usr/share/doc/claude-desktop
EOF
mkdir -p "$TOP/tmp"
rpmbuild --quiet --define "_topdir $TOP" --define "_tmppath $TOP/tmp" -bb "$TOP/SPECS/claude-desktop.spec" >&2
rpm_file="$(find "$TOP/RPMS" -name 'claude-desktop-*.rpm' | head -1)"
[[ -n "$rpm_file" ]] || die "rpmbuild produced no package"
modes="$(rpm -qp --qf '[%{FILEMODES:perms} %{FILENAMES}\n]' "$rpm_file")"
grep -q '^-rwsr-xr-x .*/chrome-sandbox$' <<<"$modes" ||
  die "chrome-sandbox missing its vendor 4755 mode"

if [[ -n "$OUT_DIR" ]]; then
  mkdir -p "$OUT_DIR"
  cp "$rpm_file" "$OUT_DIR/"
  rpm_file="$OUT_DIR/$(basename "$rpm_file")"
fi
log "built $rpm_file"
if "$BUILD_ONLY"; then
  echo "$rpm_file"
  exit 0
fi

# 8. Install: the RPM is unsigned (built locally from verified inputs).
sudo dnf install -y --setopt=localpkg_gpgcheck=0 "$rpm_file"
log "installed claude-desktop $want; launch from the app menu or run claude-desktop"
