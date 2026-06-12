//go:build fts5

package store

import (
	"testing"
)

// ftsRowCount returns the number of rows currently in nodes_fts.
func ftsRowCount(t *testing.T, s *Store) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM nodes_fts`).Scan(&n); err != nil {
		t.Fatalf("counting nodes_fts: %v", err)
	}
	return n
}

// ftsMatchCount returns the number of nodes_fts rows matching the given (already
// FTS5-safe) phrase. The phrase is wrapped as a literal FTS5 phrase here.
func ftsMatchCount(t *testing.T, s *Store, phrase string) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM nodes_fts WHERE nodes_fts MATCH ?`,
		`"`+phrase+`"`,
	).Scan(&n); err != nil {
		t.Fatalf("MATCH %q: %v", phrase, err)
	}
	return n
}

// TestAddNodePopulatesFTS (AC1b) — adding a node inserts a matching nodes_fts row.
func TestAddNodePopulatesFTS(t *testing.T) {
	s := testStore(t)

	before := ftsRowCount(t, s)
	id, err := s.AddNode(&AddNodeOpts{
		Type:    TypeProcedure,
		Subtype: "routing",
		Content: "widget alpha HS-049 details",
	})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	after := ftsRowCount(t, s)
	if after != before+1 {
		t.Fatalf("expected nodes_fts +1 (%d->%d)", before, after)
	}

	// The FTS row mirrors content + subtype and maps to the node id.
	var nodeID, content, subtype string
	if err := s.db.QueryRow(
		`SELECT node_id, content, subtype FROM nodes_fts WHERE node_id = ?`, id,
	).Scan(&nodeID, &content, &subtype); err != nil {
		t.Fatalf("reading nodes_fts row: %v", err)
	}
	if content != "widget alpha HS-049 details" {
		t.Fatalf("content mismatch: %q", content)
	}
	if subtype != "routing" {
		t.Fatalf("subtype mismatch: %q", subtype)
	}
	// The term is findable via MATCH.
	if got := ftsMatchCount(t, s, "widget"); got != 1 {
		t.Fatalf("MATCH widget = %d, want 1", got)
	}
}

// TestUpdateNodeContentUpdatesFTS (AC1c) — updating content refreshes nodes_fts;
// the old term no longer matches, the new term does.
func TestUpdateNodeContentUpdatesFTS(t *testing.T) {
	s := testStore(t)

	id, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "oldterm aaaaa"})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	if got := ftsMatchCount(t, s, "oldterm"); got != 1 {
		t.Fatalf("precondition: MATCH oldterm = %d, want 1", got)
	}

	newContent := "newterm bbbbb"
	if err := s.UpdateNode(id, UpdateNodeOpts{Content: &newContent}); err != nil {
		t.Fatalf("UpdateNode: %v", err)
	}

	if got := ftsMatchCount(t, s, "oldterm"); got != 0 {
		t.Fatalf("after update: MATCH oldterm = %d, want 0", got)
	}
	if got := ftsMatchCount(t, s, "newterm"); got != 1 {
		t.Fatalf("after update: MATCH newterm = %d, want 1", got)
	}
	// Exactly one FTS row remains for the node (no duplicate).
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM nodes_fts WHERE node_id = ?`, id).Scan(&n); err != nil {
		t.Fatalf("count by node_id: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected exactly 1 nodes_fts row for node, got %d", n)
	}
}

// TestUpdateNodeSubtypeUpdatesFTS (AC1c) — updating subtype refreshes the FTS row's subtype.
func TestUpdateNodeSubtypeUpdatesFTS(t *testing.T) {
	s := testStore(t)

	id, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "subj", Subtype: "before"})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	newSub := "after"
	if err := s.UpdateNode(id, UpdateNodeOpts{Subtype: &newSub}); err != nil {
		t.Fatalf("UpdateNode subtype: %v", err)
	}
	var subtype string
	if err := s.db.QueryRow(`SELECT subtype FROM nodes_fts WHERE node_id = ?`, id).Scan(&subtype); err != nil {
		t.Fatalf("reading subtype: %v", err)
	}
	if subtype != "after" {
		t.Fatalf("subtype not refreshed in FTS: %q", subtype)
	}
}

// TestSupersedeNodeSyncsFTS (AC1b/AC1c) — superseding inserts the new node into FTS
// within the same transaction; the new content is matchable.
func TestSupersedeNodeSyncsFTS(t *testing.T) {
	s := testStore(t)

	oldID, err := s.AddNode(&AddNodeOpts{Type: TypeEpisode, Content: "superseded-content zzz"})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	newID, err := s.SupersedeNode(oldID, &AddNodeOpts{Type: TypeEpisode, Content: "fresh-content yyy"})
	if err != nil {
		t.Fatalf("SupersedeNode: %v", err)
	}
	if got := ftsMatchCount(t, s, "fresh-content"); got != 1 {
		t.Fatalf("MATCH fresh-content = %d, want 1", got)
	}
	var content string
	if err := s.db.QueryRow(`SELECT content FROM nodes_fts WHERE node_id = ?`, newID).Scan(&content); err != nil {
		t.Fatalf("reading new node FTS row: %v", err)
	}
	if content != "fresh-content yyy" {
		t.Fatalf("new node content mismatch in FTS: %q", content)
	}
}

// TestGCDeleteRemovesFromFTS (AC1d) — the GC eviction (the only DELETE FROM nodes
// path) removes the node's nodes_fts row.
func TestGCDeleteRemovesFromFTS(t *testing.T) {
	s := testStore(t)

	id, err := s.AddNode(&AddNodeOpts{Type: TypeEpisode, Content: "evictme qqq"})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	if got := ftsMatchCount(t, s, "evictme"); got != 1 {
		t.Fatalf("precondition: MATCH evictme = %d, want 1", got)
	}

	// GC with a threshold above any retention score evicts everything.
	if _, err := s.GC(2.0, false); err != nil {
		t.Fatalf("GC: %v", err)
	}

	if got := ftsMatchCount(t, s, "evictme"); got != 0 {
		t.Fatalf("after GC: MATCH evictme = %d, want 0", got)
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM nodes_fts WHERE node_id = ?`, id).Scan(&n); err != nil {
		t.Fatalf("count by node_id: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected nodes_fts row removed for evicted node, got %d", n)
	}
}
