# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Added [agentic-k7dv]

- **Claude Code plugin.** `claude-plugin/cerebro/` ships the cerebro plugin: lifecycle hooks (session-start recall priming, post-compaction recovery, session-end GC) and the namespaced skills `/cerebro:remember`, `/cerebro:recall`, `/cerebro:consolidate`, `/cerebro:develop`, plus `/cerebro:rules` (the behavioral rules `cerebro init` appends to CLAUDE.md, loadable on demand since plugins cannot auto-load CLAUDE.md). A repo-root `.claude-plugin/marketplace.json` makes it installable via `/plugin marketplace add coetzeevs/cerebro` → `/plugin install cerebro@cerebro`. Plugin hooks self-gate on a brain existing for the project (silent elsewhere). The `stop-guard` Stop hook is deliberately NOT wired by the plugin (no per-hook disable exists in the plugin system; opt in via your own settings.json). `cerebro init` continues unchanged as the cross-tool fallback; its templates, README, and stdout now reference the plugin as the preferred Claude Code path. A lockstep test guards the plugin skills against drifting from the embedded init templates. [agentic-k7dv]
- **`cerebro hook prime|post-compact|session-end` — session-guarded lifecycle subcommands.** If both `cerebro init` hooks AND the plugin register the same lifecycle events, each event still fires its work exactly once per session: the binary records per-session-id state at `~/.cerebro/session-state/<session-id>.json` (session id from `$CLAUDE_SESSION_ID`) and no-ops repeats. `post-compact` clears the primed flag so the next prime re-fires (post-compaction context recovery); state files older than 7 days are reaped lazily on any write. Both the plugin's hooks and `cerebro init`'s settings template now route through these guarded commands. [agentic-k7dv]
- **Origin identity flows through the integration surfaces.** The `remember`/`consolidate` skill templates stamp writes with `CEREBRO_ORIGIN_ACTOR` (default `claude-code`) and `--origin-channel skill`, so agent-written memories classify `recorded`; documented in the CLAUDE.md template and README. [agentic-k7dv]

### Added [agentic-eq7a]

- **`cerebro consolidate --suggest [--limit N]` — consolidation candidate selection.** Surfaces rollup candidates (active episodes grouped by subtype, biggest groups first, oldest first within a group; group counts stay exact when the listing truncates) so the agent can synthesize concepts/procedures/reflections and consolidate each cluster with the existing atomic `--into` (derived_from provenance + source marking in one transaction). Model B throughout: the agent synthesizes, cerebro selects and wires. The `consolidate` skill template now uses the atomic `--into` instead of manual `learned_from` edges + `mark-consolidated`, and teaches the relation-registry discipline for new edge relations. Live A/B on the 604-node eval brain (18 queries, same-session pair): a real 3-episode consolidation pass left recall@10/20 unchanged and improved recall@5 0.7685 → 0.8426 (MRR 0.6585 → 0.6326). [agentic-eq7a]

### Fixed [agentic-3xz9]

- **`stop-guard` no longer forces continuation past legitimate human-confirmation gates.** A first-class confirmation-gate detector now runs BEFORE every premature-stop category: a message that stops to request human confirmation for an irreversible or outward-facing action (push, merge, deploy, destructive apply, delete) is always allowed through — even when it also matches a premature-stop pattern (the exact defect: "for now, I've held off on the force-push" was classified as scope-reduction and blocked). Fixture suite covers both classes with real phrasings, including the EDP estate's "never push without confirming" gate messages and the delivery-discipline "Decisions required" shape. Help text now states the guard is a premature-stop detector, not a memory-persistence mechanism. [agentic-3xz9]

### Changed [agentic-3xz9]

- **`stop-guard` is now disabled by default, everywhere** (operator ruling 2026-08-25: the blocking proved inefficient and its matching overly strict in live use). The guard evaluates only when the brain config flag `stop_guard_enabled` is the literal `"true"` (`cerebro config set stop_guard_enabled true` — strict opt-in mirroring the `rerank_enabled` gate); otherwise, and when no brain resolves, it emits the allow decision without evaluating. Fail-open here is fail-safe: the guard's only power is to force continuation, which is exactly the harm being ruled out. The Stop hook is no longer wired by `cerebro init` (its settings template drops the entry; `cerebro init --force` removes it from previously scaffolded projects) nor by the plugin. Enabling is a deliberate two-step opt-in: wire the Stop hook manually AND set the flag. [agentic-3xz9]

## [3.2.0] - 2026-08-25

### Added [agentic-goc7]

- **Origin identity on memory nodes** (schema v6). Four nullable columns — `origin_actor`, `origin_channel`, `origin_session`, `origin_host` — record who/what wrote a memory, through which channel, from which session and host. Stamped at write time, never inferred: the CLI derives only observed facts (channel `cli`, hostname, `$CEREBRO_ORIGIN_ACTOR`, `$CEREBRO_ORIGIN_SESSION`/`$CLAUDE_SESSION_ID`), overridable per-write with `--origin-*` flags on `add`/`supersede`. `supersede` records the superseder's identity; `promote` carries the original author's; `import` stamps origin-less bundle nodes `actor/channel="import"` so entry provenance is never silently blank. Origin survives the JSON-bundle and SQL-dump round-trips, and both search lanes (vector + keyword) carry it into recall results. `get`/`list`/`recall` JSON surface the raw fields plus a computed `origin_status`: `recorded` (actor present), `legacy` (pre-convention node — absence expected), `unknown` (post-convention node with no actor — an honest gap). The v5→v6 migration is transactional and self-healing per the v4→v5 idiom; the one-time `origin_convention_since` boundary stamp classifies every pre-existing node `legacy`. Live rehearsal on a 603-node brain copy: 56 ms, node/edge counts exact. [agentic-goc7]

### Added [agentic-8l2g]

- **Typed-relation registry** for edge relations. A new `relations` table (seeded with `derived_from`, `supports`, `contradicts`, `supersedes`) names the relations a brain's edges are expected to use, with an optional free-form traversal class. `cerebro relation add <name> [--class <c>]` / `relation list` / `relation rm <name>` manage it; `cerebro edge` warns on stderr — never errors — when the relation is unregistered (the registry is advisory: warn-not-error, fail-open on lookup errors). Removing a relation touches only the registry; existing edges keep it. Go API: `RegisterRelation`, `ListRelations`, `RemoveRelation`, `RelationRegistered`. [agentic-8l2g]

## [3.1.0] - 2026-08-25

### Fixed [agentic-0p3w]

- **`export --format sql` now emits edge `valid_at`/`invalid_at`.** The SQL text-dump edge emitter still wrote the pre-xtzn 5-column form, so a dump → replay silently reset every edge's bi-temporal window to NULL/NULL. Bounds emit in the storage layout (`2006-01-02 15:04:05`), never RFC3339, because the as-of predicate compares raw strings. Round-trip guard covers literal format, instant equality, live as-of semantics on the replayed store, and NULL preservation. [agentic-0p3w]

### Added [agentic-zifu]

- **Keyword-lane fallback when the embedder is unavailable.** A query-embed failure (e.g. Ollama down) no longer takes recall down with it: `Search`/`SearchWithGlobal` degrade to BM25 keyword-only recall with a one-line stderr notice. BM25 rank is the ranking signal; scores degrade to the importance+recency terms (`store.RescoreKeywordOnly`); subtype filtering holds; no expansion/rerank/global lane on the fallback path. If FTS is also unavailable the original embed error returns. The `none` provider still errors (configured-out is not a failure). [agentic-zifu]

### Fixed [agentic-v5js]

- **Decay is per-day, as ADR-003 documented — not per-hour.** `compositeScore` and the GC `retentionScore` applied the per-type λ to *hours* since access, ~24× faster than the ADR's half-life table (episode: 4.6 h instead of 1–2 weeks), flooring the recency term for anything not touched today. Both now apply λ per day. Same-session A/B on the 603-node eval brain: recall@5 0.7315 → **0.7685**, MRR 0.6072 → **0.6585**, recall@10/20 unchanged. The GC retention floor (importance ≥ 2×threshold is un-evictable) is now documented in-code as a deliberate safety property for a curated store. [agentic-v5js]
- **Query-mode recall now generates the usage signal the scorer was designed around.** `recall`/`search` (query mode) touch `access_count`/`last_accessed` for returned nodes via a batched, best-effort `TouchAccessed` — after six months of live use every node's `access_count` was 0 because only explicit `reinforce` ever moved it. Prime mode still stamps `last_surfaced` only (a distinct signal, ADR-007), and the Go API's `Search` stays read-only so the eval harness cannot perturb the brain between A/B runs. [agentic-v5js]

### Changed [agentic-t3c9]

- **Recall-quality measurement is drift-proof.** The eval protocol now documents same-session disabled/enabled A/B pairs as the standard (the committed baseline is reference, never a gate — it drifts as the live brain grows), and `docs/evals/baseline.json` was regenerated on the released v3.0.0 binary at 603 nodes: the first production-binary eval (r@5 .7315, r@10 .8704, r@20 .9537, MRR .6072). [agentic-t3c9]

## [3.0.0] - 2026-08-25

### Changed [agentic-62uv]

- **Go toolchain raised to `go 1.26.7`** (from 1.25.0), clearing all stdlib vulnerabilities flagged by govulncheck on main (the 8 pre-existing at go1.25.0 incl. crypto/x509 GO-2026-4866, plus 5 newer CVEs still affecting at the ticket's original 1.26.4 target) — verified 0 affecting at 1.26.7. CI and goreleaser inherit the pin via `go-version-file: go.mod`. [agentic-62uv]

### Added [agentic-lbjg]

- **Structural provenance: a built-in `derived_from` relation.** Provenance is now structural, not freeform. A derived node (concept/procedure/reflection) can carry a built-in `derived_from` edge back to each source episode it was synthesized from. `derived_from` is reserved in code as a single exported Go constant (`store.RelationDerivedFrom`, re-exported as `brain.RelationDerivedFrom`); the typed-relation *registry* that seeds it on init is a separate ticket (agentic-8l2g). [agentic-lbjg]
- **`provenance_root` column + `schemaVersion` `4`→`5` migration.** A new `nodes.provenance_root INTEGER NOT NULL DEFAULT 0` flag marks a node as a first-class provenance source. The v4→v5 step is a transaction-guarded, self-healing `ALTER TABLE nodes ADD COLUMN` (guarded on the column's actual absence via `txColumnExists`, so a partially-migrated brain advances cleanly instead of crashing with "duplicate column"); existing rows backfill to `0` with no row rewrite. A freshly-`Init`ed brain is v5 with the column. `ADD COLUMN … INTEGER NOT NULL DEFAULT 0` is legal because the default is a non-NULL constant (live-probed on SQLite 3.51.2). [agentic-lbjg]
- **`cerebro add --provenance-root`** sets `nodes.provenance_root=1`; a flagless `add` still defaults to `0`. Threaded through `Brain.Add` via a new variadic `WithProvenanceRoot()` `AddOption` — **additive, no positional signature break**. [agentic-lbjg]
- **`cerebro consolidate --into <concept-id> <episode-id…>`** (NEW command). Flips each source episode to `consolidated` **and** auto-writes a `derived_from` edge from the `--into` node to each source, in a **single atomic transaction** (the edge upsert rides `tx.Exec` with the same `ON CONFLICT … DO UPDATE … RETURNING id` SQL as `AddEdge`, never the connection-level `AddEdge` inside the open tx). **Fail-closed:** the `--into` node and every source must resolve as an episode before any write, else a non-zero exit with **zero partial write** (rollback). **Idempotent** via `UNIQUE(source_id, target_id, relation)`. Distinct from `mark-consolidated` (status-flip only, no edges), which is left untouched. [agentic-lbjg]
- **`Store.WalkRelation(startID, relation, maxDepth, outgoing)`** — a new reusable multi-hop BFS traversal primitive (the store had only single-hop queries). Depth-bounded, cycle-safe via a node-ID visited set (self-loops terminate; each reachable node appears once at its minimum BFS depth), direction-parameterised, relation-filtered. **BFS-in-Go, not a recursive CTE:** a `WITH RECURSIVE … UNION` dedupes by `(node, depth)` row and re-walks a cycle to the depth cap (`A→B→C→A` with cap 5 emits `A@0 B@1 C@2 A@3 B@4 C@5`, live-proven on SQLite 3.51.2); the node-keyed visited set gives clean per-node-once semantics. `asOf` is `nil` in v1 (as-of provenance is a documented second-iteration feature). See `docs/adrs/ADR-016-provenance-edges-and-walk-primitive.md`. agentic-sx4u inherits this primitive. [agentic-lbjg]
- **`get --with-provenance[=depth]`** (bare ⇒ depth 5) and **`recall --with-provenance[=depth]`** (bare ⇒ depth 1) attach the `derived_from` lineage chain (walked outward via `WalkRelation`). `recall --provenance-depth N` is an explicit alias (wins if both are given). Depth is clamped to a documented max of **100** (defence-in-depth on the hop budget). Omitting the flag is **byte-identical** to the pre-provenance output. [agentic-lbjg]
- **`provenance_status` computed field** on `recall`/`get`/`list` JSON output: `complete` (has ≥1 outgoing `derived_from` edge), `none` (no edge, created at/after the convention boundary), or `legacy` (no edge, created before the boundary). Computed at query time — **no stored column**, no N+1 (a batched `ProvenanceStatusBatch`). The legacy boundary is a brain-level `schema_meta provenance_convention_since` instant (the migration instant; a fresh brain stamps its birth instant so it has no legacy era); the comparison is `node.CreatedAt.Before(boundary)` — a `time.Time` compare in Go, strict `<`, never a lexicographic string compare. [agentic-lbjg]
- No new `go.mod` dependency (stdlib + the existing `mattn/go-sqlite3` driver only). Export/import (`ExportBundle`) and the `--format sql` dump round-trip `provenance_root` (the three full-column node emitters in `export.go` widen in lockstep; round-trip regression guards assert `provenance_root=1` survives). [agentic-lbjg]

### Added [agentic-xtzn]

- **Bi-temporal validity windows on edges.** Two nullable `DATETIME` columns — `valid_at` and `invalid_at` — now carry the **valid-time** axis on every edge (the half-open interval `[valid_at, invalid_at)` during which the asserted relationship holds in the world), orthogonal to the existing `created_at` transaction-time column. The agent writes both bounds explicitly; cerebro **never** infers them (no auto-invalidation, no LLM in the loop). NULL = open-ended: `NULL valid_at` means "valid from the beginning of time", `NULL invalid_at` means "still valid". Existing edges (both bounds NULL) remain queryable and match every query — no result-changing backfill. See `docs/adrs/ADR-015-bitemporal-edge-validity.md`. [agentic-xtzn]
- **`schemaVersion` `3`→`4` migration.** A transaction-guarded v3→v4 step adds the two columns to existing brains via `ALTER TABLE edges ADD COLUMN` (each guarded on the column's actual absence, so a partially-migrated brain self-heals rather than crashing with "duplicate column"); idempotent on re-open; the unconditional `initFTSTable()` keeps firing. A freshly-`Init`ed brain is v4 with the columns. [agentic-xtzn]
- **`cerebro edge <src> <tgt> <rel> --valid-at / --invalid-at`** — attach the window to the **existing** `edge` command (no `add` subcommand). Both flags accept RFC3339 (`2026-06-17T14:30:00Z`) or a date (`2026-06-17`, midnight UTC) and are normalized to UTC. An inverted window (`valid_at` strictly after `invalid_at`) is rejected with a clear error before any write; equal bounds (zero-width `[t,t)`) are allowed and valid at no instant. [agentic-xtzn]
- **`recall` / `search` / `get --as-of <time>`** — filter edges to those valid at the supplied instant. The validity predicate `(valid_at IS NULL OR valid_at <= ?) AND (invalid_at IS NULL OR invalid_at > ?)` is injected at the edge-fetch SQL only when `--as-of` is set; absent, the predicate is omitted entirely and every query is byte-identical to before. Under the half-open convention, an as-of `== valid_at` is **included** and `== invalid_at` is **excluded**. NOTE: when the lazy-expansion gate (agentic-73l6) skips expansion for a query, no edges are traversed, so `--as-of` is a no-op on that query. [agentic-xtzn]
- **Edge upsert (window re-assertion).** Re-running `cerebro edge` for an existing `(source, target, relation)` now UPDATES the validity window in place via `ON CONFLICT … DO UPDATE SET valid_at = excluded.valid_at, invalid_at = excluded.invalid_at` — no duplicate row, the existing `id` is retained (resolved via `RETURNING id`, since `LastInsertId()` is unreliable on the conflict path). A re-add is a **full-window re-assertion, not a partial patch**: omitting a flag on re-add **clears that bound to NULL** (re-opens the window). This is correct-by-invariant under the no-inference rule — COALESCE-preserve would make cerebro infer "keep the old value". [agentic-xtzn]
- No new `go.mod` dependency (stdlib `time` + the existing `mattn/go-sqlite3` driver only). Export/import (`ExportBundle`) round-trips the two columns.

### Changed [agentic-xtzn]

- **BREAKING (Go API):** `Brain.Search` and `Brain.SearchWithGlobal` gain a trailing positional `asOf *time.Time` parameter (appended after `subtypeFilter`, the OO-011 idiom); `Brain.Get` and `Store.GetNodeWithEdges` gain a trailing `asOf *time.Time`; `Store.ExpandGraph`/`GetEdgesBatch`/`getEdgesForNode` gain `asOf *time.Time`; `Store.AddEdge`/`Brain.AddEdge` take a new `AddEdgeOpts` value-struct (`{ValidAt, InvalidAt *time.Time}`). Pass `nil` / a zero `AddEdgeOpts{}` for pre-xtzn behaviour. The `Search`/`SearchWithGlobal` break affects the external `qraftworx-cli` consumer (which still calls the 4-arg form and has not yet absorbed the OO-011 `subtypeFilter` break) — a downstream absorb ticket should bundle `subtypeFilter` + `asOf`. This rides a new cerebro major; qraftworx-cli is a consumer of the stack frame and absorbs the break (it is NOT touched in this ticket). [agentic-xtzn]

### Added [agentic-73l6]

- **Lazy / threshold-bounded expansion gating.** `Brain.Search`/`SearchWithGlobal` now skip single-hop graph expansion (`store.ExpandGraph` — two SQL round-trips + neighbour scoring) when the vector top-K is already confident: top-1 raw cosine similarity strictly above `expand_threshold`, OR the full top-K similarity spread strictly below `expand_spread_threshold`. The signal is the raw `ScoredNode.Similarity` (bounded `[0,1]`, time-stable), never the composite score (unbounded, recency-varying). The skip path replicates ExpandGraph's exact sort+cap tail (`cutByScore` — "ExpandGraph over an edgeless graph"), so downstream fusion/rerank contracts are unchanged; the BM25 keyword lane (agentic-2lak) and the optional reranker (agentic-2ixw) run on every query, gated or not. All four brain-layer expansion call sites are gated; every degenerate state (empty set, singleton spread, partial result set, `0.0` sentinels, NaN/out-of-range thresholds) disables the gate rather than firing it. See `docs/adrs/ADR-014-lazy-expansion-gating.md`. [agentic-73l6]
- **Config keys `expand_threshold` (default `0.75`, ACTIVE) and `expand_spread_threshold` (default `0.0`, OFF)** — both validated to `[0,1]`, `0.0` = condition disabled. Defaults are justified by a live 18-query eval sweep (`docs/evals/lazy-gating-results.md`): 0.75 is the lowest candidate with all four metrics (recall@5/10/20, MRR) bit-identical to the same-session gate-disabled floor and a non-zero skip-rate (4/18 = 22%, inside the research-cited 8–26% reduction envelope); 0.72 regressed MRR and was rejected. The spread condition ships OFF on measured evidence: top-1→top-K spread anti-correlates with confidence on the reference brain. [agentic-73l6]
- **Skip metric `stats.expansion_skips`** — a persistent, never-resetting `schema_meta` counter incremented per gate fire via the new atomic `store.IncrMeta` UPSERT (missing key starts at 1; non-integer value resets via documented CAST semantics). The write is best-effort by contract: a metrics failure can never fail or slow a recall. `SearchWithGlobal` records both stores' skip events on the **project** brain's counter. Read deltas with `sqlite3 <brain> "SELECT value FROM schema_meta WHERE key='stats.expansion_skips'"`. [agentic-73l6]

### Fixed [agentic-73l6]

- **`validateUnitFloat` now rejects NaN (S-1).** `strconv.ParseFloat` parses `"NaN"`/`"nan"` and IEEE-754 NaN compares false to everything, so the `f < 0 || f > 1` range check silently accepted NaN for every unit-float config key (`gc_threshold`, `search_threshold`, `recall_threshold`, and the two new gate keys). All pre-existing consumers were verified fail-safe under NaN, so the fix is strictly hardening. [agentic-73l6]

### Added [agentic-2lak]

- **BM25 keyword recall via SQLite FTS5.** A new `nodes_fts` FTS5 virtual table (`node_id UNINDEXED, content, subtype`) mirrors every active node's searchable text, kept in sync by application-level writes in the store CRUD paths (`AddNode`/`UpdateNode`/`SupersedeNode` and the GC eviction delete loop) — not SQLite triggers (a trigger would crash every `INSERT` on a binary lacking the FTS5 build tag). New `Store.KeywordSearch` runs an injection-safe FTS5 `MATCH` + `bm25()` ranking, and `brain.fuseRecallRRF` fuses the keyword lane with the vector/composite lane by Reciprocal Rank Fusion (`k=60`) *before* the optional 2ixw reranker — so the exact-identifier query (`HS-049`, a precise symbol name) the vector lane misses is surfaced. The four-signal `compositeScore` weights are unchanged (BM25 enters via recall-layer fusion, not a fifth weight); the 2ixw reranker and its three config keys are untouched. Closes the exact-identifier-match gap. [agentic-2lak]
- **`fts5` build tag on every build path (REQUIRED).** `mattn/go-sqlite3 v1.14.34` gates FTS5 behind the `//go:build sqlite_fts5 || fts5` CGO build tag; without it, `CREATE VIRTUAL TABLE … USING fts5` returns `no such module: fts5` and keyword recall silently no-ops. The tag is now set in the `Makefile` (`build`/`test`/`test-cover`), `.github/workflows/ci.yml` (test step), and `.goreleaser.yml` (`builds[].tags: [fts5]`) so every binary — local, CI, and the goreleaser-produced Homebrew darwin binary — links FTS5. `release.yml` needs no edit (it reads `.goreleaser.yml`). **This is a CGO compile flag, not a new `go.mod` dependency — `go.mod`/`go.sum` are unchanged.** Build locally with `go build -tags fts5 ./cmd/cerebro`. [agentic-2lak]
- **`bm25_enabled` config key** (bool, default `true`): BM25 keyword recall is always-on when the binary has the `fts5` tag. `bm25_enabled=false` short-circuits both the keyword query and the fusion (the literal pre-BM25 path) — it is an **eval/diagnostic seam** to produce the same-session BM25-disabled non-regression floor, **not an end-user feature toggle**. Appears in `cerebro config get`/`config list`; validates as a bool. [agentic-2lak]
- Schema migration: `schemaVersion` `2`→`3` creates and one-time-backfills `nodes_fts` from existing active nodes on first open with an fts5-tagged binary; the create is a separate guarded call (never inside the `applySchema` statement loop) and logs-and-continues on a no-fts5 binary, so store open never crashes and primary `nodes` writes are never coupled to the FTS5 tag. [agentic-2lak]
- Security: the FTS5 `MATCH` builder (`buildMatchQuery`) neutralises FTS5 metacharacters by tokenising the query on whitespace and wrapping **each** token as its own literal phrase (double-quote-wrap, double internal quotes) joined with our own ` OR `, so no FTS5 operator from user text (`AND`/`OR`/`NOT`/`NEAR`/`*`/`"`/`:`/`^`) reaches the parser — `?`-binding alone does NOT close this second-order injection. An adversarial unit test covers both the single-term and multi-word payload sets. `docs/adrs/ADR-013-bm25-fts5-keyword-recall.md` records the build-tag + RRF-recall-fusion + tokenize-OR (D7) decisions; `docs/evals/bm25-results.md` records the same-session before/after eval (both rerank paths, full corpus + exact-id subset) and the tokenizer choice. [agentic-2lak]

### Fixed [agentic-2lak]

- **FTS5 MATCH tokenization: BM25 now matches inside multi-word queries.** `buildMatchQuery` previously wrapped the **whole** query in a single FTS5 phrase (`"OO-015 determinism wire"`), which requires all terms to be adjacent in the indexed text — so every multi-word query matched **0 rows** (live-proven on the 556-row dogfooded index) and the keyword lane was empty on every eval query, making BM25 inert (the prior eval's "no win"). It now tokenises on whitespace, quotes each term (doubling internal `"`), and joins with ` OR ` (`"OO-015" OR "determinism" OR "wire"`), so the rare identifier token matches inside a multi-word query and `bm25()` floats it up. Injection-safety is preserved exactly (each user term is its own quoted phrase; the `OR` is ours). Re-measured on an exact-identifier-extended corpus (18 queries): rerank=off exact-id subset gains recall@20 0.75 → 1.00 and MRR 0.26 → 0.57; full corpus lifts on every metric; AC4-NR non-regression holds on both rerank paths (N=3 deterministic). See `docs/evals/bm25-results.md`. [agentic-2lak]

### Added [agentic-2ixw]

- Optional local cross-encoder reranking of recall candidates, **default OFF**. New `internal/rerank/` package: a `Reranker` interface (mirroring `embed.Provider`) with a `noop` identity implementation (disabled-path no-op) and a `command` implementation that invokes a local operator-supplied subprocess over a JSON-on-stdin / JSON-on-stdout contract (`{"query","documents"}` → `{"scores"}`). cerebro bundles **no model**; the operator supplies the reranker command. When enabled, `Brain.Search`/`SearchWithGlobal` over-retrieve `max(limit*2, 40)` candidates, rerank, and cut to `limit`; the `compositeScore` weights are untouched (the reranker reorders only). On any reranker failure (command unset/missing/crash, malformed/short JSON, non-finite scores, timeout) it degrades to the pre-rerank composite order with a one-line stderr warning, so recall is never worse than disabled. [agentic-2ixw]
- Config keys: **`rerank_enabled`** (bool, default `false`) gates reranking; **`rerank_command`** (free-form string, default empty) names the reranker subprocess, with the `CEREBRO_RERANK_COMMAND` env var as a fallback (config-wins, env-fallback). Enable with `cerebro config set rerank_enabled true` and either `cerebro config set rerank_command "<cmd>"` or `export CEREBRO_RERANK_COMMAND="<cmd>"`. Both keys appear in `cerebro config get`/`config list`. The enable gate is env-free by design (an env-only actor cannot enable subprocess spawning). [agentic-2ixw]
- Security hardening on the subprocess surface: argv-array exec only (never a shell), an explicit `exec.CommandContext` timeout that kills a hung child, a bounded stdout read, and rejection of non-finite (NaN/±Inf) scores. [agentic-2ixw]
- `docs/adrs/ADR-012-cross-encoder-reranking.md` records the subprocess-vs-ONNX mechanism decision (onnxruntime_go needs a runtime `dlopen`'d shared lib + cgo; cerebro's cask ships a bare single binary — so the subprocess preserves the single-binary, model-free, Model-B distribution with a 0-byte binary delta). `docs/evals/reranker/` ships a reference MiniLM cross-encoder subprocess (operator-local venv, gitignored) and `docs/evals/rerank-results.md` records the before/after eval. [agentic-2ixw]
- **`rerank_fusion`** config key (default `"rrf"`): selects how the reranker ranking is combined with the composite ranking when reranking is enabled. `"rrf"` (Reciprocal Rank Fusion, the new default) fuses both rankings; `"reorder"` is the legacy pure-reorder (sort by reranker score, discard composite order). Appears in `cerebro config get`/`config list`; validator accepts exactly `rrf`/`reorder`. [agentic-2ixw]

### Changed [agentic-2ixw]

- **The default reranking combine is now Reciprocal Rank Fusion (RRF), not pure-reorder.** Pure-reorder sorted the candidate set by the cross-encoder score alone, *discarding* the composite order — which let the reranker demote a composite-strong (recall@10-relevant) item below the cut while lifting the single best item (MRR up, recall@10 down: the documented `-0.0167` dip). RRF fuses the composite rank and the reranker rank — `fused = 1/(k+rank_composite) + 1/(k+rank_reranker)`, `k=60` (Cormack, Clarke & Buettcher, SIGIR 2009; the Elasticsearch `rrf` retriever default `rank_constant`) — so a composite-top item the reranker buries keeps its strong `1/(k+1)` composite term and survives the cut. New pure helper `brain.fuseRRF`, unit-tested with synthetic rankings proving the displaced-node recovery. **Measured on the same 550-node dogfooded brain (deterministic N=3):** RRF recovers recall@10 to **0.8333** (above the disabled `0.7833` baseline; pure-reorder was `0.7667`) while still improving MRR (`0.5033 → 0.6825`, +0.18). The single-node displacement under pure-reorder was confirmed and RRF restores it. Legacy pure-reorder remains available via `cerebro config set rerank_fusion reorder`. Reranking overall is still **default OFF** (`rerank_enabled=false`). [agentic-2ixw]

### Fixed [agentic-7r28]

- `cerebro eval` no longer silently writes an all-zero baseline when the target brain contains none of the ground-truth nodes (the "wrong `-p`" case). It now aborts with a clear non-zero error (`eval aborted: none of the ground-truth node IDs resolve as active nodes in the target brain …`) before writing anything. A second defence-in-depth guard refuses any zero-query baseline. New pure helper `countResolvableGroundTruth` covers the condition (unit + preflight-integration tests). [agentic-7r28]
- `cerebro eval --out` now defaults to a gitignored scratch path (`docs/evals/baseline.local.json`) instead of the committed reference (`docs/evals/baseline.json`), so a bare `cerebro eval` can never clobber the committed baseline; updating the committed reference is now a deliberate `--out docs/evals/baseline.json`. [agentic-7r28]

## [2.1.0] - 2026-06-05

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

