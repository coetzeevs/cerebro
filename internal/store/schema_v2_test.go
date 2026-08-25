package store

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// TestSchemaVersionIsCurrent verifies a freshly initialised brain carries the
// current schemaVersion. Bumped 2->3 for the nodes_fts FTS5 keyword index
// (agentic-2lak); the version advances on both fts5 and no-fts5 binaries (the
// FTS table create is decoupled from the version bump so the no-fts5 path stays
// graceful).
func TestSchemaVersionIsCurrent(t *testing.T) {
	s := testStore(t)
	ver, err := s.GetMeta("schema_version")
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if ver != "6" {
		t.Fatalf("expected schema_version=6, got %q", ver)
	}
}

// TestMigrationFromV1 simulates an existing v1 database being opened by v2 code.
// It creates a bare SQLite database with the v1 schema (no updated_at/last_surfaced),
// then calls Open() and verifies the schema_version is bumped to "2" and the columns exist.
func TestMigrationFromV1(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "v1.sqlite")

	// Create a v1-style database directly without going through Init().
	// This mirrors an existing pre-v2 brain file.
	s, err := buildV1Database(t, path)
	if err != nil {
		t.Fatalf("buildV1Database: %v", err)
	}
	_ = s.Close()

	// Re-open via Open() — migrateSchema() should run.
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("Open v1 database: %v", err)
	}
	defer func() { _ = s2.Close() }()

	ver, err := s2.GetMeta("schema_version")
	if err != nil {
		t.Fatalf("GetMeta after migration: %v", err)
	}
	if ver != "6" {
		t.Fatalf("expected schema_version=6 after migration, got %q", ver)
	}

	// Verify the columns exist and are queryable.
	_, err = s2.db.Exec(`SELECT updated_at, last_surfaced FROM nodes LIMIT 1`)
	if err != nil {
		t.Fatalf("columns updated_at/last_surfaced missing after migration: %v", err)
	}
}

// buildV1Database creates a raw SQLite database with the v1 schema (no updated_at/last_surfaced).
// Returns an open Store. Caller must Close().
func buildV1Database(t *testing.T, path string) (*Store, error) {
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
			last_reinforced DATETIME
		)`,
		`CREATE TABLE IF NOT EXISTS edges (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source_id TEXT NOT NULL,
			target_id TEXT NOT NULL,
			relation TEXT NOT NULL,
			weight REAL DEFAULT 1.0,
			metadata JSON,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS nodes_archive (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			subtype TEXT,
			content TEXT NOT NULL,
			metadata JSON,
			importance REAL,
			status TEXT,
			archive_reason TEXT,
			original_created_at DATETIME,
			archived_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`INSERT OR IGNORE INTO schema_meta (key, value) VALUES ('schema_version', '1')`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			_ = s.Close()
			return nil, fmt.Errorf("building v1 schema: %w", err)
		}
	}
	return s, nil
}

// TestMigrationIdempotency verifies re-opening a current-version database does
// not error and leaves the schema version unchanged.
func TestMigrationIdempotency(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "current.sqlite")

	// Create a current-version database.
	s, err := Init(path)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	_ = s.Close()

	// Open it again — migrateSchema() should be a no-op for the version bump
	// (initFTSTable is idempotent CREATE … IF NOT EXISTS).
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("Open database: %v", err)
	}
	defer func() { _ = s2.Close() }()

	ver, err := s2.GetMeta("schema_version")
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if ver != "6" {
		t.Fatalf("expected schema_version=6, got %q", ver)
	}
}

// TestUpdateNodeSetsUpdatedAtOnContentChange verifies that UpdateNode with Content
// sets updated_at and that it is recent.
func TestUpdateNodeSetsUpdatedAtOnContentChange(t *testing.T) {
	s := testStore(t)

	// Use 2 seconds before to account for SQLite's 1-second CURRENT_TIMESTAMP resolution.
	before := time.Now().UTC().Add(-2 * time.Second)

	id, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "original", Importance: 0.5})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}

	newContent := "updated content"
	if err := s.UpdateNode(id, UpdateNodeOpts{Content: &newContent}); err != nil {
		t.Fatalf("UpdateNode: %v", err)
	}

	node, err := s.GetNode(id)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}

	if node.UpdatedAt == nil {
		t.Fatal("expected UpdatedAt to be set after content update, got nil")
	}
	if node.UpdatedAt.Before(before) {
		t.Errorf("UpdatedAt %v is before test start %v", node.UpdatedAt, before)
	}
}

// TestUpdateNodeNoUpdatedAtOnImportanceChange verifies that importance-only
// updates do NOT set updated_at.
func TestUpdateNodeNoUpdatedAtOnImportanceChange(t *testing.T) {
	s := testStore(t)

	id, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "original", Importance: 0.5})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}

	newImportance := 0.9
	if err := s.UpdateNode(id, UpdateNodeOpts{Importance: &newImportance}); err != nil {
		t.Fatalf("UpdateNode importance: %v", err)
	}

	node, err := s.GetNode(id)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}

	if node.UpdatedAt != nil {
		t.Errorf("expected UpdatedAt=nil for importance-only update, got %v", node.UpdatedAt)
	}
}

// TestUpdateNodeNoUpdatedAtOnMetadataChange verifies that metadata-only
// updates do NOT set updated_at.
func TestUpdateNodeNoUpdatedAtOnMetadataChange(t *testing.T) {
	s := testStore(t)

	id, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "original", Importance: 0.5})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}

	if err := s.UpdateNode(id, UpdateNodeOpts{Metadata: json.RawMessage(`{"key":"val"}`)}); err != nil {
		t.Fatalf("UpdateNode metadata: %v", err)
	}

	node, err := s.GetNode(id)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}

	if node.UpdatedAt != nil {
		t.Errorf("expected UpdatedAt=nil for metadata-only update, got %v", node.UpdatedAt)
	}
}

// TestAddNodeHasNilUpdatedAt verifies that newly-added nodes have nil updated_at.
func TestAddNodeHasNilUpdatedAt(t *testing.T) {
	s := testStore(t)

	id, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "fresh", Importance: 0.5})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}

	node, err := s.GetNode(id)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}

	if node.UpdatedAt != nil {
		t.Errorf("expected UpdatedAt=nil for new node, got %v", node.UpdatedAt)
	}
	if node.LastSurfaced != nil {
		t.Errorf("expected LastSurfaced=nil for new node, got %v", node.LastSurfaced)
	}
}

// TestTouchSurfacedBatchUpdate verifies TouchSurfaced sets last_surfaced for all IDs.
func TestTouchSurfacedBatchUpdate(t *testing.T) {
	s := testStore(t)

	before := time.Now().UTC().Add(-2 * time.Second)

	id1, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "c1", Importance: 0.5})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	id2, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "c2", Importance: 0.5})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	id3, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "c3", Importance: 0.5})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}

	// Touch only id1 and id2
	if err := s.TouchSurfaced([]string{id1, id2}); err != nil {
		t.Fatalf("TouchSurfaced: %v", err)
	}

	n1, _ := s.GetNode(id1)
	n2, _ := s.GetNode(id2)
	n3, _ := s.GetNode(id3)

	if n1.LastSurfaced == nil {
		t.Error("expected n1.LastSurfaced to be set")
	} else if n1.LastSurfaced.Before(before) {
		t.Errorf("n1.LastSurfaced %v is before test start", n1.LastSurfaced)
	}

	if n2.LastSurfaced == nil {
		t.Error("expected n2.LastSurfaced to be set")
	} else if n2.LastSurfaced.Before(before) {
		t.Errorf("n2.LastSurfaced %v is before test start", n2.LastSurfaced)
	}

	if n3.LastSurfaced != nil {
		t.Errorf("expected n3.LastSurfaced=nil (not touched), got %v", n3.LastSurfaced)
	}
}

// TestTouchSurfacedEmptySlice verifies no error on empty slice.
func TestTouchSurfacedEmptySlice(t *testing.T) {
	s := testStore(t)

	if err := s.TouchSurfaced([]string{}); err != nil {
		t.Fatalf("TouchSurfaced empty: %v", err)
	}
}

// TestListNodesOrderByRecentlyChanged verifies the "recently_changed" OrderBy value.
func TestListNodesOrderByRecentlyChanged(t *testing.T) {
	s := testStore(t)

	id1, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "oldest", Importance: 0.5})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	id2, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "newest", Importance: 0.5})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}

	// Update id1 after id2, so it should appear first in recently_changed order.
	// Force updated_at to a known-future time since SQLite has 1s resolution.
	newContent := "oldest-but-recently-updated"
	if err := s.UpdateNode(id1, UpdateNodeOpts{Content: &newContent}); err != nil {
		t.Fatalf("UpdateNode: %v", err)
	}
	// Force id1's updated_at to be clearly in the future relative to id2's created_at.
	futureTS := time.Now().UTC().Add(time.Hour).Format("2006-01-02 15:04:05")
	if _, err := s.db.Exec(`UPDATE nodes SET updated_at = ? WHERE id = ?`, futureTS, id1); err != nil {
		t.Fatalf("force updated_at: %v", err)
	}

	nodes, err := s.ListNodes(ListNodesOpts{
		Status:  "active",
		OrderBy: "recently_changed",
	})
	if err != nil {
		t.Fatalf("ListNodes recently_changed: %v", err)
	}

	if len(nodes) < 2 {
		t.Fatalf("expected at least 2 nodes, got %d", len(nodes))
	}

	// id1 was updated most recently, so it should be first.
	if nodes[0].ID != id1 {
		t.Errorf("expected first node to be id1 (most recently changed), got content=%q", nodes[0].Content)
	}
	if nodes[1].ID != id2 {
		t.Errorf("expected second node to be id2, got content=%q", nodes[1].Content)
	}
}

// TestListNodesOrderByRecentlyChangedFallsBackToCreatedAt verifies nodes
// without updated_at use created_at in the COALESCE.
func TestListNodesOrderByRecentlyChangedFallsBackToCreatedAt(t *testing.T) {
	s := testStore(t)

	// Add nodes without any content update — they have no updated_at.
	// Created in order: first, then second.
	// We manipulate created_at directly since SQLite CURRENT_TIMESTAMP has 1s resolution.
	id1, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "first", Importance: 0.5})
	if err != nil {
		t.Fatalf("AddNode first: %v", err)
	}
	// Force "second" to have a later created_at by setting it explicitly.
	id2, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "second", Importance: 0.5})
	if err != nil {
		t.Fatalf("AddNode second: %v", err)
	}
	// Set id2's created_at to be 1 hour in the future to ensure ordering.
	future := time.Now().UTC().Add(time.Hour).Format("2006-01-02 15:04:05")
	if _, err := s.db.Exec(`UPDATE nodes SET created_at = ? WHERE id = ?`, future, id2); err != nil {
		t.Fatalf("force created_at: %v", err)
	}

	nodes, err := s.ListNodes(ListNodesOpts{
		Status:  "active",
		OrderBy: "recently_changed",
	})
	if err != nil {
		t.Fatalf("ListNodes recently_changed: %v", err)
	}

	if len(nodes) < 2 {
		t.Fatalf("expected at least 2 nodes, got %d", len(nodes))
	}

	// "second" was created later, should appear first.
	if nodes[0].ID != id2 {
		t.Errorf("expected 'second' node first (most recently created), got %q id1=%s id2=%s first_id=%s",
			nodes[0].Content, id1[:8], id2[:8], nodes[0].ID[:8])
	}
}

// TestListNodesSinceChanged filters on COALESCE(updated_at, created_at).
func TestListNodesSinceChanged(t *testing.T) {
	s := testStore(t)

	// Use explicit timestamps to avoid SQLite's 1-second CURRENT_TIMESTAMP resolution.
	past := time.Now().UTC().Add(-2 * time.Hour)
	future := time.Now().UTC().Add(2 * time.Hour)

	// Add a node with an "old" timestamp.
	id1, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "old", Importance: 0.5})
	if err != nil {
		t.Fatalf("AddNode old: %v", err)
	}
	if _, err := s.db.Exec(`UPDATE nodes SET created_at = ? WHERE id = ?`,
		past.Format("2006-01-02 15:04:05"), id1); err != nil {
		t.Fatalf("set old created_at: %v", err)
	}

	// Add a node with a "new" (future) timestamp.
	id2, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "new", Importance: 0.5})
	if err != nil {
		t.Fatalf("AddNode new: %v", err)
	}
	if _, err := s.db.Exec(`UPDATE nodes SET created_at = ? WHERE id = ?`,
		future.Format("2006-01-02 15:04:05"), id2); err != nil {
		t.Fatalf("set new created_at: %v", err)
	}

	cutoff := time.Now().UTC()
	nodes, err := s.ListNodes(ListNodesOpts{
		Status:       "active",
		SinceChanged: &cutoff,
	})
	if err != nil {
		t.Fatalf("ListNodes SinceChanged: %v", err)
	}

	if len(nodes) != 1 {
		t.Fatalf("expected 1 node since cutoff, got %d", len(nodes))
	}
	if nodes[0].ID != id2 {
		t.Errorf("expected 'new' node, got %q", nodes[0].Content)
	}
}

// TestListNodesSinceChangedPicksUpUpdatedAt verifies that a node updated after
// the cutoff appears in SinceChanged results even if created before cutoff.
func TestListNodesSinceChangedPicksUpUpdatedAt(t *testing.T) {
	s := testStore(t)

	past := time.Now().UTC().Add(-2 * time.Hour)
	future := time.Now().UTC().Add(2 * time.Hour)

	// Add the node and set its created_at to the past (before cutoff).
	id1, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "original", Importance: 0.5})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	if _, err := s.db.Exec(`UPDATE nodes SET created_at = ? WHERE id = ?`,
		past.Format("2006-01-02 15:04:05"), id1); err != nil {
		t.Fatalf("set past created_at: %v", err)
	}

	// Update the node's content to set updated_at, then force it to a future timestamp.
	newContent := "updated after cutoff"
	if err := s.UpdateNode(id1, UpdateNodeOpts{Content: &newContent}); err != nil {
		t.Fatalf("UpdateNode: %v", err)
	}
	if _, err := s.db.Exec(`UPDATE nodes SET updated_at = ? WHERE id = ?`,
		future.Format("2006-01-02 15:04:05"), id1); err != nil {
		t.Fatalf("set future updated_at: %v", err)
	}

	cutoff := time.Now().UTC()
	nodes, err := s.ListNodes(ListNodesOpts{
		Status:       "active",
		SinceChanged: &cutoff,
	})
	if err != nil {
		t.Fatalf("ListNodes SinceChanged: %v", err)
	}

	if len(nodes) != 1 {
		t.Fatalf("expected 1 node since cutoff (updated_at takes over), got %d", len(nodes))
	}
	if nodes[0].ID != id1 {
		t.Errorf("expected updated node, got %q", nodes[0].Content)
	}
}

// TestExportImportRoundTripWithNewColumns verifies that updated_at and last_surfaced
// survive an export/import round-trip.
func TestExportImportRoundTripWithNewColumns(t *testing.T) {
	src := testStore(t)

	id, err := src.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "original", Importance: 0.7})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}

	// Set updated_at by doing a content update.
	newContent := "updated"
	if err := src.UpdateNode(id, UpdateNodeOpts{Content: &newContent}); err != nil {
		t.Fatalf("UpdateNode: %v", err)
	}

	// Set last_surfaced.
	if err := src.TouchSurfaced([]string{id}); err != nil {
		t.Fatalf("TouchSurfaced: %v", err)
	}

	srcNode, err := src.GetNode(id)
	if err != nil {
		t.Fatalf("GetNode src: %v", err)
	}
	if srcNode.UpdatedAt == nil {
		t.Fatal("src node: UpdatedAt should be set")
	}
	if srcNode.LastSurfaced == nil {
		t.Fatal("src node: LastSurfaced should be set")
	}

	bundle, err := src.Export()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	dst := testStore(t)
	result, err := dst.Import(bundle, ImportOptions{OnConflict: ConflictSkip})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if result.NodesImported != 1 {
		t.Errorf("expected 1 node imported, got %d", result.NodesImported)
	}

	dstNode, err := dst.GetNode(id)
	if err != nil {
		t.Fatalf("GetNode dst: %v", err)
	}

	if dstNode.UpdatedAt == nil {
		t.Fatal("dst node: UpdatedAt should survive round-trip")
	}
	if !dstNode.UpdatedAt.Equal(*srcNode.UpdatedAt) {
		t.Errorf("UpdatedAt mismatch: src=%v dst=%v", srcNode.UpdatedAt, dstNode.UpdatedAt)
	}

	if dstNode.LastSurfaced == nil {
		t.Fatal("dst node: LastSurfaced should survive round-trip")
	}
	if !dstNode.LastSurfaced.Equal(*srcNode.LastSurfaced) {
		t.Errorf("LastSurfaced mismatch: src=%v dst=%v", srcNode.LastSurfaced, dstNode.LastSurfaced)
	}
}

// TestImportV1BundleIntoV2SchemaHandlesNilColumns verifies that importing a
// bundle whose nodes have nil UpdatedAt and LastSurfaced works without error.
func TestImportV1BundleIntoV2SchemaHandlesNilColumns(t *testing.T) {
	dst := testStore(t)

	// Construct a v1-style bundle: nodes have zero-value / nil timestamps.
	bundle := &ExportBundle{
		Version: ExportVersion,
		Nodes: []Node{
			{
				ID:           "test-node-v1",
				Type:         TypeConcept,
				Content:      "v1 node",
				Importance:   0.6,
				DecayRate:    0.02,
				Status:       "active",
				UpdatedAt:    nil, // v1 nodes have no updated_at
				LastSurfaced: nil,
			},
		},
	}

	result, err := dst.Import(bundle, ImportOptions{OnConflict: ConflictSkip})
	if err != nil {
		t.Fatalf("Import v1 bundle: %v", err)
	}
	if result.NodesImported != 1 {
		t.Errorf("expected 1 node imported, got %d", result.NodesImported)
	}

	node, err := dst.GetNode("test-node-v1")
	if err != nil {
		t.Fatalf("GetNode after import: %v", err)
	}
	if node.UpdatedAt != nil {
		t.Errorf("expected UpdatedAt=nil for v1 import, got %v", node.UpdatedAt)
	}
	if node.LastSurfaced != nil {
		t.Errorf("expected LastSurfaced=nil for v1 import, got %v", node.LastSurfaced)
	}
}
