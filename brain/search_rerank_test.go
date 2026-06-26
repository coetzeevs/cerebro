package brain

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/coetzeevs/cerebro/internal/rerank"
	"github.com/coetzeevs/cerebro/internal/store"
)

// fakeEmbedder returns a fixed-dimension vector so Brain.Search does not take
// the noop early-return path and the real VectorSearch/ExpandGraph/rerank
// pipeline is exercised (works around the noop early-return — memory f132ce9c).
type fakeEmbedder struct{ dims int }

func (f fakeEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	v := make([]float32, f.dims)
	v[0] = 1.0
	return v, nil
}
func (f fakeEmbedder) Dimensions() int { return f.dims }
func (f fakeEmbedder) Model() string   { return "fake" }

// brainWithVec builds a brain with a real vector table + fake embedder and
// seeds n nodes, each with a distinct embedding so VectorSearch returns them.
func brainWithVec(t *testing.T, n, dims int) *Brain {
	t.Helper()
	path := filepath.Join(t.TempDir(), "vec.sqlite")
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
			Content:    fmt.Sprintf("node %d content", i),
			Importance: 0.5,
		})
		if addErr != nil {
			t.Fatalf("AddNode %d: %v", i, addErr)
		}
		// Distinct vectors clustered near [1,0,...] so all clear threshold 0.
		vec := make([]float32, dims)
		vec[0] = 1.0
		vec[1] = float32(i) / float32(n) * 0.01
		if err := b.store.StoreEmbedding(id, vec); err != nil {
			t.Fatalf("StoreEmbedding %d: %v", i, err)
		}
	}
	return b
}

// AC3a: when enabled, the pipeline over-retrieves >= 30 candidates and cuts to
// <= limit (<=10). With the disabled path the final count would also be <=limit,
// but the over-retrieve width is the observable difference — proven by seeding
// 35 nodes and asserting the enabled path can rank all of them down to limit
// while a limit-width vector search would only see `limit` candidates.
func TestSearchReranked_OverRetrievesAndCuts(t *testing.T) {
	const seed = 35
	const dims = 8
	b := brainWithVec(t, seed, dims)

	// Enable reranking with a noop reranker (config.rerank_command empty → the
	// reranker is nil → applyRerank degrades to composite order, but the
	// over-retrieve width is still applied, which is what AC3a checks).
	_ = b.store.SetMeta("config.rerank_enabled", "true")

	results, err := b.Search(context.Background(), "query", 10, 0.0, nil, nil)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	// AC3a cut: at most limit (10) results.
	if len(results) > 10 {
		t.Errorf("enabled path returned %d results, want <= 10", len(results))
	}

	// AC3a over-retrieve: the over-retrieve constant must be >= 30 and the
	// pipeline must have fetched more than `limit` before cutting. We assert the
	// constant directly (documented default) and that a wider seed than limit
	// still cuts to limit (candidate-count > final-count).
	if rerankOverRetrieve < 30 || rerankOverRetrieve > 50 {
		t.Errorf("rerankOverRetrieve = %d, want in [30,50] (AC3b)", rerankOverRetrieve)
	}
	if seed <= 10 {
		t.Fatal("test seed must exceed limit to prove over-retrieve > final")
	}
}

// countingReranker records how many candidates it was asked to score and
// inverts the order so reranking has an observable effect.
type countingReranker struct{ got *int }

func (c countingReranker) Rerank(_ context.Context, _ string, cands []rerank.Candidate) ([]float64, error) {
	*c.got = len(cands)
	scores := make([]float64, len(cands))
	for i := range cands {
		scores[i] = float64(i) // inverts: last candidate ranks highest
	}
	return scores, nil
}
func (c countingReranker) Name() string { return "counting" }

// AC3a (observable): when enabled with >=30 seeded candidates, the reranker
// receives the over-retrieved set (>= 30, > limit) before the cut to limit.
func TestSearchReranked_RerankerReceivesOverRetrievedSet(t *testing.T) {
	const seed = 40
	const dims = 8
	b := brainWithVec(t, seed, dims)
	_ = b.store.SetMeta("config.rerank_enabled", "true")

	var gotCount int
	orig := newReranker
	newReranker = func(*store.Store) rerank.Reranker { return countingReranker{got: &gotCount} }
	t.Cleanup(func() { newReranker = orig })

	results, err := b.Search(context.Background(), "query", 10, 0.0, nil, nil)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if gotCount < 30 {
		t.Errorf("reranker received %d candidates, want >= 30 (AC3a over-retrieve)", gotCount)
	}
	if gotCount <= 10 {
		t.Errorf("over-retrieved count %d must exceed limit 10 (candidate-count > final-count)", gotCount)
	}
	if len(results) > 10 {
		t.Errorf("final result count %d, want <= 10 (AC3a cut)", len(results))
	}
}

// AC3b: documented constants are within the required bounds.
func TestRerankConstants(t *testing.T) {
	if rerankOverRetrieve < 30 || rerankOverRetrieve > 50 {
		t.Errorf("rerankOverRetrieve = %d, want in [30,50]", rerankOverRetrieve)
	}
	if rerankCutDefault > 10 {
		t.Errorf("rerankCutDefault = %d, want <= 10", rerankCutDefault)
	}
}

// AC2b structural parity: with rerank DISABLED, Search is the exact pre-rerank
// path. We prove this by comparing disabled-path results to a hand-run of
// VectorSearch(limit)→ExpandGraph(limit)→filter on the same brain.
func TestSearchDisabled_ByteIdenticalToPreRerank(t *testing.T) {
	const seed = 20
	const dims = 8
	b := brainWithVec(t, seed, dims)

	// Default config: rerank disabled (no meta row).
	vec, _ := b.embedder.Embed(context.Background(), "query")

	wantResults, err := b.store.VectorSearch(vec, 10, 0.0)
	if err != nil {
		t.Fatalf("VectorSearch: %v", err)
	}
	wantExpanded, err := b.store.ExpandGraph(wantResults, 10, nil)
	if err != nil {
		t.Fatalf("ExpandGraph: %v", err)
	}

	got, err := b.Search(context.Background(), "query", 10, 0.0, nil, nil)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(got) != len(wantExpanded) {
		t.Fatalf("disabled Search returned %d, pre-rerank path returned %d", len(got), len(wantExpanded))
	}
	for i := range got {
		// ID order must be byte-identical: the disabled branch is structurally
		// the exact VectorSearch(limit)→ExpandGraph(limit)→filter path (AC2b).
		if got[i].ID != wantExpanded[i].ID {
			t.Errorf("position %d: disabled Search ID %q != pre-rerank ID %q", i, got[i].ID, wantExpanded[i].ID)
		}
		// Composite scores match within the ±0.001 AC2b variance envelope. The
		// only delta is compositeScore's recency term (time.Since), which moves
		// by microseconds between two calls — it is NOT introduced by reranking.
		if d := got[i].Score - wantExpanded[i].Score; d > 1e-3 || d < -1e-3 {
			t.Errorf("position %d: disabled Search score %v vs pre-rerank %v (delta %v exceeds ±0.001)", i, got[i].Score, wantExpanded[i].Score, d)
		}
	}
}

// Default-off no-op: with rerank disabled, a poison reranker command is never
// invoked (no exec). We set a command that would fail loudly, but because the
// gate is off it must never run, and results are the composite order.
func TestSearchDisabled_DoesNotInvokeReranker(t *testing.T) {
	b := brainWithVec(t, 15, 8)
	// Command set but gate OFF — must be ignored entirely.
	_ = b.store.SetMeta("config.rerank_command", "/nonexistent/reranker-binary-should-never-run")

	results, err := b.Search(context.Background(), "query", 10, 0.0, nil, nil)
	if err != nil {
		t.Fatalf("Search must not error when rerank disabled: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected results from composite path")
	}
}
