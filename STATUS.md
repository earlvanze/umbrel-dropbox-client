# Umbrel Dropbox Sync Status

Status: production foundation milestone 1 complete.

Implemented:
- Durable pending operation helpers in SQLite state store.
- Entry upsert support for scanner/sync engine.
- Local filesystem scanner with Dropbox content hashing and ignored state dirs.
- Conservative conflict policy engine with tests.
- Production task brief committed in `PRODUCTION_TASK.md`.

Safety:
- Current official Dropbox daemon was not touched.
- Sync remains dry-run/scaffolded until live auth and reconciliation are reviewed.

Next:
1. Wire `sync --once --dry-run` to scanner + state entry updates.
2. Add Dropbox list_folder/list_folder/continue interfaces with mock tests.
3. Add worker queue processor with retry/backoff semantics.
