package store

// outcome_scoring_test.go — agentic-do71 (TDD). Today only ACCESS reinforces
// a memory. Two richer signals: (1) agent-supplied outcome counters
// (success/failure) recorded in metadata multiply the composite score —
// proven-useful memories surface more, misleading ones sink; (2) in-degree
// (how many edges point AT a node) finally gives the structural component a
// baseline for plain search results (it was always 0 outside graph
// expansion). Both Model B: the agent supplies signals, cerebro scores.

import (
	"encoding/json"
	"testing"
)

func TestOutcomeFactor_Shape(t *testing.T) {
	cases := []struct {
		meta string
		min  float64
		max  float64
	}{
		{"", 1.0, 1.0},                                       // no metadata: neutral
		{`{"beadsId":"x"}`, 1.0, 1.0},                        // no outcomes key: neutral
		{`{"outcomes":{"success":3}}`, 1.01, 1.5},            // successes boost
		{`{"outcomes":{"failure":3}}`, 0.2, 0.99},            // failures penalize
		{`{"outcomes":{"success":2,"failure":2}}`, 0.5, 1.0}, // failure outweighs
		{`{"outcomes":{"failure":100}}`, 0.2, 0.35},          // floored, never zero/negative
	}
	for _, c := range cases {
		var meta json.RawMessage
		if c.meta != "" {
			meta = json.RawMessage(c.meta)
		}
		f := outcomeFactor(meta)
		if f < c.min || f > c.max {
			t.Errorf("outcomeFactor(%s) = %f, want [%f, %f]", c.meta, f, c.min, c.max)
		}
	}
}

func TestRecordOutcome_CountsInMetadata(t *testing.T) {
	s := testStore(t)
	id, err := s.AddNode(&AddNodeOpts{Type: TypeProcedure, Content: "a workflow", Importance: 0.5})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := s.RecordOutcome(id, true); err != nil {
			t.Fatalf("RecordOutcome success: %v", err)
		}
	}
	if err := s.RecordOutcome(id, false); err != nil {
		t.Fatalf("RecordOutcome failure: %v", err)
	}
	n, err := s.GetNode(id)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	var meta struct {
		Outcomes struct {
			Success int `json:"success"`
			Failure int `json:"failure"`
		} `json:"outcomes"`
	}
	if err := json.Unmarshal(n.Metadata, &meta); err != nil {
		t.Fatalf("metadata: %v (%s)", err, n.Metadata)
	}
	if meta.Outcomes.Success != 2 || meta.Outcomes.Failure != 1 {
		t.Errorf("outcomes: got %+v want success=2 failure=1", meta.Outcomes)
	}
}

// RecordOutcome preserves existing metadata keys (the beadsId/anchor merge
// discipline).
func TestRecordOutcome_PreservesMetadata(t *testing.T) {
	s := testStore(t)
	id, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "x", Importance: 0.5, Metadata: json.RawMessage(`{"beadsId":"agentic-abc"}`)})
	if err := s.RecordOutcome(id, true); err != nil {
		t.Fatalf("RecordOutcome: %v", err)
	}
	n, _ := s.GetNode(id)
	var meta map[string]any
	_ = json.Unmarshal(n.Metadata, &meta)
	if meta["beadsId"] != "agentic-abc" {
		t.Errorf("existing metadata clobbered: %s", n.Metadata)
	}
}

// In-degree feeds the structural component for plain vector-search results:
// with identical vectors and importance, the cited node outranks the
// uncited one.
func TestVectorSearch_InDegreeStructuralBaseline(t *testing.T) {
	s := testStoreWithVec(t, 4)
	cited, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "cited", Importance: 0.5})
	uncited, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "uncited", Importance: 0.5})
	for _, id := range []string{cited, uncited} {
		if err := s.StoreEmbedding(id, []float32{1, 0, 0, 0}); err != nil {
			t.Fatalf("StoreEmbedding: %v", err)
		}
	}
	// Three citations INTO the cited node.
	for i := 0; i < 3; i++ {
		src, _ := s.AddNode(&AddNodeOpts{Type: TypeEpisode, Content: "src", Importance: 0.1})
		if _, err := s.AddEdge(src, cited, "derived_from", AddEdgeOpts{}); err != nil {
			t.Fatalf("AddEdge: %v", err)
		}
	}

	results, err := s.VectorSearch([]float32{1, 0, 0, 0}, 5, 0)
	if err != nil {
		t.Fatalf("VectorSearch: %v", err)
	}
	var citedScore, uncitedScore float64
	for _, r := range results {
		switch r.ID {
		case cited:
			citedScore = r.Score
		case uncited:
			uncitedScore = r.Score
		}
	}
	if citedScore <= uncitedScore {
		t.Errorf("in-degree bonus missing: cited=%f uncited=%f", citedScore, uncitedScore)
	}
}

// The bonus sits behind a config seam (t3c9 A/B protocol): disabled, the
// scores equalize again.
func TestVectorSearch_InDegreeSeamDisables(t *testing.T) {
	s := testStoreWithVec(t, 4)
	if err := s.SetMeta("config.indegree_bonus_enabled", "false"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}
	cited, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "cited", Importance: 0.5})
	uncited, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "uncited", Importance: 0.5})
	for _, id := range []string{cited, uncited} {
		_ = s.StoreEmbedding(id, []float32{1, 0, 0, 0})
	}
	src, _ := s.AddNode(&AddNodeOpts{Type: TypeEpisode, Content: "src", Importance: 0.1})
	_, _ = s.AddEdge(src, cited, "derived_from", AddEdgeOpts{})

	results, err := s.VectorSearch([]float32{1, 0, 0, 0}, 5, 0)
	if err != nil {
		t.Fatalf("VectorSearch: %v", err)
	}
	var citedScore, uncitedScore float64
	for _, r := range results {
		switch r.ID {
		case cited:
			citedScore = r.Score
		case uncited:
			uncitedScore = r.Score
		}
	}
	// Equal up to sub-second recency drift between the two AddNode calls —
	// the structural bonus (>=0.01 at one citation) is orders larger.
	if diff := citedScore - uncitedScore; diff > 1e-6 || diff < -1e-6 {
		t.Errorf("seam off must equalize: cited=%f uncited=%f (diff %g)", citedScore, uncitedScore, diff)
	}
}
