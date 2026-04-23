package metrics

import "fmt"

// applySchema creates all metrics tables and indexes.
func (s *MetricsStore) applySchema() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS turn_metrics (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id      TEXT NOT NULL,
			turn_number     INTEGER NOT NULL,
			timestamp       TEXT NOT NULL,
			input_tokens          INTEGER DEFAULT 0,
			output_tokens         INTEGER DEFAULT 0,
			cache_read_tokens     INTEGER DEFAULT 0,
			cache_create_tokens   INTEGER DEFAULT 0,
			thinking_chars        INTEGER DEFAULT 0,
			thinking_blocks       INTEGER DEFAULT 0,
			tool_calls_total      INTEGER DEFAULT 0,
			tool_reads            INTEGER DEFAULT 0,
			tool_edits            INTEGER DEFAULT 0,
			tool_bash             INTEGER DEFAULT 0,
			tool_other            INTEGER DEFAULT 0,
			cerebro_recalls       INTEGER DEFAULT 0,
			cerebro_writes        INTEGER DEFAULT 0,
			read_edit_ratio       REAL,
			assistant_messages    INTEGER DEFAULT 1,
			stop_guard_fired      INTEGER DEFAULT 0,
			UNIQUE(session_id, turn_number)
		)`,

		`CREATE TABLE IF NOT EXISTS tool_calls (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id      TEXT NOT NULL,
			turn_number     INTEGER NOT NULL,
			timestamp       TEXT NOT NULL,
			tool_name       TEXT NOT NULL,
			file_path       TEXT,
			cerebro_op      TEXT
		)`,

		`CREATE TABLE IF NOT EXISTS daily_summary (
			date            TEXT PRIMARY KEY,
			turn_count      INTEGER DEFAULT 0,
			zero_thinking   INTEGER DEFAULT 0,
			avg_thinking    REAL,
			avg_read_edit   REAL,
			min_read_edit   REAL,
			stop_guard_fires INTEGER DEFAULT 0,
			total_reads     INTEGER DEFAULT 0,
			total_edits     INTEGER DEFAULT 0,
			cerebro_recalls INTEGER DEFAULT 0,
			cerebro_writes  INTEGER DEFAULT 0,
			tool_distribution TEXT,
			active_nodes    INTEGER,
			total_edges     INTEGER,
			nodes_by_type   TEXT
		)`,

		`CREATE TABLE IF NOT EXISTS ingest_state (
			file_path       TEXT PRIMARY KEY,
			last_offset     INTEGER DEFAULT 0,
			updated_at      TEXT DEFAULT (datetime('now'))
		)`,

		// Indexes
		`CREATE INDEX IF NOT EXISTS idx_turn_metrics_session ON turn_metrics(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_turn_metrics_timestamp ON turn_metrics(timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_turn_metrics_session_turn ON turn_metrics(session_id, turn_number)`,
		`CREATE INDEX IF NOT EXISTS idx_tool_calls_session ON tool_calls(session_id, turn_number)`,
		`CREATE INDEX IF NOT EXISTS idx_tool_calls_name ON tool_calls(tool_name)`,
		`CREATE INDEX IF NOT EXISTS idx_tool_calls_ts ON tool_calls(timestamp)`,
	}

	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("executing schema: %w", err)
		}
	}

	return nil
}
