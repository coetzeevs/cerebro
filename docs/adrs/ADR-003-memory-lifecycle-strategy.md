# ADR-003: Memory Lifecycle and Eviction Strategy

## Status
Proposed

## Context

The original architecture musing described an append-only memory store with no mechanism for forgetting, consolidation, or lifecycle management. Without these, the brain:
- Grows unboundedly, degrading search quality as irrelevant old memories compete with relevant ones
- Accumulates duplicates and contradictions
- Never distills raw experience into reusable knowledge
- Eventually exceeds SQLite brute-force search performance limits

We need a memory lifecycle that mirrors how human memory works: ingest, reconcile, reinforce, consolidate, and forget.

## Decision

Adopt a hybrid lifecycle strategy combining:
1. **Mem0's four-operation reconciliation model** (ADD/UPDATE/DELETE/NOOP) for write-time deduplication and conflict resolution
2. **Generative Agents' three-factor retrieval scoring** (recency × importance × relevance) for read-time ranking
3. **Type-dependent decay rates** for different memory categories
4. **Periodic consolidation** to distill episodes into higher-order knowledge
5. **Threshold-based eviction** to archive memories that have decayed below usefulness

## The Lifecycle

### Write Path: Reconciliation

Every new memory candidate is reconciled against existing knowledge before storage:

1. Generate embedding for candidate
2. Vector search existing nodes of compatible types (cosine > 0.85)
3. For each near-match, LLM determines one of:
   - **ADD** — new information, store as new node
   - **UPDATE** — refines existing node, modify in-place
   - **DELETE** — contradicts existing, mark old as `superseded`, store new with `supersedes` edge
   - **NOOP** — already captured, optionally reinforce existing node
4. If no near-match, ADD as new node

This prevents the unbounded growth that plagues append-only systems.

### Read Path: Composite Scoring

Retrieval uses a four-signal composite score computed at query time:

| Signal | Weight | Computation |
|--------|--------|-------------|
| Relevance | 0.35 | Cosine similarity between query embedding and node embedding |
| Importance | 0.25 | `base_importance × (1 + ln(1 + access_count)) × citation_bonus` |
| Recency | 0.25 | `e^(-decay_rate × hours_since_last_access)` |
| Structural | 0.15 | Bonus for nodes connected (via edges) to other high-scoring results |

### Decay Rates by Type

| Type | λ (decay_rate) | Half-Life | Rationale |
|------|---------------|-----------|-----------|
| Episode | 0.15 | ~1-2 weeks | Specific events become less relevant; value should be extracted into concepts/procedures |
| Concept | 0.02 | ~2-3 months | Facts persist until contradicted |
| Procedure | 0.005 | ~6+ months | Learned rules retain value long-term |
| Reflection | 0.05 | ~3-4 weeks | Higher-order observations may need refreshing |

### Reinforcement

When a memory is retrieved and actually used (incorporated into a response or decision):
- Increment `access_count`
- Update `last_accessed` to now
- Optionally increase `importance` by +0.05 (capped at 1.0)

### Consolidation

Triggered **opportunistically** (no background scheduler) or by explicit `cerebro consolidate`:

**Trigger conditions (combination — any one is sufficient):**
- Write threshold: 50 unconsolidated episodes accumulated (checked after each `remember`)
- Session boundary: consolidation hasn't run since session start (checked on `recall`)
- Time threshold: >24h since last consolidation AND >10 unconsolidated episodes (checked on `recall`)
- Explicit: user runs `cerebro consolidate`

All triggers are evaluated lazily against a `last_consolidated_at` timestamp in `schema_meta`. No daemon, no cron, no background process — the brain is only active when the orchestrator is using it.

**Process:**
1. Select unconsolidated episodes (`status='active', type='episode'`)
2. Cluster by semantic similarity and graph connectivity
3. For each cluster, LLM generates higher-order nodes (concepts, procedures, reflections)
4. Link new nodes to source episodes via `learned_from` edges
5. Mark source episodes as `status='consolidated'` (accelerated decay)
6. Update `schema_meta`: `last_consolidated_at = now()`

### Eviction

Triggered opportunistically (on session start if overdue) or by `cerebro gc`:
1. Compute retrieval_score for all active nodes
2. Candidates: score < eviction_threshold (default 0.01)
3. Safety checks before eviction:
   - Don't break graph bridges between high-value subgraphs
   - Don't evict nodes that are sole sources for active procedures/concepts
4. Move to `nodes_archive` table (not hard-deleted)
5. Remove from `vec_nodes`

## Alternatives Considered

### 1. Append-only with no lifecycle (original musing)
- **Rejected because:** Unbounded growth, search quality degradation, no deduplication, no conflict resolution. Not sustainable beyond a few hundred memories.

### 2. Fixed-window (keep last N memories)
- **Rejected because:** Discards based on age alone, ignoring importance. A critical procedure learned 6 months ago is more valuable than a trivial episode from yesterday.

### 3. LRU eviction (evict least-recently-used)
- **Rejected because:** Important but rarely accessed memories (e.g., disaster recovery procedures) would be evicted. Need importance as a separate signal from recency.

### 4. Manual curation only
- **Rejected because:** Impractical at scale. Agent generates dozens of observations per session. Human cannot manually curate every memory. Automated lifecycle with human override is the right balance.

## Consequences

### Positive
- **Self-maintaining knowledge base.** Brain stays relevant without manual curation.
- **No duplicate accumulation.** Reconciliation prevents the same fact from being stored repeatedly.
- **Contradiction resolution.** New information that conflicts with old is handled explicitly rather than silently coexisting.
- **Value extraction.** Raw episodes are distilled into reusable concepts and procedures, creating a hierarchy of abstraction.
- **Predictable scale.** With active eviction, the brain stabilizes at a manageable size rather than growing forever.

### Negative
- **LLM cost for reconciliation.** Every write requires an LLM call to determine ADD/UPDATE/DELETE/NOOP. This adds latency and cost. Mitigation: use a fast, cheap model (e.g., Haiku) for reconciliation.
- **Risk of incorrect reconciliation.** The LLM might incorrectly mark a genuinely new memory as NOOP, or incorrectly DELETE a still-valid memory. Mitigation: archive, don't hard-delete. All superseded/evicted memories are recoverable.
- **Consolidation quality depends on LLM.** The reflections generated are only as good as the LLM's synthesis capability. Low-quality consolidation could produce misleading procedures. Mitigation: consolidation results can be reviewed via `cerebro inspect`.

### Risks
- **Over-aggressive eviction.** If decay rates are too high or thresholds too aggressive, useful memories may be evicted prematurely. Mitigation: conservative defaults (threshold=0.01), archive not delete, tune based on observed behavior.
- **Consolidation timing.** If consolidation runs too infrequently, episodes accumulate and search quality degrades. If too frequently, consolidation overhead is high. Mitigation: combination of opportunistic triggers (write threshold: 50 episodes, time threshold: 24h, session boundary) with configurable defaults. No background scheduler — triggers are checked lazily on existing operations.

## References
- [Research: Agent memory architectures](../research/agent-memory-architectures-research.md)
- Packer et al. (2024). "MemGPT: Towards LLMs as Operating Systems." ICLR 2024.
- Park et al. (2023). "Generative Agents." UIST 2023.
- Mem0 Documentation: https://docs.mem0.ai/
