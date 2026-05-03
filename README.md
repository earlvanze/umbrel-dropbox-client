# Umbrel Dropbox Client

Linux-first Dropbox sync daemon for Umbrel and general Linux desktops.

This is intended to become a proper sync client, not a thin wrapper around the official Linux Dropbox daemon.

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

MVP scaffold:

- Go CLI and daemon compile.
- SQLite schema exists.
- Dropbox content-hash implementation exists.
- Dropbox account API client stub exists.
- systemd, Docker, Umbrel app, GoReleaser, and desktop integration scaffolds exist.

Not production sync-ready yet.

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
