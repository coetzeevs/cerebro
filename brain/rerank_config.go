package brain

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/coetzeevs/cerebro/internal/rerank"
	"github.com/coetzeevs/cerebro/internal/rerank/command"
	"github.com/coetzeevs/cerebro/internal/store"
)

// Reranking pipeline constants (AC3b). The over-retrieve window is the research-
// cited [30,50] range; the cut ceiling matches the recall --limit semantics.
const (
	// rerankOverRetrieve is the candidate count fetched before reranking when
	// reranking is enabled. ∈ [30,50] per documentation/research roadmap.
	rerankOverRetrieve = 40
	// rerankCutDefault documents the post-rerank ceiling. The actual cut uses
	// the caller's `limit`; this constant records the ≤10 default expectation.
	rerankCutDefault = 10
)

// rerankConfigPrefix mirrors cmd/cerebro's configMetaPrefix. It is duplicated
// here (not imported) because resolveConfig* and configMetaPrefix live in
// package main and importing them would create a brain → cmd/cerebro cycle
// (M4). The brain owns the boolean/empty resolution for the search path; the
// CLI configRegistry remains the validation/documentation/`config list` surface.
const rerankConfigPrefix = "config."

// resolveRerankEnabled reads config.rerank_enabled from the store. The
// gate is strict: only the literal "true" enables reranking; an unset row,
// "false", or any other value resolves to disabled (M4: ""→false default).
// This is intentionally env-free — no CEREBRO_RERANK_ENABLED toggle — so an
// actor who can set env but not brain config cannot enable subprocess spawning
// (S-INFO-3, security review).
func resolveRerankEnabled(s *store.Store) bool {
	val, _ := s.GetMeta(rerankConfigPrefix + "rerank_enabled")
	return val == "true"
}

// resolveRerankCommand resolves the reranker command string with config-wins,
// env-fallback precedence (matching cerebro's voyage.go env-fallback pattern):
// config.rerank_command first, then CEREBRO_RERANK_COMMAND. Returns "" when
// neither is set (the caller then degrades to composite order).
//
// The env var is read here, on the enabled path only — callers invoke this
// solely after resolveRerankEnabled returns true, so a disabled brain never
// touches os.Getenv (S-INFO-2, lazy env read).
func resolveRerankCommand(s *store.Store) string {
	if val, _ := s.GetMeta(rerankConfigPrefix + "rerank_command"); val != "" {
		return val
	}
	return os.Getenv("CEREBRO_RERANK_COMMAND")
}

// newReranker constructs the reranker for the enabled path, or returns nil when
// no command is configured (caller degrades to composite order). It is only
// called when resolveRerankEnabled(s) is true.
//
// It is a package var (not a plain func) solely so tests can inject a
// deterministic in-process reranker without standing up a subprocess; production
// always uses newRerankerFromStore.
var newReranker = newRerankerFromStore

func newRerankerFromStore(s *store.Store) rerank.Reranker {
	cmd := resolveRerankCommand(s)
	if cmd == "" {
		return nil
	}
	return command.New(cmd)
}

// reorderByRerankScore returns a copy of nodes reordered by descending rerank
// score (index-aligned to nodes). Composite ScoredNode.Score is preserved on
// each node — rerank governs ordering only, never the composite score.
// A length mismatch is a no-op (returns nodes unchanged) so a malformed score
// set can never corrupt the ranking (caller degrades).
func reorderByRerankScore(nodes []store.ScoredNode, scores []float64) []store.ScoredNode {
	if len(scores) != len(nodes) {
		return nodes
	}
	type scored struct {
		node  store.ScoredNode
		score float64
	}
	pairs := make([]scored, len(nodes))
	for i := range nodes {
		pairs[i] = scored{node: nodes[i], score: scores[i]}
	}
	sort.SliceStable(pairs, func(i, j int) bool {
		return pairs[i].score > pairs[j].score
	})
	out := make([]store.ScoredNode, len(pairs))
	for i := range pairs {
		out[i] = pairs[i].node
	}
	return out
}

// applyRerank runs the rerank step over an over-retrieved candidate set and
// cuts to limit. On ANY reranker error (command unset/missing/crash, malformed
// or short JSON, non-finite scores, timeout) it logs a one-line stderr warning
// and degrades to the pre-rerank order — recall is never worse than disabled,
// so the AC4-NR floor holds. The cut is applied in both branches.
func applyRerank(ctx context.Context, r rerank.Reranker, query string, nodes []store.ScoredNode, limit int) []store.ScoredNode {
	ordered := nodes

	// A nil reranker means rerank is enabled but no command is configured
	// (unset config + unset CEREBRO_RERANK_COMMAND). Degrade to composite order
	// with a one-line warning — recall is never worse than disabled (AC4-NR).
	if r == nil {
		fmt.Fprintln(os.Stderr, "cerebro: rerank enabled but no reranker command configured; using composite order")
	} else if len(nodes) > 0 {
		cands := make([]rerank.Candidate, len(nodes))
		for i := range nodes {
			cands[i] = rerank.Candidate{ID: nodes[i].ID, Content: nodes[i].Content}
		}
		scores, err := r.Rerank(ctx, query, cands)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cerebro: reranker %q failed (%v); using composite order\n", r.Name(), err)
		} else {
			ordered = reorderByRerankScore(nodes, scores)
		}
	}

	if limit > 0 && len(ordered) > limit {
		ordered = ordered[:limit]
	}
	return ordered
}
