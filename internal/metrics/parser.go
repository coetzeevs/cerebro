package metrics

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
)

// ParseResult contains the extracted metrics from a JSONL file.
type ParseResult struct {
	Turns     []TurnMetrics
	ToolCalls []ToolCall
}

// SessionIDs returns the distinct session IDs found in the parsed turns.
func (r *ParseResult) SessionIDs() []string {
	seen := make(map[string]bool)
	var ids []string
	for i := range r.Turns {
		if !seen[r.Turns[i].SessionID] {
			seen[r.Turns[i].SessionID] = true
			ids = append(ids, r.Turns[i].SessionID)
		}
	}
	return ids
}

// ParseJSONL reads a JSONL file from the beginning and returns extracted
// turn metrics, tool call details, and the file size (for change detection).
// Always parses from byte 0 to ensure correct turn numbering.
func ParseJSONL(filePath string) (ParseResult, int64, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return ParseResult{}, 0, fmt.Errorf("opening %s: %w", filePath, err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	// Increase buffer for large JSONL lines (some can be several KB).
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)

	var (
		currentTurn *turnBuilder
		turnNumber  int
		result      ParseResult
	)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var msg jsonlMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			continue // skip malformed lines
		}

		// Only process main-chain messages.
		if msg.IsSidechain {
			continue
		}

		switch msg.Type {
		case "user":
			if msg.IsMeta {
				continue // skip system injections
			}
			// New turn starts.
			if currentTurn != nil {
				result.Turns = append(result.Turns, currentTurn.finalize())
				result.ToolCalls = append(result.ToolCalls, currentTurn.toolCalls...)
			}
			turnNumber++
			currentTurn = &turnBuilder{
				sessionID:  msg.SessionID,
				turnNumber: turnNumber,
				timestamp:  msg.Timestamp,
			}

		case "assistant":
			if currentTurn == nil {
				continue // assistant before any user message (hook output, etc.)
			}
			currentTurn.assistantMsgs++
			currentTurn.processAssistantMessage(&msg)
		}
	}

	// Flush the last turn.
	if currentTurn != nil {
		result.Turns = append(result.Turns, currentTurn.finalize())
		result.ToolCalls = append(result.ToolCalls, currentTurn.toolCalls...)
	}

	// Return file size as the offset (used for change detection by the caller).
	info, err := f.Stat()
	if err != nil {
		return result, 0, fmt.Errorf("stat %s: %w", filePath, err)
	}

	return result, info.Size(), scanner.Err()
}

// --- Internal types for JSONL parsing ---

type jsonlMessage struct {
	Type        string          `json:"type"`
	UUID        string          `json:"uuid"`
	ParentUUID  string          `json:"parentUuid"`
	Timestamp   string          `json:"timestamp"`
	SessionID   string          `json:"sessionId"`
	IsSidechain bool            `json:"isSidechain"`
	IsMeta      bool            `json:"isMeta"`
	Message     json.RawMessage `json:"message"` // Defer parsing — user vs assistant have different schemas
}

type assistantMsg struct {
	Role       string         `json:"role"`
	Model      string         `json:"model"`
	StopReason string         `json:"stop_reason"`
	Content    []contentBlock `json:"content"`
	Usage      *usageData     `json:"usage"`
}

type contentBlock struct {
	Type     string          `json:"type"`
	Name     string          `json:"name"`     // for tool_use
	Input    json.RawMessage `json:"input"`    // for tool_use
	Thinking string          `json:"thinking"` // for thinking blocks
}

type usageData struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// --- Turn builder ---

type turnBuilder struct {
	sessionID     string
	turnNumber    int
	timestamp     string
	assistantMsgs int
	inputTokens   int
	outputTokens  int
	cacheRead     int
	cacheCreate   int
	thinkingChars int
	thinkingBlks  int
	reads         int
	edits         int
	bash          int
	other         int
	cerebroRecall int
	cerebroWrite  int
	toolCalls     []ToolCall
}

func (tb *turnBuilder) processAssistantMessage(msg *jsonlMessage) {
	if len(msg.Message) == 0 {
		return
	}

	var aMsg assistantMsg
	if err := json.Unmarshal(msg.Message, &aMsg); err != nil {
		return // not a parseable assistant message
	}

	// Accumulate token usage.
	if aMsg.Usage != nil {
		tb.inputTokens += aMsg.Usage.InputTokens
		tb.outputTokens += aMsg.Usage.OutputTokens
		tb.cacheRead += aMsg.Usage.CacheReadInputTokens
		tb.cacheCreate += aMsg.Usage.CacheCreationInputTokens
	}

	// Process content blocks.
	for i := range aMsg.Content {
		block := &aMsg.Content[i]
		switch block.Type {
		case "thinking":
			tb.thinkingChars += len(block.Thinking)
			tb.thinkingBlks++

		case "tool_use":
			tc := ToolCall{
				SessionID:  tb.sessionID,
				TurnNumber: tb.turnNumber,
				Timestamp:  msg.Timestamp,
				ToolName:   block.Name,
			}

			// Extract file path from tool input.
			filePath := extractFilePath(block.Input)
			if filePath != "" {
				tc.FilePath = &filePath
			}

			// Classify tool and extract cerebro op.
			cat := classifyTool(block.Name)
			switch cat {
			case toolRead:
				tb.reads++
			case toolEdit:
				tb.edits++
			case toolBash:
				tb.bash++
				cmd := extractBashCommand(block.Input)
				if op := extractCerebroOp(cmd); op != "" {
					tc.CerebroOp = &op
					if isCerebroRecall(op) {
						tb.cerebroRecall++
					} else {
						tb.cerebroWrite++
					}
				}
			default:
				tb.other++
			}

			tb.toolCalls = append(tb.toolCalls, tc)
		}
	}
}

func (tb *turnBuilder) finalize() TurnMetrics {
	total := tb.reads + tb.edits + tb.bash + tb.other
	var ratio *float64
	if tb.edits > 0 {
		r := float64(tb.reads) / float64(tb.edits)
		ratio = &r
	}

	return TurnMetrics{
		SessionID:         tb.sessionID,
		TurnNumber:        tb.turnNumber,
		Timestamp:         tb.timestamp,
		InputTokens:       tb.inputTokens,
		OutputTokens:      tb.outputTokens,
		CacheReadTokens:   tb.cacheRead,
		CacheCreateTokens: tb.cacheCreate,
		ThinkingChars:     tb.thinkingChars,
		ThinkingBlocks:    tb.thinkingBlks,
		ToolCallsTotal:    total,
		ToolReads:         tb.reads,
		ToolEdits:         tb.edits,
		ToolBash:          tb.bash,
		ToolOther:         tb.other,
		CerebroRecalls:    tb.cerebroRecall,
		CerebroWrites:     tb.cerebroWrite,
		ReadEditRatio:     ratio,
		AssistantMsgs:     tb.assistantMsgs,
	}
}

// --- Tool classification ---

type toolCategory int

const (
	toolRead toolCategory = iota
	toolEdit
	toolBash
	toolOther
)

func classifyTool(name string) toolCategory {
	switch name {
	case "Read", "Grep", "Glob":
		return toolRead
	case "Edit", "Write":
		return toolEdit
	case "Bash":
		return toolBash
	default:
		return toolOther
	}
}

// --- Input extraction ---

type toolInput struct {
	FilePath string `json:"file_path"`
	Command  string `json:"command"`
	Pattern  string `json:"pattern"`
	Path     string `json:"path"`
}

func extractFilePath(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var inp toolInput
	if err := json.Unmarshal(raw, &inp); err != nil {
		return ""
	}
	if inp.FilePath != "" {
		return inp.FilePath
	}
	return ""
}

func extractBashCommand(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var inp toolInput
	if err := json.Unmarshal(raw, &inp); err != nil {
		return ""
	}
	return inp.Command
}

// cerebroOpPattern matches cerebro CLI invocations in Bash commands.
var cerebroOpPattern = regexp.MustCompile(`(?:^|/)cerebro\s+(add|recall|search|update|supersede|gc|promote|edge|reinforce|list|get|export|import)\b`)

func extractCerebroOp(command string) string {
	m := cerebroOpPattern.FindStringSubmatch(command)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func isCerebroRecall(op string) bool {
	return op == "recall" || op == "search" || op == "list" || op == "get"
}

// writeTestFile is a helper for tests to create temporary files.
func writeTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644) //nolint:gosec // test helper
}
