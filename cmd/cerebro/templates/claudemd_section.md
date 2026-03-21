## Cerebro Memory System

This environment uses Cerebro for persistent memory across sessions.

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

### When to recall
Use /recall when you:
- Start working on a new area of the codebase
- Need context about past decisions or approaches
- Want to check if a similar problem was encountered before
- Need to understand project conventions for an unfamiliar area
