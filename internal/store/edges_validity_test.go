package store

import (
	"testing"
	"time"
)

// seedTwoNodes creates two concept nodes and returns their IDs.
func seedTwoNodes(t *testing.T, s *Store) (idA, idB string) {
	t.Helper()
	a, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "node A", Importance: 0.5})
	if err != nil {
		t.Fatalf("AddNode A: %v", err)
	}
	b, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "node B", Importance: 0.5})
	if err != nil {
		t.Fatalf("AddNode B: %v", err)
	}
	return a, b
}

func tp(tm time.Time) *time.Time { return &tm }

// TestAddEdgeStoresValidityWindow (AC2) verifies AddEdge persists valid_at and
// invalid_at and that they round-trip via sql.NullTime as the exact stored UTC
// instants (TL-PI-N3 — the write->read round-trip the SQL probe could not prove).
func TestAddEdgeStoresValidityWindow(t *testing.T) {
	s := testStore(t)
	idA, idB := seedTwoNodes(t, s)

	va := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ia := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	if _, err := s.AddEdge(idA, idB, "relates_to", AddEdgeOpts{ValidAt: tp(va), InvalidAt: tp(ia)}); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}

	edges, err := s.getEdgesForNode(idA, nil)
	if err != nil {
		t.Fatalf("getEdgesForNode: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	e := edges[0]
	if e.ValidAt == nil {
		t.Fatal("ValidAt is nil, expected stored value")
	}
	if !e.ValidAt.UTC().Equal(va) {
		t.Errorf("ValidAt mismatch: got %v want %v", e.ValidAt.UTC(), va)
	}
	if e.InvalidAt == nil {
		t.Fatal("InvalidAt is nil, expected stored value")
	}
	if !e.InvalidAt.UTC().Equal(ia) {
		t.Errorf("InvalidAt mismatch: got %v want %v", e.InvalidAt.UTC(), ia)
	}
}

// TestAddEdgeNilBoundsStoreNull (AC2/AC4) verifies that omitting a bound stores
// SQL NULL (a nil *time.Time), the open-ended universal case.
func TestAddEdgeNilBoundsStoreNull(t *testing.T) {
	s := testStore(t)
	idA, idB := seedTwoNodes(t, s)

	// valid_at set, invalid_at omitted (open-ended upper bound).
	va := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	if _, err := s.AddEdge(idA, idB, "relates_to", AddEdgeOpts{ValidAt: tp(va)}); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}
	edges, err := s.getEdgesForNode(idA, nil)
	if err != nil {
		t.Fatalf("getEdgesForNode: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if edges[0].ValidAt == nil {
		t.Error("ValidAt should be set")
	}
	if edges[0].InvalidAt != nil {
		t.Errorf("InvalidAt should be NULL (nil), got %v", edges[0].InvalidAt)
	}
}

// TestAddEdgeBothNilOpenEnded (AC4) verifies the today's-universal-case edge
// (both bounds NULL) stores nil/nil.
func TestAddEdgeBothNilOpenEnded(t *testing.T) {
	s := testStore(t)
	idA, idB := seedTwoNodes(t, s)

	if _, err := s.AddEdge(idA, idB, "relates_to", AddEdgeOpts{}); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}
	edges, err := s.getEdgesForNode(idA, nil)
	if err != nil {
		t.Fatalf("getEdgesForNode: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if edges[0].ValidAt != nil || edges[0].InvalidAt != nil {
		t.Errorf("expected both bounds NULL, got valid=%v invalid=%v", edges[0].ValidAt, edges[0].InvalidAt)
	}
}

// asOfWindowFixture builds a store with five edges from a common source whose
// windows mirror the design's live-probed fixture (Assumption #16). It returns
// the source id and a map of relation->edge so tests can assert membership.
//
//	E1 (NULL,NULL)               open both ways
//	E2 [2026-01-01, 2026-06-01)  bounded both ways
//	E3 [2026-03-01, NULL)        open upper
//	E4 (NULL, 2026-02-01)        open lower
//	E5 [2026-04-01, 2026-04-01)  zero-width — valid at no instant
func asOfWindowFixture(t *testing.T, s *Store) (src string) {
	t.Helper()
	src, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "src", Importance: 0.5})
	if err != nil {
		t.Fatalf("AddNode src: %v", err)
	}
	// Distinct targets so each edge is a distinct (src,tgt,rel) row.
	mk := func(label string) string {
		id, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: label, Importance: 0.5})
		if err != nil {
			t.Fatalf("AddNode %s: %v", label, err)
		}
		return id
	}
	t1 := mk("t1")
	t2 := mk("t2")
	t3 := mk("t3")
	t4 := mk("t4")
	t5 := mk("t5")

	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	feb1 := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	mar1 := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	apr1 := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	jun1 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	if _, err := s.AddEdge(src, t1, "E1", AddEdgeOpts{}); err != nil {
		t.Fatalf("E1: %v", err)
	}
	if _, err := s.AddEdge(src, t2, "E2", AddEdgeOpts{ValidAt: tp(jan1), InvalidAt: tp(jun1)}); err != nil {
		t.Fatalf("E2: %v", err)
	}
	if _, err := s.AddEdge(src, t3, "E3", AddEdgeOpts{ValidAt: tp(mar1)}); err != nil {
		t.Fatalf("E3: %v", err)
	}
	if _, err := s.AddEdge(src, t4, "E4", AddEdgeOpts{InvalidAt: tp(feb1)}); err != nil {
		t.Fatalf("E4: %v", err)
	}
	if _, err := s.AddEdge(src, t5, "E5", AddEdgeOpts{ValidAt: tp(apr1), InvalidAt: tp(apr1)}); err != nil {
		t.Fatalf("E5: %v", err)
	}
	return src
}

// relationSet collects the relation names of a slice of edges.
func relationSet(edges []Edge) map[string]bool {
	out := make(map[string]bool, len(edges))
	for i := range edges {
		out[edges[i].Relation] = true
	}
	return out
}

// TestGetEdgesForNodeAsOfBoundaries (AC3) asserts the exact membership at the
// half-open boundaries that the design live-probed:
//
//	==valid_at INCLUDED, ==invalid_at EXCLUDED, zero-width [t,t) matches NOTHING.
func TestGetEdgesForNodeAsOfBoundaries(t *testing.T) {
	s := testStore(t)
	src := asOfWindowFixture(t, s)

	cases := []struct {
		name string
		asOf time.Time
		want map[string]bool
	}{
		{
			name: "before any bounded window (2025-12-01)",
			asOf: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC),
			// E1 (open), E4 (open lower, upper=feb1 > dec) match.
			want: map[string]bool{"E1": true, "E4": true},
		},
		{
			name: "== E2.valid_at lower-closed (2026-01-01 included)",
			asOf: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			// E1, E2 (==valid_at included), E4 (jan < feb1).
			want: map[string]bool{"E1": true, "E2": true, "E4": true},
		},
		{
			name: "== E2.invalid_at upper-open (2026-06-01 excluded)",
			asOf: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
			// E1, E3 (valid from mar1, open upper). E2 EXCLUDED (==invalid_at).
			want: map[string]bool{"E1": true, "E3": true},
		},
		{
			name: "== E5 zero-width [apr1,apr1) matches nothing of E5 (2026-04-01)",
			asOf: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
			// E1, E2 (jan<=apr<jun), E3 (mar<=apr). E5 EXCLUDED (zero-width).
			want: map[string]bool{"E1": true, "E2": true, "E3": true},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			asOf := tc.asOf
			edges, err := s.getEdgesForNode(src, &asOf)
			if err != nil {
				t.Fatalf("getEdgesForNode: %v", err)
			}
			got := relationSet(edges)
			if len(got) != len(tc.want) {
				t.Errorf("count mismatch: got %v want %v", got, tc.want)
			}
			for rel := range tc.want {
				if !got[rel] {
					t.Errorf("expected %s in result, got %v", rel, got)
				}
			}
			for rel := range got {
				if !tc.want[rel] {
					t.Errorf("unexpected %s in result, want %v", rel, tc.want)
				}
			}
		})
	}
}

// TestGetEdgesForNodeAsOfNilIsUnfiltered (AC4) verifies asOf==nil returns ALL
// edges (predicate omitted) — the non-regression floor.
func TestGetEdgesForNodeAsOfNilIsUnfiltered(t *testing.T) {
	s := testStore(t)
	src := asOfWindowFixture(t, s)

	edges, err := s.getEdgesForNode(src, nil)
	if err != nil {
		t.Fatalf("getEdgesForNode: %v", err)
	}
	got := relationSet(edges)
	for _, rel := range []string{"E1", "E2", "E3", "E4", "E5"} {
		if !got[rel] {
			t.Errorf("asOf=nil should return %s; got %v", rel, got)
		}
	}
	if len(got) != 5 {
		t.Errorf("asOf=nil expected all 5 edges, got %d: %v", len(got), got)
	}
}

// TestGetEdgesBatchAsOf (AC3) asserts the same predicate applies through the
// batch query that feeds ExpandGraph.
func TestGetEdgesBatchAsOf(t *testing.T) {
	s := testStore(t)
	src := asOfWindowFixture(t, s)

	// At 2026-06-01: E2 excluded (==invalid_at), E4 excluded (feb1<=jun), E5
	// excluded (zero-width). E1 + E3 remain.
	asOf := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	batch, err := s.GetEdgesBatch([]string{src}, &asOf)
	if err != nil {
		t.Fatalf("GetEdgesBatch: %v", err)
	}
	got := relationSet(batch[src])
	want := map[string]bool{"E1": true, "E3": true}
	if len(got) != len(want) {
		t.Errorf("count mismatch: got %v want %v", got, want)
	}
	for rel := range want {
		if !got[rel] {
			t.Errorf("expected %s, got %v", rel, got)
		}
	}

	// asOf=nil through the batch path returns all 5 (AC4 non-regression).
	all, err := s.GetEdgesBatch([]string{src}, nil)
	if err != nil {
		t.Fatalf("GetEdgesBatch nil: %v", err)
	}
	if len(all[src]) != 5 {
		t.Errorf("batch asOf=nil expected 5 edges, got %d", len(all[src]))
	}
}

// TestNullNullEdgeMatchesEveryAsOf (AC4) — the open/open edge matches any
// instant: the migration-compatibility guarantee for existing edges.
func TestNullNullEdgeMatchesEveryAsOf(t *testing.T) {
	s := testStore(t)
	idA, idB := seedTwoNodes(t, s)
	if _, err := s.AddEdge(idA, idB, "open", AddEdgeOpts{}); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}
	for _, instant := range []time.Time{
		time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC),
		time.Date(2999, 12, 31, 0, 0, 0, 0, time.UTC),
	} {
		asOf := instant
		edges, err := s.getEdgesForNode(idA, &asOf)
		if err != nil {
			t.Fatalf("getEdgesForNode at %v: %v", instant, err)
		}
		if len(edges) != 1 {
			t.Errorf("NULL/NULL edge should match asOf=%v, got %d edges", instant, len(edges))
		}
	}
}

// TestAddEdgeUpsertUpdatesWindowInPlace (AC6) — re-adding an existing edge
// updates the window in place: id retained, no duplicate row, RETURNING id
// returns the persisted id (TL-PI-N4).
func TestAddEdgeUpsertUpdatesWindowInPlace(t *testing.T) {
	s := testStore(t)
	idA, idB := seedTwoNodes(t, s)

	v1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	i1 := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	firstID, err := s.AddEdge(idA, idB, "relates_to", AddEdgeOpts{ValidAt: tp(v1), InvalidAt: tp(i1)})
	if err != nil {
		t.Fatalf("first AddEdge: %v", err)
	}
	if firstID == 0 {
		t.Fatal("first AddEdge returned id 0")
	}

	// Re-add with a new window [T3, T4).
	v2 := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	i2 := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	secondID, err := s.AddEdge(idA, idB, "relates_to", AddEdgeOpts{ValidAt: tp(v2), InvalidAt: tp(i2)})
	if err != nil {
		t.Fatalf("re-AddEdge: %v", err)
	}

	// TL-PI-N4: the re-add must return the SAME persisted id (never 0/stale).
	if secondID != firstID {
		t.Errorf("upsert returned id %d, expected the existing id %d (TL-PI-N4)", secondID, firstID)
	}

	// No duplicate row.
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM edges`).Scan(&count); err != nil {
		t.Fatalf("count edges: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 edge after re-add (no duplicate), got %d", count)
	}

	// Window updated in place.
	edges, err := s.getEdgesForNode(idA, nil)
	if err != nil {
		t.Fatalf("getEdgesForNode: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if edges[0].ID != firstID {
		t.Errorf("edge id changed: got %d want %d", edges[0].ID, firstID)
	}
	if edges[0].ValidAt == nil || !edges[0].ValidAt.UTC().Equal(v2) {
		t.Errorf("ValidAt not updated to %v: got %v", v2, edges[0].ValidAt)
	}
	if edges[0].InvalidAt == nil || !edges[0].InvalidAt.UTC().Equal(i2) {
		t.Errorf("InvalidAt not updated to %v: got %v", i2, edges[0].InvalidAt)
	}
}

// TestAddEdgeUpsertNullOverwritesNonNull (AC6) — re-adding with only valid_at
// (invalid_at omitted) OVERWRITES the prior non-NULL invalid_at to NULL. This is
// the documented full-window-re-assertion semantic (NOT a partial patch).
func TestAddEdgeUpsertNullOverwritesNonNull(t *testing.T) {
	s := testStore(t)
	idA, idB := seedTwoNodes(t, s)

	v1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	i1 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if _, err := s.AddEdge(idA, idB, "relates_to", AddEdgeOpts{ValidAt: tp(v1), InvalidAt: tp(i1)}); err != nil {
		t.Fatalf("first AddEdge: %v", err)
	}

	// Re-add with only valid_at — invalid_at must be cleared to NULL.
	v2 := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	if _, err := s.AddEdge(idA, idB, "relates_to", AddEdgeOpts{ValidAt: tp(v2)}); err != nil {
		t.Fatalf("re-AddEdge: %v", err)
	}

	edges, err := s.getEdgesForNode(idA, nil)
	if err != nil {
		t.Fatalf("getEdgesForNode: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if edges[0].ValidAt == nil || !edges[0].ValidAt.UTC().Equal(v2) {
		t.Errorf("ValidAt should be updated to %v, got %v", v2, edges[0].ValidAt)
	}
	if edges[0].InvalidAt != nil {
		t.Errorf("InvalidAt should be cleared to NULL on full-window re-assertion, got %v", edges[0].InvalidAt)
	}
}
