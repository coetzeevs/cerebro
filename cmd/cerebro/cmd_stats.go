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
	// Extract time-series for each metric.
	readEdit := make([]float64, len(turns))
	thinking := make([]float64, len(turns))
	cacheHit := make([]float64, len(turns))
	stopFired := make([]bool, len(turns))

	var totalReads, totalEdits, zeroThink, totalStops int
	var cacheHitSum float64
	cacheHitCount := 0

	for i := range turns {
		t := &turns[i]
		// Read:Edit ratio — use raw value, 0 if no edits.
		if t.ReadEditRatio != nil {
			readEdit[i] = *t.ReadEditRatio
		}

		// Thinking chars.
		thinking[i] = float64(t.ThinkingChars)
		if t.ThinkingChars == 0 {
			zeroThink++
		}

		// Cache hit rate: cache_read / (input + cache_read + cache_create).
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

	width := 50

	fmt.Printf("\n## Session Metrics (last %d turns)\n\n", len(turns))
	fmt.Printf("  Read:Edit  %s\n", metrics.Sparkline(readEdit, width))
	fmt.Printf("  Thinking   %s\n", metrics.Sparkline(thinking, width))
	fmt.Printf("  Cache Hit  %s\n", metrics.Sparkline(cacheHit, width))
	fmt.Printf("  Stop Guard %s\n", metrics.BoolSparkline(stopFired, width, '*'))
	fmt.Println()

	// Summary line.
	avgRE := float64(0)
	if totalEdits > 0 {
		avgRE = float64(totalReads) / float64(totalEdits)
	}
	zeroPct := float64(zeroThink) / float64(len(turns)) * 100
	avgCache := float64(0)
	if cacheHitCount > 0 {
		avgCache = cacheHitSum / float64(cacheHitCount) * 100
	}

	fmt.Printf("  R:E: %.1f  Zero-think: %.0f%%  Cache: %.0f%%  Stops blocked: %d\n",
		avgRE, zeroPct, avgCache, totalStops)
}
