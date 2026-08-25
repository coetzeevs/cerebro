---
name: consolidate
description: Review unconsolidated episode memories and synthesize them into higher-order concepts, procedures, and reflections. Use when many episodes have accumulated or at natural stopping points.
argument-hint: "[optional: topic to focus consolidation on]"
disable-model-invocation: true
allowed-tools: Bash(cerebro *)
---

# Consolidate Memories

## Step 1: Review unconsolidated episodes

```bash
cerebro list --type episode --status active --format json -p "$CLAUDE_PROJECT_DIR"
```

If `$ARGUMENTS` was provided, filter by searching for that topic:
```bash
cerebro search "$ARGUMENTS" --limit 50 --threshold 0.3 --format json -p "$CLAUDE_PROJECT_DIR"
```

## Step 2: Identify clusters

Group the episodes by theme or topic. Look for:
- Repeated patterns (same type of issue, same approach working/failing)
- Accumulated facts about a specific area (auth, deployment, testing, etc.)
- Lessons that generalize beyond the specific episode

## Step 3: Synthesize higher-order memories

For each cluster, create appropriate higher-order nodes:

**Concepts** for accumulated factual knowledge:
```bash
CEREBRO_ORIGIN_ACTOR="${CEREBRO_ORIGIN_ACTOR:-claude-code}" cerebro add --type concept --importance <0.0-1.0> --origin-channel skill "<synthesized fact>" -p "$CLAUDE_PROJECT_DIR"
```

**Procedures** for learned rules or workflows:
```bash
CEREBRO_ORIGIN_ACTOR="${CEREBRO_ORIGIN_ACTOR:-claude-code}" cerebro add --type procedure --importance <0.0-1.0> --origin-channel skill "<rule or workflow>" -p "$CLAUDE_PROJECT_DIR"
```

**Reflections** for meta-observations:
```bash
CEREBRO_ORIGIN_ACTOR="${CEREBRO_ORIGIN_ACTOR:-claude-code}" cerebro add --type reflection --importance <0.0-1.0> --origin-channel skill "<observation>" -p "$CLAUDE_PROJECT_DIR"
```

## Step 4: Link and mark (atomic)

For each new node, consolidate its source episodes into it — this wires a
`derived_from` provenance edge to every source AND marks the sources
consolidated, in one atomic transaction:
```bash
cerebro consolidate --into <new_id> <episode_id> [<episode_id>...] -p "$CLAUDE_PROJECT_DIR"
```

## Step 5: Report

After consolidation, show the brain's current state:
```bash
cerebro stats -p "$CLAUDE_PROJECT_DIR"
```
