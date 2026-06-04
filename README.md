# Umbrel Dropbox

Linux-first Dropbox sync daemon for Umbrel and general Linux desktops.

This is a proper bidirectional sync client built on the published Dropbox HTTP API, not a thin wrapper around the official Linux Dropbox daemon.

## Trademark notice

**Dropbox** is a trademark of Dropbox, Inc. This project is an independent, community-maintained Umbrel app and is **not affiliated with, endorsed by, or sponsored by Dropbox, Inc.** All references to the Dropbox™ name, logo, and service are for identification and interoperability purposes only. The app implements the published Dropbox HTTP API to sync files between a local folder and a user's own Dropbox account; it makes no claim of ownership over the Dropbox brand or trademarks.

## Goals

- Deterministic bidirectional sync.
- Dropbox cursor-based remote delta ingestion.
- Linux inotify + periodic scanner for local changes.
- SQLite durable state and operation queue.
- Multithreaded upload/download workers.
- Conservative conflict handling with rollback paths.
- CLI + daemon + systemd service.
- GNU/Linux file explorer context menus.
- Umbrel App Store packaging.
- `.deb` and future APT repository distribution.

## Current status

Production-ready (v1.2.5).

- Bidirectional Dropbox sync via the published HTTP API (OAuth2 PKCE, refresh tokens, device-code fallback).
- Durable SQLite state store with cursor-based remote delta ingestion.
- Linux inotify + periodic scanner for local changes, with single-segment dirty-prefix scoping.
- Multithreaded upload/download workers with retry/backoff and conservative conflict handling.
- Built-in web dashboard on `:8477` (status, files, settings, conflicts, in-process restart, save prompt).
- Umbrel App Store packaging, multi-arch Docker image, `.deb` / APT repository distribution.
- Live transfer is opt-in (`allow_live=true` + explicit live scope); dry-run is the default.

Trademark: Dropbox is a trademark of Dropbox, Inc. This app is an independent community project
and is not affiliated with, endorsed by, or sponsored by Dropbox, Inc. See the
[trademark notice](#trademark-notice) above.

## Features

- Drop-in Umbrel app with a single-page web dashboard (Dashboard / Files / Settings / Conflicts tabs).
- OAuth2 PKCE + device-code auth flows, refresh-token rotation, redacted auth status.
- Per-folder sync scoping (`sync_paths`) and per-folder exclude lists, with a save button that prompts for an in-process restart.
- Conflict detection with explicit local/remote resolution, and a Conflicts page that streams `/api/conflicts` JSON.
- Health endpoints: `/healthz`, `/api/status`, `/api/events`, `/api/config`, `/api/restart`, `/api/conflicts` (and `/api/conflicts/resolve`).
- Multi-arch container image (`linux/amd64` + `linux/arm64`) pinned by OCI digest in every compose file.

## Build

```bash
go test ./...
go build -o bin/umbrel-dropbox-client ./cmd/umbrel-dropbox-client
go build -o bin/umbrel-dropbox-clientd ./cmd/umbrel-dropbox-clientd
```

## Initialize

```bash
umbrel-dropbox-client init --root ~/Dropbox
umbrel-dropbox-client status
```

## Install Linux binary

```bash
curl -fsSL https://raw.githubusercontent.com/earlvanze/umbrel-dropbox-client/master/scripts/install-linux.sh | bash
```

## Agentic coding flywheel

This repo is being aligned with the Agentic Coding Flywheel Setup (ACFS) workflow for production agent operations, installer checksum notifications, and repeatable validation. See [ACFS integration](docs/ACFS.md).

## Distribution docs

- [APT plan](docs/APT.md)
- [Desktop integration](docs/DESKTOP_INTEGRATION.md)
- [Umbrel app](docs/UMBREL_APP.md)
- [Roadmap](ROADMAP.md)
