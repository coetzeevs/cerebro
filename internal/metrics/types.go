// Package metrics implements the observability metrics store for Cerebro.
// It tracks per-turn session metrics, tool call details, and daily aggregates
// in a separate SQLite database from the brain.
package metrics

// TurnMetrics represents aggregated metrics for a single user-turn.
type TurnMetrics struct {
	ID                int64    `json:"id,omitempty"`
	SessionID         string   `json:"session_id"`
	TurnNumber        int      `json:"turn_number"`
	Timestamp         string   `json:"timestamp"`
	InputTokens       int      `json:"input_tokens"`
	OutputTokens      int      `json:"output_tokens"`
	CacheReadTokens   int      `json:"cache_read_tokens"`
	CacheCreateTokens int      `json:"cache_create_tokens"`
	ThinkingChars     int      `json:"thinking_chars"`
	ThinkingBlocks    int      `json:"thinking_blocks"`
	ToolCallsTotal    int      `json:"tool_calls_total"`
	ToolReads         int      `json:"tool_reads"`
	ToolEdits         int      `json:"tool_edits"`
	ToolBash          int      `json:"tool_bash"`
	ToolOther         int      `json:"tool_other"`
	CerebroRecalls    int      `json:"cerebro_recalls"`
	CerebroWrites     int      `json:"cerebro_writes"`
	ReadEditRatio     *float64 `json:"read_edit_ratio"`
	AssistantMsgs     int      `json:"assistant_messages"`
	StopGuardFired    int      `json:"stop_guard_fired"`
}

// ToolCall represents a single tool invocation within a turn.
type ToolCall struct {
	ID         int64   `json:"id,omitempty"`
	SessionID  string  `json:"session_id"`
	TurnNumber int     `json:"turn_number"`
	Timestamp  string  `json:"timestamp"`
	ToolName   string  `json:"tool_name"`
	FilePath   *string `json:"file_path,omitempty"`
	CerebroOp  *string `json:"cerebro_op,omitempty"`
}

// DailySummary is the aggregated view for one calendar day.
type DailySummary struct {
	Date             string   `json:"date"`
	TurnCount        int      `json:"turn_count"`
	ZeroThinking     int      `json:"zero_thinking"`
	AvgThinking      *float64 `json:"avg_thinking"`
	AvgReadEdit      *float64 `json:"avg_read_edit"`
	MinReadEdit      *float64 `json:"min_read_edit"`
	StopGuardFires   int      `json:"stop_guard_fires"`
	TotalReads       int      `json:"total_reads"`
	TotalEdits       int      `json:"total_edits"`
	CerebroRecalls   int      `json:"cerebro_recalls"`
	CerebroWrites    int      `json:"cerebro_writes"`
	ToolDistribution string   `json:"tool_distribution,omitempty"`
	ActiveNodes      *int     `json:"active_nodes,omitempty"`
	TotalEdges       *int     `json:"total_edges,omitempty"`
	NodesByType      string   `json:"nodes_by_type,omitempty"`
}

// IngestState tracks incremental parsing progress for a JSONL file.
type IngestState struct {
	FilePath   string `json:"file_path"`
	LastOffset int64  `json:"last_offset"`
}

// OrderField selects which column to sort turn queries by.
type OrderField int

const (
	// OrderByTimestamp sorts chronologically (default zero value).
	OrderByTimestamp OrderField = iota
	// OrderByTurnNumber sorts by per-session turn number.
	OrderByTurnNumber
)

// TurnFilter specifies query parameters for turn retrieval.
type TurnFilter struct {
	SessionID string
	Since     string // ISO 8601 timestamp
	Until     string // ISO 8601 timestamp
	Limit     int
	OrderBy   OrderField // default (zero) = timestamp
	OrderDesc bool
}
