package brain

import (
	"context"
	"fmt"
	"testing"
)

// failingEmbedder simulates a configured-but-unavailable provider (e.g. Ollama
// down): Dimensions() is real, every Embed call errors.
type failingEmbedder struct{}

func (failingEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return nil, fmt.Errorf("connection refused (simulated provider outage)")
}
func (failingEmbedder) Dimensions() int { return 768 }
func (failingEmbedder) Model() string   { return "nomic-embed-text" }

// TestSearchKeywordFallbackOnEmbedderFailure guards the N3 availability fix:
// when the embedder errors at query time, Search must degrade to keyword-only
// recall instead of hard-failing — exact-identifier queries keep working with
// the provider down.
func TestSearchKeywordFallbackOnEmbedderFailure(t *testing.T) {
	b := testBrain(t)
	if _, err := b.Add("HS-777 sentinel fix for the widget pipeline", Episode); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := b.Add("unrelated memory about databases", Concept); err != nil {
		t.Fatalf("Add: %v", err)
	}

	b.embedder = failingEmbedder{}
	if !b.store.FTSAvailable() {
		t.Skip("FTS5 not built in (no fts5 tag) — fallback lane unavailable by design; the no-FTS branch returns the embed error")
	}

	results, err := b.Search(context.Background(), "HS-777", 10, 0.3, nil, nil)
	if err != nil {
		t.Fatalf("Search should degrade to keyword-only on embedder failure, got error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("keyword fallback returned no results for an exact-identifier query")
	}
	found := false
	for _, r := range results {
		if r.Content == "HS-777 sentinel fix for the widget pipeline" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected the HS-777 memory in fallback results, got %d other results", len(results))
	}
}

// TestSearchKeywordFallbackRespectsSubtypeFilter: the post-search subtype
// contract must hold on the fallback path too.
func TestSearchKeywordFallbackRespectsSubtypeFilter(t *testing.T) {
	b := testBrain(t)
	if _, err := b.Add("HS-888 alpha memory", Episode, WithSubtype("incident")); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := b.Add("HS-888 beta memory", Episode, WithSubtype("discovery")); err != nil {
		t.Fatalf("Add: %v", err)
	}
	b.embedder = failingEmbedder{}
	if !b.store.FTSAvailable() {
		t.Skip("FTS5 not built in (no fts5 tag) — fallback lane unavailable by design; the no-FTS branch returns the embed error")
	}

	want := "incident"
	results, err := b.Search(context.Background(), "HS-888", 10, 0.3, &want, nil)
	if err != nil {
		t.Fatalf("Search fallback: %v", err)
	}
	for _, r := range results {
		if r.Subtype != "incident" {
			t.Errorf("subtype filter violated on fallback path: got %q", r.Subtype)
		}
	}
	if len(results) != 1 {
		t.Errorf("expected exactly 1 incident-subtype result, got %d", len(results))
	}
}

// TestSearchNoneProviderStillErrors: the 'none' provider is configured-out of
// query recall — N3 changes embedder-FAILURE behaviour only, never this.
func TestSearchNoneProviderStillErrors(t *testing.T) {
	b := testBrain(t) // provider: none => Dimensions()==0
	if _, err := b.Search(context.Background(), "anything", 5, 0.3, nil, nil); err == nil {
		t.Fatal("Search with the 'none' provider must still error (configured-out, not a failure)")
	}
}

// TestSearchWithGlobalKeywordFallback: the global variant degrades the same
// way (project-store keyword lane only; the global store's keyword lane is out
// of scope, matching the fusion contract).
func TestSearchWithGlobalKeywordFallback(t *testing.T) {
	b := testBrain(t)
	g := testBrain(t)
	if _, err := b.Add("HS-999 project-side fact", Concept); err != nil {
		t.Fatalf("Add: %v", err)
	}
	b.embedder = failingEmbedder{}
	if !b.store.FTSAvailable() {
		t.Skip("FTS5 not built in (no fts5 tag) — fallback lane unavailable by design; the no-FTS branch returns the embed error")
	}

	results, err := b.SearchWithGlobal(context.Background(), "HS-999", 10, 0.3, g, nil, nil)
	if err != nil {
		t.Fatalf("SearchWithGlobal should degrade to keyword-only, got: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("global-variant keyword fallback returned no results")
	}
}
