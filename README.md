# umbrel-dropbox-sync

A Linux-first Dropbox sync client intended to be more deterministic than the official Dropbox daemon on Umbrel/Linux.

Status: design + MVP scaffold.

## Goals
- Reliable bidirectional sync for selected Dropbox roots.
- SQLite state database with Dropbox cursors and local file metadata.
- Parallel upload/download worker pools with rate-limit handling.
- Safe conflict policy, no silent overwrites.
- Dry-run and audit logs for bulk operations.
- Systemd-friendly daemon with health/status CLI.

## Non-goals for MVP
- Dropbox LAN sync.
- Dropbox Paper.
- Smart Sync placeholders.
- Full Dropbox Business/team admin semantics.
