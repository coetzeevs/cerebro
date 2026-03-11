package main

import (
	"fmt"
	"os"

	"github.com/coetzeevs/cerebro/internal/store"
	"github.com/spf13/cobra"
)

var importConflictFlag string

func init() {
	cmd := &cobra.Command{
		Use:   "import <file>",
		Short: "Import memories from a JSON export",
		Long: `Import nodes and edges from a JSON export file into the current brain.

Conflict resolution:
  skip     Keep existing nodes on ID conflict (default)
  replace  Overwrite existing nodes with imported data`,
		Args: cobra.ExactArgs(1),
		RunE: runImport,
	}
	cmd.Flags().StringVar(&importConflictFlag, "on-conflict", "skip", "Conflict strategy: skip, replace")
	rootCmd.AddCommand(cmd)
}

func runImport(cmd *cobra.Command, args []string) error {
	filePath := args[0]

	var strategy store.ConflictStrategy
	switch importConflictFlag {
	case "skip":
		strategy = store.ConflictSkip
	case "replace":
		strategy = store.ConflictReplace
	default:
		return fmt.Errorf("invalid --on-conflict value %q: must be skip or replace", importConflictFlag)
	}

	b, err := openBrain()
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()

	f, err := os.Open(filePath) //nolint:gosec // user-specified import path
	if err != nil {
		return fmt.Errorf("opening import file: %w", err)
	}
	defer f.Close() //nolint:errcheck // read-only

	result, err := b.ImportFromJSON(f, store.ImportOptions{OnConflict: strategy})
	if err != nil {
		return err
	}

	if formatFlag == "json" {
		outputJSON(result)
	} else if !quietFlag {
		fmt.Printf("Imported: %d nodes, %d edges\n", result.NodesImported, result.EdgesImported)
		if result.NodesSkipped > 0 {
			fmt.Printf("Skipped: %d nodes (conflict)\n", result.NodesSkipped)
		}
		if result.EdgesSkipped > 0 {
			fmt.Printf("Skipped: %d edges (conflict)\n", result.EdgesSkipped)
		}
	}

	return nil
}
