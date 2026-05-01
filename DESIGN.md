# Design

## Architecture

`umbrel-dropbox-sync` has three loops:

1. **Remote delta loop**
   - Uses Dropbox `/2/files/list_folder` + `/continue` cursors.
   - Persists cursor in SQLite only after all remote changes in a batch are applied.

2. **Local watcher loop**
   - Uses Linux inotify via fsnotify.
   - Debounces changes into a local event queue.
   - Falls back to periodic tree scan for missed events.

3. **Transfer workers**
   - Separate bounded pools for uploads, downloads, deletes, and metadata moves.
   - Uses Dropbox content hash for identity checks.
   - Handles 429 / Retry-After with backoff.

## State

SQLite tables:
- `config`: key/value settings and Dropbox cursors.
- `entries`: canonical path, Dropbox id/rev, local mtime/size/hash, state.
- `pending_ops`: durable operation queue.
- `conflicts`: conflicts requiring deterministic resolution.
- `events`: append-only audit log.

## Conflict policy

Default policy is conservative:
- If only one side changed since last known sync, apply that change.
- If both changed, keep canonical path as newest by mtime only when hashes prove lineage is known.
- Otherwise create a sibling `filename (conflict YYYY-MM-DD HHMMSS source).ext`, log it, and continue.
- Never create Dropbox-native `conflicted copy` names.

## Expected CLI

```bash
umbrel-dropbox-sync init --root ~/Dropbox --remote-path ""
umbrel-dropbox-sync auth login
umbrel-dropbox-sync scan --dry-run
umbrel-dropbox-sync sync --once --dry-run
umbrel-dropbox-sync daemon
umbrel-dropbox-sync status
umbrel-dropbox-sync conflicts list
umbrel-dropbox-sync conflicts resolve --prefer local|remote|newest --path ...
```

## Systemd

Daemon should run as `umbrel`, not root, with explicit include/exclude paths and low I/O priority.
