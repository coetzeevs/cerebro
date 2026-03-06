# ADR-006: Claude Code as Primary Integration Target (Agent-Managed Memory)

## Status
Proposed

## Context

Cerebro's original architecture (sections 6, 8) described a generic orchestrator integration where Cerebro handles memory operations end-to-end — including LLM-powered reconciliation (ADD/UPDATE/DELETE/NOOP) and consolidation (episodes → concepts/procedures/reflections). This design assumed Cerebro would embed its own LLM provider for the "thinking" parts of memory management.

Three factors challenge that assumption:

### 1. Primary use case is Claude Code

The immediate integration target is Claude Code — Anthropic's agentic coding CLI. The user invokes Claude Code from a project directory, and Claude Code is the orchestrator that should use Cerebro for persistent memory. There are no near-term plans to support other orchestrators.

### 2. Claude Code Max subscription covers all LLM usage

The user operates under a Claude Code Max subscription, which covers all Claude model usage within Claude Code sessions. If Cerebro had its own LLM provider (calling Anthropic's API directly, or running Ollama), that would be a separate cost — either an API billing account or local compute for model inference. With Model B (Claude does the thinking), all LLM usage for reconciliation and consolidation is covered by the existing Max subscription at zero incremental cost.

### 3. The agent should manage its own brain

There's a philosophical and practical argument: the agent (Claude) should control its own memory management. Claude has the richest context — it knows what it just learned, why it matters, how it relates to current work, and what it already knows. Delegating reconciliation to a separate, context-poor LLM call (e.g., Haiku with a 500-token prompt) produces worse decisions than having Claude reason about its own memories inline.

This mirrors human cognition: you manage your own memory. You don't outsource the decision of "is this new or do I already know this?" to someone else.

## Decision

### 1. Claude Code is the cognition layer; Cerebro is the storage layer

The separation of concerns:

| Layer | Responsibility | Implementation |
|-------|---------------|----------------|
| **Cognition** (Claude Code) | Decides what to remember, reconciliation reasoning, consolidation synthesis, importance assessment, promotion decisions | Claude Code hooks, skills, CLAUDE.md instructions |
| **Storage** (Cerebro CLI) | Embedding generation, vector search, graph operations, composite scoring, decay calculations, status management, eviction | Go binary with pluggable embedding provider |

Cerebro has **no LLM dependency**. It is pure data infrastructure — it stores, indexes, searches, scores, and manages lifecycle state. All reasoning about memory content is performed by Claude within the Claude Code session.

### 2. Integrate via hooks + skills + CLAUDE.md (not MCP)

Claude Code offers several extension mechanisms. MCP servers are the richest integration but load tool definitions into the context window, consuming tokens on every turn. For a tool that should be invisible infrastructure, this overhead is unacceptable.

The chosen integration pattern:

| Mechanism | Token Cost | Automatic? | Role |
|-----------|-----------|-----------|------|
| **Hooks** | Zero (execute outside context) | Yes — lifecycle events | Session start prime, pre-compaction save, session end persist |
| **Skills** | Description always loaded (~100-200 tokens); full content only on invoke | Manual or Claude-auto-invoked | `/remember`, `/recall`, `/consolidate`, `/forget` |
| **CLAUDE.md** | Always in context (~200-400 tokens) | Always loaded | Behavioral rules ("use cerebro for memory management") |

### 3. CLI commands are building blocks, not high-level operations

The CLI exposes low-level operations that Claude orchestrates into higher-level workflows. Cerebro does not have a `remember` command that internally runs reconciliation — instead, Claude performs the reconciliation reasoning and calls the appropriate lower-level commands.

**Core commands (storage operations):**

| Command | Purpose | Returns |
|---------|---------|---------|
| `cerebro init` | Initialize project brain | Status |
| `cerebro add --type <type> --importance <0-1> <content>` | Store a new memory node | Node ID |
| `cerebro update <id> --content <text> [--importance <0-1>]` | Modify existing node | Status |
| `cerebro supersede <old_id> --type <type> --importance <0-1> <content>` | Mark old as superseded, store new with `supersedes` edge | New node ID |
| `cerebro reinforce <id>` | Increment access_count, update last_accessed | Status |
| `cerebro edge <source_id> <target_id> <relation>` | Create a relationship edge | Edge ID |

**Query commands (retrieval operations):**

| Command | Purpose | Returns |
|---------|---------|---------|
| `cerebro search <query> [--type <type>] [--limit N] [--threshold 0.7]` | Vector + graph similarity search | Scored node list (JSON or markdown) |
| `cerebro recall <query> [--limit N] [--format md\|json]` | Full composite-scored retrieval (recency × importance × relevance × structural) | Formatted memory context |
| `cerebro get <id>` | Retrieve a specific node with edges | Node detail |
| `cerebro list [--type <type>] [--status <status>] [--since <date>]` | List nodes by filter | Node list |

**Lifecycle commands (maintenance operations):**

| Command | Purpose | Returns |
|---------|---------|---------|
| `cerebro mark-consolidated <id> [<id>...]` | Set episodes to `status='consolidated'` | Status |
| `cerebro gc [--threshold 0.01] [--dry-run]` | Evict decayed memories to archive | Eviction report |
| `cerebro stats` | Brain health: counts by type/status, embedding status, last consolidation | Stats report |
| `cerebro export [--format sqlite\|sql\|json]` | Dump brain to portable format (default: raw .sqlite copy) | File path |

**Global commands:**

| Command | Purpose |
|---------|---------|
| `cerebro promote <id> --content <generalized_content>` | Copy to global store with generalized content |
| `cerebro recall --global <query>` | Query global store alongside project store |

All commands support `--format json` for structured output (consumed by hooks/skills) and default to human-readable markdown.

## Session Lifecycle

### Startup: Automatic Context Priming

```
Claude Code session starts
         │
         ▼
SessionStart hook fires
         │
         ▼
cerebro recall --prime --format md
         │
         ▼
stdout → injected into Claude's context (zero extra token cost for hook execution)
         │
         ▼
Claude begins session with project memory loaded
```

The `--prime` flag returns a curated selection: high-importance active memories, recently accessed context, and any active procedures. The number of memories returned is controlled by the `prime_limit` config setting (default: 20), overridable via `--limit N`. This is the "what do I already know about this project?" briefing. Claude can request additional context during the session via `/recall` if the prime was insufficient.

### During Work: Claude-Driven Memory Operations

Claude uses skills to interact with Cerebro during the session. The `/remember` skill orchestrates the full reconciliation flow:

```
Claude decides something is worth remembering
         │
         ▼
/remember skill invoked (by Claude or user)
         │
         ▼
Step 1: cerebro search "<content summary>" --limit 5 --threshold 0.7
         │
         ▼
Step 2: Claude reviews search results and reasons:
        "Node X says similar thing → UPDATE"
        "Node Y contradicts → SUPERSEDE"
        "No match → ADD"
         │
         ├── cerebro update X --content "refined content"
         ├── cerebro supersede Y --type concept --importance 0.8 "new understanding"
         └── cerebro add --type episode --importance 0.6 "new observation"
         │
         ▼
Step 3: Claude creates edges if relationships exist
        cerebro edge <new_id> <related_id> relates_to
```

The `/recall` skill provides on-demand memory retrieval beyond the session-start prime:

```
User: /recall authentication flow
         │
         ▼
cerebro recall "authentication flow" --limit 10 --format md
         │
         ▼
Results injected into conversation
```

### Pre-Compaction: Memory Preservation

This is a critical integration point. When Claude Code's context window fills up and compaction is triggered, important detailed context may be lost. The PreCompact hook gives Claude a chance to persist critical information:

```
Context approaching limit
         │
         ▼
PreCompact hook fires
         │
         ▼
Injects reminder: "Context compaction imminent. Persist critical
context to cerebro using /remember before compaction occurs."
         │
         ▼
Claude reviews working context and persists key learnings
         │
         ▼
Compaction proceeds (summary replaces detailed context)
         │
         ▼
On next interaction, SessionStart-like recall can restore key memories
```

### Session End: Opportunistic Maintenance

```
Session terminating
         │
         ▼
SessionEnd hook fires
         │
         ▼
cerebro gc --threshold 0.01 --quiet
         │
         ▼
(Optional) cerebro stats --format json → log for diagnostics
```

Consolidation (episodes → concepts/procedures) is NOT automated at session end — it requires Claude's reasoning to synthesize episodes into higher-order knowledge. Instead, consolidation is triggered explicitly via the `/consolidate` skill when the user or Claude determines it's appropriate.

## Integration Configuration

### Hooks (`.claude/settings.json` or `~/.claude/settings.json`)

```json
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "cerebro recall --prime --format md 2>/dev/null || echo 'Cerebro: no brain initialized for this project. Run cerebro init to start.'"
          }
        ]
      }
    ],
    "PreCompact": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "echo '⚠ Context compaction is imminent. Before compaction proceeds, review your current working context and use /remember to persist any critical information — architectural decisions, discovered constraints, work-in-progress state, or anything that would be costly to rediscover.'"
          }
        ]
      }
    ],
    "SessionEnd": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "cerebro gc --threshold 0.01 --quiet 2>/dev/null; true"
          }
        ]
      }
    ]
  }
}
```

### Skills

**`~/.claude/skills/remember/SKILL.md`** (global — works across all projects):

```yaml
---
name: remember
description: Store a learning, fact, or observation in Cerebro memory. Use when you discover something worth persisting across sessions — architectural decisions, bug patterns, user preferences, project conventions, or important context that would be lost on compaction.
allowed-tools: Bash(cerebro *)
---

# Remember

Store the following in Cerebro memory. Follow this reconciliation protocol:

## Step 1: Determine memory type and content

Based on what needs to be remembered, classify it:
- **episode** — a specific event, interaction, outcome, or discovery
- **concept** — a fact, entity, relationship, preference, or constraint
- **procedure** — a rule, pattern, anti-pattern, or workflow
- **reflection** — a higher-order observation synthesized from multiple experiences

Formulate the memory content as a clear, self-contained statement.

## Step 2: Check for existing related memories

```bash
cerebro search "<content summary>" --limit 5 --threshold 0.7 --format json
```

## Step 3: Reconcile

Review the search results and decide for each match:
- **ADD** — this is genuinely new information with no overlap
- **UPDATE** — this refines or extends an existing memory (use `cerebro update <id>`)
- **SUPERSEDE** — this contradicts an existing memory (use `cerebro supersede <old_id>`)
- **NOOP** — this is already captured; optionally reinforce with `cerebro reinforce <id>`

## Step 4: Execute

For ADD (no matches or genuinely new):
```bash
cerebro add --type <type> --importance <0.0-1.0> "<content>"
```

For UPDATE:
```bash
cerebro update <existing_id> --content "<refined content>"
```

For SUPERSEDE:
```bash
cerebro supersede <old_id> --type <type> --importance <0.0-1.0> "<new content>"
```

## Step 5: Create edges

If the new/updated memory relates to other known memories:
```bash
cerebro edge <source_id> <target_id> <relation>
```

Relations: relates_to, depends_on, learned_from, resulted_in, supersedes, blocks, implements

## Importance Guidelines

- 0.9-1.0: Critical constraints, hard-won lessons, architectural invariants
- 0.7-0.8: Important patterns, user preferences, key project decisions
- 0.5-0.6: General observations, useful context, standard facts
- 0.3-0.4: Minor notes, low-confidence observations
- 0.1-0.2: Ephemeral context, likely to become irrelevant

$ARGUMENTS
```

**`~/.claude/skills/recall/SKILL.md`**:

```yaml
---
name: recall
description: Retrieve memories from Cerebro relevant to a topic or question. Use when you need project context, past decisions, known patterns, or historical information.
allowed-tools: Bash(cerebro *)
---

# Recall

Retrieve relevant memories from Cerebro:

```bash
cerebro recall "$ARGUMENTS" --limit 10 --format md
```

Review the retrieved memories and integrate them into your understanding of the current context. If key information seems missing, try alternative queries or check specific memory types:

```bash
cerebro list --type procedure --status active --format md
```
```

**`~/.claude/skills/consolidate/SKILL.md`**:

```yaml
---
name: consolidate
description: Review unconsolidated episode memories and synthesize them into higher-order concepts, procedures, and reflections. Use when many episodes have accumulated or at natural stopping points.
disable-model-invocation: true
allowed-tools: Bash(cerebro *)
---

# Consolidate Memories

## Step 1: Review unconsolidated episodes

```bash
cerebro list --type episode --status active --format json
```

## Step 2: Identify clusters

Group the episodes by theme or topic. Look for:
- Repeated patterns (same type of issue, same approach working/failing)
- Accumulated facts about a specific area (auth, deployment, testing, etc.)
- Lessons that generalize beyond the specific episode

## Step 3: Synthesize higher-order memories

For each cluster, create appropriate higher-order nodes:

- **Concepts** for accumulated factual knowledge:
  ```bash
  cerebro add --type concept --importance <0.0-1.0> "<synthesized fact>"
  ```

- **Procedures** for learned rules or workflows:
  ```bash
  cerebro add --type procedure --importance <0.0-1.0> "<rule or workflow>"
  ```

- **Reflections** for meta-observations:
  ```bash
  cerebro add --type reflection --importance <0.0-1.0> "<observation>"
  ```

## Step 4: Link and mark

For each new node, link it to its source episodes:
```bash
cerebro edge <new_id> <episode_id> learned_from
```

Then mark the source episodes as consolidated:
```bash
cerebro mark-consolidated <episode_id> [<episode_id>...]
```
```

### CLAUDE.md Instructions

The following should be added to the project's `.claude/CLAUDE.md` (or the user's `~/.claude/CLAUDE.md` for global behavior):

```markdown
## Cerebro Memory System

This environment uses Cerebro for persistent memory across sessions.

### Automatic behavior
- Session start: recent memories are automatically loaded via hook
- Pre-compaction: you will be reminded to persist critical context
- Session end: garbage collection runs automatically

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
```

## Alternatives Considered

### 1. Model A: Cerebro has its own LLM provider (self-contained)

Cerebro embeds an LLM client (Anthropic API, Ollama) and handles reconciliation/consolidation internally. Claude calls `cerebro remember "X"` and cerebro does everything.

**Rejected because:**
- **Extra cost.** Separate API billing on top of Max subscription, or local Ollama overhead.
- **Context-poor decisions.** A reconciliation call with a 500-token prompt has a fraction of the context Claude has when it's mid-session reasoning about what it just learned.
- **Black box.** Claude has no visibility into whether a memory was stored, updated, or discarded.
- **Philosophical mismatch.** The agent should manage its own brain. Outsourcing memory decisions to a separate LLM is like dictating to a secretary who decides what to write.

### 2. Model C: Hybrid (cerebro has LLM, Claude can also reason)

Cerebro has its own LLM for autonomous operations, but Claude can also drive reconciliation directly.

**Rejected because:** Added complexity for marginal benefit. If Claude is always the caller, the autonomous LLM path is never used. Simpler to have one path. If non-Claude-Code orchestrators become a need in the future, the LLM provider can be added then — the interface is designed to support it.

### 3. MCP Server integration

Cerebro exposes an MCP server that Claude Code discovers and invokes as tools.

**Rejected because:** MCP tool definitions load into the context window. With 6-10 cerebro tools, that's several hundred tokens consumed on every turn of every conversation, regardless of whether memory operations are needed. Hooks and skills load nothing (hooks) or only descriptions (skills) until invoked. For infrastructure that should be invisible, MCP's token overhead is the wrong tradeoff.

### 4. Pure hooks (no skills)

Use only hooks for all memory operations — no user-invocable commands.

**Rejected because:** Hooks cannot orchestrate multi-step reasoning. The reconciliation flow (search → reason → add/update/supersede) requires Claude to reason between steps. Skills provide this naturally. Hooks are ideal for simple, automatic operations (prime, gc); skills are ideal for reasoning-heavy operations (remember, consolidate).

## Consequences

### Positive

- **Zero incremental LLM cost.** All reasoning uses Claude within the Max subscription. No separate API key, no Ollama requirement for LLM inference.
- **Best reconciliation quality.** Claude has full session context when deciding how to file a memory. A separate Haiku call can't match this.
- **Agent autonomy.** Claude manages its own brain — what to store, how to reconcile, when to consolidate. The tool doesn't make decisions for the agent.
- **Minimal token overhead.** Hooks are zero-cost. Skill descriptions are ~100-200 tokens. CLAUDE.md instructions are ~200-400 tokens. Total standing overhead: ~300-600 tokens — far less than MCP tool definitions.
- **Compaction resilience.** PreCompact hook gives Claude a chance to persist critical context before it's summarized away. This directly addresses the context loss problem.
- **Simple Cerebro implementation.** No LLM client code in Go. No prompt management. No model configuration. Cerebro is pure data infrastructure.

### Negative

- **Coupled to Claude Code.** This integration pattern is specific to Claude Code's hooks/skills system. Other orchestrators would need their own integration layer. Mitigation: Cerebro's CLI is standard — any orchestrator that can run shell commands can use it. Only the automation (hooks, skills) is Claude Code-specific.
- **Multi-step remember operations.** Each `/remember` involves 2-3 cerebro CLI calls (search, then add/update/supersede, then edge). This is more tool calls than a single `cerebro remember` would be. Mitigation: the operations are fast (<50ms each), and the reconciliation quality gain justifies the extra calls.
- **Consolidation requires explicit invocation.** Without an internal LLM, cerebro can't autonomously consolidate. It depends on Claude running `/consolidate`. Mitigation: CLAUDE.md instructions and the `/consolidate` skill description encourage this. The `stats` command reports unconsolidated episode count, creating a natural trigger.
- **Skill maintenance.** The reconciliation and consolidation logic lives in skill markdown files, not in compiled code. Changes to the protocol require updating skill files. Mitigation: skills are versioned with the project and can be updated like any other config file.

### Risks

- **Claude ignoring memory instructions.** LLMs don't always follow instructions perfectly. Claude might forget to use `/remember` even when CLAUDE.md says to. Mitigation: the SessionStart hook ensures memories are loaded (automatic, no LLM cooperation needed). The PreCompact hook provides a hard reminder. The `/remember` skill can be invoked by the user when Claude doesn't do it proactively. Over time, as Claude becomes more reliable with tool use, adherence will improve.
- **Reconciliation quality depends on skill prompt.** The `/remember` skill defines the reconciliation protocol in markdown. If the prompt is poorly written, reconciliation will be poor. Mitigation: the protocol is clear and deterministic (search, compare, decide, execute). Testing with real sessions will validate and refine the prompt.
- **PreCompact timing.** The PreCompact hook injects a reminder, but Claude may not have enough remaining context to do a thorough memory dump before compaction occurs. Mitigation: the reminder is concise and actionable. Claude should focus on the most critical items. Even partial persistence is better than none.

## References

- [ADR-003: Memory lifecycle strategy](ADR-003-memory-lifecycle-strategy.md) — reconciliation and consolidation protocol (updated to reflect caller-driven model)
- [Claude Code hooks documentation](https://docs.anthropic.com/en/docs/claude-code/hooks)
- [Claude Code skills documentation](https://docs.anthropic.com/en/docs/claude-code/skills)
- [Research: Agent memory architectures](../research/agent-memory-architectures-research.md) — Mem0 reconciliation model, Generative Agents reflection
