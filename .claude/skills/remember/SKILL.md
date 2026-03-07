---
name: remember
description: Store a learning, fact, or observation in Cerebro memory. Use when you discover something worth persisting across sessions — architectural decisions, bug patterns, user preferences, project conventions, or important context that would be lost on compaction.
argument-hint: "[what to remember]"
allowed-tools: Read, Bash(cerebro *)
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
cerebro search "$ARGUMENTS" --limit 5 --threshold 0.7 --format json
```

## Step 3: Reconcile

Review the search results and decide for each match:
- **ADD** — this is genuinely new information with no overlap
- **UPDATE** — this refines or extends an existing memory (use `cerebro update <id>`)
- **SUPERSEDE** — this contradicts or replaces an existing memory (use `cerebro supersede <old_id>`)
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

Relations: `relates_to`, `depends_on`, `learned_from`, `resulted_in`, `supersedes`, `blocks`, `implements`

## Importance Guidelines

- 0.9-1.0: Critical constraints, hard-won lessons, architectural invariants
- 0.7-0.8: Important patterns, user preferences, key project decisions
- 0.5-0.6: General observations, useful context, standard facts
- 0.3-0.4: Minor notes, low-confidence observations
- 0.1-0.2: Ephemeral context, likely to become irrelevant
