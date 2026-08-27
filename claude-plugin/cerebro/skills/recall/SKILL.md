---
name: recall
description: Retrieve memories from Cerebro relevant to a topic or question. Use when you need project context, past decisions, known patterns, or historical information.
argument-hint: "[topic or question]"
allowed-tools: Bash(cerebro *)
---

# Recall

Retrieve relevant memories from Cerebro:

```bash
cerebro recall "$ARGUMENTS" --format md -p "$CLAUDE_PROJECT_DIR"
```

Review the retrieved memories and integrate them into your understanding of the current context.

If key information seems missing, try alternative queries or check specific memory types:

```bash
cerebro list --type procedure --status active --format md -p "$CLAUDE_PROJECT_DIR"
```

```bash
cerebro list --type concept --status active --format md -p "$CLAUDE_PROJECT_DIR"
```

If still insufficient, try a broader search with lower threshold:

```bash
cerebro search "$ARGUMENTS" --limit 20 --threshold 0.3 --format md -p "$CLAUDE_PROJECT_DIR"
```

## Close the loop

After acting on retrieved memories, record outcomes so ranking improves:

```bash
cerebro outcome <id> --success -p "$CLAUDE_PROJECT_DIR"   # it helped
cerebro outcome <id> --failure -p "$CLAUDE_PROJECT_DIR"   # it misled (consider supersede)
```
