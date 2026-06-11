// Package rerank defines the cross-encoder reranker interface and
// implementations used to reorder recall candidates (agentic-2ixw).
//
// The reranker reorders an already-retrieved, already-composite-scored set of
// candidates; it never recomputes the composite score (search.go:compositeScore
// weights are out of scope). Implementations MUST be local — no cloud or
// external-API reranker client — per Cerebro's Model B (no runtime cloud LLM
// calls). The shipped implementations are:
//
//   - noop.Reranker: identity ordering, used on the disabled (default) path.
//   - command.Reranker: a local operator-supplied subprocess invoked over a
//     JSON-on-stdin / JSON-on-stdout contract (argv-array exec, never a shell).
package rerank

import "context"

// Candidate is one already-retrieved document to be scored against the query.
// ID is opaque and echoed back so the caller can re-associate scores with nodes.
type Candidate struct {
	ID      string // node ID (opaque; echoed back for re-association)
	Content string // node content scored against the query
}

// Reranker scores query-document pairs with a cross-encoder.
// Implementations MUST be local (no cloud/external-API client) per Model B.
type Reranker interface {
	// Rerank returns a relevance score per input candidate, index-aligned to
	// cands. Higher means more relevant. It MUST return exactly len(cands)
	// scores or a non-nil error. On any error the caller degrades to the
	// pre-rerank composite order (recall is never worse than disabled).
	Rerank(ctx context.Context, query string, cands []Candidate) ([]float64, error)

	// Name returns the reranker identifier (e.g. "command:ms-marco-MiniLM-L6-v2"),
	// used for logging and eval provenance.
	Name() string
}
