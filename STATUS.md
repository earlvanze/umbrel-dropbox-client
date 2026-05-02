# Umbrel Dropbox Sync Status

Status: production foundation milestone 3 in progress.

Implemented:
- Durable pending operation helpers in SQLite state store.
- Entry upsert support for scanner/sync engine.
- Local filesystem scanner with Dropbox content hashing and ignored state dirs.
- Conservative conflict policy engine with tests.
- `sync --once --dry-run` scans local root and upserts state entries.
- Dropbox `list_folder` / `list_folder/continue` client interfaces added.
- Dropbox paginated `ListFolderAll` helper added with mock API tests.
- `sync --once --dry-run --remote` now fetches remote Dropbox metadata and records remote-scanned entries/cursor without mutating local or remote files.
- Dry-run reconciliation now builds explicit upload/download/conflict plans from local scan plus remote metadata and records them as deduplicated pending ops/conflicts.
- Durable worker queue processor added under `internal/worker` with injectable local handlers, retry/backoff scheduling, Retry-After support, success completion, and terminal failure state.
- Dry-run worker CLI added to validate queued upload/download plans and complete safe local-only operations without touching Dropbox or local files.
- Secure local token file storage added with private `0600` permissions, redacted auth status, env-token import, and Dropbox OAuth device-code CLI scaffold.
- Dropbox content upload/download client methods added with mock tests for API args, auth, upload body handling, and atomic download writes.
- Production task brief committed in `PRODUCTION_TASK.md`.

Safety:
- Current official Dropbox daemon was not touched.
- Sync remains dry-run/scaffolded until live auth and reconciliation are reviewed.

Next:
1. Validate OAuth device-code flow against a non-production Dropbox test folder.
2. Wire guarded real upload/download transfer handlers into worker queue after non-production auth validation.
3. Add a non-production Dropbox fixture/integration pass for dry-run plan counts.

Validation:
- `gofmt` completed.
- `go test ./...` passed.

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
