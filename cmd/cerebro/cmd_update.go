package main

import (
	"fmt"

	"github.com/coetzeevs/cerebro/brain"
	"github.com/spf13/cobra"
)

var updateContentFlag string
var updateImportanceFlag float64
var updateSubtypeFlag string

func init() {
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Modify an existing memory node",
		Args:  cobra.ExactArgs(1),
		RunE:  runUpdate,
	}
	cmd.Flags().StringVarP(&updateContentFlag, "content", "c", "", "New content")
	cmd.Flags().Float64VarP(&updateImportanceFlag, "importance", "i", 0, "New importance score")
	cmd.Flags().StringVar(&updateSubtypeFlag, "subtype", "",
		`New subtype tag for this node. Use --subtype "" to clear the subtype to NULL.
NOTE: --subtype "" on update *clears* the subtype. On list and recall, --subtype ""
*filters* for nodes with no subtype. This asymmetry is intentional.`)
	rootCmd.AddCommand(cmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	b, err := openBrain()
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()

	id, err := resolveID(b, args[0])
	if err != nil {
		return err
	}

	var opts []brain.UpdateOption
	if updateContentFlag != "" {
		opts = append(opts, brain.WithContent(updateContentFlag))
	}
	if cmd.Flags().Changed("importance") {
		opts = append(opts, brain.WithUpdatedImportance(updateImportanceFlag))
	}
	// --subtype uses Changed() to distinguish "not provided" (no-op) from
	// "provided as empty string" (clear to NULL). This is the standard cobra idiom.
	if cmd.Flags().Changed("subtype") {
		opts = append(opts, brain.WithUpdatedSubtype(updateSubtypeFlag))
	}

	if len(opts) == 0 {
		return fmt.Errorf("nothing to update — specify --content, --importance, or --subtype")
	}

	if err := b.Update(id, opts...); err != nil {
		return err
	}

	if !quietFlag {
		fmt.Printf("Updated %s\n", id)
	}
	return nil
}
