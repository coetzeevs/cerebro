# Cerebro

Local-first persistent memory system for AI agents. SQLite-backed with vector search (sqlite-vec).

## Dogfooding

This project is its own first use case. The test of cerebro's efficacy is whether the agent (Claude Code) has recall of this project's architecture, decisions, and the process used to build it — across sessions, through context compactions, without losing continuity. If cerebro works, you should know why we chose Model B over Model C, what ADR-006 decided, and how the store layer is structured without re-reading everything from scratch.

## Development

```bash
# Build
go build ./cmd/cerebro

# Test
go test ./... -race

# Test with coverage
go test ./... -race -coverprofile=coverage.out
go tool cover -func=coverage.out

# Lint
golangci-lint run
```

## Project Structure

```
cmd/cerebro/       CLI (Cobra commands)
brain/             Public API (Brain type) + type re-exports (brain/types.go)
internal/store/    SQLite storage, schema, CRUD, vector search
internal/embed/    Embedding provider interface + implementations
```

## Key Patterns

- **CGO required**: mattn/go-sqlite3 needs a C compiler (`xcode-select --install` on Mac)
- **TDD (strict)**: Always write a failing test before implementing. Red → Green → Refactor. No production code without a covering test.
- **Functional options**: `brain.WithImportance(0.8)`, `brain.WithContent("updated")`
- **Building-block CLI**: Low-level commands composed by the calling agent
- **Pre-commit hooks**: Install with `pre-commit install` (requires [pre-commit](https://pre-commit.com/))

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

## Conventions

- Keep test fixtures in `testdata/` directories
- Use `t.TempDir()` for test databases — no cleanup needed
- Node types: `episode`, `concept`, `procedure`, `reflection`
- Format flag: `--format md` (default) or `--format json`
