#!/bin/bash

# Ensure input path is provided
if [[ -z "$1" ]]; then
	echo "Usage: $0 <input_directory>"
	exit 1
fi

# Get the input directory from the first CLI argument
INPUT_DIR="$1"

# Define the output directory (change as needed)
OUTPUT_ROOT="/mnt/data/media/music-apple"

# Ensure the output directory exists
mkdir -pv "$OUTPUT_ROOT"

# Find all FLAC files and process them
fd -e flac -0 "$INPUT_DIR" | sort -z | while IFS= read -r -d '' file; do
	if [[ -f "$file" ]]; then
		# Get the relative path of the FLAC file (strip leading directories)
		relative_path="${file#./}"

		# Change file extension from .flac to .m4a
		output_path="${relative_path%.flac}.m4a"

		# Construct the full output path in the new location
		full_output_path="$OUTPUT_ROOT/$output_path"

		# Create necessary directories in the output structure
		mkdir -p "$(dirname "$full_output_path")"

		# Convert the file to ALAC format
		ffmpeg -nostdin -i "$file" -map 0:a -c:a alac -map_metadata 0 "$full_output_path"

		echo "Converted: $file -> $full_output_path"
	else
		echo "Warning: File not found - $file"
	fi
done
