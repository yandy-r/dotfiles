#!/bin/sh
# ==============================================================================
# dotfiles bootstrap shim (Phase 7, Option A)
# ------------------------------------------------------------------------------
# Downloads the prebuilt `dotf` binary from a forge release (GitHub, Forgejo,
# Gitea, etc.), verifies its sha256 checksum, and execs it — forwarding all
# arguments. With no subcommand (or a leading flag) it injects `apply`, the
# headless-capable orchestrator.
#
# Release discovery mirrors internal/selfupdate: try /releases/latest first
# (stable), then fall back to the newest published release including
# prereleases. Private GitHub repos need GH_TOKEN/GITHUB_TOKEN or `gh auth`.
#
# When no prebuilt asset is available, the shim can build from this clone.
# On a TTY it prompts to install official Go 1.26+ under ~/.local/go (no sudo)
# and build; use --install-go to skip the prompt (required for non-interactive).
#
# Options (consumed by this shim; not forwarded to dotf):
#   --repo owner/repo
#   --repo=https://forge.example.com/owner/repo
#   --repo=git@forge.example.com:owner/repo.git
#   --install-go   # consent to install Go + build from source without a TTY prompt
# Default: yandy-r/dotfiles on github.com
# ==============================================================================
set -eu

REPO_SLUG="yandy-r/dotfiles"
REPO_ORIGIN="https://github.com/yandy-r/dotfiles"
REPO_HOST="github.com"
BINARY="dotf"
GITHUB_AUTH_TOKEN=""
INSTALL_GO=0
# Keep ≥ go.mod's `go` directive (official go.dev tarball version).
GO_BOOTSTRAP_VERSION="1.26.5"

SCRIPT_DIR="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)" || SCRIPT_DIR=""

die() {
  printf '%s\n' "dotf bootstrap: $*" >&2
  exit 1
}

# Validate and store owner/repo in REPO_SLUG.
validate_repo_slug() {
  slug="${1%/}"
  case "$slug" in
    */*)
      owner="${slug%%/*}"
      name="${slug#*/}"
      case "$name" in
        */*) die "invalid --repo '$2': expected 'owner/repo', not a deeper path" ;;
      esac
      if [ -z "$owner" ] || [ -z "$name" ]; then
        die "invalid --repo '$2': owner and repo name are required"
      fi
      REPO_SLUG="${owner}/${name}"
      ;;
    *)
      die "invalid --repo '$2': expected 'owner/repo' or a forge URL containing owner/repo"
      ;;
  esac
}

# Parse --repo into REPO_SLUG and REPO_ORIGIN (forge root, no trailing slash).
# Supports GitHub-style owner/repo, HTTPS forge URLs, host/owner/repo, and git@host:owner/repo.
set_repo() {
  input="${1%/}"
  scheme="https"

  case "$input" in
    git@*:*)
      host_path="${input#git@}"
      host="${host_path%%:*}"
      slug="${host_path#*:}"
      slug="${slug%.git}"
      validate_repo_slug "$slug" "$input"
      REPO_ORIGIN="${scheme}://${host}/${REPO_SLUG}"
      REPO_HOST="$host"
      ;;
    http://*|https://*)
      scheme="${input%%://*}"
      rest="${input#*://}"
      host="${rest%%/*}"
      slug="${rest#*/}"
      slug="${slug%/}"
      validate_repo_slug "$slug" "$input"
      REPO_ORIGIN="${scheme}://${host}/${REPO_SLUG}"
      REPO_HOST="$host"
      ;;
    */*)
      case "$input" in
        */*/*)
          host="${input%%/*}"
          slug="${input#*/}"
          slug="${slug%/}"
          validate_repo_slug "$slug" "$input"
          REPO_ORIGIN="${scheme}://${host}/${REPO_SLUG}"
          REPO_HOST="$host"
          ;;
        *)
          validate_repo_slug "$input" "$input"
          REPO_ORIGIN="${scheme}://github.com/${REPO_SLUG}"
          REPO_HOST="github.com"
          ;;
      esac
      ;;
    *)
      die "invalid --repo '$input': expected 'owner/repo' or a forge URL containing owner/repo"
      ;;
  esac
}

# --- Parse bootstrap-only flags ------------------------------------------------
while [ "$#" -gt 0 ]; do
  case "$1" in
    --repo)
      [ "$#" -ge 2 ] || die "--repo requires a value"
      set_repo "$2"
      shift 2
      ;;
    --repo=*)
      set_repo "${1#--repo=}"
      shift
      ;;
    --install-go)
      INSTALL_GO=1
      shift
      ;;
    *)
      break
      ;;
  esac
done

guard() {
  # $1 = human-readable reason
  cat >&2 <<EOF
dotf bootstrap: $1

No prebuilt binary could be used. Do ONE of the following:

  1. Tag a release so prebuilt binaries exist (maintainer, on the release remote):
       git tag v0.1.0 && git push <remote> v0.1.0
     then re-run this script.

  2. Build locally from a clone of ${REPO_ORIGIN}.git (requires Go 1.26+):
       go build -o ${BINARY} ./cmd/dotf
       ./${BINARY} apply --headless

  3. From a clone, re-run with --install-go to install official Go under
     ~/.local/go and build automatically (network required):
       ./bootstrap.sh --install-go
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

# GitHub release assets require HTTPS auth for private repos; git@ SSH access does
# not apply to curl. Accept GH_TOKEN/GITHUB_TOKEN or reuse an authenticated gh CLI.
resolve_github_auth_token() {
  if [ -n "${GH_TOKEN:-}" ]; then
    GITHUB_AUTH_TOKEN="$GH_TOKEN"
    return 0
  fi
  if [ -n "${GITHUB_TOKEN:-}" ]; then
    GITHUB_AUTH_TOKEN="$GITHUB_TOKEN"
    return 0
  fi
  if command -v gh >/dev/null 2>&1; then
    token="$(gh auth token 2>/dev/null || true)"
    if [ -n "$token" ]; then
      GITHUB_AUTH_TOKEN="$token"
      return 0
    fi
  fi
  GITHUB_AUTH_TOKEN=""
  return 1
}

is_github_host() {
  [ "$REPO_HOST" = "github.com" ]
}

# Non-interactive: env, no TTY on stdin, or remaining argv requests headless/force.
is_noninteractive() {
  if [ "${NON_INTERACTIVE:-}" = "true" ]; then
    return 0
  fi
  if [ ! -t 0 ]; then
    return 0
  fi
  for arg in "$@"; do
    case "$arg" in
      --headless|--force) return 0 ;;
    esac
  done
  return 1
}

# True when `go` on PATH reports a version >= 1.26 (go.mod floor).
go_version_ok() {
  command -v go >/dev/null 2>&1 || return 1
  ver="$(go env GOVERSION 2>/dev/null || true)"
  if [ -z "$ver" ]; then
    ver="$(go version 2>/dev/null | awk '{print $3}' || true)"
  fi
  ver="${ver#go}"
  major="${ver%%.*}"
  rest="${ver#*.}"
  minor="${rest%%.*}"
  case "$major" in
    ''|*[!0-9]*) return 1 ;;
  esac
  case "$minor" in
    ''|*[!0-9]*) return 1 ;;
  esac
  if [ "$major" -gt 1 ]; then
    return 0
  fi
  if [ "$major" -eq 1 ] && [ "$minor" -ge 26 ]; then
    return 0
  fi
  return 1
}

# Must match .goreleaser.yml archives.name_template: "dotf_{{ .Os }}_{{ .Arch }}"
# and internal/selfupdate.AssetName (Go `dotf self-update`); the three are pinned
# together by internal/selfupdate.TestAssetNaming_MatchesBootstrap.
ASSET="${BINARY}_${OS}_${ARCH}.tar.gz"
BASE="${REPO_ORIGIN}/releases/latest/download"
ALT_BASE="${REPO_ORIGIN}/releases/download/latest"

TMP="$(mktemp -d "${TMPDIR:-/tmp}/dotf-boot.XXXXXX")" || die "failed to create temp dir"

if is_github_host; then
  resolve_github_auth_token || true
fi

auth_curl() {
  if [ -n "$GITHUB_AUTH_TOKEN" ]; then
    curl -fsSL \
      -H "Authorization: Bearer ${GITHUB_AUTH_TOKEN}" \
      -H "Accept: application/octet-stream" \
      "$@"
  else
    curl -fsSL "$@"
  fi
}

# Same as auth_curl but suppresses expected probe failures (404 fallthrough).
try_auth_curl() {
  if [ -n "$GITHUB_AUTH_TOKEN" ]; then
    curl -fsSL \
      -H "Authorization: Bearer ${GITHUB_AUTH_TOKEN}" \
      -H "Accept: application/octet-stream" \
      "$@" 2>/dev/null
  else
    curl -fsSL "$@" 2>/dev/null
  fi
}

# JSON Accept for GitHub REST (tag/asset metadata).
github_api_curl() {
  if [ -n "$GITHUB_AUTH_TOKEN" ]; then
    curl -fsSL \
      -H "Authorization: Bearer ${GITHUB_AUTH_TOKEN}" \
      -H "Accept: application/vnd.github+json" \
      "$@"
  else
    curl -fsSL -H "Accept: application/vnd.github+json" "$@"
  fi
}

try_github_api_curl() {
  if [ -n "$GITHUB_AUTH_TOKEN" ]; then
    curl -fsSL \
      -H "Authorization: Bearer ${GITHUB_AUTH_TOKEN}" \
      -H "Accept: application/vnd.github+json" \
      "$@" 2>/dev/null
  else
    curl -fsSL -H "Accept: application/vnd.github+json" "$@" 2>/dev/null
  fi
}

# Extract tag_name from a single release JSON object (stdin).
python_release_tag() {
  python3 -c 'import json,sys; print(json.load(sys.stdin).get("tag_name") or "")'
}

# Extract tag_name from releases[0] (stdin = JSON array).
python_first_release_tag() {
  python3 -c 'import json,sys; r=json.load(sys.stdin); print(r[0]["tag_name"] if r else "")'
}

# Extract asset id for name=$1 from a single-release JSON object (stdin).
python_asset_id() {
  name="$1"
  python3 -c '
import json, sys
name = sys.argv[1]
rel = json.load(sys.stdin)
for a in rel.get("assets") or []:
    if a.get("name") == name:
        print(a["id"])
        break
' "$name"
}

# True when $1 looks like a release tag (not a JSON error body or null).
is_release_tag() {
  case "$1" in
    ''|null|\{*|\[*) return 1 ;;
  esac
  return 0
}

# Resolve newest published tag: /releases/latest, else /releases[0] (prerelease-inclusive).
# Mirrors internal/selfupdate.defaultLatestRelease / firstReleaseTag.
# Note: `gh api` on HTTP 404 still prints a JSON error to stdout — require a zero
# exit from gh (or python-parsed tag_name) before accepting a tag.
github_resolve_latest_tag() {
  tag=""

  if command -v gh >/dev/null 2>&1; then
    if tag="$(gh api "repos/${REPO_SLUG}/releases/latest" --jq '.tag_name' 2>/dev/null)" \
      && is_release_tag "$tag"; then
      printf '%s\n' "$tag"
      return 0
    fi
    if tag="$(gh api "repos/${REPO_SLUG}/releases" --jq '.[0].tag_name' 2>/dev/null)" \
      && is_release_tag "$tag"; then
      printf '%s\n' "$tag"
      return 0
    fi
  fi

  if ! command -v python3 >/dev/null 2>&1; then
    return 1
  fi

  body="$(try_github_api_curl "https://api.github.com/repos/${REPO_SLUG}/releases/latest" || true)"
  if [ -n "$body" ]; then
    tag="$(printf '%s' "$body" | python_release_tag 2>/dev/null || true)"
    if is_release_tag "$tag"; then
      printf '%s\n' "$tag"
      return 0
    fi
  fi

  body="$(try_github_api_curl "https://api.github.com/repos/${REPO_SLUG}/releases" || true)"
  if [ -n "$body" ]; then
    tag="$(printf '%s' "$body" | python_first_release_tag 2>/dev/null || true)"
    if is_release_tag "$tag"; then
      printf '%s\n' "$tag"
      return 0
    fi
  fi

  return 1
}

# Download via GitHub API when /releases/latest/download/ misses (private repo
# and/or prerelease-only). Resolve tag first, then try in order: download by tag
# (gh), the tag-pinned public asset URL (needs neither gh nor a token), and the
# asset id from GET …/releases/tags/$tag (token + python3). No sed over the
# full /releases list.
github_download_release_file() {
  name="$1"
  dest="$2"
  dir="${dest%/*}"

  tag="$(github_resolve_latest_tag || true)"
  if [ -z "$tag" ]; then
    if ! command -v gh >/dev/null 2>&1 && ! command -v python3 >/dev/null 2>&1; then
      printf '%s\n' "dotf bootstrap: cannot parse GitHub release JSON (need gh or python3); try --install-go from a clone" >&2
    fi
    return 1
  fi

  if command -v gh >/dev/null 2>&1; then
    if gh release download "$tag" --repo "$REPO_SLUG" --pattern "$name" -D "$dir" >/dev/null 2>&1; then
      [ -f "$dest" ] && return 0
    fi
  fi

  # Tag-pinned public asset URL. This is the ONLY path that works with neither
  # `gh` nor a token: BASE (/releases/latest/download) 404s whenever the newest
  # release is a prerelease (.goreleaser.yml sets `prerelease: auto`), and
  # ALT_BASE points at a literal tag named "latest" that GoReleaser never
  # creates. try_auth_curl (NOT auth_curl) so the expected 404 on a private
  # repo stays silent and falls through to the token+asset-id path below.
  if try_auth_curl "${REPO_ORIGIN}/releases/download/${tag}/${name}" -o "$dest"; then
    return 0
  fi

  if [ -z "$GITHUB_AUTH_TOKEN" ]; then
    return 1
  fi

  if ! command -v python3 >/dev/null 2>&1; then
    printf '%s\n' "dotf bootstrap: token download needs python3 to parse asset id (or use gh); try --install-go from a clone" >&2
    return 1
  fi

  body="$(github_api_curl "https://api.github.com/repos/${REPO_SLUG}/releases/tags/${tag}" || true)"
  if [ -z "$body" ]; then
    return 1
  fi

  asset_id="$(printf '%s' "$body" | python_asset_id "$name" || true)"
  if [ -z "$asset_id" ]; then
    return 1
  fi

  auth_curl "https://api.github.com/repos/${REPO_SLUG}/releases/assets/${asset_id}" -o "$dest"
}

# Download a release asset. GitHub and modern Forgejo use /releases/latest/download/;
# older Gitea/Forgejo instances use /releases/download/latest/ instead.
download_release_file() {
  name="$1"
  dest="$2"

  if try_auth_curl "${BASE}/${name}" -o "${dest}"; then
    return 0
  fi
  if try_auth_curl "${ALT_BASE}/${name}" -o "${dest}"; then
    return 0
  fi
  if is_github_host; then
    github_download_release_file "$name" "$dest"
  else
    return 1
  fi
}

github_guard_hint() {
  if ! is_github_host; then
    return 0
  fi
  cat >&2 <<EOF

GitHub note: release downloads use HTTPS, not git@ SSH. For private repos set
GH_TOKEN/GITHUB_TOKEN or run \`gh auth login\`. If the only release is a
pre-release, the shim falls back to the newest published release automatically
(same as \`dotf self-update\`: /releases/latest, then /releases[0]).

On Fedora/RHEL the shim itself needs only curl and tar (both in @core), but
release discovery needs gh or python3: 'sudo dnf install -y gh' (or
'sudo dnf install -y python3').
EOF
}

# Install official Go toolchain under $HOME/.local/go (no sudo).
install_go_toolchain() {
  go_os="$OS"
  go_arch="$ARCH"
  tarball="go${GO_BOOTSTRAP_VERSION}.${go_os}-${go_arch}.tar.gz"
  url="https://go.dev/dl/${tarball}"
  dest_root="${HOME}/.local"
  stamp="${TMP}/go-bootstrap.tar.gz"

  printf '%s\n' "dotf bootstrap: downloading official Go ${GO_BOOTSTRAP_VERSION} from go.dev …" >&2
  curl -fsSL "$url" -o "$stamp" || die "failed to download ${url} (network required for --install-go)"

  mkdir -p "$dest_root" || die "failed to create ${dest_root}"
  rm -rf "${dest_root}/go"
  tar -xzf "$stamp" -C "$dest_root" || die "failed to extract Go toolchain"

  [ -x "${dest_root}/go/bin/go" ] || die "Go toolchain missing ${dest_root}/go/bin/go after extract"
  PATH="${dest_root}/go/bin:${PATH}"
  export PATH
  go_version_ok || die "installed Go at ${dest_root}/go is older than 1.26"
  printf '%s\n' "dotf bootstrap: Go $(go env GOVERSION) installed at ${dest_root}/go" >&2
}

# Build dotf from this clone into $TMP and exec with apply-injection.
build_and_exec_from_source() {
  [ -n "$SCRIPT_DIR" ] && [ -d "${SCRIPT_DIR}/cmd/dotf" ] || \
    die "source build requires a clone with cmd/dotf (run from the repo root)"

  if ! go_version_ok; then
    install_go_toolchain
  fi

  printf '%s\n' "dotf bootstrap: building ${BINARY} from ${SCRIPT_DIR} …" >&2
  (
    cd "${SCRIPT_DIR}" || exit 1
    # -buildvcs=false: parent dirs (e.g. a stray ~/.git) can make `go` fail VCS
    # stamping with exit 128; bootstrap only needs a runnable binary.
    go build -buildvcs=false -o "${TMP}/${BINARY}" ./cmd/dotf
  ) || die "go build failed (module download may need network)"

  [ -x "${TMP}/${BINARY}" ] || die "go build did not produce executable ${TMP}/${BINARY}"
  exec_dotf "$@"
}

# Prompt (or --install-go) then build from source; otherwise print instructions and exit.
offer_source_build_or_exit() {
  reason="$1"
  shift

  github_guard_hint

  if [ -z "$SCRIPT_DIR" ] || [ ! -d "${SCRIPT_DIR}/cmd/dotf" ]; then
    guard "$reason (source build needs a clone with cmd/dotf)"
  fi

  need_go_install=0
  if ! go_version_ok; then
    need_go_install=1
  fi

  if [ "$INSTALL_GO" -eq 1 ]; then
    build_and_exec_from_source "$@"
  fi

  if is_noninteractive "$@"; then
    cat >&2 <<EOF
dotf bootstrap: $reason

Non-interactive mode: refusing to install Go or build without --install-go.

  • Authenticate for private/prerelease downloads: GH_TOKEN or \`gh auth login\`
  • Or from this clone, re-run with an explicit consent flag:
      ./bootstrap.sh --install-go
  • Or build manually (Go 1.26+):
      go build -o ${BINARY} ./cmd/dotf
      ./${BINARY} apply --headless
EOF
    exit 1
  fi

  printf '%s\n' "dotf bootstrap: $reason" >&2
  if [ "$need_go_install" -eq 1 ]; then
    printf '%s' "Install Go 1.26+ from the official toolchain and build dotf from this clone? [y/N] " >&2
  else
    printf '%s' "Build dotf from this clone with the Go on PATH? [y/N] " >&2
  fi

  answer=""
  if ! read -r answer; then
    answer=""
  fi
  case "$answer" in
    y|Y|yes|YES)
      build_and_exec_from_source "$@"
      ;;
    *)
      guard "$reason (declined source build)"
      ;;
  esac
}

# Inject default subcommand & exec (shared by download and source-build paths).
exec_dotf() {
  if [ "$#" -eq 0 ] || [ "${1#-}" != "$1" ]; then
    set -- apply "$@"
  fi
  exec "${TMP}/${BINARY}" "$@"
}

# --- Download archive + checksums (latest release redirect; 404 -> source offer) -
if ! download_release_file "${ASSET}" "${TMP}/${ASSET}"; then
  offer_source_build_or_exit \
    "could not download '${ASSET}' from the latest release of ${REPO_SLUG} at ${REPO_ORIGIN}" \
    "$@"
fi
if ! download_release_file "checksums.txt" "${TMP}/checksums.txt"; then
  offer_source_build_or_exit \
    "could not download 'checksums.txt' from the latest release of ${REPO_SLUG} at ${REPO_ORIGIN}" \
    "$@"
fi

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

exec_dotf "$@"
