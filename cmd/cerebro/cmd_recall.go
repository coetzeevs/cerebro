package main

import (
	"context"
	"strings"

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
	defer b.Close()

	// Prime mode without query: return top active memories by importance.
	// This requires no embeddings and works as a reliable session-start briefing.
	if recallPrimeFlag && query == "" {
		nodes, err := b.List(store.ListNodesOpts{
			Status:  "active",
			OrderBy: "importance",
			Limit:   recallLimitFlag,
		})
		if err != nil {
			return err
		}
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
