package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var consolidateIntoFlag string

func init() {
	cmd := &cobra.Command{
		Use:   "consolidate --into <concept-id> <episode-id> [episode-id...]",
		Short: "Consolidate source episodes into a node, recording derived_from provenance",
		Long: `Consolidate flips each source episode to status='consolidated' AND auto-writes
a built-in 'derived_from' provenance edge from the --into node to each source
episode, so the synthesized concept/procedure/reflection carries structural
provenance back to the episodes it came from (agentic-lbjg).

The operation is atomic and fail-closed: the --into node and every source must
resolve as an episode, else the whole command is rejected with a non-zero exit
and no partial write. It is idempotent — re-running writes no duplicate edges and
re-asserts the provenance of already-consolidated episodes without error.

This is distinct from 'mark-consolidated', which only flips episode status and
writes no edges. Use 'consolidate --into' when you have a concept to attribute
the episodes to; use 'mark-consolidated' when you do not.

Walk the recorded lineage with 'get <concept-id> --with-provenance'.`,
		Args: cobra.MinimumNArgs(1),
		RunE: runConsolidate,
	}
	cmd.Flags().StringVar(&consolidateIntoFlag, "into", "",
		"Concept/procedure/reflection node ID the episodes consolidate into (required)")
	_ = cmd.MarkFlagRequired("into")
	rootCmd.AddCommand(cmd)
}

func runConsolidate(cmd *cobra.Command, args []string) error {
	b, err := openBrain()
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()

	intoID, err := resolveID(b, consolidateIntoFlag)
	if err != nil {
		return fmt.Errorf("--into: %w", err)
	}

	episodeIDs := make([]string, len(args))
	for i, a := range args {
		id, err := resolveID(b, a)
		if err != nil {
			return fmt.Errorf("source %q: %w", a, err)
		}
		episodeIDs[i] = id
	}

	if err := b.Consolidate(intoID, episodeIDs); err != nil {
		return err
	}

	if formatFlag == "json" {
		outputJSON(map[string]any{
			"into":         intoID,
			"consolidated": episodeIDs,
			"derived_from": len(episodeIDs),
		})
	} else if !quietFlag {
		fmt.Printf("Consolidated %d episode(s) into %s (wrote %d derived_from edge(s))\n",
			len(episodeIDs), intoID[:8], len(episodeIDs))
	}
	return nil
}
