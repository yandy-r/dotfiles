#!/usr/bin/env bash
# Disk I/O speed for Waybar custom module
# Bar: root disk read/write speeds
# Tooltip: all block devices with speeds

CACHE_DIR="/tmp/.waybar-disk"
mkdir -p "$CACHE_DIR"

SECTOR=512

fmt() {
    local b=$1
    if (( b >= 1073741824 )); then
        awk "BEGIN{printf \"%5.1fG\", $b/1073741824}"
    elif (( b >= 1048576 )); then
        awk "BEGIN{printf \"%5.1fM\", $b/1048576}"
    elif (( b >= 1024 )); then
        awk "BEGIN{printf \"%5.1fK\", $b/1024}"
    else
        printf "%5dB" "$b"
    fi
}

bar_char() {
    local bps=$1
    if   (( bps >= 524288000 )); then echo '█'   # ≥500MB/s
    elif (( bps >= 104857600 )); then echo '▇'   # ≥100MB/s
    elif (( bps >= 52428800  )); then echo '▆'   # ≥50MB/s
    elif (( bps >= 10485760  )); then echo '▅'   # ≥10MB/s
    elif (( bps >= 1048576   )); then echo '▄'   # ≥1MB/s
    elif (( bps >= 102400    )); then echo '▃'   # ≥100KB/s
    elif (( bps >= 1024      )); then echo '▂'   # ≥1KB/s
    else                              echo '▁'   # idle
    fi
}

calc_speed() {
    local dev=$1 rd_sec=$2 wr_sec=$3
    local cache="$CACHE_DIR/$dev"
    local now_ns rd_bps wr_bps

    now_ns=$(date +%s%N)

    if [[ -f "$cache" ]]; then
        local prev_rd prev_wr prev_ns elapsed_ns
        read -r prev_rd prev_wr prev_ns < "$cache"
        elapsed_ns=$(( now_ns - prev_ns ))
        (( elapsed_ns <= 0 )) && elapsed_ns=1000000000
        rd_bps=$(( (rd_sec - prev_rd) * SECTOR * 1000000000 / elapsed_ns ))
        wr_bps=$(( (wr_sec - prev_wr) * SECTOR * 1000000000 / elapsed_ns ))
        (( rd_bps < 0 )) && rd_bps=0
        (( wr_bps < 0 )) && wr_bps=0
    else
        rd_bps=0
        wr_bps=0
    fi

    echo "$rd_sec $wr_sec $now_ns" > "$cache"
    echo "$rd_bps $wr_bps"
}

# Detect root disk (strip btrfs subvolume suffix like [/@])
ROOT_SRC=$(findmnt -n -o SOURCE / | sed 's/\[.*//')
ROOT_DEV=$(lsblk -ndo PKNAME "$ROOT_SRC" 2>/dev/null)
[[ -z "$ROOT_DEV" ]] && { printf '{"text": "󰋊  --", "tooltip": "No disk detected"}\n'; exit 0; }

# Collect real block devices (exclude loop, ram, dm, zram, partitions)
mapfile -t ALL_DEVS < <(awk '$3 !~ /^(loop|ram|dm-|zram)/ && $3 !~ /[0-9]p?[0-9]+$/ { print $3 }' /proc/diskstats | sort -u)

# Build tooltip with all disks
tooltip=""
root_rd_bps=0 root_wr_bps=0

for dev in "${ALL_DEVS[@]}"; do
    [[ ! -d "/sys/block/$dev" ]] && continue

    read -r rd_sec wr_sec <<< "$(awk -v d="$dev" '$3==d {print $6,$10}' /proc/diskstats)"
    [[ -z "$rd_sec" ]] && continue

    read -r rd_bps wr_bps <<< "$(calc_speed "$dev" "$rd_sec" "$wr_sec")"
    rd_h=$(fmt "$rd_bps")
    wr_h=$(fmt "$wr_bps")

    if [[ "$dev" == "$ROOT_DEV" ]]; then
        root_rd_bps=$rd_bps
        root_wr_bps=$wr_bps
        tooltip+="$dev (root)\\nRead:  ${rd_h}/s  Write:  ${wr_h}/s"
    else
        tooltip+="\\n\\n$dev\\nRead:  ${rd_h}/s  Write:  ${wr_h}/s"
    fi
done

[[ -z "$root_rd_bps" ]] && { root_rd_bps=0; root_wr_bps=0; }

rd_bar=$(bar_char "$root_rd_bps")
wr_bar=$(bar_char "$root_wr_bps")

if (( root_rd_bps >= 52428800 || root_wr_bps >= 52428800 )); then
    css_class="heavy"
elif (( root_rd_bps >= 1024 || root_wr_bps >= 1024 )); then
    css_class="active"
else
    css_class="idle"
fi

printf '{"text": "󰋊 %s%s", "tooltip": "%s", "class": "%s"}\n' \
    "$rd_bar" "$wr_bar" "$tooltip" "$css_class"
