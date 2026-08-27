package main

// cmd_embed.go — `cerebro embed --pending` (agentic-h6gc): the backfill that
// Import's "nodes will need re-embedding" contract always assumed existed.
// Embeds every active node lacking a vector (oversized content chunks +
// mean-pools in the brain layer), reports per-node results, and clears
// has_pending_embeddings only at zero remaining.

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

var embedPendingFlag bool

func init() {
	cmd := &cobra.Command{
		Use:   "embed --pending",
		Short: "Backfill embeddings for nodes missing vectors",
		Long: `Embed every active node that has no vector in vec_nodes — nodes whose
embedding failed at write time (provider down, content over the provider's
input limit) or that arrived via import.

Oversized content is chunked and mean-pooled automatically, so memories that
could never embed in one provider call gain a vector here. Per-node results
are reported; failures are warned loudly and stay pending. The
has_pending_embeddings flag clears only when zero nodes remain. Idempotent —
already-vectorized nodes are never re-embedded.`,
		Args: cobra.NoArgs,
		RunE: runEmbed,
	}
	cmd.Flags().BoolVar(&embedPendingFlag, "pending", false,
		"Backfill all active nodes lacking embeddings (required — the only embed mode)")
	_ = cmd.MarkFlagRequired("pending")
	rootCmd.AddCommand(cmd)
}

func runEmbed(cmd *cobra.Command, args []string) error {
	b, err := openBrain()
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()

	res, err := b.EmbedPending(context.Background())
	if err != nil {
		return err
	}

	if formatFlag == "json" {
		outputJSON(res)
		return nil
	}
	if !quietFlag {
		for _, id := range res.Embedded {
			fmt.Printf("embedded %s\n", id[:8])
		}
		for id, reason := range res.Failed {
			fmt.Printf("FAILED   %s: %s\n", id[:8], reason)
		}
		fmt.Printf("Embedded %d node(s), %d failed, %d remaining pending\n",
			len(res.Embedded), len(res.Failed), res.Remaining)
	}
	return nil
}
