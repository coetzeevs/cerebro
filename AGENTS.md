# Agent Instructions

## Tool Responsibilities

| Concern | Tool | Commands |
|---------|------|----------|
| What do I know? | **Cerebro** | `/recall`, `/remember`, `cerebro` CLI |
| What should I do? | **Beads** | `bd ready`, `bd show`, `bd create`, `bd close` |
| What's the plan? | **Jira** | `acli` (read-only for context) |

### Memory is Cerebro's job
- Do NOT use `bd remember` — all persistent knowledge goes through Cerebro
- Use `/remember` for decisions, patterns, conventions, bug resolutions
- Use `/recall` for retrieving past context

### Tasks are Beads' job
- Do NOT use TodoWrite, TaskCreate, or markdown TODO files
- Use `bd create` for new work items
- Use `bd ready` to find unblocked tasks
- Use `bd close <id>` to mark work complete

### Planning comes from Jira
- Stories and Epics live in Jira — don't duplicate them in Beads
- Beads holds implementation subtasks that decompose Jira stories
- Use `acli` to read Jira context when starting work

## Non-Interactive Shell Commands

ALWAYS use non-interactive flags with file operations to avoid hanging:

```bash
cp -f source dest           # NOT: cp source dest
mv -f source dest           # NOT: mv source dest
rm -f file                  # NOT: rm file
rm -rf directory            # NOT: rm -r directory
```

## Push Protocol

Never push without user confirmation. When work is ready to push:
1. Run quality gates (tests, lint, build)
2. `bd dolt push` (sync beads data)
3. State that you're ready to push and wait for confirmation
