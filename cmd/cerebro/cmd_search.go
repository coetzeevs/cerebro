package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var searchLimitFlag int
var searchThresholdFlag float64
var searchAsOfFlag string

func init() {
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Vector similarity search for related memories",
		Args:  cobra.MinimumNArgs(1),
		RunE:  runSearch,
	}
	cmd.Flags().IntVarP(&searchLimitFlag, "limit", "l", 10, "Maximum results")
	cmd.Flags().Float64VarP(&searchThresholdFlag, "threshold", "T", 0.7, "Minimum similarity threshold")
	cmd.Flags().StringVar(&searchAsOfFlag, "as-of", "",
		"Traverse only edges valid at this instant during graph expansion. RFC3339 or date (YYYY-MM-DD); UTC-normalized. Half-open [valid_at, invalid_at); NULL bounds open-ended. NOTE: a no-op when the lazy-expansion gate skips expansion for the query.")
	rootCmd.AddCommand(cmd)
}

func runSearch(cmd *cobra.Command, args []string) error {
	query := strings.Join(args, " ")

	// Parse --as-of before opening the brain so a malformed value fails fast.
	var asOf *time.Time
	if cmd.Flags().Changed("as-of") {
		t, err := parseAsOf(searchAsOfFlag)
		if err != nil {
			return fmt.Errorf("--as-of: %w", err)
		}
		asOf = &t
	}

	b, err := openBrain()
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()

	applyConfigFlag(cmd, b.Store(), "limit", "search_limit")
	applyConfigFlag(cmd, b.Store(), "threshold", "search_threshold")

	// subtypeFilter is nil here — cerebro search has no --subtype flag (out of scope per OO-011)
	results, err := b.Search(context.Background(), query, searchLimitFlag, searchThresholdFlag, nil, asOf)
	if err != nil {
		return err
	}

	outputScoredList(results)
	return nil
}
