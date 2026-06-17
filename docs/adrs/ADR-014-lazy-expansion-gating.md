# ADR-014: Lazy expansion gating — raw-cosine signal, ExpandGraph-only skip

**Status:** Accepted
**Date:** 2026-06-12
**Ticket:** agentic-73l6

## Context

Every `Brain.Search`/`SearchWithGlobal` query runs single-hop graph expansion
(`store.ExpandGraph`): two SQL round-trips (`GetEdgesBatch` +
`GetNodesByIDs`) plus neighbour scoring, even when the vector top-K already
answers the query with high confidence. The research basis
(`documentation/research/multi-hop-traversal/04-pragmatic-mitigations.md` §4:
L-RAG entropy-gated retrieval with 8–26% retrieval reduction, FLARE,
PruneRAG) identifies confidence-gated expansion as the cheapest recall-cost
lever — "only run graph expansion when the vector signal is weak".

## Decision

1. **Confidence signal = raw cosine `ScoredNode.Similarity`** — the value
   `VectorSearch` stamps on every vector-lane result (`1 − cosine_distance`,
   `internal/store/search.go`), NOT the composite score. The composite mixes
   `0.25 · Importance · (1+Log1p(AccessCount))` (unbounded — a high-importance
   high-access node exceeds 1 on that term alone) and a recency term
   `exp(-DecayRate · hoursSinceAccess)` that changes between any two
   invocations, so it is neither `[0,1]`-thresholdable nor time-stable. The
   research doc warns about exactly this ("composite scores … make threshold
   selection harder because the scale isn't probability-grounded").

2. **Gate position: immediately after `VectorSearch`, wrapping ONLY the four
   brain-layer `ExpandGraph` call sites** (`Search` rerank-disabled,
   `searchReranked`, `SearchWithGlobal` project + global). Pre-expansion is
   the only point where "top-1 similarity" is well-defined: ExpandGraph
   re-sorts by composite score and inserts `similarity=0` neighbours, and the
   BM25 fusion adds keyword-lane nodes after that. The 2lak keyword lane and
   the 2ixw reranker run unchanged on every query, gated or not.

3. **Gate predicate (pure, `brain/expand_gate.go`):** fires iff
   - **T1:** `expand_threshold > 0 && maxSim > expand_threshold`, OR
   - **T2:** `expand_spread_threshold > 0 && K ≥ 2 && K ≥ requestedK &&
     (maxSim − minSim) < expand_spread_threshold`.

   Every degenerate state disables the gate rather than firing it: empty set,
   singleton spread, partial result set (sparsity is where expansion helps
   most), `0.0` sentinels, NaN/negative/>1 thresholds. The skip path is
   `cutByScore` — ExpandGraph's exact `sort.Slice` Score-desc + cap tail, so
   the gated output is precisely "ExpandGraph over an edgeless graph" and
   downstream consumers still receive a composite-ordered set.

4. **Shipped defaults: `expand_threshold = 0.75` (ACTIVE), 
   `expand_spread_threshold = 0.0` (OFF).** The live 18-query sweep
   (`docs/evals/lazy-gating-results.md`) shows 0.72 regressing MRR
   (0.6459 → 0.6181) while 0.75/0.78/0.80 are bit-identical to the
   same-session floor; 0.75 is the lowest regression-free candidate with a
   non-zero skip-rate (4/18 = 22%, inside the L-RAG envelope). The spread
   condition ships OFF because on this brain the top-1→top-K spread
   **anti-correlates** with confidence — the lowest-confidence queries have
   the smallest spreads — so any active spread default would skip expansion
   on exactly the queries that need it. The mechanism stays implemented and
   tested; enabling it is config-only.

5. **Skip metric: `stats.expansion_skips`,** a persistent `schema_meta`
   counter incremented via the new atomic `store.IncrMeta` UPSERT on each
   gate fire, best-effort (`_ =` discard — a metrics write must never fail or
   slow a recall; the `has_pending_embeddings` idiom). It counts skip events
   per expansion site; `SearchWithGlobal` routes BOTH stores' events into the
   PROJECT brain's counter so the skip-rate arithmetic lives on one brain.

## Evidence

- **Source:** `04-pragmatic-mitigations.md` §4 (L-RAG/FLARE/PruneRAG,
  composite-scale warning); `internal/store/search.go` (similarity stamping,
  compositeScore, ExpandGraph sort/cap semantics).
- **Live checks:** the 18-query calibration table (top-1 ∈ [0.487, 0.842];
  spread anti-correlation: the worst queries all have spreads < 0.06); the
  full eval sweep + before/after runs in `docs/evals/lazy-gating-results.md`
  (561 active nodes, all runs same-session, deterministic).

## Alternatives rejected

- **Composite-score gating** — unbounded and time-varying (above); the same
  query would gate differently hours apart.
- **Gating the BM25 keyword lane too** — negligible savings (one FTS5 query)
  and it would remove keyword-only exact-identifier wins precisely on
  overconfident-vector queries, the known BM25-complementary failure mode.
- **Entropy-based signal (L-RAG proper)** — needs a probability-grounded
  score distribution cerebro doesn't have; the two-point spread is the
  bounded proxy, shipped OFF on anti-correlation evidence.

## Consequences

- Two SQL round-trips + neighbour scoring saved on ~22% of queries at zero
  measured recall cost on the rerank-disabled (default) pipeline.
- Gated queries forgo structural bonuses and edge-discovered neighbours —
  the intended skip semantic, bounded by the AC6-NR non-regression gate.
- On the **rerank-enabled** (non-default) path the neighbour-free candidate
  pool shifts RRF fusion ranks slightly: one query's first-relevant hit moved
  one rank (MRR −0.0013, recall@K unchanged) in the spot-check — recorded in
  the results artefact; operators wanting rerank-path byte-identity set
  `expand_threshold 0.0`.
- **Risk — threshold staleness:** the score distribution drifts as the brain
  grows; a query near sim ≈ 0.75 may flip across sessions (harmless to
  correctness, moves skip-rate). Mitigation: re-run the sweep whenever the
  eval baseline is regenerated (agentic-t3c9 lineage).
