package store

import (
	"fmt"
	"os"
)

const schemaVersion = "3"

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

	// nodes_fts FTS5 virtual table (agentic-2lak). This is a SEPARATE guarded
	// call OUTSIDE the stmts[] loop above: a CREATE VIRTUAL TABLE … USING fts5
	// fails with "no such module: fts5" on any binary built without the fts5
	// build tag, and the stmts[] loop returns on the FIRST error — inlining the
	// FTS create there would abort Init/Open and brick every no-fts5 binary.
	// initFTSTable logs-and-continues on failure, so keyword recall degrades
	// gracefully (the keyword lane contributes nothing) while the primary store
	// remains fully functional. Mirrors InitVectorTable's separation for vec.
	s.initFTSTable()

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

// ftsAvailable reports whether the nodes_fts FTS5 virtual table exists in this
// database. It returns false when the binary was built without the fts5 build
// tag (the CREATE failed and was logged-and-skipped) — the application-level
// CRUD sync uses this to skip FTS writes gracefully (D2: no trigger/build-tag
// coupling that would crash basic writes).
func (s *Store) ftsAvailable() bool {
	var name string
	err := s.db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name='nodes_fts'`,
	).Scan(&name)
	return err == nil && name == "nodes_fts"
}

// initFTSTable creates the nodes_fts FTS5 virtual table if it does not exist and
// backfills it from the existing nodes table when freshly created. It is
// deliberately separate from applySchema's stmts[] loop and best-effort: on a
// binary built WITHOUT the fts5 tag the CREATE returns "no such module: fts5",
// which is logged once to stderr and swallowed so the store opens normally
// (graceful degrade — keyword recall is simply absent). Mirrors the
// vec_nodes-absent tolerance in search.go/gc.go.
//
// nodes_fts is a STANDALONE FTS5 table (NOT external-content): nodes.id is a
// TEXT UUID and external-content FTS5 requires an INTEGER content_rowid. The
// node_id column is UNINDEXED (a join key, not searched); content and subtype
// are the indexed text columns.
func (s *Store) initFTSTable() {
	// Was the table already present before this call? Used to decide whether a
	// fresh-create backfill is needed.
	existed := s.ftsAvailable()

	if _, err := s.db.Exec(
		`CREATE VIRTUAL TABLE IF NOT EXISTS nodes_fts USING fts5(
			node_id UNINDEXED,
			content,
			subtype
		)`,
	); err != nil {
		// no-fts5 binary: log once, continue. Keyword recall degrades to nothing.
		fmt.Fprintf(os.Stderr, "cerebro: FTS5 keyword index unavailable (%v); keyword recall disabled\n", err)
		return
	}

	// Freshly created on a brain that already has nodes: one-time backfill so the
	// keyword index reflects existing memories (Out-of-Scope: one-time backfill
	// on first open, no re-embedding). The nodes-table guard avoids a spurious
	// "no such table: nodes" log on fresh Init, where migrateSchema runs this
	// before applySchema creates the nodes table.
	if !existed && s.tableExists("nodes") {
		if _, err := s.db.Exec(
			`INSERT INTO nodes_fts (node_id, content, subtype)
				SELECT id, content, COALESCE(subtype, '') FROM nodes WHERE status = 'active'`,
		); err != nil {
			fmt.Fprintf(os.Stderr, "cerebro: backfilling nodes_fts failed (%v); keyword recall may be incomplete\n", err)
		}
	}
}

// tableExists reports whether a regular table of the given name exists.
func (s *Store) tableExists(name string) bool {
	var got string
	err := s.db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, name,
	).Scan(&got)
	return err == nil && got == name
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

// DeleteMeta removes a key from schema_meta.
func (s *Store) DeleteMeta(key string) error {
	_, err := s.db.Exec(`DELETE FROM schema_meta WHERE key = ?`, key)
	return err
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
		version = "2"
	}

	// v2 -> v3: create + backfill the nodes_fts FTS5 keyword index (agentic-2lak).
	// initFTSTable is best-effort, idempotent (CREATE … IF NOT EXISTS), and
	// self-healing: it is called UNCONDITIONALLY on every Open so that the FIRST
	// fts5-tagged binary to open a v2-or-later brain creates + backfills the
	// index, even if a prior no-fts5 binary already advanced the version to "3"
	// without creating the table (R4 — the no-fts5 binary logs-and-skips but does
	// NOT hard-fail). The version bump records the upgrade; the table create is
	// decoupled from it precisely so the build-tag-absent path stays graceful.
	s.initFTSTable()
	if version == "2" {
		if err := s.SetMeta("schema_version", "3"); err != nil {
			return fmt.Errorf("updating schema version to 3: %w", err)
		}
	}

	return nil
}
