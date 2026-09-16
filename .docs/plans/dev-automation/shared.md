# Development Environment Automation

The dotfiles repository has comprehensive automation for development environment setup through the `bootstrap.sh` script, Ansible playbooks for symlink management, and automated tool installation. The current implementation includes cross-platform setup, intelligent tool detection, and automated configuration deployment. This plan focuses on enhancing the automation capabilities, adding more sophisticated setup workflows, and implementing better environment validation and recovery mechanisms.

## Relevant Files

- `bootstrap.sh`: Main automation script for cross-platform environment setup
- `playbooks/symlinks.yml`: Ansible playbook for automated symlink management with backup and dry-run support
- `bin/install-*.sh`: Individual installation scripts for specific tools and services
- `common/tools.sh`: Automated tool detection, installation, and setup
- `common/platform.sh`: Automated platform detection and configuration
- `ansible.cfg`: Ansible configuration for automated deployments

## Relevant Tables

- Installation status tracking and validation
- Symlink mapping and backup management
- Tool installation dependencies and order

## Relevant Patterns

**Bootstrap Pattern**: Comprehensive setup script with platform detection, tool installation, and configuration deployment (`bootstrap.sh`)

**Ansible Automation Pattern**: Idempotent configuration management with backup, dry-run, and safety checks (`playbooks/symlinks.yml`)

**Tool Installation Pattern**: Automated tool detection and installation with platform-specific package manager integration (`common/tools.sh:140-174`)

**Configuration Validation Pattern**: Setup validation and health checks for installed components

## Relevant Docs

**`CLAUDE.md`**: You _must_ read this when working on setup and deployment automation, symlink management, and bootstrap processes.

**`.cursorrules`**: You _must_ read this when working on Ansible playbook guidelines, automation best practices, and deployment standards.

**`README.md`**: You _must_ read this when working on automation documentation, quick start guides, and setup process descriptions.
