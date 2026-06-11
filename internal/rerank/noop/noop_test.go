package noop

import (
	"context"
	"testing"

	"github.com/coetzeevs/cerebro/internal/rerank"
)

func TestNoopReturnsDescendingScoresPreservingOrder(t *testing.T) {
	r := New()

	cands := []rerank.Candidate{
		{ID: "a", Content: "alpha"},
		{ID: "b", Content: "beta"},
		{ID: "c", Content: "gamma"},
	}

	scores, err := r.Rerank(context.Background(), "q", cands)
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	if len(scores) != len(cands) {
		t.Fatalf("expected %d scores, got %d", len(cands), len(scores))
	}

	// Scores must be strictly descending so that reordering by score keeps the
	// incoming composite order intact (the disabled-path no-op).
	for i := 1; i < len(scores); i++ {
		if scores[i] >= scores[i-1] {
			t.Errorf("scores not strictly descending at %d: %v", i, scores)
		}
	}
}

func TestNoopEmptyCandidates(t *testing.T) {
	r := New()
	scores, err := r.Rerank(context.Background(), "q", nil)
	if err != nil {
		t.Fatalf("Rerank(nil): %v", err)
	}
	if len(scores) != 0 {
		t.Errorf("expected 0 scores for nil candidates, got %d", len(scores))
	}
}

func TestNoopName(t *testing.T) {
	if got := New().Name(); got != "noop" {
		t.Errorf("Name() = %q, want %q", got, "noop")
	}
}
