package dashboard

import (
	"fmt"

	"charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"
	"github.com/coetzeevs/cerebro/internal/metrics"
)

// BuildTurnsTable creates a bubbles table from turn metrics data.
func BuildTurnsTable(turns []metrics.TurnMetrics, width, height int) table.Model {
	columns := []table.Column{
		{Title: "#", Width: 5},
		{Title: "Time", Width: 10},
		{Title: "Session", Width: 10},
		{Title: "In Tok", Width: 8},
		{Title: "Out Tok", Width: 8},
		{Title: "Think", Width: 6},
		{Title: "R:E", Width: 5},
		{Title: "Reads", Width: 6},
		{Title: "Edits", Width: 6},
		{Title: "Tools", Width: 6},
	}

	rows := make([]table.Row, len(turns))
	for i := range turns {
		t := &turns[i]

		// Format timestamp to HH:MM:SS.
		timeStr := ""
		if len(t.Timestamp) >= 19 {
			timeStr = t.Timestamp[11:19] // Extract HH:MM:SS from ISO 8601
		}

		// Session ID prefix.
		sid := t.SessionID
		if len(sid) > 8 {
			sid = sid[:8]
		}

		// Thinking indicator.
		thinkStr := "-"
		if t.ThinkingBlocks > 0 {
			if t.ThinkingChars > 0 {
				thinkStr = fmt.Sprintf("%d", t.ThinkingChars)
			} else {
				thinkStr = "^" // thinking happened but redacted
			}
		}

		// R:E ratio.
		reStr := "-"
		if t.ReadEditRatio != nil {
			reStr = fmt.Sprintf("%.1f", *t.ReadEditRatio)
		}

		rows[i] = table.Row{
			fmt.Sprintf("%d", t.TurnNumber),
			timeStr,
			sid,
			formatTokens(t.InputTokens),
			formatTokens(t.OutputTokens),
			thinkStr,
			reStr,
			fmt.Sprintf("%d", t.ToolReads),
			fmt.Sprintf("%d", t.ToolEdits),
			fmt.Sprintf("%d", t.ToolCallsTotal),
		}
	}

	tableHeight := height - 6
	if tableHeight < 5 {
		tableHeight = 5
	}

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(colorPrimary).
		BorderBottom(true).
		Bold(true)
	s.Selected = s.Selected.
		Foreground(colorHighlight).
		Background(colorPrimary).
		Bold(false)

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithWidth(width),
		table.WithHeight(tableHeight),
	)
	t.SetStyles(s)

	return t
}

// formatTokens formats token counts for compact display.
func formatTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}
