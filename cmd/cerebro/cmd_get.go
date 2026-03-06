package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "get <id>",
		Short: "Retrieve a specific memory node with its edges",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := openBrain()
			if err != nil {
				return err
			}
			defer b.Close()

			nwe, err := b.Get(args[0])
			if err != nil {
				return err
			}

			if formatFlag == "json" {
				outputJSON(nwe)
				return nil
			}

			fmt.Printf("# %s\n\n", nwe.ID)
			fmt.Printf("Type: %s", nwe.Type)
			if nwe.Subtype != "" {
				fmt.Printf("/%s", nwe.Subtype)
			}
			fmt.Printf("\nStatus: %s\n", nwe.Status)
			fmt.Printf("Importance: %.2f | Decay: %.4f | Access count: %d\n", nwe.Importance, nwe.DecayRate, nwe.AccessCount)
			fmt.Printf("Created: %s | Last accessed: %s\n\n", nwe.CreatedAt.Format("2006-01-02 15:04"), nwe.LastAccessed.Format("2006-01-02 15:04"))
			fmt.Printf("## Content\n%s\n\n", nwe.Content)

			if len(nwe.Edges) > 0 {
				fmt.Printf("## Edges (%d)\n", len(nwe.Edges))
				for _, e := range nwe.Edges {
					if e.SourceID == nwe.ID {
						fmt.Printf("  → %s [%s]\n", e.TargetID[:8], e.Relation)
					} else {
						fmt.Printf("  ← %s [%s]\n", e.SourceID[:8], e.Relation)
					}
				}
			}
			return nil
		},
	})
}
