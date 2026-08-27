package main

// cmd_outcome.go — `cerebro outcome <id> --success|--failure` (agentic-do71):
// the agent records whether a recalled memory actually helped. Successes
// boost the memory's retrieval weight, failures penalize it harder (a
// misleading memory is worse than an unproven one is good, floored so
// repeatedly-failed memories stay findable for deliberate supersession).

import (
	"fmt"

	"github.com/spf13/cobra"
)

var outcomeSuccessFlag, outcomeFailureFlag bool

func init() {
	cmd := &cobra.Command{
		Use:   "outcome <id> (--success | --failure)",
		Short: "Record whether a memory helped (adjusts retrieval weighting)",
		Args:  cobra.ExactArgs(1),
		RunE:  runOutcome,
	}
	cmd.Flags().BoolVar(&outcomeSuccessFlag, "success", false, "The memory led to a good outcome")
	cmd.Flags().BoolVar(&outcomeFailureFlag, "failure", false, "The memory misled or failed")
	cmd.MarkFlagsMutuallyExclusive("success", "failure")
	cmd.MarkFlagsOneRequired("success", "failure")
	rootCmd.AddCommand(cmd)
}

func runOutcome(cmd *cobra.Command, args []string) error {
	b, err := openBrain()
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()

	id, err := resolveID(b, args[0])
	if err != nil {
		return err
	}
	if err := b.RecordOutcome(id, outcomeSuccessFlag); err != nil {
		return err
	}
	if formatFlag == "json" {
		kind := "success"
		if outcomeFailureFlag {
			kind = "failure"
		}
		outputJSON(map[string]string{"id": id, "outcome": kind})
	} else if !quietFlag {
		fmt.Println(id)
	}
	return nil
}
