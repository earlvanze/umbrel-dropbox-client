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
- Remote delta ingestion loop added with stored cursor resume, paginated continue handling, file metadata application, cursor persistence, audit events, and tests.
- Remote delta ingestion wired into daemon dry-run cycles and CLI dry-run sync via `--remote-delta`, with `--token-file` support and test coverage.
- End-to-end CLI fixture tests added for init + dry-run sync + status and pause/resume flows.
- Packaged install/service smoke coverage added: `init --config` writes daemon config, install script initializes config, service path/goreleaser packaging checked, and both binaries build in test.
- Conflict-management CLI added: `conflicts` lists conflict records and `resolve-conflict --id` marks one resolved with an audit event.
- Local tombstone safeguards added: daemon marks previously known local files as `local_missing` when absent from a scan, without deleting local or remote files; `missing-local` CLI lists them.
- Guarded delete review planning added: missing tombstones can enqueue non-destructive `review_*_delete` ops only, never `delete_local` or `delete_remote`.
- Inotify/fsnotify watcher foundation added with recursive directory registration, dynamic subdirectory watching, and ignored state-directory filtering tests.
- Production task brief committed in `PRODUCTION_TASK.md`.

Safety:
- Current official Dropbox daemon was not touched.
- Sync remains dry-run/scaffolded until live auth and reconciliation are reviewed.

Next:
1. Validate OAuth device-code flow against a non-production Dropbox test folder.
2. Wire watcher events into daemon cycle scheduling/debounce.
3. Add explicit reviewed delete execution gates after manual tombstone policy approval.

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
