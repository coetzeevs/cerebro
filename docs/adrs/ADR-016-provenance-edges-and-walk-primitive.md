# ADR-016: Provenance edges, the `WalkRelation` walk primitive, and the legacy boundary

**Status:** Accepted
**Date:** 2026-06-26
**Ticket:** agentic-lbjg

## Context

Provenance in cerebro was freeform: an agent that synthesized a concept from
several episodes had no structural way to record "this concept came from those
episodes." Pairing with the bi-temporal edges of agentic-xtzn (ADR-015), we make
provenance **structural** — a built-in `derived_from` relation linking a derived
node (concept/procedure/reflection) back to the source episodes it was
consolidated from, written automatically at consolidation time and queryable as a
lineage chain.

Three load-bearing, reuse-shaping decisions need a durable record because they
outlive this ticket (agentic-sx4u inherits the walk primitive; the legacy
boundary governs every future `provenance_status` read).

## Decision 1 — `WalkRelation` is BFS-in-Go, NOT a recursive CTE

cerebro's graph-query surface was **single-hop only** (`getEdgesForNode`
edges.go:68, `GetEdgesBatch` edges.go:96, `ExpandGraph` search.go:254). The
`--with-provenance` lineage chain needs a multi-hop, depth-bounded, cycle-safe,
direction-parameterised traversal. We add **one** reusable primitive:

```go
func (s *Store) WalkRelation(startID, relation string, maxDepth int, outgoing bool) ([]NodeWithDepth, error)
```

The PM design sketch leaned toward a SQLite recursive CTE ("more efficient for
deep walks"). We chose **BFS-in-Go with a node-ID visited set** instead, decided
by a live probe, not by reasoning.

### Evidence (live probe, SQLite 3.51.2 — the version compiled into `mattn/go-sqlite3 v1.14.34`)

A 3-cycle `A→B→C→A` walked with a `WITH RECURSIVE … UNION` and a depth cap of 5:

```sql
WITH RECURSIVE walk(node, depth) AS (
    SELECT 'A', 0
    UNION
    SELECT e.target, w.depth+1 FROM walk w JOIN edges e ON e.source = w.node
    WHERE w.depth < 5
)
SELECT node, depth FROM walk ORDER BY depth, node;
```

**Actual output:** `A@0 B@1 C@2 A@3 B@4 C@5`

The CTE dedupes by `(node, depth)` *row*, not by node, so it **re-walks the cycle
to the depth cap** — A, B, C each re-appear at depths 3/4/5. For a provenance
graph where `derived_from` can legitimately contain cycles (two concepts each
consolidating an episode that cites the other), the CTE both over-walks and
returns confusing duplicate-node chains.

A node-keyed Go visited set terminates the same cycle after visiting A, B, C
exactly once (`A@0 B@1 C@2`). `SELECT sqlite_version()` on the same probe returned
`3.51.2` (recursive CTEs supported since 3.8.3, so the difference is semantic, not
a capability gap).

### Consequences

- Clean per-node-once semantics; cycles and self-loops (`A derived_from A`)
  terminate (the start node is pre-seeded in `visited`).
- The walk issues one batched `GetEdgesBatch` per depth level (≤ maxDepth
  queries), filters relation + direction in Go, and authors **no new SQL** —
  reusing the injection-safe, `asOf`-aware batched edge query.
- `asOf` is passed as `nil` in v1: provenance walks ignore the bi-temporal
  validity window. "As-of provenance" (walking only edges valid at an instant) is
  a documented second-iteration feature; the signature is forward-compatible
  because `GetEdgesBatch` already takes `*time.Time`.
- A depth clamp of **100** bounds the per-level query count defensively (the BFS
  is already O(reachable nodes) by the visited set).
- agentic-sx4u inherits this primitive; the signature is kept clean for reuse.

## Decision 2 — `provenance_status` legacy boundary via a `schema_meta` migration-instant, not a per-node column

`provenance_status` (computed at query time, **not** a stored column) reports
`complete` / `none` / `legacy`:

- `complete` — the node has ≥1 outgoing `derived_from` edge.
- `none` — no such edge, created **at/after** the convention boundary.
- `legacy` — no such edge, created **before** the boundary (predates the
  convention; absence of provenance is expected, not a signal).

The only per-node creation timestamp is `nodes.created_at` (schema.go:33). There
is **no** per-node migration timestamp. So the boundary must be a *brain-level*
value, recorded once: the v4→v5 migration writes a `schema_meta` row
`provenance_convention_since = <migration instant>` (a fresh v5 `Init` stamps it
at the brain's birth instant, so a brand-new brain has no legacy era). The value
is stored in the repo's `storageTimeLayout` string and parsed **once** into a
`time.Time` before comparison.

The legacy check is `node.CreatedAt.Before(boundary)` — a **`time.Time` compare in
Go, strict `<`** (a node created at the exact boundary instant is non-legacy),
**never a lexicographic string compare**. `created_at` scans directly into
`Node.CreatedAt` (a `time.Time`) with no `time.Parse` (nodes.go scanners), so the
comparison is apples-to-apples on time values. If the boundary meta is missing or
unparseable, no node is classified legacy (every no-edge node reads `none`) — a
defensive default that never mislabels a node as predating a convention we cannot
date. `ProvenanceStatusBatch` does the EXISTS-over-edges in one batched query (no
N+1 on `recall`/`list`).

## Decision 3 — `consolidate --into` is a NEW command, atomic, fail-closed; `derived_from` reserved in code

There was no `consolidate --into` surface before lbjg; only `mark-consolidated`
(a status-flip that writes no edges). `consolidate --into <concept> <episode...>`
is new: it flips each source episode to `consolidated` **and** writes a
`derived_from` edge from the into-node to each source, in a **single
transaction**. The edge upsert is issued via `tx.Exec` with the same
`ON CONFLICT … DO UPDATE … RETURNING id` SQL as `AddEdge` — **not** by calling the
connection-level `AddEdge` from inside the open tx (a lock/visibility hazard that
would break all-or-nothing). It is **fail-closed**: the into-node and every
source must resolve as an episode before any write, else a non-zero error with
zero partial write (rollback verified). Idempotent via
`UNIQUE(source_id, target_id, relation)`. `mark-consolidated` is left untouched.

`derived_from` is reserved as a single exported Go constant
(`store.RelationDerivedFrom`), referenced by the consolidation writer and the
walk. The typed-relation **registry** that seeds reserved relations on init is
agentic-8l2g (out of scope here); lbjg only reserves the string.

## API additivity (no SemVer break)

`provenance_root` threads through `Brain.Add` via a **new variadic
`WithProvenanceRoot()` `AddOption`** (`brain.go` already uses `opts ...AddOption`);
`Brain.Consolidate` / `WalkProvenance` / `ProvenanceStatus` are new methods. No
existing positional signature is widened, so there is no public-API break and no
new downstream-absorb debt (unlike the xtzn/OO-011 `Search` breaks). No new
`go.mod` dependency.

## Alternatives considered

- **A new `episode_source` sibling node type** (PM sketch option b) — rejected: it
  would require editing the `type` CHECK enum, a destructive table rebuild in
  SQLite (no `ALTER … DROP CONSTRAINT`), and would fork the type taxonomy. A
  `provenance_root` boolean column is additive and retroactively promotable.
- **A recursive CTE for `WalkRelation`** — rejected on the live cycle-over-walk
  evidence above.
- **A per-node `provenance_status` stored column** — rejected: cheap to compute at
  query time (EXISTS + a batched created_at compare); storing it would add write
  paths and a backfill with no read benefit.
