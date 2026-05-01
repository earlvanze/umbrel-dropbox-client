# Production build task

Owner request: build this toward a production-grade Linux/Umbrel Dropbox sync client, installable on any Linux and eventually deployable via Umbrel App Store.

Hard constraints:
- Do not stop or replace the current official Dropbox daemon on Umbrel.
- Keep destructive operations behind dry-run/safety flags until explicitly approved.
- Commit small working increments and push to GitHub.
- Run `go test ./...` before each push.
- Maintain status docs so the monitor can report progress.

Production priorities:
1. OAuth/device auth flow with secure token storage.
2. Dropbox remote delta cursor ingestion.
3. Local scanner and inotify watcher.
4. Durable SQLite pending operation queue.
5. Parallel upload/download workers with 429/Retry-After backoff.
6. Conflict-safe reconciliation, no silent overwrites.
7. CLI: status, doctor, scan, sync --once, conflicts list/resolve, pause/resume.
8. systemd user service/timer and health endpoint.
9. Docker/Umbrel app with basic web UI for auth/status/conflicts.
10. Linux desktop integration packages for Nautilus/Nemo/Caja/Dolphin.
11. GoReleaser debs + GitHub Pages APT repo.
12. Tests: content hash, scanner, state queue, conflict policy, Dropbox API mocks.

Progress reporting:
- Update `STATUS.md` after meaningful milestones.
- Keep latest state under `reports/build-status.json`.
