#!/bin/sh
set -eu

CONFIG="${UDC_CONFIG:-${UDS_CONFIG:-/data/config.json}}"
ROOT="${UDC_ROOT:-/dropbox}"
DB="${UDC_DB:-/data/state.db}"
REMOTE_PATH="${UDC_REMOTE_PATH:-/}"
TOKEN_FILE="${UDC_TOKEN_FILE:-/data/token.json}"
REMOTE_DELTA="${UDC_REMOTE_DELTA:-auto}"
WATCH="${UDC_WATCH:-true}"
WATCH_DEBOUNCE_MS="${UDC_WATCH_DEBOUNCE_MS:-1500}"
SCAN_INTERVAL_SECONDS="${UDC_SCAN_INTERVAL_SECONDS:-300}"
DRY_RUN="${UDC_DRY_RUN:-true}"
HEALTH_ADDR="${UDC_HEALTH_ADDR:-0.0.0.0:8477}"
ALLOW_LIVE="${UDC_ALLOW_LIVE:-false}"
LIVE_SCOPE="${UDC_LIVE_SCOPE:-/}"

if [ "$DRY_RUN" != "true" ]; then
  if [ "$DRY_RUN" != "false" ] || [ "$ALLOW_LIVE" != "true" ]; then
    echo "refusing to start Umbrel app with UDC_DRY_RUN=$DRY_RUN; live mode requires UDC_ALLOW_LIVE=true" >&2
    exit 1
  fi
  if [ "$REMOTE_PATH" != "$LIVE_SCOPE" ] || [ "$ROOT" != "/dropbox${LIVE_SCOPE}" ]; then
    echo "refusing live mode outside approved scope: root=$ROOT remote_path=$REMOTE_PATH live_scope=$LIVE_SCOPE" >&2
    exit 1
  fi
  if [ ! -s "$TOKEN_FILE" ]; then
    echo "refusing live mode without token file at $TOKEN_FILE" >&2
    exit 1
  fi
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
  "dry_run": $DRY_RUN,
  "allow_live": $ALLOW_LIVE,
  "health_addr": "$HEALTH_ADDR"
}
JSON

echo "starting umbrel-dropbox-clientd root=$ROOT remote_path=$REMOTE_PATH remote_delta=$REMOTE_DELTA dry_run=$DRY_RUN allow_live=$ALLOW_LIVE health=$HEALTH_ADDR"
exec umbrel-dropbox-clientd --config "$CONFIG"
