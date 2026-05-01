# ACFS Integration

This repo uses the Agentic Coding Flywheel Setup project as the model for agent-friendly production operations.

Source:
<https://github.com/Dicklesworthstone/agentic_coding_flywheel_setup>

## What is adopted here

- Installer checksum notification workflow, adapted for this repo's Linux installer.
- Validation workflow for ACFS readiness.
- Agent operating pattern: small safe increments, tests before push, status files updated every milestone.

## Local ACFS clone

The ACFS repo is mirrored locally for reference at:

```text
/home/umbrel/.openclaw/workspace/external/agentic_coding_flywheel_setup
```

Do not run the full ACFS bootstrap installer on Umbrel without explicit approval. It is designed for fresh Ubuntu VPS environments and can change shell, sudo, language runtimes, and agent tooling.

## GitHub setup still needed

To make `.github/workflows/notify-acfs.yml` dispatch to ACFS, add this repository secret:

```text
ACFS_REPO_DISPATCH_TOKEN
```

It needs permission to dispatch events to:

```text
Dicklesworthstone/agentic_coding_flywheel_setup
```

Tool key used by this repo:

```text
umbrel_dropbox_sync
```

Installer path:

```text
scripts/install-linux.sh
```

## Current ACFS delta

The upstream ACFS `checksums.yaml` does not yet include `umbrel_dropbox_sync`. Until it does, validation warns instead of failing.
