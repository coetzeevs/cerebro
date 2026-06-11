// Package noop implements the disabled-path reranker: it returns strictly
// descending scores that preserve the incoming candidate order, so reordering
// by score is a no-op. This is the implementation used when reranking is
// disabled (the default), mirroring internal/embed/noop.
package noop

import (
	"context"

	"github.com/coetzeevs/cerebro/internal/rerank"
)

// Reranker returns input order unchanged via descending sentinel scores.
type Reranker struct{}

// New creates a no-op reranker.
func New() *Reranker { return &Reranker{} }

// Rerank returns strictly descending scores (len(cands)-1, len(cands)-2, ...),
// which preserves the incoming order when the caller sorts by score descending.
func (r *Reranker) Rerank(_ context.Context, _ string, cands []rerank.Candidate) ([]float64, error) {
	scores := make([]float64, len(cands))
	for i := range cands {
		scores[i] = float64(len(cands) - i)
	}
	return scores, nil
}

// Name returns the reranker identifier.
func (r *Reranker) Name() string { return "noop" }
