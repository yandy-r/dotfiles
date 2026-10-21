#!/usr/bin/env bash
# ================================================================
# Enhanced Bash Prompt
# ================================================================
# Modern bash prompt with git integration and status indicators
# Fallback for when Starship is not available

# Check for bash version compatibility
if [[ "${BASH_VERSION%%.*}" -lt 4 ]]; then
    # Simple prompt for bash 3.x
    if [[ "$TERM" == *"256color"* ]]; then
        PS1='\[\033[01;32m\]\u@\h\[\033[00m\]:\[\033[01;34m\]\w\[\033[00m\]\$ '
    else
        PS1='\u@\h:\w\$ '
    fi
    return 0
fi

# Color definitions - check for bash 4.0+ for associative arrays
if [[ "${BASH_VERSION%%.*}" -ge 4 ]]; then
    declare -A colors=(
        [reset]='\[\033[0m\]'
        [black]='\[\033[0;30m\]'
        [red]='\[\033[0;31m\]'
        [green]='\[\033[0;32m\]'
        [yellow]='\[\033[0;33m\]'
        [blue]='\[\033[0;34m\]'
        [magenta]='\[\033[0;35m\]'
        [cyan]='\[\033[0;36m\]'
        [white]='\[\033[0;37m\]'
        [bright_black]='\[\033[1;30m\]'
        [bright_red]='\[\033[1;31m\]'
        [bright_green]='\[\033[1;32m\]'
        [bright_yellow]='\[\033[1;33m\]'
        [bright_blue]='\[\033[1;34m\]'
        [bright_magenta]='\[\033[1;35m\]'
        [bright_cyan]='\[\033[1;36m\]'
        [bright_white]='\[\033[1;37m\]'
    )
else
    # Fallback for bash 3.x - simple variables
    color_reset='\[\033[0m\]'
    color_red='\[\033[0;31m\]'
    color_green='\[\033[0;32m\]'
    color_yellow='\[\033[0;33m\]'
    color_blue='\[\033[0;34m\]'
    color_cyan='\[\033[0;36m\]'
    color_bright_green='\[\033[1;32m\]'
fi

# Helper function to get color value (works with both bash 3.x and 4+)
get_color() {
    local color_name="$1"
    if [[ "${BASH_VERSION%%.*}" -ge 4 ]]; then
        echo "${colors[$color_name]}"
    else
        # Use simple variables for bash 3.x
        case "$color_name" in
            reset) echo "$color_reset" ;;
            red) echo "$color_red" ;;
            green) echo "$color_green" ;;
            yellow) echo "$color_yellow" ;;
            blue) echo "$color_blue" ;;
            cyan) echo "$color_cyan" ;;
            bright_green) echo "$color_bright_green" ;;
            user_color) echo "$color_blue" ;;
            host_color) echo "$color_yellow" ;;
            path_color) echo "$color_cyan" ;;
            git_clean) echo "$color_bright_green" ;;
            git_dirty) echo "$color_yellow" ;;
            git_staged) echo "$color_yellow" ;;
            white) echo '\[\033[0;37m\]' ;;
            magenta) echo '\[\033[0;35m\]' ;;
            bright_white) echo '\[\033[1;37m\]' ;;
            bright_red) echo '\[\033[1;31m\]' ;;
            *) echo "$color_reset" ;;
        esac
    fi
}

# Set color scheme based on terminal capability
if [[ "$TERM" == *"256color"* ]] && [[ "${BASH_VERSION%%.*}" -ge 4 ]]; then
    colors[user_color]='\[\033[38;5;39m\]'    # Blue
    colors[host_color]='\[\033[38;5;208m\]'   # Orange
    colors[path_color]='\[\033[38;5;159m\]'   # Light cyan
    colors[git_clean]='\[\033[38;5;118m\]'    # Light green
    colors[git_dirty]='\[\033[38;5;208m\]'    # Orange
    colors[git_staged]='\[\033[38;5;226m\]'   # Yellow
elif [[ "${BASH_VERSION%%.*}" -ge 4 ]]; then
    colors[user_color]="${colors[bright_blue]}"
    colors[host_color]="${colors[bright_yellow]}"
    colors[path_color]="${colors[bright_cyan]}"
    colors[git_clean]="${colors[bright_green]}"
    colors[git_dirty]="${colors[yellow]}"
    colors[git_staged]="${colors[bright_yellow]}"
fi

# ================================================================
# GIT STATUS FUNCTIONS
# ================================================================

# Get git branch name
git_branch() {
    local branch
    branch=$(git symbolic-ref --short HEAD 2>/dev/null)
    if [[ -n "$branch" ]]; then
        echo "$branch"
    else
        # Detached HEAD
        git rev-parse --short HEAD 2>/dev/null
    fi
}

# Get git status indicators
git_status() {
    local status
    local indicators=""
    
    if ! git rev-parse --git-dir > /dev/null 2>&1; then
        return
    fi
    
    # Check for staged files
    if ! git diff --cached --quiet 2>/dev/null; then
        indicators+="●"  # Staged changes
    fi
    
    # Check for unstaged changes
    if ! git diff --quiet 2>/dev/null; then
        indicators+="○"  # Unstaged changes
    fi
    
    # Check for untracked files
    if [[ -n $(git ls-files --others --exclude-standard 2>/dev/null) ]]; then
        indicators+="?"  # Untracked files
    fi
    
    # Check for stashed files
    if git rev-parse --verify refs/stash >/dev/null 2>&1; then
        indicators+="$"  # Stashed changes
    fi
    
    echo "$indicators"
}

# Get git status color
git_status_color() {
    local status_indicators
    status_indicators=$(git_status)
    
    if [[ -z "$status_indicators" ]]; then
        echo "${colors[git_clean]}"
    elif [[ "$status_indicators" == *"●"* ]]; then
        echo "${colors[git_staged]}"
    else
        echo "${colors[git_dirty]}"
    fi
}

# ================================================================
# SYSTEM STATUS FUNCTIONS
# ================================================================

# Get Python virtual environment
python_venv() {
    if [[ -n "$VIRTUAL_ENV" ]]; then
        echo " ($(basename "$VIRTUAL_ENV"))"
    fi
}

# Get background jobs count
background_jobs() {
    local job_count
    job_count=$(jobs -r | wc -l)
    if [[ "$job_count" -gt 0 ]]; then
        echo " [${job_count}j]"
    fi
}

# Get exit code indicator
exit_code() {
    local exit_code=$1
    if [[ "$exit_code" -ne 0 ]]; then
        echo " ${colors[bright_red]}✗${colors[reset]}"
    fi
}

# ================================================================
# PROMPT BUILDER
# ================================================================

# Build git prompt segment into _dotf_ps1_git. Branch names and status come
# from the repo, which may be untrusted, so the raw values live in variables and
# the segment only references them: bash never re-expands the value of an
# expanded parameter, so `$(…)` or backticks in a branch name stay literal.
# Must run in the current shell (not a command substitution) so the variables
# PS1 references persist.
git_prompt() {
    _dotf_ps1_git=""
    _dotf_ps1_branch=$(git_branch)
    _dotf_ps1_git_status=""
    if [[ -n "$_dotf_ps1_branch" ]]; then
        _dotf_ps1_git_status=$(git_status)
        _dotf_ps1_git=" $(git_status_color)git:\${_dotf_ps1_branch}\${_dotf_ps1_git_status}${colors[reset]}"
    fi
}

# Build the main prompt and assign PS1 (called from PROMPT_COMMAND). Untrusted
# values are referenced as \${var}, never spliced into PS1; see git_prompt.
build_prompt() {
    local exit_code=$?
    local prompt=""

    # User and host
    prompt+="${colors[user_color]}\\u"
    prompt+="${colors[white]}@"
    prompt+="${colors[host_color]}\\h"
    prompt+="${colors[reset]}"

    # Current directory
    prompt+=" ${colors[path_color]}\\w${colors[reset]}"

    # Git information
    git_prompt
    prompt+="$_dotf_ps1_git"

    # Python virtual environment
    _dotf_ps1_venv=$(python_venv)
    prompt+="${colors[magenta]}\${_dotf_ps1_venv}${colors[reset]}"

    # Background jobs
    prompt+="${colors[yellow]}$(background_jobs)${colors[reset]}"

    # Exit code indicator
    prompt+="$(exit_code "$exit_code")"

    # Prompt symbol
    prompt+="\\n${colors[bright_white]}❯${colors[reset]} "

    PS1="$prompt"
}

# ================================================================
# VI MODE INDICATOR
# ================================================================

# Show vi mode in prompt
show_vi_mode() {
    case $READLINE_LINE in
        *) 
            if [[ "$KEYMAP" == "vicmd" ]]; then
                echo "${colors[bright_red]}[N]${colors[reset]} "
            else
                echo "${colors[bright_green]}[I]${colors[reset]} "
            fi
        ;;
    esac
}

# ================================================================
# PROMPT SETUP
# ================================================================

# Rebuild PS1 before every prompt. build_prompt runs in the current shell so
# the variables PS1 references are set where prompt expansion reads them.
PROMPT_COMMAND='build_prompt'

# Secondary prompt
PS2="${colors[bright_white]}❯ ${colors[reset]}"

# Debug prompt (shown when in debug mode)
PS4="${colors[bright_yellow]}+ ${colors[reset]}"