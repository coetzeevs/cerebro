package main

import (
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "stats",
		Short: "Show brain health metrics",
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := openBrain()
			if err != nil {
				return err
			}
			defer b.Close()

			stats, err := b.Stats()
			if err != nil {
				return err
			}

			outputStats(stats)
			return nil
		},
	})
}
