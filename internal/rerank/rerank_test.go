package rerank

import (
	"context"
	"testing"
)

// staticReranker is a trivial Reranker used to prove the interface is
// satisfiable and that Candidate/score index-alignment is the contract.
type staticReranker struct {
	scores []float64
	name   string
}

func (s staticReranker) Rerank(_ context.Context, _ string, cands []Candidate) ([]float64, error) {
	out := make([]float64, len(cands))
	copy(out, s.scores)
	return out, nil
}

func (s staticReranker) Name() string { return s.name }

func TestRerankerInterfaceSatisfiable(t *testing.T) {
	var r Reranker = staticReranker{scores: []float64{0.1, 0.2}, name: "static"}

	cands := []Candidate{
		{ID: "a", Content: "alpha"},
		{ID: "b", Content: "beta"},
	}
	scores, err := r.Rerank(context.Background(), "q", cands)
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	if len(scores) != len(cands) {
		t.Fatalf("expected %d scores, got %d", len(cands), len(scores))
	}
	if r.Name() != "static" {
		t.Errorf("Name() = %q, want %q", r.Name(), "static")
	}
}

func TestCandidateFields(t *testing.T) {
	c := Candidate{ID: "node-1", Content: "some content"}
	if c.ID != "node-1" || c.Content != "some content" {
		t.Errorf("Candidate fields not preserved: %+v", c)
	}
}
