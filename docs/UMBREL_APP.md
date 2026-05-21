# Umbrel app packaging

The app scaffold lives in `umbrel-app/` and is now wired to start the daemon, not just print CLI status.

## What the app does today

- Runs `umbrel-dropbox-clientd` in **dry-run mode only**.
- Defaults to the scoped canary root `/dropbox/Obsidian` mapped from `/home/umbrel/Dropbox`.
- Serves the dashboard and JSON status on port `8477`.
- Stores app state in `/data/state.db` and generated daemon config in `/data/config.json`.
- Uses `/data/token.json` when present. If the token is missing, it starts a local-only dry-run dashboard with remote delta disabled.
- Defaults to `UDC_DRY_RUN=true`. Scoped live mode requires both `UDC_DRY_RUN=false` and `UDC_ALLOW_LIVE=true`, and the entrypoint refuses live mode outside `/Obsidian`.

## Files

- `umbrel-app/umbrel-app.yml` — Umbrel manifest.
- `umbrel-app/docker-compose.yml` — app service, volume mounts, and safe defaults.
- `packaging/docker/umbrel-entrypoint.sh` — generates config, initializes state, and starts the daemon.
- `Dockerfile` — builds CLI + daemon and uses the Umbrel entrypoint.

## Local smoke test

```bash
docker build -t umbrel-dropbox-client:app-mvp .
DATA=$(mktemp -d)
ROOT=$(mktemp -d)
echo app-smoke > "$ROOT/note.txt"
docker run --rm -p 18477:8477   -v "$DATA:/data"   -v "$ROOT:/dropbox/Obsidian"   umbrel-dropbox-client:app-mvp
```

Then open:

- Dashboard: <http://127.0.0.1:18477/>
- Status JSON: <http://127.0.0.1:18477/status>

## Token setup for remote-delta canary

Copy a saved Dropbox token file into the app data directory as `token.json`:

```bash
cp ~/.local/state/umbrel-dropbox-client/token.json "$APP_DATA_DIR/data/token.json"
chmod 600 "$APP_DATA_DIR/data/token.json"
```

With the token present, the entrypoint leaves `UDC_REMOTE_DELTA=auto` enabled and the daemon performs scoped remote-delta dry-run cycles against `/Obsidian`.

## Environment knobs

The compose defaults are conservative and can be overridden:

- `UDC_ROOT=/dropbox/Obsidian`
- `UDC_REMOTE_PATH=/Obsidian`
- `UDC_TOKEN_FILE=/data/token.json`
- `UDC_REMOTE_DELTA=auto` (`true`, `false`, or `auto`)
- `UDC_DRY_RUN=true` by default. For the guarded `/Obsidian` pilot only, set `UDC_DRY_RUN=false`, `UDC_ALLOW_LIVE=true`, and leave `UDC_LIVE_SCOPE=/Obsidian`.
- `UDC_SCAN_INTERVAL_SECONDS=300`
- `UDC_HEALTH_ADDR=0.0.0.0:8477`

## Before app-store submission

1. Confirm GHCR image publishing from the Docker workflow.
2. Add screenshot gallery.
3. Decide whether auth setup stays CLI/file-based for MVP or gets a web flow.
4. Validate manifest against Umbrel's current app-store schema.
5. Submit PR to Umbrel community app store or official store path.
