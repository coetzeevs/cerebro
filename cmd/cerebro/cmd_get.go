package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var getAsOfFlag string

func init() {
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Retrieve a specific memory node with its edges",
		Long: `Retrieve a specific memory node with its edges. Accepts full UUIDs or unique short prefixes (minimum 4 characters).

With --as-of, only edges valid at the supplied instant are returned (agentic-xtzn):
an edge is valid at T when valid_at <= T < invalid_at (half-open window; NULL
bounds are open-ended). Accepts RFC3339 (2026-06-17T14:30:00Z) or a date
(2026-06-17, midnight UTC); UTC-normalized. Omit --as-of to return all edges.`,
		Args: cobra.ExactArgs(1),
		RunE: runGet,
	}
	cmd.Flags().StringVar(&getAsOfFlag, "as-of", "",
		"Filter edges to those valid at this instant. RFC3339 or date (YYYY-MM-DD); UTC-normalized. Half-open [valid_at, invalid_at); NULL bounds open-ended.")
	rootCmd.AddCommand(cmd)
}

func runGet(cmd *cobra.Command, args []string) error {
	// Parse --as-of before opening the brain so a malformed value fails fast.
	var asOf *time.Time
	if cmd.Flags().Changed("as-of") {
		t, err := parseAsOf(getAsOfFlag)
		if err != nil {
			return fmt.Errorf("--as-of: %w", err)
		}
		asOf = &t
	}

	b, err := openBrain()
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()

	id, err := resolveID(b, args[0])
	if err != nil {
		return err
	}

	nwe, err := b.Get(id, asOf)
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
		for i := range nwe.Edges {
			e := &nwe.Edges[i]
			arrow := "→"
			other := e.TargetID
			if e.SourceID != nwe.ID {
				arrow = "←"
				other = e.SourceID
			}
			fmt.Printf("  %s %s [%s]%s\n", arrow, other[:8], e.Relation, formatEdgeWindow(e.ValidAt, e.InvalidAt))
		}
	}
	return nil
}

// formatEdgeWindow renders an edge's validity window for human-readable output.
// Returns "" when both bounds are open (the today's-universal case) so unwindowed
// edges are unannotated. NULL bounds render as "-inf" / "open".
func formatEdgeWindow(validAt, invalidAt *time.Time) string {
	if validAt == nil && invalidAt == nil {
		return ""
	}
	lo := "-inf"
	if validAt != nil {
		lo = validAt.UTC().Format("2006-01-02")
	}
	hi := "open"
	if invalidAt != nil {
		hi = invalidAt.UTC().Format("2006-01-02")
	}
	return fmt.Sprintf(" (valid %s .. %s)", lo, hi)
}
