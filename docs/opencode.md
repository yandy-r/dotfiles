# OpenCode 2

OpenCode 2 is installed through `dotf` or `bin/install-opencode.sh`. OpenCode 1 and 2 both use the `opencode` command; this repository no longer installs a separate `opencode2` package.

## Install with `dotf`

OpenCode belongs to the opt-in `ai` intent:

```bash
dotf apply --intent ai
```

The manifest entry uses `bin/install-opencode.sh` instead of the generic npm backend. This is required so `dotf` can remove an existing V1 package before installing V2 while preserving shared settings and data.

`dotf` probes `~/.local/state/dotf/opencode-v2`, a symlink the installer creates only after verifying a V2 binary. It cannot probe only the `opencode` command because V1 uses the same name; doing so would incorrectly skip migration on V1 hosts. Because the marker points at the installed binary, removing OpenCode outside this repository leaves a dangling link, and `dotf` reports the package as absent.

## Manual lifecycle

```bash
# Install V2, migrating from V1 when found
./bin/install-opencode.sh --install

# Update V2
./bin/install-opencode.sh --update

# Install a specific V2 version through the official installer
./bin/install-opencode.sh --version 2.0.6

# Force reinstall
./bin/install-opencode.sh --force

# Remove binaries while retaining configuration and session data
./bin/install-opencode.sh --uninstall
```

## V1 migration

OpenCode's V2 migration guide requires package-managed V1 installations to be removed before V2 installation. V1 and V2 share configuration locations, so the installer deliberately does **not** delete or rewrite:

- `~/.config/opencode/`
- OpenCode data, sessions, credentials, or project `.opencode/` directories

The installer detects V1 by version and known legacy packages:

- Homebrew: `opencode`
- npm: `opencode-ai`
- Arch/AUR: `opencode-bin`

It removes those packages, removes any remaining V1 binary found on `PATH`, then installs V2. V2 reads supported V1 configuration and migrates terminal client settings on first startup.

## Official platform methods

Methods follow current [OpenCode V2 installation docs](https://opencode.ai/v2/docs/):

### macOS

1. Homebrew: `brew install anomalyco/tap/opencode-v2`
2. Official V2 installer: `curl -fsSL https://opencode.ai/v2/install | bash`
3. Node package manager fallback (postinstall explicitly allowed; see below)

### Arch Linux

1. AUR: `paru -S opencode-beta`
2. Official V2 installer
3. Node package manager fallback

### Ubuntu, Fedora, openSUSE, WSL, and other Linux

1. Official V2 installer
2. Node package manager fallback

### Other Unix-like systems

1. Official V2 installer when supported
2. Node package manager fallback

The official installer receives `--no-modify-path` because this repository already adds `~/.opencode/bin` in `common/paths.sh`.

### Postinstall script gating

`@opencode/cli` selects its platform binary in a postinstall script. Package managers that block dependency install scripts must be told to allow it, otherwise the install completes without a working binary:

```bash
npm install -g --allow-scripts=@opencode/cli @opencode/cli@latest
bun install -g --trust @opencode/cli@latest
pnpm add -g --allow-build=@opencode/cli @opencode/cli@latest
```

The installer uses these forms. Notes:

- The requested version tag (`latest` or `OPENCODE_VERSION`) is explicit. Package-manager supply-chain policies, such as a minimum release age, remain in force and may defer a newly published `latest` version.
- npm releases older than 11.16 do not recognize `--allow-scripts`, so the installer omits the flag for them.

## Verify

```bash
command -v opencode
opencode --version
```

Expected major version: `2` or newer.

## Configuration migration

V2 continues reading supported V1 configuration from:

```text
~/.config/opencode/opencode.json(c)
<project>/opencode.json(c)
<project>/.opencode/opencode.json(c)
```

Configuration conversion is optional. See [Migrate from V1](https://opencode.ai/v2/docs/migrate-v1) for plugin/API breaking changes and native V2 shapes.

## Usage

```bash
cd /path/to/project
opencode
```

Useful V2 commands:

```bash
opencode run "Explain this repository"
opencode mini
opencode service status
```

Alias `oc` runs `opencode --auto`.

## Troubleshooting

### V1 remains first on `PATH`

```bash
type -a opencode
opencode --version
```

Remove the reported V1 package or binary, then rerun:

```bash
./bin/install-opencode.sh --force
```

### `opencode` not found after curl installation

Ensure repository shell paths are loaded, or add:

```bash
export PATH="$HOME/.opencode/bin:$PATH"
```

### Preserve settings during uninstall

Use this repository's `--uninstall` action. Do not run bare `opencode uninstall` unless its data-removal behavior is intended; V2 documents `opencode uninstall --keep-config --keep-data` for retaining both configuration and data.

## Resources

- [OpenCode V2 docs](https://opencode.ai/v2/docs/)
- [V1 to V2 migration](https://opencode.ai/v2/docs/migrate-v1)
- [V2 CLI guide](https://opencode.ai/v2/docs/cli)
