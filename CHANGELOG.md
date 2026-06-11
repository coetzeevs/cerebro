# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Fixed [agentic-7r28]

- `cerebro eval` no longer silently writes an all-zero baseline when the target brain contains none of the ground-truth nodes (the "wrong `-p`" case). It now aborts with a clear non-zero error (`eval aborted: none of the ground-truth node IDs resolve as active nodes in the target brain …`) before writing anything. A second defence-in-depth guard refuses any zero-query baseline. New pure helper `countResolvableGroundTruth` covers the condition (unit + preflight-integration tests). [agentic-7r28]
- `cerebro eval --out` now defaults to a gitignored scratch path (`docs/evals/baseline.local.json`) instead of the committed reference (`docs/evals/baseline.json`), so a bare `cerebro eval` can never clobber the committed baseline; updating the committed reference is now a deliberate `--out docs/evals/baseline.json`. [agentic-7r28]

### Added [agentic-x183]

- `cerebro eval` subcommand: recall-quality evaluation harness for the composite scorer. Measures recall@5, recall@10, recall@20, and MRR against a hand-authored ground-truth corpus. Flags: `--queries`, `--ground-truth`, `--corpus`, `--out`, `--limit`, `--threshold`. Drives the unmodified `Brain.Search(limit=20, subtypeFilter=nil)` pipeline; reads brain stats at runtime (no hardcoded node count). Pure metric helpers `computeRecallAtK` and `computeMRR` are unit-tested with synthetic inputs — no Ollama required for the test suite. Ground-truth AC2b preflight validates node IDs against the live `nodes` table; pruned IDs are reported to stderr and skipped from recall denominators. Scorer weights (`relevance=0.35, importance=0.25, recency=0.25, structural=0.15`) recorded in `baseline.json` (AC4 contract). [agentic-x183]
- `docs/evals/`: eval corpus — `queries.jsonl` (10 sanitised queries), `ground-truth.jsonl` (opaque UUID ground-truth sets), `corpus.md` (brain path-class + provenance manifest), `README.md` (AC5 documentation + public-repo content-hygiene caution), `baseline.json` (committed baseline metrics), `.gitignore` (S-INFO-1 guard against raw SQLite blob commits). [agentic-x183]
- `docs/adrs/ADR-011-recall-quality-eval-harness.md`: records three architectural decisions — `cerebro eval` vs standalone binary vs `go test` harness; committed node-IDs + live-validation vs frozen DB snapshot; macro recall@K + MRR without per-component contribution (deferred). [agentic-x183]

### Changed [HS-020]

- Added `pi-cerebro/.npmrc` at project root with three hardened defaults: `audit-level=moderate` (gates `npm audit --audit-level=moderate` exit code — see S-LOW-1 note below), `engine-strict=true` (enforces `engines.node >=24.0.0` at `npm install`/`npm ci` time), `ignore-scripts=true` (suppresses transitive postinstall scripts). File is byte-identical (sha256: `6c40bb0881cd2146f534c67fbf1cbb0db6d42cf465c43b5f434dd26be488ebdd`) across all four repos per S-INFO-3 and AC5 (no `registry=` / no `_authToken`). Discharges HS-001 Security Review deferred-audit item 6. Cross-repo engine divergence (intentional): pi-cerebro `>=24.0.0` (node:sqlite requires Node 24 unflagged, HS-039); pi-claude-code-cli `>=22.6.0` (`@types/node: ^22.0.0`). **S-LOW-1 note (npm 8+ semantics):** In npm 8+, `audit-level=moderate` gates `npm audit` exit code, NOT `npm install`/`npm ci`. AC1 is satisfied via `npm audit --audit-level=moderate --omit=dev`. [HS-020]

### Added [HS-046]

- pi-cerebro: `cerebro_remember` now enforces strict-reject when calling agent's session has a `currentBeadsId` binding but the effective beadsId for the write is NULL — sibling enforcement to HS-045 at the memory-write boundary. Hard MCP error replaces silent untagged storage. Error message: `Cannot create unlinked memory: session is bound to bd ticket <id>. Pass beadsId explicitly OR call bd_clear to release the session binding.` (`bd_clear` is forward-looking — see HS-050 backlog.) Implementation: new `enforceCerebroBoundBeadsId` + `CEREBRO_BEADS_ENFORCEMENT_ERROR` const + `formatCerebroBeadsEnforcementError` helpers in `pi-cerebro/src/session-context-reader.ts`. Gate fires only when session is bound — unbound sessions proceed unchanged (HS-039 back-compat). 5 new tests in `pi-cerebro/tests/cerebro-remember-enforcement.test.mjs`. [HS-046]

### Added
- `cerebro add --beads-id <id>`: new flag to tag a persisted memory with the active beads task id, stored as `{"beadsId":"..."}` in the node's existing `metadata` JSON column (zero schema migration). Input trimmed then validated against the HS-029 canonical regex `^[a-z][a-z0-9-]{0,31}-[0-9a-z]{3,32}$` (propagation site 3; byte-identical to `validate-hello-stack.sh:243` and `validate-meepo-beads-link.sh:79`). Empty/whitespace-only post-trim → flag treated as absent (no metadata write). Non-matching value → CLI error with canonical pattern in message (N-S1). JSON encoded via `json.Marshal` (not `fmt.Sprintf`). Stale-metadata merge contract documented: if a future `--metadata` flag is added, `beadsId` wins on key collision. [HS-039]
- `pi-cerebro` session-context reader (`src/session-context-reader.ts`): new `readCurrentBeadsId()` helper reads the calling agent's `currentBeadsId` slot from Meepo's `subagents.db` session-context substrate (HS-036 schema) via `node:sqlite` read-only cross-extension access. Agent identity resolved from `process.env.PI_TMUX_AGENTS_CHILD_ID` (`||` guard per HS-009 convention). Best-effort: any error (DB missing, env unset, no row, schema mismatch) logs a sanitised warning and returns `null` — `cerebro_remember` must never fail due to session-context unavailability (AC3 back-compat). `getAgentDir()` path resolution supports `PI_CODING_AGENT_DIR` env var override for testing. [HS-039]
- `pi-cerebro` `cerebro_remember` execute now reads `currentBeadsId` via `readCurrentBeadsId()` and passes it to `runAdd` as the new `beadsId` parameter. `runAdd` appends `["--beads-id", beadsId]` as separate positional argv tokens when non-null (TL-N2 argv-array discipline; never inline-concat). Top-level agents without `PI_TMUX_AGENTS_CHILD_ID` store memories without beadsId tagging (AC3 back-compat). [HS-039]
- pi-cerebro heuristic compaction detector: `message_end` hook watches `sessionManager.getEntries().length` and, when entries drop by more than 50% in a tick, logs `[pi-cerebro] compaction detected: re-priming memories` and re-invokes `cerebro recall --boot`. Threshold lives as `COMPACTION_DROP_RATIO` constant in `pi-cerebro/src/compaction.ts`. Per Operational Ontology §5.5 Decision Q1; closes the Stage 2 G2 gate dependency for HS-016 `validate-cerebro-pi.sh`. [HS-010]
- `pi-cerebro` TypeScript Pi extension package (`@coetzeevs/pi-cerebro`) in `pi-cerebro/`. Registers `cerebro_recall` and `cerebro_remember` LLM-callable tools and a `session_start` hook that boot-primes recent memories into the agent's context via `cerebro recall --boot`. Fail-fast binary validation with stale-shim defence (`/\d+\.\d+/` version-pattern check). All shell-outs use argv-array form; nodeId capture bounded by `/^([0-9a-f]{8,64})/m`; external output sanitised to 200 chars; `CLAUDE_PROJECT_DIR` guard uses `||` to treat empty string as absent. CI job `pi-cerebro-ts` added on Node 22.6.0 (ADR-0001 floor). [HS-009]
- `pi-init` subcommand emits a deterministic `pi.config.json` snippet for the `pi-cerebro` extension. Resolves the project path via `filepath.EvalSymlinks` per Ontology §5.14 (rule 26), verifies or creates the brain at `~/.cerebro/projects/<sha256(realpath)>.sqlite`, prints structured JSON to stdout, and emits status to stderr only. Idempotent: second run produces byte-identical output. [HS-007]
- `cerebro migrate --realpath-hashes` consolidates legacy duplicate brains created before HS-008 by scanning project paths, recomputing the realpath-based hash, and either renaming the old brain file (Case A) or merging it into the realpath-keyed brain (Case C). Walks `$HOME` (or `--scan-root <path>`, repeatable) to `--max-depth 4` by default. `--dry-run` previews changes without mutating files. Mandatory backup of both source and destination before merge to `~/.cerebro/backups/migrate-<timestamp>/`. Acquires `~/.cerebro/migrate.lock` to prevent concurrent migration. Skips symlinked directories in the walk (CWE-59 guard). Idempotent — second run reports "Nothing to migrate". [HS-008]

### Changed
- `pi-cerebro` Node engine bumped from `>=22.6.0` to `>=24.0.0`. CI workflow `pi-cerebro-ts` job now runs on Node `24.x` (was `22.6.0`). Reason: `node:sqlite` (used by `session-context-reader.ts` for cross-extension read of Meepo's `subagents.db`) is gated behind `--experimental-sqlite` on Node 22.x and only unflagged from Node 24. CI on 22.6.0 was failing all tests transitively importing `src/index.ts` with `error: 'No such built-in module: node:sqlite'`. [HS-039]

### Fixed
- `pi-cerebro` `runBootPrime` was shelling out to `cerebro recall --boot`, a flag that cerebro never shipped. Corrected argv to `cerebro recall --prime` (the actual cobra binding at `cmd_recall.go:35`). Both call sites are covered by the single-site fix: `session_start` handler and `message_end` compaction re-prime (HS-010). Regression test `tests/run-boot-prime-argv.test.mjs` added asserting the argv contains `--prime` and does not contain `--boot`. Fixes Hello Stack G5 blocker — `validate-cerebro-pi.sh` G2 grep can now match `[pi-cerebro] session_start: primed N memories.`. [HS-024]

### Changed
- `brain.ProjectPath` now resolves symlinks via `filepath.EvalSymlinks` before hashing (Operational Ontology §5.14, rule 26). On macOS, `cerebro -p /tmp/myproject` now opens the same brain as `cerebro -p /private/tmp/myproject`. Public Go signature is unchanged; behaviour for callers passing symlinked paths changes: their brains move to a new file on disk. Run `cerebro migrate --realpath-hashes` once to consolidate. Falls back to `filepath.Abs` for nonexistent paths (pre-HS-008 behaviour preserved for CI-bootstrap callers). Not BREAKING. [HS-008]
- `internal/metrics.MetricsPath` now resolves symlinks via the same EvalSymlinks-with-fallback pattern as `brain.ProjectPath` (N1 stack-wide fold-in). Without this fix, `.metrics.sqlite` files for `/tmp/proj` and `/private/tmp/proj` would remain separate post-HS-008. Both functions are kept intentionally decoupled (separate implementations, same algorithm). [HS-008]

## [2.0.0] - 2026-05-16

### Added
- `update`, `list`, `recall`: new `--subtype` flag for setting/filtering memory subtypes [OO-011]
  - `cerebro update <id> --subtype <value>` sets subtype on an existing node; stamps `updated_at`
  - `cerebro update <id> --subtype ""` clears the subtype to NULL
  - `cerebro list --subtype <value>` filters list output by subtype (exact match)
  - `cerebro list --subtype ""` filters to nodes with no subtype (NULL rows only)
  - `cerebro recall <query> --subtype <value>` filters semantic results by subtype after composite scoring and graph expansion
  - All flags compose with existing filters (`--type`, `--status`, `--limit`, `--threshold`)
  - Subtype changes via `update` stamp `updated_at` (knowledge-classification metadata semantics)
  - Backward compatible: omitting the flag yields the pre-OO-011 behaviour exactly
  - `--subtype` is ignored in `--prime` mode (prime uses stratified/MMR selection, not query mode)

### Changed
- **BREAKING**: `Brain.Search` and `Brain.SearchWithGlobal` (in `brain/brain.go`) now accept a
  `subtypeFilter *string` positional parameter as the last argument. Pass `nil` to preserve
  pre-OO-011 behaviour. All 5 in-repo callers updated (`cmd_search.go`, `cmd_recall.go` ×2,
  `brain_test.go` ×2). External consumers (e.g. `qraftworx-cli`) must update their wrappers
  (follow-up ticket: QWX-001). [OO-011]
- `AGENTS.md`: bump `Ontology version:` pin from `1.1` to `1.4` to reflect §7 rules 27, 28, 29, and 30 added in Ontology v1.2 → v1.4. No behavioural change in cerebro; satisfies the Ontology §9 "no project pin lags more than one minor version" invariant. [OO-012]

### Migration
- Go consumers calling `brain.Brain.Search(...)` or `brain.Brain.SearchWithGlobal(...)`: append `nil` as the final argument to preserve pre-2.0.0 semantics, or pass a `*string` pointing at the subtype to filter results post-scoring. Example: `b.Search(ctx, q, 10, 0.3, nil)`. See QWX-001 for a worked consumer-side absorption against `qraftworx-cli`.

## [1.13.0]

### Fixes
- `stop-guard`: false positives on legitimate approval gates and risky operation confirmations
  - Workflow gates (e.g. `/develop` Phase 3 plan review) no longer blocked
  - Risky operation confirmations (push, deploy, delete, merge) no longer blocked
  - `stop_hook_active` safety valve prevents infinite blocking loops
  - Lazy permission-seeking ("Shall I also update the docs?") still blocked
- Global `~/.claude/settings.json` Stop hook now delegates to `cerebro stop-guard` instead of inline bash (single implementation, fails safe if cerebro not on PATH)

## [1.12.0] - 2026-04-24

### Features
- Dashboard Detail panel (3): full metrics for selected turn, quality flags, individual tool calls
  - Select a turn from Turns panel and press Enter to view
  - Quality flags: [!] ZERO THINKING, [!] EDIT WITHOUT READS, [!] STOP GUARD FIRED
  - Shows individual tool calls with file paths and cerebro operations
- Dashboard Tools panel (4): tool distribution bar chart with percentages, cerebro operations breakdown
- Dashboard Trends panel (5): daily sparklines (R:E, zero-think %, cache hit %, stop-guard, turn volume) + 14-day summary table
- New store methods: `ToolDistribution()`, `ToolCallsForTurn()`

## [1.11.0] - 2026-04-24

### Features
- `cerebro dashboard` — interactive full-screen performance metrics TUI
  - Overview panel: session summary, quality sparklines (R:E, thinking, cache, stop-guard), brain health
  - Turns panel: scrollable table of per-turn metrics with anomaly highlighting
  - Live refresh every 5 seconds via incremental JSONL re-parse
  - Tab navigation (1-5), keyboard controls (j/k scroll, r refresh, q quit)
  - Alt-screen mode (full terminal takeover, clean restore on exit)
- Bubble Tea v2 ecosystem added (bubbletea v2.0.6, lipgloss v2.0.3, bubbles v2.1.0)
- New `internal/dashboard/` package

### Miscellaneous
- Binary size increase: 13MB → 15MB (Charm stack is pure Go, no CGO)

## [1.10.1] - 2026-04-24

### Bug Fixes
- Fix `cerebro stats --metrics` showing all underscores for Read:Edit and Thinking sparklines
  - Add 10-turn sliding window R:E ratio alongside per-turn ratio (reads and edits rarely co-occur in the same turn)
  - Use thinking blocks (boolean: did thinking occur?) instead of thinking chars (always 0 when content is redacted)
  - Show Think Depth sparkline conditionally when thinking content is non-empty
  - Fix query ordering: chronological timestamp ordering instead of per-session turn_number
  - Add "(redacted)" annotation when thinking content is infrastructure-redacted
- Add `OrderField` type to `TurnFilter` for explicit timestamp vs turn_number ordering (prepares for dashboard Phase C)

## [1.10.0] - 2026-04-23

### Features
- `cerebro stats --metrics` shows inline Unicode sparklines for per-turn quality signals
  - Read:Edit ratio, thinking depth, cache hit rate, stop-guard fires
  - Summary line with aggregate stats (avg R:E, zero-think %, cache %, stops blocked)
  - `--last N` flag controls how many recent turns to display (default 50)
- Sparkline renderer (`internal/metrics/sparkline.go`) — reusable Unicode block character renderer for both CLI and future dashboard

## [1.9.1] - 2026-04-23

### Bug Fixes
- Fix silent data loss in incremental JSONL parsing: turn counter reset to 0 on each parse, causing `INSERT OR IGNORE` to silently drop new turns that collided with existing turn numbers
- Replace `INSERT OR IGNORE` with `INSERT OR REPLACE` for turn_metrics so re-parsing growing sessions overwrites partial turns with complete data
- Delete and re-insert tool_calls per session on re-parse to prevent duplicate accumulation
- Eliminate offset-based incremental seeking (premature optimization that caused partial-line boundary issues); always parse from byte 0, use file size only for change detection

## [1.9.0] - 2026-04-23

### Features
- `cerebro ingest` command for collecting per-turn performance metrics from Claude Code session files
  - Parses JSONL session files incrementally (byte offset tracking, idempotent)
  - Extracts per-turn: tool usage, token consumption, thinking depth, cerebro operations
  - Per-tool-call detail table for full analytical flexibility (tool name, file path, cerebro op)
  - Separate metrics database (`~/.cerebro/projects/<hash>.metrics.sqlite`) — isolated from brain data
  - Auto-discovers Claude Code session directory from project path
- `evalStopGuard` now returns matched category name for metrics correlation
- SessionEnd hook template updated to run `cerebro ingest` automatically after GC

### Miscellaneous
- New `internal/metrics/` package: store, schema, types, JSONL parser, tool classification
- Test fixtures for JSONL parsing in `internal/metrics/testdata/`

## [1.8.0] - 2026-04-23

### Features
- `cerebro stop-guard` subcommand for Stop hook quality enforcement
  - Reads Claude Code Stop hook JSON from stdin, checks `last_assistant_message` against phrase patterns
  - Three phrase categories with specific corrective guidance: permission-seeking, premature-stopping, scope-reduction
  - Uses JSON decision protocol (exit 0 + `{"decision": "block", "reason": "..."}`)
  - Cross-platform Go implementation — no shell/python dependencies
- `/develop` skill template scaffolded by `cerebro init`
  - 5-phase structured workflow: context, research, plan (approval gate), execute, verify
  - `effort: high` frontmatter forces deep reasoning when skill is active
  - Universal — no project-specific dependencies (Jira, swarm agents, etc.)
- Stop hook added to settings.json template — auto-added to existing projects on next `cerebro init`

## [1.7.0] - 2026-04-22

### Features
- `cerebro config` command for per-brain configuration (set/get/list/reset)
- Brain-local config stored in `schema_meta` — no external config files needed
- Config keys: `prime_limit`, `gc_threshold`, `search_limit`, `search_threshold`, `recall_threshold`
- Precedence: CLI flag > brain config > compiled default
- Config values travel with the brain via export/import
- `recall` command now has `--threshold` / `-T` flag (was hardcoded at 0.3)
- `cerebro init` output now includes a config hint

### Miscellaneous
- GC hook template no longer hardcodes `--threshold 0.01` — uses brain config or compiled default
- Recall skill template no longer hardcodes `--limit 10` — uses brain config or compiled default
- CLAUDE.md template updated with configuration section
- `store.DeleteMeta()` method added for config reset support

## [1.6.2] - 2026-04-22

### Bug Fixes
- settings.json marshaling no longer unicode-escapes shell operators (`&`, `>`, `<`) making commands unreadable ([#28](https://github.com/coetzeevs/cerebro/pull/28))

## [1.6.1] - 2026-04-22

### Bug Fixes
- `--force` flag now replaces settings.json cerebro hooks with latest template (previously skipped) ([#27](https://github.com/coetzeevs/cerebro/pull/27))
- `--force` CLAUDE.md replacement no longer destroys independent sections (e.g. `## Conventions`) that follow the Cerebro section ([#27](https://github.com/coetzeevs/cerebro/pull/27))
- All skill templates (remember, recall, consolidate) now pass `-p "$CLAUDE_PROJECT_DIR"` to every cerebro command ([#27](https://github.com/coetzeevs/cerebro/pull/27))

### Documentation
- Add harness management research doc (`docs/research/harness-management-research.md`)

## [1.6.0] - 2026-04-22

### Features
- Deterministic session confirmation via JSON `systemMessage` hook output — user sees "Cerebro online. N memories loaded." in terminal on session start without depending on Claude following instructions
- `CLAUDE_ENV_FILE` propagation: startup hook writes `CLAUDE_PROJECT_DIR` to session env so skills resolve the correct brain even when cwd drifts
- All SessionStart matchers (startup, resume, compact, clear) and UserPromptSubmit now include env propagation and memory count

### Documentation
- Fix README quick start examples (removed non-existent `--name`, `--body` flags, corrected `edge` syntax)
- Fix README brain path description, broken ADR-006 link, add missing `backup` command
- Add Memory types, Embedding providers, and Project vs global store sections to README
- Update ADR statuses 001-007 from "Proposed" to "Accepted"
- Update system-architecture.md from v0.2.0-Draft to v1.0.0-Accepted
- Fix SECURITY.md env var (`VOYAGE_API_KEY` → `CEREBRO_VOYAGE_API_KEY`)
- Fix CONTRIBUTING.md CI checks list (remove non-existent `govulncheck`)
- Fix CHANGELOG v1.5.2 duplicate bug fix entry

## [1.5.2] - 2026-04-21

### Features
- `--force` flag on `cerebro init` now also replaces the CLAUDE.md Cerebro section with the latest template

## [1.5.1] - 2026-04-21

### Bug Fixes
- Run schema migration before apply in Init() for v1 databases (#23) ([a2d3f28](https://github.com/coetzeevs/cerebro/commit/a2d3f28)) ([#23](https://github.com/coetzeevs/cerebro/pull/23))

## [1.5.0] - 2026-04-21

### Features
- Session-pinned brain resolution via CLAUDE_PROJECT_DIR (ADR-008) (#20) ([5df4722](https://github.com/coetzeevs/cerebro/commit/5df4722)) ([#20](https://github.com/coetzeevs/cerebro/pull/20))
  - `resolveProjectDir()` checks `--project` flag > `CLAUDE_PROJECT_DIR` env var > cwd
  - All skill templates now pass `-p "$CLAUDE_PROJECT_DIR"` to every cerebro command
  - CLAUDE.md template includes project directory usage instructions
- Safe init with automatic backup and `cerebro backup` command (ADR-009) (#21) ([10cca42](https://github.com/coetzeevs/cerebro/commit/10cca42)) ([#21](https://github.com/coetzeevs/cerebro/pull/21))
  - Automatic backup before re-initializing an existing brain (`~/.cerebro/backups/`)
  - New `cerebro backup` command for on-demand backups (supports `-o` for custom path)
  - New `--force` flag on `cerebro init` to overwrite existing skill templates
  - Improved init output distinguishing fresh vs re-initialization

## [1.4.0] - 2026-04-14

### Features
- Recall recency enhancement (ADR-007, Phases 1-3) (#19) ([9d82a69](https://github.com/coetzeevs/cerebro/commit/9d82a69)) ([#19](https://github.com/coetzeevs/cerebro/pull/19))

## [1.3.0] - 2026-04-07

### Documentation
- Cleanup stale qraft-cli docs, add SECURITY.md and CONTRIBUTING.md (#18) ([37d5ed9](https://github.com/coetzeevs/cerebro/commit/37d5ed9)) ([#18](https://github.com/coetzeevs/cerebro/pull/18))

## [1.2.0] - 2026-03-27

### Features
- Re-export internal/store types from brain/ for external consumers (#17) ([06ad734](https://github.com/coetzeevs/cerebro/commit/06ad734)) ([#17](https://github.com/coetzeevs/cerebro/pull/17))

Type aliases added to `brain/types.go`: `Node`, `Edge`, `ScoredNode`, `NodeWithEdges`, `NodeType`, `ListNodesOpts`, `Stats`, `GCResult`, `ExportBundle`, `ImportOptions`, `ImportResult`, `ConflictStrategy`. Constants: `Episode`, `Concept`, `Procedure`, `Reflection`, `ConflictSkip`, `ConflictReplace`, `ExportVersion`.

This enables external Go modules (e.g., [qraftworx-cli](https://github.com/coetzeevs/qraftworx-cli)) to use `brain/` without importing `internal/store/`.

## [1.1.1] - 2026-03-27

### Miscellaneous
- Revert changelog commit approach ([f968b3f](https://github.com/coetzeevs/cerebro/commit/f968b3f))

## [1.1.0] - 2026-03-21

### Features
- Fix hook reliability, add short ID prefix resolution (#13) ([92c613c](https://github.com/coetzeevs/cerebro/commit/92c613c5481587bfd36c412e9fb31103959645d2)) ([#13](https://github.com/coetzeevs/cerebro/pull/13))

## [1.0.2] - 2026-03-11

### Bug Fixes
- Ad-hoc codesign binaries and strip quarantine on install (#12) ([c94a8ec](https://github.com/coetzeevs/cerebro/commit/c94a8ecb0eb766b642d6e2dee5c537c861257f75)) ([#12](https://github.com/coetzeevs/cerebro/pull/12))

## [1.0.1] - 2026-03-11

### Miscellaneous
- Configure Homebrew cask publishing via GoReleaser (#11) ([aed32e2](https://github.com/coetzeevs/cerebro/commit/aed32e2b4548c0fcbfd92467bc2313b6727caf62)) ([#11](https://github.com/coetzeevs/cerebro/pull/11))

## [1.0.0] - 2026-03-11

### Bug Fixes
- Use cosine distance metric in vec0 for correct similarity scores ([f0299af](https://github.com/coetzeevs/cerebro/commit/f0299aff5f260cfceed965d57198b097f4a257b0))
- Check AddNode error returns in test to satisfy errcheck ([10216a5](https://github.com/coetzeevs/cerebro/commit/10216a50a331c9ee9c13064190b260e7194a4104))
- Lint issues (gofmt, rangeValCopy) and install pre-commit hooks ([0d921ac](https://github.com/coetzeevs/cerebro/commit/0d921acace407ac04265edb16c0febc0f5b8ad1a))
- Add compact and clear matchers to SessionStart hooks ([c87d206](https://github.com/coetzeevs/cerebro/commit/c87d206cb8f8d3aec251bf31dfa15794c695c075))
- Resolve lint issues surfaced by golangci-lint v2 ([e23d334](https://github.com/coetzeevs/cerebro/commit/e23d3342f80bd3245286e31250d06299dbcfebe3))
- Upgrade golangci-lint-action to v7 for lint v2 support ([c898c99](https://github.com/coetzeevs/cerebro/commit/c898c997dcb19ac052090b280bd499a2dbc0bd82))
- Golangci-lint v2 config and nolint explanations ([8ad89ba](https://github.com/coetzeevs/cerebro/commit/8ad89babbed76b8dedb9d586b8e3d865d96be716))

### Documentation
- Add future musings, v0 gaps, gitignore settings.local.json ([50c40d8](https://github.com/coetzeevs/cerebro/commit/50c40d8790f956208f1637304eed72cfd609f1e2))

### Features
- Integrate sqlite-vec for vector search ([9fceba8](https://github.com/coetzeevs/cerebro/commit/9fceba8820137537af25985decdb6ce590b6e027))
- Add Claude Code integration layer (hooks, skills, CLAUDE.md) ([f3ae7be](https://github.com/coetzeevs/cerebro/commit/f3ae7be86721b50de9517e79e473d1ad8b1aa2db))
- Support recall --prime without query for session-start priming ([7c49dbb](https://github.com/coetzeevs/cerebro/commit/7c49dbb6eb29cc97574f51c7286752b01c0abff6))
- Implement GC eviction, fix hooks, stratified recall --prime ([4ab2e8d](https://github.com/coetzeevs/cerebro/commit/4ab2e8dbcf67e1c0307d3d6bb7206e8025be1c13))
- Implement graph expansion for composite scoring ([9d32b8e](https://github.com/coetzeevs/cerebro/commit/9d32b8e95ed4f796cf5a307259a7c8c9374c36b2))
- Global store, promote command, and dual-store recall ([99c8792](https://github.com/coetzeevs/cerebro/commit/99c879219c4790ac7c6b027df76f5b845a1b82b2))
- Implement export and import commands ([5603f20](https://github.com/coetzeevs/cerebro/commit/5603f2072572a07ffcc7f4108b1d4dc975199a7a))
- Cerebro init bootstraps Claude Code integration ([de036df](https://github.com/coetzeevs/cerebro/commit/de036df8f26bdbc5f27e99ce6f8b41d228f070e8))

## [0.1.0] - 2026-03-06

### Documentation
- Initial architecture design for Cerebro agent brain ([40baaed](https://github.com/coetzeevs/cerebro/commit/40baaed4f14ff9f085b79d806995efd658b9623c))
- Resolve open questions — Go, opportunistic triggers, scoping ([b77a2a1](https://github.com/coetzeevs/cerebro/commit/b77a2a1d6c14c9b610201de4fd1f3858ea212759))
- Add Claude Code integration pattern (ADR-006) and align all docs ([66724fc](https://github.com/coetzeevs/cerebro/commit/66724fc5973203c6a7d28c5a0edfbef62a4fb80c))
- Add work tracking approach and dogfooding note to CLAUDE.md ([0580ec1](https://github.com/coetzeevs/cerebro/commit/0580ec191efa871f8c4ab70d278d744a67404ed0))

### Features
- Add Go scaffold with store, brain API, embedding providers, and CLI ([a43747c](https://github.com/coetzeevs/cerebro/commit/a43747ce864a0f6f8f8ee11055a2ec154e50105c))
- Add CI, pre-commit hooks, golangci-lint, goreleaser, and tests ([babd68e](https://github.com/coetzeevs/cerebro/commit/babd68efc018a878f82e39627777fac55772ce34))

### Miscellaneous
- Move init doc to subfolder and add note ([79e7c6a](https://github.com/coetzeevs/cerebro/commit/79e7c6ae6dd57e8a76799ef842b8535044b0865a))

