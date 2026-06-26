package brain

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/coetzeevs/cerebro/internal/store"
)

// gateBrainSeeded builds a brain with a real vector table + fakeEmbedder and
// seeds vec-indexed nodes whose cosine similarity against the fixed query
// vector [1,0,...] is controlled per node: sim_i = 1/sqrt(1 + c_i^2) for
// second-component c_i. Returns the brain and the seeded node IDs in seed
// order. (Same fixture lineage as brainWithVec — works around the
// noop-embedder early return, memory f132ce9c.)
func gateBrainSeeded(t *testing.T, comps []float64, dims int) (b *Brain, ids []string) {
	t.Helper()
	b = brainWithVec(t, 0, dims)

	ids = make([]string, len(comps))
	for i, c := range comps {
		id, err := b.store.AddNode(&store.AddNodeOpts{
			Type:       store.TypeConcept,
			Content:    fmt.Sprintf("gate seed %d", i),
			Importance: 0.5,
		})
		if err != nil {
			t.Fatalf("AddNode %d: %v", i, err)
		}
		vec := make([]float32, dims)
		vec[0] = 1.0
		vec[1] = float32(c)
		if err := b.store.StoreEmbedding(id, vec); err != nil {
			t.Fatalf("StoreEmbedding %d: %v", i, err)
		}
		ids[i] = id
	}
	return b, ids
}

// addUnembeddedNeighbor adds a node WITHOUT an embedding (so it can never
// enter VectorSearch results) and connects it to anchorID. It is reachable
// ONLY via ExpandGraph's edge walk — its presence in Search output is the
// observable proof that ExpandGraph ran; its absence (when seeded sims are
// high) proves the gate skipped expansion.
func addUnembeddedNeighbor(t *testing.T, b *Brain, anchorID string) string {
	t.Helper()
	nid, err := b.store.AddNode(&store.AddNodeOpts{
		Type:       store.TypeConcept,
		Content:    "edge-only neighbor",
		Importance: 0.5,
	})
	if err != nil {
		t.Fatalf("AddNode neighbor: %v", err)
	}
	if _, err := b.store.AddEdge(anchorID, nid, "relates_to", store.AddEdgeOpts{}); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}
	return nid
}

// gateContainsID reports whether id is present in nodes. Local to this file
// (NOT reusing search_bm25_test.go's containsID) because that file carries the
// fts5 build tag and this one must compile without it.
func gateContainsID(nodes []store.ScoredNode, id string) bool {
	for i := range nodes {
		if nodes[i].ID == id {
			return true
		}
	}
	return false
}

func skipCounter(t *testing.T, b *Brain) int {
	t.Helper()
	v, err := b.store.GetMeta(metaExpansionSkips)
	if err != nil {
		t.Fatalf("GetMeta(%s): %v", metaExpansionSkips, err)
	}
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		t.Fatalf("counter %s holds non-integer %q", metaExpansionSkips, v)
	}
	return n
}

// AC2a (observable outcome): with the synthetic top-1 similarity above
// expand_threshold, ExpandGraph is NOT called — the edge-only neighbor never
// appears, and the AC4 counter increments by exactly 1.
func TestSearch_GateSkipsExpansionAboveThreshold(t *testing.T) {
	// sims ≈ {1.0, 0.995, 0.981} — top-1 strictly exceeds 0.9.
	b, ids := gateBrainSeeded(t, []float64{0.0, 0.1, 0.2}, 8)
	neighbor := addUnembeddedNeighbor(t, b, ids[0])
	_ = b.store.SetMeta("config.expand_threshold", "0.9")
	_ = b.store.SetMeta("config.expand_spread_threshold", "0.0")

	before := skipCounter(t, b)
	got, err := b.Search(context.Background(), "qqqqq", 10, 0.0, nil, nil)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if gateContainsID(got, neighbor) {
		t.Errorf("gate-triggering query surfaced edge-only neighbor %s — ExpandGraph ran despite T1 fire", neighbor)
	}
	if d := skipCounter(t, b) - before; d != 1 {
		t.Errorf("skip counter delta = %d, want 1 (AC4: one skip event for one gated Search)", d)
	}
	if len(got) != len(ids) {
		t.Errorf("gated Search returned %d nodes, want the %d vector-lane nodes", len(got), len(ids))
	}
}

// AC2b (observable outcome): with a full result set (K == requestedK) whose
// spread is below expand_spread_threshold, expansion is skipped. The counter
// is the observable here (the limit-3 cap would cut the low-scored neighbor
// even on the expansion path, so neighbor-absence would not be probative).
func TestSearch_GateSkipsExpansionOnSmallSpread(t *testing.T) {
	// sims ≈ {0.894, 0.893, 0.892, 0.891, 0.890} — spread ≈ 0.004 over the
	// limit-3 window; K = R = 3 after the cap, so the K >= R guard passes.
	b, _ := gateBrainSeeded(t, []float64{0.50, 0.505, 0.51, 0.515, 0.52}, 8)
	_ = b.store.SetMeta("config.expand_threshold", "0.0") // T1 off
	_ = b.store.SetMeta("config.expand_spread_threshold", "0.05")

	before := skipCounter(t, b)
	if _, err := b.Search(context.Background(), "qqqqq", 3, 0.0, nil, nil); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if d := skipCounter(t, b) - before; d != 1 {
		t.Errorf("skip counter delta = %d, want 1 (T2 spread fire)", d)
	}

	// Contrast: spread condition at the 0.0 sentinel — same brain, no fire.
	_ = b.store.SetMeta("config.expand_spread_threshold", "0.0")
	before = skipCounter(t, b)
	if _, err := b.Search(context.Background(), "qqqqq", 3, 0.0, nil, nil); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if d := skipCounter(t, b) - before; d != 0 {
		t.Errorf("skip counter delta = %d, want 0 (0.0 sentinel must disable T2)", d)
	}
}

// AC2c (observable outcome): when NEITHER condition is met the existing
// expansion path is unchanged — ExpandGraph runs (the edge-only neighbor
// appears) and the counter stays flat.
func TestSearch_GateDoesNotFireBelowThreshold(t *testing.T) {
	// sims ≈ {0.894, 0.876, 0.857} — top-1 below 0.99; spread disabled.
	b, ids := gateBrainSeeded(t, []float64{0.50, 0.55, 0.60}, 8)
	neighbor := addUnembeddedNeighbor(t, b, ids[0])
	_ = b.store.SetMeta("config.expand_threshold", "0.99")
	_ = b.store.SetMeta("config.expand_spread_threshold", "0.0")

	before := skipCounter(t, b)
	got, err := b.Search(context.Background(), "qqqqq", 10, 0.0, nil, nil)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if !gateContainsID(got, neighbor) {
		t.Errorf("non-triggering query did NOT surface edge-only neighbor %s — expansion path changed (AC2c)", neighbor)
	}
	if d := skipCounter(t, b) - before; d != 0 {
		t.Errorf("skip counter delta = %d, want 0 for a non-triggering query (AC4 clause 3)", d)
	}
}

// AC3 (equivalence): at (0.0, 0.0) the gate never fires and Search output is
// identical to the hand-run pre-feature pipeline — VectorSearch → ExpandGraph
// → keyword-fusion(identity for this query) → cut — on the same brain WITH
// edges present. Comparison is rank-ordered IDs + Similarity (time-stable,
// pure cosine); raw composite scores are compared within ±0.001 because the
// recency term moves between any two invocations (TL Finding 1).
func TestSearch_ZeroZeroIdenticalToPreFeaturePath(t *testing.T) {
	b, ids := gateBrainSeeded(t, []float64{0.0, 0.1, 0.2, 0.5}, 8)
	neighbor := addUnembeddedNeighbor(t, b, ids[0])
	_ = b.store.SetMeta("config.expand_threshold", "0.0")
	_ = b.store.SetMeta("config.expand_spread_threshold", "0.0")

	const limit = 10
	before := skipCounter(t, b)
	got, err := b.Search(context.Background(), "qqqqq", limit, 0.0, nil, nil)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if d := skipCounter(t, b) - before; d != 0 {
		t.Errorf("skip counter delta = %d, want 0 at (0.0, 0.0)", d)
	}

	// Hand-run the literal pre-feature statements.
	vec, _ := b.embedder.Embed(context.Background(), "qqqqq")
	want, err := b.store.VectorSearch(vec, limit, 0.0)
	if err != nil {
		t.Fatalf("VectorSearch: %v", err)
	}
	want, err = b.store.ExpandGraph(want, limit, nil)
	if err != nil {
		t.Fatalf("ExpandGraph: %v", err)
	}
	if len(want) > limit {
		want = want[:limit]
	}

	if !gateContainsID(got, neighbor) {
		t.Fatalf("(0.0, 0.0) Search did not surface edge neighbor %s — gate fired when disabled", neighbor)
	}
	if len(got) != len(want) {
		t.Fatalf("(0.0, 0.0) Search returned %d, pre-feature path returned %d", len(got), len(want))
	}
	for i := range want {
		if got[i].ID != want[i].ID {
			t.Errorf("position %d: ID %q != pre-feature %q (rank order must match)", i, got[i].ID, want[i].ID)
		}
		if got[i].Similarity != want[i].Similarity {
			t.Errorf("position %d: Similarity %v != pre-feature %v (time-stable field must be exact)", i, got[i].Similarity, want[i].Similarity)
		}
		if d := got[i].Score - want[i].Score; d > 1e-3 || d < -1e-3 {
			t.Errorf("position %d: Score %v vs %v (recency-drift tolerance ±0.001 exceeded)", i, got[i].Score, want[i].Score)
		}
	}
}

// Skip-path parity: on an EDGELESS brain the gated output is exactly
// "ExpandGraph over an edgeless graph" — cutByScore replicates the identical
// sort+cap tail, so rank order matches the hand-run expansion byte-for-byte.
func TestSearch_GatedOutputEqualsEdgelessExpandGraph(t *testing.T) {
	b, _ := gateBrainSeeded(t, []float64{0.0, 0.1, 0.2, 0.3, 0.4}, 8)
	_ = b.store.SetMeta("config.expand_threshold", "0.5") // sims ≈ 1.0 → fires
	_ = b.store.SetMeta("config.expand_spread_threshold", "0.0")

	const limit = 3
	got, err := b.Search(context.Background(), "qqqqq", limit, 0.0, nil, nil)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	vec, _ := b.embedder.Embed(context.Background(), "qqqqq")
	want, err := b.store.VectorSearch(vec, limit, 0.0)
	if err != nil {
		t.Fatalf("VectorSearch: %v", err)
	}
	want, err = b.store.ExpandGraph(want, limit, nil) // edgeless: sort+cap only
	if err != nil {
		t.Fatalf("ExpandGraph: %v", err)
	}
	if len(want) > limit {
		want = want[:limit]
	}

	if len(got) != len(want) {
		t.Fatalf("gated Search returned %d, edgeless ExpandGraph path returned %d", len(got), len(want))
	}
	for i := range want {
		if got[i].ID != want[i].ID {
			t.Errorf("position %d: gated ID %q != edgeless-ExpandGraph ID %q", i, got[i].ID, want[i].ID)
		}
	}
}

// searchReranked site: the gate also covers the rerank-enabled branch. With a
// nil reranker (no command) the pipeline degrades to composite order, but the
// gate decision happens before rerank — counter increments on a confident set.
func TestSearchReranked_GateSkipsExpansion(t *testing.T) {
	b, ids := gateBrainSeeded(t, []float64{0.0, 0.1, 0.2}, 8)
	neighbor := addUnembeddedNeighbor(t, b, ids[0])
	_ = b.store.SetMeta("config.rerank_enabled", "true")
	_ = b.store.SetMeta("config.expand_threshold", "0.9")

	before := skipCounter(t, b)
	got, err := b.Search(context.Background(), "qqqqq", 10, 0.0, nil, nil)
	if err != nil {
		t.Fatalf("Search (reranked): %v", err)
	}
	if gateContainsID(got, neighbor) {
		t.Errorf("reranked gated query surfaced edge-only neighbor %s", neighbor)
	}
	if d := skipCounter(t, b) - before; d != 1 {
		t.Errorf("skip counter delta = %d, want 1 (searchReranked site gated)", d)
	}
}

// SearchWithGlobal sites: both per-store gates fire independently on their
// own result sets, and BOTH increment the PROJECT brain's counter (TL
// Finding 2 — single-counter arithmetic; the global brain's schema_meta is
// never written).
func TestSearchWithGlobal_GatesBothStoresCounterOnProject(t *testing.T) {
	proj, pids := gateBrainSeeded(t, []float64{0.0, 0.1}, 8)
	glob, _ := gateBrainSeeded(t, []float64{0.05, 0.15}, 8)
	neighbor := addUnembeddedNeighbor(t, proj, pids[0])
	_ = proj.store.SetMeta("config.expand_threshold", "0.9")
	_ = proj.store.SetMeta("config.expand_spread_threshold", "0.0")

	beforeProj := skipCounter(t, proj)
	got, err := proj.SearchWithGlobal(context.Background(), "qqqqq", 5, 0.0, glob, nil, nil)
	if err != nil {
		t.Fatalf("SearchWithGlobal: %v", err)
	}

	if gateContainsID(got, neighbor) {
		t.Errorf("gated SearchWithGlobal surfaced project edge-only neighbor %s", neighbor)
	}
	if d := skipCounter(t, proj) - beforeProj; d != 2 {
		t.Errorf("project skip counter delta = %d, want 2 (project + global gates, both events on the PROJECT brain)", d)
	}
	if g := skipCounter(t, glob); g != 0 {
		t.Errorf("global brain counter = %d, want 0 (skip events must never scatter across brains)", g)
	}
}

// SearchWithGlobal at (0.0, 0.0): both expansions run (the project edge
// neighbor surfaces) and no counter moves — the disabled path is the
// pre-feature path at both sites.
func TestSearchWithGlobal_ZeroZeroExpandsBothStores(t *testing.T) {
	proj, pids := gateBrainSeeded(t, []float64{0.0, 0.1}, 8)
	glob, _ := gateBrainSeeded(t, []float64{0.05, 0.15}, 8)
	neighbor := addUnembeddedNeighbor(t, proj, pids[0])
	_ = proj.store.SetMeta("config.expand_threshold", "0.0")
	_ = proj.store.SetMeta("config.expand_spread_threshold", "0.0")

	before := skipCounter(t, proj)
	got, err := proj.SearchWithGlobal(context.Background(), "qqqqq", 5, 0.0, glob, nil, nil)
	if err != nil {
		t.Fatalf("SearchWithGlobal: %v", err)
	}
	if !gateContainsID(got, neighbor) {
		t.Errorf("(0.0, 0.0) SearchWithGlobal did not surface project edge neighbor %s", neighbor)
	}
	if d := skipCounter(t, proj) - before; d != 0 {
		t.Errorf("skip counter delta = %d, want 0 at (0.0, 0.0)", d)
	}
}
