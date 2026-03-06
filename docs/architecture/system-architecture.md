# Cerebro: System Architecture

> **Version:** 0.2.0 — Draft
> **Date:** 2026-03-06
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
│                       CLAUDE CODE SESSION                        │
│                    (cognition layer — the agent)                  │
├──────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌─────────────┐  ┌──────────────┐  ┌────────────────────────┐  │
│  │  /recall     │  │  /remember   │  │     /consolidate       │  │
│  │  skill       │  │  skill       │  │     skill              │  │
│  │             │  │             │  │                        │  │
│  │ query brain │  │ reconcile +  │  │ synthesize episodes    │  │
│  │ for context │  │ store memory │  │ → concepts/procedures  │  │
│  └──────┬──────┘  └──────┬───────┘  └────────┬───────────────┘  │
│         │                │                   │                   │
│  ┌──────┴────────────────┴───────────────────┴───────────────┐  │
│  │                   HOOKS (automatic)                        │  │
│  │  SessionStart → prime    PreCompact → save    End → gc    │  │
│  └──────────────────────────┬────────────────────────────────┘  │
└─────────────────────────────┼────────────────────────────────────┘
                              │ CLI calls
                              ▼
┌──────────────────────────────────────────────────────────────────┐
│                     CEREBRO CLI (Go binary)                      │
│                   (storage layer — data infrastructure)           │
│                                                                  │
│  ┌────────────┐  ┌────────────┐  ┌────────────┐  ┌───────────┐ │
│  │  Retrieval  │  │  Storage   │  │ Lifecycle  │  │ Embedding │ │
│  │  search +   │  │  add/upd/  │  │ decay,     │  │ Provider  │ │
│  │  graph      │  │  supersede │  │ scoring,   │  │ (pluggable│ │
│  │  scoring    │  │  edges     │  │ evict      │  │  Ollama/  │ │
│  │             │  │            │  │            │  │  Voyage)  │ │
│  └──────┬──────┘  └─────┬──────┘  └─────┬──────┘  └────┬──────┘ │
│         └───────────────┴──────────────┴───────────────┘        │
│                              │                                   │
│                              ▼                                   │
│                    ┌───────────────────┐                         │
│                    │   brain.sqlite    │                         │
│                    │                   │                         │
│                    │  nodes (graph)    │                         │
│                    │  edges (rels)     │                         │
│                    │  vec_nodes (vec)  │                         │
│                    │  nodes_archive    │                         │
│                    └───────────────────┘                         │
└──────────────────────────────────────────────────────────────────┘
```

**Key separation:** Claude (the agent running in Claude Code) handles all reasoning — what to remember, how to reconcile conflicts, when to consolidate. Cerebro handles all data operations — embedding, storage, search, scoring, decay, eviction. Cerebro has no LLM dependency. See [ADR-006](../adrs/ADR-006-claude-code-integration-pattern.md) for the rationale.

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

**Ingest:** Raw information enters from the agent's observations during a session.

**Reconcile (Mem0 protocol, caller-driven):** The calling agent searches for similar existing nodes (`cerebro search --threshold 0.7`) and reasons about each candidate. The protocol follows Mem0's four operations, but the decision-maker is the agent (Claude), not an internal LLM:
- **ADD** — genuinely new information, no overlap
- **UPDATE** — refines or extends an existing memory
- **DELETE** — contradicts an existing memory (old is superseded)
- **NOOP** — already captured, skip

See section 8.1 for the detailed caller-driven flow and [ADR-006](../adrs/ADR-006-claude-code-integration-pattern.md) for the rationale.

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

## 6. Claude Code Integration

Cerebro's primary integration target is **Claude Code** — Anthropic's agentic coding CLI. See [ADR-006](../adrs/ADR-006-claude-code-integration-pattern.md) for the full rationale.

### 6.1 Integration Mechanisms

Claude Code offers three extension points. We use all three for different purposes:

| Mechanism | Token Cost | Automatic? | Role in Cerebro |
|-----------|-----------|-----------|-----------------|
| **Hooks** | Zero (execute outside context) | Yes — lifecycle events | Session prime, pre-compaction save, session-end GC |
| **Skills** | ~100-200 tokens (description only); full content on invoke | Manual `/command` or Claude-auto-invoked | `/remember`, `/recall`, `/consolidate` |
| **CLAUDE.md** | ~200-400 tokens (always loaded) | Always | Behavioral rules for when/how to use memory |

**Why not MCP:** MCP tool definitions load into the context window on every turn. With 6-10 cerebro tools, that's several hundred tokens consumed permanently regardless of whether memory operations are needed. Hooks + skills are the right tradeoff for invisible infrastructure.

### 6.2 Session Lifecycle

```
┌─ SESSION START ──────────────────────────────────────────────────┐
│  SessionStart hook → cerebro recall --prime --format md          │
│  stdout → injected into Claude's context automatically           │
│  Claude begins session with project memory loaded                │
└──────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─ DURING WORK ────────────────────────────────────────────────────┐
│  /remember skill — Claude reconciles and stores new memories     │
│  /recall skill   — on-demand retrieval for specific topics       │
│  CLAUDE.md       — rules for when to remember/recall             │
└──────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─ PRE-COMPACTION ─────────────────────────────────────────────────┐
│  PreCompact hook → injects reminder to persist critical context  │
│  Claude uses /remember to save key information before compaction │
└──────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─ SESSION END ────────────────────────────────────────────────────┐
│  SessionEnd hook → cerebro gc --threshold 0.01 --quiet           │
│  Opportunistic eviction of decayed memories                      │
└──────────────────────────────────────────────────────────────────┘
```

### 6.3 Agent-Managed Memory (Model B)

Claude (the agent) handles all reasoning about memory. Cerebro handles all data operations:

| Concern | Owner | Examples |
|---------|-------|---------|
| **What** to remember | Claude | "This architectural constraint is important" |
| **How** to reconcile | Claude | "This updates existing node X" vs "This is new" |
| **When** to consolidate | Claude | "These 5 episodes form a pattern" |
| **What** importance to assign | Claude | "This is critical (0.9)" vs "minor note (0.3)" |
| **Storage** operations | Cerebro | add, update, supersede, search, edge creation |
| **Embedding** generation | Cerebro | Vector encoding via pluggable provider |
| **Scoring** and ranking | Cerebro | Composite retrieval score computation |
| **Decay** and eviction | Cerebro | Time-based decay, threshold eviction, archival |

This model is chosen because:
1. **Zero incremental cost** — Claude Code Max subscription covers all LLM usage. No separate API billing.
2. **Best reasoning quality** — Claude has full session context when reasoning about memory, far richer than a separate LLM call.
3. **Agent autonomy** — The agent manages its own brain, consistent with the cognitive metaphor.

### 6.4 Sub-Agent Context Injection

Sub-agents (launched via Claude Code's Task tool) **never** interact with Cerebro directly. Claude:
1. **Recalls** relevant memory before dispatching sub-agents
2. **Injects** formatted context into the sub-agent prompt
3. **Observes** sub-agent results
4. **Remembers** new knowledge from the interaction

Sub-agents receive memories as authoritative context — they don't know the information comes from a memory system:

```xml
<memory_context>
  <relevant_knowledge>
    - Project uses PostgreSQL 16 with pgvector extension
    - Module auth depends on: user, session, token
  </relevant_knowledge>
  <applicable_rules>
    - ALWAYS update /docs/api.yaml when modifying API endpoints
    - Run `make lint` before considering code complete
  </applicable_rules>
  <past_issues>
    - Similar change broke session middleware on 2026-02-20 because
      the token format changed without updating the validation regex.
  </past_issues>
</memory_context>
```

### 6.5 Memory Budget

In the agent-managed model, memory budgets are **guidelines, not hard limits**. Claude decides how much context to inject based on the situation — a complex architectural task may warrant 30+ memories, while a simple typo fix may need none.

**Session prime (SessionStart hook):**
- Default: `cerebro recall --prime --limit 20 --format md`
- The `--limit` flag is a CLI default, configurable in `~/.cerebro/config.toml`
- Claude can request more context via `/recall` during the session if the prime was insufficient

**Sub-agent injection:**
- Claude judges how much memory context each sub-agent needs
- Guideline: ~2,000-4,000 tokens for focused tasks, more for complex architectural work
- No hard cap enforced by Cerebro — the agent manages its own token budget

**Prioritization (applied by `cerebro recall`):**
- Results ranked by composite retrieval score (section 3.3)
- Active procedures whose triggers match the query are boosted
- Type balance emerges naturally from scoring, not from rigid quotas

**Configuration:**
```toml
# ~/.cerebro/config.toml
[recall]
default_limit = 20           # Default --limit for recall commands
prime_limit = 20              # Default --limit for --prime flag
```

These defaults can be overridden on any CLI call via `--limit N`.

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

In the agent-managed model (section 6.3), core operations split into two categories:
- **Caller-driven** — Claude orchestrates the workflow using Cerebro's building-block commands
- **Cerebro-internal** — Cerebro handles autonomously (scoring, decay, eviction)

### 8.1 Remember (caller-driven)

Triggered by Claude via the `/remember` skill during a session, or proactively when Claude identifies something worth persisting.

```
Claude identifies something worth remembering
  │
  ▼
Step 1: Claude formulates memory content and classifies type
        (episode, concept, procedure, reflection)
  │
  ▼
Step 2: cerebro search "<content>" --limit 5 --threshold 0.7
        (Cerebro handles: embedding query, vector search, scoring)
  │
  ▼
Step 3: Claude reviews results and reasons about reconciliation:
  │
  ├── No matches → ADD
  │     cerebro add --type <type> --importance <0-1> "<content>"
  │
  ├── Match refines existing → UPDATE
  │     cerebro update <id> --content "<refined content>"
  │
  ├── Match contradicts existing → SUPERSEDE
  │     cerebro supersede <old_id> --type <type> --importance <0-1> "<new>"
  │
  └── Already captured → NOOP (optionally reinforce)
        cerebro reinforce <id>
  │
  ▼
Step 4: Claude creates edges for relationships
        cerebro edge <source_id> <target_id> <relation>
```

The reconciliation protocol (ADD/UPDATE/DELETE/NOOP from the Mem0 model) is preserved — only the decision-maker changes. Claude reasons about reconciliation with full session context instead of a separate LLM call with a narrow prompt.

### 8.2 Recall (Cerebro-internal + caller-invoked)

Triggered automatically at session start (via hook) or on-demand (via `/recall` skill).

```
Input: query text (from hook --prime flag or skill arguments)
  │
  ▼
cerebro recall "<query>" --limit N --format md
  │  ┌─────────────────────────────────────────────────┐
  │  │  Cerebro handles internally:                     │
  │  │  1. Generate query embedding                     │
  │  │  2. Vector search (top-K by cosine similarity)   │
  │  │  3. Filter by status='active'                    │
  │  │  4. Compute composite score:                     │
  │  │     relevance(0.35) × importance(0.25) ×         │
  │  │     recency(0.25) × structural(0.15)             │
  │  │  5. Graph expansion (1-2 hops from top results)  │
  │  │  6. Procedural lookup (trigger matching)          │
  │  │  7. Merge project + global stores                │
  │  │  8. Format output (respect token budget)         │
  │  └─────────────────────────────────────────────────┘
  │
  ▼
Output: formatted memory context (markdown or JSON)
```

The `--prime` flag (used by SessionStart hook) returns a curated selection optimized for session startup: high-importance active memories, recently accessed context, and active procedures.

### 8.3 Consolidate (caller-driven)

Triggered by Claude via the `/consolidate` skill when episode count warrants it, or when the user requests it. Unlike v0.1.0 which described internal LLM-driven consolidation, this is now a Claude-orchestrated process.

```
Claude invokes /consolidate
  │
  ▼
Step 1: cerebro list --type episode --status active --format json
        → returns unconsolidated episodes
  │
  ▼
Step 2: Claude clusters episodes by theme/topic and synthesizes:
        "These 5 episodes about auth all show the same pattern..."
  │
  ▼
Step 3: For each synthesized insight:
  ├── cerebro add --type concept --importance 0.7 "<synthesized fact>"
  ├── cerebro add --type procedure --importance 0.8 "<learned rule>"
  └── cerebro add --type reflection --importance 0.6 "<meta observation>"
  │
  ▼
Step 4: Link and mark
  ├── cerebro edge <new_id> <episode_id> learned_from
  └── cerebro mark-consolidated <episode_id> [<episode_id>...]
```

**When to consolidate:** Claude can check `cerebro stats` for unconsolidated episode count. CLAUDE.md instructions guide Claude to consolidate when episode count exceeds ~30-50 or at natural stopping points. The `--prime` output from session start can include a note if consolidation is overdue.

### 8.4 Forget (Cerebro-internal)

Triggered automatically by `SessionEnd` hook or explicitly by `cerebro gc`.

```
cerebro gc [--threshold 0.01] [--dry-run]
  │
  ▼
For all nodes where status='active':
  │
  ▼
Compute retrieval_score (composite of all four signals)
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

Eviction is fully automated — no LLM reasoning needed. It's pure data operations based on computed scores and graph analysis.

### 8.5 Promote (caller-driven)

Triggered by Claude when it identifies knowledge that transcends the current project.

```
Claude identifies cross-project knowledge
  │
  ▼
Step 1: Claude generalizes content (strips project-specific details)
  │
  ▼
Step 2: cerebro search --global "<generalized content>" --limit 3
        (check for conflicts in global store)
  │
  ▼
Step 3: Claude decides:
  ├── No conflict → cerebro promote <id> --content "<generalized>"
  ├── Conflict → Claude reconciles (update global or skip)
  └── Already global → NOOP
```

---

## 9. Concurrency Model

**SQLite WAL mode** with orchestrator-as-sole-writer:

- The orchestrator is the only writer. No write contention by design.
- Multiple readers are supported (e.g., a monitoring/inspection tool can read while the orchestrator writes).
- `busy_timeout=5000` handles edge cases where a reader holds a lock during checkpointing.

This is the simplest viable concurrency model and is correct for the single-orchestrator use case.

---

## 10. Implementation Model

Cerebro is implemented in **Go** (see [ADR-005](../adrs/ADR-005-go-as-implementation-language.md)) and distributed as a single static binary with no runtime dependencies (beyond CGO for sqlite-vec).

### 10.1 CLI Tool — Building Blocks

The CLI exposes **low-level building-block commands** that Claude orchestrates into higher-level workflows. This is a deliberate design choice: Cerebro does not have a monolithic `remember` command that internally runs reconciliation — instead, Claude performs the reasoning and calls the appropriate primitives.

**Storage commands:**

| Command | Purpose | Returns |
|---------|---------|---------|
| `cerebro init` | Initialize project brain | Status |
| `cerebro add --type <type> --importance <0-1> <content>` | Store a new memory node | Node ID |
| `cerebro update <id> --content <text> [--importance <0-1>]` | Modify existing node | Status |
| `cerebro supersede <old_id> --type <type> --importance <0-1> <content>` | Mark old as superseded, store new with `supersedes` edge | New node ID |
| `cerebro reinforce <id>` | Increment access_count, update last_accessed | Status |
| `cerebro edge <source_id> <target_id> <relation>` | Create a relationship edge | Edge ID |
| `cerebro mark-consolidated <id> [<id>...]` | Set episodes to `status='consolidated'` | Status |

**Query commands:**

| Command | Purpose | Returns |
|---------|---------|---------|
| `cerebro search <query> [--type <type>] [--limit N] [--threshold 0.7]` | Vector + graph similarity search | Scored node list |
| `cerebro recall <query> [--limit N] [--format md\|json] [--prime]` | Full composite-scored retrieval | Formatted memory context |
| `cerebro get <id>` | Retrieve a specific node with edges | Node detail |
| `cerebro list [--type <type>] [--status <status>] [--since <date>]` | List nodes by filter | Node list |

**Lifecycle commands:**

| Command | Purpose | Returns |
|---------|---------|---------|
| `cerebro gc [--threshold 0.01] [--dry-run]` | Evict decayed memories to archive | Eviction report |
| `cerebro stats` | Brain health metrics | Stats report |
| `cerebro export [--format sqlite\|sql\|json]` | Dump brain to portable format | File path |
| `cerebro import <file>` | Import from a dump | Status |

**Global commands:**

| Command | Purpose |
|---------|---------|
| `cerebro promote <id> --content <generalized_content>` | Copy to global store with generalized content |
| `cerebro recall --global <query>` | Query global store alongside project store |

All commands support `--format json` for structured output (consumed by hooks/skills) and default to human-readable markdown. The `--quiet` flag suppresses non-essential output (for hooks).

### 10.2 Go Library

For direct Go integration, Cerebro can be imported as a package:

```go
import "github.com/coetzeevs/cerebro/brain"

b, err := brain.Open("/path/to/project")

// Low-level storage operations
id, err := b.Add("The auth module uses JWT", brain.TypeConcept, brain.WithImportance(0.8))
err = b.Update(id, brain.WithContent("The auth module uses JWT with RS256"))
err = b.Reinforce(id)

// Search and recall
results, err := b.Search("authentication", brain.SearchOpts{Limit: 5, Threshold: 0.7})
ctx, err := b.Recall("authentication", brain.RecallOpts{TopK: 10, Format: "md"})

// Lifecycle
report, err := b.GC(brain.GCOpts{EvictionThreshold: 0.01})
stats, err := b.Stats()
```

The CLI is a thin wrapper around this library — same code paths, same behavior.

### 10.3 Project Structure

```
cerebro/
  cmd/cerebro/            # CLI entrypoint (thin wrapper)
  brain/                  # Public Go API (Brain type, operations)
  internal/
    store/                # SQLite + sqlite-vec operations
    embed/                # Embedding provider abstraction
      ollama/             # Ollama provider
      voyage/             # Voyage AI provider
      noop/               # No-op provider (graph-only mode)
    lifecycle/            # Decay scoring, eviction logic
    graph/                # Graph traversal, edge management
  proto/                  # gRPC service definitions (future)
  docs/                   # Architecture docs (this document)
```

The `brain/` package is the stable public API. `internal/` packages are implementation details. Note: there is no `internal/reconcile/` package — reconciliation reasoning lives in Claude's skills, not in Go code.

### 10.4 gRPC Service (future)

Go's native protobuf/gRPC support enables a future local service mode for non-Claude-Code orchestrators. Not in scope for v1, but internal package boundaries support it.

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
| Memory lifecycle | Mem0 reconciliation protocol + Generative Agents decay/reflection | Prevents unbounded growth. Type-aware lifecycle. Reconciliation is caller-driven. | ADR-003 |
| Scoping model | Multi-file (project.sqlite + global.sqlite) | Follows git-config precedence. Portable per-project brains. | ADR-004 |
| Implementation language | Go | Type-safe, single-binary distribution, native protobuf/gRPC support, performant. | ADR-005 |
| Integration pattern | Claude Code hooks + skills + CLAUDE.md (agent-managed memory) | Zero incremental LLM cost (Max subscription). Agent controls its own brain. Minimal token overhead. | ADR-006 |

---

## 13. Resolved Questions

| # | Question | Decision | Rationale |
|---|----------|----------|-----------|
| 1 | Implementation language | **Go** | Type-safe, performant, single-binary distribution, protobuf/gRPC native, lower learning curve than Rust. See ADR-005. |
| 2 | Consolidation triggers | **Claude-driven via `/consolidate` skill** | Claude decides when to consolidate based on episode count and natural stopping points. `cerebro stats` reports unconsolidated count. No background scheduler. |
| 3 | Global memory initialization | **Starts empty** | Seed memories would be assumptions about the user's ecosystem. Global memory is populated organically through promotion from project brains. |
| 4 | Brain portability format | **Raw .sqlite copy (default), SQL text dump (secondary)** | Raw copy is most efficient. SQL text via `cerebro export --format sql` for cross-platform portability. Portability is not the primary directive. |
| 5 | Observability | **Deferred to future work** | Not part of v1 architecture. Tracked as a future addition. |
| 6 | LLM for reconciliation/consolidation | **Claude (the calling agent) handles all reasoning** | Max subscription covers usage. Agent has richest context for decisions. Cerebro has no LLM dependency. See ADR-006. |
| 7 | Integration mechanism | **Hooks + skills + CLAUDE.md (not MCP)** | MCP tool definitions consume context tokens permanently. Hooks are zero-cost. Skills load only on invoke. See ADR-006. |

---

## 14. Future Work

- **gRPC service mode.** Local socket-based service for non-Claude-Code orchestrators. Internal boundaries support this (see section 10.4).
- **Observability.** Metrics export (node counts, recall latency, eviction rates) for monitoring brain health.
- **UI/visualization.** Brain inspection tooling, graph visualization, dashboards.
- **Multi-orchestrator support.** Adding an internal LLM provider so cerebro can operate autonomously with orchestrators that can't perform reconciliation reasoning (non-Claude-Code agents).
- **`UserPromptSubmit` hook augmentation.** Optionally augment every prompt with lightweight cerebro context (e.g., active procedures). Deferred due to latency concerns.

---

## 15. What This Architecture Does NOT Cover

- **Sub-agent implementation.** Cerebro is storage + retrieval. How sub-agents are dispatched — that's the orchestrator's domain.
- **Orchestrator design beyond Claude Code.** The integration pattern (section 6) is Claude Code-specific. Other orchestrators would need their own integration layer, though the CLI is universal.
- **LLM model selection.** Which Claude model version is used in Claude Code sessions is outside Cerebro's control.
- **Skill prompt engineering.** The exact prompts in `/remember`, `/recall`, `/consolidate` skills will evolve through use. This architecture defines the protocol; skill prompts are implementation details.
