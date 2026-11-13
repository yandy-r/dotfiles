#!/usr/bin/env bash
# ================================================================
# Linux-Specific Aliases
# ================================================================
# Aliases that only work on Linux systems

# ================================================================
# PACKAGE MANAGEMENT
# ================================================================

# Detect package manager and create appropriate aliases
case "$DOTFILES_PKG_MGR" in
    apt)
        # Debian/Ubuntu APT
        alias install="sudo apt install"
        alias search="apt search"
        alias update="sudo apt update"
        alias upgrade="sudo apt update && sudo apt upgrade"
        alias full-upgrade="sudo apt update && sudo apt full-upgrade"
        alias remove="sudo apt remove"
        alias purge="sudo apt purge"
        alias autoremove="sudo apt autoremove"
        alias autoclean="sudo apt autoclean"
        alias show="apt show"
        alias list="apt list"
        alias listinstalled="apt list --installed"
        alias policy="apt policy"
        
        # APT shortcuts
        alias apti="sudo apt install"
        alias apts="apt search"
        alias aptu="sudo apt update && sudo apt upgrade"
        alias aptr="sudo apt remove"
        alias aptac="sudo apt autoclean"
        alias aptar="sudo apt autoremove"

        # Maintenance (see MAINTENANCE AND CLEANUP below)
        alias cleanup-packages="sudo apt autoremove && sudo apt autoclean"
        _dotf_updateall="sudo apt update && sudo apt upgrade"
        ;;
        
    dnf)
        # Fedora/RHEL DNF
        alias install="sudo dnf install"
        alias search="dnf search"
        alias update="sudo dnf upgrade"
        alias upgrade="sudo dnf upgrade"
        alias remove="sudo dnf remove"
        alias autoremove="sudo dnf autoremove"
        alias clean="sudo dnf clean all"
        alias show="dnf info"
        alias list="dnf list"
        alias listinstalled="dnf list installed"
        # Not `history`: that is a shell builtin and aliases win over builtins.
        alias pkghistory="dnf history"
        
        # DNF shortcuts
        alias dnfi="sudo dnf install"
        alias dnfs="dnf search"
        alias dnfu="sudo dnf upgrade"
        alias dnfr="sudo dnf remove"

        # Maintenance (see MAINTENANCE AND CLEANUP below)
        alias cleanup-packages="sudo dnf autoremove && sudo dnf clean all"
        _dotf_updateall="sudo dnf upgrade"
        ;;
        
    yum)
        # CentOS/RHEL YUM
        alias install="sudo yum install"
        alias search="yum search"
        alias update="sudo yum update"
        alias upgrade="sudo yum upgrade"
        alias remove="sudo yum remove"
        alias clean="sudo yum clean all"
        alias show="yum info"
        alias list="yum list"
        alias listinstalled="yum list installed"
        # Not `history`: that is a shell builtin and aliases win over builtins.
        alias pkghistory="yum history"

        # Maintenance (see MAINTENANCE AND CLEANUP below)
        alias cleanup-packages="sudo yum autoremove && sudo yum clean all"
        _dotf_updateall="sudo yum update"
        ;;
        
    pacman)
        # Arch Linux Pacman
        alias install="sudo pacman -S"
        alias search="pacman -Ss"
        alias update="sudo pacman -Syu"
        alias upgrade="sudo pacman -Syu"
        alias remove="sudo pacman -R"
        alias removeall="sudo pacman -Rns"
        alias clean="sudo pacman -Sc"
        alias show="pacman -Si"
        alias list="pacman -Q"
        alias listinstalled="pacman -Q"
        alias orphans="pacman -Qtdq"
        
        # Pacman shortcuts
        alias pac="sudo pacman"
        alias paci="sudo pacman -S"
        alias pacs="pacman -Ss"
        alias pacu="sudo pacman -Syu"
        alias pacr="sudo pacman -R"
        alias pacc="sudo pacman -Sc"

        # Maintenance (see MAINTENANCE AND CLEANUP below). `pacman -Qtdq` exits
        # non-zero when there are no orphans, which guards the removal. The
        # orphan list is deliberately unquoted so it splits into targets.
        alias cleanup-packages='sudo pacman -Sc; pacman -Qtdq >/dev/null 2>&1 && sudo pacman -Rns $(pacman -Qtdq)'
        _dotf_updateall="sudo pacman -Syu"

        # AUR helpers - comprehensive aliases
        if has_command yay; then
            # Installation and removal
            alias yayi="yay -S"              # Install package
            alias yays="yay -Ss"             # Search packages
            alias yayu="yay -Syu"            # Update all packages (including AUR)
            alias yayr="yay -R"              # Remove package
            alias yayrns="yay -Rns"          # Remove package with deps and config

            # Query operations
            alias yayq="yay -Q"              # Query installed packages
            alias yayqi="yay -Qi"            # Query package info
            alias yayql="yay -Ql"            # List package files
            alias yayqo="yay -Qo"            # Query which package owns file
            alias yayqe="yay -Qe"            # List explicitly installed
            alias yayqm="yay -Qm"            # List AUR/foreign packages

            # Maintenance
            alias yayc="yay -Sc"             # Clean package cache
            alias yaycc="yay -Scc"           # Clean all cache
            alias yayo="yay -Yc"             # Remove unneeded dependencies

            # Info and stats
            alias yaysi="yay -Si"            # Show remote package info
            alias yayps="yay -Ps"            # Print system stats

            # Build options
            alias yayg="yay -G"              # Get PKGBUILD
            alias yayb="yay -S --rebuild"    # Rebuild package

            # Make yay the default for generic package aliases on Arch
            alias install="yay -S"
            alias search="yay -Ss"
            alias update="yay -Syu"
            alias remove="yay -R"
            # Not `info`: GNU texinfo ships `info` in base-devel, which is a
            # build prerequisite for yay itself.
            alias pkginfo="yay -Si"
            _dotf_updateall="yay -Syu"

        elif has_command paru; then
            # Installation and removal
            alias parui="paru -S"            # Install package
            alias parus="paru -Ss"           # Search packages
            alias paruu="paru -Syu"          # Update all
            alias parur="paru -R"            # Remove package
            alias parurns="paru -Rns"        # Remove with deps and config

            # Query operations
            alias paruq="paru -Q"            # Query installed
            alias paruqi="paru -Qi"          # Query info
            alias paruqm="paru -Qm"          # List AUR packages

            # Maintenance
            alias paruc="paru -Sc"           # Clean cache
            alias parucc="paru -Scc"         # Clean all cache

            # Make paru the default
            alias install="paru -S"
            alias search="paru -Ss"
            alias update="paru -Syu"
            alias remove="paru -R"
            # Not `info`: GNU texinfo ships `info` in base-devel, which is a
            # build prerequisite for paru itself.
            alias pkginfo="paru -Si"
            _dotf_updateall="paru -Syu"
        fi
        ;;
        
    zypper)
        # openSUSE Zypper
        alias install="sudo zypper install"
        alias search="zypper search"
        alias update="sudo zypper refresh && sudo zypper update"
        alias upgrade="sudo zypper dup"
        alias remove="sudo zypper remove"
        alias clean="sudo zypper clean"
        alias show="zypper info"
        alias list="zypper packages"
        alias listinstalled="zypper packages --installed-only"

        # Maintenance (see MAINTENANCE AND CLEANUP below)
        alias cleanup-packages="sudo zypper clean"
        _dotf_updateall="sudo zypper dup"
        ;;
esac

# Flatpak (if available)
if has_command flatpak; then
    alias finstall="flatpak install"
    alias fsearch="flatpak search"
    alias fupdate="flatpak update"
    alias fremove="flatpak uninstall"
    alias flist="flatpak list"
    alias frun="flatpak run"
fi

# Snap (if available)
if has_command snap; then
    alias sinstall="sudo snap install"
    alias ssearch="snap search"
    alias supdate="sudo snap refresh"
    alias sremove="sudo snap remove"
    alias slist="snap list"
fi

# `updateall` is composed here rather than in the dispatch above because the
# supplementary managers are optional: the base command comes from the host's
# own package manager and snap/flatpak are appended only when installed. On an
# unrecognised package manager no base exists and the alias is simply not
# defined, which is honest about not knowing the command.
if [[ -n "${_dotf_updateall:-}" ]]; then
    if has_command snap; then
        _dotf_updateall="$_dotf_updateall && sudo snap refresh"
    fi
    if has_command flatpak; then
        _dotf_updateall="$_dotf_updateall && flatpak update"
    fi
    # shellcheck disable=SC2139 # expanding at definition time is intended here
    alias updateall="$_dotf_updateall"
fi
unset _dotf_updateall

# ================================================================
# SYSTEM SERVICES (SYSTEMD)
# ================================================================

# SystemD service management
alias services="systemctl list-units --type=service"
alias start="sudo systemctl start"
alias stop="sudo systemctl stop"
alias restart="sudo systemctl restart"
alias reload="sudo systemctl reload"
# Not `enable`/`disable`: both are shell builtins (bash and zsh) and aliases
# win over builtins. Same rule as the `history` -> `pkghistory` rename above.
alias sysenable="sudo systemctl enable"
alias sysdisable="sudo systemctl disable"
alias status="systemctl status"
alias mask="sudo systemctl mask"
alias unmask="sudo systemctl unmask"

# System control
alias reboot="sudo systemctl reboot"
alias poweroff="sudo systemctl poweroff"
# Not `suspend`: that is a shell builtin and aliases win over builtins, so a
# nested shell where the user types `suspend` would suspend the machine.
alias syssuspend="sudo systemctl suspend"
alias hibernate="sudo systemctl hibernate"

# SystemD shortcuts
alias sc="systemctl"
alias scs="systemctl status"
alias scr="sudo systemctl restart"
alias sce="sudo systemctl enable"
alias scd="sudo systemctl disable"

# Journal logs
alias logs="sudo journalctl -f"
alias syslog="sudo journalctl -u"
alias bootlog="sudo journalctl -b"
alias kerror="sudo journalctl -p err"

# ================================================================
# PROCESS AND SYSTEM MONITORING
# ================================================================

# Process management
alias psg="ps aux | grep -v grep | grep -i"
alias pstree="pstree -p"

# System information (sysinfo function is defined in common/functions.sh)
alias cpuinfo="lscpu"
alias meminfo="free -h"
alias diskinfo="df -h"
alias uptime="uptime -p"

# Hardware information
alias lshw="sudo lshw -short"
alias lsblk="lsblk -f"

# ================================================================
# NETWORK MANAGEMENT
# ================================================================

# Network interfaces
alias interfaces="ip link show"
alias ipaddr="ip addr show"
alias iproute="ip route show"
# No `netstat` alias: net-tools' netstat takes different options (`netstat -rn`
# would silently become an `ss` socket table), and `ports` below is the same
# command under a name that shadows nothing.
alias ports="ss -tuln"
alias listening="ss -tuln | grep LISTEN"

# NetworkManager (if available)
if has_command nmcli; then
    alias wifi="nmcli dev wifi"
    alias wifilist="nmcli dev wifi list"
    alias wificonnect="nmcli dev wifi connect"
    alias wifidisconnect="nmcli dev disconnect"
    alias connections="nmcli connection show"
fi

# Wireless tools
if has_command iwconfig; then
    alias wireless="iwconfig"
    alias wifiscan="sudo iwlist scan"
fi

# ================================================================
# FILE SYSTEM AND STORAGE
# ================================================================

# Disk usage and management
alias df="df -h"
alias du="du -h"
alias du1="du -h --max-depth=1"
alias dus="du -sh"
alias free="free -h"

# Mount operations
# No `mount` alias: piping it swallows every argument, so `mount /dev/sdb1 /mnt`
# would run `column -t /dev/sdb1 /mnt`. Use `mounts` for the pretty listing.
alias mounts="findmnt -D"
alias umount="sudo umount"

# File permissions
alias chown="sudo chown"
alias chgrp="sudo chgrp"

# ================================================================
# DESKTOP ENVIRONMENT
# ================================================================

# X11/Wayland clipboard
if has_command xclip; then
    alias clipboard="xclip -selection clipboard"
    alias copy="xclip -selection clipboard"
    alias paste="xclip -selection clipboard -o"
elif has_command xsel; then
    alias clipboard="xsel --clipboard"
    alias copy="xsel --clipboard --input"
    alias paste="xsel --clipboard --output"
elif has_command wl-copy && has_command wl-paste; then
    alias copy="wl-copy"
    alias paste="wl-paste"
fi

# Display management
alias xdisplay="xrandr"
alias displays="xrandr --listmonitors"

# Desktop environment specific
if [[ "$XDG_CURRENT_DESKTOP" == *"GNOME"* ]]; then
    alias settings="gnome-control-center"
    alias extensions="gnome-extensions-app"
elif [[ "$XDG_CURRENT_DESKTOP" == *"KDE"* ]]; then
    # Plasma 6 renamed the binary back to `systemsettings`; Plasma 5 shipped
    # `systemsettings5`. Probe rather than guess.
    if has_command systemsettings; then
        alias settings="systemsettings"
    elif has_command systemsettings5; then
        alias settings="systemsettings5"
    fi
elif [[ "$XDG_CURRENT_DESKTOP" == *"XFCE"* ]]; then
    alias settings="xfce4-settings-manager"
fi

# ================================================================
# FIREWALL MANAGEMENT
# ================================================================

# UFW (Uncomplicated Firewall) on Debian/Ubuntu, firewalld on Fedora/RHEL and
# openSUSE. internal/sysconfig/linux.go routes the RedHat family to firewalld,
# so the shell layer offers the same alias names on both backends.
if has_command ufw; then
    alias firewall="sudo ufw status"
    alias fwenable="sudo ufw enable"
    alias fwdisable="sudo ufw disable"
    alias fwallow="sudo ufw allow"
    alias fwdeny="sudo ufw deny"
    alias fwdelete="sudo ufw delete"
    alias fwreset="sudo ufw --force reset"
elif has_command firewall-cmd; then
    # firewalld has no deny rules and no reset: removing a permanent service or
    # port is the deny, and reloading discards unsaved runtime changes.
    alias firewall="sudo firewall-cmd --state && sudo firewall-cmd --list-all"
    alias fwenable="sudo systemctl enable --now firewalld"
    alias fwdisable="sudo systemctl disable --now firewalld"
    alias fwallow="sudo firewall-cmd --permanent --add-service"
    alias fwdeny="sudo firewall-cmd --permanent --remove-service"
    alias fwdelete="sudo firewall-cmd --permanent --remove-port"
    alias fwreset="sudo firewall-cmd --reload"
fi

# iptables
if has_command iptables; then
    alias iptlist="sudo iptables -L -n -v"
    alias iptblock="sudo iptables -A INPUT -s"
    alias iptunblock="sudo iptables -D INPUT -s"
fi

# ================================================================
# CONTAINER MANAGEMENT
# ================================================================

# Podman (alternative to Docker on some Linux systems)
if has_command podman; then
    alias pd="podman"
    alias pps="podman ps"
    alias pimg="podman images"
    alias prun="podman run"
    alias pexec="podman exec"
    alias plogs="podman logs"
fi

# ================================================================
# DEVELOPMENT TOOLS
# ================================================================

# GCC/Build tools
# Not `gcc`/`g++`: injected warning flags cannot be overridden by the user's own
# invocation and flip the exit status of `gcc -Werror ...`. Opt in by name.
alias gccw="gcc -Wall -Wextra"
alias gxxw="g++ -Wall -Wextra"
# `make` is kept deliberately: unlike the compiler flags above, make takes the
# last `-j`, so a user's own `-j1` still wins. Accepted risk: interactive builds
# are parallel by default.
alias make='make -j"$(nproc)"'

# Debugging tools
if has_command gdb; then
    alias debug="gdb"
fi

if has_command valgrind; then
    alias memcheck="valgrind --tool=memcheck --leak-check=full"
fi

# ================================================================
# ARCHIVE AND COMPRESSION
# ================================================================

# Archive creation shortcuts
alias targz="tar -czf"
alias tarbz2="tar -cjf"
alias tarxz="tar -cJf"

# Archive extraction shortcuts
alias untargz="tar -xzf"
alias untarbz2="tar -xjf"
alias untarxz="tar -xJf"

# ================================================================
# SECURITY TOOLS
# ================================================================

# SSL/TLS
if has_command openssl; then
    alias sslcert="openssl x509 -text -noout -in"
    alias sslkey="openssl rsa -text -noout -in"
    alias sslcheck="openssl s_client -connect"
fi

# GPG
if has_command gpg; then
    alias gpglist="gpg --list-keys"
    alias gpgsecret="gpg --list-secret-keys"
    alias gpgencrypt="gpg --encrypt --armor"
    alias gpgdecrypt="gpg --decrypt"
fi

# ================================================================
# PERFORMANCE MONITORING
# ================================================================

# System monitoring tools
if has_command htop; then
    alias top="htop"
    alias cpu="htop"
fi

if has_command iotop; then
    alias iotop="sudo iotop"
fi

if has_command nethogs; then
    alias bandwidth="sudo nethogs"
fi

if has_command ncdu; then
    alias diskusage="ncdu"
fi

# ================================================================
# CRON AND SCHEDULING
# ================================================================

# Crontab management
alias cronjobs="crontab -l"
alias cronjobsedit="crontab -e"

# ================================================================
# KERNEL AND MODULES
# ================================================================

# Kernel information
alias kernel="uname -a"
alias kernelversion="uname -r"
alias modules="lsmod"
alias loadmodule="sudo modprobe"
alias unloadmodule="sudo modprobe -r"

# ================================================================
# USER AND GROUP MANAGEMENT
# ================================================================

# User information
alias lastlogin="last"
alias currentusers="who"

# ================================================================
# ENVIRONMENT SPECIFIC
# ================================================================

# WSL specific
if is_wsl; then
    alias explorer="explorer.exe"
    alias notepad="notepad.exe"
    alias cmd="cmd.exe"
    alias powershell="powershell.exe"
    alias winget="winget.exe"
    
    # Path conversion
    alias winpath="wslpath -w"
    alias linuxpath="wslpath -u"
fi

# ================================================================
# MAINTENANCE AND CLEANUP
# ================================================================

# System cleanup
# `cleanup-packages` and `updateall` are package-manager specific and are
# defined in the PACKAGE MANAGEMENT dispatch at the top of this file.
alias cleanup-logs="sudo journalctl --vacuum-time=30d"
alias cleanup-temp="sudo rm -rf /tmp/* /var/tmp/*"
