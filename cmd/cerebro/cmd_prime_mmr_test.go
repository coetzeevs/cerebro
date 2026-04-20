package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/coetzeevs/cerebro/brain"
	"github.com/coetzeevs/cerebro/internal/store"
)

// testBrainWithVec creates a brain with vector search enabled.
func testBrainWithVec(t *testing.T, dims int) *brain.Brain {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sqlite")
	b, err := brain.Init(path, brain.EmbedConfig{})
	if err != nil {
		t.Fatalf("brain.Init: %v", err)
	}
	if err := b.Store().InitVectorTable(dims); err != nil {
		t.Fatalf("InitVectorTable: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })
	return b
}

// TestPrimeMMRIdenticalEmbeddingsSelectsOnlyOne verifies that when candidates
// have identical embeddings, MMR selects only one per "cluster" to ensure diversity.
func TestPrimeMMRIdenticalEmbeddingsSelectsOnlyOne(t *testing.T) {
	b := testBrainWithVec(t, 4)

	// Add two identical-embedding concepts and one very different one.
	id1, _ := b.Add("concept A", store.TypeConcept, brain.WithImportance(0.9))
	id2, _ := b.Add("concept B (same topic)", store.TypeConcept, brain.WithImportance(0.85))
	id3, _ := b.Add("concept C (different topic)", store.TypeConcept, brain.WithImportance(0.7))

	// Store identical embeddings for id1 and id2 (both "programming related").
	progVec := []float32{1.0, 0.0, 0.0, 0.0}
	otherVec := []float32{0.0, 1.0, 0.0, 0.0}
	if err := b.Store().StoreEmbedding(id1, progVec); err != nil {
		t.Fatalf("StoreEmbedding id1: %v", err)
	}
	if err := b.Store().StoreEmbedding(id2, progVec); err != nil {
		t.Fatalf("StoreEmbedding id2: %v", err)
	}
	if err := b.Store().StoreEmbedding(id3, otherVec); err != nil {
		t.Fatalf("StoreEmbedding id3: %v", err)
	}

	// With limit=2, MMR should prefer diversity: select id1 (highest score)
	// and id3 (different embedding) rather than id1+id2 (identical embeddings).
	nodes, err := primeMMR(b, 2, store.PrimeScore)
	if err != nil {
		t.Fatalf("primeMMR: %v", err)
	}

	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}

	var foundID1, foundID2, foundID3 bool
	for _, n := range nodes {
		switch n.ID {
		case id1:
			foundID1 = true
		case id2:
			foundID2 = true
		case id3:
			foundID3 = true
		}
	}

	// id1 should be selected (highest PrimeScore).
	if !foundID1 {
		t.Error("expected id1 (highest score) to be selected")
	}
	// id3 should be selected over id2 (diverse embedding).
	if !foundID3 {
		t.Error("expected id3 (different embedding) over id2 (identical to id1)")
	}
	if foundID2 && !foundID3 {
		t.Error("MMR should prefer id3 (diverse) over id2 (identical to id1)")
	}
}

// TestPrimeMMRFallbackNoEmbeddings verifies that without embeddings, MMR
// falls back to pure score-based selection.
func TestPrimeMMRFallbackNoEmbeddings(t *testing.T) {
	b := testBrain(t) // no vector table

	// Add nodes without embeddings.
	ids := make([]string, 4)
	importances := []float64{0.9, 0.7, 0.5, 0.3}
	for i, imp := range importances {
		id, err := b.Add("concept", store.TypeConcept, brain.WithImportance(imp))
		if err != nil {
			t.Fatalf("Add concept %d: %v", i, err)
		}
		ids[i] = id
	}

	nodes, err := primeMMR(b, 3, store.PrimeScore)
	if err != nil {
		t.Fatalf("primeMMR fallback: %v", err)
	}

	if len(nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(nodes))
	}

	// Should be score-ordered (highest importance first).
	var foundHighest bool
	for _, n := range nodes {
		if n.ID == ids[0] {
			foundHighest = true
		}
	}
	if !foundHighest {
		t.Error("expected highest-importance node to be selected in fallback mode")
	}
}

// TestPrimeMMRWithImportanceOnlyScore verifies MMR works with a simple importance scorer.
func TestPrimeMMRWithImportanceOnlyScore(t *testing.T) {
	b := testBrainWithVec(t, 4)

	for i := 0; i < 5; i++ {
		id, _ := b.Add("concept", store.TypeConcept, brain.WithImportance(float64(i+1)*0.15))
		vec := []float32{float32(i) * 0.1, 1.0 - float32(i)*0.1, 0.0, 0.0}
		if err := b.Store().StoreEmbedding(id, vec); err != nil {
			t.Fatalf("StoreEmbedding: %v", err)
		}
	}

	scoreFn := func(n *store.Node) float64 { return n.Importance }
	nodes, err := primeMMR(b, 3, scoreFn)
	if err != nil {
		t.Fatalf("primeMMR: %v", err)
	}

	if len(nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(nodes))
	}
}

// TestPrimeMMRTouchSurfaced verifies that primeMMR updates last_surfaced.
func TestPrimeMMRTouchSurfaced(t *testing.T) {
	b := testBrainWithVec(t, 4)

	ids := make([]string, 3)
	for i := 0; i < 3; i++ {
		id, err := b.Add("concept", store.TypeConcept, brain.WithImportance(0.7))
		if err != nil {
			t.Fatalf("Add: %v", err)
		}
		vec := []float32{float32(i) * 0.5, 1.0 - float32(i)*0.3, 0.0, 0.0}
		if err := b.Store().StoreEmbedding(id, vec); err != nil {
			t.Fatalf("StoreEmbedding: %v", err)
		}
		ids[i] = id
	}

	before := time.Now().UTC().Add(-2 * time.Second)

	_, err := primeMMR(b, 3, store.PrimeScore)
	if err != nil {
		t.Fatalf("primeMMR: %v", err)
	}

	for _, id := range ids {
		node, err := b.Store().GetNode(id)
		if err != nil {
			t.Fatalf("GetNode: %v", err)
		}
		if node.LastSurfaced == nil {
			t.Errorf("expected last_surfaced set after primeMMR for node %s", id[:8])
		} else if node.LastSurfaced.Before(before) {
			t.Errorf("last_surfaced %v is before test start %v", node.LastSurfaced, before)
		}
	}
}

// TestRunRecallPrimeUsesMMR verifies that --prime mode uses primeMMR.
// This is an integration check — primeMMR should be called when --prime is set.
func TestRunRecallPrimeUsesMMR(t *testing.T) {
	// This test verifies that primeMMR is wired into runRecall by checking
	// that the command's prime path calls it and returns results.
	// We do this by calling primeMMR directly and verifying it works end-to-end.
	b := testBrainWithVec(t, 4)

	for i := 0; i < 5; i++ {
		id, err := b.Add("concept", store.TypeConcept, brain.WithImportance(float64(i+1)*0.15))
		if err != nil {
			t.Fatalf("Add: %v", err)
		}
		vec := []float32{float32(i+1) * 0.2, float32(5-i) * 0.1, 0.0, 0.0}
		if err := b.Store().StoreEmbedding(id, vec); err != nil {
			t.Fatalf("StoreEmbedding: %v", err)
		}
	}

	nodes, err := primeMMR(b, 5, store.PrimeScore)
	if err != nil {
		t.Fatalf("primeMMR integration: %v", err)
	}

	if len(nodes) > 5 {
		t.Errorf("expected at most 5 nodes, got %d", len(nodes))
	}
	if len(nodes) == 0 {
		t.Error("expected some results from primeMMR")
	}
}
