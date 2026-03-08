package store

import (
	"math"
	"path/filepath"
	"testing"
)

func testStoreWithVec(t *testing.T, dims int) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.sqlite")
	s, err := Init(path)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.InitVectorTable(dims); err != nil {
		t.Fatalf("InitVectorTable: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestInitVectorTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.sqlite")
	s, err := Init(path)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer s.Close()

	if err := s.InitVectorTable(4); err != nil {
		t.Fatalf("InitVectorTable: %v", err)
	}

	// Verify vec_version() is available (proves sqlite-vec is loaded)
	var vecVersion string
	if err := s.db.QueryRow("SELECT vec_version()").Scan(&vecVersion); err != nil {
		t.Fatalf("vec_version(): %v", err)
	}
	if vecVersion == "" {
		t.Error("expected non-empty vec_version")
	}
}

func TestStoreAndSearchEmbedding(t *testing.T) {
	s := testStoreWithVec(t, 4)

	// Add nodes with known content
	id1, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "Go programming language", Importance: 0.8})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	id2, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "Python programming language", Importance: 0.6})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	id3, err := s.AddNode(&AddNodeOpts{Type: TypeEpisode, Content: "Grocery shopping list", Importance: 0.3})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}

	// Store embeddings (fake 4-dim vectors)
	// id1 and id2 are similar (programming), id3 is different (groceries)
	if err := s.StoreEmbedding(id1, []float32{0.9, 0.1, 0.1, 0.0}); err != nil {
		t.Fatalf("StoreEmbedding id1: %v", err)
	}
	if err := s.StoreEmbedding(id2, []float32{0.8, 0.2, 0.1, 0.0}); err != nil {
		t.Fatalf("StoreEmbedding id2: %v", err)
	}
	if err := s.StoreEmbedding(id3, []float32{0.0, 0.0, 0.1, 0.9}); err != nil {
		t.Fatalf("StoreEmbedding id3: %v", err)
	}

	// Search with a query vector similar to programming nodes
	results, err := s.VectorSearch([]float32{0.85, 0.15, 0.1, 0.0}, 10, 0.0)
	if err != nil {
		t.Fatalf("VectorSearch: %v", err)
	}

	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}

	// First result should be the most similar (id1 - Go programming)
	if results[0].ID != id1 {
		t.Errorf("expected first result to be id1 (%s), got %s", id1[:8], results[0].ID[:8])
	}

	// Similarities should be ordered descending
	for i := 1; i < len(results); i++ {
		if results[i].Similarity > results[i-1].Similarity {
			t.Errorf("results not ordered by similarity: [%d]=%f > [%d]=%f",
				i, results[i].Similarity, i-1, results[i-1].Similarity)
		}
	}
}

func TestVectorSearchThreshold(t *testing.T) {
	s := testStoreWithVec(t, 4)

	id1, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "similar", Importance: 0.5})
	id2, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "different", Importance: 0.5})

	if err := s.StoreEmbedding(id1, []float32{1.0, 0.0, 0.0, 0.0}); err != nil {
		t.Fatalf("StoreEmbedding: %v", err)
	}
	if err := s.StoreEmbedding(id2, []float32{0.0, 0.0, 0.0, 1.0}); err != nil {
		t.Fatalf("StoreEmbedding: %v", err)
	}

	// High threshold should only return the very similar node
	results, err := s.VectorSearch([]float32{1.0, 0.0, 0.0, 0.0}, 10, 0.9)
	if err != nil {
		t.Fatalf("VectorSearch: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 result above threshold 0.9, got %d", len(results))
	}
	if len(results) > 0 && results[0].ID != id1 {
		t.Errorf("expected result to be id1, got %s", results[0].ID[:8])
	}
}

func TestVectorSearchLimit(t *testing.T) {
	s := testStoreWithVec(t, 4)

	for i := 0; i < 5; i++ {
		id, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "node", Importance: 0.5})
		vec := []float32{float32(i) * 0.1, 0.1, 0.1, 0.1}
		if err := s.StoreEmbedding(id, vec); err != nil {
			t.Fatalf("StoreEmbedding: %v", err)
		}
	}

	results, err := s.VectorSearch([]float32{0.2, 0.1, 0.1, 0.1}, 2, 0.0)
	if err != nil {
		t.Fatalf("VectorSearch: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results with limit=2, got %d", len(results))
	}
}

func TestVectorSearchSkipsNonActiveNodes(t *testing.T) {
	s := testStoreWithVec(t, 4)

	id1, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "active node", Importance: 0.5})
	id2, _ := s.AddNode(&AddNodeOpts{Type: TypeEpisode, Content: "consolidated node", Importance: 0.5})

	vec := []float32{0.5, 0.5, 0.0, 0.0}
	if err := s.StoreEmbedding(id1, vec); err != nil {
		t.Fatalf("StoreEmbedding: %v", err)
	}
	if err := s.StoreEmbedding(id2, vec); err != nil {
		t.Fatalf("StoreEmbedding: %v", err)
	}

	// Mark id2 as consolidated
	if err := s.MarkConsolidated([]string{id2}); err != nil {
		t.Fatalf("MarkConsolidated: %v", err)
	}

	results, err := s.VectorSearch(vec, 10, 0.0)
	if err != nil {
		t.Fatalf("VectorSearch: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 active result, got %d", len(results))
	}
	if len(results) > 0 && results[0].ID != id1 {
		t.Errorf("expected active node id1, got %s", results[0].ID[:8])
	}
}

func TestStoreEmbeddingUpsert(t *testing.T) {
	s := testStoreWithVec(t, 4)

	id, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "test", Importance: 0.5})

	// Store initial embedding
	if err := s.StoreEmbedding(id, []float32{1.0, 0.0, 0.0, 0.0}); err != nil {
		t.Fatalf("StoreEmbedding initial: %v", err)
	}

	// Overwrite with new embedding
	if err := s.StoreEmbedding(id, []float32{0.0, 1.0, 0.0, 0.0}); err != nil {
		t.Fatalf("StoreEmbedding overwrite: %v", err)
	}

	// Search should find the updated embedding
	results, err := s.VectorSearch([]float32{0.0, 1.0, 0.0, 0.0}, 1, 0.0)
	if err != nil {
		t.Fatalf("VectorSearch: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != id {
		t.Errorf("expected result to be our node, got %s", results[0].ID[:8])
	}
	// Similarity to the updated vector should be very high
	if results[0].Similarity < 0.9 {
		t.Errorf("expected high similarity to updated embedding, got %f", results[0].Similarity)
	}
}

func TestVectorSearchCosineDistance(t *testing.T) {
	s := testStoreWithVec(t, 4)

	// Add two nodes: one similar to query, one orthogonal
	id1, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "similar", Importance: 0.5})
	id2, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "orthogonal", Importance: 0.5})

	// Normalized vectors (unit length) for clean cosine similarity
	if err := s.StoreEmbedding(id1, []float32{1.0, 0.0, 0.0, 0.0}); err != nil {
		t.Fatalf("StoreEmbedding: %v", err)
	}
	if err := s.StoreEmbedding(id2, []float32{0.0, 1.0, 0.0, 0.0}); err != nil {
		t.Fatalf("StoreEmbedding: %v", err)
	}

	// Search for exact match — similarity should be ~1.0
	results, err := s.VectorSearch([]float32{1.0, 0.0, 0.0, 0.0}, 10, 0.0)
	if err != nil {
		t.Fatalf("VectorSearch: %v", err)
	}

	if len(results) < 1 {
		t.Fatal("expected at least 1 result")
	}

	// Exact match should have similarity very close to 1.0
	if results[0].Similarity < 0.95 {
		t.Errorf("expected similarity ~1.0 for exact match, got %f", results[0].Similarity)
	}

	// Orthogonal vector should have similarity ~0.0
	if len(results) >= 2 && results[1].Similarity > 0.1 {
		t.Errorf("expected similarity ~0.0 for orthogonal vector, got %f", results[1].Similarity)
	}

	// All similarities should be in [0, 1] range (cosine property)
	for i, r := range results {
		if r.Similarity < -0.01 || r.Similarity > 1.01 {
			t.Errorf("result[%d] similarity %f outside [0, 1] range", i, r.Similarity)
		}
	}

	// Threshold should correctly filter: 0.5 should exclude orthogonal
	filtered, err := s.VectorSearch([]float32{1.0, 0.0, 0.0, 0.0}, 10, 0.5)
	if err != nil {
		t.Fatalf("VectorSearch filtered: %v", err)
	}
	if len(filtered) != 1 {
		t.Errorf("expected 1 result above 0.5 threshold, got %d", len(filtered))
	}
}

func TestCompositeScore(t *testing.T) {
	now := func() *Node {
		return &Node{
			Importance:  0.8,
			AccessCount: 0,
			DecayRate:   0.02,
		}
	}

	// Basic score with fresh node
	n := now()
	score := compositeScore(n, 0.9)
	if score <= 0 {
		t.Errorf("expected positive score, got %f", score)
	}

	// Higher similarity should give higher score
	scoreLow := compositeScore(n, 0.3)
	scoreHigh := compositeScore(n, 0.9)
	if scoreHigh <= scoreLow {
		t.Errorf("higher similarity should give higher score: %.4f <= %.4f", scoreHigh, scoreLow)
	}

	// More access should increase score (via importance reinforcement)
	n2 := now()
	n2.AccessCount = 10
	scoreReinforced := compositeScore(n2, 0.9)
	scoreBase := compositeScore(now(), 0.9)
	if scoreReinforced <= scoreBase {
		t.Errorf("reinforced node should score higher: %.4f <= %.4f", scoreReinforced, scoreBase)
	}

	// Score should be bounded reasonable (not NaN, not Inf)
	if math.IsNaN(score) || math.IsInf(score, 0) {
		t.Errorf("score should be finite, got %f", score)
	}
}
