# Cerebro

@AGENTS.md

Local-first persistent memory system for AI agents. SQLite-backed with vector search (sqlite-vec).

## Dogfooding

This project is its own first use case. The test of cerebro's efficacy is whether the agent (Claude Code) has recall of this project's architecture, decisions, and the process used to build it — across sessions, through context compactions, without losing continuity. If cerebro works, you should know why we chose Model B over Model C, what ADR-006 decided, and how the store layer is structured without re-reading everything from scratch.

## Development

```bash
# Build
go build ./cmd/cerebro

# Test
go test ./... -race

# Test with coverage
go test ./... -race -coverprofile=coverage.out
go tool cover -func=coverage.out

# Lint
golangci-lint run
```

## Project Structure

```
cmd/cerebro/       CLI (Cobra commands)
brain/             Public API (Brain type) + type re-exports (brain/types.go)
internal/store/    SQLite storage, schema, CRUD, vector search
internal/embed/    Embedding provider interface + implementations
```

## Key Patterns

- **CGO required**: mattn/go-sqlite3 needs a C compiler (`xcode-select --install` on Mac)
- **TDD (strict)**: Always write a failing test before implementing. Red → Green → Refactor. No production code without a covering test.
- **Functional options**: `brain.WithImportance(0.8)`, `brain.WithContent("updated")`
- **Building-block CLI**: Low-level commands composed by the calling agent
- **Pre-commit hooks**: Install with `pre-commit install` (requires [pre-commit](https://pre-commit.com/))

## Cerebro Memory System

This environment uses Cerebro for persistent memory across sessions.

> Using Claude Code? The **cerebro plugin** is the preferred installation path
> (`/plugin marketplace add coetzeevs/cerebro`, then `/plugin install cerebro@cerebro`).
> `cerebro init` (which wrote this section) remains fully supported as the
> cross-tool fallback; the two coexist safely — lifecycle hooks are
> session-guarded in the binary and fire exactly once per session.

### Capability map (what cerebro can do — reach for these without running help)
<!-- cerebro:capabilities:begin -->
Always pass `-p "$CLAUDE_PROJECT_DIR"` (EDP estates: `-p "${EDP_BRAIN_ROOT:-$CLAUDE_PROJECT_DIR}"`). `--format json` for structured output. Full detail: `cerebro usage` or `cerebro <cmd> --help`.

**Remember (write)**
- learned something worth keeping → `cerebro add "<content>" -t episode|concept|procedure|reflection -i 0.0-1.0 [--subtype s]` — search first, reconcile (see supersede/update/reinforce)
- a file proves the memory → `cerebro add ... --anchor <path> [--anchor-ref <sha>]` — cites the source; recall reports verified|stale|missing
- new info contradicts/replaces a memory → `cerebro supersede <old-id> "<new content>" -t <type>` — old stays as history, new takes over
- refine an existing memory in place → `cerebro update <id> --content|--importance|--subtype`
- a memory proved right again (no new info) → `cerebro reinforce <id>` — boosts retention
- unsure it belongs in the brain yet → `cerebro inbox add "<content>"` — quarantined until `inbox approve <id>` / `inbox discard <id>`; `inbox list` to review
- two memories are related → `cerebro edge <src-id> <dst-id> <relation>` — use `cerebro relation list` names; register new ones deliberately
- curate the relation vocabulary → `cerebro relation add <name> [--class c] | list | rm <name>`

**Retrieve (read)**
- need context on a topic → `cerebro recall "<query>"` — composite-scored, THE default retrieval; `--prime` for session-start selection
- pure semantic similarity → `cerebro search "<query>" [--limit N --threshold 0.x]` — vector lane only
- inspect one memory + its edges → `cerebro get <id> [--with-provenance] [--as-of <time>]` — JSON carries origin/provenance/anchor status
- browse or filter → `cerebro list [--type t] [--status s] [--subtype x]`

**Feedback (close the loop — do this after acting on a recall)**
- a recalled memory helped → `cerebro outcome <id> --success` — boosts its future ranking
- a recalled memory misled you → `cerebro outcome <id> --failure` — sinks it (and consider supersede)

**Distill & maintain**
- many episodes accumulated → `cerebro consolidate --suggest` to see clusters; synthesize a concept with add, then `cerebro consolidate --into <new-id> <episode-ids...>` (wires provenance, marks sources)
- episodes distilled elsewhere → `cerebro mark-consolidated <ids...>` — status flip only, no provenance
- scrub a subject before sharing a brain → `cerebro forget --subject "<pattern>" [--subtype s] [--hard]` — DRY-RUN by default; add --apply to execute
- vectors missing (import, embed failures) → `cerebro embed --pending` — backfills; oversized content chunks automatically
- evict decayed memories → `cerebro gc [--dry-run]` — score-based, archives
- brain health check → `cerebro stats` — counts, schema, pending embeddings

**Share & lifecycle**
- memory useful across all projects → `cerebro promote <id>` — copies to the global brain with provenance
- move/backup brains → `cerebro export [--format json|sql|sqlite]` — full-fidelity bundle
- restore/merge a bundle → `cerebro import <file> [--on-conflict skip|replace]`
- snapshot before risky work → `cerebro backup`
- consolidate duplicate brains / formats → `cerebro migrate --realpath-hashes [--dry-run]`
- new project setup → `cerebro init -p <dir>` — brain + hooks + skills + this capability map in CLAUDE.md

**Operator/infra (rarely agent-invoked)**
- see this map again → `cerebro usage`
- tune per-brain defaults → `cerebro config list|get|set` — thresholds, seams (e.g. indegree_bonus_enabled)
- recall-quality measurement → `cerebro eval` — A/B protocol in docs/evals/README.md
- session metrics → `cerebro ingest`
- metrics dashboard → `cerebro dashboard`
- lifecycle hooks (wired by init/plugin) → `cerebro hook prime|post-compact|session-end` — session-guarded, not for manual use
- premature-stop detector (opt-in, default off) → `cerebro stop-guard` — inert unless stop_guard_enabled=true AND a Stop hook is wired
- Pi runtime config snippet → `cerebro pi-init`
<!-- cerebro:capabilities:end -->

### Automatic behavior
- Session start: recent memories are loaded via hook (reliable additionalContext channel)
- First prompt fallback: if the session-start prime was not delivered, memories are injected on your first prompt
- Post-compaction: the primed flag is cleared so memories re-load after compaction
- Session end: garbage collection and metrics ingest run automatically

### Post-compaction recovery
If you don't see Cerebro memories in your context after compaction (no primed memories in system reminders), proactively run `/recall` to restore context.

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

### Close the loop
After acting on a recalled memory, record the outcome: `cerebro outcome <id> --success` when it helped, `--failure` when it misled (then consider superseding it). This is how the brain learns which memories to surface.

### Origin identity
Memory writes are stamped with origin identity (who/what wrote them). Set
`CEREBRO_ORIGIN_ACTOR` (e.g. `claude-code`) in the environment so agent writes
classify `recorded`; the session id flows automatically via `CLAUDE_SESSION_ID`.

### Configuration
Per-brain defaults can be customized with `cerebro config set <key> <value>`.
Run `cerebro config list` to see available settings. CLI flags always override brain config.

## Conventions

- Keep test fixtures in `testdata/` directories
- Use `t.TempDir()` for test databases — no cleanup needed
- Node types: `episode`, `concept`, `procedure`, `reflection`
- Format flag: `--format md` (default) or `--format json`
- Every PR must include a CHANGELOG.md entry in the same branch
- Git tags must have a meaningful annotation summarizing the work shipped — don't just repeat the version number
- `cerebro pi-init -p <dir>` emits a deterministic `pi.config.json` snippet for the `pi-cerebro` Pi extension; stdout is pure JSON, stderr carries status messages only (HS-007)
- `pi-cerebro/` is a polyglot TypeScript subdirectory (package `@coetzeevs/pi-cerebro`) that lives inside the Go cerebro repo. It is a Pi extension (not a cerebro Go subcommand). Test runner: `node:test` with glob `tests/**/*.test.mjs` (ADR-0001 stack-frame default). TypeScript strict + NodeNext. Stub binaries in `pi-cerebro/tests/fixtures/` are invoked via PATH prepend — executable bit (0755) required. Shell-out discipline: all `execFileSync` calls use argv-array form, no `shell: true`. (HS-009)
- `cerebro migrate --realpath-hashes` is the one-shot migration to consolidate pre-HS-008 duplicate brains created under unresolved-path hashes. Run once after upgrading to HS-008; idempotent. Use `--dry-run` first to preview. (HS-008)

## Task Tracking with Beads

Role boundaries (memory/tasks/planning), forbidden tool uses, and the durable-handoff rules now live in the ontology at `/Users/q/projects/agentic/documentation/Operational Ontology.md` (§3, §4, §5, §7). See `AGENTS.md` for the project-level pointers.

Project-local convention: `bd dolt push` after each session-completing close (sync Beads data to Dolt remote before stopping).
