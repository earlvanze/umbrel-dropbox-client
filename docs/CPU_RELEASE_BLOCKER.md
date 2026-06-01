# CPU Release Blocker, Full Dropbox Root Scan

Date: 2026-05-23
Status: **release blocker, do not promote Umbrel Dropbox Client**

## Immediate containment

Stopped the live `umbrel-dropbox-master` container after observing sustained high CPU:

- Process: `/usr/bin/umbrel-dropbox-clientd --config /data/config.json`
- CPU observed: ~237% across ~11 hot worker/runtime threads
- Runtime before stop: ~18 hours
- Container: `umbrel-dropbox-master`, image `umbrel-dropbox-client:local`

The tor sidecar remains running, but the high-CPU client daemon is no longer active.

## Current pilot shape

The problematic config was the broad master pilot:

```json
{
  "root": "/dropbox",
  "remote_delta": false,
  "watch": true,
  "watch_debounce_ms": 1500,
  "scan_interval_seconds": 300,
  "dry_run": false,
  "allow_live": true
}
```

This maps `/dropbox` to `/home/umbrel/Dropbox`, which is currently a large, active tree:

- ~168k files
- ~19.6k directories
- state DB: ~105 MB
- stored entries: ~160k `local_scanned`, 16 `local_missing`

Logs show repeated full-tree cycles over the entire root, often every ~3 to 6 minutes:

```text
sync cycle complete root=/dropbox local_files=160154 ...
sync cycle complete root=/dropbox local_files=160157 ...
```

## Root cause hypothesis

The daemon still uses a whole-tree scan as the primary local change detector:

1. `fsnotify` watches every directory recursively.
2. Any local event starts a debounce timer.
3. When the timer fires, `RunCycle` runs a full `scan.Walk` over the entire configured root.
4. Each cycle upserts every scanned file into SQLite, even unchanged files.
5. For unknown or metadata-changed files, it computes Dropbox content hashes.

Recent hash reuse avoids rehashing files whose size and mtime match cached state, which is good, but it does not solve the big CPU/IO cost of walking ~168k files and writing/upserting ~160k rows repeatedly.

This is especially bad when the root is the active `~/Dropbox` tree, because normal project/editor/browser/OpenClaw activity under Dropbox can keep triggering watch cycles.

## Native Dropbox Linux client comparison

The official Linux package has two parts:

- `/home/umbrel/dropbox.py`: GPL Python front-end/wrapper, installer, CLI status/exclude/lansync controls.
- `/home/umbrel/.dropbox-dist/dropbox-lnx.x86_64-254.3.2403/dropboxd`: proprietary Python/native sync daemon bundle.

The wrapper is not the sync engine. The bundled daemon includes native extensions and internal modules around filesystem APIs, nucleus/sync engine, selective sync, ignore sync, smart sync, sync prioritization, and local status APIs. It also exposes user-facing controls such as:

- `dropbox status`
- `dropbox filestatus`
- `dropbox exclude add/remove/list`
- `dropbox lansync`

Relevant optimization lessons from the native client design:

1. **Incremental local index, not full-root rescans on every event.** The sync engine appears built around a local database plus filesystem event ingestion. Full scans may happen for recovery, but should not be the steady-state hot path.
2. **Selective sync and ignore sync are first-class.** Large folders can be excluded before they create local CPU churn.
3. **Status queries are metadata-driven.** CLI `filestatus` asks the daemon for known state rather than walking and hashing the tree itself.
4. **Native FS/path modules exist for hot paths.** The bundle includes native filesystem and path extensions, while our current MVP does most work in Go plus SQLite. Go is fine, but our algorithm must avoid repeated O(tree) work.

## Required optimizations before promotion

Promotion should stay blocked until these are implemented and validated:

1. **Event-scoped reconciliation.** Watch events should enqueue changed paths and their parent directories, then scan only those paths. Full `scan.Walk(root)` should be reserved for startup, scheduled audits, or explicit repair.
2. **Dirty-path debounce batching.** Debounce should coalesce many events into a bounded dirty set, not into a full-tree cycle.
3. **Skip unchanged upserts.** Add an `UpsertEntryIfChanged` or batch transaction that avoids rewriting identical rows.
4. **Directory snapshot / local index.** Keep directory membership in state so deletion/missing detection can be scoped to watched directories instead of `MarkMissingLocal` across the whole tree every cycle.
5. **CPU and scan budgets.** Add config caps such as max files per incremental cycle, max scan duration, min quiet period, and CPU-friendly backoff when the tree is hot.
6. **Large-root guardrails.** Refuse or warn loudly for `root=/dropbox` live mode unless explicit `allow_large_root=true` exists and CPU soak tests pass. Default Umbrel app scope should remain a small folder such as `/dropbox/Obsidian`.
7. **Selective/ignore sync parity.** Add user-visible excludes before broad sync. At minimum skip known heavy/generated dirs like `.git`, `node_modules`, cache/build dirs, and the client state dir.
8. **Performance acceptance tests.** Add a large-tree benchmark/soak that proves idle CPU stays low and a single-file change does not trigger a full-tree scan.

## Acceptance gate

Do not promote until a live or dry-run soak on a large tree demonstrates:

- idle CPU under 1 to 2% after initial indexing,
- a single-file edit scans O(1) or O(directory) paths, not O(root),
- no repeated full-root cycles during normal project activity,
- full-tree repair is manual or infrequent and visibly budgeted,
- state DB writes per no-op cycle are near zero.
