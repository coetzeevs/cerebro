package dashboard

import (
	"fmt"
	"strings"

	"github.com/coetzeevs/cerebro/internal/metrics"
)

// OverviewData holds the computed data for the overview panel.
type OverviewData struct {
	TotalTurns    int
	TotalSessions int
	TotalReads    int
	TotalEdits    int
	AvgRE         float64
	ThinkActive   int
	ThinkPct      float64
	ThinkRedacted bool
	CacheHitPct   float64
	StopsBlocked  int

	// Sparkline data (last N turns, chronological).
	PerTurnRE     []float64
	WindowRE      []float64
	ThinkBlocks   []bool
	ThinkDepth    []float64
	HasThinkDepth bool
	CacheHit      []float64
	StopGuard     []bool

	// Brain health.
	BrainNodes     int
	BrainEdges     int
	BrainByType    map[string]int
	BrainColdNodes int
}

// ComputeOverview computes the overview panel data from turn metrics.
func ComputeOverview(turns []metrics.TurnMetrics) OverviewData {
	n := len(turns)
	d := OverviewData{
		TotalTurns:  n,
		BrainByType: make(map[string]int),
	}

	if n == 0 {
		return d
	}

	// Count distinct sessions.
	sessions := make(map[string]bool)
	var cacheHitSum float64
	cacheHitCount := 0

	d.PerTurnRE = make([]float64, n)
	d.ThinkBlocks = make([]bool, n)
	d.ThinkDepth = make([]float64, n)
	d.CacheHit = make([]float64, n)
	d.StopGuard = make([]bool, n)

	for i := range turns {
		t := &turns[i]
		sessions[t.SessionID] = true

		if t.ReadEditRatio != nil {
			d.PerTurnRE[i] = *t.ReadEditRatio
		}

		if t.ThinkingBlocks > 0 {
			d.ThinkBlocks[i] = true
			d.ThinkActive++
		}
		d.ThinkDepth[i] = float64(t.ThinkingChars)
		if t.ThinkingChars > 0 {
			d.HasThinkDepth = true
		}

		total := t.InputTokens + t.CacheReadTokens + t.CacheCreateTokens
		if total > 0 {
			rate := float64(t.CacheReadTokens) / float64(total)
			d.CacheHit[i] = rate
			cacheHitSum += rate
			cacheHitCount++
		}

		if t.StopGuardFired > 0 {
			d.StopGuard[i] = true
			d.StopsBlocked++
		}

		d.TotalReads += t.ToolReads
		d.TotalEdits += t.ToolEdits
	}

	d.TotalSessions = len(sessions)
	if d.TotalEdits > 0 {
		d.AvgRE = float64(d.TotalReads) / float64(d.TotalEdits)
	}
	d.ThinkPct = float64(d.ThinkActive) / float64(n) * 100
	d.ThinkRedacted = d.ThinkActive > 0 && !d.HasThinkDepth
	if cacheHitCount > 0 {
		d.CacheHitPct = cacheHitSum / float64(cacheHitCount) * 100
	}

	// Windowed R:E (10-turn sliding window).
	d.WindowRE = windowedRE(turns, 10)

	return d
}

// RenderOverview renders the overview panel.
func RenderOverview(d *OverviewData, width, height int) string {
	var b strings.Builder
	sparkWidth := width/2 - 4
	if sparkWidth < 20 {
		sparkWidth = 20
	}
	if sparkWidth > 60 {
		sparkWidth = 60
	}

	// Session summary.
	b.WriteString(sectionStyle.Render("Session Summary"))
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "  %s %s\n", labelStyle.Render("Turns:"), valueStyle.Render(fmt.Sprintf("%d", d.TotalTurns)))
	fmt.Fprintf(&b, "  %s %s\n", labelStyle.Render("Sessions:"), valueStyle.Render(fmt.Sprintf("%d", d.TotalSessions)))
	fmt.Fprintf(&b, "  %s %s\n", labelStyle.Render("Reads:"), valueStyle.Render(fmt.Sprintf("%d", d.TotalReads)))
	fmt.Fprintf(&b, "  %s %s\n", labelStyle.Render("Edits:"), valueStyle.Render(fmt.Sprintf("%d", d.TotalEdits)))

	// Brain health.
	if d.BrainNodes > 0 {
		fmt.Fprintf(&b, "  %s %s\n", labelStyle.Render("Brain nodes:"), valueStyle.Render(fmt.Sprintf("%d", d.BrainNodes)))
		fmt.Fprintf(&b, "  %s %s\n", labelStyle.Render("Brain edges:"), valueStyle.Render(fmt.Sprintf("%d", d.BrainEdges)))
		if d.BrainColdNodes > 0 {
			fmt.Fprintf(&b, "  %s %s\n", labelStyle.Render("Cold nodes:"), warningStyle.Render(fmt.Sprintf("%d", d.BrainColdNodes)))
		}
	}

	b.WriteString("\n")
	b.WriteString(sectionStyle.Render("Quality Signals"))
	b.WriteString("\n\n")

	// Sparklines.
	fmt.Fprintf(&b, "  %s %s\n", labelStyle.Render("R:E (turn):"), metrics.Sparkline(d.PerTurnRE, sparkWidth))
	fmt.Fprintf(&b, "  %s %s\n", labelStyle.Render("R:E (w10):"), metrics.Sparkline(d.WindowRE, sparkWidth))
	fmt.Fprintf(&b, "  %s %s\n", labelStyle.Render("Thinking:"), metrics.BoolSparkline(d.ThinkBlocks, sparkWidth, '^'))
	if d.HasThinkDepth {
		fmt.Fprintf(&b, "  %s %s\n", labelStyle.Render("Think Depth:"), metrics.Sparkline(d.ThinkDepth, sparkWidth))
	}
	fmt.Fprintf(&b, "  %s %s\n", labelStyle.Render("Cache Hit:"), metrics.Sparkline(d.CacheHit, sparkWidth))
	fmt.Fprintf(&b, "  %s %s\n", labelStyle.Render("Stop Guard:"), metrics.BoolSparkline(d.StopGuard, sparkWidth, '*'))

	// Summary line.
	b.WriteString("\n")
	thinkLabel := fmt.Sprintf("%.0f%% active", d.ThinkPct)
	if d.ThinkRedacted {
		thinkLabel += " (redacted)"
	}
	fmt.Fprintf(&b, "  R:E: %.1f  Think: %s  Cache: %.0f%%  Stops: %d\n",
		d.AvgRE, thinkLabel, d.CacheHitPct, d.StopsBlocked)

	return b.String()
}

// windowedRE computes a 10-turn sliding window R:E ratio.
func windowedRE(turns []metrics.TurnMetrics, window int) []float64 {
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
