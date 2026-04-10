package main

import (
	"sort"
	"time"

	"github.com/coetzeevs/cerebro/brain"
	"github.com/coetzeevs/cerebro/internal/store"
)

// primeMMR returns a diverse, high-scoring selection of memories for session priming
// using Maximal Marginal Relevance (Carbonell & Goldstein 1998).
//
// Algorithm:
//  1. Fetch candidate pools per type stratum (3x budget per type).
//  2. Pool all candidates, deduplicate.
//  3. Load embeddings for all candidates.
//  4. Greedily select up to `limit` items using MMR:
//     MMR(i) = lambda * scoreFn(i) - (1-lambda) * max_similarity(i, selected)
//  5. Fall back to pure score order if embeddings are unavailable.
//  6. Call TouchSurfaced for all selected nodes.
//
// The scoreFn parameter allows callers to inject different scoring strategies:
// use store.PrimeScore for the full surprise-aware score, or a simpler function
// for testing or fallback.
func primeMMR(b *brain.Brain, limit int, scoreFn func(*store.Node) float64) ([]store.Node, error) {
	const lambda = 0.6 // 60% individual score, 40% diversity

	// Fetch candidate pools.
	candidates, err := fetchMMRCandidates(b, limit)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	// Load embeddings for all candidates.
	ids := make([]string, len(candidates))
	for i := range candidates {
		ids[i] = candidates[i].ID
	}
	embeddings, _ := b.Store().GetEmbeddings(ids) // ignore error — graceful fallback

	embeddingsAvailable := len(embeddings) > 0

	// Build internal scored candidates.
	type scoredCandidate struct {
		node  store.Node
		score float64
		emb   []float32
	}
	scored := make([]scoredCandidate, len(candidates))
	for i := range candidates {
		scored[i] = scoredCandidate{
			node:  candidates[i],
			score: scoreFn(&candidates[i]),
			emb:   embeddings[candidates[i].ID],
		}
	}

	// Sort by score descending for deterministic first-item selection.
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Greedy MMR selection.
	selected := make([]scoredCandidate, 0, limit)
	remaining := make([]scoredCandidate, len(scored))
	copy(remaining, scored)

	for len(selected) < limit && len(remaining) > 0 {
		var bestIdx int
		var bestMMR float64
		first := true

		for i := range remaining {
			c := &remaining[i]
			var mmr float64
			if !embeddingsAvailable || c.emb == nil {
				// Fallback: no diversity penalty — pure score.
				mmr = c.score
			} else {
				// Compute max similarity to already-selected items.
				maxSim := 0.0
				for j := range selected {
					if selected[j].emb != nil {
						sim := store.CosineSimilarity(c.emb, selected[j].emb)
						if sim > maxSim {
							maxSim = sim
						}
					}
				}
				mmr = lambda*c.score - (1-lambda)*maxSim
			}

			if first || mmr > bestMMR {
				bestMMR = mmr
				bestIdx = i
				first = false
			}
		}

		selected = append(selected, remaining[bestIdx])
		remaining = append(remaining[:bestIdx], remaining[bestIdx+1:]...)
	}

	// Extract nodes and update last_surfaced.
	result := make([]store.Node, len(selected))
	resultIDs := make([]string, len(selected))
	for i := range selected {
		result[i] = selected[i].node
		resultIDs[i] = selected[i].node.ID
	}
	_ = b.Store().TouchSurfaced(resultIDs) // best-effort

	return result, nil
}

// fetchMMRCandidates collects candidate nodes from all type strata for MMR selection.
// Each type stratum fetches 3x the per-stratum budget; the recent stratum fetches 5x.
// Returns deduplicated candidates.
func fetchMMRCandidates(b *brain.Brain, limit int) ([]store.Node, error) {
	sevenDaysAgo := time.Now().Add(-7 * 24 * time.Hour)
	fortyEightHoursAgo := time.Now().Add(-48 * time.Hour)

	type stratumSpec struct {
		opts       store.ListNodesOpts
		budget     int
		multiplier int
	}

	strata := []stratumSpec{
		{
			opts: store.ListNodesOpts{
				Type:    store.TypeConcept,
				Status:  "active",
				OrderBy: "importance",
			},
			budget:     int(float64(limit)*0.35 + 0.5),
			multiplier: 3,
		},
		{
			opts: store.ListNodesOpts{
				Type:    store.TypeProcedure,
				Status:  "active",
				OrderBy: "importance",
			},
			budget:     int(float64(limit)*0.25 + 0.5),
			multiplier: 3,
		},
		{
			opts: store.ListNodesOpts{
				Type:    store.TypeEpisode,
				Status:  "active",
				OrderBy: "created_at",
				Since:   &sevenDaysAgo,
			},
			budget:     int(float64(limit)*0.20 + 0.5),
			multiplier: 2,
		},
		{
			opts: store.ListNodesOpts{
				Type:    store.TypeReflection,
				Status:  "active",
				OrderBy: "importance",
			},
			budget:     int(float64(limit)*0.10 + 0.5),
			multiplier: 3,
		},
		{
			// Recent stratum: any type, ordered by recently changed, last 48h.
			opts: store.ListNodesOpts{
				Status:       "active",
				OrderBy:      "recently_changed",
				SinceChanged: &fortyEightHoursAgo,
			},
			budget:     int(float64(limit)*0.10 + 0.5),
			multiplier: 5,
		},
	}

	seen := make(map[string]bool)
	var all []store.Node

	for _, s := range strata {
		budget := s.budget
		if budget < 1 {
			budget = 1
		}
		opts := s.opts
		opts.Limit = budget * s.multiplier

		nodes, err := b.List(opts)
		if err != nil {
			continue
		}
		for i := range nodes {
			if !seen[nodes[i].ID] {
				seen[nodes[i].ID] = true
				all = append(all, nodes[i])
			}
		}
	}

	return all, nil
}
