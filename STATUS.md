# Umbrel Dropbox Sync Status

Status: production foundation milestone 2 in progress.

Implemented:
- Durable pending operation helpers in SQLite state store.
- Entry upsert support for scanner/sync engine.
- Local filesystem scanner with Dropbox content hashing and ignored state dirs.
- Conservative conflict policy engine with tests.
- `sync --once --dry-run` scans local root and upserts state entries.
- Dropbox `list_folder` / `list_folder/continue` client interfaces added.
- Production task brief committed in `PRODUCTION_TASK.md`.

Safety:
- Current official Dropbox daemon was not touched.
- Sync remains dry-run/scaffolded until live auth and reconciliation are reviewed.

Next:
1. Wire remote Dropbox listing into dry-run reconciliation.
2. Add mock tests for Dropbox list_folder/list_folder/continue interfaces.
3. Add worker queue processor with retry/backoff semantics.

## ACFS integration

Status: ACFS integration milestone complete.

- Local ACFS reference clone: `/home/umbrel/.openclaw/workspace/external/agentic_coding_flywheel_setup`
- Added `.github/workflows/notify-acfs.yml` for installer checksum dispatch.
- Added `.github/workflows/validate-acfs.yml`.
- Added `docs/ACFS.md`.
- Full ACFS bootstrap installer was not run on Umbrel because it targets fresh Ubuntu VPS systems and can mutate shell/sudo/runtime state.

Pending:
1. Add `ACFS_REPO_DISPATCH_TOKEN` repo secret if dispatch to ACFS is desired.
2. Add `umbrel_dropbox_sync` to upstream ACFS `checksums.yaml` if we want first-class ACFS tracking.
