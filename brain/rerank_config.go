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

// Combine-mode names for the rerank_fusion config key (agentic-2ixw recall@10
// investigation). The default is RRF — pure-reorder is retained for parity/A-B
// comparison and for operators who want the cross-encoder ranking verbatim.
const (
	// fusionModeRRF fuses the composite ranking and the reranker ranking via
	// Reciprocal Rank Fusion. This is the default: it preserves a composite-top
	// item that the cross-encoder demotes, recovering the recall@10 dip the
	// pure-reorder combine exhibited (it discards the composite order entirely).
	fusionModeRRF = "rrf"
	// fusionModeReorder is the legacy pure-reorder combine: sort by the reranker
	// score alone, discarding the composite order. Retained behind config so the
	// before/after tradeoff stays measurable.
	fusionModeReorder = "reorder"
)

// defaultRRFK is the Reciprocal Rank Fusion rank constant. 60 is the standard
// value introduced by Cormack, Clarke & Buettcher (SIGIR 2009) and the default
// `rank_constant` in production engines (e.g. Elasticsearch's rrf retriever,
// docs: reference/elasticsearch/rest-apis/reciprocal-rank-fusion.md — default
// 60, must be ≥1). A higher k flattens rank influence (lower-ranked docs matter
// more); 60 is the parameter-free, no-tuning default.
const defaultRRFK = 60

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

// resolveRerankFusion reads config.rerank_fusion from the store and returns the
// combine mode for the enabled path. The default — and the fallback for any
// unrecognised value — is RRF (fusionModeRRF); only the literal "reorder"
// selects the legacy pure-reorder combine. Env-free by design (mirrors
// resolveRerankEnabled): the combine mode is a brain-config decision, not an
// env toggle.
func resolveRerankFusion(s *store.Store) string {
	val, _ := s.GetMeta(rerankConfigPrefix + "rerank_fusion")
	if val == fusionModeReorder {
		return fusionModeReorder
	}
	return fusionModeRRF
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

// fuseRRF combines the composite ranking (the input `nodes` order, after
// VectorSearch+ExpandGraph) with the reranker ranking (derived from `scores`)
// via Reciprocal Rank Fusion, and returns nodes sorted by descending fused
// score. Composite ScoredNode.Score is preserved on each node — fusion governs
// ordering only, never the composite score.
//
// For each node the fused score is the sum of two reciprocal-rank terms:
//
//	fused = 1/(k + rank_composite) + 1/(k + rank_reranker)
//
// with ranks starting at 1 (Cormack et al., SIGIR 2009; Elasticsearch rrf
// retriever default rank_constant=60). Because both rankings cover the SAME
// candidate set, every node contributes from both terms — so a composite-top
// item the reranker buries keeps the strong 1/(k+1) composite term and survives
// the cut, which is exactly the recall@10 recovery pure-reorder lacks.
//
// A length mismatch is a no-op (returns nodes unchanged) so a malformed score
// set can never corrupt the ranking (caller degrades). A non-positive k is
// clamped to defaultRRFK to keep the denominator ≥1 (the RRF definition).
func fuseRRF(nodes []store.ScoredNode, scores []float64, k int) []store.ScoredNode {
	if len(scores) != len(nodes) {
		return nodes
	}
	if k < 1 {
		k = defaultRRFK
	}

	// Composite rank = input index (already composite-ordered), 1-based.
	// Reranker rank = position in descending-score order, 1-based.
	rerankRank := make([]int, len(nodes))
	order := make([]int, len(nodes))
	for i := range order {
		order[i] = i
	}
	// Stable sort indices by descending reranker score; ties keep composite order
	// (lower original index first) so the fusion is deterministic.
	sort.SliceStable(order, func(i, j int) bool {
		return scores[order[i]] > scores[order[j]]
	})
	for pos, idx := range order {
		rerankRank[idx] = pos + 1
	}

	type fused struct {
		node  store.ScoredNode
		score float64
		comp  int // composite rank, for deterministic tie-breaking
	}
	pairs := make([]fused, len(nodes))
	for i := range nodes {
		compRank := i + 1
		rrf := 1.0/float64(k+compRank) + 1.0/float64(k+rerankRank[i])
		pairs[i] = fused{node: nodes[i], score: rrf, comp: compRank}
	}
	// Sort by descending fused score; ties broken by ascending composite rank so
	// the composite-order winner stays ahead (deterministic).
	sort.SliceStable(pairs, func(i, j int) bool {
		if pairs[i].score != pairs[j].score {
			return pairs[i].score > pairs[j].score
		}
		return pairs[i].comp < pairs[j].comp
	})

	out := make([]store.ScoredNode, len(pairs))
	for i := range pairs {
		out[i] = pairs[i].node
	}
	return out
}

// applyRerank is the RRF-default convenience seam retained for callers/tests
// that do not select a combine mode explicitly. It delegates to
// applyRerankWithFusion with the RRF default (the recall@10-recovering combine).
func applyRerank(ctx context.Context, r rerank.Reranker, query string, nodes []store.ScoredNode, limit int) []store.ScoredNode {
	return applyRerankWithFusion(ctx, r, query, nodes, limit, fusionModeRRF)
}

// combineRerankScores combines the composite-ordered nodes with the reranker
// scores per the selected fusion mode. fusionModeRRF (default) fuses both
// rankings via Reciprocal Rank Fusion; fusionModeReorder is the legacy
// pure-reorder (sort by reranker score, discard composite order).
func combineRerankScores(nodes []store.ScoredNode, scores []float64, mode string) []store.ScoredNode {
	if mode == fusionModeReorder {
		return reorderByRerankScore(nodes, scores)
	}
	return fuseRRF(nodes, scores, defaultRRFK)
}

// applyRerankWithFusion runs the rerank step over an over-retrieved candidate
// set, combines the reranker scores with the composite order per `mode`, and
// cuts to limit. On ANY reranker error (command unset/missing/crash, malformed
// or short JSON, non-finite scores, timeout) it logs a one-line stderr warning
// and degrades to the pre-rerank composite order — recall is never worse than
// disabled, so the AC4-NR floor holds. The cut is applied in both branches.
func applyRerankWithFusion(ctx context.Context, r rerank.Reranker, query string, nodes []store.ScoredNode, limit int, mode string) []store.ScoredNode {
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
			ordered = combineRerankScores(nodes, scores, mode)
		}
	}

	if limit > 0 && len(ordered) > limit {
		ordered = ordered[:limit]
	}
	return ordered
}
