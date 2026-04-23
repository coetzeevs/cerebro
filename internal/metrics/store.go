package metrics

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

// MetricsStore wraps a SQLite database for observability metrics.
type MetricsStore struct {
	db   *sql.DB
	path string
}

// MetricsPath returns the metrics database path for a project directory.
// Mirrors brain.ProjectPath() with a .metrics.sqlite extension.
func MetricsPath(projectDir string) string {
	abs, _ := filepath.Abs(projectDir)
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(abs)))
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cerebro", "projects", hash+".metrics.sqlite")
}

// Init creates and initializes a new metrics database at the given path.
func Init(path string) (*MetricsStore, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("creating directory %s: %w", dir, err)
	}

	s, err := openDB(path)
	if err != nil {
		return nil, err
	}

	if err := s.applySchema(); err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("applying schema: %w", err)
	}

	return s, nil
}

// Open opens an existing metrics database at the given path.
func Open(path string) (*MetricsStore, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("metrics database not found at %s — run 'cerebro ingest' first", path)
	}
	return openDB(path)
}

// OpenOrInit opens the metrics database, creating it if it doesn't exist.
func OpenOrInit(path string) (*MetricsStore, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return Init(path)
	}
	return openDB(path)
}

func openDB(path string) (*MetricsStore, error) {
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=ON&_cache_size=-65536", path)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening metrics database: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("connecting to metrics database: %w", err)
	}
	return &MetricsStore{db: db, path: path}, nil
}

// Close closes the database connection.
func (s *MetricsStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// DB returns the underlying *sql.DB for advanced operations.
func (s *MetricsStore) DB() *sql.DB {
	return s.db
}

// InsertTurnMetrics inserts turn metrics rows. Uses INSERT OR REPLACE so that
// re-parsing a growing session file overwrites partial turns with complete data.
func (s *MetricsStore) InsertTurnMetrics(rows []TurnMetrics) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO turn_metrics (
		session_id, turn_number, timestamp,
		input_tokens, output_tokens, cache_read_tokens, cache_create_tokens,
		thinking_chars, thinking_blocks,
		tool_calls_total, tool_reads, tool_edits, tool_bash, tool_other,
		cerebro_recalls, cerebro_writes,
		read_edit_ratio, assistant_messages, stop_guard_fired
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for i := range rows {
		r := &rows[i]
		_, err := stmt.Exec(
			r.SessionID, r.TurnNumber, r.Timestamp,
			r.InputTokens, r.OutputTokens, r.CacheReadTokens, r.CacheCreateTokens,
			r.ThinkingChars, r.ThinkingBlocks,
			r.ToolCallsTotal, r.ToolReads, r.ToolEdits, r.ToolBash, r.ToolOther,
			r.CerebroRecalls, r.CerebroWrites,
			r.ReadEditRatio, r.AssistantMsgs, r.StopGuardFired,
		)
		if err != nil {
			return fmt.Errorf("insert turn %s/%d: %w", r.SessionID, r.TurnNumber, err)
		}
	}

	return tx.Commit()
}

// InsertToolCalls inserts tool call detail rows.
func (s *MetricsStore) InsertToolCalls(calls []ToolCall) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`INSERT INTO tool_calls (
		session_id, turn_number, timestamp, tool_name, file_path, cerebro_op
	) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, c := range calls {
		if _, err := stmt.Exec(c.SessionID, c.TurnNumber, c.Timestamp, c.ToolName, c.FilePath, c.CerebroOp); err != nil {
			return fmt.Errorf("insert tool call: %w", err)
		}
	}

	return tx.Commit()
}

// QueryTurns returns turn metrics matching the given filter.
func (s *MetricsStore) QueryTurns(f TurnFilter) ([]TurnMetrics, error) {
	query := `SELECT id, session_id, turn_number, timestamp,
		input_tokens, output_tokens, cache_read_tokens, cache_create_tokens,
		thinking_chars, thinking_blocks,
		tool_calls_total, tool_reads, tool_edits, tool_bash, tool_other,
		cerebro_recalls, cerebro_writes,
		read_edit_ratio, assistant_messages, stop_guard_fired
		FROM turn_metrics WHERE 1=1`

	var args []any

	if f.SessionID != "" {
		query += " AND session_id = ?"
		args = append(args, f.SessionID)
	}
	if f.Since != "" {
		query += " AND timestamp >= ?"
		args = append(args, f.Since)
	}
	if f.Until != "" {
		query += " AND timestamp <= ?"
		args = append(args, f.Until)
	}

	if f.OrderDesc {
		query += " ORDER BY turn_number DESC"
	} else {
		query += " ORDER BY turn_number ASC"
	}

	if f.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, f.Limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query turns: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []TurnMetrics
	for rows.Next() {
		var tm TurnMetrics
		if err := rows.Scan(
			&tm.ID, &tm.SessionID, &tm.TurnNumber, &tm.Timestamp,
			&tm.InputTokens, &tm.OutputTokens, &tm.CacheReadTokens, &tm.CacheCreateTokens,
			&tm.ThinkingChars, &tm.ThinkingBlocks,
			&tm.ToolCallsTotal, &tm.ToolReads, &tm.ToolEdits, &tm.ToolBash, &tm.ToolOther,
			&tm.CerebroRecalls, &tm.CerebroWrites,
			&tm.ReadEditRatio, &tm.AssistantMsgs, &tm.StopGuardFired,
		); err != nil {
			return nil, fmt.Errorf("scan turn: %w", err)
		}
		result = append(result, tm)
	}

	return result, rows.Err()
}

// GetIngestState returns the last-processed offset for a file.
func (s *MetricsStore) GetIngestState(filePath string) (IngestState, error) {
	var state IngestState
	state.FilePath = filePath
	err := s.db.QueryRow(
		`SELECT last_offset FROM ingest_state WHERE file_path = ?`, filePath,
	).Scan(&state.LastOffset)
	if err != nil {
		return IngestState{FilePath: filePath}, nil // not found = offset 0
	}
	return state, nil
}

// SetIngestState updates the last-processed offset for a file.
func (s *MetricsStore) SetIngestState(filePath string, offset int64) error {
	_, err := s.db.Exec(
		`INSERT INTO ingest_state (file_path, last_offset) VALUES (?, ?)
		 ON CONFLICT(file_path) DO UPDATE SET last_offset = excluded.last_offset, updated_at = datetime('now')`,
		filePath, offset,
	)
	return err
}

// DeleteToolCallsForSession removes tool calls for a session to allow re-ingest.
func (s *MetricsStore) DeleteToolCallsForSession(sessionID string) error {
	_, err := s.db.Exec(`DELETE FROM tool_calls WHERE session_id = ?`, sessionID)
	return err
}

// DistinctSessions returns all unique session IDs that have a given turn already ingested.
func (s *MetricsStore) DistinctSessions() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT session_id, MAX(turn_number) FROM turn_metrics GROUP BY session_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string]int)
	for rows.Next() {
		var sid string
		var maxTurn int
		if err := rows.Scan(&sid, &maxTurn); err != nil {
			return nil, err
		}
		result[sid] = maxTurn
	}
	return result, rows.Err()
}
