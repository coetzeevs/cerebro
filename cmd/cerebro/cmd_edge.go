package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "edge <source-id> <target-id> <relation>",
		Short: "Create a relationship edge between two nodes",
		Long:  `Create a relationship edge between two nodes. IDs accept full UUIDs or unique short prefixes (minimum 4 characters).`,
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := openBrain()
			if err != nil {
				return err
			}
			defer func() { _ = b.Close() }()

			sourceID, err := resolveID(b, args[0])
			if err != nil {
				return fmt.Errorf("resolving source ID: %w", err)
			}
			targetID, err := resolveID(b, args[1])
			if err != nil {
				return fmt.Errorf("resolving target ID: %w", err)
			}

			id, err := b.AddEdge(sourceID, targetID, args[2])
			if err != nil {
				return err
			}

			if formatFlag == "json" {
				outputJSON(map[string]int64{"id": id})
			} else if !quietFlag {
				fmt.Printf("Edge %d: %s -[%s]-> %s\n", id, sourceID[:8], args[2], targetID[:8])
			}
			return nil
		},
	})
}
