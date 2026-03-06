# Agent Memory Architectures: Research for Cerebro

> **Date:** 2026-03-04
> **Context:** Research for Cerebro, a local-first orchestrator-only agent brain with project-scoped memory.

---

## 1. State of the Art in Agent Memory (2024-2026)

### 1.1 MemGPT / Letta

**Source:** Packer et al., "MemGPT: Towards LLMs as Operating Systems" (ICLR 2024). Commercialized as Letta.

**Core Insight:** Treat the LLM's context window like virtual memory in an OS. Page data between active context and external storage.

**Three-Tier Architecture:**

| Tier | OS Analogy | Characteristics | Cerebro Mapping |
|------|-----------|-----------------|-----------------|
| **Core Memory** | RAM / registers | Always in context. Editable by agent. System persona + user/session info. ~2-8KB. | Active context nodes loaded at session start |
| **Recall Memory** | Read-ahead buffer | Conversational history. Searchable by recency, keyword, embedding. Agent can search but not edit. | Episodic memory with temporal indexing |
| **Archival Memory** | Disk / cold storage | Unlimited external storage. Agent explicitly reads/writes. Vector similarity search. | Full node+vector store (bulk of brain.sqlite) |

**Key Design Decisions:**
1. **Agent-controlled memory editing:** Agent itself decides what to write using tool calls (`core_memory_append`, `archival_memory_insert`, etc.). The agent is the author of its own memory.
2. **Structured core memory blocks:** Named blocks (persona, human, project_context) with character limits. Forces active information management.
3. **Automatic recall vs explicit archival:** Conversation flows automatically into recall. Archival requires deliberate action.
4. **Heartbeat mechanism:** Agent can trigger additional processing cycles for multi-step memory operations.

**Limitations for Cerebro:**
- Server-based (requires running Letta server). Cerebro's local-file approach is simpler.
- Per-agent, not per-project. No native project vs global scoping.
- No graph relationships between memories -- purely vector + keyword search.

### 1.2 LangChain / LangGraph Memory

**Key Patterns (2025):**

1. **Checkpointed State (Short-term):** SQLite/Postgres-backed checkpointer persists full graph state at every super-step. Thread-scoped.
2. **Cross-thread Memory Store (Long-term):** Key-value storage scoped by namespace tuples. e.g., `("project", "cerebro", "decisions")`. Supports vector search over stored items.
3. **Memory Profiles / Schemas:** Structured types (`UserProfile`, `ProjectContext`) that agents update over time. Prevents bloat.
4. **Reflection-based memory formation:** Secondary LLM call processes conversation to extract memories, separating "doing work" from "remembering work."

**Key Takeaway:** The namespace-scoped Store maps directly to Cerebro's project/global scoping. Project memory = `("project", "<dir_hash>", ...)`. Global = `("global", ...)`.

### 1.3 Mem0

**Core Approach:** Automatic memory extraction and lifecycle management. Dual vector + graph store.

**The Four-Operation Reconciliation Model (most important finding):**

On every new piece of information, determine one of:
- **ADD:** New memory, no duplicates or conflicts. Store as new entry.
- **UPDATE:** Modifies existing memory. e.g., "user prefers Python" → "user prefers Python 3.12 specifically." Old version replaced.
- **DELETE:** New info contradicts existing. Old memory removed/superseded.
- **NOOP:** Already captured. No action.

This creates an explicit lifecycle where new information is always reconciled against existing knowledge, preventing unbounded growth.

**Graph Memory (late 2024):** Knowledge graph layer extracting entities and relationships. Structured traversal alongside vector similarity.

**Key Takeaway:** The four-operation model is the strongest pattern for memory lifecycle management. Validates Cerebro's dual vector+graph approach.

### 1.4 Key Academic Papers

**"Generative Agents" (Park et al., Stanford, UIST 2023):**
- **Memory stream:** Timestamped append-only log of observations.
- **Three-factor retrieval:** Scores memories by:
  - **Recency:** Exponential decay based on time since last access
  - **Importance:** LLM-rated 1-10 at creation time
  - **Relevance:** Embedding cosine similarity to current query
- **Reflection:** Periodically generates higher-order observations from clusters of memories. Reflections become memories that participate in further reflection.

**"Cognitive Architectures for Language Agents" (CoALA, Sumers et al., 2024):**
- Four memory types from cognitive science: working, episodic, semantic, procedural
- Most agent frameworks only implement working + one external store
- Richer architectures need all four operating together

**"Reflexion" (Shinn et al., NeurIPS 2023):**
- Agents maintain memory of past failures and reflections on why they failed
- Storing the *reflection* on failure is more useful than storing the failure itself
- Directly applicable to procedural memory

---

## 2. Memory Taxonomy for Agents

### 2.1 Episodic Memory (What Happened)

Records of specific events with temporal context. Timestamped, includes context (who, what, why, outcome). Subject to fast decay. Highest volume type.

**Examples:**
- "User asked to refactor auth module. Approach X failed (circular dependency). Approach Y succeeded."
- "Deployment failed because DATABASE_URL was missing."

**Graph mapping:**
```
Node: {type: "episode", subtype: "interaction", metadata: {content, outcome, actors, importance}}
Edges: episode -> relates_to -> concept, episode -> resulted_in -> decision
```

### 2.2 Semantic Memory (What I Know)

Facts, concepts, relationships. Accumulated from multiple episodes. More stable -- persists until contradicted.

**Examples:**
- "Project uses PostgreSQL 16 with pgvector."
- "User prefers explicit error handling."
- "Module A depends on Module B."

**Graph mapping:**
```
Node: {type: "concept", subtype: "entity", metadata: {content, confidence, last_verified}}
Edges: concept -> instance_of -> concept, concept -> depends_on -> concept
```

### 2.3 Procedural Memory (How To Do Things)

Learned procedures, patterns, failure-avoidance rules. Derived from episodic experience through reflection. High value, low volume.

**Examples:**
- "When deploying to staging, always run migrations first."
- "This user prefers to review diffs before any push."
- "When modifying the API, always update the OpenAPI spec."

**Graph mapping:**
```
Node: {type: "procedure", subtype: "rule", metadata: {content, trigger, action, reason, times_applied, success_rate}}
Edges: procedure -> learned_from -> episode, procedure -> applies_to -> concept
```

### 2.4 Reflection Memory (What I've Concluded)

Higher-order observations from analyzing memory clusters. Meta-cognition stored as memory.

**Examples:**
- "Three bugs this week from inconsistent date formatting -- systemic issue, not individual mistakes."
- "User consistently pushes back on abstractions -- strong preference, not one-time."

### 2.5 Recommended Type Hierarchy

```
memory_types:
  episode:     [interaction, incident, discovery, decision_point]
  concept:     [entity, relationship, preference, constraint]
  procedure:   [rule, pattern, anti_pattern, workflow]
  reflection:  [summary, lesson, insight, open_question]
```

---

## 3. Memory Lifecycle Patterns

### 3.1 Decay / Forgetting

**Exponential Temporal Decay (Generative Agents):**
```
recency_score = e^(-lambda * hours_since_access)
```

**Type-Dependent Decay Rates:**

| Memory Type | Decay Rate | Half-Life | Rationale |
|------------|-----------|-----------|-----------|
| Episodes | Fast (0.15) | ~1-2 weeks | Value extracted into concepts/procedures |
| Concepts | Slow (0.02) | ~2-3 months | Facts persist until contradicted |
| Procedures | Very slow (0.005) | ~6+ months | Rules retain value unless contradicted |
| Reflections | Medium (0.05) | ~3-4 weeks | May need refreshing |

**Schema additions:**
```sql
last_accessed DATETIME
access_count INTEGER DEFAULT 0
importance REAL DEFAULT 0.5      -- 0.0 to 1.0, LLM-assessed
decay_rate REAL DEFAULT 0.1      -- type-dependent
```

**Composite retrieval score:**
```
retrieval_score = importance * (1 + ln(1 + access_count)) * exp(-decay_rate * hours_since_access)
```

### 3.2 Consolidation

**Real-time reconciliation (Mem0 model):** On insert, vector search against existing nodes. If cosine > 0.85, trigger LLM reconciliation using ADD/UPDATE/DELETE/NOOP.

**Periodic reflection (Generative Agents model):** After N sessions or on-demand (`cerebro consolidate`), process unconsolidated episodes into semantic/procedural nodes.

### 3.3 Reinforcement

Three signals combined:
1. **Access-based:** Each use increments importance by delta (0.05)
2. **Outcome-based:** Successful task → reinforce used memories. Failed task → create anti-patterns.
3. **Citation-based:** Nodes with high in-degree (many edges pointing to them) get importance bonus.

```
effective_importance = base_importance*0.3 + access_reinforcement*0.3 + citation_score*0.2 + recency*0.2
```

### 3.4 Eviction

**Priority order:**
1. **Contradiction:** Superseded by newer verified information. Mark, don't delete.
2. **Decay:** retrieval_score below threshold. Check graph connectivity first (don't break bridges).
3. **Redundancy:** Cosine > 0.95 to higher-importance node. Absorb unique edges.
4. **Capacity:** If node count exceeds budget, evict lowest-scoring.

**Archive, don't delete:**
```sql
CREATE TABLE nodes_archive (
    ...original columns...,
    archive_reason TEXT,  -- 'decayed', 'superseded', 'redundant', 'capacity'
    archived_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

---

## 4. Orchestrator-Only Access Patterns

### 4.1 Existing Systems

- **CrewAI:** Shared memory, but crew manager controls what each agent receives. All-or-nothing sharing.
- **AutoGen:** Per-agent memory, no orchestrator-level memory. No cross-session learning at orchestrator level.
- **LangGraph:** Orchestrator node reads from Store, decides what to include in state passed to sub-agents. **Closest to Cerebro's model.**

### 4.2 Recommended Pattern for Cerebro

```
1. User request → Orchestrator
2. Orchestrator queries Cerebro:
   a. Semantic search: relevant to this request
   b. Graph traversal: connected to relevant concepts
   c. Procedural lookup: rules for this task type
3. Orchestrator constructs sub-agent prompt:
   - Task description
   - Relevant context from memory
   - Applicable procedures as instructions
   - Past failure warnings
4. Sub-agent executes with injected context
5. Orchestrator observes result
6. Orchestrator updates Cerebro:
   - New episode
   - Updated concepts
   - New/reinforced procedures
```

### 4.3 Context Injection Format

```xml
<memory_context>
  <relevant_knowledge>
    - Project uses PostgreSQL 16 with pgvector
    - Module auth depends on: user, session, token
  </relevant_knowledge>
  <applicable_rules>
    - ALWAYS update /docs/api.yaml when modifying API endpoints
    - Run `make lint` before considering code complete
  </applicable_rules>
  <past_issues>
    - Similar change broke session middleware on 2026-02-20
      because token format changed without updating validation regex
  </past_issues>
</memory_context>
```

**Key principle:** Sub-agents should not know they're receiving "memories." Present as authoritative context and instructions.

### 4.4 Memory Budget Per Sub-Agent Call

- **Hard limit:** ~2,000 tokens for memory injection
- **Mandatory:** Active procedures matching current task type
- **Type balance:** ~30% episodes, 30% concepts, 30% procedures, 10% reflections

---

## 5. Project-Scoped vs. Global Memory

### 5.1 Storage Architecture

```
~/.cerebro/
  global.sqlite              # Global memory
  projects/
    <dir_hash_1>.sqlite      # Project memory for /path/to/project-a
    <dir_hash_2>.sqlite      # Project memory for /path/to/project-b
```

Or alternatively, `.cerebro/brain.sqlite` within each project directory.

**Follows git's configuration model:** `~/.gitconfig` (global) < `.git/config` (project-local).

### 5.2 Promotion Criteria: Project → Global

**Automatic candidates:**
- User preferences and working style (no project-specific references)
- General tool/technology knowledge
- Communication and interaction patterns

**Never promote:**
- Project-specific architecture, entities, procedures, relationships

**Semi-automatic (require confirmation):**
- Patterns observed across 2+ projects
- Technology knowledge that might be project-specific

### 5.3 Promotion Mechanism

1. Identify candidate by criteria
2. Check for conflicts in global store (vector search)
3. Copy with generalization (strip project-specific details)
4. Set initial global importance to 0.5
5. Add provenance metadata
6. Link in project store: `original → promoted_to → global:<id>`

### 5.4 Retrieval Precedence

1. Query project store (weight 1.0)
2. Query global store (weight 0.7)
3. Merge, deduplicate (cosine > 0.9 = duplicate)
4. Duplicates: project version wins
5. Return top-K by weighted composite score

---

## 6. Core Operations Summary

| Operation | Trigger | Description |
|-----------|---------|-------------|
| **Remember** | After each interaction | Extract memories; reconcile via ADD/UPDATE/DELETE/NOOP |
| **Recall** | At task/sub-agent dispatch start | Semantic search + graph traversal + procedural lookup |
| **Reflect** | Periodic or `cerebro consolidate` | Process episodes into semantic/procedural nodes |
| **Reinforce** | After successful task | Increase importance of contributing memories |
| **Forget** | Periodic or `cerebro gc` | Archive below-threshold memories |
| **Promote** | Cross-project pattern detection | Copy generalized memory to global store |
| **Inject** | Sub-agent dispatch | Format memories as structured context |

---

## 7. Key Design Principles

1. **Agent-authored memory:** Orchestrator LLM decides what to remember, consolidate, forget. Cerebro provides the machinery.
2. **Reconcile, don't append:** Every new memory compared against existing knowledge (Mem0 model).
3. **Type-aware lifecycle:** Episodes decay fast → consolidate into concepts/procedures. Procedures reinforced by use.
4. **Project-first, global-second:** Project memory takes precedence. Promotion is deliberate. Follows git config model.
5. **Orchestrator is sole interface:** Sub-agents never touch Cerebro. They receive pre-formatted context.
6. **Multi-signal retrieval:** Combines vector similarity, graph traversal, recency, and importance.

---

## References

1. Packer et al. (2024). "MemGPT: Towards LLMs as Operating Systems." ICLR 2024.
2. Park et al. (2023). "Generative Agents: Interactive Simulacra of Human Behavior." UIST 2023.
3. Sumers et al. (2024). "Cognitive Architectures for Language Agents." (CoALA)
4. Shinn et al. (2023). "Reflexion: Language Agents with Verbal Reinforcement Learning." NeurIPS 2023.
5. Zhang et al. (2024). "A Survey on the Memory Mechanism of LLM-based Agents."
6. Letta Documentation: https://docs.letta.com/
7. Mem0 Documentation: https://docs.mem0.ai/
8. LangGraph Documentation: https://langchain-ai.github.io/langgraph/
