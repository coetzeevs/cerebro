# ADR-013: BM25 Keyword Recall via SQLite FTS5

**Date:** 2026-06-12
**Status:** Accepted
**Ticket:** agentic-2lak
**Supersedes:** —
**Superseded by:** —

## Context

Cerebro retrieves recall candidates purely by vector similarity
(`internal/store/VectorSearch` → `ExpandGraph`, scored by the four-signal
`compositeScore` = `0.35·relevance + 0.25·importance + 0.25·recency +
0.15·structural`). Vector similarity misses the **exact-identifier-match gap**:
a query for `HS-049`, `BUG-001`, or a precise symbol name surfaces
semantically-adjacent memories but not necessarily the memory that literally
contains the token. The recall-quality epic (`agentic-0ndn`) identifies
keyword/BM25 recall as a density-independent lever that closes this gap.

Two load-bearing realities constrain the implementation:

1. **FTS5 is NOT in cerebro's current build.** `mattn/go-sqlite3 v1.14.34` gates
   FTS5 behind the `//go:build sqlite_fts5 || fts5` CGO build tag
   (`sqlite3_opt_fts5.go:6`), set nowhere in cerebro. Live-proven: without the
   tag, `CREATE VIRTUAL TABLE … USING fts5` → `no such module: fts5`; with
   `-tags fts5`, it links. It remains **zero new Go-module dependencies** (the
   tag is a CGO compile flag on the already-vendored driver), but it is a
   cross-cutting build-config change that must reach **every** binary the build
   emits — local, CI, and the goreleaser-produced Homebrew darwin binary — or
   BM25 silently fails at runtime.

2. **The recall pipeline changed under agentic-2ixw (just merged).**
   `Brain.Search` now over-retrieves → optional cross-encoder rerank → RRF-fuse →
   cut, with `compositeScore` untouched. Adding BM25 must compose with both the
   existing composite AND the rerank/RRF layer without breaking the 2ixw
   default-off rerank contract.

## Decision

### D1 — Standalone FTS5 table, not external-content

`nodes_fts(node_id UNINDEXED, content, subtype)` is a standalone FTS5 virtual
table mirroring each active node's searchable text. `nodes.id` is a TEXT UUID;
FTS5 external-content tables (`content='nodes'`) require an INTEGER
`content_rowid`, so a standalone table sidesteps the rowid-mapping complexity
entirely. The storage cost (content duplicated in the index) is acceptable for a
single-operator workstation brain (~550 nodes).

### D2 — Application-level CRUD sync, not SQLite triggers

`nodes_fts` is kept in sync by **application-level writes in the store CRUD
methods** (`AddNode`, `UpdateNode`, `SupersedeNode`, and the GC eviction delete
loop), NOT by SQLite triggers. A trigger referencing `nodes_fts` fires
*unconditionally inside SQLite*, so a binary built **without** the `fts5` tag
would fail **every `INSERT INTO nodes`** the moment the trigger references a
table that cannot exist — coupling the optional build tag to basic writes.
Application-level sync lets the FTS write be conditionally skipped when
`nodes_fts` is absent (graceful degrade, mirroring the existing `vec_nodes`
tolerance in `search.go`/`gc.go`). `content`/`subtype` are bound as `?`
parameters (never concatenated). On the transactional write paths
(`SupersedeNode`, GC) the FTS write shares the primary `nodes` write's
transaction so a sync error rolls the whole operation back atomically — `nodes`
and `nodes_fts` can never half-commit.

### D3 — RRF fusion of two recall sets, not a fifth composite weight

BM25 composes via **Reciprocal Rank Fusion of two recall sets** (the
vector/composite-ordered lane and the keyword/BM25-ordered lane), not as a fifth
term in `compositeScore`. `bm25()` is unbounded-negative while cosine similarity
is `[0,1]`; adding BM25 to `compositeScore` would require normalising an
unbounded score and re-tuning all weights — high-risk for the non-regression
floor. RRF consumes *ranks*, never raw scores, is parameter-free (`k=60`,
Cormack/Clarke/Buettcher SIGIR 2009 — the same constant the 2ixw reranker
already uses), and **degrades to the identity when the keyword lane is empty**.
A new `brain.fuseRecallRRF(vectorSet, keywordSet, k)` implements it, reusing the
shipped `fuseRRF` rank idea but operating over two candidate *sets* (which may
overlap) rather than one set with a parallel score array. A node present in only
one lane still contributes its single reciprocal-rank term, so the
exact-identifier node the vector lane missed is added to the fused set.

### D4 — Fuse at the recall layer, before the optional reranker

The keyword lane is fused into the candidate set *before* `searchReranked` /
`applyRerankWithFusion` runs, so the 2ixw reranker receives a
keyword-aware-but-composite-ordered set exactly as it does today. The reranker
package (`internal/rerank/`) and its three config keys (`rerank_enabled`,
`rerank_command`, `rerank_fusion`) are **untouched**. Both paths non-regress:
`rerank=off` → fuse → cut; `rerank=rrf` → fuse → reranker → RRF → cut.

### D5 — BM25 always-on when FTS5 is present, with a config seam

BM25 ships unconditionally alongside vector recall. One config key
`bm25_enabled` (bool, **default `true`**) mirrors the shipped `rerank_enabled`
pattern. `bm25_enabled=false` short-circuits **both** `KeywordSearch` and
`fuseRecallRRF`, producing the literal pre-BM25 code path — this is the
eval/diagnostic seam that yields the same-session BM25-disabled floor for the
non-regression protocol. It is **not** an end-user feature toggle (the README
frames it as a diagnostic seam).

### D6 — `nodes_fts` created and backfilled on schema migration

`nodes_fts` is created by a **separate guarded call** (`initFTSTable`), never
inside the `applySchema` `stmts[]` loop (which aborts `Init`/`Open` on the first
error — inlining the FTS create there would brick every no-fts5 binary on store
open). On a no-fts5 binary the create logs once and continues. `schemaVersion`
moves `"2"→"3"`; the v2→v3 migration creates + one-time-backfills `nodes_fts`
from the existing active nodes. `initFTSTable` is idempotent and self-healing —
called on every `Open` — so the first fts5-tagged binary to open a brain creates
the index even if a prior no-fts5 binary already advanced the version.

### Build tag on all build paths

`-tags fts5` is added to the `Makefile` (`build`/`test`/`test-cover`),
`.github/workflows/ci.yml` (test step), and `.goreleaser.yml`
(`builds[].tags: [fts5]`). `.github/workflows/release.yml` needs no edit — it
fires goreleaser, which reads `.goreleaser.yml`. The released darwin binary is
verified to link FTS5 by probing the goreleaser-produced artifact
(`CREATE VIRTUAL TABLE … USING fts5` succeeds / `nodes_fts` is created).

## Security

The FTS5 MATCH query is a **second-order injection surface even under `?`
parameter binding**: `?`-binding stops classic SQL injection, but the FTS5
parser still treats the bound value as an *expression* — `AND/OR/NOT/NEAR/*/":/^`
in user text error or silently change semantics. The MATCH-expression builder
(`buildMatchQuery`) neutralises this by wrapping the whole user query as a single
literal FTS5 phrase (double-quote-wrap, double internal quotes), so no operator
from user text reaches the FTS5 parser. Live-proven (mattn/go-sqlite3 v1.14.34):
every adversarial payload (`HS-049 AND`, `HS-049 OR cats`, stray quote,
`content : cats`, `foo*`, `NEAR(a b)`, `^ticket`, embedded quotes) returns
cleanly (no error, no column hijack), while exact `HS-049` still matches under
the default `unicode61` tokenizer. An adversarial unit test covers the exact set
(S-PI-N1). This is graded availability/correctness (not breach) for the
single-operator local trust boundary, and re-grades to A03 HIGH if `nodes_fts`
MATCH is ever fed external/multi-tenant query input.

## Alternatives Rejected

- **External-content FTS5 table** — needs a synthetic INTEGER rowid on `nodes` +
  `content_rowid` plumbing; more migration surface, no recall benefit (D1).
- **SQLite triggers for sync** — cleaner SQL, but couples the build tag to basic
  writes: a no-fts5 binary would crash every `INSERT INTO nodes` (D2).
- **Weighted-sum / additive BM25 into `compositeScore`** — scale-fragile
  (unbounded bm25 vs `[0,1]` cosine), re-tunes four shipped weights, high-risk
  for the non-regression floor (D3).
- **`tokenize='unicode61 tokenchars '-''`** — considered for hyphenated IDs, but
  the default `unicode61` tokenizer already matches `HS-049` via phrase-quoting
  (live-proven count=1), so no custom tokenizer is warranted.

## Consequences

- BM25 keyword recall closes the exact-identifier gap with zero new Go-module
  dependencies and the four-signal composite weights unchanged.
- The non-regression floor is structurally protected: with the keyword lane
  empty (or `bm25_enabled=false`), the fused order collapses to today's composite
  order. Measured (same-session, both rerank paths) the BM25-on metrics equal the
  BM25-off floor exactly — no regression. See `docs/evals/bm25-results.md`.
- Every binary must carry the `fts5` tag or keyword recall silently no-ops; the
  AC5a grep + AC5b released-binary probe are the controls.
- The keyword lane reshapes the candidate set for exact-identifier queries
  (new nodes enter the fused top-K), but the committed eval corpus is not
  exact-identifier-stressed, so aggregate recall@K/MRR is unmoved on it — an
  honest directional finding recorded in the results doc.
