# Installer Scripts Reference

Comprehensive collection of installer scripts for cloud tools, CLI utilities, and development tools with official installation methods.

## Table of Contents

- [AWS Tools](#aws-tools)
- [Azure CLI](#azure-cli)
- [HashiCorp Tools](#hashicorp-tools)
- [Container/K8s Tools](#containerk8s-tools)
- [Common Features](#common-features)

---

## AWS Tools

**Script:** `install-aws-tools.sh`

Install AWS CLI tools using official AWS installation methods.

### Tools Included

| Tool      | Description    | Source                       |
| --------- | -------------- | ---------------------------- |
| `aws`     | AWS CLI v2     | Official AWS installer       |
| `sam`     | AWS SAM CLI    | Official AWS installer       |
| `ecs-cli` | Amazon ECS CLI | Official AWS S3 distribution |

### Usage

```bash
# Install all AWS tools
./bin/install-aws-tools.sh

# Install specific tools
./bin/install-aws-tools.sh aws sam

# Check versions
./bin/install-aws-tools.sh --check

# Preview installation
./bin/install-aws-tools.sh --dry-run

# Custom installation directory
./bin/install-aws-tools.sh --dir ~/.local/bin
```

### Installation Methods

- **AWS CLI v2:** Official installer packages
  - macOS: PKG installer (arm64 and x86_64)
  - Linux: Bundled installer (x86_64 and aarch64)
- **SAM CLI:** Official AWS installer
  - macOS: PKG installer (arm64 and x86_64)
  - Linux: Bundled installer (x86_64 and arm64)
- **ECS CLI:** Binary from official AWS S3 distribution (`amazon-ecs-cli.s3.amazonaws.com`)

### Notes

- AWS CLI v2 includes auto-completion
- SAM CLI symlinks to the configured install directory on Linux and `/usr/local/bin` on macOS
- ECS CLI: macOS is amd64-only (Rosetta 2 on Apple Silicon); Linux supports amd64 and arm64

---

## Azure CLI

**Script:** `install-azure-cli.sh`

Install Azure CLI using Microsoft's official installation method.

### Tools Included

| Tool | Description | Source                        |
| ---- | ----------- | ----------------------------- |
| `az` | Azure CLI   | Official Microsoft repository |

### Usage

```bash
# Install Azure CLI
./bin/install-azure-cli.sh

# Check version
./bin/install-azure-cli.sh --check

# Preview installation
./bin/install-azure-cli.sh --dry-run
```

### Installation Methods

- **Debian/Ubuntu:** Microsoft APT repository (official script)
- **Arch Linux:** AUR package (yay/paru) or community repo
- **RHEL/Fedora/CentOS:** Microsoft Yum repository
- **macOS:** Homebrew
- **Fallback:** pip3 install

### Notes

- Uses Microsoft's official installation script on Debian/Ubuntu
- Automatically configures Microsoft's package repositories
- Supports all major Linux distributions
- Includes shell completion for bash and zsh

---

## HashiCorp Tools

**Script:** `install-hashicorp-tools.sh`

Install HashiCorp tools using official HashiCorp repositories and releases.

### Tools Included

| Tool        | Description            | Source                        |
| ----------- | ---------------------- | ----------------------------- |
| `terraform` | Infrastructure as Code | Official HashiCorp repository |
| `tflint`    | Terraform linter       | GitHub releases               |

### Usage

```bash
# Install all HashiCorp tools
./bin/install-hashicorp-tools.sh

# Install specific tools
./bin/install-hashicorp-tools.sh terraform

# Check versions
./bin/install-hashicorp-tools.sh --check

# Preview installation
./bin/install-hashicorp-tools.sh --dry-run

# Custom installation directory
./bin/install-hashicorp-tools.sh --dir /usr/local/bin
```

### Installation Methods

- **Terraform:**
  - macOS: Homebrew tap (hashicorp/tap)
  - Debian/Ubuntu: HashiCorp APT repository
  - RHEL/Fedora: HashiCorp Yum repository
  - Arch: pacman (community repository)
  - Fallback: Binary from releases.hashicorp.com
- **TFLint:** Binary from GitHub releases

### Notes

- Automatically adds HashiCorp official repositories
- GPG keys verified for Debian/Ubuntu installations
- Supports all architectures (amd64, arm64)
- TFLint supports multiple Terraform versions

---

## Container/K8s Tools

**Script:** `install-container-tools.sh`

Install container and Kubernetes development tools.

### Tools Included

| Tool       | Description              | Source                        |
| ---------- | ------------------------ | ----------------------------- |
| `skaffold` | K8s development workflow | Official Google Cloud storage |
| `hadolint` | Dockerfile linter        | GitHub releases               |

### Usage

```bash
# Install all container tools
./bin/install-container-tools.sh

# Install specific tools
./bin/install-container-tools.sh skaffold hadolint

# Check versions
./bin/install-container-tools.sh --check

# Preview installation
./bin/install-container-tools.sh --dry-run

# Custom installation directory
./bin/install-container-tools.sh --dir ~/.local/bin
```

### Installation Methods

- **Skaffold:**
  - macOS: Homebrew
  - Linux: Official installer from storage.googleapis.com
- **Hadolint:** Binary from GitHub releases

### Notes

- Skaffold supports live development workflow with K8s
- Hadolint validates Dockerfile best practices
- Both tools support amd64 and arm64 architectures
- Works with Docker, Podman, and containerd

---

## Common Features

All installer scripts share these features:

### ✅ Check Mode

Check installed versions without installing:

```bash
./bin/install-aws-tools.sh --check
./bin/install-azure-cli.sh --check
./bin/install-hashicorp-tools.sh --check
```

### ✅ Dry Run Mode

Preview what would be done without making changes:

```bash
./bin/install-aws-tools.sh --dry-run
./bin/install-container-tools.sh --dry-run
```

### ✅ Selective Installation

Install only specific tools:

```bash
./bin/install-aws-tools.sh aws sam          # Only AWS CLI and SAM
./bin/install-hashicorp-tools.sh terraform  # Only Terraform
```

### ✅ Custom Installation Directory

Specify where to install binaries:

```bash
./bin/install-aws-tools.sh --dir ~/.local/bin
./bin/install-hashicorp-tools.sh --dir /usr/local/bin
```

### ✅ Multiple Architecture Support

- **x86_64** / amd64
- **aarch64** / arm64
- Automatic detection using `common/platform.sh`

### ✅ Platform Support

- **macOS:** arm64 (Apple Silicon) and x86_64 (Intel)
- **Linux:** All major distributions
  - Debian/Ubuntu
  - RHEL/Fedora/CentOS/Rocky/Alma
  - Arch/Manjaro
- **WSL:** Supported via Linux detection

### ✅ Error Handling

- Set `-euo pipefail` for strict error checking
- Validates downloads and checksums where available
- Verifies binaries work after installation
- Cleanup of temporary files on failure

### ✅ Colored Output

Consistent color-coded status messages:

- 🔵 **BLUE** - Info messages
- 🟢 **GREEN** - Success messages
- 🟡 **YELLOW** - Warnings
- 🔴 **RED** - Errors
- 🔷 **CYAN** - Headers

### ✅ Official Sources Only

All tools installed from official sources:

- Official vendor repositories (AWS, Microsoft, HashiCorp)
- Official GitHub releases (verified organizations)
- Official installation scripts
- No third-party mirrors or unofficial sources

---

## Installation Best Practices

### Before Installing

1. **Check prerequisites:**

   ```bash
   # Verify platform detection
   source common/platform.sh && echo "$DOTFILES_OS $DOTFILES_ARCH"
   ```

2. **Use dry-run first:**

   ```bash
   ./bin/install-aws-tools.sh --dry-run
   ```

3. **Check existing installations:**

   ```bash
   ./bin/install-aws-tools.sh --check
   ```

### Installation Preferences

Scripts prefer installation methods in this order:

1. **System package manager** (apt, yum, pacman)
   - Integrates with system updates
   - Handles dependencies automatically

2. **Official repositories** (HashiCorp, Microsoft)
   - Receives updates via package manager
   - GPG verified packages

3. **Homebrew** (macOS and Linux)
   - Consistent experience across platforms
   - Easy updates with `brew upgrade`

4. **pip3** (Python tools)
   - For Python-based CLI tools
   - Installs to `~/.local/bin`

5. **Binary releases** (GitHub)
   - Fallback for unsupported distributions
   - Always uses latest stable version

### Security Considerations

- ✅ All downloads use HTTPS
- ✅ GPG keys verified where available
- ✅ Binaries tested after installation
- ✅ Official sources only
- ✅ Version verification via API
- ⚠️ Manual checksum verification not yet implemented (future enhancement)

### Troubleshooting

#### Tool not in PATH

```bash
# Add to your shell config (.zshrc or .bashrc)
export PATH="$HOME/.local/bin:$PATH"
export PATH="/usr/local/bin:$PATH"
```

#### Permission denied

```bash
# Create local bin directory
mkdir -p ~/.local/bin

# Install to user directory
./bin/install-aws-tools.sh --dir ~/.local/bin
```

#### Download fails

```bash
# Check internet connection
curl -I https://github.com

# Check GitHub API rate limits
curl -H "Accept: application/vnd.github.v3+json" https://api.github.com/rate_limit
```

#### Binary doesn't work

```bash
# Check architecture
uname -m

# Verify binary
file $(which tool-name)

# Try reinstalling
./bin/install-tool.sh --dry-run tool-name
```

---

## Related Documentation

- **Platform Detection:** `common/platform.sh` - OS and architecture detection
- **Tool Detection:** `common/tools.sh` - Comprehensive tool management
- **Bootstrap:** `bootstrap.sh` - Initial system setup

---

## Future Enhancements

Planned improvements for installer scripts:

- [ ] Checksum verification for all downloads
- [ ] Version pinning support
- [ ] Parallel installation of multiple tools
- [ ] Installation progress indicators
- [ ] Automatic updates via scheduled job
- [ ] Rollback to previous versions
- [ ] Tool configuration management
- [ ] Integration with dotfiles bootstrap

---

## Contributing

When adding new installer scripts:

1. **Follow the template** from existing scripts
2. **Use official sources** only
3. **Support multiple architectures** (x86_64, arm64)
4. **Include dry-run mode** for testing
5. **Add error handling** with proper cleanup
6. **Document in this file** with usage examples
7. **Test on multiple platforms** before committing

---

## Support

For issues or questions:

1. Check script help: `./bin/script-name.sh --help`
2. Use dry-run mode: `./bin/script-name.sh --dry-run`
3. Review this documentation
4. Check individual tool documentation
5. Verify platform support in `common/platform.sh`
