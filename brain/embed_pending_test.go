package brain

// embed_pending_test.go — agentic-h6gc (TDD).
//
// has_pending_embeddings was set on every embed failure but NOTHING ever
// re-embedded pending nodes or cleared the flag; Import's "nodes will need
// re-embedding" contract depended on a command that did not exist. Live
// impact: nodes invisible to vector recall. Also: the installed Ollama
// hard-fails (HTTP 500) on inputs over ~6-7.8KB chars, so two ~8KB estate
// memories could NEVER embed — oversized content must chunk + mean-pool
// through the shared embed path (fixing add/update too, not just backfill).

import (
	"context"
	"errors"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coetzeevs/cerebro/internal/embed"
	"github.com/coetzeevs/cerebro/internal/store"
)

// limitedEmbedder mimics the Ollama failure mode: hard error on inputs
// longer than maxChars; records every call's input length.
type limitedEmbedder struct {
	maxChars int
	calls    []int
	vec      []float32
}

func (e *limitedEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	e.calls = append(e.calls, len(text))
	if len(text) > e.maxChars {
		return nil, errors.New("input exceeds model limit (HTTP 500)")
	}
	out := make([]float32, len(e.vec))
	copy(out, e.vec)
	return out, nil
}
func (e *limitedEmbedder) Dimensions() int { return len(e.vec) }
func (e *limitedEmbedder) Model() string   { return "limited-test" }

func testBrainWithEmbedder(t *testing.T, e embed.Provider, dims int) *Brain {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.sqlite")
	b, err := Init(path, EmbedConfig{Provider: "none"})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })
	if err := b.store.InitVectorTable(dims); err != nil {
		t.Fatalf("InitVectorTable: %v", err)
	}
	b.embedder = e
	return b
}

// Oversized content chunks below the provider limit and stores ONE
// mean-pooled, L2-normalized vector — through the shared embed path, so a
// plain Add of a huge memory embeds instead of silently going pending.
func TestEmbedAndStore_ChunksOversizedContent(t *testing.T) {
	e := &limitedEmbedder{maxChars: 6000, vec: []float32{0.6, 0.8, 0, 0}}
	b := testBrainWithEmbedder(t, e, 4)

	big := strings.Repeat("memory content sentence. ", 400) // ~10KB
	id, err := b.Add(big, store.TypeEpisode)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if len(e.calls) < 2 {
		t.Fatalf("oversized content must embed in chunks: got %d call(s)", len(e.calls))
	}
	for i, n := range e.calls {
		if n > 6000 {
			t.Errorf("chunk %d exceeds provider limit: %d chars", i, n)
		}
	}
	pending, err := b.store.PendingEmbeddingNodes()
	if err != nil {
		t.Fatalf("PendingEmbeddingNodes: %v", err)
	}
	for _, n := range pending {
		if n.ID == id {
			t.Error("oversized node still pending — chunked vector not stored")
		}
	}
	// Mean of identical unit vectors normalizes back to the same unit vector.
	results, err := b.store.VectorSearch([]float32{0.6, 0.8, 0, 0}, 1, 0)
	if err != nil || len(results) == 0 {
		t.Fatalf("VectorSearch after chunked store: %v (%d results)", err, len(results))
	}
	if results[0].ID != id {
		t.Errorf("stored chunked vector not retrievable: got %s", results[0].ID)
	}
	_ = math.Sqrt // keep math import honest if assertions change
}

// EmbedPending backfills every active node lacking a vector, reports
// per-node results, and clears has_pending_embeddings only at zero remaining.
func TestEmbedPending_BackfillsAndClearsFlag(t *testing.T) {
	e := &limitedEmbedder{maxChars: 6000, vec: []float32{1, 0, 0, 0}}
	b := testBrainWithEmbedder(t, e, 4)

	// Two nodes stored WITHOUT vectors (simulating import / embed-down adds).
	idA, _ := b.store.AddNode(&store.AddNodeOpts{Type: store.TypeConcept, Content: "pending A", Importance: 0.5})
	idB, _ := b.store.AddNode(&store.AddNodeOpts{Type: store.TypeConcept, Content: "pending B", Importance: 0.5})
	if err := b.store.SetMeta("has_pending_embeddings", "true"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}

	res, err := b.EmbedPending(context.Background())
	if err != nil {
		t.Fatalf("EmbedPending: %v", err)
	}
	if len(res.Embedded) != 2 {
		t.Fatalf("embedded %d nodes, want 2 (%+v)", len(res.Embedded), res)
	}
	if len(res.Failed) != 0 || res.Remaining != 0 {
		t.Errorf("unexpected failures/remaining: %+v", res)
	}
	if v, _ := b.GetMeta("has_pending_embeddings"); v == "true" {
		t.Error("flag not cleared at zero remaining")
	}
	// Idempotent: second run embeds nothing.
	res2, _ := b.EmbedPending(context.Background())
	if len(res2.Embedded) != 0 {
		t.Errorf("second run re-embedded %d nodes, want 0", len(res2.Embedded))
	}
	_ = idA
	_ = idB
}

// A node whose embedding fails stays pending, is reported, and keeps the
// flag set — never silently dropped.
func TestEmbedPending_FailureKeepsFlagAndReports(t *testing.T) {
	// maxChars 10: every embed fails (chunking floor is far above this, so
	// even chunked attempts exceed the limit).
	e := &limitedEmbedder{maxChars: 10, vec: []float32{1, 0, 0, 0}}
	b := testBrainWithEmbedder(t, e, 4)

	id, _ := b.store.AddNode(&store.AddNodeOpts{Type: store.TypeConcept, Content: "this content cannot embed on this provider", Importance: 0.5})
	_ = b.store.SetMeta("has_pending_embeddings", "true")

	res, err := b.EmbedPending(context.Background())
	if err != nil {
		t.Fatalf("EmbedPending: %v", err)
	}
	if _, ok := res.Failed[id]; !ok {
		t.Errorf("failed node not reported: %+v", res)
	}
	if res.Remaining != 1 {
		t.Errorf("remaining: got %d want 1", res.Remaining)
	}
	if v, _ := b.GetMeta("has_pending_embeddings"); v != "true" {
		t.Error("flag must stay set while nodes remain pending")
	}
}
