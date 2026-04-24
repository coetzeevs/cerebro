package dashboard

import (
	"fmt"
	"sort"
	"strings"

	"github.com/coetzeevs/cerebro/internal/metrics"
)

// ToolsData holds computed data for the tools panel.
type ToolsData struct {
	Distribution []toolEntry // sorted by count descending
	Total        int
	CerebroOps   map[string]int // cerebro operation breakdown
}

type toolEntry struct {
	Name  string
	Count int
}

// ComputeToolsData computes tool distribution from turns and a distribution map.
func ComputeToolsData(turns []metrics.TurnMetrics, dist map[string]int) ToolsData {
	d := ToolsData{
		CerebroOps: make(map[string]int),
	}

	// Tool distribution.
	for name, count := range dist {
		d.Distribution = append(d.Distribution, toolEntry{Name: name, Count: count})
		d.Total += count
	}
	sort.Slice(d.Distribution, func(i, j int) bool {
		return d.Distribution[i].Count > d.Distribution[j].Count
	})

	// Cerebro ops from turns.
	for i := range turns {
		d.CerebroOps["recalls"] += turns[i].CerebroRecalls
		d.CerebroOps["writes"] += turns[i].CerebroWrites
	}

	return d
}

// RenderTools renders the tools distribution panel.
func RenderTools(d *ToolsData, width, height int) string {
	var b strings.Builder

	fmt.Fprintf(&b, "%s\n\n", sectionStyle.Render("Tool Distribution"))

	if len(d.Distribution) == 0 {
		b.WriteString("  No tool call data.\n")
		return b.String()
	}

	// Bar chart — proportional to the max count.
	maxCount := d.Distribution[0].Count
	barWidth := width/2 - 20
	if barWidth < 10 {
		barWidth = 10
	}
	if barWidth > 40 {
		barWidth = 40
	}

	for _, entry := range d.Distribution {
		barLen := int(float64(entry.Count) / float64(maxCount) * float64(barWidth))
		if barLen < 1 && entry.Count > 0 {
			barLen = 1
		}
		bar := strings.Repeat("█", barLen)
		pct := float64(entry.Count) / float64(d.Total) * 100

		fmt.Fprintf(&b, "  %-14s %s %d (%.0f%%)\n",
			labelStyle.Render(entry.Name), valueStyle.Render(bar), entry.Count, pct)
	}

	b.WriteString("\n")
	fmt.Fprintf(&b, "  %s %s\n\n", labelStyle.Render("Total calls:"), valueStyle.Render(fmt.Sprintf("%d", d.Total)))

	// Cerebro operations.
	if d.CerebroOps["recalls"] > 0 || d.CerebroOps["writes"] > 0 {
		fmt.Fprintf(&b, "%s\n\n", sectionStyle.Render("Cerebro Operations"))
		fmt.Fprintf(&b, "  Recalls: %d  Writes: %d\n",
			d.CerebroOps["recalls"], d.CerebroOps["writes"])
	}

	return b.String()
}
