//go:build fts5

package brain

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/coetzeevs/cerebro/internal/store"
)

// brainWithVecFTS builds a brain (fts5 build) with a real vector table + fake
// embedder, seeds n generic nodes, and additionally inserts one node whose
// content carries an exact identifier the vector lane will NOT rank first. It
// returns the brain and the identifier node's ID.
func brainWithVecFTS(t *testing.T, n, dims int) (b *Brain, idNodeID string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "vecfts.sqlite")
	b, err := Init(path, EmbedConfig{Provider: "none"})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })

	if err := b.store.InitVectorTable(dims); err != nil {
		t.Fatalf("InitVectorTable: %v", err)
	}
	b.embedder = fakeEmbedder{dims: dims}

	for i := 0; i < n; i++ {
		id, addErr := b.store.AddNode(&store.AddNodeOpts{
			Type:       store.TypeConcept,
			Content:    fmt.Sprintf("node %d generic content", i),
			Importance: 0.5,
		})
		if addErr != nil {
			t.Fatalf("AddNode %d: %v", i, addErr)
		}
		vec := make([]float32, dims)
		vec[0] = 1.0
		vec[1] = float32(i) / float32(n) * 0.01
		if err := b.store.StoreEmbedding(id, vec); err != nil {
			t.Fatalf("StoreEmbedding %d: %v", i, err)
		}
	}

	// The exact-identifier node: a vector pointing AWAY from the query cluster so
	// the vector lane ranks it last, but the keyword lane (MATCH HS-049) finds it.
	idNode, err := b.store.AddNode(&store.AddNodeOpts{
		Type:       store.TypeProcedure,
		Content:    "HS-049 incident exact identifier postmortem",
		Importance: 0.5,
	})
	if err != nil {
		t.Fatalf("AddNode id-node: %v", err)
	}
	awayVec := make([]float32, dims)
	awayVec[dims-1] = 1.0 // orthogonal-ish to the [1,0,...] query
	if err := b.store.StoreEmbedding(idNode, awayVec); err != nil {
		t.Fatalf("StoreEmbedding id-node: %v", err)
	}
	return b, idNode
}

// TestBrainSearch_BM25SurfacesExactIdentifier (AC2) — with BM25 enabled
// (default), a query containing an exact identifier present in nodes_fts surfaces
// the matching node into the result set, and that node carries a non-zero BM25
// keyword signal. The vector lane alone ranks the node last (away vector), so its
// presence in the top results is attributable to the keyword lane.
func TestBrainSearch_BM25SurfacesExactIdentifier(t *testing.T) {
	b, idNode := brainWithVecFTS(t, 12, 8)

	// Sanity: the keyword lane carries a non-zero BM25 signal for the match.
	kw, err := b.store.KeywordSearch("HS-049", 10)
	if err != nil {
		t.Fatalf("KeywordSearch: %v", err)
	}
	if len(kw) == 0 || kw[0].Similarity <= 0 {
		t.Fatalf("expected non-zero BM25 contribution for HS-049, got %+v", kw)
	}

	// BM25 is on by default — Search must surface the identifier node.
	res, err := b.Search(context.Background(), "HS-049", 5, 0.0, nil)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !containsID(res, idNode) {
		t.Fatalf("BM25-enabled Search did not surface exact-identifier node %s: %v", idNode, ids(res))
	}
}

// TestBrainSearch_BM25DisabledIsIdentity (AC4-NR floor / TL finding 2) — with
// bm25_enabled=false, Search returns the EXACT pre-BM25 result (same nodes, same
// order) as a brain that never had the keyword lane. We assert the disabled-path
// output equals VectorSearch->ExpandGraph->cut directly.
func TestBrainSearch_BM25DisabledIsIdentity(t *testing.T) {
	b, _ := brainWithVecFTS(t, 12, 8)
	if err := b.store.SetMeta("config.bm25_enabled", "false"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}

	const limit = 5
	got, err := b.Search(context.Background(), "HS-049", limit, 0.0, nil)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	// Reconstruct the literal pre-BM25 path.
	vec, _ := b.embedder.Embed(context.Background(), "HS-049")
	results, err := b.store.VectorSearch(vec, limit, 0.0)
	if err != nil {
		t.Fatalf("VectorSearch: %v", err)
	}
	expanded, err := b.store.ExpandGraph(results, limit)
	if err != nil {
		t.Fatalf("ExpandGraph: %v", err)
	}
	want := expanded
	if len(want) > limit {
		want = want[:limit]
	}

	if len(got) != len(want) {
		t.Fatalf("disabled path length %d != pre-BM25 length %d", len(got), len(want))
	}
	for i := range want {
		if got[i].ID != want[i].ID {
			t.Fatalf("disabled path NOT identity at %d: got %v, want %v", i, ids(got), ids(want))
		}
	}
}

func containsID(nodes []store.ScoredNode, id string) bool {
	for i := range nodes {
		if nodes[i].ID == id {
			return true
		}
	}
	return false
}
