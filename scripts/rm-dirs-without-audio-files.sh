#!/bin/bash

# Use fd to find directories and check their contents for audio files
fd -t d . | while read -r dir; do
	# Check if the directory contains any audio files
	if ! fd -t f -e mp3 -e m4a -e wav -e flac -e aac . "$dir" | grep -q .; then
		echo "Deleting empty or non-audio directory: $dir"
		rm -rf "$dir"
	fi
done
