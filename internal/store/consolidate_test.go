package store

import (
	"testing"
)

// edgeCount returns the number of edges of a given relation from source.
func derivedEdgeCount(t *testing.T, s *Store, source string) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM edges WHERE source_id = ? AND relation = ?`, source, RelationDerivedFrom,
	).Scan(&n); err != nil {
		t.Fatalf("counting derived_from edges: %v", err)
	}
	return n
}

func statusOf(t *testing.T, s *Store, id string) string {
	t.Helper()
	var st string
	if err := s.db.QueryRow(`SELECT status FROM nodes WHERE id = ?`, id).Scan(&st); err != nil {
		t.Fatalf("reading status of %s: %v", id, err)
	}
	return st
}

// TestConsolidateIntoWritesEdgesAndFlips asserts ConsolidateInto writes a
// derived_from edge from the into-node to each source episode and flips each
// episode to consolidated (AC3).
func TestConsolidateIntoWritesEdgesAndFlips(t *testing.T) {
	s := testStore(t)
	concept, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "synthesis", Importance: 0.8})
	e1, _ := s.AddNode(&AddNodeOpts{Type: TypeEpisode, Content: "ep1", Importance: 0.5})
	e2, _ := s.AddNode(&AddNodeOpts{Type: TypeEpisode, Content: "ep2", Importance: 0.5})

	if err := s.ConsolidateInto(concept, []string{e1, e2}); err != nil {
		t.Fatalf("ConsolidateInto: %v", err)
	}

	if derivedEdgeCount(t, s, concept) != 2 {
		t.Fatalf("expected 2 derived_from edges, got %d", derivedEdgeCount(t, s, concept))
	}
	if statusOf(t, s, e1) != "consolidated" || statusOf(t, s, e2) != "consolidated" {
		t.Fatalf("episodes not consolidated: e1=%s e2=%s", statusOf(t, s, e1), statusOf(t, s, e2))
	}

	// Edges must point concept -> episode (provenance direction).
	chain, err := s.WalkRelation(concept, RelationDerivedFrom, 5, true)
	if err != nil {
		t.Fatalf("WalkRelation: %v", err)
	}
	if len(chain) != 3 { // concept + 2 episodes
		t.Fatalf("walk from concept should reach 2 sources, got %d", len(chain))
	}
}

// TestConsolidateIntoIdempotent asserts re-running produces no duplicate edges
// and no error (the UNIQUE(source,target,relation) upsert).
func TestConsolidateIntoIdempotent(t *testing.T) {
	s := testStore(t)
	concept, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "synthesis", Importance: 0.8})
	e1, _ := s.AddNode(&AddNodeOpts{Type: TypeEpisode, Content: "ep1", Importance: 0.5})

	if err := s.ConsolidateInto(concept, []string{e1}); err != nil {
		t.Fatalf("ConsolidateInto (1): %v", err)
	}
	if err := s.ConsolidateInto(concept, []string{e1}); err != nil {
		t.Fatalf("ConsolidateInto (2, idempotent): %v", err)
	}
	if got := derivedEdgeCount(t, s, concept); got != 1 {
		t.Fatalf("re-run produced duplicate edges: count=%d, want 1", got)
	}
}

// TestConsolidateIntoMissingIntoFailsClosed asserts a missing into-node fails
// with non-zero error and writes nothing.
func TestConsolidateIntoMissingIntoFailsClosed(t *testing.T) {
	s := testStore(t)
	e1, _ := s.AddNode(&AddNodeOpts{Type: TypeEpisode, Content: "ep1", Importance: 0.5})

	err := s.ConsolidateInto("nonexistent-concept", []string{e1})
	if err == nil {
		t.Fatal("expected error for missing into-node, got nil")
	}
	// No edge, no status flip.
	if derivedEdgeCount(t, s, "nonexistent-concept") != 0 {
		t.Fatal("missing into-node wrote a derived_from edge")
	}
	if statusOf(t, s, e1) != "active" {
		t.Fatalf("episode flipped despite failure: status=%s", statusOf(t, s, e1))
	}
}

// TestConsolidateIntoMissingSourceFailsClosed asserts an unresolvable source
// rejects the WHOLE command with zero partial write (rollback verified).
func TestConsolidateIntoMissingSourceFailsClosed(t *testing.T) {
	s := testStore(t)
	concept, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "synthesis", Importance: 0.8})
	e1, _ := s.AddNode(&AddNodeOpts{Type: TypeEpisode, Content: "ep1", Importance: 0.5})

	// e1 is valid but the second source does not exist → reject all-or-nothing.
	err := s.ConsolidateInto(concept, []string{e1, "ghost-episode"})
	if err == nil {
		t.Fatal("expected error for unresolvable source, got nil")
	}
	// Rollback: NO edge written for the valid source either, e1 stays active.
	if derivedEdgeCount(t, s, concept) != 0 {
		t.Fatalf("partial write: %d edges after a failed consolidation, want 0", derivedEdgeCount(t, s, concept))
	}
	if statusOf(t, s, e1) != "active" {
		t.Fatalf("partial flip: e1 status=%s after failed consolidation, want active", statusOf(t, s, e1))
	}
}

// TestConsolidateIntoNonEpisodeSourceFailsClosed asserts a non-episode source is
// rejected (the consolidation predicate is type='episode').
func TestConsolidateIntoNonEpisodeSourceFailsClosed(t *testing.T) {
	s := testStore(t)
	concept, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "synthesis", Importance: 0.8})
	otherConcept, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "not an episode", Importance: 0.5})

	err := s.ConsolidateInto(concept, []string{otherConcept})
	if err == nil {
		t.Fatal("expected error consolidating a non-episode source, got nil")
	}
	if derivedEdgeCount(t, s, concept) != 0 {
		t.Fatal("non-episode source wrote a derived_from edge")
	}
}

// TestConsolidateIntoEmptySources asserts an empty source list is an error
// (nothing to consolidate is a misuse, not a silent no-op that flips nothing).
func TestConsolidateIntoEmptySources(t *testing.T) {
	s := testStore(t)
	concept, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "synthesis", Importance: 0.8})
	if err := s.ConsolidateInto(concept, nil); err == nil {
		t.Fatal("expected error for empty source list, got nil")
	}
}

// TestConsolidateIntoAlreadyConsolidatedReasserts asserts a source that is a
// valid episode but already consolidated re-asserts its edge idempotently
// without error (re-running a consolidation is safe).
func TestConsolidateIntoAlreadyConsolidatedReasserts(t *testing.T) {
	s := testStore(t)
	concept, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "synthesis", Importance: 0.8})
	e1, _ := s.AddNode(&AddNodeOpts{Type: TypeEpisode, Content: "ep1", Importance: 0.5})

	if err := s.ConsolidateInto(concept, []string{e1}); err != nil {
		t.Fatalf("first ConsolidateInto: %v", err)
	}
	// e1 is now consolidated; re-running must not error and must keep one edge.
	if err := s.ConsolidateInto(concept, []string{e1}); err != nil {
		t.Fatalf("re-consolidating an already-consolidated source errored: %v", err)
	}
	if got := derivedEdgeCount(t, s, concept); got != 1 {
		t.Fatalf("re-assert produced %d edges, want 1", got)
	}
}
