package main

import (
	"fmt"

	"github.com/coetzeevs/cerebro/brain"
	"github.com/spf13/cobra"
)

var edgeValidAtFlag string
var edgeInvalidAtFlag string

func init() {
	cmd := &cobra.Command{
		Use:   "edge <source-id> <target-id> <relation>",
		Short: "Create a relationship edge between two nodes",
		Long: `Create a relationship edge between two nodes. IDs accept full UUIDs or unique short prefixes (minimum 4 characters).

A bi-temporal validity window can be attached with --valid-at / --invalid-at
(agentic-xtzn). The window is the half-open interval [valid-at, invalid-at): an
edge is valid at instant T when valid-at <= T < invalid-at. Omitting a bound
leaves it open-ended (NULL): no --valid-at means "valid from the beginning of
time"; no --invalid-at means "still valid". Both flags accept RFC3339
(2026-06-17T14:30:00Z) or a date (2026-06-17, midnight UTC); all values are
normalized to UTC.

Re-running edge for an existing source/target/relation UPDATES its window in
place (no duplicate edge): the re-add re-asserts the FULL window, so omitting a
flag clears that bound to NULL rather than preserving the previous value.`,
		Args: cobra.ExactArgs(3),
		RunE: runEdge,
	}
	cmd.Flags().StringVar(&edgeValidAtFlag, "valid-at", "",
		"Validity-window lower bound (inclusive). RFC3339 or date (YYYY-MM-DD); UTC-normalized. Omit for open-ended (NULL = valid from -inf).")
	cmd.Flags().StringVar(&edgeInvalidAtFlag, "invalid-at", "",
		"Validity-window upper bound (exclusive). RFC3339 or date (YYYY-MM-DD); UTC-normalized. Omit for open-ended (NULL = still valid).")
	rootCmd.AddCommand(cmd)
}

func runEdge(cmd *cobra.Command, args []string) error {
	// Parse the validity bounds BEFORE opening the brain so a malformed flag
	// fails fast with no store side effects (Security guardrail).
	var opts brain.AddEdgeOpts
	if cmd.Flags().Changed("valid-at") {
		t, err := parseAsOf(edgeValidAtFlag)
		if err != nil {
			return fmt.Errorf("--valid-at: %w", err)
		}
		opts.ValidAt = &t
	}
	if cmd.Flags().Changed("invalid-at") {
		t, err := parseAsOf(edgeInvalidAtFlag)
		if err != nil {
			return fmt.Errorf("--invalid-at: %w", err)
		}
		opts.InvalidAt = &t
	}
	// Reject an inverted window (E-2): valid_at strictly after invalid_at is
	// never valid at any instant under the half-open convention and is almost
	// certainly a typo. Equal bounds (zero-width [t,t)) are allowed — they are a
	// well-defined degenerate window valid at no instant.
	if opts.ValidAt != nil && opts.InvalidAt != nil && opts.ValidAt.After(*opts.InvalidAt) {
		return fmt.Errorf("invalid window: --valid-at (%s) is after --invalid-at (%s); the window must satisfy valid-at <= invalid-at",
			opts.ValidAt.Format("2006-01-02T15:04:05Z07:00"), opts.InvalidAt.Format("2006-01-02T15:04:05Z07:00"))
	}

	b, err := openBrain()
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()

	sourceID, err := resolveID(b, args[0])
	if err != nil {
		return fmt.Errorf("resolving source ID: %w", err)
	}
	targetID, err := resolveID(b, args[1])
	if err != nil {
		return fmt.Errorf("resolving target ID: %w", err)
	}

	// AddEdge returns the PERSISTED id via RETURNING id, so on a re-add (upsert)
	// the existing edge's id is reported — never 0 / stale (TL-PI-N4).
	id, err := b.AddEdge(sourceID, targetID, args[2], opts)
	if err != nil {
		return err
	}

	if formatFlag == "json" {
		outputJSON(map[string]int64{"id": id})
	} else if !quietFlag {
		fmt.Printf("Edge %d: %s -[%s]-> %s\n", id, sourceID[:8], args[2], targetID[:8])
	}
	return nil
}
