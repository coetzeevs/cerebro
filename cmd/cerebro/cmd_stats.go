package main

import (
	"fmt"
	"os"

	"github.com/coetzeevs/cerebro/internal/metrics"
	"github.com/spf13/cobra"
)

var statsMetricsFlag bool
var statsLastFlag int

func init() {
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Show brain health metrics",
		Long: `Display brain health statistics and optionally session performance metrics.

Use --metrics to show per-turn sparklines for quality signals
(read:edit ratio, thinking depth, cache efficiency, stop-guard fires).`,
		RunE: runStats,
	}
	cmd.Flags().BoolVar(&statsMetricsFlag, "metrics", false, "Show session performance sparklines")
	cmd.Flags().IntVar(&statsLastFlag, "last", 50, "Number of recent turns to show in sparklines")
	rootCmd.AddCommand(cmd)
}

func runStats(cmd *cobra.Command, args []string) error {
	b, err := openBrain()
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()

	stats, err := b.Stats()
	if err != nil {
		return err
	}

	outputStats(stats)

	if !statsMetricsFlag {
		return nil
	}

	// Open metrics DB — may not exist yet.
	metricsPath := metrics.MetricsPath(resolveProjectDir())
	if _, err := os.Stat(metricsPath); os.IsNotExist(err) {
		fmt.Println("\nNo metrics data. Run 'cerebro ingest' to collect session metrics.")
		return nil
	}

	ms, err := metrics.Open(metricsPath)
	if err != nil {
		return fmt.Errorf("opening metrics: %w", err)
	}
	defer func() { _ = ms.Close() }()

	turns, err := ms.QueryTurns(metrics.TurnFilter{
		Limit:     statsLastFlag,
		OrderDesc: true,
	})
	if err != nil {
		return fmt.Errorf("querying turns: %w", err)
	}

	if len(turns) == 0 {
		fmt.Println("\nNo turn data. Run 'cerebro ingest' to collect session metrics.")
		return nil
	}

	// Reverse so oldest is first (sparkline reads left-to-right chronologically).
	for i, j := 0, len(turns)-1; i < j; i, j = i+1, j-1 {
		turns[i], turns[j] = turns[j], turns[i]
	}

	if formatFlag == "json" {
		outputJSON(turns)
		return nil
	}

	renderMetricsSparklines(turns)
	return nil
}

func renderMetricsSparklines(turns []metrics.TurnMetrics) {
	n := len(turns)

	// Extract per-turn time-series.
	perTurnRE := make([]float64, n)
	thinkingBlocks := make([]bool, n)
	thinkingChars := make([]float64, n)
	cacheHit := make([]float64, n)
	stopFired := make([]bool, n)

	var totalReads, totalEdits, noThinkTurns, totalStops int
	var hasAnyThinkingChars bool
	var cacheHitSum float64
	cacheHitCount := 0

	for i := range turns {
		t := &turns[i]

		// Per-turn R:E ratio (sparse — reads and edits rarely co-occur in one turn).
		if t.ReadEditRatio != nil {
			perTurnRE[i] = *t.ReadEditRatio
		}

		// Thinking: blocks (boolean) and chars (depth when visible).
		if t.ThinkingBlocks > 0 {
			thinkingBlocks[i] = true
		} else {
			noThinkTurns++
		}
		thinkingChars[i] = float64(t.ThinkingChars)
		if t.ThinkingChars > 0 {
			hasAnyThinkingChars = true
		}

		// Cache hit rate.
		total := t.InputTokens + t.CacheReadTokens + t.CacheCreateTokens
		if total > 0 {
			rate := float64(t.CacheReadTokens) / float64(total)
			cacheHit[i] = rate
			cacheHitSum += rate
			cacheHitCount++
		}

		// Stop guard.
		if t.StopGuardFired > 0 {
			stopFired[i] = true
			totalStops++
		}

		totalReads += t.ToolReads
		totalEdits += t.ToolEdits
	}

	// Windowed R:E ratio (10-turn sliding window captures cross-turn read-then-edit pattern).
	windowRE := windowedReadEditRatio(turns, 10)

	width := 50

	fmt.Printf("\n## Session Metrics (last %d turns)\n\n", n)
	fmt.Printf("  R:E          %s\n", metrics.Sparkline(perTurnRE, width))
	fmt.Printf("  R:E (w10)    %s\n", metrics.Sparkline(windowRE, width))
	fmt.Printf("  Thinking     %s\n", metrics.BoolSparkline(thinkingBlocks, width, '^'))
	if hasAnyThinkingChars {
		fmt.Printf("  Think Depth  %s\n", metrics.Sparkline(thinkingChars, width))
	}
	fmt.Printf("  Cache Hit    %s\n", metrics.Sparkline(cacheHit, width))
	fmt.Printf("  Stop Guard   %s\n", metrics.BoolSparkline(stopFired, width, '*'))
	fmt.Println()

	// Summary line.
	avgRE := float64(0)
	if totalEdits > 0 {
		avgRE = float64(totalReads) / float64(totalEdits)
	}

	thinkActive := n - noThinkTurns
	thinkPct := float64(thinkActive) / float64(n) * 100
	thinkLabel := fmt.Sprintf("Think: %.0f%% active", thinkPct)
	if thinkActive > 0 && !hasAnyThinkingChars {
		thinkLabel += " (redacted)"
	}

	avgCache := float64(0)
	if cacheHitCount > 0 {
		avgCache = cacheHitSum / float64(cacheHitCount) * 100
	}

	fmt.Printf("  R:E: %.1f  %s  Cache: %.0f%%  Stops: %d\n",
		avgRE, thinkLabel, avgCache, totalStops)
}

// windowedReadEditRatio computes a sliding-window Read:Edit ratio from per-turn tool counts.
// For each position i, it sums reads and edits over the window [i-window+1 .. i].
// Returns 0 when the window contains no edits.
func windowedReadEditRatio(turns []metrics.TurnMetrics, window int) []float64 {
	n := len(turns)
	result := make([]float64, n)
	var sumReads, sumEdits int
	for i := 0; i < n; i++ {
		sumReads += turns[i].ToolReads
		sumEdits += turns[i].ToolEdits
		if i >= window {
			sumReads -= turns[i-window].ToolReads
			sumEdits -= turns[i-window].ToolEdits
		}
		if sumEdits > 0 {
			result[i] = float64(sumReads) / float64(sumEdits)
		}
	}
	return result
}
