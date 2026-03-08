package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "mark-consolidated <id> [id...]",
		Short: "Mark episode memories as consolidated",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := openBrain()
			if err != nil {
				return err
			}
			defer func() { _ = b.Close() }()

			if err := b.MarkConsolidated(args); err != nil {
				return err
			}
			if !quietFlag {
				fmt.Printf("Marked %d episode(s) as consolidated\n", len(args))
			}
			return nil
		},
	})
}
