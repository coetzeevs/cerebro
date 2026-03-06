package main

import (
	"context"
	"strings"

	"github.com/spf13/cobra"
)

var recallLimitFlag int
var recallPrimeFlag bool

func init() {
	cmd := &cobra.Command{
		Use:   "recall <query>",
		Short: "Retrieve scored memories relevant to a query",
		Long: `Recall performs composite-scored retrieval combining vector similarity,
importance, recency, and graph structure. Use --prime for session-start context.`,
		Args: cobra.MinimumNArgs(1),
		RunE: runRecall,
	}
	cmd.Flags().IntVarP(&recallLimitFlag, "limit", "l", 20, "Maximum results")
	cmd.Flags().BoolVar(&recallPrimeFlag, "prime", false, "Session-start mode: curated high-value selection")
	rootCmd.AddCommand(cmd)
}

func runRecall(cmd *cobra.Command, args []string) error {
	query := strings.Join(args, " ")

	b, err := openBrain()
	if err != nil {
		return err
	}
	defer b.Close()

	// For now, recall is implemented as search with composite scoring.
	// Future: add graph expansion, procedural lookup, dual-store merge.
	results, err := b.Search(context.Background(), query, recallLimitFlag, 0.3) // lower threshold for recall
	if err != nil {
		return err
	}

	outputScoredList(results)
	return nil
}
