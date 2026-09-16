#!/bin/bash

TARGET="$1"
PORT=80
LOW=1400
HIGH=1500

while ((LOW <= HIGH)); do
	MID=$(((LOW + HIGH) / 2))
	echo -n "Testing payload size $MID ... "
	if sudo hping3 -c 1 -D -S -p "$PORT" -d "$MID" "$TARGET" | grep -q "flags=SA"; then
		echo "Success"
		LOW=$((MID + 1))
	else
		echo "Fail"
		HIGH=$((MID - 1))
	fi
done

echo "Max payload without fragmentation: $HIGH bytes"
echo "Estimated MTU: $((HIGH + 40)) bytes"
