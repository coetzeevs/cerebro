# Agent Instructions

<!-- ontology-stack-frame -->
Ontology version: 1.1
Stack classification: stack-frame (Cerebro is the Memory layer in the agentic stack)

This project is part of the agentic stack; the operational ontology at `/Users/q/projects/agentic/documentation/Operational Ontology.md` is authoritative for any rule about cross-project responsibilities (memory vs tasks vs planning, runtime vs orchestration, etc.). Local rules below apply only to in-project behaviour and must not contradict the ontology. If a rule below conflicts with the ontology, the ontology wins and this file must be reconciled.

## Stack-wide rules (canonical location)

The cross-tool role boundaries that previously lived here (memory = Cerebro, tasks = Beads, planning = Jira; forbidden uses of `bd remember`, TaskCreate, TodoWrite; etc.) now live in the ontology:

- Layer model and System of Record table: `documentation/Operational Ontology.md` §3, §4
- Memory vs `bd remember` rule: §5.2
- Tasks vs Claude Code TaskCreate/TodoWrite: §5.6
- Beads threads for durable handoffs: §5.8
- All operational do-nots: §7

Do not duplicate those rules here. Refer to the ontology when in doubt.

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
