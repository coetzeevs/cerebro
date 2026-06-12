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

### D7 — Tokenize-OR MATCH builder, not whole-query phrase (rework)

The MATCH-expression builder (`buildMatchQuery`) **tokenises the query on
whitespace, wraps each token in its own double-quoted FTS5 phrase (doubling
internal `"`), and joins the phrases with ` OR `** — e.g.
`"OO-015" OR "determinism" OR "wire"`.

The first implementation wrapped the **whole** query in one FTS5 phrase
(`"OO-015 determinism wire"`). An FTS5 phrase requires its terms to be
**adjacent** in the indexed text (sqlite.org/fts5.html §3, "a string of one or
more tokens enclosed in double quotes is a phrase"), so any multi-word query
matched nothing. Live-proven against the 556-row dogfooded index: the whole-query
phrase returned **0 rows**, while the tokenize-OR form returned **303 rows** and
the single-term `"HS-049"` returned **36 rows**. Because the entire eval corpus is
multi-word, the keyword lane was empty on every query and BM25 was inert — the
defect that made the first measurement show "no win". The single-term
exact-identifier case (the bare `"HS-049"`) always worked; only multi-word queries
were broken.

Tokenize-OR keeps injection safety **exactly** (S-PI-N1): every user token is an
individual quoted phrase, so no FTS5 operator from user text (`AND/OR/NOT/NEAR/*/:
/^/( )`) reaches the parser as syntax — it is literal phrase content. The ` OR ` is
**ours**, not the user's. A user word like `OR`/`AND`/`NEAR` becomes a quoted
literal phrase (inert, live-proven). Each term still matches independently, so the
rare identifier token matches inside a multi-word query and `bm25()`'s term-rarity
weighting (IDF) floats the rare identifier matches to the top of the keyword lane.

Noise consideration: OR-joining means a very common token (e.g. "the") can match
many nodes. This is acceptable because (a) `bm25()` down-weights common (low-IDF)
terms so they don't dominate the lane ranking, (b) RRF consumes only the *rank* of
each keyword hit and fuses it with the vector lane, and (c) the `width`/`limit` cut
bounds the lane size. The empirical result (`docs/evals/bm25-results.md`) confirms
no regression and a measured improvement, so the common-token noise does not harm
recall in practice.

Empty / all-whitespace input yields `""` (no tokens); `KeywordSearch`'s
empty-query guard short-circuits before this is ever bound as a MATCH.

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
(`buildMatchQuery`, D7) neutralises this by tokenising the query on whitespace and
wrapping **each** token as its own literal FTS5 phrase (double-quote-wrap, double
internal quotes), joined with our own ` OR `, so no operator from user text
reaches the FTS5 parser as syntax. Live-proven (mattn/go-sqlite3 v1.14.34): every
single-term adversarial payload (`HS-049 AND`, `HS-049 OR cats`, stray quote,
`content : cats`, `foo*`, `NEAR(a b)`, `^ticket`, embedded quotes) AND every
multi-word injection payload (`foo HS-049" OR x`, `a NEAR(b c)`, `x* y`,
`alpha AND beta OR gamma NOT delta`) returns cleanly (no error, no column hijack,
no parser syntax error — proof the operator never reached the parser), while exact
`HS-049` still matches under the default `unicode61` tokenizer. An adversarial unit
test covers both the single-term and multi-word sets (S-PI-N1). This is graded
availability/correctness (not breach) for the single-operator local trust
boundary, and re-grades to A03 HIGH if `nodes_fts` MATCH is ever fed
external/multi-tenant query input.

## Alternatives Rejected

- **External-content FTS5 table** — needs a synthetic INTEGER rowid on `nodes` +
  `content_rowid` plumbing; more migration surface, no recall benefit (D1).
- **SQLite triggers for sync** — cleaner SQL, but couples the build tag to basic
  writes: a no-fts5 binary would crash every `INSERT INTO nodes` (D2).
- **Weighted-sum / additive BM25 into `compositeScore`** — scale-fragile
  (unbounded bm25 vs `[0,1]` cosine), re-tunes four shipped weights, high-risk
  for the non-regression floor (D3).
- **`tokenize='unicode61 tokenchars '-''`** — considered for hyphenated IDs, but
  the default `unicode61` tokenizer already matches `HS-049` via per-token phrase
  quoting (live-proven), so no custom tokenizer is warranted.
- **Whole-query single-phrase MATCH** (the first implementation) — required all
  query terms to be adjacent, so every multi-word query matched 0 rows and BM25
  was inert; replaced by the tokenize-OR builder (D7).

## Consequences

- BM25 keyword recall closes the exact-identifier gap with zero new Go-module
  dependencies and the four-signal composite weights unchanged.
- The non-regression floor is structurally protected: with the keyword lane
  empty (or `bm25_enabled=false`), the fused order collapses to today's composite
  order. Measured (same-session, both rerank paths, N=3 deterministic) every
  BM25-on metric is `>=` its same-session BM25-off floor — AC4-NR PASS. See
  `docs/evals/bm25-results.md`.
- Every binary must carry the `fts5` tag or keyword recall silently no-ops; the
  AC5a grep + AC5b released-binary probe are the controls.
- With the tokenize-OR builder (D7) and the corpus extended with 8 multi-word
  exact-identifier queries (`q11`–`q18`), BM25 now shows a **measured aggregate
  improvement**, not just a directional finding: on the default rerank=off path
  the exact-id subset gains recall@5 +0.125, recall@10 +0.125, recall@20 +0.250
  (0.75 → 1.00), MRR +0.309, and the full corpus lifts on every metric. On the
  rerank=rrf path the cross-encoder already saturates recall@K on the exact-id
  subset, so BM25's contribution there is rank-quality (MRR +0.18) rather than
  coverage. Three of the eight new queries are genuine recall@K wins (the
  canonical identifier node was outside vector top-K, often missed entirely, and
  BM25 fusion pulls it in); the other five had the target already in vector top-5
  and are recorded honestly as MRR-only improvements.
