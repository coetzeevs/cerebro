# ADR-015: Bi-temporal edge validity windows (valid_at, invalid_at)

**Status:** Accepted
**Date:** 2026-06-19
**Ticket:** agentic-xtzn

## Context

cerebro edges carried only a `created_at` transaction-time column (when the row
was written). Agents need to record *when an asserted relationship holds in the
world* — to answer "what was true on date X" and "show only currently-valid
edges" — and to capture when a relationship became relevant and whether it still
holds. This is the foundation for the episode/provenance primitive
(agentic-lbjg) and hybrid-ingest mode.

The change must stay within cerebro's invariants: **Model B** (no runtime LLM in
cerebro) and **no auto-invalidation** — the agent writes the bounds explicitly;
cerebro stores and filters on them but never infers them.

## Decision

### 1. A separate valid-time axis, not a reuse of `created_at`

Two new nullable `DATETIME` columns — `valid_at` and `invalid_at` — carry the
**valid-time** axis (when the relationship holds in the world), distinct from
the existing `created_at` **transaction-time** axis (when the row was written).
This is the "bi-temporal" split. Reusing `created_at` was rejected: conflating
the two destroys the ability to backdate a relationship (assert today that an
edge was valid last year) and breaks the moment an agent records historical
knowledge. Only the valid-time window is added here; full system-time
bitemporality (a transaction-time *history* table) is out of scope.

Both columns are nullable with NO default. **NULL = open-ended:** `NULL valid_at`
means "valid from −∞" (no lower bound); `NULL invalid_at` means "still valid"
(no upper bound). The today's-universal edge — both bounds NULL — matches every
query, so existing brains are non-regressing with no backfill.

### 2. Half-open `[valid_at, invalid_at)` boundary convention

An edge is valid at instant T when `valid_at <= T < invalid_at` (lower-closed,
upper-open). Consequences, all live-probed (SQLite 3.51) and unit-tested:

- An as-of `== valid_at` is **included** (the edge becomes valid *at* that
  instant).
- An as-of `== invalid_at` is **excluded** (the edge goes invalid *at* that
  instant — it is not valid at the instant it ends).
- Abutting windows `[a, b)` and `[b, c)` never both match at `b` — exactly one
  is valid at any instant.
- A zero-width window `[t, t)` is a well-defined degenerate window valid at **no**
  instant (allowed at write time; an *inverted* window `valid_at > invalid_at`
  is rejected at the CLI as a near-certain typo).

Closed-closed `[valid_at, invalid_at]` was rejected: it makes abutting windows
double-match at the shared boundary (a relationship simultaneously valid and
just-invalidated), which is ill-defined. Half-open is the standard
temporal-database convention.

### 3. The filter is a SQL predicate at the edge-fetch source

When an as-of instant is supplied, the predicate
`(valid_at IS NULL OR valid_at <= ?) AND (invalid_at IS NULL OR invalid_at > ?)`
is appended to the two edge-fetch queries (`getEdgesForNode`, `GetEdgesBatch`),
with the instant bound twice via `?` placeholders. When no as-of is supplied the
predicate is **omitted entirely**, so every existing query is byte-identical to
the pre-feature path (no result-changing behaviour without the flag). Filtering
at the SQL source (rather than Go-side post-filtering after a full fetch) keeps
a single source of truth and never materialises an unbounded edge set; it reuses
the shipped `created_at >= ?` + `.UTC().Format("2006-01-02 15:04:05")` mechanism.

Storage layout is TEXT `"2006-01-02 15:04:05"` (consistent with every other
cerebro temporal column), which sorts lexicographically == chronologically, so
the `<=` / `>` comparisons are correct. An INTEGER epoch column was rejected as
the lone inconsistent representation. Scanning uses `sql.NullTime` (the driver
auto-parses the declared `DATETIME` columns into `time.Time`), NOT the RFC3339
`time.Parse` idiom used elsewhere — that idiom silently discards a parse error
and would make a windowed edge look open-ended, defeating the filter.

### 4. CLI time-format convention (new)

cerebro had no user-facing time-string parser. This ticket defines `parseAsOf`:
it accepts **RFC3339** (`2026-06-17T14:30:00Z`, the primary machine-readable form
cerebro already emits) AND a **date-only** `2006-01-02` form (interpreted as
midnight UTC) for ergonomics. All input is normalized to UTC before storage /
comparison. `parseAsOf` returns an error — and never panics — for empty,
whitespace, garbage, partial, or out-of-range input, so a malformed flag surfaces
as a CLI error before any store write or query. The flags are
`--valid-at` / `--invalid-at` on `cerebro edge` and `--as-of` on
`recall` / `search` / `get`.

### 5. Re-add is a full-window re-assertion (upsert overwrite)

Re-running `cerebro edge` for an existing `(source, target, relation)` UPDATES
the validity window in place via
`ON CONFLICT (source_id, target_id, relation) DO UPDATE SET valid_at = excluded.valid_at, invalid_at = excluded.invalid_at`
— no duplicate row, the existing `id` retained. `excluded.*` is the native
SQLite alias for the `?`-bound conflicting-insert row (the in-repo upsert idiom),
so this introduces no new injection surface.

The re-add carries the **full window, not a partial patch**: a NULL bound on the
re-add **overwrites** any prior non-NULL value (e.g. re-adding with only
`--valid-at` clears `invalid_at` to NULL, re-opening the window). The
COALESCE-preserve alternative ("keep the old value when a bound is omitted") was
rejected because it would make cerebro **infer** intent — forbidden by the
no-inference invariant — and would weaken determinism (the result would depend on
hidden DB state rather than the argv). Full overwrite is correct-by-invariant; an
inverted window is rejected before the write.

The persisted `id` is resolved via `RETURNING id` (SQLite ≥ 3.35), because
`LastInsertId()` is unreliable on the conflict/update path (AUTOINCREMENT is not
re-fired), so a re-add reliably returns the existing id rather than 0 or a stale
value.

### 6. `AddEdgeOpts` struct, not variadic options

`Store.AddEdge` / `Brain.AddEdge` take a value-struct `AddEdgeOpts{ValidAt,
InvalidAt *time.Time}`. The `*time.Time` pointers already encode optionality, and
a struct avoids signature churn at future call sites. Variadic functional options
(`WithValidAt(...)`) were the alternative but were rejected following the OO-011
precedent, where the Tech Lead deemed variadic over-engineering for a small fixed
set of optional fields and chose the struct/positional idiom. The brain-layer
as-of threading uses a trailing positional `asOf *time.Time` on
`Search`/`SearchWithGlobal`/`Get` (after `subtypeFilter`) — the same OO-011
positional-pointer idiom.

## Consequences

- **Breaking Go API change.** `Brain.Search`/`SearchWithGlobal` gain a positional
  `asOf *time.Time`; `Brain.Get`, `Store.GetNodeWithEdges`,
  `Store.ExpandGraph`/`GetEdgesBatch`/`getEdgesForNode` gain `asOf`;
  `AddEdge` takes `AddEdgeOpts`. This rides a new cerebro major. The external
  `qraftworx-cli` consumer (which still calls the pre-OO-011 4-arg `Search`)
  absorbs this break downstream, bundling `subtypeFilter` + `asOf`.
- **Lazy-gate interaction.** When the agentic-73l6 expansion gate skips
  `ExpandGraph` for a query, no edges are traversed, so `--as-of` is a no-op on
  that query (nothing to filter). Defensible and documented; both gate paths are
  tested.
- **No new dependency.** stdlib `time` + the existing `mattn/go-sqlite3` driver.
  Export/import round-trips the two columns.
- **Trust model.** On the local single-operator brain, the full-window-overwrite
  semantic is a usability footgun (silent re-open of a closed window), not a
  security boundary — the agent already owns its graph. IF cerebro ever gates
  edges for access-control on shared/multi-operator data, silent re-open
  re-grades to a MEDIUM integrity-tampering vector and should gate that
  transition.

## Alternatives considered

1. Reuse `created_at` as valid-time — rejected (conflates transaction time with
   valid time; breaks backdating).
2. Go-side post-filtering after fetch — rejected (materialises the full edge set,
   duplicated at every call site).
3. Closed-closed `[valid_at, invalid_at]` interval — rejected (abutting windows
   double-match at the boundary).
4. INTEGER epoch storage — rejected (lone inconsistent representation).
5. Variadic functional options on `AddEdge` — rejected (OO-011 precedent: struct
   chosen over variadic for a small optional set).
6. COALESCE-preserve on re-add — rejected (would infer "keep old value", violating
   the no-inference invariant and weakening determinism).
