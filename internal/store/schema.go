package store

import (
	"database/sql"
	"fmt"
	"os"
	"time"
)

const schemaVersion = "7"

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
			last_surfaced DATETIME,
			-- provenance_root marks a node as a first-class provenance source
			-- (agentic-lbjg). 0/1 integer flag (SQLite has no native BOOLEAN);
			-- NOT NULL DEFAULT 0 so existing rows backfill to 0 on the v4->v5
			-- ALTER and a flagless add defaults to 0.
			provenance_root INTEGER NOT NULL DEFAULT 0,
			-- Origin identity (agentic-goc7): who/what wrote the memory,
			-- via which channel, session, host. NULL = not recorded.
			origin_actor TEXT,
			origin_channel TEXT,
			origin_session TEXT,
			origin_host TEXT
		)`,

		// Relationship edges. valid_at/invalid_at carry the bi-temporal
		// valid-time window (agentic-xtzn): the half-open interval
		// [valid_at, invalid_at) during which the asserted relationship holds in
		// the world. Both are nullable with NO default — NULL valid_at means
		// "valid from -inf", NULL invalid_at means "still valid / open-ended".
		// This is the valid-time axis, orthogonal to created_at (transaction
		// time, when the row was written). See ADR-015.
		`CREATE TABLE IF NOT EXISTS edges (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source_id TEXT NOT NULL,
			target_id TEXT NOT NULL,
			relation TEXT NOT NULL,
			weight REAL DEFAULT 1.0,
			metadata JSON,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			valid_at DATETIME,
			invalid_at DATETIME,
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
		`CREATE INDEX IF NOT EXISTS idx_nodes_origin_actor ON nodes(origin_actor)`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_origin_host ON nodes(origin_host)`,
		// Typed-relation registry (agentic-8l2g): warn-not-error ontology
		// discipline for edge relations. Seeded with the built-ins below.
		`CREATE TABLE IF NOT EXISTS relations (
			name TEXT PRIMARY KEY,
			traversal_class TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`INSERT OR IGNORE INTO relations (name) VALUES ('derived_from'), ('supports'), ('contradicts'), ('supersedes')`,
		// Capture-with-approval quarantine inbox (agentic-m8m3): candidates
		// live OUTSIDE nodes so no retrieval surface can see them.
		`CREATE TABLE IF NOT EXISTS inbox_candidates (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL CHECK (type IN ('episode', 'concept', 'procedure', 'reflection')),
			subtype TEXT,
			content TEXT NOT NULL,
			metadata JSON,
			importance REAL DEFAULT 0.5,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			origin_actor TEXT,
			origin_channel TEXT,
			origin_session TEXT,
			origin_host TEXT
		)`,
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

	// Provenance convention boundary (agentic-lbjg). On a fresh v5 Init the brain
	// has no legacy era: stamp the boundary at the brain's birth instant so every
	// node created from now on is at/after the boundary (reads none/complete, never
	// legacy). INSERT OR IGNORE is the one-time-stamp guard — a re-Init or a brain
	// migrated to v5 (which set the boundary inside the v4->v5 tx) is never
	// re-stamped. The value is the storage-layout instant, parsed back into a
	// time.Time at comparison time (provenanceStatus), never compared as a string.
	if _, err := s.db.Exec(
		`INSERT OR IGNORE INTO schema_meta (key, value) VALUES (?, ?)`,
		MetaProvenanceConventionSince, time.Now().UTC().Format(storageTimeLayout),
	); err != nil {
		return fmt.Errorf("setting provenance convention boundary: %w", err)
	}

	// Origin convention boundary (agentic-goc7). Same one-time-stamp discipline
	// as the provenance boundary above: fresh v6 brains stamp at birth, migrated
	// brains were stamped inside the v5->v6 tx, INSERT OR IGNORE never re-stamps.
	if _, err := s.db.Exec(
		`INSERT OR IGNORE INTO schema_meta (key, value) VALUES (?, ?)`,
		MetaOriginConventionSince, time.Now().UTC().Format(storageTimeLayout),
	); err != nil {
		return fmt.Errorf("setting origin convention boundary: %w", err)
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

// IncrMeta atomically increments an integer-valued schema_meta key (UPSERT —
// no read-modify-write race). A missing key starts at 1. A non-integer
// existing value resets via CAST (SQLite CAST of non-numeric text → 0, then
// +1 = 1) — documented semantics, accepted for observability counters such as
// the lazy-expansion skip metric stats.expansion_skips (agentic-73l6 AC4/R5);
// do not use IncrMeta for ledger data. The key is always a compile-time Go
// constant at call sites; it is parameterized here regardless.
func (s *Store) IncrMeta(key string) error {
	_, err := s.db.Exec(
		`INSERT INTO schema_meta (key, value) VALUES (?, '1')
		 ON CONFLICT(key) DO UPDATE SET value = CAST(CAST(value AS INTEGER) + 1 AS TEXT)`,
		key,
	)
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
		version = "3"
	}

	// v3 -> v4: add the bi-temporal valid-time window columns to edges
	// (agentic-xtzn). Follows the v1->v2 transaction-guarded ALTER idiom
	// verbatim (NOT the 2lak guarded-create path): ALTER TABLE ADD COLUMN is a
	// constant-time metadata-only operation that cannot fail on a build-tag /
	// module-availability basis (unlike CREATE VIRTUAL TABLE … USING fts5), so
	// it belongs inside the transaction, not the log-and-continue path. The
	// `if version == "3"` guard runs the ALTERs exactly once: on a v4 brain this
	// block is skipped, so a re-open never hits a duplicate-column error. The
	// whole step is one transaction — a crash mid-migration rolls back and
	// re-runs on next open. Placed AFTER the unconditional initFTSTable() call
	// above so the FTS keyword index keeps firing on every Open regardless of
	// edge-schema version.
	if version == "3" {
		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("beginning v3->v4 migration: %w", err)
		}
		defer func() { _ = tx.Rollback() }()

		// Each ADD COLUMN is guarded on the column's actual absence (not just the
		// version meta) so the step is self-healing: a brain whose edges table
		// already carries a column but whose schema_meta lags at v3 (e.g. a
		// partially-migrated or hand-edited brain) advances cleanly instead of
		// hitting "duplicate column name". SQLite has no ALTER TABLE ADD COLUMN
		// IF NOT EXISTS, so the presence check is explicit.
		for _, col := range []string{"valid_at", "invalid_at"} {
			has, err := txColumnExists(tx, "edges", col)
			if err != nil {
				return fmt.Errorf("checking edges.%s before v3->v4 migration: %w", col, err)
			}
			if has {
				continue
			}
			if _, err := tx.Exec(`ALTER TABLE edges ADD COLUMN ` + col + ` DATETIME`); err != nil {
				return fmt.Errorf("migrating v3->v4 (add %s): %w", col, err)
			}
		}

		if _, err := tx.Exec(
			`INSERT INTO schema_meta (key, value) VALUES ('schema_version', '4')
             ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		); err != nil {
			return fmt.Errorf("updating schema version to 4: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing v3->v4 migration: %w", err)
		}
		version = "4"
	}

	// v4 -> v5: add the provenance_root flag column to nodes (agentic-lbjg) and
	// stamp the provenance-convention boundary. Follows the v3->v4 transaction-
	// guarded ALTER idiom verbatim: ALTER TABLE ADD COLUMN is a constant-time
	// metadata-only operation (not a build-tag-dependent CREATE VIRTUAL TABLE), so
	// it belongs inside the transaction. The ADD is guarded on the column's actual
	// absence (txColumnExists) so a partially-migrated brain self-heals instead of
	// hitting "duplicate column name". `INTEGER NOT NULL DEFAULT 0` is legal for
	// ADD COLUMN because the default is a non-NULL constant (live-probed on SQLite
	// 3.51.2); existing rows backfill to 0 with no row rewrite. The boundary meta
	// is the migration instant, so all pre-existing nodes read `legacy` and
	// post-migration nodes read `none`/`complete`. The whole step is one tx — a
	// crash mid-migration rolls back and re-runs on next open. See ADR-016.
	if version == "4" {
		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("beginning v4->v5 migration: %w", err)
		}
		defer func() { _ = tx.Rollback() }()

		has, err := txColumnExists(tx, "nodes", "provenance_root")
		if err != nil {
			return fmt.Errorf("checking nodes.provenance_root before v4->v5 migration: %w", err)
		}
		if !has {
			if _, err := tx.Exec(
				`ALTER TABLE nodes ADD COLUMN provenance_root INTEGER NOT NULL DEFAULT 0`,
			); err != nil {
				return fmt.Errorf("migrating v4->v5 (add provenance_root): %w", err)
			}
		}

		// Stamp the convention boundary inside the tx. INSERT OR IGNORE so a brain
		// that already carries the boundary (e.g. re-run after a partial migration)
		// is never re-stamped — the boundary is a one-time instant.
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO schema_meta (key, value) VALUES (?, ?)`,
			MetaProvenanceConventionSince, time.Now().UTC().Format(storageTimeLayout),
		); err != nil {
			return fmt.Errorf("migrating v4->v5 (set provenance boundary): %w", err)
		}

		if _, err := tx.Exec(
			`INSERT INTO schema_meta (key, value) VALUES ('schema_version', '5')
             ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		); err != nil {
			return fmt.Errorf("updating schema version to 5: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing v4->v5 migration: %w", err)
		}
		version = "5"
	}

	// v5 -> v6: origin identity (agentic-goc7) + typed-relation registry
	// (agentic-8l2g). Four nullable TEXT origin columns on nodes (plain ADD
	// COLUMN, no default needed — NULL means "not recorded", which is exactly
	// right for every pre-existing row), the relations registry table with its
	// built-in seeds, supporting indexes, and the origin convention boundary
	// stamped at the migration instant so pre-existing nodes classify "legacy".
	// Open() runs migrations only (applySchema is Init-only), so this block
	// must create everything a v6 brain needs — it cannot lean on the fresh-DDL
	// list. Same tx-guarded, txColumnExists-self-healing idiom as v4->v5.
	if version == "5" {
		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("beginning v5->v6 migration: %w", err)
		}
		defer func() { _ = tx.Rollback() }()

		for _, col := range []string{"origin_actor", "origin_channel", "origin_session", "origin_host"} {
			has, err := txColumnExists(tx, "nodes", col)
			if err != nil {
				return fmt.Errorf("checking nodes.%s before v5->v6 migration: %w", col, err)
			}
			if has {
				continue
			}
			if _, err := tx.Exec(`ALTER TABLE nodes ADD COLUMN ` + col + ` TEXT`); err != nil {
				return fmt.Errorf("migrating v5->v6 (add %s): %w", col, err)
			}
		}

		for _, stmt := range []string{
			`CREATE TABLE IF NOT EXISTS relations (
				name TEXT PRIMARY KEY,
				traversal_class TEXT,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP
			)`,
			`INSERT OR IGNORE INTO relations (name) VALUES ('derived_from'), ('supports'), ('contradicts'), ('supersedes')`,
			// Capture-with-approval quarantine inbox (agentic-m8m3): candidates
			// live OUTSIDE nodes so no retrieval surface can see them.
			`CREATE TABLE IF NOT EXISTS inbox_candidates (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL CHECK (type IN ('episode', 'concept', 'procedure', 'reflection')),
			subtype TEXT,
			content TEXT NOT NULL,
			metadata JSON,
			importance REAL DEFAULT 0.5,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			origin_actor TEXT,
			origin_channel TEXT,
			origin_session TEXT,
			origin_host TEXT
		)`,
			`CREATE INDEX IF NOT EXISTS idx_nodes_origin_actor ON nodes(origin_actor)`,
			`CREATE INDEX IF NOT EXISTS idx_nodes_origin_host ON nodes(origin_host)`,
		} {
			if _, err := tx.Exec(stmt); err != nil {
				return fmt.Errorf("migrating v5->v6 (relations/indexes): %w", err)
			}
		}

		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO schema_meta (key, value) VALUES (?, ?)`,
			MetaOriginConventionSince, time.Now().UTC().Format(storageTimeLayout),
		); err != nil {
			return fmt.Errorf("migrating v5->v6 (set origin boundary): %w", err)
		}

		if _, err := tx.Exec(
			`INSERT INTO schema_meta (key, value) VALUES ('schema_version', '6')
             ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		); err != nil {
			return fmt.Errorf("updating schema version to 6: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing v5->v6 migration: %w", err)
		}
		version = "6"
	}

	// v6 -> v7: the quarantine inbox table (agentic-m8m3). A pure CREATE
	// TABLE IF NOT EXISTS — no ALTER, no backfill, constant-time; same
	// tx-guarded idiom as prior migrations. Open() runs migrations only, so
	// the table must be created here as well as in the fresh DDL.
	if version == "6" {
		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("beginning v6->v7 migration: %w", err)
		}
		defer func() { _ = tx.Rollback() }()

		if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS inbox_candidates (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL CHECK (type IN ('episode', 'concept', 'procedure', 'reflection')),
			subtype TEXT,
			content TEXT NOT NULL,
			metadata JSON,
			importance REAL DEFAULT 0.5,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			origin_actor TEXT,
			origin_channel TEXT,
			origin_session TEXT,
			origin_host TEXT
		)`); err != nil {
			return fmt.Errorf("migrating v6->v7 (inbox_candidates): %w", err)
		}
		if _, err := tx.Exec(
			`INSERT INTO schema_meta (key, value) VALUES ('schema_version', '7')
             ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		); err != nil {
			return fmt.Errorf("updating schema version to 7: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing v6->v7 migration: %w", err)
		}
		version = "7"
	}

	// version is intentionally re-assigned above for clarity and to keep each
	// migration block self-contained; the value is consumed only by subsequent
	// blocks (none follow v7 today).
	_ = version

	return nil
}

// txColumnExists reports whether the given table has a column of the given name,
// using PRAGMA table_info within the supplied transaction. The column name is a
// compile-time Go constant at all call sites; PRAGMA table_info takes the table
// name as an identifier (not a bindable parameter), and `table` here is likewise
// a constant — no user input reaches this query.
func txColumnExists(tx *sql.Tx, table, column string) (bool, error) {
	rows, err := tx.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			cid     int
			name    string
			ctyp    string
			notNull int
			dflt    sql.NullString
			pk      int
		)
		if err := rows.Scan(&cid, &name, &ctyp, &notNull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}
