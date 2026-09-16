# Security Hardening

The dotfiles repository has several security configurations in place, including SSH configuration with security best practices, GPG setup for commit signing, secrets management with 1Password CLI, and telemetry disabling for privacy. The current implementation includes enhanced SSH configuration, proper key management, and security-focused environment variables. This plan focuses on expanding security measures, implementing additional hardening configurations, and adding security monitoring capabilities.

## Relevant Files

- `ssh/config`: Enhanced SSH configuration with security best practices and host-specific settings
- `common/exports.sh`: Security-focused environment variables and telemetry disabling
- `common/gitconfig`: Git configuration with GPG signing and security settings
- `scripts/enableSSH.ps1`: Windows SSH server configuration script
- `bootstrap.sh`: Security-aware setup and configuration
- `common/tools.sh`: Security tool integration (1Password CLI, GPG, etc.)

## Relevant Tables

- SSH host configurations with security settings
- GPG key management and signing configurations
- Security tool installation and setup status

## Relevant Patterns

**SSH Security Pattern**: Enhanced SSH configuration with proper key management, host verification, and secure defaults (`ssh/config:1-261`)

**Secrets Management Pattern**: 1Password CLI integration with service account support and secure environment variable handling

**Telemetry Disabling Pattern**: Systematic disabling of telemetry and tracking across tools and services (`common/exports.sh`)

**Security Tool Integration Pattern**: Secure setup and configuration of security tools with proper key management

## Relevant Docs

**`CLAUDE.md`**: You _must_ read this when working on security best practices, secrets management, SSH and GPG configuration, and privacy settings.

**`.cursorrules`**: You _must_ read this when working on security best practices, secrets management guidelines, and SSH/GPG setup requirements.

**`README.md`**: You _must_ read this when working on security feature documentation and privacy-focused configurations.
