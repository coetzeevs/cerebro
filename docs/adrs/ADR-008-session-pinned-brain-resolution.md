# ADR-008: Session-Pinned Brain Resolution

## Status

Accepted

## Context

Cerebro uses a single SQLite file per project, stored at `~/.cerebro/projects/<sha256_of_abs_path>.sqlite`. The brain path was resolved via `resolveBrainPath()` which checked the `-p` flag then fell back to `os.Getwd()`.

This caused "brain not found" errors when:

1. **Skills didn't pass `-p`**: The skill templates (recall, remember, consolidate) invoked cerebro without `-p`, relying on cwd.
2. **Agent cwd drifts**: During a session, the AI agent's working directory changes (subagents, cd into subdirs, temp dirs) while the intended brain stays the same.
3. **Nested projects**: A user with `/projects/` as a shared brain and child directories as local brains couldn't express "use the parent brain by default."

The core insight: a Claude Code session always starts from a specific project folder. That folder should be pinned for the session's duration.

## Decision

Use the `CLAUDE_PROJECT_DIR` environment variable (set by Claude Code to the session start directory) as the primary fallback when no explicit `-p` flag is passed.

### Resolution order

```
1. --project / -p flag         (explicit override, always wins)
2. CLAUDE_PROJECT_DIR env var  (session-pinned, set by Claude Code)
3. cwd                         (backward compatible fallback)
```

### Implementation

- Extract `resolveProjectDir()` function in the CLI layer with the 3-step resolution
- `resolveBrainPath()` delegates to `resolveProjectDir()`
- All skill templates pass `-p "$CLAUDE_PROJECT_DIR"` (belt-and-suspenders)
- CLAUDE.md template instructs agents to pass `-p "$CLAUDE_PROJECT_DIR"` for direct invocations

### Design principles

- Env var awareness stays in the CLI layer (`cmd/cerebro/`), not in `brain/`. `brain.ProjectPath()` remains a pure function.
- No new dependencies, no config files, no directory traversal.
- The user can override with `-p` at any point in a session.

## Alternatives Considered

### 1. Directory walk-up with `.cerebro` marker file

Walk up from cwd looking for a marker file. Rejected: implicit, unpredictable with nested projects, ambiguous when parent and child both have markers.

### 2. `~/.cerebro/config.toml` with directory mappings and longest-prefix-match

Centralized config mapping directories to brains. Rejected: longest-prefix-match conflicts with intended use when working at the parent level but the agent cds into a child project. Over-engineered for the actual problem.

### 3. `CEREBRO_PROJECT_DIR` custom env var

A cerebro-specific env var for non-Claude-Code orchestrators. Deferred: the only current orchestrator is Claude Code. Non-Claude orchestrators can use `-p` or set `CLAUDE_PROJECT_DIR`. Easy to add later if needed.

### 4. `~/.cerebro/config.toml` with `default_brain`

A global default brain for unmatched directories. Deferred: the session-pinned approach solves the actual problem without configuration. A default brain concept serves only standalone CLI usage.

## Consequences

### Positive

- Zero new complexity: 3-line function, no new packages or files (beyond the ADR)
- Backward compatible: if `CLAUDE_PROJECT_DIR` is unset, behavior is identical to before
- Solves the skill problem: skills now pass `-p` and the CLI checks the env var
- Session-pinned: the brain stays fixed to the session start directory regardless of cwd drift
- Debuggable: `echo $CLAUDE_PROJECT_DIR` shows exactly what cerebro will use

### Negative

- Claude Code coupling: relies on `CLAUDE_PROJECT_DIR` being set. Other orchestrators must use `-p` explicitly.
- No multi-brain switching within a session without `-p`. Accepted: the user can prompt the agent to use a specific brain.
