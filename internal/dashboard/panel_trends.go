package dashboard

import (
	"fmt"
	"strings"

	"github.com/coetzeevs/cerebro/internal/metrics"
)

// TrendsData holds computed daily aggregates for the trends panel.
type TrendsData struct {
	Days []dayData
}

type dayData struct {
	Date           string
	TurnCount      int
	AvgRE          float64
	ZeroThinkPct   float64
	CacheHitPct    float64
	StopGuardFires int
	TotalReads     int
	TotalEdits     int
}

// ComputeTrendsData aggregates turn metrics into daily buckets.
func ComputeTrendsData(turns []metrics.TurnMetrics) TrendsData {
	// Group by date (YYYY-MM-DD from timestamp).
	dayMap := make(map[string]*dayData)
	var dayOrder []string

	for i := range turns {
		t := &turns[i]
		date := ""
		if len(t.Timestamp) >= 10 {
			date = t.Timestamp[:10]
		}
		if date == "" {
			continue
		}

		d, exists := dayMap[date]
		if !exists {
			d = &dayData{Date: date}
			dayMap[date] = d
			dayOrder = append(dayOrder, date)
		}

		d.TurnCount++
		d.TotalReads += t.ToolReads
		d.TotalEdits += t.ToolEdits
		d.StopGuardFires += t.StopGuardFired

		if t.ThinkingBlocks == 0 {
			d.ZeroThinkPct++ // count, convert to pct later
		}

		total := t.InputTokens + t.CacheReadTokens + t.CacheCreateTokens
		if total > 0 {
			d.CacheHitPct += float64(t.CacheReadTokens) / float64(total)
		}
	}

	// Compute averages.
	var result TrendsData
	for _, date := range dayOrder {
		d := dayMap[date]
		if d.TotalEdits > 0 {
			d.AvgRE = float64(d.TotalReads) / float64(d.TotalEdits)
		}
		d.ZeroThinkPct = d.ZeroThinkPct / float64(d.TurnCount) * 100
		d.CacheHitPct = d.CacheHitPct / float64(d.TurnCount) * 100
		result.Days = append(result.Days, *d)
	}

	return result
}

// RenderTrends renders the trends panel with daily sparklines.
func RenderTrends(d *TrendsData, width, height int) string {
	var b strings.Builder

	if len(d.Days) == 0 {
		b.WriteString(contentStyle.Render("\n  No trend data. Run 'cerebro ingest' to collect metrics.\n"))
		return b.String()
	}

	sparkWidth := width - 30
	if sparkWidth < 20 {
		sparkWidth = 20
	}
	if sparkWidth > 60 {
		sparkWidth = 60
	}

	// Extract daily time series.
	re := make([]float64, len(d.Days))
	zeroThink := make([]float64, len(d.Days))
	cacheHit := make([]float64, len(d.Days))
	stops := make([]bool, len(d.Days))
	turnCounts := make([]float64, len(d.Days))

	for i := range d.Days {
		re[i] = d.Days[i].AvgRE
		zeroThink[i] = d.Days[i].ZeroThinkPct
		cacheHit[i] = d.Days[i].CacheHitPct
		stops[i] = d.Days[i].StopGuardFires > 0
		turnCounts[i] = float64(d.Days[i].TurnCount)
	}

	// Date range label.
	first := d.Days[0].Date
	last := d.Days[len(d.Days)-1].Date
	if len(first) > 5 {
		first = first[5:] // MM-DD
	}
	if len(last) > 5 {
		last = last[5:]
	}

	fmt.Fprintf(&b, "%s\n\n", sectionStyle.Render(fmt.Sprintf("Trends (%s to %s, %d days)", first, last, len(d.Days))))

	fmt.Fprintf(&b, "  %s %s\n", labelStyle.Render("R:E (daily):"), metrics.Sparkline(re, sparkWidth))
	fmt.Fprintf(&b, "  %s %s\n", labelStyle.Render("Zero-think %%:"), metrics.Sparkline(zeroThink, sparkWidth))
	fmt.Fprintf(&b, "  %s %s\n", labelStyle.Render("Cache hit %%:"), metrics.Sparkline(cacheHit, sparkWidth))
	fmt.Fprintf(&b, "  %s %s\n", labelStyle.Render("Stop guard:"), metrics.BoolSparkline(stops, sparkWidth, '*'))
	fmt.Fprintf(&b, "  %s %s\n", labelStyle.Render("Turn volume:"), metrics.Sparkline(turnCounts, sparkWidth))

	b.WriteString("\n")

	// Daily table.
	fmt.Fprintf(&b, "  %s\n\n", sectionStyle.Render("Daily Summary"))
	fmt.Fprintf(&b, "  %-12s %6s %6s %8s %6s %5s\n",
		labelStyle.Render("Date"), "Turns", "R:E", "0-Think", "Cache", "Stops")

	limit := len(d.Days)
	if limit > 14 {
		limit = 14 // show at most 14 days
	}
	// Show most recent first.
	for i := len(d.Days) - 1; i >= len(d.Days)-limit; i-- {
		day := &d.Days[i]
		date := day.Date
		if len(date) > 5 {
			date = date[5:]
		}
		fmt.Fprintf(&b, "  %-12s %6d %6.1f %7.0f%% %5.0f%% %5d\n",
			date, day.TurnCount, day.AvgRE, day.ZeroThinkPct, day.CacheHitPct, day.StopGuardFires)
	}

	return b.String()
}
