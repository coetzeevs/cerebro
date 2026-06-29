package store

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// nodeColumns returns the column names of the nodes table via PRAGMA table_info.
func nodeColumns(t *testing.T, s *Store) map[string]bool {
	t.Helper()
	rows, err := s.db.Query(`PRAGMA table_info(nodes)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info(nodes): %v", err)
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

// TestFreshInitHasProvenanceRootAtV5 verifies a freshly-initialised brain
// carries schema_version=5, the nodes table has provenance_root, and the
// provenance-convention boundary meta is set (AC1 — fresh Init is v5).
func TestFreshInitHasProvenanceRootAtV5(t *testing.T) {
	s := testStore(t)

	ver, err := s.GetMeta("schema_version")
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if ver != "5" {
		t.Fatalf("expected schema_version=5 on fresh Init, got %q", ver)
	}

	if !nodeColumns(t, s)["provenance_root"] {
		t.Error("fresh Init: nodes table missing provenance_root column")
	}

	// The provenance-convention boundary must be set so provenance_status can
	// distinguish legacy from none. On a fresh brain it must be parseable.
	since, err := s.GetMeta(MetaProvenanceConventionSince)
	if err != nil {
		t.Fatalf("GetMeta(%s): %v", MetaProvenanceConventionSince, err)
	}
	if since == "" {
		t.Fatalf("fresh Init: %s meta not set", MetaProvenanceConventionSince)
	}
	if _, perr := time.Parse(storageTimeLayout, since); perr != nil {
		t.Fatalf("%s=%q not parseable with storageTimeLayout: %v", MetaProvenanceConventionSince, since, perr)
	}
}

// buildV4Database creates a raw SQLite database with the pre-lbjg v4 nodes/edges
// schema (no provenance_root column) and schema_version=4. It mirrors an
// existing v4 brain file. Returns an open Store; caller must Close().
func buildV4Database(t *testing.T, path string) (*Store, error) {
	t.Helper()
	s, err := open(path)
	if err != nil {
		return nil, err
	}

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS schema_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		// v4 nodes DDL: NO provenance_root column.
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
		// v4 edges DDL: validity columns present.
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
		`INSERT OR IGNORE INTO schema_meta (key, value) VALUES ('schema_version', '4')`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			_ = s.Close()
			return nil, fmt.Errorf("building v4 schema: %w", err)
		}
	}
	return s, nil
}

// TestMigrationFromV4AddsProvenanceRoot simulates an existing v4 brain opened by
// lbjg-aware code. The v4->v5 migration must add provenance_root, bump
// schema_version to 5, and write the provenance-convention boundary meta (AC1).
func TestMigrationFromV4AddsProvenanceRoot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "v4.sqlite")

	s, err := buildV4Database(t, path)
	if err != nil {
		t.Fatalf("buildV4Database: %v", err)
	}
	// Seed a pre-migration node so we can assert it reads provenance_root=0.
	if _, err := s.db.Exec(
		`INSERT INTO nodes (id, type, content, decay_rate) VALUES ('legacy-node', 'episode', 'old memory', 0.15)`,
	); err != nil {
		t.Fatalf("seeding legacy node: %v", err)
	}
	if nodeColumns(t, s)["provenance_root"] {
		t.Fatal("precondition failed: v4 brain unexpectedly has provenance_root")
	}
	_ = s.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("Open v4 database: %v", err)
	}
	defer func() { _ = s2.Close() }()

	ver, err := s2.GetMeta("schema_version")
	if err != nil {
		t.Fatalf("GetMeta after migration: %v", err)
	}
	if ver != "5" {
		t.Fatalf("expected schema_version=5 after v4->v5 migration, got %q", ver)
	}
	if !nodeColumns(t, s2)["provenance_root"] {
		t.Fatalf("v4->v5 migration did not add provenance_root")
	}

	// Existing rows must read provenance_root = 0 (DEFAULT 0, backfill-safe).
	var pr int
	if err := s2.db.QueryRow(`SELECT provenance_root FROM nodes WHERE id='legacy-node'`).Scan(&pr); err != nil {
		t.Fatalf("reading provenance_root: %v", err)
	}
	if pr != 0 {
		t.Fatalf("legacy node provenance_root=%d, want 0", pr)
	}

	// The boundary meta must be written and parseable.
	since, err := s2.GetMeta(MetaProvenanceConventionSince)
	if err != nil {
		t.Fatalf("GetMeta(%s): %v", MetaProvenanceConventionSince, err)
	}
	if since == "" {
		t.Fatalf("migration: %s meta not set", MetaProvenanceConventionSince)
	}
	if _, perr := time.Parse(storageTimeLayout, since); perr != nil {
		t.Fatalf("%s=%q not parseable: %v", MetaProvenanceConventionSince, since, perr)
	}
}

// TestMigrationFromV4Idempotent verifies that re-opening a migrated v5 brain is
// a no-op: no duplicate-column error, version stays 5 (AC1 idempotency).
func TestMigrationFromV4Idempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "v4.sqlite")

	s, err := buildV4Database(t, path)
	if err != nil {
		t.Fatalf("buildV4Database: %v", err)
	}
	_ = s.Close()

	s1, err := Open(path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	since1, _ := s1.GetMeta(MetaProvenanceConventionSince)
	_ = s1.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("second Open (idempotency) errored: %v", err)
	}
	defer func() { _ = s2.Close() }()

	ver, err := s2.GetMeta("schema_version")
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if ver != "5" {
		t.Fatalf("expected schema_version=5 after idempotent re-open, got %q", ver)
	}
	// The boundary must NOT be re-stamped on re-open (it is a one-time instant).
	since2, _ := s2.GetMeta(MetaProvenanceConventionSince)
	if since1 != since2 {
		t.Fatalf("provenance boundary re-stamped on re-open: %q != %q", since1, since2)
	}
}

// TestMigrationV4ToV5SelfHealsExistingColumn verifies the v4->v5 migration is
// robust when the nodes table already carries provenance_root but schema_meta
// lags at v4 (a partially-migrated / hand-edited brain). The ALTER is guarded on
// column presence, so the open must NOT crash with "duplicate column name" and
// must still advance the version to 5 (AC1 crash-safety).
func TestMigrationV4ToV5SelfHealsExistingColumn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "partial.sqlite")

	// Full v5 Init (nodes already has provenance_root), then force version meta
	// back to "4" to emulate a brain whose column exists but version lags.
	s, err := Init(path)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if !nodeColumns(t, s)["provenance_root"] {
		t.Fatal("precondition: fresh Init should already have provenance_root")
	}
	if err := s.SetMeta("schema_version", "4"); err != nil {
		t.Fatalf("SetMeta v4: %v", err)
	}
	_ = s.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("Open partially-migrated v4 brain crashed: %v", err)
	}
	defer func() { _ = s2.Close() }()

	ver, err := s2.GetMeta("schema_version")
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if ver != "5" {
		t.Fatalf("expected schema_version=5 after self-heal, got %q", ver)
	}
	if !nodeColumns(t, s2)["provenance_root"] {
		t.Fatalf("self-heal lost provenance_root column")
	}
}

// TestMigrationFromV1ReachesV5 verifies the full v1->...->v5 ladder runs in a
// single Open so a legacy v1 brain gets provenance_root too.
func TestMigrationFromV1ReachesV5(t *testing.T) {
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
	if ver != "5" {
		t.Fatalf("expected schema_version=5 after full ladder, got %q", ver)
	}
	if !nodeColumns(t, s2)["provenance_root"] {
		t.Fatalf("full ladder did not add provenance_root column")
	}
}
