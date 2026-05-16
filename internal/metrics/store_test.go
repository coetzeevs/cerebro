package metrics

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestMetricsPath_ResolvesSymlinks verifies that MetricsPath resolves symlinks
// before hashing so that a symlinked project path and its realpath produce
// identical metrics paths (HS-008 N1 stack-wide fold-in, mirrors TestProjectPath_ResolvesSymlinks).
func TestMetricsPath_ResolvesSymlinks(t *testing.T) {
	realDir := t.TempDir()
	symlinkDir := filepath.Join(t.TempDir(), "symlink-project")

	if err := os.Symlink(realDir, symlinkDir); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	pathViaReal := MetricsPath(realDir)
	pathViaSym := MetricsPath(symlinkDir)

	if pathViaReal != pathViaSym {
		t.Errorf("symlinked and real paths should produce the same metrics path:\n  real: %s\n  sym:  %s", pathViaReal, pathViaSym)
	}
}

func TestInit_CreatesDatabase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.metrics.sqlite")

	s, err := Init(path)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer func() { _ = s.Close() }()

	// Verify tables exist by querying them.
	for _, table := range []string{"turn_metrics", "tool_calls", "daily_summary", "ingest_state"} {
		row := s.db.QueryRow("SELECT COUNT(*) FROM " + table)
		var count int
		if err := row.Scan(&count); err != nil {
			t.Errorf("table %s not accessible: %v", table, err)
		}
	}
}

func TestOpen_ExistingDatabase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.metrics.sqlite")

	s, err := Init(path)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	_ = s.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	_ = s2.Close()
}

func TestOpen_NonExistent(t *testing.T) {
	_, err := Open("/nonexistent/path/test.metrics.sqlite")
	if err == nil {
		t.Error("expected error for non-existent path")
	}
}

func TestInsertTurnMetrics(t *testing.T) {
	s := testStore(t)

	tm := TurnMetrics{
		SessionID:       "test-session-1",
		TurnNumber:      1,
		Timestamp:       "2026-04-23T10:00:00Z",
		InputTokens:     5000,
		OutputTokens:    1200,
		CacheReadTokens: 4800,
		ThinkingChars:   850,
		ThinkingBlocks:  1,
		ToolCallsTotal:  5,
		ToolReads:       3,
		ToolEdits:       1,
		ToolBash:        1,
		ReadEditRatio:   ptr(3.0),
		AssistantMsgs:   1,
	}

	if err := s.InsertTurnMetrics([]TurnMetrics{tm}); err != nil {
		t.Fatalf("InsertTurnMetrics: %v", err)
	}

	// Verify it was inserted.
	rows, err := s.QueryTurns(TurnFilter{SessionID: "test-session-1"})
	if err != nil {
		t.Fatalf("QueryTurns: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].InputTokens != 5000 {
		t.Errorf("InputTokens = %d, want 5000", rows[0].InputTokens)
	}
	if rows[0].ThinkingChars != 850 {
		t.Errorf("ThinkingChars = %d, want 850", rows[0].ThinkingChars)
	}
}

func TestInsertTurnMetrics_ReplaceOnConflict(t *testing.T) {
	s := testStore(t)

	// Insert a turn with initial data.
	tm1 := TurnMetrics{
		SessionID:   "test-session-1",
		TurnNumber:  1,
		Timestamp:   "2026-04-23T10:00:00Z",
		InputTokens: 5000,
		ToolReads:   2,
	}
	if err := s.InsertTurnMetrics([]TurnMetrics{tm1}); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	// Re-insert the same turn with updated data (simulating a re-parse
	// where the turn is now more complete).
	tm2 := TurnMetrics{
		SessionID:   "test-session-1",
		TurnNumber:  1,
		Timestamp:   "2026-04-23T10:00:00Z",
		InputTokens: 8000,
		ToolReads:   4,
	}
	if err := s.InsertTurnMetrics([]TurnMetrics{tm2}); err != nil {
		t.Fatalf("second insert: %v", err)
	}

	// Should have 1 row with the UPDATED data (REPLACE semantics).
	rows, err := s.QueryTurns(TurnFilter{SessionID: "test-session-1"})
	if err != nil {
		t.Fatalf("QueryTurns: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].InputTokens != 8000 {
		t.Errorf("InputTokens = %d, want 8000 (second insert should win)", rows[0].InputTokens)
	}
	if rows[0].ToolReads != 4 {
		t.Errorf("ToolReads = %d, want 4 (second insert should win)", rows[0].ToolReads)
	}
}

func TestInsertToolCalls(t *testing.T) {
	s := testStore(t)

	calls := []ToolCall{
		{SessionID: "s1", TurnNumber: 1, Timestamp: "2026-04-23T10:00:00Z", ToolName: "Read", FilePath: ptr("/foo/bar.go")},
		{SessionID: "s1", TurnNumber: 1, Timestamp: "2026-04-23T10:00:01Z", ToolName: "Edit", FilePath: ptr("/foo/bar.go")},
		{SessionID: "s1", TurnNumber: 1, Timestamp: "2026-04-23T10:00:02Z", ToolName: "Bash", CerebroOp: ptr("recall")},
	}

	if err := s.InsertToolCalls(calls); err != nil {
		t.Fatalf("InsertToolCalls: %v", err)
	}

	// Verify by counting.
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM tool_calls WHERE session_id = 's1'").Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 tool calls, got %d", count)
	}
}

func TestQueryTurns_FilterBySession(t *testing.T) {
	s := testStore(t)

	_ = s.InsertTurnMetrics([]TurnMetrics{
		{SessionID: "s1", TurnNumber: 1, Timestamp: "2026-04-23T10:00:00Z"},
		{SessionID: "s1", TurnNumber: 2, Timestamp: "2026-04-23T10:01:00Z"},
		{SessionID: "s2", TurnNumber: 1, Timestamp: "2026-04-23T10:02:00Z"},
	})

	rows, err := s.QueryTurns(TurnFilter{SessionID: "s1"})
	if err != nil {
		t.Fatalf("QueryTurns: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 rows for s1, got %d", len(rows))
	}
}

func TestQueryTurns_Limit(t *testing.T) {
	s := testStore(t)

	for i := 1; i <= 10; i++ {
		ts := fmt.Sprintf("2026-04-23T10:%02d:00Z", i)
		_ = s.InsertTurnMetrics([]TurnMetrics{
			{SessionID: "s1", TurnNumber: i, Timestamp: ts},
		})
	}

	rows, err := s.QueryTurns(TurnFilter{SessionID: "s1", Limit: 3, OrderDesc: true})
	if err != nil {
		t.Fatalf("QueryTurns: %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("expected 3 rows with limit, got %d", len(rows))
	}
	if rows[0].TurnNumber != 10 {
		t.Errorf("expected turn 10 first (desc by timestamp), got %d", rows[0].TurnNumber)
	}
}

func TestQueryTurns_CrossSession_OrdersByTimestamp(t *testing.T) {
	s := testStore(t)

	// Two sessions with interleaved timestamps.
	// Session A: turns at 10:00, 10:02, 10:04
	// Session B: turns at 10:01, 10:03, 10:05
	_ = s.InsertTurnMetrics([]TurnMetrics{
		{SessionID: "sA", TurnNumber: 1, Timestamp: "2026-04-23T10:00:00Z"},
		{SessionID: "sA", TurnNumber: 2, Timestamp: "2026-04-23T10:02:00Z"},
		{SessionID: "sA", TurnNumber: 3, Timestamp: "2026-04-23T10:04:00Z"},
		{SessionID: "sB", TurnNumber: 1, Timestamp: "2026-04-23T10:01:00Z"},
		{SessionID: "sB", TurnNumber: 2, Timestamp: "2026-04-23T10:03:00Z"},
		{SessionID: "sB", TurnNumber: 3, Timestamp: "2026-04-23T10:05:00Z"},
	})

	// Last 3 by timestamp should be: sB/3 (10:05), sA/3 (10:04), sB/2 (10:03).
	rows, err := s.QueryTurns(TurnFilter{Limit: 3, OrderDesc: true})
	if err != nil {
		t.Fatalf("QueryTurns: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	if rows[0].SessionID != "sB" || rows[0].TurnNumber != 3 {
		t.Errorf("row 0: expected sB/3, got %s/%d", rows[0].SessionID, rows[0].TurnNumber)
	}
	if rows[1].SessionID != "sA" || rows[1].TurnNumber != 3 {
		t.Errorf("row 1: expected sA/3, got %s/%d", rows[1].SessionID, rows[1].TurnNumber)
	}
	if rows[2].SessionID != "sB" || rows[2].TurnNumber != 2 {
		t.Errorf("row 2: expected sB/2, got %s/%d", rows[2].SessionID, rows[2].TurnNumber)
	}

	// OrderByTurnNumber should give different results: sA/3, sB/3 (both turn_number=3).
	rows2, err := s.QueryTurns(TurnFilter{Limit: 2, OrderBy: OrderByTurnNumber, OrderDesc: true})
	if err != nil {
		t.Fatalf("QueryTurns by turn_number: %v", err)
	}
	if len(rows2) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows2))
	}
	// Both should have turn_number=3.
	for _, r := range rows2 {
		if r.TurnNumber != 3 {
			t.Errorf("expected turn_number=3, got %d", r.TurnNumber)
		}
	}
}

func TestIngestState_RoundTrip(t *testing.T) {
	s := testStore(t)

	// Initially empty.
	state, err := s.GetIngestState("/some/file.jsonl")
	if err != nil {
		t.Fatalf("GetIngestState: %v", err)
	}
	if state.LastOffset != 0 {
		t.Errorf("expected 0 offset for new file, got %d", state.LastOffset)
	}

	// Set offset.
	if err := s.SetIngestState("/some/file.jsonl", 12345); err != nil {
		t.Fatalf("SetIngestState: %v", err)
	}

	// Read back.
	state, err = s.GetIngestState("/some/file.jsonl")
	if err != nil {
		t.Fatalf("GetIngestState: %v", err)
	}
	if state.LastOffset != 12345 {
		t.Errorf("expected offset 12345, got %d", state.LastOffset)
	}

	// Update.
	if err := s.SetIngestState("/some/file.jsonl", 99999); err != nil {
		t.Fatalf("SetIngestState update: %v", err)
	}
	state, _ = s.GetIngestState("/some/file.jsonl")
	if state.LastOffset != 99999 {
		t.Errorf("expected offset 99999 after update, got %d", state.LastOffset)
	}
}

func TestMetricsPath(t *testing.T) {
	path := MetricsPath("/Users/q/projects/agentic/cerebro")
	if path == "" {
		t.Error("MetricsPath returned empty")
	}
	if filepath.Ext(path) != ".sqlite" {
		t.Errorf("expected .sqlite extension, got %s", filepath.Ext(path))
	}
	// Should contain "metrics" to distinguish from brain.
	if !contains(path, ".metrics.") {
		t.Errorf("expected '.metrics.' in path, got %s", path)
	}
}

// --- Test helpers ---

func testStore(t *testing.T) *MetricsStore {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.metrics.sqlite")
	s, err := Init(path)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func ptr[T any](v T) *T {
	return &v
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
