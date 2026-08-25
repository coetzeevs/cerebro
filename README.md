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
brew install --cask coetzeevs/tap/cerebro
```

### Claude Code plugin (preferred for Claude Code users)

With the CLI installed, add the plugin for lifecycle hooks (session-start
recall priming, post-compaction recovery, session-end GC) and the
`/cerebro:remember`, `/cerebro:recall`, `/cerebro:consolidate`,
`/cerebro:develop`, `/cerebro:rules` skills:

```
/plugin marketplace add coetzeevs/cerebro
/plugin install cerebro@cerebro
```

The plugin's hooks self-gate on a brain existing for the project (silent
elsewhere) and are session-guarded in the binary, so running `cerebro init`
in the same project is safe — each lifecycle event fires exactly once per
session no matter how many paths registered it. `cerebro init` remains fully
supported as the cross-tool fallback (it also appends behavioral rules to the
project CLAUDE.md, which plugins cannot do; the plugin ships the same rules
as the on-demand `/cerebro:rules` skill).

The `cerebro stop-guard` Stop hook is deliberately NOT wired by the plugin
(plugins cannot ship a hook disabled, and the guard is opt-in): to enable it,
add the Stop hook from `cerebro init`'s settings template to your own
`.claude/settings.json`.

Set `CEREBRO_ORIGIN_ACTOR` (e.g. `claude-code`) in your environment so
memory writes are stamped with a recorded origin actor.

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
| `mark-consolidated` | Mark episodes as consolidated (status only, no edges) |
| `consolidate` | Consolidate source episodes into a node, recording `derived_from` provenance |
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

## Provenance (agentic-lbjg)

Provenance is structural, not freeform. A built-in `derived_from` relation links a
derived node (concept/procedure/reflection) back to the source episodes it was
synthesized from.

```bash
# Consolidate two episodes into a concept, auto-writing derived_from edges
cerebro consolidate --into <concept-id> <episode-id-1> <episode-id-2>

# Walk the lineage chain (default depth 5)
cerebro get <concept-id> --with-provenance
cerebro get <concept-id> --with-provenance=3        # custom depth (clamped to 100)

# Attach a chain per recall result (default depth 1)
cerebro recall "query" --with-provenance
cerebro recall "query" --provenance-depth 3

# Mark a node as a first-class provenance source
cerebro add "source memory" --type episode --provenance-root
```

`consolidate --into` is atomic and fail-closed (the into-node and every source
must resolve as an episode, else a non-zero exit with no partial write) and
idempotent (re-running writes no duplicate edges). It is distinct from
`mark-consolidated`, which only flips status and writes no edges.

`get`/`list`/`recall` JSON output carries a computed **`provenance_status`** field:

| Value | Meaning |
|-------|---------|
| `complete` | has ≥1 outgoing `derived_from` edge |
| `none` | no provenance edge, created after the convention boundary |
| `legacy` | no provenance edge, predates the convention (a v4-or-earlier brain's pre-migration nodes) |

`provenance_status` is computed at query time (no stored column). The legacy
boundary is recorded once, at the v4→v5 migration instant, in `schema_meta`. See
`docs/adrs/ADR-016-provenance-edges-and-walk-primitive.md` for the BFS-vs-CTE walk
decision and the legacy-boundary mechanism.

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
| `rerank_enabled` | `false` | Enable local cross-encoder reranking of recall candidates (see below) |
| `rerank_command` | _(empty)_ | Local reranker subprocess; empty falls back to `CEREBRO_RERANK_COMMAND` env or disables |
| `rerank_fusion` | `rrf` | Combine mode when reranking is on: `rrf` (Reciprocal Rank Fusion) or `reorder` (legacy pure-reorder) |
| `bm25_enabled` | `true` | BM25/FTS5 keyword recall — always-on when the binary has the `fts5` build tag. `false` is an eval/diagnostic seam, not a feature toggle (see below) |
| `expand_threshold` | 0.75 | Skip graph expansion when the top-1 vector cosine similarity strictly exceeds this value (range `[0,1]`; `0.0` disables — see below) |
| `expand_spread_threshold` | 0.0 | Skip graph expansion when the full top-K similarity spread (top-1 − top-K) is strictly below this value (range `[0,1]`; `0.0` = off, the shipped default — see below) |

Config values travel with the brain — they are preserved across `export`/`import`.

### Optional cross-encoder reranking (agentic-2ixw)

Reranking is **off by default**; when off, recall is byte-identical to the
pre-rerank pipeline. When enabled, `recall`/`search` over-retrieve a wider
candidate set by composite score, rerank it with a local cross-encoder, and cut
to the limit (`≤10` typical) — so the most relevant memories rank highest. The
composite scorer weights are unchanged; reranking only governs the final
ordering of the cut.

**Combine mode (`rerank_fusion`).** The reranker ranking is combined with the
composite ranking in one of two ways. The default, **`rrf`** (Reciprocal Rank
Fusion, `fused = 1/(60+rank_composite) + 1/(60+rank_reranker)`), fuses both
rankings so a composite-strong item the cross-encoder demotes still survives the
cut — this recovers a recall@10 dip that pure-reorder exhibits. **`reorder`**
(legacy) sorts by the reranker score alone, discarding the composite order
(maximises MRR at some cost to recall@10). See
[`docs/evals/rerank-results.md`](docs/evals/rerank-results.md) and
[ADR-012](docs/adrs/ADR-012-cross-encoder-reranking.md) for the measured
tradeoff. Switch with `cerebro config set rerank_fusion reorder -p <brain>`.

cerebro bundles **no model.** You supply a local reranker subprocess that reads a
JSON request on stdin and writes a JSON response on stdout:

```
stdin:  {"query": "<text>", "documents": ["doc0", "doc1", ...]}
stdout: {"scores": [s0, s1, ...]}      # one finite score per document, index-aligned
```

Enable it:

```bash
cerebro config set rerank_enabled true -p <brain>
# either set the command in brain config (wins over the env var)...
cerebro config set rerank_command "/path/to/python /path/to/rerank.py" -p <brain>
# ...or via the environment:
export CEREBRO_RERANK_COMMAND="/path/to/python /path/to/rerank.py"
```

A reference reranker (MiniLM cross-encoder, ~90MB, downloaded by the script not
by cerebro) ships under [`docs/evals/reranker/`](docs/evals/reranker/). The
recommended model is `cross-encoder/ms-marco-MiniLM-L6-v2` for footprint;
`bge-reranker-v2-m3` is a higher-quality multilingual option if you accept its
~568MB size.

**Graceful degradation.** If reranking is enabled but the command is
unset/missing/crashes or returns malformed, short, or non-finite output, cerebro
logs a one-line stderr warning and falls back to the pre-rerank composite order
— recall is never worse than disabled. The command is run argv-array style
(never a shell) with a bounded timeout.

### BM25 keyword recall (agentic-2lak)

`recall`/`search` compose a **BM25 keyword lane** alongside vector similarity, so
exact-identifier and exact-term queries (a ticket ID like `HS-049`, a precise
symbol name) surface the memory that literally contains the token — cases pure
semantic similarity misses. A `nodes_fts` FTS5 virtual table mirrors each active
memory's content + subtype; the keyword lane runs an FTS5 `bm25()` ranking and is
fused with the vector/composite lane by **Reciprocal Rank Fusion** (`k=60`)
*before* the optional reranker. The four-signal composite scorer weights are
unchanged — BM25 enters via recall-layer fusion, not by re-weighting the
composite. See [ADR-013](docs/adrs/ADR-013-bm25-fts5-keyword-recall.md) and
[`docs/evals/bm25-results.md`](docs/evals/bm25-results.md).

**Requires the `fts5` build tag.** `mattn/go-sqlite3` gates FTS5 behind a CGO
build tag (no new Go dependency — it is a compile flag, not a module). The
`Makefile`, CI, and goreleaser config all set it, so released Homebrew binaries
and `make build` link FTS5. If you build by hand, use
`go build -tags fts5 ./cmd/cerebro` — a binary without the tag runs normally but
the keyword lane silently contributes nothing (graceful degrade: store open and
writes are never coupled to the tag).

**`bm25_enabled` is a diagnostic seam, not a feature toggle.** BM25 is always-on
when the binary has the `fts5` tag. `cerebro config set bm25_enabled false`
short-circuits the keyword lane to the literal pre-BM25 pipeline — its purpose is
to produce the same-session BM25-disabled baseline for non-regression evaluation
(see the eval protocol in `docs/evals/bm25-results.md`), not to let end users opt
out of keyword recall.

### Lazy expansion gating (agentic-73l6)

`recall`/`search` skip single-hop **graph expansion** (the edge-walk that pulls
in connected neighbours) when the vector top-K is already confident — saving
two SQL round-trips per gated query with zero measured recall change (see
[`docs/evals/lazy-gating-results.md`](docs/evals/lazy-gating-results.md)). The
gate fires when the top-1 raw cosine similarity strictly exceeds
`expand_threshold` (shipped active at `0.75` — 22% of eval queries gate on the
reference brain), or when the full top-K similarity spread is strictly below
`expand_spread_threshold` (shipped **off** at `0.0`: on the reference brain the
spread anti-correlates with confidence). Setting both keys to `0.0` disables
the gate entirely — the pipeline is then byte-identical to the pre-gate
pipeline. The BM25 keyword lane and the optional reranker run unchanged on
every query, gated or not. See
[ADR-014](docs/adrs/ADR-014-lazy-expansion-gating.md).

**Skip metric.** Each gate fire increments the persistent counter
`stats.expansion_skips` in the brain's `schema_meta` table (best-effort — a
counter write can never fail or slow a recall). It counts skip events per
expansion site and never resets, so read deltas:

```bash
sqlite3 ~/.cerebro/projects/<hash>.sqlite \
  "SELECT value FROM schema_meta WHERE key='stats.expansion_skips'"
```

`SearchWithGlobal` (project + global stores) gates each store independently on
its own result set, and both skip events are recorded on the **project**
brain's counter.

## Go library usage

As of v1.2.0, `brain/` re-exports all types needed by external Go modules:

```go
import "github.com/coetzeevs/cerebro/brain"

b, _ := brain.Open(brain.ProjectPath("/my/project"))
defer b.Close()

id, _ := b.Add("PLA prints at 210C", brain.Concept, brain.WithImportance(0.7))
results, _ := b.Search(ctx, "filament temperature", 5, 0.3)

// Provenance (agentic-lbjg) — additive API:
root, _ := b.Add("source episode", brain.Episode, brain.WithProvenanceRoot())
_ = b.Consolidate(conceptID, []string{episodeID})          // auto-write derived_from edges
chain, _ := b.WalkProvenance(conceptID, 5)                  // []brain.NodeWithDepth lineage
status, _ := b.ProvenanceStatus([]string{conceptID})        // id -> complete|none|legacy
```

Types available: `brain.Node`, `brain.ScoredNode`, `brain.NodeWithEdges`, `brain.NodeWithDepth`, `brain.NodeType`, `brain.ListNodesOpts`, `brain.Stats`, `brain.Edge`, `brain.GCResult`, `brain.ExportBundle`, `brain.ImportOptions`, `brain.ImportResult`.

Constants: `brain.Episode`, `brain.Concept`, `brain.Procedure`, `brain.Reflection`, `brain.RelationDerivedFrom`.

Provenance methods: `brain.WithProvenanceRoot()` (AddOption), `b.Consolidate(intoID, episodeIDs)`, `b.WalkProvenance(id, depth)`, `b.ProvenanceStatus(ids)`. The underlying reusable BFS primitive is `store.WalkRelation(startID, relation, maxDepth, outgoing)`.

## Ecosystem

| Project | Description |
|---------|-------------|
| [QraftWorx CLI](https://github.com/coetzeevs/qraftworx-cli) | Go CLI for AI-powered content automation. Uses Gemini as reasoning engine and Cerebro as persistent memory. Imports `brain/` directly. |

## Migration (HS-008 realpath hashing)

From HS-008 onwards, `brain.ProjectPath` resolves symlinks before hashing. On macOS `/tmp` is a symlink to `/private/tmp`, so pre-HS-008 sessions started via `/tmp/myproject` created a different brain than sessions started via `/private/tmp/myproject`. The migration command consolidates these duplicates:

```bash
# Preview what would be migrated (no files changed)
cerebro migrate --realpath-hashes --dry-run

# Run the migration (idempotent; safe to run multiple times)
cerebro migrate --realpath-hashes

# Narrow the scan to a specific directory tree
cerebro migrate --realpath-hashes --scan-root ~/projects --max-depth 3
```

The command walks `$HOME` (or `--scan-root`) to depth 4 by default, looking for project directories whose realpath hash differs from their unresolved-path hash. For each such directory:

- **Case A** — only old-hash brain exists: renamed atomically to new hash (+ WAL companions)
- **Case C** — both old and new hash exist: old brain is merged into new with `ConflictSkip` (destination wins), then deleted. Both are backed up to `~/.cerebro/backups/migrate-<timestamp>/` unconditionally before any mutation.

A process-level lockfile (`~/.cerebro/migrate.lock`) prevents concurrent migration runs.

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
