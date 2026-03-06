# Cerebro: System Architecture

> **Version:** 0.1.0 — Draft
> **Date:** 2026-03-04
> **Status:** Proposed
> **Supersedes:** `docs/sqlit-graph-architecture.md` (initial musing)

---

## 1. Vision

Cerebro is a **local-first, zero-infrastructure persistent memory system** for AI agent orchestrators. It combines a knowledge graph with vector similarity search in a single SQLite file, giving orchestrators structured recall, semantic search, and a living knowledge base that grows, consolidates, and forgets — like a brain.

### Design Principles

1. **Single-file, zero-server.** The entire memory for a project is one `.sqlite` file. No Docker, no mandatory background services.
2. **Orchestrator-only access.** Only the orchestrator reads from and writes to Cerebro. Sub-agents receive pre-formatted context injected by the orchestrator. They never touch the brain directly.
3. **Project-scoped by default, global by promotion.** Each project gets its own brain. Knowledge that transcends projects can be promoted to a global brain, following a git-config-like precedence model.
4. **Reconcile, don't append.** Every new memory is compared against existing knowledge. Duplicates are merged, contradictions resolved, and only genuinely new information is added.
5. **Type-aware lifecycle.** Different memory types (episodes, concepts, procedures, reflections) decay, consolidate, and reinforce at different rates.
6. **Multi-signal retrieval.** Memory recall combines vector similarity, graph traversal, temporal recency, and demonstrated importance — no single signal dominates.

---

## 2. Architecture Overview

```
┌──────────────────────────────────────────────────────────────────┐
│                        ORCHESTRATOR LLM                          │
│                    (Claude, GPT, etc.)                            │
├──────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌─────────────┐    ┌──────────────┐    ┌────────────────────┐  │
│  │   RECALL    │    │   REMEMBER   │    │     INJECT         │  │
│  │ query brain │    │ store memory │    │ format for         │  │
│  │ at task     │    │ after task   │    │ sub-agents         │  │
│  │ start       │    │ completion   │    │                    │  │
│  └──────┬──────┘    └──────┬───────┘    └────────┬───────────┘  │
│         │                  │                     │               │
└─────────┼──────────────────┼─────────────────────┼───────────────┘
          │                  │                     │
          ▼                  ▼                     ▼
┌──────────────────────────────────────────────────────────────────┐
│                       CEREBRO ENGINE                             │
│                                                                  │
│  ┌────────────┐  ┌──────────────┐  ┌────────────┐  ┌─────────┐ │
│  │  Retrieval  │  │Reconciliation│  │ Lifecycle  │  │Embedding│ │
│  │  (search +  │  │ (ADD/UPDATE/ │  │ (decay,    │  │Provider │ │
│  │   graph     │  │  DELETE/NOOP)│  │  consolidate│ │(pluggable│ │
│  │   traverse) │  │              │  │  evict)    │  │         │ │
│  └──────┬──────┘  └──────┬───────┘  └─────┬──────┘  └────┬────┘ │
│         │                │               │              │       │
│         └────────────────┴───────────────┴──────────────┘       │
│                              │                                   │
│                              ▼                                   │
│                    ┌───────────────────┐                         │
│                    │   brain.sqlite    │                         │
│                    │                   │                         │
│                    │  nodes (graph)    │                         │
│                    │  edges (graph)    │                         │
│                    │  vec_nodes (vec)  │                         │
│                    │  nodes_archive    │                         │
│                    └───────────────────┘                         │
└──────────────────────────────────────────────────────────────────┘
```

---

## 3. Memory Model

### 3.1 Memory Types

Cerebro classifies memories into four cognitive types, each with distinct lifecycle characteristics.

| Type | What It Stores | Decay Rate | Half-Life | Example |
|------|---------------|------------|-----------|---------|
| **Episode** | Specific events, interactions, outcomes | Fast (λ=0.15) | ~1-2 weeks | "Refactored auth module; composition pattern worked, inheritance didn't" |
| **Concept** | Facts, entities, relationships, preferences | Slow (λ=0.02) | ~2-3 months | "Project uses PostgreSQL 16 with pgvector" |
| **Procedure** | Rules, patterns, anti-patterns, workflows | Very slow (λ=0.005) | ~6+ months | "Always run migrations before deploying to staging" |
| **Reflection** | Higher-order observations from memory clusters | Medium (λ=0.05) | ~3-4 weeks | "Three bugs this week from date formatting — systemic issue" |

**Subtypes:**
```
episode:     [interaction, incident, discovery, decision_point]
concept:     [entity, relationship, preference, constraint]
procedure:   [rule, pattern, anti_pattern, workflow]
reflection:  [summary, lesson, insight, open_question]
```

### 3.2 Memory Lifecycle

```
  ┌─────────┐     ┌───────────┐     ┌──────────────┐     ┌──────────┐
  │  INGEST │────▶│RECONCILE  │────▶│    ACTIVE     │────▶│ ARCHIVE  │
  │         │     │ADD/UPDATE/│     │               │     │          │
  │ raw     │     │DELETE/NOOP│     │ decay +       │     │ evicted  │
  │ input   │     │           │     │ reinforce     │     │ memories │
  └─────────┘     └───────────┘     └───────┬───────┘     └──────────┘
                                            │
                                            │ periodic
                                            ▼
                                    ┌───────────────┐
                                    │  CONSOLIDATE  │
                                    │               │
                                    │ episodes →    │
                                    │ concepts +    │
                                    │ procedures    │
                                    └───────────────┘
```

**Ingest:** Raw information enters from orchestrator observations.

**Reconcile (Mem0 model):** Every new memory is compared against existing nodes via vector similarity. For each near-match (cosine > 0.85):
- **ADD** — genuinely new information, no overlap
- **UPDATE** — refines or extends an existing memory
- **DELETE** — contradicts an existing memory (old is superseded)
- **NOOP** — already captured, skip

**Active:** Memories live in the active store. Their retrieval score is computed dynamically:
```
retrieval_score = importance × (1 + ln(1 + access_count)) × e^(-decay_rate × hours_since_access)
```

**Consolidate:** Periodic or on-demand. Processes unconsolidated episodes into concepts, procedures, and reflections. Source episodes are marked `status='consolidated'` and decay faster.

**Archive:** Memories below eviction threshold move to `nodes_archive`. Not deleted — recoverable and auditable.

### 3.3 Retrieval Scoring

When the orchestrator queries memory, candidates are scored by a composite of four signals:

| Signal | Weight | Source | Description |
|--------|--------|--------|-------------|
| **Relevance** | 0.35 | Vector similarity | Cosine distance between query embedding and memory embedding |
| **Importance** | 0.25 | Stored + computed | Base importance × access reinforcement × citation score |
| **Recency** | 0.25 | Temporal decay | Exponential decay from last access time |
| **Structural** | 0.15 | Graph connectivity | Bonus for nodes connected to other high-scoring results |

Graph expansion follows retrieval: after top-K semantic matches are found, their graph neighbors (1-2 hops) are pulled in to provide structural context.

---

## 4. Database Schema

### 4.1 Core Tables

```sql
-- Configuration
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
PRAGMA cache_size = -65536;        -- 64MB page cache
PRAGMA foreign_keys = ON;

-- Schema version tracking
CREATE TABLE IF NOT EXISTS schema_meta (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
INSERT OR IGNORE INTO schema_meta VALUES ('schema_version', '1');
INSERT OR IGNORE INTO schema_meta VALUES ('embedding_model', 'nomic-embed-text-v1.5');
INSERT OR IGNORE INTO schema_meta VALUES ('embedding_dimensions', '768');

-- Memory nodes
CREATE TABLE IF NOT EXISTS nodes (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL CHECK (type IN ('episode', 'concept', 'procedure', 'reflection')),
    subtype TEXT,
    content TEXT NOT NULL,                -- Original text (never discard — needed for re-embedding)
    metadata JSON,                        -- Structured data (tags, actors, triggers, etc.)
    importance REAL DEFAULT 0.5 CHECK (importance BETWEEN 0.0 AND 1.0),
    decay_rate REAL NOT NULL,             -- Set by type at creation
    access_count INTEGER DEFAULT 0,
    times_reinforced INTEGER DEFAULT 0,
    status TEXT DEFAULT 'active' CHECK (status IN ('active', 'consolidated', 'superseded', 'archived')),
    embedding_model TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_accessed DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_reinforced DATETIME
);

-- Relationship edges
CREATE TABLE IF NOT EXISTS edges (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id TEXT NOT NULL,
    target_id TEXT NOT NULL,
    relation TEXT NOT NULL,               -- e.g., 'relates_to', 'learned_from', 'supersedes', etc.
    weight REAL DEFAULT 1.0,
    metadata JSON,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (source_id) REFERENCES nodes(id) ON DELETE CASCADE,
    FOREIGN KEY (target_id) REFERENCES nodes(id) ON DELETE CASCADE,
    UNIQUE (source_id, target_id, relation)
);

-- Archive for evicted memories
CREATE TABLE IF NOT EXISTS nodes_archive (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    subtype TEXT,
    content TEXT NOT NULL,
    metadata JSON,
    importance REAL,
    status TEXT,
    archive_reason TEXT CHECK (archive_reason IN ('decayed', 'superseded', 'redundant', 'capacity')),
    original_created_at DATETIME,
    archived_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Performance indexes
CREATE INDEX IF NOT EXISTS idx_nodes_type ON nodes(type);
CREATE INDEX IF NOT EXISTS idx_nodes_status ON nodes(status);
CREATE INDEX IF NOT EXISTS idx_nodes_type_status ON nodes(type, status);
CREATE INDEX IF NOT EXISTS idx_nodes_importance ON nodes(importance DESC);
CREATE INDEX IF NOT EXISTS idx_nodes_last_accessed ON nodes(last_accessed);
CREATE INDEX IF NOT EXISTS idx_edges_source ON edges(source_id);
CREATE INDEX IF NOT EXISTS idx_edges_target ON edges(target_id);
CREATE INDEX IF NOT EXISTS idx_edges_relation ON edges(relation);
```

### 4.2 Vector Table

```sql
-- Semantic embedding index (sqlite-vec)
CREATE VIRTUAL TABLE IF NOT EXISTS vec_nodes USING vec0(
    node_id TEXT,
    embedding float[768],                 -- nomic-embed-text native dimensionality
    distance_metric = 'cosine'
);
```

### 4.3 Edge Relation Types

| Relation | Meaning | Example |
|----------|---------|---------|
| `relates_to` | General association | concept:auth → relates_to → concept:jwt |
| `depends_on` | Structural dependency | concept:module_a → depends_on → concept:module_b |
| `learned_from` | Derived from experience | procedure:run_migrations → learned_from → episode:staging_incident |
| `resulted_in` | Causal outcome | episode:refactor → resulted_in → concept:composition_pattern |
| `supersedes` | Replaces older knowledge | concept:uses_pg16 → supersedes → concept:uses_pg15 |
| `blocks` | Prevents progress | concept:circular_dep → blocks → episode:refactor_attempt |
| `implements` | Realizes a concept | procedure:deploy_steps → implements → concept:deployment |
| `promoted_to` | Linked to global copy | concept:local → promoted_to → global:concept_id |

---

## 5. Scoping: Project and Global Memory

### 5.1 Storage Layout

```
~/.cerebro/
  config.toml                     # Global Cerebro configuration
  global.sqlite                   # Global memory store
  projects/
    <sha256_of_abs_path>.sqlite   # Project memory, indexed by directory hash
```

Each project's brain is initialized when the orchestrator is first invoked in that directory. The global brain is created on first use.

**Alternative:** `.cerebro/brain.sqlite` within each project directory (more visible, but risks accidental commits). Decision deferred to ADR-004.

### 5.2 Dual-Store Query Model

At session start, the orchestrator opens both stores:

```
1. Query project store  → results with weight 1.0
2. Query global store   → results with weight 0.7
3. Merge and deduplicate (cosine > 0.9 between results = duplicate)
4. For duplicates: project-specific version wins
5. Return top-K by weighted composite score
```

### 5.3 Promotion: Project → Global

**Automatic candidates** (no confirmation needed):
- User preferences with no project-specific references
- General tool/technology knowledge
- Communication and interaction patterns

**Require human confirmation:**
- Patterns observed across 2+ projects
- Technology knowledge that might be project-specific

**Never promote:**
- Project-specific architecture, entities, procedures, relationships

**Promotion flow:**
1. Identify candidate by criteria
2. Check for conflicts in global store (vector similarity)
3. Generalize content — strip project-specific details
4. Copy to global store with initial importance 0.5
5. Add provenance metadata (source project, date)
6. Link in project store: `original → promoted_to → global:<id>`

---

## 6. Orchestrator Integration

### 6.1 The Orchestrator as Sole Interface

Sub-agents **never** interact with Cerebro. The orchestrator:
1. **Recalls** relevant memory before dispatching work
2. **Injects** formatted context into sub-agent prompts
3. **Observes** sub-agent results
4. **Remembers** new knowledge from the interaction

### 6.2 Memory Injection Format

The orchestrator translates raw memories into authoritative sub-agent context:

```xml
<memory_context>
  <relevant_knowledge>
    - Project uses PostgreSQL 16 with pgvector extension
    - Module auth depends on: user, session, token
    - API follows REST conventions; spec in /docs/api.yaml
  </relevant_knowledge>
  <applicable_rules>
    - ALWAYS update /docs/api.yaml when modifying API endpoints
    - NEVER use raw SQL queries; use the query builder in /src/db/
    - Run `make lint` before considering code complete
  </applicable_rules>
  <past_issues>
    - On 2026-02-20, a similar change broke session middleware because
      the token format changed without updating the validation regex.
      Ensure token validation is consistent across modules.
  </past_issues>
</memory_context>
```

**Key:** Sub-agents don't know they're receiving memories. Procedures are presented as instructions. Knowledge is presented as facts. Episodes are presented as warnings.

### 6.3 Memory Budget

- **Hard limit:** ~10,000 tokens for memory injection per sub-agent call
- **Mandatory inclusions:** Active procedures whose trigger matches the current task type
- **Type balance:** ~30% concepts, 30% procedures, 30% episodes, 10% reflections
- **Prioritization:** By composite retrieval score, highest first

---

## 7. Embedding Strategy

### 7.1 Provider Abstraction

Embedding generation is a **pluggable provider** behind a simple interface:

```
embed(text: string) → float[N]
```

Cerebro is agnostic about where embeddings come from. The provider is configured, not hardcoded. The rest of the system (reconciliation, decay, graph traversal, retrieval scoring) is completely independent of the embedding source.

### 7.2 Provider Tiers

| Tier | Provider | Tradeoff |
|------|----------|----------|
| **Local** | Ollama (nomic-embed-text, mxbai-embed-large, etc.) | Free, offline, requires local model runtime |
| **API** | Voyage AI (Anthropic's endorsed provider), OpenAI, Cohere | Paid, requires internet + API key, zero local setup |
| **None** | — | Graph-only mode — no semantic search, only explicit edge traversal |

### 7.3 Configuration

```toml
# ~/.cerebro/config.toml

[embeddings]
provider = "ollama"                    # "ollama" | "voyage" | "openai" | "none"
model = "nomic-embed-text"             # Provider-specific model identifier
dimensions = 768                       # Must match the model's output

# Provider-specific settings
[embeddings.ollama]
base_url = "http://localhost:11434"    # Default Ollama endpoint

[embeddings.voyage]
# api_key from env: CEREBRO_VOYAGE_API_KEY
model = "voyage-3.5"                   # Or voyage-code-3 for code-heavy projects
dimensions = 1024

[embeddings.openai]
# api_key from env: CEREBRO_OPENAI_API_KEY
model = "text-embedding-3-small"
dimensions = 1536
```

API keys are never stored in config files. They are read from environment variables (`CEREBRO_VOYAGE_API_KEY`, `CEREBRO_OPENAI_API_KEY`, etc.) or from the system keychain.

Project-level config can override global config via a `[embeddings]` section in the project's brain metadata, enabling per-project provider choice.

### 7.4 Recommended Default

**nomic-embed-text-v1.5 via Ollama** is the recommended default provider:

| Property | Value |
|----------|-------|
| Dimensions | 768 |
| Context window | 8,192 tokens |
| Latency (M-series) | ~10ms per embedding |
| Model size | ~274 MB |
| Matryoshka support | 768, 512, 256, 128 dims |
| License | Apache 2.0 |

This is a recommendation, not a requirement. Users who prefer API-based providers or who don't want to run Ollama can configure an alternative at `cerebro init` time.

### 7.5 Embedding Flow

```
Embedding request
        │
        ▼
  Is configured provider available?
  /                    \
YES                     NO
 │                       │
 ▼                       ▼
Generate embedding    Store text with status='pending_embedding'
Store in vec_nodes    Queue for retry when provider becomes available
                      (graph-based recall still works for unembedded nodes)
```

### 7.6 Critical Constraint: No Mixed Vector Spaces

Vectors from different models are **incompatible**. Cosine similarity between a nomic-embed-text vector and an OpenAI vector is meaningless. Therefore:

- All vectors in a single `vec_nodes` table **must** come from the same model.
- The active model is recorded in `schema_meta` and on each node's `embedding_model` field.
- Switching providers triggers a **migration**, not a mix.

### 7.7 Provider Migration

When changing providers (e.g., Ollama → Voyage, or nomic-embed-text → a newer model):

1. Create new `vec_nodes_v2` virtual table with new dimensions
2. Background re-embed all active nodes using the new provider
3. Switch queries to new table
4. Drop old table
5. Update `schema_meta` with new model/dimensions

Estimated time for 100K nodes:
- Local (Ollama): ~15-20 minutes on M-series Mac
- API (Voyage AI/OpenAI): ~5-10 minutes (batched, network-bound), ~$0.04-$0.12 cost

For users in the Claude/Anthropic ecosystem, Voyage AI is the recommended API provider — it's Anthropic's official embedding partner. `voyage-code-3` is particularly relevant for agent brains focused on software engineering tasks.

The `embedding_model` field on each node tracks provenance. Nodes with a mismatched model are flagged for re-embedding.

---

## 8. Core Operations

### 8.1 Remember

Triggered after each orchestrator interaction.

```
Input: raw observation/fact/lesson from interaction
  │
  ▼
Extract memory candidates (LLM call)
  │
  ▼
For each candidate:
  ├── Generate embedding (via configured provider)
  ├── Vector search existing nodes (cosine > 0.85 threshold)
  ├── If near-match found → LLM reconciliation:
  │     ├── ADD:    Insert as new node + edges
  │     ├── UPDATE: Modify existing node content/metadata
  │     ├── DELETE: Mark existing as superseded, insert new
  │     └── NOOP:   Skip, optionally reinforce existing
  └── If no match → ADD: Insert new node + edges + vector
```

### 8.2 Recall

Triggered at the start of each task or sub-agent dispatch.

```
Input: task description / user request
  │
  ▼
Generate query embedding
  │
  ├── Vector search (top-K by cosine similarity)
  ├── Filter by status='active'
  ├── Score by composite (relevance × importance × recency × structural)
  │
  ▼
Graph expansion (1-2 hops from top results)
  │
  ▼
Procedural lookup (match procedure triggers against task type)
  │
  ▼
Merge results from project + global stores
  │
  ▼
Format into injection context (respect token budget)
```

### 8.3 Reflect

Triggered **opportunistically** or by explicit `cerebro consolidate` command. There is no background scheduler or daemon — consolidation piggybacks on existing operations.

**Trigger conditions (combination — any one is sufficient):**

| Trigger | Condition | When Checked |
|---------|-----------|--------------|
| Write threshold | N unconsolidated episodes accumulated (default: 50) | After each `remember` operation |
| Session boundary | Consolidation hasn't run since last session start | On `cerebro recall` (session start) |
| Time threshold | >24 hours since last consolidation AND >10 unconsolidated episodes | On `cerebro recall` (session start) |
| Explicit | User runs `cerebro consolidate` | Manual |

The time threshold is checked against a `last_consolidated_at` timestamp stored in `schema_meta` — no clock-watching daemon needed. It's evaluated lazily when the brain is already being used.

```
Check consolidation triggers (on recall or after remember)
  │
  ▼
If threshold met:
  │
  ▼
Select unconsolidated episodes (status='active', type='episode', not yet consolidated)
  │
  ▼
Cluster by semantic similarity and graph connectivity
  │
  ▼
For each cluster:
  ├── Present to LLM: "What are the key patterns, facts, and rules from these experiences?"
  ├── Create new concept/procedure/reflection nodes from LLM output
  ├── Link new nodes to source episodes via 'learned_from' edges
  └── Mark source episodes as status='consolidated'
  │
  ▼
Update schema_meta: last_consolidated_at = now()
```

Eviction (`gc`) follows the same opportunistic pattern — checked on session start, run if thresholds are met, or triggered explicitly via `cerebro gc`.

### 8.4 Forget

Triggered opportunistically (on session start if overdue) or by `cerebro gc`.

```
For all nodes where status='active':
  │
  ▼
Compute retrieval_score
  │
  ▼
If score < eviction_threshold (default 0.01):
  ├── Check graph connectivity (don't break bridges between high-value subgraphs)
  ├── Check if source for active procedures/concepts (don't evict if value not yet extracted)
  ├── If safe to evict:
  │     ├── Move to nodes_archive with archive_reason
  │     ├── Remove from vec_nodes
  │     └── Transfer unique edges to surviving neighbors (or archive)
  └── If not safe: skip (let it decay further)
```

### 8.5 Promote

Triggered on detection of cross-project patterns or by explicit user instruction.

```
Candidate identified (by type, content analysis, or cross-project pattern)
  │
  ▼
Generalize content (strip project-specific paths, names, etc.)
  │
  ▼
Check global store for conflicts (vector search)
  │
  ├── No conflict → Copy to global store (importance=0.5)
  ├── Conflict → LLM reconciliation (UPDATE global or NOOP)
  │
  ▼
Add 'promoted_to' edge in project store
```

---

## 9. Concurrency Model

**SQLite WAL mode** with orchestrator-as-sole-writer:

- The orchestrator is the only writer. No write contention by design.
- Multiple readers are supported (e.g., a monitoring/inspection tool can read while the orchestrator writes).
- `busy_timeout=5000` handles edge cases where a reader holds a lock during checkpointing.

This is the simplest viable concurrency model and is correct for the single-orchestrator use case.

---

## 10. Integration Model

Cerebro is implemented in **Go** (see ADR-005) and distributed as a single static binary with no runtime dependencies. It exposes three integration surfaces, in order of priority:

### 10.1 CLI Tool

The primary interface. The orchestrator (or user) invokes `cerebro` as a subprocess. Output is structured (JSON by default) for easy parsing.

| Command | Description |
|---------|-------------|
| `cerebro init` | Initialize brain.sqlite in the project or ~/.cerebro/projects/ |
| `cerebro remember <text>` | Ingest a memory (with reconciliation) |
| `cerebro recall <query>` | Semantic + graph recall, returns formatted context |
| `cerebro consolidate` | Run reflection pass on unconsolidated episodes |
| `cerebro gc` | Run eviction pass on decayed memories |
| `cerebro promote <node_id>` | Promote a project memory to global |
| `cerebro status` | Show brain stats (node count by type, health, embedding status) |
| `cerebro inspect <node_id>` | Show a specific node with its edges |
| `cerebro graph <node_id>` | Show the subgraph around a node (N hops) |
| `cerebro export [--format sqlite|sql]` | Dump brain (default: raw .sqlite copy) |
| `cerebro import <file>` | Import from a dump |

### 10.2 Go Library

For Go-based orchestrators, Cerebro can be imported directly as a package — no subprocess overhead.

```go
import "github.com/coetzeevs/cerebro/brain"

b, err := brain.Open("/path/to/project")

// Remember
err = b.Remember("The auth module uses JWT with RS256", brain.TypeConcept)

// Recall
ctx, err := b.Recall("authentication implementation", brain.RecallOpts{TopK: 10})

// Consolidate
err = b.Consolidate(brain.ConsolidateOpts{MaxEpisodes: 50})

// GC
err = b.GC(brain.GCOpts{EvictionThreshold: 0.01})
```

The CLI is a thin wrapper around this library — same code paths, same behavior.

### 10.3 gRPC Service (future)

Go's native protobuf/gRPC support enables a future local service mode where non-Go orchestrators (Python, TypeScript) can interact with Cerebro over a socket without shelling out to a CLI. This is not in scope for v1 but the internal package boundaries are designed to support it:

```
cerebro (binary)
  ├── cmd/cerebro/       # CLI entrypoint
  ├── brain/             # Core library (public Go API)
  ├── internal/store/    # SQLite + sqlite-vec operations
  ├── internal/embed/    # Embedding provider abstraction
  ├── internal/lifecycle/ # Decay, consolidation, eviction
  └── proto/             # gRPC service definitions (future)
```

The `brain/` package is the stable public API. Everything under `internal/` is implementation detail.

---

## 11. Scaling Characteristics

| Metric | 10K nodes | 50K nodes | 100K nodes | Notes |
|--------|-----------|-----------|------------|-------|
| DB file size | ~30-50 MB | ~150-250 MB | ~300-500 MB | Vectors dominate |
| Vector search (768-dim) | ~4ms | ~20ms | ~40ms | Brute-force, M-series |
| Graph traversal (3-hop) | <2ms | ~5ms | ~10ms | With indexes |
| Full recall pipeline | ~10ms | ~30ms | ~60ms | Search + graph + scoring |
| Memory per query | ~5 MB | ~20 MB | ~40 MB | Proportional to scan |

**Scaling limits:** Brute-force vector search remains interactive (<100ms) up to ~100K nodes. Beyond that, either reduce dimensions (Matryoshka 256-dim), use int8 quantization, or wait for sqlite-vec ANN support. Most project-scoped brains will stay well under 50K nodes.

---

## 12. Technology Decisions

| Decision | Choice | Rationale | ADR |
|----------|--------|-----------|-----|
| Storage engine | SQLite + sqlite-vec | Single-file, zero-infra, SQL-native graph+vector in one store | ADR-001 |
| Embedding strategy | Pluggable provider; nomic-embed-text-v1.5 via Ollama recommended | Provider-agnostic. Local default for zero-cost offline use. API providers supported. | ADR-002 |
| Memory lifecycle | Mem0 reconciliation + Generative Agents decay/reflection | Prevents unbounded growth. Type-aware lifecycle. | ADR-003 |
| Scoping model | Multi-file (project.sqlite + global.sqlite) | Follows git-config precedence. Portable per-project brains. | ADR-004 |
| Implementation language | Go | Type-safe, single-binary distribution, native protobuf/gRPC support, performant. | ADR-005 |

---

## 13. Resolved Questions

| # | Question | Decision | Rationale |
|---|----------|----------|-----------|
| 1 | Implementation language | **Go** | Type-safe, performant, single-binary distribution, protobuf/gRPC native, lower learning curve than Rust. See ADR-005. |
| 2 | Consolidation triggers | **Opportunistic combination** | Write threshold (50 episodes), session boundary, time threshold (24h) — all checked lazily, no scheduled jobs. See section 8.3. |
| 3 | Global memory initialization | **Starts empty** | Seed memories would be assumptions about the user's ecosystem. Global memory is populated organically through promotion from project brains. |
| 4 | Brain portability format | **Raw .sqlite copy (default), SQL text dump (secondary)** | Raw copy is most efficient. SQL text via `cerebro export --format sql` for cross-platform portability. Portability is not the primary directive. |
| 5 | Observability | **Deferred to future work** | Not part of v1 architecture. Tracked as a future addition. |

---

## 14. Future Work

- **gRPC service mode.** Local socket-based service for non-Go orchestrators. Internal boundaries are designed to support this (see section 10.3).
- **Observability.** Metrics export (node counts, recall latency, eviction rates) for monitoring orchestrator health.
- **UI/visualization.** Brain inspection tooling, graph visualization, dashboards.
- **Multi-user collaboration.** Sharing brains between team members.

---

## 15. What This Architecture Does NOT Cover

- **Sub-agent implementation.** Cerebro is storage + retrieval. How the orchestrator is built, how sub-agents are dispatched — that's the orchestrator's domain.
- **LLM selection.** Which LLM powers the orchestrator or the memory extraction/reconciliation calls.
- **Orchestrator design.** How the orchestrator decides what to remember, when to recall, or how to format context for sub-agents — that's orchestrator logic, not Cerebro's concern.
