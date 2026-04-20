package store

import (
	"math"
	"path/filepath"
	"testing"
)

// TestCosineSimilarityIdentical verifies that identical vectors return 1.0.
func TestCosineSimilarityIdentical(t *testing.T) {
	a := []float32{1.0, 0.0, 0.0}
	got := CosineSimilarity(a, a)
	if math.Abs(got-1.0) > 0.001 {
		t.Errorf("CosineSimilarity(identical) = %f, want ~1.0", got)
	}
}

// TestCosineSimilarityOrthogonal verifies that orthogonal vectors return ~0.0.
func TestCosineSimilarityOrthogonal(t *testing.T) {
	a := []float32{1.0, 0.0, 0.0}
	b := []float32{0.0, 1.0, 0.0}
	got := CosineSimilarity(a, b)
	if math.Abs(got) > 0.001 {
		t.Errorf("CosineSimilarity(orthogonal) = %f, want ~0.0", got)
	}
}

// TestCosineSimilarityOpposite verifies that opposite vectors return ~-1.0.
func TestCosineSimilarityOpposite(t *testing.T) {
	a := []float32{1.0, 0.0, 0.0}
	b := []float32{-1.0, 0.0, 0.0}
	got := CosineSimilarity(a, b)
	if math.Abs(got+1.0) > 0.001 {
		t.Errorf("CosineSimilarity(opposite) = %f, want ~-1.0", got)
	}
}

// TestCosineSimilarityKnownValue verifies a known cosine similarity value.
func TestCosineSimilarityKnownValue(t *testing.T) {
	// a = [1, 1, 0], b = [1, 0, 0]
	// dot(a, b) = 1, |a| = sqrt(2), |b| = 1 → cos = 1/sqrt(2) ≈ 0.707
	a := []float32{1.0, 1.0, 0.0}
	b := []float32{1.0, 0.0, 0.0}
	got := CosineSimilarity(a, b)
	expected := 1.0 / math.Sqrt(2)
	if math.Abs(got-expected) > 0.001 {
		t.Errorf("CosineSimilarity([1,1,0],[1,0,0]) = %f, want %f", got, expected)
	}
}

// TestCosineSimilarityZeroVector verifies graceful handling of zero vectors.
func TestCosineSimilarityZeroVector(t *testing.T) {
	zero := []float32{0.0, 0.0, 0.0}
	a := []float32{1.0, 0.0, 0.0}
	got := CosineSimilarity(zero, a)
	// Zero vector: no panic, return 0
	if math.IsNaN(got) || math.IsInf(got, 0) {
		t.Errorf("CosineSimilarity(zero, a) = %f, want finite value", got)
	}
}

// TestGetEmbeddingsReturnsMatchingVectors verifies batch embedding fetch.
func TestGetEmbeddingsReturnsMatchingVectors(t *testing.T) {
	s := testStoreWithVec(t, 4)

	id1, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "node1", Importance: 0.5})
	id2, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "node2", Importance: 0.5})

	vec1 := []float32{1.0, 0.0, 0.0, 0.0}
	vec2 := []float32{0.0, 1.0, 0.0, 0.0}
	if err := s.StoreEmbedding(id1, vec1); err != nil {
		t.Fatalf("StoreEmbedding id1: %v", err)
	}
	if err := s.StoreEmbedding(id2, vec2); err != nil {
		t.Fatalf("StoreEmbedding id2: %v", err)
	}

	embs, err := s.GetEmbeddings([]string{id1, id2})
	if err != nil {
		t.Fatalf("GetEmbeddings: %v", err)
	}

	if len(embs) != 2 {
		t.Errorf("expected 2 embeddings, got %d", len(embs))
	}

	e1, ok1 := embs[id1]
	e2, ok2 := embs[id2]
	if !ok1 {
		t.Error("missing embedding for id1")
	}
	if !ok2 {
		t.Error("missing embedding for id2")
	}

	if ok1 && CosineSimilarity(e1, vec1) < 0.99 {
		t.Errorf("id1 embedding mismatch: similarity=%f", CosineSimilarity(e1, vec1))
	}
	if ok2 && CosineSimilarity(e2, vec2) < 0.99 {
		t.Errorf("id2 embedding mismatch: similarity=%f", CosineSimilarity(e2, vec2))
	}
}

// TestGetEmbeddingsEmptySlice verifies empty input returns empty map.
func TestGetEmbeddingsEmptySlice(t *testing.T) {
	s := testStoreWithVec(t, 4)

	embs, err := s.GetEmbeddings([]string{})
	if err != nil {
		t.Fatalf("GetEmbeddings empty: %v", err)
	}
	if len(embs) != 0 {
		t.Errorf("expected empty map, got %d entries", len(embs))
	}
}

// TestGetEmbeddingsMissingNode verifies that nodes without embeddings are absent from result.
func TestGetEmbeddingsMissingNode(t *testing.T) {
	s := testStoreWithVec(t, 4)

	id1, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "no-embedding", Importance: 0.5})
	id2, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "has-embedding", Importance: 0.5})
	if err := s.StoreEmbedding(id2, []float32{1.0, 0.0, 0.0, 0.0}); err != nil {
		t.Fatalf("StoreEmbedding: %v", err)
	}

	embs, err := s.GetEmbeddings([]string{id1, id2})
	if err != nil {
		t.Fatalf("GetEmbeddings: %v", err)
	}

	if _, ok := embs[id1]; ok {
		t.Error("id1 (no embedding) should not appear in result")
	}
	if _, ok := embs[id2]; !ok {
		t.Error("id2 (has embedding) should appear in result")
	}
}

// TestScoredNodeHasEmbeddingField verifies the Embedding field exists on ScoredNode.
func TestScoredNodeHasEmbeddingField(t *testing.T) {
	sn := ScoredNode{}
	sn.Embedding = []float32{1.0, 0.0}
	if len(sn.Embedding) != 2 {
		t.Errorf("expected Embedding field to hold 2 values, got %d", len(sn.Embedding))
	}
}

// TestGetEmbeddingsStorePath verifies the function works with the real store path setup.
func TestGetEmbeddingsStorePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "emb_test.sqlite")
	s, err := Init(path)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer func() { _ = s.Close() }()

	// No vector table — GetEmbeddings should handle this gracefully.
	embs, err := s.GetEmbeddings([]string{"nonexistent"})
	// Either error or empty map are acceptable (vec_nodes may not exist).
	if err == nil && len(embs) != 0 {
		t.Errorf("expected empty result without vec_nodes, got %d", len(embs))
	}
}
