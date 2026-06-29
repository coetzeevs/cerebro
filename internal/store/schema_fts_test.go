//go:build fts5

package store

import (
	"path/filepath"
	"testing"
)

// TestNodesFTSCreatedOnInit (AC1a) verifies that a freshly initialised brain
// (built with the fts5 tag) has the nodes_fts FTS5 virtual table present, and
// that the "CREATE VIRTUAL TABLE … USING fts5" statement does NOT return
// "no such module: fts5".
func TestNodesFTSCreatedOnInit(t *testing.T) {
	s := testStore(t)

	// nodes_fts exists as a virtual table in sqlite_master.
	var name string
	err := s.db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name='nodes_fts'`,
	).Scan(&name)
	if err != nil {
		t.Fatalf("nodes_fts not found in sqlite_master: %v", err)
	}
	if name != "nodes_fts" {
		t.Fatalf("expected nodes_fts, got %q", name)
	}

	// FTS5 module is genuinely linked: a fresh CREATE VIRTUAL TABLE … USING fts5
	// must succeed (the premise-correction probe).
	if _, err := s.db.Exec(`CREATE VIRTUAL TABLE nodes_fts_probe USING fts5(x)`); err != nil {
		t.Fatalf("CREATE VIRTUAL TABLE … USING fts5 failed (FTS5 not linked): %v", err)
	}
}

// TestNodesFTSMigrationFromV2 (AC1a / R4) simulates an existing v2 brain (no
// nodes_fts) being opened by v3 code: migrateSchema must create nodes_fts and
// backfill it from the existing nodes rows.
func TestNodesFTSMigrationFromV2(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "v2.sqlite")

	// Build a v2 brain: Init creates v3 today, so force the version back to "2"
	// and drop nodes_fts to emulate a pre-2lak brain.
	s, err := Init(path)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "HS-049 ticket about widgets"}); err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	if _, err := s.db.Exec(`DROP TABLE IF EXISTS nodes_fts`); err != nil {
		t.Fatalf("drop nodes_fts: %v", err)
	}
	if err := s.SetMeta("schema_version", "2"); err != nil {
		t.Fatalf("SetMeta v2: %v", err)
	}
	_ = s.Close()

	// Re-open via Open() — migrateSchema must create + backfill nodes_fts.
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("Open v2 database: %v", err)
	}
	defer func() { _ = s2.Close() }()

	ver, _ := s2.GetMeta("schema_version")
	if ver != "5" {
		t.Fatalf("expected schema_version=5 after migration, got %q", ver)
	}

	// nodes_fts must be backfilled with the pre-existing node.
	var count int
	if err := s2.db.QueryRow(`SELECT COUNT(*) FROM nodes_fts`).Scan(&count); err != nil {
		t.Fatalf("counting nodes_fts after migration: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 backfilled row in nodes_fts, got %d", count)
	}
}
