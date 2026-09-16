#!/bin/bash

# Define the desired asset name and installation directory
if [ "$(uname)" == "Darwin" ] && uname -m | grep -q -i "arm64"; then
	ASSET_NAME_CONTAINS="bws-aarch64-apple-darwin"
	echo "ARM64"
elif [ "$(uname)" == "Darwin" ] && uname -m | grep -q -i "x86"; then
	ASSET_NAME_CONTAINS="bws-x86_64-apple-darwin"
elif [ "$(uname)" == "Linux" ] && uname -m | grep -q -i "arm64"; then
	ASSET_NAME_CONTAINS="bws-aarch64-unknown-linux-gnu"
elif [ "$(uname)" == "Linux" ] && uname -m | grep -q -i "x86"; then
	ASSET_NAME_CONTAINS="bws-x86_64-unknown-linux-gnu"
else
	echo "Unsupported platform"
	exit 1
fi

echo "------------------------------------------------------------------------------"
echo "Asset match to download: $ASSET_NAME_CONTAINS"
echo "------------------------------------------------------------------------------"

REPO=bitwarden/sdk
INSTALL_DIR="/usr/local/bin"

# Extract the download URL for the asset named "bitwarden_sdk-1.0.0"
# ASSET_URL=$(jq -r --arg name "$ASSET_NAME" '.[] | .assets[] | select(.name | contains($name)) | .browser_download_url' "https://api.github.com/repos/$REPO/releases")
ASSET_URL=$(curl -s "https://api.github.com/repos/$REPO/releases" |
	jq -r --arg name "$ASSET_NAME_CONTAINS" '.[] | .assets[] | select(.name | contains($name)) | .browser_download_url' |
	head -n 1)

ASSET_VERSION=$(curl -s "https://api.github.com/repos/$REPO/releases" |
	jq -r --arg name "$ASSET_NAME_CONTAINS" '.[] | .assets[] | select(.name | contains($name)) | .name' |
	command grep -o '[0-9]\+\.[0-9]\+\.[0-9]\+' |
	head -n 1)

# normalize the asset name
ASSET_NAME=$(basename "$ASSET_URL")

# Check if the URL was found
if [ -z "$ASSET_URL" ]; then
	echo "Failed to find the asset with the name $ASSET_NAME."
	exit 1
fi

# Download the asset
echo "------------------------------------------------------------------------------"
echo "Downloading Bitwarden CLI version: $ASSET_VERSION from:"
echo "$ASSET_URL"
echo "------------------------------------------------------------------------------"
curl -L -o "$ASSET_NAME" "$ASSET_URL"
unzip "$ASSET_NAME"
echo "------------------------------------------------------------------------------"
echo
# # Move the downloaded file to the installation directory and make it executable
echo "------------------------------------------------------------------------------"
echo "Installing $ASSET_NAME to $INSTALL_DIR"
echo "------------------------------------------------------------------------------"
sudo mv -v -f "bws" "$INSTALL_DIR/bws"
sudo chmod +x "$INSTALL_DIR/bws"
echo
echo "------------------------------------------------------------------------------"
echo "$ASSET_NAME has been installed to $INSTALL_DIR and made executable."
echo "------------------------------------------------------------------------------"
echo
echo "------------------------------------------------------------------------------"
echo "Deleting the downloaded file"
rm -rfv "$ASSET_NAME"
echo "------------------------------------------------------------------------------"
