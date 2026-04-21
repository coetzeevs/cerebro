# ADR-009: Safe Init with Automatic Backup

## Status

Accepted

## Context

Running `cerebro init` on an already-initialized project is technically safe — all DDL uses `CREATE TABLE IF NOT EXISTS`, so no data is dropped. However:

1. **No user feedback**: There was no indication that the brain already existed. Users had to trust that init was safe without verification.
2. **No backup mechanism**: If something went wrong (corrupted DB, accidental parameter change), there was no safety net. The only option was `cerebro export --format sqlite`, which is manual.
3. **No update path for skills**: `scaffoldSkills()` skipped existing files, meaning template improvements (like adding `-p "$CLAUDE_PROJECT_DIR"` in PR #20) couldn't reach existing installations without manual intervention.
4. **Embedding metadata silently overwritten**: Re-running init with different `--embed-provider` or `--embed-dims` would silently overwrite the provider/model/dimensions metadata via `ON CONFLICT DO UPDATE`.

## Decision

### 1. Automatic backup on re-init

When `cerebro init` detects an existing brain file, it creates a timestamped backup to `~/.cerebro/backups/` before proceeding. The backup is a raw file copy (same mechanism as `ExportSQLite`). If the backup fails, init aborts.

### 2. `cerebro backup` standalone command

A new command for on-demand backups:

```bash
cerebro backup                    # → ~/.cerebro/backups/<hash>_<timestamp>.sqlite
cerebro backup -o /path/to/file   # → explicit output path
```

### 3. `--force` flag for skill updates

`cerebro init --force` overwrites existing skill templates with the latest versions. Without `--force`, existing skills are skipped (preserving any user customizations). The skip message now hints: `"Skipped skills (already present, use --force to update)"`.

### 4. Improved messaging

Init output now distinguishes between fresh initialization and re-initialization:

```
# Fresh:
Initialized brain at ~/.cerebro/projects/<hash>.sqlite

# Re-init:
Brain already exists at ~/.cerebro/projects/<hash>.sqlite
Backed up to ~/.cerebro/backups/<hash>_20260421_120000.sqlite
Re-initialized brain (schema validated)
```

## Consequences

### Positive

- **Safety net**: Every re-init is backed up. Users can recover from accidental parameter changes.
- **Clear upgrade path**: `cerebro init --force` updates skill templates without manual file deletion.
- **Transparent**: Users see exactly what's happening (backup path, re-init confirmation).
- **No data model changes**: All logic is in the CLI layer. No changes to `brain/` or `internal/store/`.

### Negative

- **Disk usage**: Backups accumulate in `~/.cerebro/backups/`. No automatic pruning (can be added later).
- **Slight slowdown**: File copy on re-init adds milliseconds. Negligible for SQLite files.

## Alternatives Considered

1. **Refuse to init if brain exists**: Too restrictive. Re-init is a valid use case (schema migration, config change, integration upgrade).
2. **Prompt for confirmation**: Cerebro is a non-interactive CLI consumed by AI agents. Prompts would break automation.
3. **Automatic backup pruning (keep N most recent)**: Deferred. Not needed until disk usage becomes a concern.
