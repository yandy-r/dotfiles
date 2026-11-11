# Quick Install Reference

Fast reference for installing cloud and CLI tools using official methods.

## 🎯 Quick Commands

### AWS Tools

```bash
# Install all AWS tools (CLI v2, SAM, ECS CLI)
./bin/install-aws-tools.sh

# Check what's installed
./bin/install-aws-tools.sh --check

# Install only specific tools
./bin/install-aws-tools.sh aws sam
```

### Azure CLI

```bash
# Install Azure CLI
./bin/install-azure-cli.sh

# Check version
./bin/install-azure-cli.sh --check
```

### HashiCorp Tools

```bash
# Install Terraform and TFLint
./bin/install-hashicorp-tools.sh

# Install only Terraform
./bin/install-hashicorp-tools.sh terraform

# Check installed versions
./bin/install-hashicorp-tools.sh --check
```

### Container/Kubernetes Tools

```bash
# Install Skaffold and Hadolint
./bin/install-container-tools.sh

# Kubernetes ecosystem (kubectl, helm, k9s, etc.)
./bin/k8s-tools.sh

# Check K8s tools status
./bin/k8s-tools.sh --check
```

### Modern CLI Tools

```bash
# Install all modern CLI tools
# (lazygit, delta, bottom, eza, carapace, xh)
./bin/install-modern-cli.sh

# Install only specific tools
./bin/install-modern-cli.sh lazygit delta

# Check what's installed
./bin/install-modern-cli.sh --check

# Install to custom directory
./bin/install-modern-cli.sh --dir ~/.local/bin
```

## 🔍 Common Options

All installer scripts support these options:

| Option      | Description                | Example                                          |
| ----------- | -------------------------- | ------------------------------------------------ |
| `--check`   | Show installed versions    | `./bin/install-aws-tools.sh --check`             |
| `--dry-run` | Preview without installing | `./bin/install-azure-cli.sh --dry-run`           |
| `--dir DIR` | Custom install directory   | `./bin/install-modern-cli.sh --dir ~/.local/bin` |
| `--help`    | Show help message          | `./bin/install-hashicorp-tools.sh --help`        |

## 📦 What Each Script Installs

### AWS Tools (`install-aws-tools.sh`)

- **aws** - AWS CLI v2 (official installer)
- **sam** - AWS SAM CLI (official installer)
- **ecs-cli** - Amazon ECS CLI (GitHub releases)

### Azure (`install-azure-cli.sh`)

- **az** - Azure CLI (Microsoft official repo)

### HashiCorp (`install-hashicorp-tools.sh`)

- **terraform** - Infrastructure as Code (official HashiCorp repo)
- **tflint** - Terraform linter (GitHub releases)

### Container Tools (`install-container-tools.sh`)

- **skaffold** - K8s development workflow (official Google installer)
- **hadolint** - Dockerfile linter (GitHub releases)

### Kubernetes (`k8s-tools.sh`)

- **kubectl** - Kubernetes CLI
- **helm** - Kubernetes package manager
- **k9s** - Kubernetes TUI
- **kubectx/kubens** - Context/namespace switchers
- **kustomize** - Kubernetes customization
- **stern** - Multi-pod log tailing
- **krew** - kubectl plugin manager

### Modern CLI (`install-modern-cli.sh`)

- **lazygit** - Terminal UI for git
- **delta** - Better diff viewer
- **bottom** (btm) - System monitor
- **eza** - Better ls alternative
- **carapace** - Universal completion generator
- **xh** - Friendly HTTP client

## 🚀 Installation Workflow

### First Time Setup

```bash
# 1. Run bootstrap to set up base system
./bootstrap.sh

# 2. Install cloud tools you need
./bin/install-aws-tools.sh
./bin/install-azure-cli.sh
./bin/install-hashicorp-tools.sh

# 3. Install Kubernetes tools
./bin/k8s-tools.sh

# 4. Install modern CLI tools
./bin/install-modern-cli.sh

# 5. Check everything is installed
./bin/install-aws-tools.sh --check
./bin/install-modern-cli.sh --check
./bin/k8s-tools.sh --check
```

### Preview Before Installing

```bash
# Test installation without making changes
./bin/install-aws-tools.sh --dry-run
./bin/install-azure-cli.sh --dry-run
./bin/install-modern-cli.sh --dry-run
```

### Selective Installation

```bash
# Install only what you need
./bin/install-aws-tools.sh aws          # AWS CLI only
./bin/install-hashicorp-tools.sh terraform
./bin/install-modern-cli.sh lazygit delta eza
./bin/k8s-tools.sh kubectl helm
```

## 📍 Installation Locations

Scripts install binaries to different locations based on permissions:

| Directory          | When Used             | Requires Sudo |
| ------------------ | --------------------- | ------------- |
| `/usr/local/bin`   | System-wide (default) | Yes           |
| `~/.local/bin`     | User-only (fallback)  | No            |
| Custom via `--dir` | User-specified        | Depends       |

**Tip:** For user-local installations without sudo:

```bash
./bin/install-modern-cli.sh --dir ~/.local/bin
```

## 🔧 Troubleshooting

### Tool not found after installation

```bash
# Add to PATH (add to ~/.zshrc or ~/.bashrc)
export PATH="$HOME/.local/bin:$PATH"
export PATH="/usr/local/bin:$PATH"

# Reload shell
exec zsh  # or: exec bash
```

### Permission denied errors

```bash
# Use user-local directory instead
./bin/install-modern-cli.sh --dir ~/.local/bin
```

### Download failures

```bash
# Check internet connection
curl -I https://github.com

# Check GitHub API rate limits
curl https://api.github.com/rate_limit
```

### Already installed but want to reinstall

```bash
# Remove existing installation first
which tool-name  # Find location
sudo rm $(which tool-name)  # Remove it

# Then reinstall
./bin/install-tool-script.sh tool-name
```

## ✅ Verification

After installation, verify tools are working:

```bash
# AWS Tools
aws --version
sam --version
ecs-cli --version

# Azure
az --version

# HashiCorp
terraform version
tflint --version

# Container Tools
skaffold version
hadolint --version

# Kubernetes
kubectl version --client
helm version
k9s version

# Modern CLI
lazygit --version
delta --version
btm --version
eza --version
carapace --version
xh --version
```

## 📚 More Information

- **Full Documentation:** [installers.md](installers.md)
- **Platform Detection:** `common/platform.sh`
- **Tool Management:** `common/tools.sh`
- **Bootstrap Guide:** `README.md`

## 💡 Pro Tips

1. **Use check mode first** to see what's already installed
2. **Preview with dry-run** before committing to installation
3. **Install selectively** - only install tools you actually use
4. **User-local installation** avoids sudo and system-wide changes
5. **Keep PATH updated** - add `~/.local/bin` to your PATH for user installs

## 🔄 Updates

To update tools later:

```bash
# Kubernetes tools support update mode
./bin/k8s-tools.sh --update kubectl k9s

# For others, reinstall to get latest version
./bin/install-aws-tools.sh aws
./bin/install-modern-cli.sh lazygit
```

## 🎯 Most Common Use Cases

### DevOps Engineer

```bash
./bin/install-aws-tools.sh
./bin/install-hashicorp-tools.sh
./bin/k8s-tools.sh
./bin/install-container-tools.sh
```

### Cloud Developer

```bash
./bin/install-aws-tools.sh
./bin/install-azure-cli.sh
./bin/k8s-tools.sh kubectl helm
```

### Modern CLI Enthusiast

```bash
./bin/install-modern-cli.sh
./bin/k8s-tools.sh --check  # Verify K8s tools
```

### Minimal Server Setup

```bash
./bin/install-aws-tools.sh aws
./bin/install-modern-cli.sh lazygit
```

---

**Questions?** Check the full documentation: [installers.md](installers.md)
