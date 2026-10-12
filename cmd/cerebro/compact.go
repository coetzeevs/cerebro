package main

// compact.go — the budgeted compact index (agentic-thnd, ADR-018).
//
// Claude Code caps each hook additionalContext string and plain hook stdout at
// 10,000 characters; over the cap it substitutes a file path plus a 2,000-char
// preview that Claude is never asked to read
// (https://code.claude.com/docs/en/hooks, "Size limits on hook output"). The
// session-start prime therefore renders as a framed one-line-per-memory index
// under a hard UTF-8 byte budget, and the renderer returns the exact ids it
// printed so callers stamp last_surfaced / access telemetry only for those.

import (
	"github.com/coetzeevs/cerebro/internal/store"
)

const (
	compactMinHeadline   = 40   // runes
	compactMaxHeadline   = 160  // runes
	compactHeadlineScan  = 4096 // bytes of content inspected for the headline
	gcNoticeMax          = 320  // bytes
	defaultCompactBudget = 9500 // bytes; registry default for prime_budget and recall_budget
	minCompactBudget     = 2000
	maxCompactBudget     = 10000 // Claude Code hook cap (characters); bytes >= characters
	compactFrameFooter   = "</cerebro-memories>"
)

// compactRecord is one candidate line of the compact index.
type compactRecord struct {
	ID      string // full UUID; rendered as ID[:8]
	Type    store.NodeType
	Metric  string // preformatted: "%.2f" importance (prime) or "%.3f" score (query)
	Content string // raw stored content; only compactHeadline reads it
}

func compactFrameHeader(metric, source string) string { return "" }

func compactHeadline(content string, width int) string { return "" }

func renderCompactIndex(banner func(k, n int) string, header string, recs []compactRecord,
	budget int, trailer string) (out string, delivered []string) {
	return "", nil
}
