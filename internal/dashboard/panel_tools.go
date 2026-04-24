package dashboard

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/coetzeevs/cerebro/internal/metrics"
)

// ToolsData holds computed data for the tools panel.
type ToolsData struct {
	Distribution []toolEntry // sorted by count descending, MCP grouped, capped
	Total        int
	CerebroOps   map[string]int // cerebro operation breakdown
}

type toolEntry struct {
	Name  string
	Count int
}

const maxToolRows = 12

// ComputeToolsData computes tool distribution from turns and a distribution map.
// MCP tools are grouped by provider (e.g., mcp__claude_ai_Atlassian_Rovo__* → "MCP: Atlassian Rovo").
// Low-count tools beyond the top entries are bucketed into "Other".
func ComputeToolsData(turns []metrics.TurnMetrics, dist map[string]int) ToolsData {
	d := ToolsData{
		CerebroOps: make(map[string]int),
	}

	// Group MCP tools by provider, keep others as-is.
	grouped := make(map[string]int)
	for name, count := range dist {
		groupName := groupToolName(name)
		grouped[groupName] += count
		d.Total += count
	}

	// Convert to sorted slice.
	var entries []toolEntry
	for name, count := range grouped {
		entries = append(entries, toolEntry{Name: name, Count: count})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Count > entries[j].Count
	})

	// Cap at maxToolRows, bucket the rest into "Other".
	if len(entries) > maxToolRows {
		otherCount := 0
		for _, e := range entries[maxToolRows-1:] {
			otherCount += e.Count
		}
		entries = entries[:maxToolRows-1]
		entries = append(entries, toolEntry{Name: "Other", Count: otherCount})
	}

	d.Distribution = entries

	// Cerebro ops from turns.
	for i := range turns {
		d.CerebroOps["recalls"] += turns[i].CerebroRecalls
		d.CerebroOps["writes"] += turns[i].CerebroWrites
	}

	return d
}

// groupToolName groups MCP tool names by provider prefix.
// "mcp__claude_ai_Atlassian_Rovo__editJiraIssue" → "MCP: Atlassian Rovo"
// "mcp__plugin_context7_context7__query-docs" → "MCP: context7"
// Non-MCP tools are returned as-is.
func groupToolName(name string) string {
	if !strings.HasPrefix(name, "mcp__") {
		return name
	}

	// Strip "mcp__" prefix.
	rest := name[5:]

	// Find the double-underscore separator before the method name.
	methodSep := strings.Index(rest, "__")
	if methodSep < 0 {
		return name // malformed, return as-is
	}

	providerPart := rest[:methodSep]

	// The provider part is like "claude_ai_Atlassian_Rovo" or "plugin_context7_context7".
	// Extract a readable name: strip common prefixes and deduplicate.
	providerPart = strings.TrimPrefix(providerPart, "claude_ai_")
	providerPart = strings.TrimPrefix(providerPart, "plugin_")

	// Deduplicate repeated segments (e.g., "context7_context7" → "context7").
	parts := strings.Split(providerPart, "_")
	seen := make(map[string]bool)
	var deduped []string
	for _, p := range parts {
		lower := strings.ToLower(p)
		if !seen[lower] {
			seen[lower] = true
			deduped = append(deduped, p)
		}
	}

	return "MCP: " + strings.Join(deduped, " ")
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

	// Find the longest tool name for alignment.
	maxNameLen := 0
	for _, entry := range d.Distribution {
		if len(entry.Name) > maxNameLen {
			maxNameLen = len(entry.Name)
		}
	}
	if maxNameLen < 10 {
		maxNameLen = 10
	}

	toolLabel := lipgloss.NewStyle().Foreground(colorMuted).Width(maxNameLen)

	for _, entry := range d.Distribution {
		barLen := int(float64(entry.Count) / float64(maxCount) * float64(barWidth))
		if barLen < 1 && entry.Count > 0 {
			barLen = 1
		}
		bar := strings.Repeat("█", barLen)
		pct := float64(entry.Count) / float64(d.Total) * 100

		fmt.Fprintf(&b, "  %s %s %d (%.0f%%)\n",
			toolLabel.Render(entry.Name), valueStyle.Render(bar), entry.Count, pct)
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
