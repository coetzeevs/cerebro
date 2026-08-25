package brain

import (
	"fmt"
	"os"
	"sort"

	"github.com/coetzeevs/cerebro/internal/store"
)

// fuseKeywordLane composes the BM25 keyword recall lane into an
// already-composite-ordered candidate set, returning the keyword-aware fused
// set. It is the single gated seam used by every Brain.Search variant.
//
// TL finding 2 (the AC4-NR floor must be the LITERAL pre-BM25 path): when
// bm25_enabled=false, this SHORT-CIRCUITS — it does not call KeywordSearch and
// does not call fuseRecallRRF, returning the input candidate set byte-identical
// to the pre-BM25 pipeline. So the same-session disabled floor is the exact
// pre-2lak code path, not merely the fusion-identity of an empty keyword set.
//
// When enabled, it runs KeywordSearch (injection-safe, S-PI-N1; graceful empty
// slice when nodes_fts is absent on a no-fts5 binary) over the same width as the
// candidate set and fuses by RRF. An empty keyword set still degrades to the
// identity inside fuseRecallRRF — a second line of defence.
func (b *Brain) fuseKeywordLane(query string, candidates []store.ScoredNode, width int) []store.ScoredNode {
	if !resolveBM25Enabled(b.store) {
		return candidates // literal pre-BM25 path (AC4-NR floor)
	}
	keyword, err := b.store.KeywordSearch(query, width)
	if err != nil {
		// Keyword lane must never worsen recall: on any error, degrade to the
		// composite-ordered candidate set (the disabled floor). No keyword signal.
		return candidates
	}
	return fuseRecallRRF(candidates, keyword, defaultRRFK)
}

// fuseRecallRRF fuses the vector/composite-ordered recall set with the
// keyword/BM25-ordered recall set via Reciprocal Rank Fusion (agentic-2lak D3),
// returning a single ranked, de-duplicated slice. It reuses the same RRF idea
// the 2ixw reranker shipped (fuseRRF), but operates over TWO candidate SETS
// (which may have different/overlapping members) rather than one set with a
// parallel score array.
//
// For each node the fused score is the sum of reciprocal-rank terms from each
// lane it appears in:
//
//	fused(n) = 1/(k + rank_vector(n))  [if n in vector lane]
//	         + 1/(k + rank_keyword(n)) [if n in keyword lane]
//
// with ranks 1-based (Cormack et al., SIGIR 2009; the defaultRRFK=60 standard
// the 2ixw reranker already uses). A node in only one lane still contributes its
// single term — so the exact-identifier node the vector lane missed is added.
//
// Identity contract (the AC4-NR floor, §5): an EMPTY keyword set returns the
// vector set UNCHANGED (same nodes, same order, same composite Score) — the
// keyword lane contributes nothing, so the fused order collapses to today's
// composite order. The disabled-BM25 path skips this call entirely (see
// Brain.Search), so the floor is the literal pre-BM25 code path; this identity
// is the belt-and-braces second line of defence.
//
// Composite ScoredNode.Score is preserved on each node — fusion governs ordering
// only. When a node appears in both lanes, the vector-lane copy (which carries
// the composite Score and Similarity) is kept; the keyword-lane copy is used
// only for its rank contribution. A node present only in the keyword lane keeps
// that lane's ScoredNode (its keyword-relevance Similarity, zero composite
// Score). A non-positive k is clamped to defaultRRFK.
func fuseRecallRRF(vectorSet, keywordSet []store.ScoredNode, k int) []store.ScoredNode {
	// Identity: no keyword signal => return the vector set unchanged.
	if len(keywordSet) == 0 {
		return vectorSet
	}
	if k < 1 {
		k = defaultRRFK
	}

	type entry struct {
		node  store.ScoredNode
		score float64
		order int // first-seen order, for deterministic tie-breaking
	}
	index := make(map[string]*entry, len(vectorSet)+len(keywordSet))
	order := make([]*entry, 0, len(vectorSet)+len(keywordSet))

	// Vector lane: 1-based ranks. The vector copy carries the composite Score, so
	// it is the canonical ScoredNode kept for nodes present in both lanes.
	for i := range vectorSet {
		id := vectorSet[i].ID
		e, ok := index[id]
		if !ok {
			e = &entry{node: vectorSet[i], order: len(order)}
			index[id] = e
			order = append(order, e)
		}
		e.score += 1.0 / float64(k+i+1)
	}

	// Keyword lane: 1-based ranks. For a node already seen in the vector lane we
	// only ADD its reciprocal-rank term (keep the vector copy). For a keyword-only
	// node we register its keyword ScoredNode.
	for i := range keywordSet {
		id := keywordSet[i].ID
		e, ok := index[id]
		if !ok {
			e = &entry{node: keywordSet[i], order: len(order)}
			index[id] = e
			order = append(order, e)
		}
		e.score += 1.0 / float64(k+i+1)
	}

	// Sort by descending fused score; ties broken by ascending first-seen order
	// (vector lane first) so fusion is deterministic and the vector-ordered
	// winner stays ahead.
	sort.SliceStable(order, func(a, b int) bool {
		if order[a].score != order[b].score {
			return order[a].score > order[b].score
		}
		return order[a].order < order[b].order
	})

	out := make([]store.ScoredNode, len(order))
	for i := range order {
		out[i] = order[i].node
	}
	return out
}

// searchKeywordOnly is the N3 embedder-failure fallback: keyword-lane-only
// recall when the query cannot be embedded. BM25 rank is the ranking signal
// (mirroring the fusion contract); Scores degrade to importance+recency via
// RescoreKeywordOnly. No expansion (the gate's similarity precondition does
// not hold), no rerank, no global-store lane. If the FTS index is unavailable
// too, the original embed error is returned — an empty silent success would
// hide a total outage.
func (b *Brain) searchKeywordOnly(query string, limit int, subtypeFilter *string, embedErr error) ([]store.ScoredNode, error) {
	if !b.store.FTSAvailable() {
		return nil, fmt.Errorf("embedding query: %w (and FTS5 keyword index unavailable — no fallback lane)", embedErr)
	}
	fmt.Fprintf(os.Stderr, "cerebro: embedding provider unavailable (%v); falling back to keyword-only recall (no semantic ranking, project store only)\n", embedErr)
	results, err := b.store.KeywordSearch(query, limit)
	if err != nil {
		return nil, fmt.Errorf("keyword fallback: %w (after embed failure: %v)", err, embedErr)
	}
	results = b.store.RescoreKeywordOnly(results)
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return filterScoredNodesBySubtype(results, subtypeFilter), nil
}
