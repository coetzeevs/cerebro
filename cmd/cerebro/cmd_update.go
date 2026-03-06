package main

import (
	"fmt"

	"github.com/coetzeevs/cerebro/brain"
	"github.com/spf13/cobra"
)

var updateContentFlag string
var updateImportanceFlag float64
var updateImportanceSet bool

func init() {
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Modify an existing memory node",
		Args:  cobra.ExactArgs(1),
		RunE:  runUpdate,
	}
	cmd.Flags().StringVarP(&updateContentFlag, "content", "c", "", "New content")
	cmd.Flags().Float64VarP(&updateImportanceFlag, "importance", "i", 0, "New importance score")
	rootCmd.AddCommand(cmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	id := args[0]

	b, err := openBrain()
	if err != nil {
		return err
	}
	defer b.Close()

	var opts []brain.UpdateOption
	if updateContentFlag != "" {
		opts = append(opts, brain.WithContent(updateContentFlag))
	}
	if cmd.Flags().Changed("importance") {
		opts = append(opts, brain.WithUpdatedImportance(updateImportanceFlag))
	}

	if len(opts) == 0 {
		return fmt.Errorf("nothing to update — specify --content or --importance")
	}

	if err := b.Update(id, opts...); err != nil {
		return err
	}

	if !quietFlag {
		fmt.Printf("Updated %s\n", id)
	}
	return nil
}
