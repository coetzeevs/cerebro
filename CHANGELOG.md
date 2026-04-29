# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

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

