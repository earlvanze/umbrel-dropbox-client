#!/bin/sh
set -eu

CONFIG="${UDC_CONFIG:-${UDS_CONFIG:-/data/config.json}}"
ROOT="${UDC_ROOT:-/dropbox/Obsidian}"
DB="${UDC_DB:-/data/state.db}"
REMOTE_PATH="${UDC_REMOTE_PATH:-/Obsidian}"
TOKEN_FILE="${UDC_TOKEN_FILE:-/data/token.json}"
REMOTE_DELTA="${UDC_REMOTE_DELTA:-auto}"
WATCH="${UDC_WATCH:-true}"
WATCH_DEBOUNCE_MS="${UDC_WATCH_DEBOUNCE_MS:-1500}"
SCAN_INTERVAL_SECONDS="${UDC_SCAN_INTERVAL_SECONDS:-300}"
DRY_RUN="${UDC_DRY_RUN:-true}"
HEALTH_ADDR="${UDC_HEALTH_ADDR:-0.0.0.0:8477}"

if [ "$DRY_RUN" != "true" ]; then
  echo "refusing to start Umbrel app with UDC_DRY_RUN=$DRY_RUN; app MVP is dry-run only" >&2
  exit 1
fi

if [ "$REMOTE_DELTA" = "auto" ]; then
  if [ -s "$TOKEN_FILE" ]; then
    REMOTE_DELTA=true
  else
    REMOTE_DELTA=false
    echo "token file missing at $TOKEN_FILE; starting local-only dry-run dashboard" >&2
  fi
fi

mkdir -p "$(dirname "$CONFIG")" "$(dirname "$DB")" "$ROOT"
umbrel-dropbox-client init --root "$ROOT" --db "$DB" >/dev/null

cat > "$CONFIG" <<JSON
{
  "root": "$ROOT",
  "db_path": "$DB",
  "remote_path": "$REMOTE_PATH",
  "remote_delta": $REMOTE_DELTA,
  "token_file": "$TOKEN_FILE",
  "watch": $WATCH,
  "watch_debounce_ms": $WATCH_DEBOUNCE_MS,
  "upload_workers": 4,
  "download_workers": 4,
  "scan_interval_seconds": $SCAN_INTERVAL_SECONDS,
  "dry_run": true,
  "health_addr": "$HEALTH_ADDR"
}
JSON

echo "starting umbrel-dropbox-clientd root=$ROOT remote_path=$REMOTE_PATH remote_delta=$REMOTE_DELTA dry_run=true health=$HEALTH_ADDR"
exec umbrel-dropbox-clientd --config "$CONFIG"
