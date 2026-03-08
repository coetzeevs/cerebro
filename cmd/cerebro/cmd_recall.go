package main

import (
	"context"
	"strings"
	"time"

	"github.com/coetzeevs/cerebro/brain"
	"github.com/coetzeevs/cerebro/internal/store"
	"github.com/spf13/cobra"
)

var recallLimitFlag int
var recallPrimeFlag bool

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
	// Future: add graph expansion, procedural lookup, dual-store merge.
	results, err := b.Search(context.Background(), query, recallLimitFlag, 0.3)
	if err != nil {
		return err
	}

	outputScoredList(results)
	return nil
}

// primeStratified returns a type-balanced selection of memories for session priming.
// Budget: 40% concepts, 30% procedures, 20% episodes (recent), 10% reflections.
func primeStratified(b *brain.Brain, limit int) []store.Node {
	type stratum struct {
		nodeType store.NodeType
		fraction float64
		orderBy  string
		since    *time.Time // optional: only fetch recent
	}

	sevenDaysAgo := time.Now().Add(-7 * 24 * time.Hour)
	strata := []stratum{
		{store.TypeConcept, 0.40, "importance", nil},
		{store.TypeProcedure, 0.30, "importance", nil},
		{store.TypeEpisode, 0.20, "created_at", &sevenDaysAgo},
		{store.TypeReflection, 0.10, "importance", nil},
	}

	seen := make(map[string]bool)
	var result []store.Node

	for _, s := range strata {
		budget := int(float64(limit)*s.fraction + 0.5)
		if budget < 1 {
			budget = 1
		}
		nodes, err := b.List(store.ListNodesOpts{
			Type:    s.nodeType,
			Status:  "active",
			OrderBy: s.orderBy,
			Limit:   budget,
			Since:   s.since,
		})
		if err != nil {
			continue
		}
		for i := range nodes {
			if !seen[nodes[i].ID] {
				seen[nodes[i].ID] = true
				result = append(result, nodes[i])
			}
		}
	}

	// Cap at limit
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}
