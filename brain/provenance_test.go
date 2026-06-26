package brain

import (
	"path/filepath"
	"testing"

	"github.com/coetzeevs/cerebro/internal/store"
)

// newTestBrain creates a noop-embedder brain in a temp dir.
func newTestBrain(t *testing.T) *Brain {
	t.Helper()
	path := filepath.Join(t.TempDir(), "brain.sqlite")
	b, err := Init(path, EmbedConfig{Provider: "none"})
	if err != nil {
		t.Fatalf("Init brain: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })
	return b
}

// TestBrainConsolidateAndWalkProvenance asserts the additive brain API:
// Consolidate writes derived_from edges and WalkProvenance walks them outward.
func TestBrainConsolidateAndWalkProvenance(t *testing.T) {
	b := newTestBrain(t)

	concept, err := b.Add("synthesis", store.TypeConcept)
	if err != nil {
		t.Fatalf("Add concept: %v", err)
	}
	e1, _ := b.Add("ep1", store.TypeEpisode)
	e2, _ := b.Add("ep2", store.TypeEpisode)

	if err := b.Consolidate(concept, []string{e1, e2}); err != nil {
		t.Fatalf("Consolidate: %v", err)
	}

	chain, err := b.WalkProvenance(concept, 5)
	if err != nil {
		t.Fatalf("WalkProvenance: %v", err)
	}
	if len(chain) != 3 { // concept@0 + 2 sources@1
		t.Fatalf("expected 3 nodes in provenance chain, got %d", len(chain))
	}
	if chain[0].ID != concept || chain[0].Depth != 0 {
		t.Fatalf("expected concept first at depth 0, got %s@%d", chain[0].ID[:8], chain[0].Depth)
	}
}

// TestBrainProvenanceStatus asserts the batched status API surfaces complete/none.
func TestBrainProvenanceStatus(t *testing.T) {
	b := newTestBrain(t)
	concept, _ := b.Add("synth", store.TypeConcept)
	ep, _ := b.Add("src", store.TypeEpisode)
	none, _ := b.Add("bare", store.TypeConcept)
	if err := b.Consolidate(concept, []string{ep}); err != nil {
		t.Fatalf("Consolidate: %v", err)
	}

	statuses, err := b.ProvenanceStatus([]string{concept, none})
	if err != nil {
		t.Fatalf("ProvenanceStatus: %v", err)
	}
	if statuses[concept] != "complete" {
		t.Fatalf("concept should be complete, got %q", statuses[concept])
	}
	if statuses[none] != "none" {
		t.Fatalf("bare node should be none, got %q", statuses[none])
	}
}

// TestBrainAddWithProvenanceRoot asserts the additive WithProvenanceRoot option
// sets nodes.provenance_root=1, and a flagless Add defaults to 0.
func TestBrainAddWithProvenanceRoot(t *testing.T) {
	b := newTestBrain(t)

	rootID, err := b.Add("root", store.TypeEpisode, WithProvenanceRoot())
	if err != nil {
		t.Fatalf("Add with provenance root: %v", err)
	}
	plainID, err := b.Add("plain", store.TypeEpisode)
	if err != nil {
		t.Fatalf("Add plain: %v", err)
	}

	root, err := b.Get(rootID, nil)
	if err != nil {
		t.Fatalf("Get root: %v", err)
	}
	if !root.ProvenanceRoot {
		t.Error("WithProvenanceRoot() did not set provenance_root=true")
	}
	plain, err := b.Get(plainID, nil)
	if err != nil {
		t.Fatalf("Get plain: %v", err)
	}
	if plain.ProvenanceRoot {
		t.Error("flagless Add should default provenance_root=false")
	}
}
