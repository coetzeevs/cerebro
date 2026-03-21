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
			defer func() { _ = b.Close() }()

			id, err := resolveID(b, args[0])
			if err != nil {
				return err
			}

			if err := b.Reinforce(id); err != nil {
				return err
			}
			if !quietFlag {
				fmt.Printf("Reinforced %s\n", id[:8])
			}
			return nil
		},
	})
}
