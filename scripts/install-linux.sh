#!/usr/bin/env bash
set -euo pipefail
repo="earlvanze/umbrel-dropbox-client"
arch="$(uname -m)"
case "$arch" in x86_64) arch=amd64;; aarch64|arm64) arch=arm64;; *) echo "Unsupported arch: $arch" >&2; exit 1;; esac
url="https://github.com/$repo/releases/latest/download/umbrel-dropbox-client_Linux_${arch}.tar.gz"
tmp="$(mktemp -d)"
curl -fsSL "$url" -o "$tmp/pkg.tgz"
tar -xzf "$tmp/pkg.tgz" -C "$tmp"
sudo install -m 0755 "$tmp/umbrel-dropbox-client" /usr/local/bin/umbrel-dropbox-client
sudo install -m 0755 "$tmp/umbrel-dropbox-clientd" /usr/local/bin/umbrel-dropbox-clientd
mkdir -p ~/.config/umbrel-dropbox-client
umbrel-dropbox-client init --root "$HOME/Dropbox" --config "$HOME/.config/umbrel-dropbox-client/config.json"
echo "Installed. Configure OAuth token/device auth before enabling daemon."
