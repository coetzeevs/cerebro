package main

import (
	"fmt"

	"github.com/coetzeevs/cerebro/brain"
	"github.com/spf13/cobra"
)

var consolidateIntoFlag string
var consolidateSuggestFlag bool
var consolidateSuggestLimitFlag int

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

Walk the recorded lineage with 'get <concept-id> --with-provenance'.

With --suggest, no consolidation happens: cerebro surfaces rollup CANDIDATES —
active episodes grouped by subtype, biggest groups first, oldest first within a
group (agentic-eq7a). Read the groups, synthesize a concept/procedure/reflection
with 'cerebro add', then consolidate the cluster into it with --into. The agent
does the synthesis; cerebro only selects and wires provenance.`,
		Args: cobra.ArbitraryArgs,
		RunE: runConsolidate,
	}
	cmd.Flags().StringVar(&consolidateIntoFlag, "into", "",
		"Concept/procedure/reflection node ID the episodes consolidate into")
	cmd.Flags().BoolVar(&consolidateSuggestFlag, "suggest", false,
		"List consolidation candidates (active episodes grouped by subtype) instead of consolidating")
	cmd.Flags().IntVar(&consolidateSuggestLimitFlag, "limit", 10,
		"With --suggest: max episodes listed per group (group counts stay exact)")
	cmd.MarkFlagsMutuallyExclusive("into", "suggest")
	cmd.MarkFlagsOneRequired("into", "suggest")
	rootCmd.AddCommand(cmd)
}

func runConsolidate(cmd *cobra.Command, args []string) error {
	b, err := openBrain()
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()

	if consolidateSuggestFlag {
		return runConsolidateSuggest(b, args)
	}
	if len(args) < 1 {
		return fmt.Errorf("requires at least 1 episode id with --into")
	}

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

func runConsolidateSuggest(b *brain.Brain, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("--suggest takes no positional arguments")
	}
	groups, err := b.ConsolidationCandidates(consolidateSuggestLimitFlag)
	if err != nil {
		return err
	}
	if formatFlag == "json" {
		outputJSON(groups)
		return nil
	}
	if len(groups) == 0 {
		if !quietFlag {
			fmt.Println("No active episodes to consolidate.")
		}
		return nil
	}
	for _, g := range groups {
		label := g.Subtype
		if label == "" {
			label = "(no subtype)"
		}
		fmt.Printf("## %s — %d active episode(s)\n", label, g.Count)
		for i := range g.Nodes {
			n := &g.Nodes[i]
			preview := n.Content
			if len(preview) > 96 {
				preview = preview[:96] + "…"
			}
			fmt.Printf("  %s  %s  acc=%d  %s\n", n.ID[:8], n.CreatedAt.Format("2006-01-02"), n.AccessCount, preview)
		}
		if g.Count > len(g.Nodes) {
			fmt.Printf("  … and %d more (raise --limit to list)\n", g.Count-len(g.Nodes))
		}
		fmt.Println()
	}
	fmt.Println("Synthesize a concept/procedure/reflection per cluster with 'cerebro add',")
	fmt.Println("then: cerebro consolidate --into <new-id> <episode-id>...")
	return nil
}
