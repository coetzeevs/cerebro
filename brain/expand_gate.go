package brain

// Lazy / threshold-bounded expansion policy (agentic-73l6).
//
// Graph expansion (store.ExpandGraph — two SQL round-trips: GetEdgesBatch +
// GetNodesByIDs, plus neighbour scoring) is skipped when the vector top-K is
// already confident. The confidence signal is the RAW COSINE SIMILARITY that
// VectorSearch stamps on every vector-lane result (ScoredNode.Similarity) —
// NOT the composite score, which is unbounded (the importance term
// Importance*(1+Log1p(AccessCount)) can exceed 1) and time-varying (recency
// decay), so it is not threshold-friendly (ADR-014; research basis:
// documentation/research/multi-hop-traversal/04-pragmatic-mitigations.md §4 —
// L-RAG / FLARE / PruneRAG confidence-gated retrieval).
//
// The gate wraps ONLY the four brain-layer ExpandGraph call sites. The BM25
// keyword lane (fuseKeywordLane, agentic-2lak) and the optional reranker
// (agentic-2ixw) run exactly as before on every query, gated or not.

import (
	"sort"
	"strconv"

	"github.com/coetzeevs/cerebro/internal/store"
)

// metaExpansionSkips is the schema_meta key for the AC4 skip counter. It
// counts SKIP EVENTS per expansion site (Search = one site per query;
// SearchWithGlobal = up to two events per query — project + global gates).
// The counter never resets; consumers read deltas. ALL sites — including the
// global store's gate — increment the PROJECT brain's counter, consistent
// with the project brain's config governing the whole call (the
// resolveRerankEnabled(b.store) precedent in SearchWithGlobal).
const metaExpansionSkips = "stats.expansion_skips"

// Compiled defaults (Design §5, confirmed by the eval sweep in
// docs/evals/lazy-gating-results.md). Mirrored as strings in the cmd/cerebro
// configRegistry, the README config table and the CHANGELOG — keep all four
// surfaces in sync when re-sweeping.
const (
	// defaultExpandThreshold ships ACTIVE: skip expansion when the top-1
	// cosine similarity strictly exceeds 0.80 — the most conservative
	// candidate with a non-zero live skip-rate (within the L-RAG-cited 8–26%
	// retrieval-reduction envelope). 0.0 disables the condition.
	defaultExpandThreshold = 0.80
	// defaultExpandSpreadThreshold ships OFF: on the live brain the top-1 to
	// top-K spread ANTI-correlates with confidence (the lowest-confidence
	// queries have the smallest spreads), so an active spread default would
	// skip expansion on exactly the queries that need it most. The mechanism
	// stays implemented and tested; enabling it is config-only.
	defaultExpandSpreadThreshold = 0.0
)

// resolveExpandThreshold reads config.expand_threshold via GetMeta (the
// rerankConfigPrefix idiom — env-free by design, mirroring resolveBM25Enabled
// and resolveRerankEnabled; no env var can enable or tune the gate).
// Unset or unparseable values fall back to the compiled default. A
// parseable-but-out-of-range value (NaN / negative / >1, possible only by
// writing schema_meta directly — the CLI validator rejects it) passes
// through: shouldSkipExpansion's guards then fail open to the baseline
// (full expansion runs; the gate never fires on a degenerate threshold).
func resolveExpandThreshold(s *store.Store) float64 {
	return resolveExpandFloat(s, "expand_threshold", defaultExpandThreshold)
}

// resolveExpandSpreadThreshold reads config.expand_spread_threshold — same
// contract as resolveExpandThreshold, default OFF (0.0).
func resolveExpandSpreadThreshold(s *store.Store) float64 {
	return resolveExpandFloat(s, "expand_spread_threshold", defaultExpandSpreadThreshold)
}

func resolveExpandFloat(s *store.Store, key string, fallback float64) float64 {
	val, _ := s.GetMeta(rerankConfigPrefix + key)
	if val == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return fallback
	}
	return f
}

// shouldSkipExpansion is the PURE lazy-gate predicate (no I/O, no Ollama —
// the f132ce9c pure-helper pattern). results is the raw VectorSearch output
// for this call site (every element carries a genuine vector-lane
// Similarity; no similarity=0 expansion neighbours or keyword-lane nodes
// exist yet). requestedK is the retrieve width passed to VectorSearch at the
// site (limit / over / perStore).
//
// The gate fires (skip expansion) iff T1 ∨ T2 (Design §2):
//
//	T1 (top-1 confidence):  topThreshold > 0 && maxSim > topThreshold
//	T2 (spread plateau):    spreadThreshold > 0 && K >= 2 && K >= requestedK
//	                        && (maxSim - minSim) < spreadThreshold
//
// maxSim/minSim are computed over the whole set (order-independent), so an
// upstream reorder can never silently break the predicate. Every degenerate
// state DISABLES the gate rather than firing it: K=0 (empty set), K=1 for
// T2 (spread undefined on a singleton), K < requestedK for T2 (a partial set
// signals sparsity — where expansion helps most), 0.0 sentinels (the > 0
// guards: structurally OFF, never "always fire"), NaN thresholds (NaN
// compares false to everything), negative thresholds (> 0 guard), and a
// top threshold > 1 (cosine sim ≤ 1 never strictly exceeds it).
func shouldSkipExpansion(results []store.ScoredNode, requestedK int, topThreshold, spreadThreshold float64) bool {
	k := len(results)
	if k == 0 {
		return false
	}

	maxSim, minSim := results[0].Similarity, results[0].Similarity
	for i := 1; i < k; i++ {
		s := results[i].Similarity
		if s > maxSim {
			maxSim = s
		}
		if s < minSim {
			minSim = s
		}
	}

	// T1 — top-1 confidence (strict >, per AC2a "exceeds").
	if topThreshold > 0 && maxSim > topThreshold {
		return true
	}

	// T2 — spread plateau (strict <, per AC2b "below").
	if spreadThreshold > 0 && k >= 2 && k >= requestedK && (maxSim-minSim) < spreadThreshold {
		return true
	}

	return false
}

// cutByScore replicates ExpandGraph's tail on the skipped path: sort by
// composite Score descending with the IDENTICAL comparator (sort.Slice, not
// SliceStable — search.go ExpandGraph parity), then cap at limit. Pure; no
// SQL. The gated result is exactly "ExpandGraph over an edgeless graph", so
// downstream consumers still receive the composite-ordered candidate set
// their contract documents (recall_fusion.go). Structural bonuses are
// intentionally forgone on gated queries — that is the skip semantic (no
// edge reads); AC6-NR bounds the recall cost.
func cutByScore(results []store.ScoredNode, limit int) []store.ScoredNode {
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results
}

// noteExpansionSkipped increments the AC4 skip counter, best-effort.
//
// The error discard below is LOAD-BEARING (S-3 / Tech Lead review): the
// counter is observability, and a metrics write must NEVER fail or slow a
// recall — a locked or read-only DB loses at most one count (the
// has_pending_embeddings idiom in Brain.Add). Do NOT "improve" this into a
// returned error.
func noteExpansionSkipped(s *store.Store) {
	_ = s.IncrMeta(metaExpansionSkips)
}
