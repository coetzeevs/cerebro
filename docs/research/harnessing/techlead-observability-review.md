# Tech Lead Review: Observability System Feasibility Analysis

**Date:** 2026-04-23
**Reviewer:** Technical Lead (Code Review Gate)
**Scope:** Implementation feasibility of per-turn observability with Bubble Tea dashboard
**Method:** Empirical benchmarks against actual project data, dependency analysis, codebase audit

---

## 1. Per-Turn Data Collection: Cost Analysis

### 1.1 Empirical usage profile (measured from this project's JSONL data)

| Metric | Value | Source |
|--------|-------|--------|
| Active days measured | 7 | JSONL files in `~/.claude/projects/-Users-q-projects-agentic-cerebro/` |
| Total turns | 1,077 | Parsed from 12 JSONL files |
| Total tool calls | 969 | Extracted from assistant message content blocks |
| Average turns/day (active) | 154 | 1077 / 7 |
| Average tool calls/day (active) | 138 | 969 / 7 |
| Average tool calls/turn | 10.5 | 283 / 27 (single session sample) |
| Peak tool calls in one turn | 105 | Single heavy implementation turn |
| Sessions with `system:turn_duration` markers | 16 per 941-line file | Claude Code emits these natively |

### 1.2 Storage overhead estimate

**Schema: one row per turn (aggregated), not one row per tool call.**

If we store per-turn summaries (tool call counts by type, total duration, read:edit ratio, timestamp), a row is approximately 200-300 bytes.

| Scenario | Turns/day | Rows/day | Bytes/day | Rows/year | Size/year |
|----------|-----------|----------|-----------|-----------|-----------|
| Light day | 30 | 30 | ~9 KB | 10,950 | ~3.2 MB |
| Heavy day (measured) | 154 | 154 | ~46 KB | 56,210 | ~16.4 MB |
| Extreme day (all-day session) | 500 | 500 | ~150 KB | 182,500 | ~53 MB |

**If we store per-tool-call granularity instead (one row per tool call):**

| Scenario | Tool calls/day | Bytes/day | Rows/year | Size/year |
|----------|---------------|-----------|-----------|-----------|
| Light | 50 | ~10 KB | 18,250 | ~3.6 MB |
| Heavy (measured) | 138 | ~28 KB | 50,370 | ~10 MB |
| Extreme | 500 | ~100 KB | 182,500 | ~36 MB |

**Recommendation: Store per-tool-call rows.** The size is negligible (under 40 MB/year at extreme usage). Per-tool-call gives the most analytical flexibility -- you can always aggregate up to per-turn, but cannot disaggregate from per-turn summaries. SQLite handles millions of rows efficiently with proper indexing.

### 1.3 Retention strategy

Given the small storage footprint:

- **Default retention: 90 days.** Configurable via `cerebro config set metrics_retention 180`.
- **Cleanup mechanism:** Piggyback on the existing `cerebro gc` command (already runs from SessionEnd hook). Add a `DELETE FROM metrics WHERE ts < datetime('now', '-' || retention_days || ' days')` step.
- **No archival needed.** At <40 MB/year, there is no practical reason to archive. If a user wants to keep forever, `metrics_retention = 0` disables cleanup.

### 1.4 Query performance at scale

SQLite handles the projected volumes trivially:

- 200K rows (extreme full-year) with a `(session_id, turn_number)` or `(ts)` index: queries complete in <1ms for point lookups, <10ms for date-range aggregations.
- The current brain DB already uses WAL mode with `_busy_timeout=5000` and `_cache_size=-65536` (64MB). These settings are more than sufficient for concurrent metric writes.

---

## 2. JSONL Parsing Feasibility

### 2.1 Benchmark results (measured on this machine)

| File | Size | Lines | Parse time | Rate |
|------|------|-------|------------|------|
| 5714a20e (largest) | 3.1 MB | 941 | 29 ms | 107 MB/s |
| e39f605c | 2.2 MB | 504 | 20 ms | 110 MB/s |

**Go's `bufio.Scanner` + `encoding/json.Unmarshal` parses 3MB JSONL in under 30ms.** This is fast enough for real-time dashboard rendering (even at 60fps, parsing takes <2% of a frame budget). No streaming parser or pre-indexing is needed.

### 2.2 The JSONL schema (empirically verified)

Key message types for observability, with their fields:

| Type | Subtype | Relevant fields |
|------|---------|----------------|
| `user` | (none) | `promptId`, `uuid`, `timestamp`, `isMeta`, `sessionId` |
| `assistant` | (none) | `message.content[]` (contains `tool_use` blocks with `name`, `id`) |
| `system` | `turn_duration` | `durationMs`, `messageCount`, `timestamp` |
| `system` | `stop_hook_summary` | Hook block/allow decisions |
| `attachment` | (via `.attachment.type`) | `hook_success`, `hook_error` for hook outcomes |

**Turn boundaries are identifiable** via `promptId` on user messages (each turn has a unique `promptId`), reinforced by `system:turn_duration` markers that Claude Code emits at turn end.

### 2.3 Indexing strategy recommendation

**Primary key:** `(session_id, timestamp)` -- not turn_number. Rationale:
- JSONL does not contain an explicit turn counter. You would have to derive it by counting non-meta user prompts in order -- fragile.
- `promptId` is unique per turn and serves as a natural grouping key, but it is a UUID -- poor for range queries.
- Timestamps are monotonic within a session and support natural range queries ("last hour", "today").

**If collecting via hooks (real-time):** Index on `(session_id, ts)`.
**If parsing JSONL (batch):** Index on `(session_id, ts)` with a secondary index on `(date)` for daily aggregations.

### 2.4 Real-time hooks vs. batch JSONL parsing

Two data collection strategies are available. They are not mutually exclusive.

| Approach | Latency | Data completeness | Complexity |
|----------|---------|-------------------|------------|
| PostToolUse hooks (real-time) | <1ms per write | Tool name + timing only; no turn context until turn ends | Low -- one SQLite INSERT per hook fire |
| JSONL parsing (batch/on-demand) | 30ms for full file | Complete -- all message types, full content, turn structure | Medium -- parser + JSONL discovery |

**Recommendation: Both.**
- **PostToolUse hooks** for real-time metrics (tool counts, stop-guard blocks). These write to the metrics table as the session runs.
- **JSONL parsing** for retrospective analysis (read:edit ratios, turn duration distributions, session-level aggregations). This runs on-demand when the dashboard opens or `cerebro stats` is invoked.

This avoids the complexity of trying to reconstruct turn boundaries in real-time from hook events, while still getting immediate feedback during a session.

---

## 3. Bubble Tea Dependency Assessment

### 3.1 Dependency count impact

| Configuration | Module count | Delta |
|---------------|-------------|-------|
| Current cerebro (cobra + sqlite3 + sqlite-vec) | 6 direct + 9 indirect = **15 total** | baseline |
| + bubbletea v1.3.10 | **26 total** | +11 |
| + bubbletea + lipgloss | **26 total** | +0 (lipgloss is already a bubbletea transitive dep) |
| + bubbletea + lipgloss + bubbles (components) | **36 total** | +10 more |

Adding Bubble Tea + Lipgloss + Bubbles takes the dependency count from 15 to 36 -- a 2.4x increase in module count.

### 3.2 Binary size impact

| Build | Size | Delta |
|-------|------|-------|
| Current cerebro binary | 13 MB | baseline |
| Minimal bubbletea binary (no sqlite) | 3.3 MB | -- |
| **Estimated combined** | **~15-16 MB** | **+2-3 MB** |

The binary size increase is modest because Bubble Tea is pure Go (no CGO, no C dependencies). It adds terminal handling code but no additional native libraries. The existing 13 MB binary is dominated by CGO sqlite3 + sqlite-vec.

### 3.3 CGO implications

**None.** Bubble Tea and all charmbracelet packages are pure Go. They do not require CGO and do not conflict with the existing mattn/go-sqlite3 CGO dependency. Build times should increase by 2-5 seconds at most (pure Go packages compile fast).

### 3.4 Bubbles component library gaps

The `charmbracelet/bubbles` package provides: `table`, `progress`, `spinner`, `viewport`, `textinput`, `textarea`, `list`, `paginator`, `help`, `key`, `timer`, `stopwatch`, `cursor`, `filepicker`.

**Notably absent: sparkline.** There is no built-in sparkline component. Options:
1. **Use `progress` bars as a proxy** -- visually different but available out of the box.
2. **Implement a minimal sparkline renderer** using block characters (Unicode block elements: `\u2581` through `\u2588`). This is approximately 30-50 lines of Go -- trivial.
3. **Use a third-party sparkline package** -- several exist (`joliv/spark`, etc.) but add yet another dependency.

**Recommendation: Option 2.** A custom sparkline renderer using Unicode block characters is small, has zero dependencies, and fits the "simplest approach" principle. The `lipgloss` styling can color the blocks.

### 3.5 Build-behind-feature-flag option

If the dependency bloat is concerning, the dashboard can be placed behind a Go build tag:

```go
//go:build dashboard
```

This means `go build` excludes Bubble Tea by default; `go build -tags dashboard` includes it. However, this adds build complexity and makes the feature harder to discover. Given the modest impact (+2-3 MB binary, +21 deps), a feature flag is probably not worth the ergonomic cost.

---

## 4. Real-Time Collection via Hooks: Latency Analysis

### 4.1 SQLite write latency (benchmarked on this machine)

| Operation | Latency |
|-----------|---------|
| Single INSERT (WAL mode) | **31 microseconds** (0.031 ms) |
| 100 INSERTs in one transaction | **411 microseconds total** (4.1 us/row) |

**A PostToolUse hook that writes one row adds 0.03ms of latency.** This is imperceptible. For context, a typical tool call (Read, Bash, Grep) takes 50-500ms. The 0.03ms write is 0.006% of the fastest tool call.

### 4.2 WAL mode concurrency

The current `store.go` opens SQLite with `_journal_mode=WAL`. WAL mode supports:
- **One writer + multiple concurrent readers.** The hook process (writer) and the dashboard process (reader) can operate simultaneously without blocking.
- **`_busy_timeout=5000`** (5 seconds) is already configured, providing a generous retry window if contention occurs.

**Potential issue: Different processes accessing the same DB file.** The hook runs as a subprocess (`cerebro` invoked by Claude Code). The dashboard runs as a separate `cerebro dashboard` process. Both access the same SQLite file. WAL mode handles this correctly as long as both are on the same filesystem (no network mounts). This is always the case for local-first Cerebro.

### 4.3 Hook fire frequency

From the measured data:
- Average: 138 tool calls/day = 138 hook fires/day = 138 writes/day.
- Peak: 105 tool calls in a single turn (observed). Even at this burst rate, 105 writes at 31us each = 3.3ms total. Negligible.
- Worst case: If hooks fire synchronously and block the tool pipeline, 0.03ms per tool call adds 3.15ms to a 105-call turn. Unmeasurable.

### 4.4 Separate metrics DB vs. brain DB

**Question: Should metrics go in the brain DB or a separate SQLite file?**

| Option | Pros | Cons |
|--------|------|------|
| Brain DB (`brain.sqlite`) | Single file. Existing migration system. Backup covers metrics. | Mixes operational data with analytical data. Brain DB is small (~100KB); metrics add bulk. Metrics retention differs from brain retention. |
| Separate DB (`metrics.sqlite`) | Clean separation. Independent retention. Independent backup. Can be deleted without affecting brain. | Two DB files to manage. Hook needs to know the path. Dashboard needs to open two files. |

**Recommendation: Separate `metrics.sqlite` file in the same directory as `brain.sqlite`.** Rationale:
- Metrics are disposable; brain data is not. They should not share a retention lifecycle.
- `cerebro gc` already knows the project directory. Adding `metrics.sqlite` cleanup is trivial.
- The brain DB schema migration system is for brain schema. Metrics schema changes should not require brain migration version bumps.
- If a user deletes metrics, their brain is unaffected. If they export/import a brain, metrics are correctly excluded.

---

## 5. Testing Strategy for a TUI

### 5.1 Bubble Tea's testing model

The Elm Architecture (Model-View-Update) that Bubble Tea uses is inherently testable:

1. **Model tests:** Create a model, call `Update(msg)`, assert the resulting model state. No terminal needed.
2. **View tests:** Call `View()` on a model, assert the returned string contains expected content. Pure string comparison.
3. **Integration tests:** Bubble Tea v2 provides `tea.WithColorProfile()` and `tea.WithWindowSize(w, h)` for programmatic testing.
4. **Golden file tests:** `charmbracelet/x/exp/golden` (already a transitive dependency) supports snapshot testing of rendered output.

### 5.2 Concrete testing strategy for the dashboard

| Layer | What to test | How | Terminal needed? |
|-------|-------------|-----|-----------------|
| Metrics store | CRUD operations on metrics table | Standard Go tests with `t.TempDir()` SQLite DBs | No |
| JSONL parser | Parsing, turn boundary detection, tool call extraction | Table-driven tests with fixture JSONL files in `testdata/` | No |
| Dashboard model | State transitions (tab switching, filter changes, sort orders) | Send tea.Msg values to `Update()`, assert model state | No |
| Dashboard view | Rendered output for known data | Call `View()` with fixture data, assert substring content or golden files | No |
| Sparkline renderer | Unicode block output for known data series | Pure function: `[]float64 -> string`, table-driven tests | No |
| CLI integration | `cerebro dashboard` starts and exits cleanly | `exec.Command` with stdin/stdout piping, short timeout | Minimal (but can use `tea.WithInput/WithOutput`) |

**Every layer except CLI integration is fully testable without a real terminal.** This follows the existing project pattern where `cmd_stop_guard_test.go` tests the `evalStopGuard` function by passing `io.Reader`/`io.Writer` directly.

### 5.3 Test file and coverage estimate

| Component | Test file | Estimated test count |
|-----------|-----------|---------------------|
| Metrics store | `internal/store/metrics_test.go` | 8-10 (CRUD, retention, aggregation queries) |
| JSONL parser | `internal/observe/parser_test.go` | 10-12 (each message type, edge cases, malformed input) |
| Dashboard model | `cmd/cerebro/cmd_dashboard_test.go` | 8-10 (state transitions, keybindings, filtering) |
| Sparkline | `cmd/cerebro/sparkline_test.go` or in dashboard test | 4-5 (empty data, single point, normal, edge values) |
| Hook collector | `cmd/cerebro/cmd_metric_collect_test.go` | 4-5 (stdin parsing, DB write, missing fields) |

**Total: ~35-42 new tests.** For context, the project currently has 189 tests across ~10,593 lines of Go.

---

## 6. Concrete File/Line Estimates

### 6.1 New files

| File | Purpose | Estimated lines |
|------|---------|----------------|
| `internal/store/metrics_schema.go` | Metrics table DDL, migration | 60-80 |
| `internal/store/metrics.go` | Metrics CRUD (insert, query, aggregate, prune) | 150-200 |
| `internal/store/metrics_test.go` | Tests for metrics store | 200-250 |
| `internal/observe/parser.go` | JSONL parser, turn boundary detection, tool extraction | 200-250 |
| `internal/observe/parser_test.go` | Parser tests with fixtures | 250-300 |
| `internal/observe/testdata/*.jsonl` | Test fixture JSONL files (3-5 files) | ~100 total |
| `cmd/cerebro/cmd_dashboard.go` | Bubble Tea model, update, view, keybindings | 400-500 |
| `cmd/cerebro/cmd_dashboard_test.go` | Dashboard model/view tests | 200-250 |
| `cmd/cerebro/sparkline.go` | Unicode sparkline renderer | 40-60 |
| `cmd/cerebro/sparkline_test.go` | Sparkline tests | 40-60 |
| `cmd/cerebro/cmd_metric_collect.go` | PostToolUse hook handler (stdin -> metrics DB) | 60-80 |
| `cmd/cerebro/cmd_metric_collect_test.go` | Hook handler tests | 60-80 |

### 6.2 Modified files

| File | Change | Lines added |
|------|--------|-------------|
| `go.mod` / `go.sum` | Add bubbletea, lipgloss, bubbles | ~25 |
| `cmd/cerebro/cmd_stats.go` | Extend with metrics summary (or reference dashboard) | 10-20 |
| `cmd/cerebro/helpers.go` | Add `openMetricsStore()` helper | 10-15 |
| `cmd/cerebro/config.go` | Add `metrics_retention` config key | 10-15 |
| `templates/settings.json` | Add PostToolUse hook for metric collection | 10-15 |
| `scaffold.go` | Wire PostToolUse hook into scaffolding | 5-10 |
| `CHANGELOG.md` | Document the feature | 10-20 |

### 6.3 Summary

| Category | Files | Lines |
|----------|-------|-------|
| New production files | 7 | 910-1,170 |
| New test files | 5 + fixtures | 750-940 |
| Modified files | 7 | 80-130 |
| **Total** | **~19** | **~1,740-2,240** |

This is a 16-21% increase to the codebase (from 10,593 lines). Significant but manageable. The production-to-test ratio is approximately 1:0.8, which is appropriate for a feature with this much state management.

### 6.4 Maintenance burden

- **Bubble Tea updates:** The charmbracelet ecosystem is actively maintained and has a v2 in development. The v1 API is stable. Breaking changes would be in v2 (different import path), so the v1 dependency is low-maintenance.
- **JSONL schema changes:** Claude Code's JSONL format is undocumented and could change. The parser should be defensive (ignore unknown fields, handle missing fields gracefully). This is the highest maintenance risk.
- **Metrics schema:** Once defined, the metrics schema is unlikely to change frequently. The existing migration pattern (`migrateSchema()` in `schema.go`) extends naturally.

---

## 7. What Can Be Reused from the Existing Codebase

### 7.1 Store patterns (`internal/store/store.go`)

**Directly reusable:**
- `open()` function pattern (DSN construction with WAL, busy_timeout, foreign_keys, cache_size) -- line 72
- `Store` struct wrapping `*sql.DB` with `Path()`, `Close()`, `DB()` -- can create an analogous `MetricsStore` or reuse `Store` with a separate `Init` path
- `applySchema()` pattern (execute DDL statements in order) -- `schema.go` lines 8-93
- `migrateSchema()` pattern (version-guarded migrations in transactions) -- `schema.go` lines 141-182
- `SetMeta()` / `GetMeta()` for metrics config storage

**Recommendation:** Create a `MetricsStore` type in `internal/store/` that follows the same `Open`/`Init`/`Close` pattern as `Store`. Alternatively, a standalone `metrics.Open(path)` function that returns a `*sql.DB` would be simpler if the metrics schema is flat (no migrations anticipated in v1).

### 7.2 Helper patterns (`cmd/cerebro/helpers.go`)

**Directly reusable:**
- `resolveProjectDir()` -- metrics DB should live alongside the brain DB, so the same resolution logic applies
- `outputJSON()` -- for `cerebro stats --format json` extensions
- `outputStats()` -- extend to include metrics summary

**New helper needed:**
- `openMetricsDB()` or `openMetrics()` -- resolves project dir, constructs metrics.sqlite path, opens with WAL mode.

### 7.3 Config patterns (`cmd/cerebro/config.go`)

**Directly reusable:**
- `configRegistry` pattern for new keys (`metrics_retention`, `dashboard_refresh_rate`)
- `validatePositiveInt` validator for retention days
- `resolveConfigInt()` for reading config at runtime
- `applyConfigFlag()` for CLI flag override of config values

### 7.4 Hook patterns (`cmd_stop_guard.go`)

**Directly reusable:**
- Stdin-reading pattern (`io.ReadAll` + `json.Unmarshal`) for the PostToolUse metric collector hook
- JSON output pattern for hook decisions
- Test pattern: `evalStopGuard(r io.Reader, w io.Writer)` -- the metric collector should similarly accept `io.Reader` for testability

### 7.5 Scaffold patterns (`scaffold.go`)

**Directly reusable:**
- `//go:embed` pattern for PostToolUse hook template
- `addMissingEvents()` for adding the PostToolUse hook to existing settings
- `replaceCerebro()` for `--force` upgrades

---

## 8. Architecture Recommendation

### 8.1 Per-turn granularity via two complementary paths

```
                       REAL-TIME PATH                    BATCH PATH
                    (during session)                  (on-demand)
                          |                               |
                PostToolUse hook fires                cerebro dashboard/stats
                          |                               |
                cerebro metric-collect                Parse JSONL files
                (reads stdin, writes DB)              (from ~/.claude/projects/)
                          |                               |
                    metrics.sqlite                   In-memory aggregation
                    (per-tool-call rows)              (turn summaries, ratios)
                          |                               |
                          +---------- MERGE ------------->+
                                                          |
                                                    Dashboard renders
                                                    (Bubble Tea TUI)
```

### 8.2 Metrics table schema (proposed)

```sql
CREATE TABLE IF NOT EXISTS tool_calls (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    tool_name TEXT NOT NULL,
    file_path TEXT,           -- for Read/Edit/Write: which file
    duration_ms INTEGER,      -- if measurable from hook context
    ts TEXT NOT NULL,         -- ISO8601 timestamp
    hook_event TEXT NOT NULL  -- 'PostToolUse', 'Stop', etc.
);

CREATE INDEX idx_tool_calls_session ON tool_calls(session_id);
CREATE INDEX idx_tool_calls_ts ON tool_calls(ts);
CREATE INDEX idx_tool_calls_tool ON tool_calls(tool_name);

CREATE TABLE IF NOT EXISTS turn_summaries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    prompt_id TEXT,
    turn_number INTEGER,
    tool_call_count INTEGER,
    read_count INTEGER,
    edit_count INTEGER,
    write_count INTEGER,
    duration_ms INTEGER,
    ts_start TEXT NOT NULL,
    ts_end TEXT,
    source TEXT NOT NULL CHECK (source IN ('hook', 'jsonl'))
);

CREATE INDEX idx_turn_summaries_session ON turn_summaries(session_id);
CREATE INDEX idx_turn_summaries_ts ON turn_summaries(ts_start);
```

**Design notes:**
- `tool_calls` is populated by the PostToolUse hook in real-time. Lightweight, append-only.
- `turn_summaries` is populated either by aggregating `tool_calls` at turn boundaries, or by batch JSONL parsing. The `source` column tracks provenance.
- No foreign keys between the two tables -- they are independent data paths that converge in the dashboard.

### 8.3 PostToolUse hook input (needs verification)

The PostToolUse hook receives JSON on stdin. Based on the Claude Code hook documentation, expected fields include:
- `tool_name` -- which tool was called
- `tool_input` -- the input parameters (contains file paths for Read/Edit/Write)
- `tool_output` -- the result (may be large; do not store)

**Critical: This needs empirical verification before implementation.** The exact PostToolUse JSON schema should be confirmed by deploying a logging hook (`cat | tee -a /tmp/posttooluse.jsonl`) and inspecting the output. Do not assume field names based on documentation alone -- the consolidated proposal already documented field name corrections (Section 2.4: `stop_message` was wrong, actual field is `last_assistant_message`).

### 8.4 Dashboard views (proposed)

| View | Content | Data source |
|------|---------|-------------|
| **Turn timeline** | Chronological list of turns with tool call counts, duration, read:edit ratio | turn_summaries |
| **Tool distribution** | Bar chart / table of tool usage by name | tool_calls aggregate |
| **Read:Edit ratio** | Sparkline over time, current ratio, trend | tool_calls aggregate |
| **Stop guard** | Block count, block reasons, block rate | tool_calls where hook_event='Stop' |
| **Session summary** | Total turns, duration, most-used tools | Both tables |

### 8.5 CLI commands (proposed)

| Command | Purpose | Complexity |
|---------|---------|------------|
| `cerebro metric-collect` | PostToolUse hook handler: stdin -> metrics.sqlite | Low (60-80 lines) |
| `cerebro dashboard` | Full Bubble Tea interactive TUI | High (400-500 lines) |
| `cerebro stats` (extended) | Add metrics summary to existing stats output | Low (10-20 lines added) |

---

## 9. Risk Assessment

| Risk | Severity | Likelihood | Mitigation |
|------|----------|------------|------------|
| JSONL schema changes break parser | Major | Medium | Defensive parsing (ignore unknown fields, handle missing). Version-sniff from `version` field in JSONL. |
| PostToolUse hook input schema undocumented | Major | High | Deploy logging hook first. Verify before coding. |
| Bubble Tea v2 migration | Minor | Low (years) | Pin to v1. v2 uses different import path; can migrate incrementally. |
| Metrics DB grows unbounded | Minor | Low | Default 90-day retention + GC integration. |
| Dashboard complexity exceeds estimate | Major | Medium | Start with a single-view MVP (turn timeline + read:edit sparkline). Add views incrementally. |
| Two-DB management confuses users | Minor | Low | Same directory, documented in `cerebro stats` output, cleaned by `cerebro gc`. |

---

## 10. Implementation Phasing Recommendation

### Phase A: Instrumentation (can start immediately)

1. Deploy a PostToolUse logging hook (`cat | tee -a /tmp/cerebro-posttooluse.jsonl`) to capture the actual hook JSON schema. Run for 1-2 sessions.
2. Verify the exact field names and data available.

### Phase B: Metrics Store + Collector (1 session)

1. `internal/store/metrics_schema.go` + `metrics.go` + tests
2. `cmd/cerebro/cmd_metric_collect.go` + tests
3. Update `templates/settings.json` with PostToolUse hook
4. Update `scaffold.go` to wire the new hook

### Phase C: JSONL Parser (1 session)

1. `internal/observe/parser.go` + tests + fixtures
2. Turn boundary detection, tool call extraction, read:edit ratio calculation

### Phase D: Dashboard (1-2 sessions)

1. Sparkline renderer
2. Dashboard model + views
3. `cerebro dashboard` command
4. Tests

### Phase E: Integration + Polish (1 session)

1. Extend `cerebro stats` with metrics summary
2. Add `metrics_retention` config
3. Wire GC to prune metrics
4. CHANGELOG + documentation

**Estimated total: 4-6 sessions.** This is a significant feature. The Bubble Tea dashboard alone (Phase D) is the largest single component Cerebro has ever added.

---

## 11. Open Questions Requiring User Input

1. **Scope confirmation:** Is the dashboard for this project only, or should it aggregate across all projects? (The JSONL files are per-project in `~/.claude/projects/`, but the user may want a global view.)

2. **PostToolUse hook schema:** Has the user already deployed a logging hook to capture the actual JSON? If not, that should be step zero.

3. **Dashboard refresh:** Should the dashboard auto-refresh (poll the DB every N seconds) or require manual refresh (press `r`)? Auto-refresh during an active session would show live metrics but adds complexity (tea.Tick).

4. **Sparkline scope:** "Sparklines for quick views" -- does this mean in the `cerebro stats` CLI output (inline Unicode sparklines in the terminal), or only within the Bubble Tea dashboard?

5. **Priority relative to Phase 1 harness work:** The consolidated proposal's Phase 1 (stop-guard + /implement template) is already on `feat/harness-phase0`. Does observability block that, or can they proceed in parallel?
