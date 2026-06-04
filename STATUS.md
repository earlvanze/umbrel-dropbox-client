# Umbrel Dropbox Client Status

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
- Browser-based OAuth2 PKCE auth added for public Docker/Umbrel installs, including authorize URL generation, localhost callback handling, token exchange, secure token save, and tests.
- Dropbox content upload/download client methods added with mock tests for API args, auth, upload body handling, and atomic download writes.
- Guarded live transfer worker handler added with explicit `AllowLive` gate, sync-root containment checks, upload hash revalidation, no-overwrite downloads, and state entry updates.
- Worker CLI now supports explicit guarded live mode via `--live --i-understand-risk`, token-file/env access token loading, and configured/overridden sync root.
- Daemon cycle now performs dry-run local scans, upserts entries, processes dry-run queue work, records audit events, and refuses daemon live mode until separately enabled.
- Pause/resume state and CLI commands added; daemon skips cycles while paused.
- Daemon health/status HTTP handler added for `/healthz` and `/status` JSON responses when `health_addr` is configured.
- Basic daemon web dashboard added at `/` / `/ui`, with status cards, recent conflicts, auth hint, plus `/conflicts` JSON for Umbrel UI scaffolding.
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
- Watcher events wired into daemon scheduling with debounce; watch-triggered cycles are tested end-to-end.
- `smoke-test` CLI added for throwaway dry-run and explicitly gated live Dropbox upload validation against a provided remote path.
- Explicit reviewed delete execution gates added: live worker delete handling now requires `--live --i-understand-risk --execute-reviewed-deletes`, a pending `review_*_delete` op, matching current `local_missing` entry state, and unchanged rev/id/hash before any remote delete or state prune path can complete.
- Production task brief committed in `PRODUCTION_TASK.md`.

Safety:
- Current official Dropbox daemon was not touched.
- Sync remains dry-run/scaffolded until live auth and reconciliation are reviewed.
- Reviewed delete execution remains opt-in and disabled by default; ordinary `delete_local` / `delete_remote` pending ops remain unsupported.

Next:
1. Validate OAuth device-code flow against a non-production Dropbox test folder.
2. Run `smoke-test --live --i-understand-risk` against a non-production Dropbox test folder after OAuth is validated.

Validation:
- Files changed for reviewed delete gates: `cmd/umbrel-dropbox-client/main.go`, `internal/dropbox/client.go`, `internal/dropbox/client_test.go`, `internal/state/entries.go`, `internal/worker/delete_review.go`, `internal/worker/delete_review_test.go`, `STATUS.md`.
- `gofmt` completed locally after worker handoff.
- `go test ./...` passed locally after worker handoff.
- Commit requested but not created in this worker environment: `.git` is mounted read-only and `git add` failed creating `.git/index.lock`.

## Release Blocker: CPU Usage — Resolved

- On 2026-05-23, `umbrel-dropbox-master` was stopped after `umbrel-dropbox-clientd` held ~237% CPU on a ~168k-file Dropbox tree.
- Root cause: watch/timer cycles performed full-root scans and unchanged SQLite upserts on every cycle.
- See `docs/CPU_RELEASE_BLOCKER.md` for original findings and required optimization gates.

Implemented CPU optimizations (2026-06-01, v1.0.x):

- **Event-scoped reconciliation**: Watch events now collect dirty paths into a `DirtySet` and trigger incremental directory-scoped cycles instead of full-tree walks.
- **Skip unchanged upserts**: `UpsertEntryIfChanged` queries existing rows before writing; unchanged entries (matching content_hash, size, mtime, state) are skipped entirely, eliminating ~160k unnecessary writes per cycle.
- **Incremental `WalkDirs`**: `scan.WalkDirs` scans only dirty directories; when `start == root` it scans only root-level files (no recursion) so a root-only change does not walk the whole subtree.
- **Scoped missing detection**: `MarkMissingLocalInDirs` only queries entries under changed directory prefixes; the empty-prefix case now matches root-level files only, not the whole entries table.
- **Full-scan interval**: `full_scan_interval_seconds` config (default 3600s) ensures full-tree scans only happen hourly; periodic cycles between full scans are skipped if no dirty paths exist.
- **Extended ignore dirs**: `DefaultIgnoreDirs` plus configurable `ignore_dirs` in daemon config.
- **Dirty path tracking**: `watch.DirtySet` collects filesystem event paths, deduplicates into parent directories, and filters ignored directory names.

v1.1.0 / v1.2.0 (2026-06-01):

- **Selective sync**: `sync_paths` and `exclude_paths` config keys; `scan.Options.ShouldScan` is honored during both full and incremental scans.
- **Interactive setup wizard**: `/setup` page with device-code OAuth, folder picker, and config persistence.
- **Rich dashboard**: tab-based file manager, settings panel, conflicts tab with one-click resolution.

v1.2.1 (2026-06-03, this commit):

- **`SplitParentDirs` fix**: previously a single-segment subdir (e.g. `umbrel-dropbox-client-smoke-test`) was being collapsed to `""`, which caused a root-level full walk; now a slash-less input is treated as a directory itself.
- **`WalkDirs` rootOnly mode**: when `start == root` the walk stops at depth 1, so a root-only change only enumerates root-level files.
- **`LocalEntriesInDirs` / `MarkMissingLocalInDirs` empty-prefix fix**: the empty string is now scoped to root-level paths (path = '' OR path LIKE '/%' AND path NOT LIKE '/%/%') instead of a 1=1 match-all.
- **Panic recovery in `RunCycleIncremental`**: panics are logged and returned as errors instead of silently killing the daemon.
- **Watch channel drain after initial scan**: prevents the initial 4-minute full scan from queueing thousands of inotify events that would otherwise cause a feedback loop.

Validation (2026-06-03, 158k file `/home/umbrel/Dropbox`):

- Initial full scan: ~3:26, idle CPU 0% afterwards.
- Subdir touch: incremental cycle, 12-13 files scanned, 1 changed, 0 missing.
- Root-level touch: incremental cycle, 15-16 root-level files scanned, 1 changed, 0 missing.
- Deep touch (e.g. `Projects/umbrel-dropbox-client/`): incremental cycle, 120,576 files scanned (the dirty subtree), 1 changed, 0 missing.
- No `local_missing` tombstones are produced on the touched subdirs or root.

Acceptance gates (1-5) are met. Promotion of the Umbrel Dropbox Client is no longer blocked by the CPU issue.

Followups still tracked (not part of this resolution):

- Multi-arch Umbrel App Store image build and submission to the Umbrel community app store.


v1.2.1 release (2026-06-04):

- Multi-arch Docker image built and pushed to `ghcr.io/earlvanze/umbrel-dropbox-client:v1.2.1` and `:latest`
  (linux/amd64 + linux/arm64). Verified locally with a `/dropbox/Obsidian` canary container: scoped dry-run
  mode active by default, dashboard redirects to `/setup`, `/api/files`, `/status`, `/healthz` all respond
  JSON 200, and the setup wizard page renders.
- `umbrel-app/umbrel-app.yml` bumped to `1.2.1` with rewritten release notes describing the CPU fixes.
- `umbrel-app/docker-compose.yml` now points at `:v1.2.1`, scopes `UDC_ROOT` to `/dropbox/Obsidian`
  (instead of the entire Dropbox root), and adds `UDC_REMOTE_PATH=/Obsidian` and `UDC_LIVE_SCOPE=/Obsidian`
  so the entrypoint's live-mode guard is satisfied out of the box.
- The Personal Umbrel App Store repo `earlvanze/umbrel-personal-apps` was updated to mirror the v1.2.1
  manifest and compose (`commit f7f9cf8`).
- The host's live Umbrel app container (`umbrel-dropbox-client_server_1`) is running on the new image,
  172 entries in `/dropbox/Obsidian`, both full and incremental cycles observed in the events log.

Validation (2026-06-04, fresh 158k-file soak against `/home/umbrel/Dropbox`):

- Initial full scan: ~12 min for 158,357 files (cold cache; previously ~3:30 once warm).
- Three controlled touch tests (subdir, root, deep): all three new paths appear in the entries table
  after a single incremental cycle. No `local_missing` tombstones for our test files.
- The 1:35 incremental cycle scanned only 120,579 files (the dirty `Projects/umbrel-dropbox-client/`
  subtree), not the full root — confirming the watch-debounce path is now scope-bounded.
- Mode is correctly reported as `incremental` (never silently falling back to `full` after a single touch).

Followups still tracked (not part of this release):

- Submit a PR to the upstream `getumbrel/umbrel-apps` community app store per Umbrel's submission
  workflow. Personal store is already serving the v1.2.1 build.
- Optional: pin the compose image to a per-arch sha for stricter supply-chain reproducibility.


Umbrel App Store submission (2026-06-04):

- **PR #5717** opened against `getumbrel/umbrel-apps`: https://github.com/getumbrel/umbrel-apps/pull/5717
  - Submits `umbrel-dropbox-client` v1.2.1 for the official Umbrel App Store.
  - Manifest `1` (matches upstream convention), `category: files`, `port: 8477`, no dependencies.
  - Docker image: `ghcr.io/earlvanze/umbrel-dropbox-client:v1.2.1` (multi-arch: linux/amd64 + linux/arm64).
  - Local verification on this Umbrel install: `umbrel-dropbox-client_server_1` running the v1.2.1
    image, dashboard responding on `:8477`, dry-run by default with `UDC_LIVE_SCOPE=/Obsidian`.

## ACFS integration

Status: ACFS integration milestone complete.

- Local ACFS reference clone: `/home/umbrel/.openclaw/workspace/external/agentic_coding_flywheel_setup`
- Added `.github/workflows/notify-acfs.yml` for installer checksum dispatch.
- Added `.github/workflows/validate-acfs.yml`.
- Added `docs/ACFS.md`.
- Full ACFS bootstrap installer was not run on Umbrel because it targets fresh Ubuntu VPS systems and can mutate shell/sudo/runtime state.

Pending:
1. Add `ACFS_REPO_DISPATCH_TOKEN` repo secret if dispatch to ACFS is desired.
2. Add `umbrel_dropbox_client` to upstream ACFS `checksums.yaml` if we want first-class ACFS tracking.
