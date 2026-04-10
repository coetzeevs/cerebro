package store

import "fmt"

const schemaVersion = "2"

// applySchema creates all tables and indexes if they don't exist.
func (s *Store) applySchema() error {
	stmts := []string{
		// Schema version tracking
		`CREATE TABLE IF NOT EXISTS schema_meta (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,

		// Memory nodes
		`CREATE TABLE IF NOT EXISTS nodes (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL CHECK (type IN ('episode', 'concept', 'procedure', 'reflection')),
			subtype TEXT,
			content TEXT NOT NULL,
			metadata JSON,
			importance REAL DEFAULT 0.5 CHECK (importance BETWEEN 0.0 AND 1.0),
			decay_rate REAL NOT NULL,
			access_count INTEGER DEFAULT 0,
			times_reinforced INTEGER DEFAULT 0,
			status TEXT DEFAULT 'active' CHECK (status IN ('active', 'consolidated', 'superseded', 'archived')),
			embedding_model TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_accessed DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_reinforced DATETIME,
			updated_at DATETIME,
			last_surfaced DATETIME
		)`,

		// Relationship edges
		`CREATE TABLE IF NOT EXISTS edges (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source_id TEXT NOT NULL,
			target_id TEXT NOT NULL,
			relation TEXT NOT NULL,
			weight REAL DEFAULT 1.0,
			metadata JSON,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (source_id) REFERENCES nodes(id) ON DELETE CASCADE,
			FOREIGN KEY (target_id) REFERENCES nodes(id) ON DELETE CASCADE,
			UNIQUE (source_id, target_id, relation)
		)`,

		// Archive for evicted memories
		`CREATE TABLE IF NOT EXISTS nodes_archive (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			subtype TEXT,
			content TEXT NOT NULL,
			metadata JSON,
			importance REAL,
			status TEXT,
			archive_reason TEXT CHECK (archive_reason IN ('decayed', 'superseded', 'redundant', 'capacity')),
			original_created_at DATETIME,
			archived_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// Performance indexes
		`CREATE INDEX IF NOT EXISTS idx_nodes_type ON nodes(type)`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_status ON nodes(status)`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_type_status ON nodes(type, status)`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_importance ON nodes(importance DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_last_accessed ON nodes(last_accessed)`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_updated_at ON nodes(updated_at)`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_last_surfaced ON nodes(last_surfaced)`,
		`CREATE INDEX IF NOT EXISTS idx_edges_source ON edges(source_id)`,
		`CREATE INDEX IF NOT EXISTS idx_edges_target ON edges(target_id)`,
		`CREATE INDEX IF NOT EXISTS idx_edges_relation ON edges(relation)`,
	}

	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("executing %q: %w", stmt[:60], err)
		}
	}

	// Set schema version if not present
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO schema_meta (key, value) VALUES ('schema_version', ?)`,
		schemaVersion,
	)
	if err != nil {
		return fmt.Errorf("setting schema version: %w", err)
	}

	return nil
}

// InitVectorTable creates the vec_nodes virtual table with the given dimensions.
// This is separate from applySchema because it requires sqlite-vec to be loaded
// and the dimensions depend on the configured embedding provider.
func (s *Store) InitVectorTable(dimensions int) error {
	stmt := fmt.Sprintf(
		`CREATE VIRTUAL TABLE IF NOT EXISTS vec_nodes USING vec0(
			node_id TEXT,
			embedding float[%d] distance_metric=cosine
		)`, dimensions)

	if _, err := s.db.Exec(stmt); err != nil {
		return fmt.Errorf("creating vec_nodes (dim=%d): %w", dimensions, err)
	}

	return nil
}

// SetMeta sets a key-value pair in schema_meta.
func (s *Store) SetMeta(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO schema_meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	return err
}

// GetMeta retrieves a value from schema_meta. Returns empty string if not found.
func (s *Store) GetMeta(key string) (string, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM schema_meta WHERE key = ?`, key).Scan(&value)
	if err != nil {
		return "", nil // key not found is not an error
	}
	return value, nil
}

// migrateSchema applies incremental schema migrations to an existing database.
// It is called from Open() so that existing databases are upgraded on first access.
// Each migration is guarded by a version check and wrapped in a transaction to
// ensure atomicity: a crash mid-migration causes a full rollback on next open.
func (s *Store) migrateSchema() error {
	version, err := s.GetMeta("schema_version")
	if err != nil {
		return err
	}

	if version == "1" {
		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("beginning v1->v2 migration: %w", err)
		}
		defer func() { _ = tx.Rollback() }()

		stmts := []string{
			`ALTER TABLE nodes ADD COLUMN updated_at DATETIME`,
			`ALTER TABLE nodes ADD COLUMN last_surfaced DATETIME`,
			`CREATE INDEX IF NOT EXISTS idx_nodes_updated_at ON nodes(updated_at)`,
			`CREATE INDEX IF NOT EXISTS idx_nodes_last_surfaced ON nodes(last_surfaced)`,
		}
		for _, stmt := range stmts {
			if _, err := tx.Exec(stmt); err != nil {
				return fmt.Errorf("migrating v1->v2: %w", err)
			}
		}

		// Version update MUST be inside the transaction. Using tx.Exec (not s.SetMeta)
		// because SetMeta operates on s.db (the connection), not the transaction handle.
		// This makes the entire migration atomic.
		if _, err := tx.Exec(
			`INSERT INTO schema_meta (key, value) VALUES ('schema_version', '2')
             ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		); err != nil {
			return fmt.Errorf("updating schema version to 2: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing v1->v2 migration: %w", err)
		}
	}

	return nil
}
