# Cerebro

Local-first persistent memory system for AI agents. Combines a knowledge graph with vector similarity search in a single SQLite file — no servers, no infrastructure.

## Why

AI coding agents lose context between sessions. Cerebro gives them durable memory: a per-project (or global) brain that stores concepts, episodes, procedures, and reflections as graph nodes with vector embeddings. Memories are scored by a composite of semantic relevance, importance, recency, and structural connectedness — so the agent retrieves what actually matters.

## Features

- **Single-file storage** — one SQLite database per project, easy to back up or move
- **Vector search** — sqlite-vec powered similarity search with cosine distance
- **Knowledge graph** — typed edges between memory nodes (e.g. `derived_from`, `contradicts`, `supports`)
- **Composite scoring** — relevance (0.35) + importance (0.25) + recency (0.25) + structural (0.15)
- **Lifecycle management** — garbage collection, reinforcement, supersession, consolidation
- **Global store** — promote memories to a cross-project global brain
- **Export/Import** — JSON, SQL dump, or full SQLite copy; import with skip/replace conflict resolution
- **Claude Code integration** — hooks, skills, and `cerebro init` for zero-config setup

## Install

### Homebrew (macOS)

```bash
brew install coetzeevs/tap/cerebro
```

### From source

Requires Go 1.24+ and a C compiler (CGO is needed for sqlite-vec):

```bash
# macOS
xcode-select --install

# Build and install
git clone https://github.com/coetzeevs/cerebro.git
cd cerebro
make install
```

## Quick start

```bash
# Initialize a brain for your project (creates .cerebro/brain.db)
cerebro init

# Store a memory
cerebro add --type concept --name "auth-flow" \
  --body "JWT-based auth with refresh tokens, 15min access / 7d refresh" \
  --importance 0.8

# Recall relevant memories (composite-scored)
cerebro recall "how does authentication work"

# Link memories
cerebro edge <source-id> <target-id> --type derived_from

# Search by vector similarity only
cerebro search "token expiration"

# View brain health
cerebro stats
```

## Commands

| Command | Description |
|---------|-------------|
| `add` | Store a new memory node |
| `get` | Retrieve a node with its edges |
| `update` | Modify an existing node |
| `list` | List nodes with optional filters |
| `recall` | Composite-scored retrieval for a query |
| `search` | Raw vector similarity search |
| `edge` | Create a relationship between nodes |
| `reinforce` | Increment access count on a memory |
| `supersede` | Replace a memory with an updated version |
| `mark-consolidated` | Mark episodes as consolidated |
| `gc` | Evict decayed memories to archive |
| `promote` | Copy a node to the global store |
| `export` | Export brain (json, sql, or sqlite) |
| `import` | Import memories from JSON export |
| `init` | Bootstrap brain + Claude Code integration |
| `stats` | Show brain health metrics |

## Claude Code integration

`cerebro init` scaffolds everything needed for Claude Code:

- **Hooks** — automatic memory recall on session start, save reminders before compaction, GC on exit
- **Skills** — `/remember` and `/recall` slash commands
- **CLAUDE.md** — project instructions for when/how to use memory

This makes memory transparent to the agent — it just works across sessions without manual setup.

## Architecture

Cerebro follows **Model B** (agent-managed memory): the AI agent decides what to store and retrieve. Cerebro is pure storage infrastructure with no LLM of its own. See [ADR-006](docs/adrs/adr-006-system-architecture-model-b.md) for the rationale.

```
cmd/cerebro/       CLI (Cobra commands)
brain/             Public API (Brain type)
internal/store/    SQLite storage, schema, CRUD, vector search
internal/embed/    Embedding provider interface + implementations
```

## Development

```bash
make build        # Build binary
make test         # Run tests with race detector
make test-cover   # Tests + coverage report
make lint         # golangci-lint
make clean        # Remove artifacts
```

Pre-commit hooks: `pre-commit install` (requires [pre-commit](https://pre-commit.com/))

## License

MIT
