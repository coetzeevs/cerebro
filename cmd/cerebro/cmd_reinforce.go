package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "reinforce <id>",
		Short: "Reinforce a memory (increment access count)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := openBrain()
			if err != nil {
				return err
			}
			defer b.Close()

			if err := b.Reinforce(args[0]); err != nil {
				return err
			}
			if !quietFlag {
				fmt.Printf("Reinforced %s\n", args[0])
			}
			return nil
		},
	})
}
