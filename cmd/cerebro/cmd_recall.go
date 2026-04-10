package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/coetzeevs/cerebro/brain"
	"github.com/coetzeevs/cerebro/internal/store"
	"github.com/spf13/cobra"
)

var recallLimitFlag int
var recallPrimeFlag bool
var recallGlobalFlag bool

func init() {
	cmd := &cobra.Command{
		Use:   "recall [query]",
		Short: "Retrieve scored memories relevant to a query",
		Long: `Recall performs composite-scored retrieval combining vector similarity,
importance, recency, and graph structure. Use --prime for session-start context.

With --prime and no query, returns top memories by importance (no embeddings needed).
With --prime and a query, performs vector search with a low threshold.`,
		Args: cobra.ArbitraryArgs,
		RunE: runRecall,
	}
	cmd.Flags().IntVarP(&recallLimitFlag, "limit", "l", 20, "Maximum results")
	cmd.Flags().BoolVar(&recallPrimeFlag, "prime", false, "Session-start mode: curated high-value selection")
	cmd.Flags().BoolVar(&recallGlobalFlag, "global", false, "Query global store in addition to project store")
	rootCmd.AddCommand(cmd)
}

func runRecall(cmd *cobra.Command, args []string) error {
	query := strings.Join(args, " ")

	if query == "" && !recallPrimeFlag {
		return cmd.Help()
	}

	b, err := openBrain()
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()

	// Prime mode without query: type-stratified retrieval for balanced session briefing.
	// Budget: 40% concepts, 30% procedures, 20% episodes, 10% reflections.
	// No embeddings needed — works as a reliable session-start briefing.
	if recallPrimeFlag && query == "" {
		nodes := primeStratified(b, recallLimitFlag)
		outputNodeList(nodes)
		return nil
	}

	// Query mode: vector search with composite scoring.
	var results []store.ScoredNode
	if recallGlobalFlag {
		global, globalErr := brain.Open(brain.GlobalPath())
		if globalErr != nil {
			return fmt.Errorf("global store not initialized — run 'cerebro init --global' first: %w", globalErr)
		}
		defer func() { _ = global.Close() }()
		results, err = b.SearchWithGlobal(context.Background(), query, recallLimitFlag, 0.3, global)
	} else {
		results, err = b.Search(context.Background(), query, recallLimitFlag, 0.3)
	}
	if err != nil {
		return err
	}

	outputScoredList(results)
	return nil
}

// primeStratified returns a type-balanced selection of memories for session priming.
// Budget: concepts 35%, procedures 25%, episodes 20%, reflections 10%, recent 10%.
// The recent stratum (last 48h, any type, ordered by recently_changed) is processed
// LAST so it acts as a supplement to the type strata rather than competing with them.
func primeStratified(b *brain.Brain, limit int) []store.Node {
	type stratum struct {
		nodeType     store.NodeType // empty = any type
		fraction     float64
		orderBy      string
		since        *time.Time // filter on created_at (episodes)
		sinceChanged *time.Time // filter on COALESCE(updated_at, created_at)
		// candidateMultiplier controls how many candidates to fetch relative to budget.
		// Used to ensure deduplication doesn't starve a stratum.
		candidateMultiplier int
	}

	sevenDaysAgo := time.Now().Add(-7 * 24 * time.Hour)
	fortyEightHoursAgo := time.Now().Add(-48 * time.Hour)

	strata := []stratum{
		{store.TypeConcept, 0.35, "importance", nil, nil, 1},
		{store.TypeProcedure, 0.25, "importance", nil, nil, 1},
		{store.TypeEpisode, 0.20, "created_at", &sevenDaysAgo, nil, 1},
		{store.TypeReflection, 0.10, "importance", nil, nil, 1},
		// Recent stratum: any type, ordered by recently changed, last 48h.
		// Processed last — supplements type strata without competing.
		// Fetch 5x the budget to ensure deduplication doesn't starve this stratum:
		// most recently-changed nodes may already be in seen from type strata.
		{"", 0.10, "recently_changed", nil, &fortyEightHoursAgo, 5},
	}

	seen := make(map[string]bool)
	var result []store.Node

	for _, s := range strata {
		budget := int(float64(limit)*s.fraction + 0.5)
		if budget < 1 {
			budget = 1
		}
		fetchLimit := budget * s.candidateMultiplier
		nodes, err := b.List(store.ListNodesOpts{
			Type:         s.nodeType,
			Status:       "active",
			OrderBy:      s.orderBy,
			Limit:        fetchLimit,
			Since:        s.since,
			SinceChanged: s.sinceChanged,
		})
		if err != nil {
			continue
		}
		added := 0
		for i := range nodes {
			if added >= budget {
				break
			}
			if !seen[nodes[i].ID] {
				seen[nodes[i].ID] = true
				result = append(result, nodes[i])
				added++
			}
		}
	}

	// Cap at limit
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}
