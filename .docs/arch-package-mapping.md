# Arch Linux Package Mapping for Homebrew Packages

Research completed: 2025-11-05

## Package Mapping Table

| Homebrew Package        | Arch Package            | Repository | Notes/Alternative Method                        |
| ----------------------- | ----------------------- | ---------- | ----------------------------------------------- |
| ack                     | ack                     | official   | Perl-based grep alternative                     |
| amazon-ecs-cli          | amazon-ecs-cli          | AUR        | AWS ECS command line interface                  |
| ansible                 | ansible                 | official   | Automation platform                             |
| ansible-language-server | ansible-language-server | official   | LSP for Ansible                                 |
| asciinema               | asciinema               | official   | Terminal session recorder                       |
| autojump                | autojump                | official   | Fast directory navigation                       |
| awscli                  | aws-cli                 | official   | AWS command line interface                      |
| aws-sam-cli             | aws-sam-cli             | AUR        | AWS Serverless Application Model CLI            |
| azure-cli               | azure-cli               | AUR        | Microsoft Azure CLI                             |
| bash                    | bash                    | official   | GNU Bourne Again shell                          |
| bat                     | bat                     | official   | Cat clone with syntax highlighting              |
| bitwarden-cli           | bitwarden-cli           | official   | Bitwarden command line interface                |
| bottom                  | bottom                  | official   | System monitor (btm)                            |
| carapace                | carapace-bin            | AUR        | Multi-shell completion generator                |
| cfn-lint                | python-cfn-lint         | official   | CloudFormation linter                           |
| clang-format            | clang                   | official   | Part of clang package                           |
| cmake                   | cmake                   | official   | Cross-platform build system                     |
| coreutils               | coreutils               | official   | GNU core utilities (native on Linux)            |
| curl                    | curl                    | official   | URL transfer tool                               |
| direnv                  | direnv                  | official   | Environment variable manager                    |
| dotenvx                 | dotenvx                 | AUR        | Enhanced dotenv tool                            |
| exiftool                | perl-image-exiftool     | official   | Image metadata tool                             |
| eza                     | eza                     | official   | Modern ls replacement                           |
| fd                      | fd                      | official   | Fast find alternative                           |
| ffmpeg                  | ffmpeg                  | official   | Multimedia framework                            |
| file                    | file                    | official   | File type identification                        |
| flow                    | flow                    | official   | Static type checker for JavaScript              |
| fping                   | fping                   | official   | Parallel ping utility                           |
| freetype                | freetype2               | official   | Font rendering library                          |
| fzf                     | fzf                     | official   | Fuzzy finder                                    |
| gawk                    | gawk                    | official   | GNU awk                                         |
| gcc                     | gcc                     | official   | GNU Compiler Collection                         |
| gemini-cli              | gemini-cli              | AUR        | CLI for Google Gemini API                       |
| gh                      | github-cli              | official   | GitHub CLI                                      |
| git                     | git                     | official   | Version control system                          |
| git-delta               | git-delta               | official   | Syntax-highlighting pager for git               |
| git-flow                | gitflow-avh             | official   | Git extensions for Vincent Driessen's model     |
| git-lfs                 | git-lfs                 | official   | Git Large File Storage                          |
| gnu-sed                 | sed                     | official   | GNU sed (native on Linux)                       |
| golang                  | go                      | official   | Go programming language                         |
| golangci-lint           | golangci-lint           | official   | Go linters aggregator                           |
| hadolint                | hadolint-bin            | AUR        | Dockerfile linter                               |
| terraform               | terraform               | official   | Infrastructure as code tool                     |
| httpie                  | httpie                  | official   | HTTP client                                     |
| jq                      | jq                      | official   | JSON processor                                  |
| lazygit                 | lazygit                 | official   | Terminal UI for git                             |
| libgit2                 | libgit2                 | official   | Git linkable library                            |
| libjpeg                 | libjpeg-turbo           | official   | JPEG library                                    |
| libpng                  | libpng                  | official   | PNG library                                     |
| lua                     | lua                     | official   | Lua programming language                        |
| luarocks                | luarocks                | official   | Lua package manager                             |
| mtr                     | mtr                     | official   | Network diagnostic tool                         |
| netcat                  | gnu-netcat              | official   | GNU netcat (also openbsd-netcat)                |
| nmap                    | nmap                    | official   | Network mapper                                  |
| pandoc                  | pandoc                  | official   | Universal document converter                    |
| pkg-config              | pkgconf                 | official   | Package config system                           |
| prettier                | prettier                | official   | Code formatter                                  |
| protobuf                | protobuf                | official   | Protocol buffers                                |
| pyenv                   | pyenv                   | AUR        | Python version manager                          |
| python                  | python                  | official   | Python 3 interpreter                            |
| rg (ripgrep)            | ripgrep                 | official   | Fast grep alternative                           |
| rtmpdump                | rtmpdump                | official   | RTMP streaming media client                     |
| ruby                    | ruby                    | official   | Ruby programming language                       |
| screen                  | screen                  | official   | Terminal multiplexer                            |
| sipcalc                 | sipcalc                 | official   | IP subnet calculator                            |
| skaffold                | skaffold                | AUR        | Kubernetes development tool                     |
| speedtest-cli           | speedtest-cli           | official   | Command line speedtest.net client               |
| tcpdump                 | tcpdump                 | official   | Network packet analyzer                         |
| tflint                  | tflint-bin              | AUR        | Terraform linter                                |
| thefuck                 | thefuck                 | official   | Command correction tool                         |
| tmux                    | tmux                    | official   | Terminal multiplexer                            |
| tmuxinator-completion   | N/A                     | N/A        | Not needed - use tmuxinator from official repos |
| tmux-mem-cpu-load       | tmux-mem-cpu-load       | AUR        | System stats for tmux                           |
| tmux-sessionizer        | N/A                     | N/A        | Custom script - install manually                |
| tmux-xpanes             | tmux-xpanes             | AUR        | Multi-pane tmux manager                         |
| tree                    | tree                    | official   | Directory tree viewer                           |
| tree-sitter             | tree-sitter             | official   | Parser generator tool                           |
| tree-sitter-cli         | tree-sitter-cli         | official   | CLI for tree-sitter                             |
| watch                   | procps-ng               | official   | Part of procps-ng                               |
| wget                    | wget                    | official   | URL transfer tool                               |
| xh                      | xh                      | official   | HTTP client (httpie alternative)                |
| yq                      | yq                      | official   | YAML processor                                  |
| zmq                     | zeromq                  | official   | High-performance messaging library              |
| zsh                     | zsh                     | official   | Z shell                                         |
| zsh-autosuggestions     | zsh-autosuggestions     | official   | Additional completion definitions               |
| zsh-completions         | zsh-completions         | official   | Additional completion definitions               |
| zsh-syntax-highlighting | zsh-syntax-highlighting | official   | Fish-like syntax highlighting                   |

## Installation Instructions

### Official Repositories

Install packages from official repositories using `pacman`:

```bash
sudo pacman -S package-name
```

### AUR Packages

Install AUR packages using an AUR helper like `yay` or `paru`:

```bash
yay -S package-name
# or
paru -S package-name
```

Alternatively, build manually:

```bash
git clone https://aur.archlinux.org/package-name.git
cd package-name
makepkg -si
```

## Key Differences from Homebrew

1. **Native GNU Tools**: Linux uses GNU coreutils, sed, etc. by default (no `g` prefix needed)
2. **Package Names**: Some packages have different names:
   - `awscli` → `aws-cli`
   - `gh` → `github-cli`
   - `golang` → `go`
   - `pkg-config` → `pkgconf`
   - `rg` → `ripgrep`
3. **Library Packages**: Libraries use standard names without prefixes:
   - `libjpeg` → `libjpeg-turbo`
   - `freetype` → `freetype2`
4. **Python Packages**: Python libraries prefixed with `python-`:
   - `cfn-lint` → `python-cfn-lint`
5. **Binary Packages**: Some AUR packages have `-bin` suffix for pre-compiled binaries:
   - `carapace-bin`, `hadolint-bin`, `tflint-bin`

## Notes

- **tmuxinator**: Available as `tmuxinator` in official repos; separate completion package not needed
- **tree-sitter**: Both library (`tree-sitter`) and CLI (`tree-sitter-cli`) available
- **netcat**: Multiple variants available (`gnu-netcat`, `openbsd-netcat`)
- **watch**: Included in `procps-ng` package
- **Custom Scripts**: Tools like `tmux-sessionizer` are custom scripts and should be installed manually

## Package Groups

Some related packages are available as groups:

- `base-devel`: Essential development tools including `gcc`, `make`, etc.

## References

- [Arch Linux Official Repositories](https://archlinux.org/packages/)
- [Arch User Repository (AUR)](https://aur.archlinux.org/)
- [Arch Wiki - List of Applications](https://wiki.archlinux.org/title/List_of_applications)
