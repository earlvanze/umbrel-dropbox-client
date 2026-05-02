#!/usr/bin/env bash
set -euo pipefail
repo="earlvanze/umbrel-dropbox-sync"
arch="$(uname -m)"
case "$arch" in x86_64) arch=amd64;; aarch64|arm64) arch=arm64;; *) echo "Unsupported arch: $arch" >&2; exit 1;; esac
url="https://github.com/$repo/releases/latest/download/umbrel-dropbox-sync_Linux_${arch}.tar.gz"
tmp="$(mktemp -d)"
curl -fsSL "$url" -o "$tmp/pkg.tgz"
tar -xzf "$tmp/pkg.tgz" -C "$tmp"
sudo install -m 0755 "$tmp/umbrel-dropbox-sync" /usr/local/bin/umbrel-dropbox-sync
sudo install -m 0755 "$tmp/umbrel-dropbox-syncd" /usr/local/bin/umbrel-dropbox-syncd
mkdir -p ~/.config/umbrel-dropbox-sync
umbrel-dropbox-sync init --root "$HOME/Dropbox" --config "$HOME/.config/umbrel-dropbox-sync/config.json"
echo "Installed. Configure OAuth token/device auth before enabling daemon."
