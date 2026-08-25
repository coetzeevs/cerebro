---
name: rules
description: Load the Cerebro behavioral rules (memory-system usage conventions) into context on demand. Use at session start in a project with a Cerebro brain, or when unsure how to use /cerebro:remember, /cerebro:recall, or origin identity conventions.
disable-model-invocation: false
allowed-tools: Bash(cerebro *)
---

# Cerebro Behavioral Rules

Plugins cannot auto-load CLAUDE.md content; this skill carries the same rules
`cerebro init` appends to a project's CLAUDE.md, loadable on demand.

## Cerebro Memory System

This environment uses Cerebro for persistent memory across sessions.

> Using Claude Code? The **cerebro plugin** is the preferred installation path
> (`/plugin marketplace add coetzeevs/cerebro`, then `/plugin install cerebro@cerebro`).
> `cerebro init` (which wrote this section) remains fully supported as the
> cross-tool fallback; the two coexist safely — lifecycle hooks are
> session-guarded in the binary and fire exactly once per session.

### Automatic behavior
- Session start: recent memories are loaded via hook (known to be intermittent — see fallback below)
- First prompt fallback: if session start hook fails silently, memories are injected on your first prompt
- Post-compaction: sentinel is cleared so memories are re-loaded on next prompt after compaction
- Session end: garbage collection runs automatically

### Post-compaction recovery
If you don't see Cerebro memories in your context after compaction (no primed memories in system reminders), proactively run `/recall` to restore context. This is a safety net for known hook injection bugs.

### When to remember
Use /remember proactively when you:
- Discover an architectural decision or constraint
- Learn a project convention or pattern
- Encounter and resolve a bug (especially if the root cause was non-obvious)
- Receive explicit user preferences or corrections
- Complete a significant task (capture the approach and outcome)
- Are about to lose context (compaction warning, session ending)

### Project directory
When invoking cerebro commands directly (outside /recall and /remember skills),
always pass `-p "$CLAUDE_PROJECT_DIR"` to ensure the correct brain is used.
The `$CLAUDE_PROJECT_DIR` env var is set by Claude Code to the session start directory.

### When to recall
Use /recall when you:
- Start working on a new area of the codebase
- Need context about past decisions or approaches
- Want to check if a similar problem was encountered before
- Need to understand project conventions for an unfamiliar area

### Origin identity
Memory writes are stamped with origin identity (who/what wrote them). Set
`CEREBRO_ORIGIN_ACTOR` (e.g. `claude-code`) in the environment so agent writes
classify `recorded`; the session id flows automatically via `CLAUDE_SESSION_ID`.

### Configuration
Per-brain defaults can be customized with `cerebro config set <key> <value>`.
Run `cerebro config list` to see available settings. CLI flags always override brain config.
