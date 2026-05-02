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
- Guarded live transfer worker handler added with explicit `AllowLive` gate, sync-root containment checks, upload hash revalidation, no-overwrite downloads, and state entry updates.
- Worker CLI now supports explicit guarded live mode via `--live --i-understand-risk`, token-file/env access token loading, and configured/overridden sync root.
- Daemon cycle now performs dry-run local scans, upserts entries, processes dry-run queue work, records audit events, and refuses daemon live mode until separately enabled.
- Pause/resume state and CLI commands added; daemon skips cycles while paused.
- Daemon health/status HTTP handler added for `/healthz` and `/status` JSON responses when `health_addr` is configured.
- `doctor` CLI added to validate DB initialization, sync root, token file hygiene, Dropbox DNS, and local state summary.
- Deterministic dry-run integration fixture added for local scan + remote metadata reconciliation + queue/conflict persistence counts.
- Remote cursor helpers added: Dropbox latest-cursor API, remote metadata application helper, and tests for file-only state ingestion.
- Production task brief committed in `PRODUCTION_TASK.md`.

Safety:
- Current official Dropbox daemon was not touched.
- Sync remains dry-run/scaffolded until live auth and reconciliation are reviewed.

Next:
1. Validate OAuth device-code flow against a non-production Dropbox test folder.
2. Add remote delta cursor incremental ingestion loop/tests using stored cursors.
3. Add end-to-end dry-run CLI fixture command tests.

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
