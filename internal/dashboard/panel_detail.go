package dashboard

import (
	"fmt"
	"strings"

	"github.com/coetzeevs/cerebro/internal/metrics"
)

// RenderDetail renders the detail panel for a selected turn.
func RenderDetail(turn *metrics.TurnMetrics, toolCalls []metrics.ToolCall, width, height int) string {
	if turn == nil {
		return contentStyle.Render("\n  Select a turn from the Turns panel (2) and press Enter.\n")
	}

	var b strings.Builder

	// Header.
	fmt.Fprintf(&b, "%s\n\n", sectionStyle.Render(fmt.Sprintf("Turn %d Detail", turn.TurnNumber)))

	// Session + time.
	sid := turn.SessionID
	if len(sid) > 12 {
		sid = sid[:12]
	}
	fmt.Fprintf(&b, "  %s %s\n", labelStyle.Render("Session:"), sid)
	fmt.Fprintf(&b, "  %s %s\n\n", labelStyle.Render("Timestamp:"), turn.Timestamp)

	// Token breakdown.
	fmt.Fprintf(&b, "  %s\n", sectionStyle.Render("Tokens"))
	fmt.Fprintf(&b, "    Input:        %s\n", valueStyle.Render(formatTokens(turn.InputTokens)))
	fmt.Fprintf(&b, "    Output:       %s\n", valueStyle.Render(formatTokens(turn.OutputTokens)))
	fmt.Fprintf(&b, "    Cache read:   %s\n", valueStyle.Render(formatTokens(turn.CacheReadTokens)))
	fmt.Fprintf(&b, "    Cache create: %s\n\n", valueStyle.Render(formatTokens(turn.CacheCreateTokens)))

	// Thinking.
	fmt.Fprintf(&b, "  %s\n", sectionStyle.Render("Thinking"))
	fmt.Fprintf(&b, "    Blocks: %d  Chars: %d\n\n", turn.ThinkingBlocks, turn.ThinkingChars)

	// Tool summary.
	fmt.Fprintf(&b, "  %s\n", sectionStyle.Render("Tools"))
	fmt.Fprintf(&b, "    Reads: %d  Edits: %d  Bash: %d  Other: %d  Total: %d\n",
		turn.ToolReads, turn.ToolEdits, turn.ToolBash, turn.ToolOther, turn.ToolCallsTotal)

	if turn.ReadEditRatio != nil {
		fmt.Fprintf(&b, "    R:E Ratio: %.1f\n", *turn.ReadEditRatio)
	}
	if turn.CerebroRecalls > 0 || turn.CerebroWrites > 0 {
		fmt.Fprintf(&b, "    Cerebro: %d recalls, %d writes\n", turn.CerebroRecalls, turn.CerebroWrites)
	}
	b.WriteString("\n")

	// Quality flags.
	fmt.Fprintf(&b, "  %s\n", sectionStyle.Render("Quality Flags"))
	if turn.ThinkingBlocks == 0 {
		fmt.Fprintf(&b, "    %s ZERO THINKING\n", warningStyle.Render("[!]"))
	}
	if turn.ToolEdits > 0 && turn.ToolReads == 0 {
		fmt.Fprintf(&b, "    %s EDIT WITHOUT READS\n", warningStyle.Render("[!]"))
	}
	if turn.StopGuardFired > 0 {
		fmt.Fprintf(&b, "    %s STOP GUARD FIRED\n", warningStyle.Render("[!]"))
	}
	if turn.ThinkingBlocks > 0 && turn.ThinkingChars == 0 {
		fmt.Fprintf(&b, "    %s Thinking redacted (blocks present, content empty)\n", helpStyle.Render("[i]"))
	}
	if turn.ThinkingBlocks > 0 && turn.ToolEdits == 0 && turn.ToolReads > 0 {
		b.WriteString("    [ ] Research turn (reads only, no edits)\n")
	}
	b.WriteString("\n")

	// Individual tool calls.
	if len(toolCalls) > 0 {
		fmt.Fprintf(&b, "  %s\n", sectionStyle.Render("Tool Calls"))
		limit := len(toolCalls)
		if limit > 15 {
			limit = 15
		}
		for i := 0; i < limit; i++ {
			tc := &toolCalls[i]
			detail := tc.ToolName
			if tc.FilePath != nil {
				// Show just the filename, not full path.
				path := *tc.FilePath
				if idx := strings.LastIndex(path, "/"); idx >= 0 {
					path = path[idx+1:]
				}
				detail += " " + helpStyle.Render(path)
			}
			if tc.CerebroOp != nil {
				detail += " " + helpStyle.Render("("+*tc.CerebroOp+")")
			}
			fmt.Fprintf(&b, "    %s\n", detail)
		}
		if len(toolCalls) > 15 {
			fmt.Fprintf(&b, "    ... and %d more\n", len(toolCalls)-15)
		}
	}

	return b.String()
}
