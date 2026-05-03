# Roadmap: Umbrel Dropbox Client

## Phase 1, MVP daemon
- OAuth/device-code auth.
- Remote cursor ingestion.
- Local scanner and inotify watcher.
- Durable queue + parallel transfer workers.
- Conflict-safe reconciliation.
- `status`, `pause`, `resume`, `conflicts`, `doctor` CLI.

## Phase 2, Linux desktop integration
- Nautilus, Nemo, Caja, Dolphin context menus.
- Actions: sync now, copy Dropbox link, view status, resolve conflict.
- Status emblems via extension adapters where supported.
- Freedesktop `.desktop` entries and DBus helper.

## Phase 3, packaging
- `.deb` via GoReleaser/nfpm.
- APT repository metadata published from GitHub Pages.
- systemd user/service units.
- AppArmor-friendly paths and config.

## Phase 4, Umbrel App Store
- Docker image with web UI.
- Umbrel `umbrel-app.yml`, compose file, icon, screenshots.
- Health endpoint and logs.
- Path mapping to `/home/umbrel/Dropbox` or user-configured mount.

## Phase 5, hardening
- End-to-end sync torture tests.
- Dropbox API 429/backoff test harness.
- SQLite crash recovery tests.
- Large tree benchmark and resume tests.
