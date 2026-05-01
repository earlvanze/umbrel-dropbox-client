# Umbrel app packaging

Current scaffold lives in `umbrel-app/`:

- `umbrel-app.yml`
- `docker-compose.yml`

Before app-store submission:

1. Publish GHCR image from release workflow.
2. Add screenshot gallery.
3. Add web UI for auth/status/conflicts.
4. Validate app manifest against Umbrel's current app-store schema.
5. Submit PR to Umbrel community app store or official store path.
