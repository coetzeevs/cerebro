---
name: recall
description: Retrieve memories from Cerebro relevant to a topic or question. Use when you need project context, past decisions, known patterns, or historical information.
argument-hint: "[topic or question]"
allowed-tools: Bash(cerebro *)
---

# Recall

Retrieve relevant memories from Cerebro:

```bash
cerebro recall "$ARGUMENTS" --limit 10 --format md
```

Review the retrieved memories and integrate them into your understanding of the current context.

If key information seems missing, try alternative queries or check specific memory types:

```bash
cerebro list --type procedure --status active --format md
```

```bash
cerebro list --type concept --status active --format md
```

If still insufficient, try a broader search with lower threshold:

```bash
cerebro search "$ARGUMENTS" --limit 20 --threshold 0.3 --format md
```
