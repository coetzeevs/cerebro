package main

import (
	"context"
	"strings"

	"github.com/spf13/cobra"
)

var searchLimitFlag int
var searchThresholdFlag float64

func init() {
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Vector similarity search for related memories",
		Args:  cobra.MinimumNArgs(1),
		RunE:  runSearch,
	}
	cmd.Flags().IntVarP(&searchLimitFlag, "limit", "l", 10, "Maximum results")
	cmd.Flags().Float64VarP(&searchThresholdFlag, "threshold", "T", 0.7, "Minimum similarity threshold")
	rootCmd.AddCommand(cmd)
}

func runSearch(cmd *cobra.Command, args []string) error {
	query := strings.Join(args, " ")

	b, err := openBrain()
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()

	applyConfigFlag(cmd, b.Store(), "limit", "search_limit")
	applyConfigFlag(cmd, b.Store(), "threshold", "search_threshold")

	// subtypeFilter is nil here — cerebro search has no --subtype flag (out of scope per OO-011)
	results, err := b.Search(context.Background(), query, searchLimitFlag, searchThresholdFlag, nil)
	if err != nil {
		return err
	}

	outputScoredList(results)
	return nil
}
