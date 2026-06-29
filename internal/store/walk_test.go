package store

import (
	"testing"
)

// addWalkNode is a tiny helper that adds a node and returns its ID.
func addWalkNode(t *testing.T, s *Store, content string) string {
	t.Helper()
	id, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: content, Importance: 0.5})
	if err != nil {
		t.Fatalf("AddNode(%q): %v", content, err)
	}
	return id
}

// depthOf returns the depth recorded for id in a NodeWithDepth slice, or -1.
func depthOf(nodes []NodeWithDepth, id string) int {
	for i := range nodes {
		if nodes[i].ID == id {
			return nodes[i].Depth
		}
	}
	return -1
}

// idsOf collects the node IDs from a NodeWithDepth slice in order.
func idsOf(nodes []NodeWithDepth) []string {
	out := make([]string, len(nodes))
	for i := range nodes {
		out[i] = nodes[i].ID
	}
	return out
}

// TestWalkRelationChain walks a depth-3 chain A->B->C->D via derived_from and
// asserts each node is returned exactly once with its BFS depth, in BFS order.
func TestWalkRelationChain(t *testing.T) {
	s := testStore(t)
	a := addWalkNode(t, s, "A")
	b := addWalkNode(t, s, "B")
	c := addWalkNode(t, s, "C")
	d := addWalkNode(t, s, "D")
	for _, p := range [][2]string{{a, b}, {b, c}, {c, d}} {
		if _, err := s.AddEdge(p[0], p[1], RelationDerivedFrom, AddEdgeOpts{}); err != nil {
			t.Fatalf("AddEdge: %v", err)
		}
	}

	got, err := s.WalkRelation(a, RelationDerivedFrom, 5, true)
	if err != nil {
		t.Fatalf("WalkRelation: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("expected 4 nodes, got %d (%v)", len(got), idsOf(got))
	}
	if depthOf(got, a) != 0 || depthOf(got, b) != 1 || depthOf(got, c) != 2 || depthOf(got, d) != 3 {
		t.Fatalf("bad depths: a=%d b=%d c=%d d=%d", depthOf(got, a), depthOf(got, b), depthOf(got, c), depthOf(got, d))
	}
	// Start node must be first.
	if got[0].ID != a {
		t.Fatalf("expected start node first, got %s", got[0].ID[:8])
	}
}

// TestWalkRelationDepthCap asserts maxDepth truncates a chain longer than the cap.
func TestWalkRelationDepthCap(t *testing.T) {
	s := testStore(t)
	a := addWalkNode(t, s, "A")
	b := addWalkNode(t, s, "B")
	c := addWalkNode(t, s, "C")
	d := addWalkNode(t, s, "D")
	for _, p := range [][2]string{{a, b}, {b, c}, {c, d}} {
		if _, err := s.AddEdge(p[0], p[1], RelationDerivedFrom, AddEdgeOpts{}); err != nil {
			t.Fatalf("AddEdge: %v", err)
		}
	}

	got, err := s.WalkRelation(a, RelationDerivedFrom, 2, true)
	if err != nil {
		t.Fatalf("WalkRelation: %v", err)
	}
	// depth 2 => A@0, B@1, C@2; D@3 truncated.
	if len(got) != 3 {
		t.Fatalf("expected 3 nodes at maxDepth=2, got %d (%v)", len(got), idsOf(got))
	}
	if depthOf(got, d) != -1 {
		t.Fatalf("D should be truncated at maxDepth=2, got depth %d", depthOf(got, d))
	}
}

// TestWalkRelationDepthZero asserts maxDepth=0 returns only the start node.
func TestWalkRelationDepthZero(t *testing.T) {
	s := testStore(t)
	a := addWalkNode(t, s, "A")
	b := addWalkNode(t, s, "B")
	if _, err := s.AddEdge(a, b, RelationDerivedFrom, AddEdgeOpts{}); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}

	got, err := s.WalkRelation(a, RelationDerivedFrom, 0, true)
	if err != nil {
		t.Fatalf("WalkRelation: %v", err)
	}
	if len(got) != 1 || got[0].ID != a || got[0].Depth != 0 {
		t.Fatalf("maxDepth=0 should return [start@0], got %v", idsOf(got))
	}
}

// TestWalkRelationNegativeDepth asserts a negative maxDepth also returns [start@0].
func TestWalkRelationNegativeDepth(t *testing.T) {
	s := testStore(t)
	a := addWalkNode(t, s, "A")
	b := addWalkNode(t, s, "B")
	if _, err := s.AddEdge(a, b, RelationDerivedFrom, AddEdgeOpts{}); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}
	got, err := s.WalkRelation(a, RelationDerivedFrom, -3, true)
	if err != nil {
		t.Fatalf("WalkRelation: %v", err)
	}
	if len(got) != 1 || got[0].ID != a {
		t.Fatalf("negative maxDepth should return [start@0], got %v", idsOf(got))
	}
}

// TestWalkRelationCycle asserts a cycle A->B->C->A terminates and returns each
// node exactly once (the BFS-in-Go visited-set property; a recursive CTE would
// re-walk the cycle to the cap).
func TestWalkRelationCycle(t *testing.T) {
	s := testStore(t)
	a := addWalkNode(t, s, "A")
	b := addWalkNode(t, s, "B")
	c := addWalkNode(t, s, "C")
	for _, p := range [][2]string{{a, b}, {b, c}, {c, a}} {
		if _, err := s.AddEdge(p[0], p[1], RelationDerivedFrom, AddEdgeOpts{}); err != nil {
			t.Fatalf("AddEdge: %v", err)
		}
	}

	got, err := s.WalkRelation(a, RelationDerivedFrom, 10, true)
	if err != nil {
		t.Fatalf("WalkRelation: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("cycle should return each node once (3), got %d (%v)", len(got), idsOf(got))
	}
	if depthOf(got, a) != 0 || depthOf(got, b) != 1 || depthOf(got, c) != 2 {
		t.Fatalf("cycle depths wrong: a=%d b=%d c=%d", depthOf(got, a), depthOf(got, b), depthOf(got, c))
	}
}

// TestWalkRelationSelfLoop asserts a self-loop A derived_from A returns [A@0]
// and terminates (A is pre-seeded in the visited set).
func TestWalkRelationSelfLoop(t *testing.T) {
	s := testStore(t)
	a := addWalkNode(t, s, "A")
	if _, err := s.AddEdge(a, a, RelationDerivedFrom, AddEdgeOpts{}); err != nil {
		t.Fatalf("AddEdge self-loop: %v", err)
	}
	got, err := s.WalkRelation(a, RelationDerivedFrom, 5, true)
	if err != nil {
		t.Fatalf("WalkRelation: %v", err)
	}
	if len(got) != 1 || got[0].ID != a || got[0].Depth != 0 {
		t.Fatalf("self-loop should return [A@0], got %v", idsOf(got))
	}
}

// TestWalkRelationDirection asserts outgoing=true walks source->target and
// outgoing=false walks target->source.
func TestWalkRelationDirection(t *testing.T) {
	s := testStore(t)
	a := addWalkNode(t, s, "A")
	b := addWalkNode(t, s, "B")
	if _, err := s.AddEdge(a, b, RelationDerivedFrom, AddEdgeOpts{}); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}

	// Outgoing from A reaches B.
	outFromA, _ := s.WalkRelation(a, RelationDerivedFrom, 5, true)
	if len(outFromA) != 2 || depthOf(outFromA, b) != 1 {
		t.Fatalf("outgoing from A should reach B@1, got %v", idsOf(outFromA))
	}
	// Outgoing from B reaches nobody (B is a leaf for outgoing).
	outFromB, _ := s.WalkRelation(b, RelationDerivedFrom, 5, true)
	if len(outFromB) != 1 {
		t.Fatalf("outgoing from B should be [B@0], got %v", idsOf(outFromB))
	}
	// Incoming (outgoing=false) from B reaches A.
	inFromB, _ := s.WalkRelation(b, RelationDerivedFrom, 5, false)
	if len(inFromB) != 2 || depthOf(inFromB, a) != 1 {
		t.Fatalf("incoming from B should reach A@1, got %v", idsOf(inFromB))
	}
}

// TestWalkRelationFiltersByRelation asserts edges of OTHER relations are ignored.
func TestWalkRelationFiltersByRelation(t *testing.T) {
	s := testStore(t)
	a := addWalkNode(t, s, "A")
	b := addWalkNode(t, s, "B")
	c := addWalkNode(t, s, "C")
	if _, err := s.AddEdge(a, b, RelationDerivedFrom, AddEdgeOpts{}); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}
	if _, err := s.AddEdge(a, c, "relates_to", AddEdgeOpts{}); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}

	got, _ := s.WalkRelation(a, RelationDerivedFrom, 5, true)
	if len(got) != 2 || depthOf(got, c) != -1 {
		t.Fatalf("walk should follow only derived_from (A,B), got %v", idsOf(got))
	}
}

// TestWalkRelationNoEdges asserts a node with no matching edges returns [start@0].
func TestWalkRelationNoEdges(t *testing.T) {
	s := testStore(t)
	a := addWalkNode(t, s, "A")
	got, err := s.WalkRelation(a, RelationDerivedFrom, 5, true)
	if err != nil {
		t.Fatalf("WalkRelation: %v", err)
	}
	if len(got) != 1 || got[0].ID != a {
		t.Fatalf("no-edges node should return [start@0], got %v", idsOf(got))
	}
}

// TestWalkRelationUnknownStart asserts an unknown start node is an error
// (matching GetNode's contract — TL ratification).
func TestWalkRelationUnknownStart(t *testing.T) {
	s := testStore(t)
	_, err := s.WalkRelation("does-not-exist", RelationDerivedFrom, 5, true)
	if err == nil {
		t.Fatal("expected error for unknown start node, got nil")
	}
}

// TestWalkRelationDiamondMinDepth asserts a node reachable by two paths records
// its MINIMUM (BFS) depth, and appears exactly once.
func TestWalkRelationDiamondMinDepth(t *testing.T) {
	s := testStore(t)
	a := addWalkNode(t, s, "A")
	b := addWalkNode(t, s, "B")
	c := addWalkNode(t, s, "C")
	d := addWalkNode(t, s, "D")
	// A->B, A->C, B->D, C->D : D reachable at depth 2 by both paths.
	for _, p := range [][2]string{{a, b}, {a, c}, {b, d}, {c, d}} {
		if _, err := s.AddEdge(p[0], p[1], RelationDerivedFrom, AddEdgeOpts{}); err != nil {
			t.Fatalf("AddEdge: %v", err)
		}
	}
	got, _ := s.WalkRelation(a, RelationDerivedFrom, 5, true)
	if len(got) != 4 {
		t.Fatalf("diamond should return 4 nodes, got %d (%v)", len(got), idsOf(got))
	}
	if depthOf(got, d) != 2 {
		t.Fatalf("D min depth should be 2, got %d", depthOf(got, d))
	}
}
