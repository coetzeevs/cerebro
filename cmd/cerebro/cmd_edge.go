package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "edge <source-id> <target-id> <relation>",
		Short: "Create a relationship edge between two nodes",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := openBrain()
			if err != nil {
				return err
			}
			defer b.Close()

			id, err := b.AddEdge(args[0], args[1], args[2])
			if err != nil {
				return err
			}

			if formatFlag == "json" {
				outputJSON(map[string]int64{"id": id})
			} else if !quietFlag {
				fmt.Printf("Edge %d: %s -[%s]-> %s\n", id, args[0][:8], args[2], args[1][:8])
			}
			return nil
		},
	})
}
