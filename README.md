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
# Initialize a brain for your project (stored at ~/.cerebro/projects/<hash>.sqlite)
cerebro init

# Store a memory
cerebro add --type concept --importance 0.8 \
  "JWT-based auth with refresh tokens, 15min access / 7d refresh"

# Recall relevant memories (composite-scored)
cerebro recall "how does authentication work"

# Link memories
cerebro edge <source-id> <target-id> derived_from

# Search by vector similarity only
cerebro search "token expiration"

# View brain health
cerebro stats
```

## Commands

| Command | Description |
|---------|-------------|
| `init` | Bootstrap brain + Claude Code integration |
| `add` | Store a new memory node |
| `get` | Retrieve a node with its edges |
| `update` | Modify an existing node's content or importance |
| `list` | List nodes with optional filters |
| `recall` | Composite-scored retrieval (supports `--prime` for session start) |
| `search` | Raw vector similarity search |
| `edge` | Create a relationship between nodes |
| `reinforce` | Increment access count on a memory |
| `supersede` | Replace a memory with an updated version |
| `mark-consolidated` | Mark episodes as consolidated |
| `promote` | Copy a node to the global store |
| `gc` | Evict decayed memories to archive |
| `export` | Export brain (json, sql, or sqlite) |
| `import` | Import memories from JSON export |
| `backup` | Create a timestamped backup of the brain database |
| `config` | View and modify brain configuration (set/get/list/reset) |
| `stop-guard` | Evaluate Stop hook input for premature stopping patterns |
| `ingest` | Parse Claude Code session files and collect per-turn performance metrics |
| `dashboard` | Interactive full-screen performance metrics TUI with live refresh |
| `stats` | Show brain health metrics (add `--metrics` for session sparklines) |

## Memory types

Cerebro supports four semantic memory types, each with a different decay rate:

| Type | Purpose | Half-life |
|------|---------|-----------|
| `episode` | What happened — event-based memories | ~1-2 weeks |
| `concept` | What I know — learned knowledge | ~2-3 months |
| `procedure` | How to do it — procedural knowledge | ~6+ months |
| `reflection` | What I concluded — self-reflective insights | ~3-4 weeks |

Decay drives garbage collection: low-importance episodes fade quickly, while high-importance procedures persist for months.

## Embedding providers

Cerebro requires an embedding provider for vector search. Choose one at `cerebro init`:

| Provider | Flag | Default model | Dimensions | Notes |
|----------|------|---------------|------------|-------|
| **Ollama** (local) | `--embed-provider ollama` | `nomic-embed-text` | 768 | Default. Requires [Ollama](https://ollama.com) running locally. |
| **Voyage AI** (cloud) | `--embed-provider voyage` | `voyage-3.5` | 1024 | Set `CEREBRO_VOYAGE_API_KEY` env var. |
| **None** (graph-only) | `--embed-provider none` | — | — | Disables vector search; graph and metadata only. |

```bash
# Local embeddings (default)
cerebro init --embed-provider ollama

# Cloud embeddings
export CEREBRO_VOYAGE_API_KEY=your-key
cerebro init --embed-provider voyage

# No embeddings (graph-only mode)
cerebro init --embed-provider none
```

## Project vs global store

Each project gets its own brain, stored at `~/.cerebro/projects/<sha256-of-path>.sqlite`. You can also maintain a **global store** for cross-project knowledge:

```bash
# Initialize global store
cerebro init --global

# Promote a project memory to global
cerebro promote <node-id> "Generalized version of this knowledge"

# Recall with global fallback (project results weighted 1.0, global 0.7)
cerebro recall --global "deployment patterns"
```

## Claude Code integration

`cerebro init` scaffolds everything needed for [Claude Code](https://docs.anthropic.com/en/docs/claude-code):

- **Hooks** — memory recall on session start, GC on exit, stop-guard to prevent premature stopping
- **Skills** — `/remember`, `/recall`, `/consolidate`, and `/develop` (structured implementation workflow)
- **CLAUDE.md** — project instructions for when/how to use memory

This makes memory transparent to the agent — it just works across sessions without manual setup.

Use `--skip-integration` to create only the database without Claude Code files. Use `--force` to replace existing hooks, skill templates, and CLAUDE.md section with the latest versions.

## Configuration

Each brain carries its own configuration, stored alongside memory data. Defaults work out of the box — configuration is opt-in.

```bash
# View all settings and their current values
cerebro config list

# Override a default
cerebro config set prime_limit 30

# Check a specific value
cerebro config get prime_limit

# Revert to the compiled default
cerebro config reset prime_limit
```

**Precedence:** CLI flag (explicit) > brain config > compiled default.

Available settings:

| Key | Default | Description |
|-----|---------|-------------|
| `prime_limit` | 20 | Memories loaded at session start (`recall --prime`) |
| `gc_threshold` | 0.01 | GC eviction threshold |
| `search_limit` | 10 | Max results for the `search` command |
| `search_threshold` | 0.7 | Min similarity for the `search` command |
| `recall_threshold` | 0.3 | Min similarity for `recall` query mode |

Config values travel with the brain — they are preserved across `export`/`import`.

## Go library usage

As of v1.2.0, `brain/` re-exports all types needed by external Go modules:

```go
import "github.com/coetzeevs/cerebro/brain"

b, _ := brain.Open(brain.ProjectPath("/my/project"))
defer b.Close()

id, _ := b.Add("PLA prints at 210C", brain.Concept, brain.WithImportance(0.7))
results, _ := b.Search(ctx, "filament temperature", 5, 0.3)
```

Types available: `brain.Node`, `brain.ScoredNode`, `brain.NodeWithEdges`, `brain.NodeType`, `brain.ListNodesOpts`, `brain.Stats`, `brain.Edge`, `brain.GCResult`, `brain.ExportBundle`, `brain.ImportOptions`, `brain.ImportResult`.

Constants: `brain.Episode`, `brain.Concept`, `brain.Procedure`, `brain.Reflection`.

## Ecosystem

| Project | Description |
|---------|-------------|
| [QraftWorx CLI](https://github.com/coetzeevs/qraftworx-cli) | Go CLI for AI-powered content automation. Uses Gemini as reasoning engine and Cerebro as persistent memory. Imports `brain/` directly. |

## Architecture

Cerebro follows **Model B** (agent-managed memory): the AI agent decides what to store and retrieve. Cerebro is pure storage infrastructure with no LLM of its own. See [ADR-006](docs/adrs/ADR-006-claude-code-integration-pattern.md) for the rationale.

```
cmd/cerebro/       CLI (Cobra commands)
brain/             Public API (Brain type)
internal/store/    SQLite storage, schema, CRUD, vector search
internal/embed/    Embedding provider interface + implementations
```

Full architecture: [system-architecture.md](docs/architecture/system-architecture.md) | ADRs: [docs/adrs/](docs/adrs/)

## Development

```bash
make build        # Build binary
make install      # Build + install to /usr/local/bin
make test         # Run tests with race detector
make test-cover   # Tests + coverage report
make lint         # golangci-lint
make clean        # Remove artifacts
```

Pre-commit hooks: `pre-commit install` (requires [pre-commit](https://pre-commit.com/))

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full development workflow.

## License

MIT
