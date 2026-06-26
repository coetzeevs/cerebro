package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/coetzeevs/cerebro/brain"
	"github.com/coetzeevs/cerebro/internal/store"
	"github.com/spf13/cobra"
)

var recallLimitFlag int
var recallThresholdFlag float64
var recallPrimeFlag bool
var recallGlobalFlag bool
var recallSubtypeFlag string
var recallAsOfFlag string
var recallWithProvenanceFlag int
var recallProvenanceDepthFlag int

func init() {
	cmd := &cobra.Command{
		Use:   "recall [query]",
		Short: "Retrieve scored memories relevant to a query",
		Long: `Recall performs composite-scored retrieval combining vector similarity,
importance, recency, and graph structure. Use --prime for session-start context.

With --prime and no query, returns top memories by importance (no embeddings needed).
With --prime and a query, performs vector search with a low threshold.`,
		Args: cobra.ArbitraryArgs,
		RunE: runRecall,
	}
	cmd.Flags().IntVarP(&recallLimitFlag, "limit", "l", 20, "Maximum results")
	cmd.Flags().Float64VarP(&recallThresholdFlag, "threshold", "T", 0.3, "Minimum similarity threshold for query mode")
	cmd.Flags().BoolVar(&recallPrimeFlag, "prime", false, "Session-start mode: curated high-value selection")
	cmd.Flags().BoolVar(&recallGlobalFlag, "global", false, "Query global store in addition to project store")
	cmd.Flags().StringVar(&recallSubtypeFlag, "subtype", "",
		`Filter results by subtype after composite scoring. Applied post-ExpandGraph so
composite scores, threshold, and ranking are unaffected by the filter.
Use --subtype "" to return only nodes with no subtype (NULL).
Non-empty value performs exact match.
NOTE: --subtype is ignored in --prime mode (prime uses stratified/MMR selection).
NOTE: --subtype "" on recall *filters* for NULL-subtype nodes. On update, --subtype "" *clears* the subtype.`)
	cmd.Flags().StringVar(&recallAsOfFlag, "as-of", "",
		`Traverse only edges valid at this instant during graph expansion (agentic-xtzn).
RFC3339 (2026-06-17T14:30:00Z) or date (2026-06-17, midnight UTC); UTC-normalized.
Half-open window [valid_at, invalid_at); NULL bounds are open-ended.
Independent of --subtype (both may be supplied).
NOTE: --as-of is a no-op when the lazy-expansion gate skips expansion for the query
(no edges are traversed, so there is nothing to filter), and is ignored in --prime mode.`)
	cmd.Flags().IntVar(&recallWithProvenanceFlag, "with-provenance", 1,
		`Attach the derived_from lineage chain per result (agentic-lbjg).
Bare --with-provenance walks depth 1 (immediate parents); --with-provenance=N walks depth N.
Depth is clamped to 100. Omitting the flag is byte-identical to pre-provenance output.
A computed provenance_status field (complete|none|legacy) always appears in JSON output.`)
	cmd.Flags().Lookup("with-provenance").NoOptDefVal = "1"
	cmd.Flags().IntVar(&recallProvenanceDepthFlag, "provenance-depth", 1,
		`Explicit provenance-walk depth (alias for --with-provenance=N; wins if both are given). Clamped to 100.`)
	rootCmd.AddCommand(cmd)
}

func runRecall(cmd *cobra.Command, args []string) error {
	query := strings.Join(args, " ")

	if query == "" && !recallPrimeFlag {
		return cmd.Help()
	}

	b, err := openBrain()
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()

	// Apply brain config: prime_limit for --prime mode, recall_threshold for query mode.
	if recallPrimeFlag {
		applyConfigFlag(cmd, b.Store(), "limit", "prime_limit")
	}
	applyConfigFlag(cmd, b.Store(), "threshold", "recall_threshold")

	// Prime mode without query: MMR-based retrieval for diverse, surprise-aware
	// session briefing. Falls back to stratified if MMR encounters an error.
	if recallPrimeFlag && query == "" {
		nodes, err := primeMMR(b, recallLimitFlag, store.PrimeScore)
		if err != nil || nodes == nil {
			// Fallback to stratified (no embeddings or other error)
			nodes = primeStratified(b, recallLimitFlag)
		}
		outputNodeList(nodes)
		return nil
	}

	// Query mode: vector search with composite scoring.
	// --subtype filter is resolved here: nil if flag not provided (backward compat),
	// &value if flag was provided (including empty string for NULL-match).
	var subtypeFilter *string
	if cmd.Flags().Changed("subtype") {
		subtypeFilter = &recallSubtypeFlag
	}

	// --as-of: nil when the flag is absent (pre-xtzn behaviour — edge predicate
	// omitted). Parsed before the brain call so a malformed value fails fast.
	var asOf *time.Time
	if cmd.Flags().Changed("as-of") {
		t, parseErr := parseAsOf(recallAsOfFlag)
		if parseErr != nil {
			return fmt.Errorf("--as-of: %w", parseErr)
		}
		asOf = &t
	}

	var results []store.ScoredNode
	if recallGlobalFlag {
		global, globalErr := brain.Open(brain.GlobalPath())
		if globalErr != nil {
			return fmt.Errorf("global store not initialized — run 'cerebro init --global' first: %w", globalErr)
		}
		defer func() { _ = global.Close() }()
		results, err = b.SearchWithGlobal(context.Background(), query, recallLimitFlag, recallThresholdFlag, global, subtypeFilter, asOf)
	} else {
		results, err = b.Search(context.Background(), query, recallLimitFlag, recallThresholdFlag, subtypeFilter, asOf)
	}
	if err != nil {
		return err
	}

	// Provenance resolution (AC5): --provenance-depth wins over --with-provenance=N
	// if both are given (documented). The chain is attached only when at least one
	// of the flags is engaged, so the flag-absent path stays byte-identical.
	withProvenance := cmd.Flags().Changed("with-provenance") || cmd.Flags().Changed("provenance-depth")
	depth := recallWithProvenanceFlag
	if cmd.Flags().Changed("provenance-depth") {
		depth = recallProvenanceDepthFlag
	}
	depth = clampProvenanceDepth(depth)

	outputScoredListWithProvenance(b, results, withProvenance, depth)
	return nil
}

// outputScoredListWithProvenance renders recall results, attaching the computed
// provenance_status (always, in JSON) and the optional --with-provenance lineage
// chain per result. When withProvenance is false and format is md, the output is
// byte-identical to the pre-lbjg outputScoredList path.
func outputScoredListWithProvenance(b *brain.Brain, nodes []store.ScoredNode, withProvenance bool, depth int) {
	if formatFlag == "json" {
		ids := make([]string, len(nodes))
		for i := range nodes {
			ids[i] = nodes[i].ID
		}
		statuses := provenanceStatusForAll(b, ids)

		out := make([]scoredNodeWithProvenance, len(nodes))
		for i := range nodes {
			out[i] = scoredNodeWithProvenance{
				ScoredNode:       nodes[i],
				ProvenanceStatus: statuses[nodes[i].ID],
			}
			if withProvenance {
				walk, err := b.WalkProvenance(nodes[i].ID, depth)
				if err == nil {
					out[i].Provenance = toProvenanceChain(walk)
				}
			}
		}
		outputJSON(out)
		return
	}

	// md: when no provenance flag is engaged, this is byte-identical to
	// outputScoredList (the pre-lbjg path).
	if !withProvenance {
		outputScoredList(nodes)
		return
	}
	if len(nodes) == 0 {
		fmt.Println("No relevant memories found.")
		return
	}
	for i := range nodes {
		outputScoredNode(&nodes[i])
		walk, err := b.WalkProvenance(nodes[i].ID, depth)
		if err == nil {
			renderProvenanceChainMD(toProvenanceChain(walk), depth)
		}
	}
}

// primeStratified returns a type-balanced selection of memories for session priming.
// Budget: concepts 35%, procedures 25%, episodes 20%, reflections 10%, recent 10%.
//
// For concepts, procedures, and reflections strata, candidates are fetched in
// importance order from the DB, then re-sorted by PrimeScore (blends importance
// with surprise signal) before budget selection. This ensures stale high-value
// memories surface above recently-seen low-surprise ones.
//
// The recent stratum (last 48h, any type, ordered by recently_changed) is processed
// LAST so it acts as a supplement to the type strata rather than competing with them.
//
// After selection, all chosen nodes have their last_surfaced timestamp updated via
// TouchSurfaced so the surprise signal remains accurate across sessions.
func primeStratified(b *brain.Brain, limit int) []store.Node {
	type stratum struct {
		nodeType            store.NodeType // empty = any type
		fraction            float64
		orderBy             string
		since               *time.Time // filter on created_at (episodes)
		sinceChanged        *time.Time // filter on COALESCE(updated_at, created_at)
		usePrimeScore       bool       // re-sort candidates by PrimeScore before selection
		candidateMultiplier int        // multiplier on budget for candidate fetch
	}

	sevenDaysAgo := time.Now().Add(-7 * 24 * time.Hour)
	fortyEightHoursAgo := time.Now().Add(-48 * time.Hour)

	strata := []stratum{
		// Concepts, procedures, reflections: fetch 3x budget, sort by PrimeScore.
		{store.TypeConcept, 0.35, "importance", nil, nil, true, 3},
		{store.TypeProcedure, 0.25, "importance", nil, nil, true, 3},
		// Episodes: temporal ordering (no PrimeScore re-sort — episodes are inherently temporal).
		{store.TypeEpisode, 0.20, "created_at", &sevenDaysAgo, nil, false, 1},
		{store.TypeReflection, 0.10, "importance", nil, nil, true, 3},
		// Recent stratum: any type, ordered by recently changed, last 48h.
		// Processed last — supplements type strata without competing.
		// Fetch 5x budget to account for deduplication with type strata.
		{"", 0.10, "recently_changed", nil, &fortyEightHoursAgo, false, 5},
	}

	seen := make(map[string]bool)
	var result []store.Node

	for _, s := range strata {
		budget := int(float64(limit)*s.fraction + 0.5)
		if budget < 1 {
			budget = 1
		}
		fetchLimit := budget * s.candidateMultiplier

		nodes, err := b.List(store.ListNodesOpts{
			Type:         s.nodeType,
			Status:       "active",
			OrderBy:      s.orderBy,
			Limit:        fetchLimit,
			Since:        s.since,
			SinceChanged: s.sinceChanged,
		})
		if err != nil {
			continue
		}

		// Re-sort by PrimeScore for type strata that benefit from surprise blending.
		if s.usePrimeScore && len(nodes) > 1 {
			sort.Slice(nodes, func(i, j int) bool {
				return store.PrimeScore(&nodes[i]) > store.PrimeScore(&nodes[j])
			})
		}

		added := 0
		for i := range nodes {
			if added >= budget {
				break
			}
			if !seen[nodes[i].ID] {
				seen[nodes[i].ID] = true
				result = append(result, nodes[i])
				added++
			}
		}
	}

	// Cap at limit
	if len(result) > limit {
		result = result[:limit]
	}

	// Update last_surfaced for all selected nodes so the surprise signal remains
	// accurate: nodes included in this briefing are "known" by the agent.
	if len(result) > 0 {
		ids := make([]string, len(result))
		for i := range result {
			ids[i] = result[i].ID
		}
		_ = b.Store().TouchSurfaced(ids) // best-effort; failure does not affect results
	}

	return result
}
