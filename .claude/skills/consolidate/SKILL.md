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
cerebro list --type episode --status active --format json
```

If `$ARGUMENTS` was provided, filter by searching for that topic:
```bash
cerebro search "$ARGUMENTS" --limit 50 --threshold 0.3 --format json
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
cerebro add --type concept --importance <0.0-1.0> "<synthesized fact>"
```

**Procedures** for learned rules or workflows:
```bash
cerebro add --type procedure --importance <0.0-1.0> "<rule or workflow>"
```

**Reflections** for meta-observations:
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

## Step 5: Report

After consolidation, show the brain's current state:
```bash
cerebro stats
```
