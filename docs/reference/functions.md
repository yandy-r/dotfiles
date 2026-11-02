# Utility Functions Reference

This document provides a complete reference for all shell functions defined in the dotfiles repository. Functions are organized by category and platform availability.

## Table of Contents

- [Directory and File Operations](#directory-and-file-operations)
- [Archive Operations](#archive-operations)
- [Text Processing](#text-processing)
- [Network Functions](#network-functions)
- [Git Functions](#git-functions)
- [File Compression and Comparison](#file-compression-and-comparison)
- [Process Management](#process-management)
- [Development Functions](#development-functions)
- [System Information](#system-information)
- [Docker Functions](#docker-functions)
- [Clipboard Functions](#clipboard-functions)
- [Tmux Functions](#tmux-functions)
- [Herdr Functions](#herdr-functions)
- [macOS-Specific Functions](#macos-specific-functions)

---

## Directory and File Operations

Functions for creating, finding, and managing files and directories.

### mkd

Create a directory and change into it.

```bash
mkd <directory_path>
```

**Parameters:**

| Parameter        | Type   | Required | Description              |
| ---------------- | ------ | -------- | ------------------------ |
| `directory_path` | string | Yes      | Path to create and enter |

**Example:**

```bash
# Create nested directories and cd into them
mkd my-project/src/components

# After execution, you'll be in: ./my-project/src/components
```

---

### mkf

Create a file along with any necessary parent directories.

```bash
mkf <file_path>
```

**Parameters:**

| Parameter   | Type   | Required | Description                |
| ----------- | ------ | -------- | -------------------------- |
| `file_path` | string | Yes      | Path to the file to create |

**Example:**

```bash
# Create file with directory structure
mkf src/components/Button/Button.tsx

# Creates: src/components/Button/ directory
# Creates: src/components/Button/Button.tsx file
```

---

### f

Find files by name. Uses `fd` or `fdfind` (`fd` preferred) when one is on PATH, otherwise `find`.

```bash
f <pattern> [additional_args]
```

**Parameters:**

| Parameter         | Type   | Required | Description                            |
| ----------------- | ------ | -------- | -------------------------------------- |
| `pattern`         | string | Yes      | Filename pattern to search             |
| `additional_args` | string | No       | Additional arguments passed to fd/find |

**Example:**

```bash
# Find all TypeScript files
f "*.ts"

# Find files containing "config"
f config
```

**Fallback Behavior:**

- With `fd` or `fdfind`: that command, `"$@"`
- With neither: `find . -name "*$1*" -type f`

---

### fd_dir

Find directories by name. Uses `fd` or `fdfind` (`fd` preferred) when one is on PATH, otherwise `find`.

```bash
fd_dir <pattern> [additional_args]
```

**Parameters:**

| Parameter         | Type   | Required | Description                            |
| ----------------- | ------ | -------- | -------------------------------------- |
| `pattern`         | string | Yes      | Directory name pattern to search       |
| `additional_args` | string | No       | Additional arguments passed to fd/find |

**Example:**

```bash
# Find all directories named "components"
fd_dir components

# Find directories containing "test"
fd_dir test
```

---

### cdf

Change to the frontmost Finder window directory (macOS) or most recently accessed directory (Linux).

```bash
cdf
```

**Platform Behavior:**

- **macOS**: Changes to the directory shown in the frontmost Finder window
- **Linux**: Changes to the most recently accessed directory from `~/.local/share/recently-used.xbel`

**Example:**

```bash
# On macOS: Navigate to current Finder location
cdf
```

---

### filesize

Get file size in human-readable format.

```bash
filesize <file>
```

**Parameters:**

| Parameter | Type   | Required | Description      |
| --------- | ------ | -------- | ---------------- |
| `file`    | string | Yes      | Path to the file |

**Example:**

```bash
filesize package.json
# Output: 2.3K
```

---

### cleanup_files

Recursively delete files matching a pattern.

```bash
cleanup_files [pattern] [path]
```

**Parameters:**

| Parameter | Type   | Required | Default     | Description           |
| --------- | ------ | -------- | ----------- | --------------------- |
| `pattern` | string | No       | `.DS_Store` | File pattern to match |
| `path`    | string | No       | `.`         | Starting directory    |

**Example:**

```bash
# Delete all .DS_Store files in current directory
cleanup_files

# Delete all .pyc files in src directory
cleanup_files "*.pyc" ./src

# Delete all backup files
cleanup_files "*~" ~/Documents
```

---

## Archive Operations

Functions for extracting and creating archives.

### extract

Universal archive extractor supporting multiple formats.

```bash
extract <archive_file>
```

**Parameters:**

| Parameter      | Type   | Required | Description         |
| -------------- | ------ | -------- | ------------------- |
| `archive_file` | string | Yes      | Path to the archive |

**Supported Formats:**

| Extension                   | Tool Used  |
| --------------------------- | ---------- |
| `.tar.bz2`, `.tbz2`, `.tbz` | tar xjf    |
| `.tar.gz`, `.tgz`           | tar xzf    |
| `.tar.xz`, `.txz`           | tar Jxf    |
| `.tar.Z`, `.taz`            | tar xzf    |
| `.tar`                      | tar xf     |
| `.zip`                      | unzip      |
| `.rar`                      | unrar      |
| `.7z`                       | 7z         |
| `.gz`                       | gunzip     |
| `.bz2`                      | bunzip2    |
| `.xz`                       | unxz       |
| `.Z`                        | uncompress |

**Example:**

```bash
# Extract any supported archive
extract project.tar.gz
extract backup.zip
extract archive.7z
```

**Behavior:**

- Creates a directory with the archive name (without extension)
- Prompts before overwriting existing directories
- Changes into the extracted directory

---

### archive

Create archives in various formats.

```bash
archive <archive_name> <files...>
```

**Parameters:**

| Parameter      | Type   | Required | Description                                              |
| -------------- | ------ | -------- | -------------------------------------------------------- |
| `archive_name` | string | Yes      | Output archive filename (format determined by extension) |
| `files`        | string | Yes      | Files/directories to archive                             |

**Supported Formats:**

| Extension           | Tool Used |
| ------------------- | --------- |
| `.tar.gz`, `.tgz`   | tar czf   |
| `.tar.bz2`, `.tbz2` | tar cjf   |
| `.tar.xz`, `.txz`   | tar cJf   |
| `.tar`              | tar cf    |
| `.zip`              | zip -r    |
| `.7z`               | 7z a      |

**Example:**

```bash
# Create a gzipped tarball
archive backup.tar.gz src/ config/

# Create a zip file
archive project.zip *.js package.json

# Create a 7z archive
archive data.7z large-directory/
```

---

## Text Processing

Functions for processing and transforming text.

### json

Pretty print and syntax highlight JSON input.

```bash
json [json_string]
# or
echo '{"key": "value"}' | json
```

**Parameters:**

| Parameter     | Type   | Required | Description                   |
| ------------- | ------ | -------- | ----------------------------- |
| `json_string` | string | No       | JSON to format (or use stdin) |

**Example:**

```bash
# From argument
json '{"name": "John", "age": 30}'

# From pipe
curl -s https://api.example.com/data | json

# From file
cat data.json | json
```

**Tool Fallback:**

1. Uses `jq` if available
2. Falls back to `python3 -m json.tool`
3. Pipes through `bat` or `batcat` (`bat` preferred) for syntax highlighting when one is on PATH

---

### urlencode

URL encode a string.

```bash
urlencode <string>
```

**Parameters:**

| Parameter | Type   | Required | Description      |
| --------- | ------ | -------- | ---------------- |
| `string`  | string | Yes      | String to encode |

**Example:**

```bash
urlencode "Hello World!"
# Output: Hello+World%21

urlencode "https://example.com?q=test query"
# Output: https%3A%2F%2Fexample.com%3Fq%3Dtest+query
```

---

### urldecode

URL decode a string.

```bash
urldecode <string>
```

**Parameters:**

| Parameter | Type   | Required | Description                  |
| --------- | ------ | -------- | ---------------------------- |
| `string`  | string | Yes      | URL-encoded string to decode |

**Example:**

```bash
urldecode "Hello+World%21"
# Output: Hello World!
```

---

### b64encode

Base64 encode data.

```bash
b64encode <string>
# or
echo "data" | b64encode
```

**Parameters:**

| Parameter | Type   | Required | Description                     |
| --------- | ------ | -------- | ------------------------------- |
| `string`  | string | No       | String to encode (or use stdin) |

**Example:**

```bash
# From argument
b64encode "Hello World"
# Output: SGVsbG8gV29ybGQ=

# From pipe
cat file.bin | b64encode
```

---

### b64decode

Base64 decode data.

```bash
b64decode <string>
# or
echo "encoded" | b64decode
```

**Parameters:**

| Parameter | Type   | Required | Description                          |
| --------- | ------ | -------- | ------------------------------------ |
| `string`  | string | No       | Base64-encoded string (or use stdin) |

**Example:**

```bash
b64decode "SGVsbG8gV29ybGQ="
# Output: Hello World
```

---

### escape

Escape UTF-8 characters as hex sequences.

```bash
escape <string>
```

**Parameters:**

| Parameter | Type   | Required | Description                        |
| --------- | ------ | -------- | ---------------------------------- |
| `string`  | string | Yes      | String containing UTF-8 characters |

**Example:**

```bash
escape "Hello"
# Output: \x48\x65\x6C\x6C\x6F
```

---

### unidecode

Decode Unicode escape sequences.

```bash
unidecode <string>
```

**Parameters:**

| Parameter | Type   | Required | Description                 |
| --------- | ------ | -------- | --------------------------- |
| `string`  | string | Yes      | String with Unicode escapes |

**Example:**

```bash
unidecode '\u0048\u0065\u006C\u006C\u006F'
# Output: Hello
```

---

## Network Functions

Functions for network diagnostics and HTTP testing.

### httpcompression

Test if a URL uses HTTP compression.

```bash
httpcompression <url>
```

**Parameters:**

| Parameter | Type   | Required | Description |
| --------- | ------ | -------- | ----------- |
| `url`     | string | Yes      | URL to test |

**Example:**

```bash
httpcompression https://example.com
# Output: https://example.com is encoded using gzip
```

---

### httpstatus

Get the HTTP status code for a URL.

```bash
httpstatus <url>
```

**Parameters:**

| Parameter | Type   | Required | Description  |
| --------- | ------ | -------- | ------------ |
| `url`     | string | Yes      | URL to check |

**Example:**

```bash
httpstatus https://example.com
# Output: 200

httpstatus https://example.com/nonexistent
# Output: 404
```

---

### httpheaders

Get HTTP response headers for a URL.

```bash
httpheaders <url>
```

**Parameters:**

| Parameter | Type   | Required | Description               |
| --------- | ------ | -------- | ------------------------- |
| `url`     | string | Yes      | URL to fetch headers from |

**Example:**

```bash
httpheaders https://example.com
# Output:
# HTTP/2 200
# content-type: text/html; charset=UTF-8
# ...
```

---

### portscan

Scan a range of ports on a host.

```bash
portscan [host] [start_port] [end_port]
```

**Parameters:**

| Parameter    | Type    | Required | Default     | Description   |
| ------------ | ------- | -------- | ----------- | ------------- |
| `host`       | string  | No       | `localhost` | Host to scan  |
| `start_port` | integer | No       | `1`         | Starting port |
| `end_port`   | integer | No       | `1000`      | Ending port   |

**Example:**

```bash
# Scan localhost ports 1-1000
portscan

# Scan specific host
portscan 192.168.1.1 80 443

# Scan web server ports
portscan example.com 80 8080
```

---

### myips

Display public and local IP addresses.

```bash
myips
```

**Example:**

```bash
myips
# Output:
# Public IP: 203.0.113.42
# Local IPs:
#   192.168.1.100
#   10.0.0.5
```

---

### dig_all

Perform DNS lookups for multiple record types.

```bash
dig_all <domain>
```

**Parameters:**

| Parameter | Type   | Required | Description     |
| --------- | ------ | -------- | --------------- |
| `domain`  | string | Yes      | Domain to query |

**Record Types Queried:** A, AAAA, CNAME, MX, TXT, NS

**Example:**

```bash
dig_all example.com
# Output:
# === A records for example.com ===
# 93.184.216.34
#
# === AAAA records for example.com ===
# 2606:2800:220:1:248:1893:25c8:1946
# ...
```

---

## Git Functions

Functions for enhanced Git workflows.

### git_log_github

Display Git log with clickable GitHub URLs (works best in iTerm).

```bash
git_log_github [git_log_args]
```

**Parameters:**

| Parameter      | Type   | Required | Description                      |
| -------------- | ------ | -------- | -------------------------------- |
| `git_log_args` | string | No       | Additional arguments for git log |

**Example:**

```bash
# Show log with GitHub links
git_log_github

# Limit to last 10 commits
git_log_github -n 10
```

---

### gitclone

Clone a repository and change into its directory.

```bash
gitclone <repository_url>
```

**Parameters:**

| Parameter        | Type   | Required | Description        |
| ---------------- | ------ | -------- | ------------------ |
| `repository_url` | string | Yes      | Git repository URL |

**Example:**

```bash
gitclone https://github.com/user/repo.git
# Clones repo and cd's into ./repo
```

---

### gitexport

Export the current Git repository without the `.git` directory.

```bash
gitexport [target_directory]
```

**Parameters:**

| Parameter          | Type   | Required | Default | Description        |
| ------------------ | ------ | -------- | ------- | ------------------ |
| `target_directory` | string | No       | `.`     | Export destination |

**Example:**

```bash
# Export to current directory
gitexport

# Export to specific location
gitexport /tmp/export
```

---

### gitclean

Remove merged branches (except main, master, develop).

```bash
gitclean
```

**Example:**

```bash
gitclean
# Output:
# Cleaning merged branches...
# Deleted branch feature/old-feature (was abc1234).
```

---

## File Compression and Comparison

### gz

Show original and gzipped file sizes with compression ratio.

```bash
gz <file>
```

**Parameters:**

| Parameter | Type   | Required | Description     |
| --------- | ------ | -------- | --------------- |
| `file`    | string | Yes      | File to analyze |

**Example:**

```bash
gz large-file.json
# Output:
# Original size: 102400 bytes
# Gzipped size:  15360 bytes
# Compression:   15.00%
```

---

### compare

Compare two files side by side using delta or diff.

```bash
compare <file1> <file2>
```

**Parameters:**

| Parameter | Type   | Required | Description |
| --------- | ------ | -------- | ----------- |
| `file1`   | string | Yes      | First file  |
| `file2`   | string | Yes      | Second file |

**Example:**

```bash
compare old-config.json new-config.json
```

**Tool Fallback:**

1. Uses `delta` if available (with syntax highlighting)
2. Falls back to `diff -u` piped through `bat` or `batcat` (`bat` preferred)

---

## Process Management

Functions for managing processes.

### killall_name

Find and kill processes by name with confirmation.

```bash
killall_name <process_name>
```

**Parameters:**

| Parameter      | Type   | Required | Description                   |
| -------------- | ------ | -------- | ----------------------------- |
| `process_name` | string | Yes      | Process name pattern to match |

**Example:**

```bash
killall_name node
# Output:
# Found processes matching 'node':
#   PID COMMAND          ARGS
# 12345 node             /usr/bin/node server.js
# 12346 node             /usr/bin/node worker.js
# Kill these processes? (y/n)
```

---

### killport

Kill the process using a specific port.

```bash
killport <port>
```

**Parameters:**

| Parameter | Type    | Required | Description |
| --------- | ------- | -------- | ----------- |
| `port`    | integer | Yes      | Port number |

**Example:**

```bash
killport 3000
# Output: Killing process 12345 using port 3000
```

---

### whoisport

Show what process is using a specific port.

```bash
whoisport <port>
```

**Parameters:**

| Parameter | Type    | Required | Description |
| --------- | ------- | -------- | ----------- |
| `port`    | integer | Yes      | Port number |

**Example:**

```bash
whoisport 8080
# Output: (shows lsof or ss output for the port)
```

---

## Development Functions

Functions for development workflows.

### graphify

Run graphify. When `TOKENHOP_OPENAI_API_KEY` is unset, this calls the real
binary with the arguments unchanged.

When that key is set, the call exports:

- `GRAPHIFY_BACKEND` — `openai`, unless `GRAPHIFY_BACKEND` is already set
- `OPENAI_API_KEY` — the tokenhop key
- `OPENAI_BASE_URL` — `TOKENHOP_OPENAI_BASE_URL`, when that variable is set
- `OPENAI_MODEL` and `GRAPHIFY_OPENAI_MODEL` — `TOKENHOP_OPENAI_MODEL`, when that variable is set

`common/exports.sh` loads those `TOKENHOP_OPENAI_*` variables from
`$XDG_CONFIG_HOME/env/tokenhop_openai_api_key`,
`tokenhop_openai_base_url`, and `tokenhop_openai_model`. A `--backend` argument
is passed through.

### graphify-raw

Run the `graphify` binary directly, with no tokenhop environment.

```bash
graphify-raw update .
```

### serve

Start a simple HTTP server.

```bash
serve [port] [directory]
```

**Parameters:**

| Parameter   | Type    | Required | Default | Description        |
| ----------- | ------- | -------- | ------- | ------------------ |
| `port`      | integer | No       | `8000`  | Port to serve on   |
| `directory` | string  | No       | `.`     | Directory to serve |

**Example:**

```bash
# Serve current directory on port 8000
serve

# Serve specific directory on custom port
serve 3000 ./dist

# Serve build output
serve 9000 ./build
```

**Tool Fallback:**

1. Uses `python3 -m http.server` if available
2. Falls back to `python -m SimpleHTTPServer`
3. Falls back to `npx serve`

---

### genpass

Generate a random password.

```bash
genpass [length]
```

**Parameters:**

| Parameter | Type    | Required | Default | Description     |
| --------- | ------- | -------- | ------- | --------------- |
| `length`  | integer | No       | `16`    | Password length |

**Example:**

```bash
genpass
# Output: Xk7mN2pQ9rT4vW8z

genpass 32
# Output: Xk7mN2pQ9rT4vW8zAb3cD5eF6gH7jK8m
```

---

### qr

Generate a QR code for text.

```bash
qr <text>
```

**Parameters:**

| Parameter | Type   | Required | Description           |
| --------- | ------ | -------- | --------------------- |
| `text`    | string | Yes      | Text or URL to encode |

**Example:**

```bash
qr "https://example.com"
# Output: UTF-8 QR code in terminal

qr "WiFi:S:MyNetwork;T:WPA;P:password;;"
```

**Tool Fallback:**

1. Uses `qrencode -t UTF8` if available
2. Falls back to online QR API

---

## System Information

Functions for displaying system information.

### sysinfo

Display comprehensive system information.

```bash
sysinfo
```

**Example:**

```bash
sysinfo
# Output:
# === System Information ===
# OS: macos
# Architecture: arm64
# Container: none
# Package Manager: homebrew
# Homebrew Path: /opt/homebrew
#
# === Hardware ===
# Model Name: MacBook Pro
# Chip: Apple M1 Pro
# Memory: 16 GB
#
# === Disk Usage ===
# /dev/disk1s1   466Gi  234Gi  232Gi    51%  /
```

---

### needreboot

Check if a system reboot is required (Linux only).

```bash
needreboot
```

**Example:**

```bash
needreboot
# Output: *** System restart required ***
# linux-image-5.15.0-generic

# or
# Output: No reboot required
```

---

## Docker Functions

Functions for Docker operations.

### docker-clean

Clean up Docker containers, images, volumes, and networks.

```bash
docker-clean
```

**Example:**

```bash
docker-clean
# Output:
# Cleaning Docker containers, images, and volumes...
# Deleted Containers:
#   abc123def456
# Total reclaimed space: 1.2GB
```

---

### op-dc-up

Start Docker Compose with optional 1Password integration.

```bash
op-dc-up
```

**Behavior:**

- If `op` (1Password CLI) is available, runs with secrets injection
- Otherwise, runs standard `docker compose up -d`

**Example:**

```bash
# With 1Password
op-dc-up
# Runs: op run --no-masking -- docker compose up -d --remove-orphans

# Without 1Password
op-dc-up
# Runs: docker compose up -d --remove-orphans
```

---

### op-dc-down

Stop Docker Compose and clean up networks.

```bash
op-dc-down
```

**Example:**

```bash
op-dc-down
# Runs: docker compose down --remove-orphans
# Runs: docker network prune -f
```

---

### dsh

Open a shell in a Docker container.

```bash
dsh <container_name> [shell]
```

**Parameters:**

| Parameter        | Type   | Required | Default | Description          |
| ---------------- | ------ | -------- | ------- | -------------------- |
| `container_name` | string | Yes      | -       | Container name or ID |
| `shell`          | string | No       | `sh`    | Shell to use         |

**Example:**

```bash
# Open sh shell
dsh my-container

# Open bash shell
dsh my-container bash

# List available containers (no argument)
dsh
# Output:
# Available containers:
# NAMES          IMAGE               STATUS
# web            nginx:latest        Up 2 hours
# db             postgres:13         Up 2 hours
```

---

## Clipboard Functions

Functions for clipboard operations.

### clip

Copy text to clipboard (enhanced version of the `copy` alias).

```bash
clip <text>
# or
echo "text" | clip
```

**Parameters:**

| Parameter | Type   | Required | Description                 |
| --------- | ------ | -------- | --------------------------- |
| `text`    | string | No       | Text to copy (or use stdin) |

**Example:**

```bash
# Copy argument
clip "Hello World"

# Copy from pipe
cat file.txt | clip

# Copy command output
pwd | clip
```

---

### unclip

Paste from clipboard (enhanced version of the `paste` alias).

```bash
unclip
```

**Example:**

```bash
# Paste clipboard contents
unclip
# Output: (clipboard contents)
```

---

### copypath

Copy the current directory path to clipboard.

```bash
copypath
```

**Example:**

```bash
copypath
# Output: Current directory path copied to clipboard
# Clipboard now contains: /Users/user/projects/my-project
```

---

## Tmux Functions

Functions for tmux session management.

### fix_tmux_path

Update the PATH environment variable in tmux to prefer Homebrew tools.

```bash
fix_tmux_path
```

**Example:**

```bash
# Inside a tmux session
fix_tmux_path
# Output: Tmux PATH updated with Homebrew tools preferred
```

---

### tmux_session

Create or attach to a tmux session with proper PATH environment.

```bash
tmux_session <session_name>
```

**Parameters:**

| Parameter      | Type   | Required | Description              |
| -------------- | ------ | -------- | ------------------------ |
| `session_name` | string | Yes      | Name of the tmux session |

**Example:**

```bash
# Create or attach to session "dev"
tmux_session dev
```

---

### ta

Attach to a tmux session (function for tab completion support).

```bash
ta <session_name>
```

**Parameters:**

| Parameter      | Type   | Required | Description          |
| -------------- | ------ | -------- | -------------------- |
| `session_name` | string | Yes      | Session to attach to |

**Example:**

```bash
ta my-session
```

---

### ts

Create a new tmux session (function for tab completion support).

```bash
ts <session_name>
```

**Parameters:**

| Parameter      | Type   | Required | Description              |
| -------------- | ------ | -------- | ------------------------ |
| `session_name` | string | Yes      | Name for the new session |

**Example:**

```bash
ts new-project
```

---

### tk

Kill a tmux session (function for tab completion support).

```bash
tk <session_name>
```

**Parameters:**

| Parameter      | Type   | Required | Description     |
| -------------- | ------ | -------- | --------------- |
| `session_name` | string | Yes      | Session to kill |

**Example:**

```bash
tk old-session
```

---

## Herdr Functions

Functions for [Herdr](https://github.com/herdrdev/herdr) panes. Defined in
`common/functions.sh`, so bash and zsh both load them.

### herdr-run-all

Send one command to other Herdr panes. Herdr has no equivalent of tmux
`synchronize-panes`. The default scope is the current workspace. The calling
pane is skipped, and so is any pane whose `.agent` field is set, because
`herdr pane run` writes into an agent TUI rather than a shell prompt.

```bash
herdr-run-all [-a|-t] [-s] [-A] [-n] <command> [args...]
```

**Parameters:**

| Parameter | Type | Required | Description |
| --------- | ---- | -------- | ----------- |
| `-a`, `--all-workspaces` | flag | No | Every pane in every workspace |
| `-t`, `--tab` | flag | No | Only panes in the current tab (`HERDR_TAB_ID`) |
| `-s`, `--include-self` | flag | No | Also send to the calling pane |
| `-A`, `--include-agents` | flag | No | Also send to panes hosting a coding agent |
| `-n`, `--dry-run` | flag | No | Print target pane IDs instead of sending |
| `command` | string | Yes | Command passed to `herdr pane run` |

Passing both `-a` and `-t` keeps the one that appears last. The function
returns 2 for a missing command or an unknown option. It returns 1 when
`HERDR_ENV` is not `1`, `herdr` or `jq` is missing, `HERDR_WORKSPACE_ID` is
unset for a workspace or tab scope, `herdr pane list` fails, or no pane
matches. A failed `herdr pane run` is reported and the function returns 1
after the remaining panes.

**Example:**

```bash
# From a Herdr pane: run in every other shell pane of this workspace
herdr-run-all git status

# Show the pane IDs that -t would target, including this pane
herdr-run-all -t -s -n true
```

---

## macOS-Specific Functions

Functions available only on macOS.

### macos_info

Display detailed macOS system information.

```bash
macos_info
```

**Example:**

```bash
macos_info
# Output:
# === macOS System Information ===
# ProductName:    macOS
# ProductVersion: 14.0
# BuildVersion:   23A344
#
# Model Name: MacBook Pro
# Processor: Apple M1 Pro
# Memory: 16 GB
#
# Uptime: 5 days 3 hours
```

---

### checkupdates

Check for all available updates (macOS, App Store, Homebrew).

```bash
checkupdates
```

**Example:**

```bash
checkupdates
# Output:
# Checking for macOS updates...
# (software update list)
#
# Checking Mac App Store updates...
# (mas outdated list)
#
# Checking Homebrew updates...
# (brew outdated list)
```

---

### installUpdates

Install all available updates.

```bash
installUpdates
```

**Example:**

```bash
installUpdates
# Installs: macOS updates, App Store updates, Homebrew updates
```

---

### killapp

Kill an application by name.

```bash
killapp <application_name>
```

**Parameters:**

| Parameter          | Type   | Required | Description             |
| ------------------ | ------ | -------- | ----------------------- |
| `application_name` | string | Yes      | Name of the application |

**Example:**

```bash
killapp Safari
# Output: Killed Safari

killapp Chrome
# Output: Chrome not found or already stopped
```

---

### launch

Launch an application by name.

```bash
launch <application_name>
```

**Parameters:**

| Parameter          | Type   | Required | Description             |
| ------------------ | ------ | -------- | ----------------------- |
| `application_name` | string | Yes      | Name of the application |

**Example:**

```bash
launch Safari
launch "Visual Studio Code"
```

---

### appinfo

Get detailed information about an installed application.

```bash
appinfo <application_name>
```

**Parameters:**

| Parameter          | Type   | Required | Description             |
| ------------------ | ------ | -------- | ----------------------- |
| `application_name` | string | Yes      | Name of the application |

**Example:**

```bash
appinfo Safari
# Output:
# Application: Safari
# Path: /Applications/Safari.app
# Version: 17.0
# Bundle ID: com.apple.Safari
# Size: 35M
```

---

### showfinder

Show a file or directory in Finder.

```bash
showfinder [path]
```

**Parameters:**

| Parameter | Type   | Required | Default | Description                 |
| --------- | ------ | -------- | ------- | --------------------------- |
| `path`    | string | No       | `.`     | File or directory to reveal |

**Example:**

```bash
showfinder
showfinder ~/Documents/report.pdf
```

---

### finderselection

Get the path of the currently selected item in Finder.

```bash
finderselection
```

**Example:**

```bash
finderselection
# Output: /Users/user/Documents/selected-file.txt
```

---

### finder_show_hidden / finder_hide_hidden

Toggle visibility of hidden files in Finder.

```bash
finder_show_hidden
finder_hide_hidden
```

**Example:**

```bash
finder_show_hidden
# Output: Hidden files are now visible in Finder

finder_hide_hidden
# Output: Hidden files are now hidden in Finder
```

---

### dock_add

Add an application to the Dock.

```bash
dock_add <application_path>
```

**Parameters:**

| Parameter          | Type   | Required | Description                  |
| ------------------ | ------ | -------- | ---------------------------- |
| `application_path` | string | Yes      | Full path to the application |

**Example:**

```bash
dock_add /Applications/Safari.app
```

---

### dock_remove

Remove an application from the Dock.

```bash
dock_remove <application_name>
```

**Parameters:**

| Parameter          | Type   | Required | Description             |
| ------------------ | ------ | -------- | ----------------------- |
| `application_name` | string | Yes      | Name of the application |

**Note:** Requires `dockutil` to be installed (`brew install dockutil`).

**Example:**

```bash
dock_remove Safari
```

---

### dock_spacer

Add a spacer tile to the Dock.

```bash
dock_spacer
```

**Example:**

```bash
dock_spacer
# Output: Spacer added to Dock
```

---

### screenshot

Take a screenshot with various options.

```bash
screenshot [type] [delay] [filename] [destination]
```

**Parameters:**

| Parameter     | Type    | Required | Default        | Description                           |
| ------------- | ------- | -------- | -------------- | ------------------------------------- |
| `type`        | string  | No       | `selection`    | Type: `selection`, `window`, `screen` |
| `delay`       | integer | No       | `0`            | Delay in seconds                      |
| `filename`    | string  | No       | Auto-generated | Output filename                       |
| `destination` | string  | No       | `~/Desktop`    | Save directory                        |

**Example:**

```bash
# Interactive selection
screenshot

# Capture window with 3 second delay
screenshot window 3

# Full screen to specific location
screenshot screen 0 myscreen.png ~/Pictures
```

---

### screenshotclip

Take a screenshot and copy to clipboard.

```bash
screenshotclip [type]
```

**Parameters:**

| Parameter | Type   | Required | Default     | Description                           |
| --------- | ------ | -------- | ----------- | ------------------------------------- |
| `type`    | string | No       | `selection` | Type: `selection`, `window`, `screen` |

**Example:**

```bash
screenshotclip window
```

---

### vol

Get or set the system volume.

```bash
vol [level]
```

**Parameters:**

| Parameter | Type    | Required | Description                               |
| --------- | ------- | -------- | ----------------------------------------- |
| `level`   | integer | No       | Volume level 0-100 (omit to show current) |

**Example:**

```bash
vol
# Output: 50

vol 75
# Output: Volume set to 75%
```

---

### mute_toggle

Toggle audio mute state.

```bash
mute_toggle
```

**Example:**

```bash
mute_toggle
# Output: Audio muted

mute_toggle
# Output: Audio unmuted
```

---

### wifi_status / wifi_on / wifi_off

Manage WiFi power state.

```bash
wifi_status
wifi_on
wifi_off
```

**Example:**

```bash
wifi_status
# Output: Wi-Fi Power (en0): On

wifi_off
# Output: WiFi turned off
```

---

### wifi_connect

Connect to a WiFi network.

```bash
wifi_connect <network_name> [password]
```

**Parameters:**

| Parameter      | Type   | Required | Description      |
| -------------- | ------ | -------- | ---------------- |
| `network_name` | string | Yes      | Network SSID     |
| `password`     | string | No       | Network password |

**Example:**

```bash
# Connect to open network
wifi_connect "Coffee Shop WiFi"

# Connect with password
wifi_connect "Home Network" "mypassword123"

# List preferred networks (no argument)
wifi_connect
```

---

### wifi_scan

Scan for available WiFi networks.

```bash
wifi_scan
```

**Example:**

```bash
wifi_scan
# Output: (list of available networks with signal strength)
```

---

### netinfo

Display comprehensive network information.

```bash
netinfo
```

**Example:**

```bash
netinfo
# Output:
# === Network Information ===
# WiFi Status: Wi-Fi Power (en0): On
# Current Network: Airport: Home-Network
# Internal IP (en0): 192.168.1.100
# Internal IP (en1): Not available
# External IP: 203.0.113.42
# DNS Servers: 8.8.8.8 8.8.4.4
```

---

### diskusage

Display disk usage and largest directories.

```bash
diskusage
```

**Example:**

```bash
diskusage
# Output:
# === Disk Usage ===
# /dev/disk1s1   466Gi  234Gi  232Gi    51%  /
#
# === Largest Directories ===
# (top 20 largest directories)
```

---

### eject

Eject a disk by name or path.

```bash
eject [disk_name]
```

**Parameters:**

| Parameter   | Type   | Required | Description                      |
| ----------- | ------ | -------- | -------------------------------- |
| `disk_name` | string | No       | Disk name (omit to list volumes) |

**Example:**

```bash
# List available volumes
eject
# Output:
# Available volumes:
# Macintosh HD
# External Drive

# Eject specific disk
eject "External Drive"
```

---

### volumes

Show mounted volumes.

```bash
volumes
```

**Example:**

```bash
volumes
# Output:
# === Mounted Volumes ===
# /dev/disk1s1 on / (apfs, local, journaled)
```

---

### battery / battery_info

Display battery status and information.

```bash
battery
battery_info
```

**Example:**

```bash
battery
# Output: 85%

battery_info
# Output:
# === Battery Information ===
# Now drawing from 'Battery Power'
# -InternalBattery-0 (id=...)  85%; charging; 1:30 remaining
#
# Cycle Count: 124
# Condition: Normal
```

---

### plist_read / plist_write

Read and write plist values.

```bash
plist_read <plist_file> <key>
plist_write <plist_file> <key> <value> [type]
```

**Parameters for plist_write:**

| Parameter    | Type   | Required | Default  | Description                            |
| ------------ | ------ | -------- | -------- | -------------------------------------- |
| `plist_file` | string | Yes      | -        | Path to plist file                     |
| `key`        | string | Yes      | -        | Key name                               |
| `value`      | string | Yes      | -        | Value to write                         |
| `type`       | string | No       | `string` | Type: `string`, `int`, `bool`, `float` |

**Example:**

```bash
plist_read ~/Library/Preferences/com.apple.finder.plist ShowHardDrivesOnDesktop
# Output: 0

plist_write ~/test.plist myKey "myValue" string
```

---

### ql

Quick Look preview files.

```bash
ql <file> [file2] [file3] ...
```

**Parameters:**

| Parameter | Type   | Required | Description        |
| --------- | ------ | -------- | ------------------ |
| `file`    | string | Yes      | File(s) to preview |

**Example:**

```bash
ql document.pdf
ql image1.png image2.png image3.png
```

---

### xcode_clean

Clean Xcode derived data.

```bash
xcode_clean
```

**Example:**

```bash
xcode_clean
# Output:
# Cleaning Xcode derived data...
# Xcode derived data cleaned
```

---

### ios_simulator / ios_list / ios_boot

Manage iOS simulators.

```bash
ios_simulator  # Open Simulator app
ios_list       # List available simulators
ios_boot [device_name]  # Boot specific simulator
```

**Example:**

```bash
ios_simulator

ios_list
# Output: (list of all simulators)

ios_boot "iPhone 15 Pro"
```

---

### spotlight

Search files using Spotlight.

```bash
spotlight <search_term>
```

**Parameters:**

| Parameter     | Type   | Required | Description        |
| ------------- | ------ | -------- | ------------------ |
| `search_term` | string | Yes      | Term to search for |

**Example:**

```bash
spotlight "project report"
# Output: (list of matching files)
```

---

### keychain_find / keychain_get

Search and retrieve Keychain items.

```bash
keychain_find <service_name>
keychain_get <service_name> [account]
```

**Example:**

```bash
keychain_find "GitHub"
# Output: (keychain entry details)

keychain_get "api.github.com" "username"
# Output: (password for that service/account)
```

---

### syspref

Open specific System Preference panes.

```bash
syspref <pane_name>
```

**Available Panes:** `general`, `desktop`, `dock`, `security`, `network`, `bluetooth`, `sound`, `users`, `time`

**Example:**

```bash
syspref network
syspref security
```

---

### brew_maintenance

Run Homebrew maintenance tasks.

```bash
brew_maintenance
```

**Example:**

```bash
brew_maintenance
# Output:
# Running Homebrew maintenance...
# (update, upgrade, cleanup, doctor output)
# Homebrew maintenance complete
```

---

### brew_which

Find which Homebrew package provides a command.

```bash
brew_which <command>
```

**Parameters:**

| Parameter | Type   | Required | Description             |
| --------- | ------ | -------- | ----------------------- |
| `command` | string | Yes      | Command name to look up |

**Example:**

```bash
brew_which rg
# Output: ripgrep
```

---

## Source Files

- **Cross-Platform Functions**: `/Users/yandy/.config/dotfiles/common/functions.sh`
- **macOS-Specific Functions**: `/Users/yandy/.config/dotfiles/common/macos/functions.sh`
