package store

import (
	"testing"
)

// ptr returns a pointer to the given string value, for test readability.
func ptr(s string) *string { return &s }

// --- UpdateNode subtype tests [OO-011] ---

// TestUpdateNode_SubtypeSet verifies that a node with NULL subtype can have its
// subtype set to a non-empty value, and that updated_at is stamped in the process.
func TestUpdateNode_SubtypeSet(t *testing.T) {
	s := testStore(t)

	id, err := s.AddNode(&AddNodeOpts{
		Type:    TypeEpisode,
		Content: "original content",
	})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}

	// Verify initial subtype is empty
	node, err := s.GetNode(id)
	if err != nil {
		t.Fatalf("GetNode before update: %v", err)
	}
	if node.Subtype != "" {
		t.Fatalf("expected empty subtype initially, got %q", node.Subtype)
	}
	updatedAtBefore := node.UpdatedAt

	// Set subtype
	if err := s.UpdateNode(id, UpdateNodeOpts{Subtype: ptr("routing-discovery")}); err != nil {
		t.Fatalf("UpdateNode (set subtype): %v", err)
	}

	// Verify subtype was set and updated_at was stamped
	node, err = s.GetNode(id)
	if err != nil {
		t.Fatalf("GetNode after update: %v", err)
	}
	if node.Subtype != "routing-discovery" {
		t.Errorf("expected subtype=routing-discovery, got %q", node.Subtype)
	}
	if node.UpdatedAt == nil {
		t.Error("expected updated_at to be set after subtype change")
	}
	// updated_at must have changed (was nil before, now set)
	_ = updatedAtBefore // was nil; now non-nil is the assertion above
}

// TestUpdateNode_SubtypeClear verifies that setting Subtype to &"" clears the
// subtype to NULL in the database.
func TestUpdateNode_SubtypeClear(t *testing.T) {
	s := testStore(t)

	id, err := s.AddNode(&AddNodeOpts{
		Type:    TypeEpisode,
		Subtype: "routing-discovery",
		Content: "some content",
	})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}

	// Verify initial subtype
	node, err := s.GetNode(id)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if node.Subtype != "routing-discovery" {
		t.Fatalf("expected subtype=routing-discovery initially, got %q", node.Subtype)
	}

	// Clear subtype with empty string
	emptyStr := ""
	if err := s.UpdateNode(id, UpdateNodeOpts{Subtype: &emptyStr}); err != nil {
		t.Fatalf("UpdateNode (clear subtype): %v", err)
	}

	// Verify subtype is now NULL (empty string)
	node, err = s.GetNode(id)
	if err != nil {
		t.Fatalf("GetNode after clear: %v", err)
	}
	if node.Subtype != "" {
		t.Errorf("expected empty subtype after clear, got %q", node.Subtype)
	}
}

// TestUpdateNode_SubtypeUnchanged verifies that when Subtype is nil in UpdateNodeOpts,
// the existing subtype value is left untouched.
func TestUpdateNode_SubtypeUnchanged(t *testing.T) {
	s := testStore(t)

	id, err := s.AddNode(&AddNodeOpts{
		Type:    TypeConcept,
		Subtype: "operator-safety",
		Content: "important rule",
	})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}

	// Update content only; Subtype is nil
	content := "updated important rule"
	if err := s.UpdateNode(id, UpdateNodeOpts{Content: &content}); err != nil {
		t.Fatalf("UpdateNode (content only): %v", err)
	}

	// Verify subtype is unchanged
	node, err := s.GetNode(id)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if node.Subtype != "operator-safety" {
		t.Errorf("expected subtype=operator-safety unchanged, got %q", node.Subtype)
	}
	if node.Content != "updated important rule" {
		t.Errorf("expected updated content, got %q", node.Content)
	}
}

// --- ListNodes subtype filter tests [OO-011] ---

// TestListNodes_FilterBySubtype verifies the three-way subtype filter:
// - &"X" returns only nodes with subtype = "X"
// - &""  returns only nodes with NULL subtype
// - nil  returns all nodes regardless of subtype
// Also includes an adversarial SQL injection sub-case per Security review S-OO11-8.
func TestListNodes_FilterBySubtype(t *testing.T) {
	s := testStore(t)

	// Add 3 nodes with subtype "X"
	xIDs := make([]string, 3)
	for i := range xIDs {
		id, err := s.AddNode(&AddNodeOpts{
			Type:    TypeConcept,
			Subtype: "X",
			Content: "concept with subtype X",
		})
		if err != nil {
			t.Fatalf("AddNode (X): %v", err)
		}
		xIDs[i] = id
	}

	// Add 2 nodes with NULL subtype
	for range 2 {
		if _, err := s.AddNode(&AddNodeOpts{
			Type:    TypeEpisode,
			Content: "node with null subtype",
		}); err != nil {
			t.Fatalf("AddNode (null subtype): %v", err)
		}
	}

	// Test &"X" returns 3 nodes
	nodes, err := s.ListNodes(ListNodesOpts{Subtype: ptr("X")})
	if err != nil {
		t.Fatalf("ListNodes(Subtype=X): %v", err)
	}
	if len(nodes) != 3 {
		t.Errorf("expected 3 nodes with subtype=X, got %d", len(nodes))
	}
	for _, n := range nodes {
		if n.Subtype != "X" {
			t.Errorf("expected all returned nodes to have subtype=X, got %q", n.Subtype)
		}
	}

	// Test &"" returns 2 NULL-subtype nodes
	emptyStr := ""
	nodes, err = s.ListNodes(ListNodesOpts{Subtype: &emptyStr})
	if err != nil {
		t.Fatalf("ListNodes(Subtype=\"\"): %v", err)
	}
	if len(nodes) != 2 {
		t.Errorf("expected 2 NULL-subtype nodes, got %d", len(nodes))
	}
	for _, n := range nodes {
		if n.Subtype != "" {
			t.Errorf("expected all NULL-subtype nodes, got subtype=%q", n.Subtype)
		}
	}

	// Test nil returns all 5 nodes
	nodes, err = s.ListNodes(ListNodesOpts{})
	if err != nil {
		t.Fatalf("ListNodes(no filter): %v", err)
	}
	if len(nodes) != 5 {
		t.Errorf("expected all 5 nodes with nil filter, got %d", len(nodes))
	}

	// Adversarial sub-case [Security S-OO11-8]:
	// An SQL injection payload must return 0 rows AND leave the nodes table intact.
	injectionPayload := "'; DROP TABLE nodes;--"
	nodes, err = s.ListNodes(ListNodesOpts{Subtype: &injectionPayload})
	if err != nil {
		t.Fatalf("ListNodes(injection payload): unexpected error: %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("expected 0 rows for injection payload, got %d", len(nodes))
	}
	// Verify nodes table still intact
	stats, err := s.GetStats()
	if err != nil {
		t.Fatalf("GetStats after injection attempt: %v (nodes table may have been dropped)", err)
	}
	if stats.TotalNodes != 5 {
		t.Errorf("expected nodes table intact with 5 nodes, got %d total", stats.TotalNodes)
	}
}

// TestListNodes_SubtypeComposesWithTypeAndStatus verifies that --type, --subtype,
// and --status all compose as AND (AC scenario 4).
func TestListNodes_SubtypeComposesWithTypeAndStatus(t *testing.T) {
	s := testStore(t)

	// Add nodes with various combinations
	// 2 active procedures with subtype "routing-discovery"
	for range 2 {
		if _, err := s.AddNode(&AddNodeOpts{
			Type:    TypeProcedure,
			Subtype: "routing-discovery",
			Content: "a routing discovery procedure",
		}); err != nil {
			t.Fatalf("AddNode (procedure+routing-discovery): %v", err)
		}
	}
	// 1 active concept with subtype "routing-discovery" (different type)
	if _, err := s.AddNode(&AddNodeOpts{
		Type:    TypeConcept,
		Subtype: "routing-discovery",
		Content: "a routing discovery concept",
	}); err != nil {
		t.Fatalf("AddNode (concept+routing-discovery): %v", err)
	}
	// 1 active procedure without subtype
	if _, err := s.AddNode(&AddNodeOpts{
		Type:    TypeProcedure,
		Content: "a generic procedure",
	}); err != nil {
		t.Fatalf("AddNode (procedure, no subtype): %v", err)
	}

	// Filter by type=procedure AND subtype=routing-discovery AND status=active
	nodes, err := s.ListNodes(ListNodesOpts{
		Type:    TypeProcedure,
		Subtype: ptr("routing-discovery"),
		Status:  "active",
	})
	if err != nil {
		t.Fatalf("ListNodes (all three filters): %v", err)
	}
	if len(nodes) != 2 {
		t.Errorf("expected 2 nodes matching all three filters, got %d", len(nodes))
	}
	for _, n := range nodes {
		if n.Type != TypeProcedure {
			t.Errorf("expected type=procedure, got %s", n.Type)
		}
		if n.Subtype != "routing-discovery" {
			t.Errorf("expected subtype=routing-discovery, got %q", n.Subtype)
		}
		if n.Status != "active" {
			t.Errorf("expected status=active, got %q", n.Status)
		}
	}
}

// ---- agentic-h6gc: pending-embedding selection + stats predicate ----

func TestPendingEmbeddingNodes_NoVecRowDefinition(t *testing.T) {
	s := testStore(t)
	if err := s.InitVectorTable(4); err != nil {
		t.Fatalf("InitVectorTable: %v", err)
	}
	withVec, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "has vector", Importance: 0.5, EmbeddingModel: "m"})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	if err := s.StoreEmbedding(withVec, []float32{1, 0, 0, 0}); err != nil {
		t.Fatalf("StoreEmbedding: %v", err)
	}
	// The undercount case the stats predicate missed: embedding_model SET
	// (the write path stamps it before embedding) but no vec row landed.
	noVec, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "no vector", Importance: 0.5, EmbeddingModel: "m"})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	archived, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "archived", Importance: 0.5})
	if _, err := s.db.Exec(`UPDATE nodes SET status='archived' WHERE id=?`, archived); err != nil {
		t.Fatalf("archiving: %v", err)
	}

	pending, err := s.PendingEmbeddingNodes()
	if err != nil {
		t.Fatalf("PendingEmbeddingNodes: %v", err)
	}
	ids := map[string]bool{}
	for _, n := range pending {
		ids[n.ID] = true
	}
	if !ids[noVec] {
		t.Error("vec-less active node not selected (the embedding_model='' undercount)")
	}
	if ids[withVec] {
		t.Error("vectorized node wrongly selected")
	}
	if ids[archived] {
		t.Error("non-active node wrongly selected")
	}

	stats, err := s.GetStats()
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.PendingEmbeddings != len(pending) {
		t.Errorf("stats.PendingEmbeddings=%d disagrees with PendingEmbeddingNodes=%d (predicates must match)", stats.PendingEmbeddings, len(pending))
	}
}
