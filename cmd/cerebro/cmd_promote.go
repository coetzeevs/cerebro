package main

import (
	"fmt"
	"strings"

	"github.com/coetzeevs/cerebro/brain"
	"github.com/spf13/cobra"
)

var promoteContentFlag string

func init() {
	cmd := &cobra.Command{
		Use:   "promote <id> [generalized content]",
		Short: "Copy a memory node to the global store",
		Long: `Promote copies a node from the current project store to the global store.
Use positional args or --content to supply generalized content (stripping project-specific details).
The source node is updated with a reference to the promoted global node.`,
		Args: cobra.MinimumNArgs(1),
		RunE: runPromote,
	}
	cmd.Flags().StringVar(&promoteContentFlag, "content", "", "Generalized content for the global copy")
	rootCmd.AddCommand(cmd)
}

func runPromote(cmd *cobra.Command, args []string) error {
	src, err := openBrain()
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	nodeID, err := resolveID(src, args[0])
	if err != nil {
		return err
	}

	dst, err := brain.Open(brain.GlobalPath())
	if err != nil {
		return fmt.Errorf("global store not initialized — run 'cerebro init --global' first: %w", err)
	}
	defer func() { _ = dst.Close() }()

	var opts []brain.PromoteOption
	content := promoteContentFlag
	if content == "" && len(args) > 1 {
		content = strings.Join(args[1:], " ")
	}
	if content != "" {
		opts = append(opts, brain.WithPromoteContent(content))
	}

	globalID, err := src.Promote(cmd.Context(), nodeID, dst, opts...)
	if err != nil {
		return err
	}

	if formatFlag == "json" {
		outputJSON(map[string]string{
			"global_id":  globalID,
			"project_id": nodeID,
		})
	} else if !quietFlag {
		fmt.Printf("Promoted to global store: %s\n", globalID)
	}

	return nil
}
