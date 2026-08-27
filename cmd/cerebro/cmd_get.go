package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var getAsOfFlag string
var getWithProvenanceFlag int

func init() {
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Retrieve a specific memory node with its edges",
		Long: `Retrieve a specific memory node with its edges. Accepts full UUIDs or unique short prefixes (minimum 4 characters).

With --as-of, only edges valid at the supplied instant are returned (agentic-xtzn):
an edge is valid at T when valid_at <= T < invalid_at (half-open window; NULL
bounds are open-ended). Accepts RFC3339 (2026-06-17T14:30:00Z) or a date
(2026-06-17, midnight UTC); UTC-normalized. Omit --as-of to return all edges.

With --with-provenance, the node's derived_from lineage chain is attached (the
source episodes it was consolidated from, walked outward). Bare --with-provenance
walks depth 5; --with-provenance=N walks depth N (clamped to 100). Omitting the
flag is byte-identical to the pre-provenance output. A computed provenance_status
field (complete|none|legacy; agentic-lbjg) always appears in JSON output.`,
		Args: cobra.ExactArgs(1),
		RunE: runGet,
	}
	cmd.Flags().StringVar(&getAsOfFlag, "as-of", "",
		"Filter edges to those valid at this instant. RFC3339 or date (YYYY-MM-DD); UTC-normalized. Half-open [valid_at, invalid_at); NULL bounds open-ended.")
	cmd.Flags().IntVar(&getWithProvenanceFlag, "with-provenance", 5,
		"Attach the derived_from lineage chain (default depth 5; --with-provenance=N for depth N, clamped to 100)")
	// Optional-value flag: bare --with-provenance => depth 5 (NoOptDefVal).
	cmd.Flags().Lookup("with-provenance").NoOptDefVal = "5"
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

	// Provenance chain (AC5): walk derived_from outward only when the flag is
	// engaged, so the flag-absent output stays byte-identical.
	withProvenance := cmd.Flags().Changed("with-provenance")
	var chain []provenanceChainItem
	depth := clampProvenanceDepth(getWithProvenanceFlag)
	if withProvenance {
		walk, walkErr := b.WalkProvenance(id, depth)
		if walkErr != nil {
			return walkErr
		}
		chain = toProvenanceChain(walk)
	}

	if formatFlag == "json" {
		// provenance_status (AC6) is ALWAYS present in JSON; the chain is attached
		// only when --with-provenance is engaged (omitempty).
		out := nodeWithProvenance{
			NodeWithEdges:    nwe,
			ProvenanceStatus: provenanceStatusFor(b, id),
			OriginStatus:     b.OriginStatus(&nwe.Node),
			AnchorStatus:     anchorStatusFor(&nwe.Node, resolveProjectDir()),
		}
		if withProvenance {
			out.Provenance = chain
		}
		outputJSON(out)
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

	// Provenance chain block (md) — only when --with-provenance is engaged, so the
	// flag-absent output is byte-identical to the pre-lbjg path.
	if withProvenance {
		fmt.Println()
		renderProvenanceChainMD(chain, depth)
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
