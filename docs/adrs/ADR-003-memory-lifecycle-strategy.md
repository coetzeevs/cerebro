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

Every new memory candidate is reconciled against existing knowledge before storage. The reconciliation protocol follows Mem0's four-operation model, but the **decision-maker is the calling agent (Claude), not an internal LLM** (see [ADR-006](ADR-006-claude-code-integration-pattern.md)).

**Protocol (caller-driven):**
1. Caller formulates memory content and classifies type
2. `cerebro search "<content>" --threshold 0.7` — Cerebro generates embedding and returns similar nodes
3. Caller reviews results and determines one of:
   - **ADD** — new information → `cerebro add --type <type> --importance <n> "<content>"`
   - **UPDATE** — refines existing → `cerebro update <id> --content "<refined>"`
   - **DELETE** — contradicts existing → `cerebro supersede <old_id> --type <type> "<new>"`
   - **NOOP** — already captured → optionally `cerebro reinforce <id>`
4. If no near-match, ADD as new node
5. Caller creates edges for relationships: `cerebro edge <src> <tgt> <relation>`

**Why caller-driven:** The calling agent (Claude in Claude Code) has full session context — it knows what it just learned, why it matters, and how it relates to current work. This produces better reconciliation decisions than a separate LLM call with a narrow prompt, at zero incremental cost (covered by Max subscription).

**Cerebro's role:** Embedding generation, vector similarity search, composite scoring, storage operations. No LLM reasoning.

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

Triggered by the calling agent via the `/consolidate` skill, or explicitly by user request. There is no background scheduler — consolidation is a caller-driven reasoning process.

**When to consolidate:** The `cerebro stats` command reports unconsolidated episode count. CLAUDE.md instructions guide Claude to consolidate when episode count exceeds ~30-50, at natural stopping points, or when `cerebro recall --prime` reports consolidation is overdue.

**Process (caller-driven):**
1. `cerebro list --type episode --status active` — returns unconsolidated episodes
2. Caller clusters episodes by theme/topic and synthesizes higher-order knowledge
3. For each synthesized insight, caller creates new nodes:
   - `cerebro add --type concept "<synthesized fact>"`
   - `cerebro add --type procedure "<learned rule>"`
   - `cerebro add --type reflection "<meta observation>"`
4. Caller links new nodes to source episodes: `cerebro edge <new_id> <episode_id> learned_from`
5. Caller marks sources: `cerebro mark-consolidated <episode_id> [...]`

**Why caller-driven:** Consolidation requires high-quality synthesis — identifying patterns across experiences and generating useful abstractions. The calling agent (Claude) has the richest context for this reasoning. The quality of consolidated knowledge directly impacts future recall, so it's worth using the best available reasoning rather than a separate, context-poor LLM call.

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
- **Multi-step remember operations.** Each write involves 2-3 CLI calls (search → add/update/supersede → edge). More tool invocations than a single monolithic `remember` command. Mitigation: operations are fast (<50ms each), and reconciliation quality gain justifies the extra calls.
- **Risk of incorrect reconciliation.** The caller might incorrectly mark a genuinely new memory as NOOP, or incorrectly supersede a still-valid memory. Mitigation: archive, don't hard-delete. All superseded/evicted memories are recoverable via `nodes_archive`.
- **Consolidation quality depends on caller.** The synthesized concepts/procedures are only as good as the calling agent's reasoning. Mitigation: consolidation results can be reviewed via `cerebro get <id>` and `cerebro list --type reflection`.
- **Consolidation requires explicit invocation.** Without internal LLM, cerebro can't autonomously consolidate. Mitigation: `cerebro stats` reports unconsolidated count; `cerebro recall --prime` flags when consolidation is overdue; CLAUDE.md instructions guide the agent.

### Risks
- **Over-aggressive eviction.** If decay rates are too high or thresholds too aggressive, useful memories may be evicted prematurely. Mitigation: conservative defaults (threshold=0.01), archive not delete, tune based on observed behavior.
- **Consolidation neglect.** If the calling agent doesn't invoke consolidation, episodes accumulate and search quality degrades. Mitigation: `cerebro recall --prime` output includes a consolidation-overdue warning when unconsolidated episodes exceed threshold. CLAUDE.md instructions create a behavioral nudge.
- **Caller-dependent consistency.** Different agent sessions might make different reconciliation decisions for similar content. Mitigation: the `/remember` skill defines a deterministic reconciliation protocol. The search → reason → execute flow produces consistent results given consistent skill instructions.

## References
- [ADR-006: Claude Code integration pattern](ADR-006-claude-code-integration-pattern.md) — caller-driven reconciliation rationale
- [Research: Agent memory architectures](../research/agent-memory-architectures-research.md)
- Packer et al. (2024). "MemGPT: Towards LLMs as Operating Systems." ICLR 2024.
- Park et al. (2023). "Generative Agents." UIST 2023.
- Mem0 Documentation: https://docs.mem0.ai/
