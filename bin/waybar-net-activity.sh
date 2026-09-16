#!/usr/bin/env bash
# Network activity for Waybar custom module
# Bar: compact Unicode block bars for download/upload speed
# Tooltip: interface details, WiFi info, and human-readable speeds

CACHE_DIR="/tmp/.waybar-net"
mkdir -p "$CACHE_DIR"

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
    if   (( bps >= 104857600 )); then echo '█'   # ≥100MB/s
    elif (( bps >= 52428800  )); then echo '▇'   # ≥50MB/s
    elif (( bps >= 10485760  )); then echo '▆'   # ≥10MB/s
    elif (( bps >= 1048576   )); then echo '▅'   # ≥1MB/s
    elif (( bps >= 102400    )); then echo '▄'   # ≥100KB/s
    elif (( bps >= 10240     )); then echo '▃'   # ≥10KB/s
    elif (( bps >= 1024      )); then echo '▂'   # ≥1KB/s
    else                              echo '▁'   # idle
    fi
}

# Detect active interface
IFACE=$(ip route get 1.1.1.1 2>/dev/null | awk '{for(i=1;i<=NF;i++) if($i=="dev") print $(i+1); exit}')

if [[ -z "$IFACE" ]]; then
    printf '{"text": "󰲛 ✕", "tooltip": "No connection", "class": "disconnected"}\n'
    exit 0
fi

# Detect wired vs wireless
if [[ -d "/sys/class/net/$IFACE/wireless" ]]; then
    ICON="󰖩"
else
    ICON="󰈀"
fi

# Read rx/tx bytes from /proc/net/dev
read -r rx_bytes tx_bytes <<< "$(awk -v iface="$IFACE:" '$1==iface {print $2, $10}' /proc/net/dev)"

if [[ -z "$rx_bytes" ]]; then
    printf '{"text": "%s ✕", "tooltip": "No stats for %s", "class": "disconnected"}\n' "$ICON" "$IFACE"
    exit 0
fi

# Calculate speeds using cached previous reading
now_ns=$(date +%s%N)
cache="$CACHE_DIR/$IFACE"

if [[ -f "$cache" ]]; then
    read -r prev_rx prev_tx prev_ns < "$cache"
    elapsed_ns=$(( now_ns - prev_ns ))
    (( elapsed_ns <= 0 )) && elapsed_ns=1000000000
    rx_bps=$(( (rx_bytes - prev_rx) * 1000000000 / elapsed_ns ))
    tx_bps=$(( (tx_bytes - prev_tx) * 1000000000 / elapsed_ns ))
    (( rx_bps < 0 )) && rx_bps=0
    (( tx_bps < 0 )) && tx_bps=0
else
    rx_bps=0
    tx_bps=0
fi

echo "$rx_bytes $tx_bytes $now_ns" > "$cache"

# Build bar text
rx_bar=$(bar_char "$rx_bps")
tx_bar=$(bar_char "$tx_bps")

# Determine CSS class
if (( rx_bps >= 10485760 || tx_bps >= 10485760 )); then
    css_class="heavy"
elif (( rx_bps >= 1024 || tx_bps >= 1024 )); then
    css_class="active"
else
    css_class="idle"
fi

# Build tooltip
rx_h=$(fmt "$rx_bps")
tx_h=$(fmt "$tx_bps")
tooltip="$IFACE"

# Get IP address
ip_addr=$(ip -4 addr show "$IFACE" 2>/dev/null | awk '/inet / {print $2; exit}')
[[ -n "$ip_addr" ]] && tooltip+="  $ip_addr"

# Get gateway
gw=$(ip route show default dev "$IFACE" 2>/dev/null | awk '{print $3; exit}')
[[ -n "$gw" ]] && tooltip+="\\nGateway  $gw"

# WiFi details
if [[ -d "/sys/class/net/$IFACE/wireless" ]]; then
    ssid=$(iw dev "$IFACE" info 2>/dev/null | awk '/ssid/ {print $2}')
    signal=$(iw dev "$IFACE" station dump 2>/dev/null | awk '/signal:/ {print $2; exit}')
    [[ -n "$ssid" ]] && tooltip="󰖩  $ssid\\n$tooltip"
    [[ -n "$signal" ]] && tooltip+="\\nSignal  ${signal} dBm"
fi

tooltip+="\\n\\n↓ ${rx_h}/s  ↑ ${tx_h}/s"

printf '{"text": "%s %s%s", "tooltip": "%s", "class": "%s"}\n' \
    "$ICON" "$rx_bar" "$tx_bar" "$tooltip" "$css_class"
