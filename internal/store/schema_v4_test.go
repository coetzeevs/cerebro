package store

import (
	"fmt"
	"path/filepath"
	"testing"
)

// edgeColumns returns the column names of the edges table via PRAGMA table_info.
func edgeColumns(t *testing.T, s *Store) map[string]bool {
	t.Helper()
	rows, err := s.db.Query(`PRAGMA table_info(edges)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info(edges): %v", err)
	}
	defer func() { _ = rows.Close() }()

	cols := make(map[string]bool)
	for rows.Next() {
		var (
			cid        int
			name, ctyp string
			notNull    int
			dflt       any
			pk         int
		)
		if err := rows.Scan(&cid, &name, &ctyp, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scanning table_info: %v", err)
		}
		cols[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("table_info rows: %v", err)
	}
	return cols
}

// TestFreshInitHasValidityColumnsAtV4 verifies a freshly-initialised brain
// carries the current schema_version (now 5 after the lbjg provenance_root bump)
// and the edges table still has the xtzn valid_at + invalid_at columns.
func TestFreshInitHasValidityColumnsAtV4(t *testing.T) {
	s := testStore(t)

	ver, err := s.GetMeta("schema_version")
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if ver != "6" {
		t.Fatalf("expected schema_version=6 on fresh Init, got %q", ver)
	}

	cols := edgeColumns(t, s)
	if !cols["valid_at"] {
		t.Error("fresh Init: edges table missing valid_at column")
	}
	if !cols["invalid_at"] {
		t.Error("fresh Init: edges table missing invalid_at column")
	}
}

// buildV3Database creates a raw SQLite database with the pre-xtzn v3 edges
// schema (no valid_at/invalid_at columns) and schema_version=3. It mirrors an
// existing v3 brain file. Returns an open Store; caller must Close().
func buildV3Database(t *testing.T, path string) (*Store, error) {
	t.Helper()
	s, err := open(path)
	if err != nil {
		return nil, err
	}

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS schema_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS nodes (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL CHECK (type IN ('episode', 'concept', 'procedure', 'reflection')),
			subtype TEXT,
			content TEXT NOT NULL,
			metadata JSON,
			importance REAL DEFAULT 0.5,
			decay_rate REAL NOT NULL,
			access_count INTEGER DEFAULT 0,
			times_reinforced INTEGER DEFAULT 0,
			status TEXT DEFAULT 'active',
			embedding_model TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_accessed DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_reinforced DATETIME,
			updated_at DATETIME,
			last_surfaced DATETIME
		)`,
		// v3 edges DDL: created_at present, NO valid_at/invalid_at.
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
		`INSERT OR IGNORE INTO schema_meta (key, value) VALUES ('schema_version', '3')`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			_ = s.Close()
			return nil, fmt.Errorf("building v3 schema: %w", err)
		}
	}
	return s, nil
}

// TestMigrationFromV3AddsValidityColumns simulates an existing v3 brain opened
// by xtzn-aware code. The v3->v4 migration must add valid_at/invalid_at and
// bump schema_version to 4 without crashing (AC1).
func TestMigrationFromV3AddsValidityColumns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "v3.sqlite")

	s, err := buildV3Database(t, path)
	if err != nil {
		t.Fatalf("buildV3Database: %v", err)
	}
	// Sanity: the v3 brain does NOT have the columns yet.
	if edgeColumns(t, s)["valid_at"] {
		t.Fatal("precondition failed: v3 brain unexpectedly has valid_at")
	}
	_ = s.Close()

	// Re-open via Open() — migrateSchema() should run the v3->v4 step.
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("Open v3 database: %v", err)
	}
	defer func() { _ = s2.Close() }()

	ver, err := s2.GetMeta("schema_version")
	if err != nil {
		t.Fatalf("GetMeta after migration: %v", err)
	}
	if ver != "6" {
		t.Fatalf("expected schema_version=6 after v3->v4->v5 ladder, got %q", ver)
	}

	cols := edgeColumns(t, s2)
	if !cols["valid_at"] || !cols["invalid_at"] {
		t.Fatalf("v3->v4 migration did not add validity columns: have %v", cols)
	}

	// The columns must be queryable.
	if _, err := s2.db.Exec(`SELECT valid_at, invalid_at FROM edges LIMIT 1`); err != nil {
		t.Fatalf("validity columns not queryable after migration: %v", err)
	}
}

// TestMigrationFromV3Idempotent verifies that re-opening a migrated v4 brain is
// a no-op: no duplicate-column error, version stays 4 (AC1 idempotency).
func TestMigrationFromV3Idempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "v3.sqlite")

	s, err := buildV3Database(t, path)
	if err != nil {
		t.Fatalf("buildV3Database: %v", err)
	}
	_ = s.Close()

	// First open: runs v3->v4.
	s1, err := Open(path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	_ = s1.Close()

	// Second open: must NOT re-run the ALTERs (would error "duplicate column").
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("second Open (idempotency) errored: %v", err)
	}
	defer func() { _ = s2.Close() }()

	ver, err := s2.GetMeta("schema_version")
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if ver != "6" {
		t.Fatalf("expected schema_version=6 after idempotent re-open, got %q", ver)
	}
}

// TestMigrationV3ToV4SelfHealsExistingColumns verifies the v3->v4 migration is
// robust when the edges table already carries the validity columns but
// schema_meta lags at v3 (a partially-migrated / hand-edited brain). The ALTERs
// are guarded on column presence, so the open must NOT crash with
// "duplicate column name" and must still advance the version to 4 (AC1
// crash-safety).
func TestMigrationV3ToV4SelfHealsExistingColumns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "partial.sqlite")

	// Start from a full v4 Init (edges already has both columns), then force the
	// version meta back to "3" to emulate a brain whose columns exist but whose
	// recorded version lags.
	s, err := Init(path)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if !edgeColumns(t, s)["valid_at"] {
		t.Fatal("precondition: fresh Init should already have valid_at")
	}
	if err := s.SetMeta("schema_version", "3"); err != nil {
		t.Fatalf("SetMeta v3: %v", err)
	}
	_ = s.Close()

	// Re-open: the v3->v4 block runs but the columns already exist — must not error.
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("Open partially-migrated v3 brain crashed: %v", err)
	}
	defer func() { _ = s2.Close() }()

	ver, err := s2.GetMeta("schema_version")
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if ver != "6" {
		t.Fatalf("expected schema_version=6 after self-heal, got %q", ver)
	}
	cols := edgeColumns(t, s2)
	if !cols["valid_at"] || !cols["invalid_at"] {
		t.Fatalf("self-heal lost validity columns: have %v", cols)
	}
}

// TestMigrationFromV1ReachesV4 verifies the full v1->v2->v3->v4 ladder runs in a
// single Open so a legacy v1 brain gets the validity columns too.
func TestMigrationFromV1ReachesV4(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "v1.sqlite")

	s, err := buildV1Database(t, path)
	if err != nil {
		t.Fatalf("buildV1Database: %v", err)
	}
	_ = s.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("Open v1 database: %v", err)
	}
	defer func() { _ = s2.Close() }()

	ver, err := s2.GetMeta("schema_version")
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if ver != "6" {
		t.Fatalf("expected schema_version=6 after full ladder, got %q", ver)
	}
	cols := edgeColumns(t, s2)
	if !cols["valid_at"] || !cols["invalid_at"] {
		t.Fatalf("full ladder did not add validity columns: have %v", cols)
	}
}
