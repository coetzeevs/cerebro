package metrics

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func testdataPath(t *testing.T, name string) string {
	t.Helper()
	// Try runtime.Caller first (works in source tree).
	_, filename, _, ok := runtime.Caller(0)
	if ok {
		p := filepath.Join(filepath.Dir(filename), "testdata", name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	// Fallback: relative to working directory.
	p := filepath.Join("testdata", name)
	if _, err := os.Stat(p); err == nil {
		return p
	}
	t.Fatalf("testdata file not found: %s", name)
	return ""
}

func TestParseJSONL_SimpleSession(t *testing.T) {
	path := testdataPath(t, "simple_session.jsonl")
	result, _, err := ParseJSONL(path)
	if err != nil {
		t.Fatalf("ParseJSONL: %v", err)
	}

	// 3 user messages = 3 turns.
	if len(result.Turns) != 3 {
		t.Fatalf("expected 3 turns, got %d", len(result.Turns))
	}

	// Turn 1: Read + Grep + Edit = 3 tool calls, 2 reads (Read+Grep), 1 edit.
	turn1 := result.Turns[0]
	if turn1.TurnNumber != 1 {
		t.Errorf("turn1 number = %d, want 1", turn1.TurnNumber)
	}
	if turn1.SessionID != "test-session-1" {
		t.Errorf("turn1 session = %q, want test-session-1", turn1.SessionID)
	}
	if turn1.ToolCallsTotal != 3 {
		t.Errorf("turn1 tool_calls_total = %d, want 3", turn1.ToolCallsTotal)
	}
	if turn1.ToolReads != 2 {
		t.Errorf("turn1 tool_reads = %d, want 2 (Read + Grep)", turn1.ToolReads)
	}
	if turn1.ToolEdits != 1 {
		t.Errorf("turn1 tool_edits = %d, want 1", turn1.ToolEdits)
	}
	if turn1.ThinkingChars == 0 {
		t.Error("turn1 thinking_chars should be > 0 (has thinking block)")
	}
	if turn1.ThinkingBlocks != 1 {
		t.Errorf("turn1 thinking_blocks = %d, want 1", turn1.ThinkingBlocks)
	}
	if turn1.AssistantMsgs != 2 {
		t.Errorf("turn1 assistant_messages = %d, want 2", turn1.AssistantMsgs)
	}
	// Tokens should be sum of both assistant messages.
	if turn1.InputTokens != 11000 {
		t.Errorf("turn1 input_tokens = %d, want 11000", turn1.InputTokens)
	}

	// Turn 1 read:edit ratio = 2/1 = 2.0
	if turn1.ReadEditRatio == nil || *turn1.ReadEditRatio != 2.0 {
		t.Errorf("turn1 read_edit_ratio = %v, want 2.0", turn1.ReadEditRatio)
	}

	// Turn 2: Bash only.
	turn2 := result.Turns[1]
	if turn2.ToolBash != 1 {
		t.Errorf("turn2 tool_bash = %d, want 1", turn2.ToolBash)
	}
	if turn2.ReadEditRatio != nil {
		t.Errorf("turn2 read_edit_ratio should be nil (no edits), got %v", turn2.ReadEditRatio)
	}

	// Turn 3: cerebro add + cerebro recall.
	turn3 := result.Turns[2]
	if turn3.CerebroWrites != 1 {
		t.Errorf("turn3 cerebro_writes = %d, want 1", turn3.CerebroWrites)
	}
	if turn3.CerebroRecalls != 1 {
		t.Errorf("turn3 cerebro_recalls = %d, want 1", turn3.CerebroRecalls)
	}
}

func TestParseJSONL_ToolCalls(t *testing.T) {
	path := testdataPath(t, "simple_session.jsonl")
	result, _, err := ParseJSONL(path)
	if err != nil {
		t.Fatalf("ParseJSONL: %v", err)
	}

	// Total tool calls across all turns: Read, Grep, Edit, Bash, Bash(cerebro add), Bash(cerebro recall) = 6.
	if len(result.ToolCalls) != 6 {
		t.Fatalf("expected 6 tool calls, got %d", len(result.ToolCalls))
	}

	// Verify tool names.
	names := make(map[string]int)
	for _, tc := range result.ToolCalls {
		names[tc.ToolName]++
	}
	if names["Read"] != 1 {
		t.Errorf("expected 1 Read call, got %d", names["Read"])
	}
	if names["Grep"] != 1 {
		t.Errorf("expected 1 Grep call, got %d", names["Grep"])
	}
	if names["Edit"] != 1 {
		t.Errorf("expected 1 Edit call, got %d", names["Edit"])
	}
	if names["Bash"] != 3 {
		t.Errorf("expected 3 Bash calls, got %d", names["Bash"])
	}

	// Check file paths on Read and Edit.
	for _, tc := range result.ToolCalls {
		if tc.ToolName == "Read" && (tc.FilePath == nil || *tc.FilePath != "/src/main.go") {
			t.Errorf("Read call file_path = %v, want /src/main.go", tc.FilePath)
		}
		if tc.ToolName == "Edit" && (tc.FilePath == nil || *tc.FilePath != "/src/main.go") {
			t.Errorf("Edit call file_path = %v, want /src/main.go", tc.FilePath)
		}
	}

	// Check cerebro op extraction.
	var cerebroOps []string
	for _, tc := range result.ToolCalls {
		if tc.CerebroOp != nil {
			cerebroOps = append(cerebroOps, *tc.CerebroOp)
		}
	}
	if len(cerebroOps) != 2 {
		t.Fatalf("expected 2 cerebro ops, got %d", len(cerebroOps))
	}
}

func TestParseJSONL_ExcludesSidechains(t *testing.T) {
	path := testdataPath(t, "sidechain_session.jsonl")
	result, _, err := ParseJSONL(path)
	if err != nil {
		t.Fatalf("ParseJSONL: %v", err)
	}

	// 2 non-sidechain, non-meta user messages = 2 turns.
	if len(result.Turns) != 2 {
		t.Fatalf("expected 2 turns (sidechain + meta excluded), got %d", len(result.Turns))
	}

	// The sidechain Write should not appear in tool calls.
	for _, tc := range result.ToolCalls {
		if tc.ToolName == "Write" {
			t.Error("sidechain Write tool call should have been excluded")
		}
	}
}

func TestParseJSONL_TurnNumbersAreSequential(t *testing.T) {
	path := testdataPath(t, "simple_session.jsonl")
	result, _, err := ParseJSONL(path)
	if err != nil {
		t.Fatalf("ParseJSONL: %v", err)
	}

	for i, turn := range result.Turns {
		expected := i + 1
		if turn.TurnNumber != expected {
			t.Errorf("turn %d has TurnNumber=%d, want %d", i, turn.TurnNumber, expected)
		}
	}
}

func TestParseJSONL_ReturnsFileSize(t *testing.T) {
	path := testdataPath(t, "simple_session.jsonl")
	info, _ := os.Stat(path)

	_, offset, err := ParseJSONL(path)
	if err != nil {
		t.Fatalf("ParseJSONL: %v", err)
	}
	if offset != info.Size() {
		t.Errorf("offset = %d, want file size %d", offset, info.Size())
	}
}

func TestParseJSONL_AppendedData(t *testing.T) {
	// Copy the fixture to a temp file so we can append to it.
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")

	src := testdataPath(t, "simple_session.jsonl")
	srcData, _ := os.ReadFile(src)
	if err := os.WriteFile(path, srcData, 0o600); err != nil { //nolint:gosec // test file in t.TempDir()
		t.Fatal(err)
	}

	// First parse: 3 turns.
	result1, offset1, err := ParseJSONL(path)
	if err != nil {
		t.Fatalf("first parse: %v", err)
	}
	if len(result1.Turns) != 3 {
		t.Fatalf("first parse: expected 3 turns, got %d", len(result1.Turns))
	}

	// Append a new turn (user + assistant) to the file.
	newTurn := `{"type":"user","uuid":"u4","timestamp":"2026-04-23T10:03:00Z","sessionId":"test-session-1","isSidechain":false,"message":{"role":"user","content":"Deploy it"}}` + "\n" +
		`{"type":"assistant","uuid":"a5","parentUuid":"u4","timestamp":"2026-04-23T10:03:05Z","sessionId":"test-session-1","isSidechain":false,"message":{"role":"assistant","model":"claude-opus-4-6","stop_reason":"end_turn","content":[{"type":"tool_use","id":"t7","name":"Bash","input":{"command":"make deploy"}}],"usage":{"input_tokens":9000,"output_tokens":500,"cache_creation_input_tokens":0,"cache_read_input_tokens":8500}}}` + "\n"

	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	_, _ = f.WriteString(newTurn)
	_ = f.Close()

	// Second parse (full re-parse): should now have 4 turns with correct numbering.
	result2, offset2, err := ParseJSONL(path)
	if err != nil {
		t.Fatalf("second parse: %v", err)
	}
	if len(result2.Turns) != 4 {
		t.Fatalf("second parse: expected 4 turns, got %d", len(result2.Turns))
	}

	// Verify turn numbers are sequential 1-4.
	for i, turn := range result2.Turns {
		expected := i + 1
		if turn.TurnNumber != expected {
			t.Errorf("turn %d has TurnNumber=%d, want %d", i, turn.TurnNumber, expected)
		}
	}

	// Offset should be larger than before (file grew).
	if offset2 <= offset1 {
		t.Errorf("offset should have grown: %d <= %d", offset2, offset1)
	}

	// New turn should have correct data.
	turn4 := result2.Turns[3]
	if turn4.ToolBash != 1 {
		t.Errorf("turn4 tool_bash = %d, want 1", turn4.ToolBash)
	}
}

func TestParseResult_SessionIDs(t *testing.T) {
	result := ParseResult{
		Turns: []TurnMetrics{
			{SessionID: "s1", TurnNumber: 1},
			{SessionID: "s1", TurnNumber: 2},
			{SessionID: "s2", TurnNumber: 1},
		},
	}
	ids := result.SessionIDs()
	if len(ids) != 2 {
		t.Fatalf("expected 2 session IDs, got %d", len(ids))
	}
}

func TestParseJSONL_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")
	if err := writeFile(path, ""); err != nil {
		t.Fatal(err)
	}

	result, _, err := ParseJSONL(path)
	if err != nil {
		t.Fatalf("ParseJSONL empty: %v", err)
	}
	if len(result.Turns) != 0 {
		t.Errorf("expected 0 turns from empty file, got %d", len(result.Turns))
	}
}

func TestClassifyTool(t *testing.T) {
	tests := []struct {
		name     string
		category toolCategory
	}{
		{"Read", toolRead},
		{"Grep", toolRead},
		{"Glob", toolRead},
		{"Edit", toolEdit},
		{"Write", toolEdit},
		{"Bash", toolBash},
		{"Agent", toolOther},
		{"TaskCreate", toolOther},
		{"Unknown", toolOther},
	}

	for _, tt := range tests {
		got := classifyTool(tt.name)
		if got != tt.category {
			t.Errorf("classifyTool(%q) = %v, want %v", tt.name, got, tt.category)
		}
	}
}

func TestExtractCerebroOp(t *testing.T) {
	tests := []struct {
		command string
		want    string
	}{
		{"cerebro add --type concept 'test' -p /project", "add"},
		{"cerebro recall 'something' -p /project", "recall"},
		{"cerebro search 'query' --limit 5 -p /project", "search"},
		{"cerebro update abc123 --content 'new' -p /project", "update"},
		{"cerebro supersede abc123 --type concept 'new' -p /project", "supersede"},
		{"go test ./...", ""},
		{"git commit -m 'fix'", ""},
		{"./cerebro add 'test'", "add"},
	}

	for _, tt := range tests {
		got := extractCerebroOp(tt.command)
		if got != tt.want {
			t.Errorf("extractCerebroOp(%q) = %q, want %q", tt.command, got, tt.want)
		}
	}
}

func writeFile(path, content string) error {
	return writeTestFile(path, content)
}
