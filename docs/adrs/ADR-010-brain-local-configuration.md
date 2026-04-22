# ADR-010: Brain-Local Configuration

## Status

Accepted

## Context

All cerebro runtime defaults are hardcoded: the recall `--prime` limit is 20 (`cmd_recall.go`), the GC threshold is 0.01 (`cmd_gc.go`), search thresholds and limits are baked into Cobra flag defaults and skill templates. When hooks call `cerebro recall --prime`, the only way to change the limit is to edit the shell command inside `.claude/settings.json` — fragile, error-prone, and invisible to the brain itself.

ADR-002 specified a `~/.cerebro/config.toml` file with an `[embeddings]` section, but this was never implemented. Now that the CLI is mature and the integration hooks are stable, we need a configuration mechanism that lets users tune defaults without editing hooks or skill templates.

### Requirements

1. **Ease of use** — no file creation, no format to learn
2. **Extensibility** — add new config keys without schema changes
3. **Simplicity** — no new dependencies, minimal new code
4. **Agentic-optimized** — AI agents are the primary consumers; config should be accessible via the same CLI pattern as every other cerebro command

## Decision

Store configuration as key-value pairs in the existing `schema_meta` table, namespaced with a `config.` prefix. Expose via a `cerebro config` subcommand group (set/get/list/reset).

### Why brain-local config over a config file

Three approaches were evaluated:

| Approach | Ease of use | Extensibility | Simplicity | Agentic UX | Portability |
|----------|-------------|---------------|------------|------------|-------------|
| **A: Brain-local (schema_meta)** | High — `cerebro config set` | High — add a row | High — reuses existing store | Highest — same CLI pattern | Config travels with export/import |
| **B: Config file (TOML)** | Medium — create/edit file | High | Low — TOML dep, file resolution | Low — agents must read/write TOML | Config separate from brain |
| **C: Hybrid (brain + file)** | Medium — two places to configure | High | Low — most complex | Medium | Partial |

**Approach A was chosen** because:

- `schema_meta` already exists in every brain with `SetMeta`/`GetMeta` methods
- `GetAllMeta()` in `store.Export()` already exports all meta rows — config travels with the brain for free
- No new dependencies (no TOML parser, no file resolution logic)
- Agents call `cerebro config set prime_limit 30` — identical pattern to `cerebro add` or `cerebro gc`
- ADR-002's intent (configurable parameters) is served; the mechanism adapts to what we learned building the CLI-first architecture

### Precedence chain

```
CLI flag (explicit)  >  brain config  >  compiled default
```

Detection uses Cobra's `cmd.Flags().Changed()` — if a flag was not explicitly passed on the command line, the brain's config value is applied via `cmd.Flags().Set()`. If the brain has no config for that key, the compiled default from the Cobra flag definition stands.

### Initial config keys

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `prime_limit` | int | 20 | Memories loaded at session start |
| `gc_threshold` | float64 | 0.01 | GC eviction threshold |
| `search_limit` | int | 10 | Max results for `search` |
| `search_threshold` | float64 | 0.7 | Min similarity for `search` |
| `recall_threshold` | float64 | 0.3 | Min similarity for `recall` query mode |

Keys are validated on set (type checking, range constraints). Unknown keys are rejected with a helpful error.

### Skill template implications

Skills that hardcode values intrinsic to their algorithm (e.g., `/remember`'s `--limit 5 --threshold 0.7` for reconciliation) keep their explicit flags — these are intentional, not user preferences.

Skills where the value represents a user preference (e.g., `/recall`'s `--limit 10`) have the explicit flag removed so the brain's config takes effect.

The GC hook template's `--threshold 0.01` was removed for the same reason — the brain's `gc_threshold` config (or compiled default) now controls it.

## Consequences

### Positive

- **Zero-friction configuration.** `cerebro config set prime_limit 30` — done. No files, no formats, no paths.
- **Portable.** Config travels with the brain on export/import. A brain moved between machines retains its tuning.
- **Backward compatible.** Brains created before this change have no `config.*` rows — all resolves return compiled defaults. No migration needed.
- **Agent-native.** The config surface is CLI commands, not file editing. AI agents can configure cerebro programmatically.
- **Extensible.** Adding a new config key means adding one entry to the `configRegistry` map. No schema changes, no file format changes.

### Negative

- **Not human-editable without the CLI.** Users cannot open a `.toml` and tweak values — they must use `cerebro config set`. This is acceptable because the primary audience is AI agents.
- **No global defaults.** There is no `~/.cerebro/config.toml` for user-wide defaults. Each brain must be configured independently. A global config layer could be added later if needed.
- **Config keys in schema_meta.** The `config.` prefix prevents collisions with existing keys (`schema_version`, `embedding_*`), but the table now serves two purposes (metadata + config). This is a pragmatic tradeoff.

### Risks

- **Orphaned config keys.** If a config key is renamed or removed in a future version, old brains may have stale `config.*` rows. Mitigation: `resolveConfig*` functions ignore unknown keys gracefully; `config list` only shows registered keys.
- **ADR-002 divergence.** ADR-002 specified TOML config. This ADR supersedes that design for runtime config. Embedding configuration at init time remains as-is (CLI flags stored in schema_meta).

## References

- [ADR-002: Embedding Strategy](ADR-002-embedding-model-selection.md) — original config.toml specification
- [ADR-006: Claude Code Integration](ADR-006-claude-code-integration-pattern.md) — agent-first design philosophy
- Evidence: git config (3-tier precedence), ruff (TOML with sensible defaults), aider (YAML config hierarchy)
