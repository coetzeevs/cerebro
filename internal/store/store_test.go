package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitAndOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sqlite")

	// Init creates the database
	s, err := Init(path)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("database file not created")
	}

	// Check schema version
	ver, err := s.GetMeta("schema_version")
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if ver != "1" {
		t.Fatalf("expected schema_version=1, got %q", ver)
	}

	_ = s.Close()

	// Open existing database
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	_ = s2.Close()
}

func TestAddAndGetNode(t *testing.T) {
	s := testStore(t)

	id, err := s.AddNode(&AddNodeOpts{
		Type:       TypeConcept,
		Content:    "test concept",
		Importance: 0.8,
	})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}

	node, err := s.GetNode(id)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}

	if node.Type != TypeConcept {
		t.Errorf("expected type=concept, got %s", node.Type)
	}
	if node.Content != "test concept" {
		t.Errorf("expected content='test concept', got %q", node.Content)
	}
	if node.Importance != 0.8 {
		t.Errorf("expected importance=0.8, got %f", node.Importance)
	}
	if node.Status != "active" {
		t.Errorf("expected status=active, got %s", node.Status)
	}
	if node.DecayRate != DefaultDecayRate(TypeConcept) {
		t.Errorf("expected decay_rate=%f, got %f", DefaultDecayRate(TypeConcept), node.DecayRate)
	}
}

func TestSupersedeNode(t *testing.T) {
	s := testStore(t)

	oldID, err := s.AddNode(&AddNodeOpts{
		Type:       TypeConcept,
		Content:    "old fact",
		Importance: 0.5,
	})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}

	newID, err := s.SupersedeNode(oldID, &AddNodeOpts{
		Type:       TypeConcept,
		Content:    "new fact",
		Importance: 0.7,
	})
	if err != nil {
		t.Fatalf("SupersedeNode: %v", err)
	}

	// Old node should be superseded
	old, _ := s.GetNode(oldID)
	if old.Status != "superseded" {
		t.Errorf("old node status: expected superseded, got %s", old.Status)
	}

	// New node should be active
	new_, _ := s.GetNode(newID)
	if new_.Status != "active" {
		t.Errorf("new node status: expected active, got %s", new_.Status)
	}

	// Edge should exist
	nwe, _ := s.GetNodeWithEdges(newID)
	if len(nwe.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(nwe.Edges))
	}
	if nwe.Edges[0].Relation != "supersedes" {
		t.Errorf("expected edge relation=supersedes, got %s", nwe.Edges[0].Relation)
	}
}

func TestReinforceNode(t *testing.T) {
	s := testStore(t)

	id, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "test", Importance: 0.5})

	if err := s.ReinforceNode(id); err != nil {
		t.Fatalf("ReinforceNode: %v", err)
	}

	node, _ := s.GetNode(id)
	if node.AccessCount != 1 {
		t.Errorf("expected access_count=1, got %d", node.AccessCount)
	}
	if node.TimesReinforced != 1 {
		t.Errorf("expected times_reinforced=1, got %d", node.TimesReinforced)
	}
}

func TestListNodes(t *testing.T) {
	s := testStore(t)

	if _, err := s.AddNode(&AddNodeOpts{Type: TypeEpisode, Content: "ep1", Importance: 0.5}); err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	if _, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "c1", Importance: 0.5}); err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	if _, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "c2", Importance: 0.5}); err != nil {
		t.Fatalf("AddNode: %v", err)
	}

	// List all
	all, err := s.ListNodes(ListNodesOpts{})
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(all))
	}

	// Filter by type
	concepts, _ := s.ListNodes(ListNodesOpts{Type: TypeConcept})
	if len(concepts) != 2 {
		t.Errorf("expected 2 concepts, got %d", len(concepts))
	}

	// Limit
	limited, _ := s.ListNodes(ListNodesOpts{Limit: 1})
	if len(limited) != 1 {
		t.Errorf("expected 1 node, got %d", len(limited))
	}
}

func TestMarkConsolidated(t *testing.T) {
	s := testStore(t)

	id1, _ := s.AddNode(&AddNodeOpts{Type: TypeEpisode, Content: "ep1", Importance: 0.5})
	id2, _ := s.AddNode(&AddNodeOpts{Type: TypeEpisode, Content: "ep2", Importance: 0.5})

	if err := s.MarkConsolidated([]string{id1, id2}); err != nil {
		t.Fatalf("MarkConsolidated: %v", err)
	}

	n1, _ := s.GetNode(id1)
	n2, _ := s.GetNode(id2)
	if n1.Status != "consolidated" {
		t.Errorf("node1 status: expected consolidated, got %s", n1.Status)
	}
	if n2.Status != "consolidated" {
		t.Errorf("node2 status: expected consolidated, got %s", n2.Status)
	}
}

func TestStats(t *testing.T) {
	s := testStore(t)

	if _, err := s.AddNode(&AddNodeOpts{Type: TypeEpisode, Content: "ep1", Importance: 0.5}); err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	if _, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "c1", Importance: 0.5}); err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	if _, err := s.AddEdge("a", "b", "relates_to"); err != nil {
		// Expected: edges with nonexistent nodes won't be added due to FK
		_ = err
	}

	stats, err := s.GetStats()
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}

	if stats.TotalNodes != 2 {
		t.Errorf("expected 2 total nodes, got %d", stats.TotalNodes)
	}
	if stats.ActiveNodes != 2 {
		t.Errorf("expected 2 active nodes, got %d", stats.ActiveNodes)
	}
	if stats.NodesByType["episode"] != 1 {
		t.Errorf("expected 1 episode, got %d", stats.NodesByType["episode"])
	}
}

func TestUpdateNode(t *testing.T) {
	s := testStore(t)

	id, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "original", Importance: 0.5})

	// Update content
	newContent := "updated content"
	if err := s.UpdateNode(id, UpdateNodeOpts{Content: &newContent}); err != nil {
		t.Fatalf("UpdateNode content: %v", err)
	}

	node, _ := s.GetNode(id)
	if node.Content != "updated content" {
		t.Errorf("expected content='updated content', got %q", node.Content)
	}

	// Update importance
	newImportance := 0.9
	if err := s.UpdateNode(id, UpdateNodeOpts{Importance: &newImportance}); err != nil {
		t.Fatalf("UpdateNode importance: %v", err)
	}

	node, _ = s.GetNode(id)
	if node.Importance != 0.9 {
		t.Errorf("expected importance=0.9, got %f", node.Importance)
	}

	// Update both at once
	bothContent := "both updated"
	bothImportance := 0.3
	if err := s.UpdateNode(id, UpdateNodeOpts{Content: &bothContent, Importance: &bothImportance}); err != nil {
		t.Fatalf("UpdateNode both: %v", err)
	}

	node, _ = s.GetNode(id)
	if node.Content != "both updated" || node.Importance != 0.3 {
		t.Errorf("expected content='both updated' importance=0.3, got %q %f", node.Content, node.Importance)
	}
}

func TestSetAndGetMeta(t *testing.T) {
	s := testStore(t)

	if err := s.SetMeta("test_key", "test_value"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}

	val, err := s.GetMeta("test_key")
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if val != "test_value" {
		t.Errorf("expected 'test_value', got %q", val)
	}

	// Overwrite
	if err := s.SetMeta("test_key", "new_value"); err != nil {
		t.Fatalf("SetMeta overwrite: %v", err)
	}

	val, _ = s.GetMeta("test_key")
	if val != "new_value" {
		t.Errorf("expected 'new_value', got %q", val)
	}

	// Nonexistent key returns empty string
	val, err = s.GetMeta("nonexistent")
	if err != nil {
		t.Fatalf("GetMeta nonexistent: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty string for nonexistent key, got %q", val)
	}
}

func TestDefaultDecayRate(t *testing.T) {
	tests := []struct {
		nodeType NodeType
		expected float64
	}{
		{TypeEpisode, 0.15},
		{TypeConcept, 0.02},
		{TypeProcedure, 0.005},
		{TypeReflection, 0.05},
		{NodeType("unknown"), 0.1},
	}

	for _, tt := range tests {
		got := DefaultDecayRate(tt.nodeType)
		if got != tt.expected {
			t.Errorf("DefaultDecayRate(%s) = %f, want %f", tt.nodeType, got, tt.expected)
		}
	}
}

func TestOpenNonexistent(t *testing.T) {
	_, err := Open(filepath.Join(t.TempDir(), "nope.sqlite"))
	if err == nil {
		t.Fatal("expected error opening nonexistent database")
	}
}

func TestPathAndDB(t *testing.T) {
	s := testStore(t)

	if s.Path() == "" {
		t.Error("expected non-empty path")
	}
	if s.DB() == nil {
		t.Error("expected non-nil DB")
	}
}

func TestAddEdge(t *testing.T) {
	s := testStore(t)

	id1, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "n1", Importance: 0.5})
	id2, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "n2", Importance: 0.5})

	edgeID, err := s.AddEdge(id1, id2, "relates_to")
	if err != nil {
		t.Fatalf("AddEdge: %v", err)
	}
	if edgeID == 0 {
		t.Error("expected non-zero edge ID")
	}

	// Duplicate should not error (ON CONFLICT DO NOTHING)
	_, err = s.AddEdge(id1, id2, "relates_to")
	if err != nil {
		t.Fatalf("AddEdge duplicate: %v", err)
	}

	// Different relation should create new edge
	edgeID2, err := s.AddEdge(id1, id2, "supports")
	if err != nil {
		t.Fatalf("AddEdge different relation: %v", err)
	}
	if edgeID2 == 0 {
		t.Error("expected non-zero edge ID for different relation")
	}
}

func TestGetNodeWithEdges(t *testing.T) {
	s := testStore(t)

	id1, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "n1", Importance: 0.5})
	id2, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "n2", Importance: 0.5})
	id3, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "n3", Importance: 0.5})

	if _, err := s.AddEdge(id1, id2, "relates_to"); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}
	if _, err := s.AddEdge(id3, id1, "supports"); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}

	nwe, err := s.GetNodeWithEdges(id1)
	if err != nil {
		t.Fatalf("GetNodeWithEdges: %v", err)
	}
	if nwe.Content != "n1" {
		t.Errorf("expected content='n1', got %q", nwe.Content)
	}
	// id1 is source in one edge, target in another
	if len(nwe.Edges) != 2 {
		t.Errorf("expected 2 edges, got %d", len(nwe.Edges))
	}
}

func TestReinforceNonexistentNode(t *testing.T) {
	s := testStore(t)

	err := s.ReinforceNode("nonexistent-id")
	if err == nil {
		t.Fatal("expected error reinforcing nonexistent node")
	}
}

func TestSupersedeNonexistentNode(t *testing.T) {
	s := testStore(t)

	_, err := s.SupersedeNode("nonexistent-id", &AddNodeOpts{
		Type:       TypeConcept,
		Content:    "new",
		Importance: 0.5,
	})
	if err == nil {
		t.Fatal("expected error superseding nonexistent node")
	}
}

func TestListNodesFilterByStatus(t *testing.T) {
	s := testStore(t)

	id1, _ := s.AddNode(&AddNodeOpts{Type: TypeEpisode, Content: "ep1", Importance: 0.5})
	if _, err := s.AddNode(&AddNodeOpts{Type: TypeEpisode, Content: "ep2", Importance: 0.5}); err != nil {
		t.Fatalf("AddNode: %v", err)
	}

	if err := s.MarkConsolidated([]string{id1}); err != nil {
		t.Fatalf("MarkConsolidated: %v", err)
	}

	active, _ := s.ListNodes(ListNodesOpts{Status: "active"})
	if len(active) != 1 {
		t.Errorf("expected 1 active node, got %d", len(active))
	}

	consolidated, _ := s.ListNodes(ListNodesOpts{Status: "consolidated"})
	if len(consolidated) != 1 {
		t.Errorf("expected 1 consolidated node, got %d", len(consolidated))
	}
}

func TestAddNodeDefaultImportance(t *testing.T) {
	s := testStore(t)

	id, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "test"})

	node, _ := s.GetNode(id)
	if node.Importance != 0.5 {
		t.Errorf("expected default importance=0.5, got %f", node.Importance)
	}
}

func TestAddNodeWithSubtypeAndMetadata(t *testing.T) {
	s := testStore(t)

	id, err := s.AddNode(&AddNodeOpts{
		Type:       TypeEpisode,
		Subtype:    "debug_session",
		Content:    "test",
		Metadata:   []byte(`{"key":"value"}`),
		Importance: 0.6,
	})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}

	node, _ := s.GetNode(id)
	if node.Subtype != "debug_session" {
		t.Errorf("expected subtype='debug_session', got %q", node.Subtype)
	}
	if string(node.Metadata) != `{"key":"value"}` {
		t.Errorf("expected metadata={\"key\":\"value\"}, got %s", node.Metadata)
	}
}

func TestStatsWithEdges(t *testing.T) {
	s := testStore(t)

	id1, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "n1", Importance: 0.5})
	id2, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "n2", Importance: 0.5})
	if _, err := s.AddEdge(id1, id2, "relates_to"); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}

	stats, err := s.GetStats()
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.TotalEdges != 1 {
		t.Errorf("expected 1 edge, got %d", stats.TotalEdges)
	}
}

func TestCloseNilDB(t *testing.T) {
	s := &Store{}
	if err := s.Close(); err != nil {
		t.Fatalf("Close on nil db: %v", err)
	}
}

func TestListNodesOrderByImportance(t *testing.T) {
	s := testStore(t)

	// Add nodes with different importance levels
	if _, err := s.AddNode(&AddNodeOpts{Type: TypeEpisode, Content: "low", Importance: 0.2}); err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	if _, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "high", Importance: 0.9}); err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	if _, err := s.AddNode(&AddNodeOpts{Type: TypeProcedure, Content: "mid", Importance: 0.5}); err != nil {
		t.Fatalf("AddNode: %v", err)
	}

	nodes, err := s.ListNodes(ListNodesOpts{
		Status:  "active",
		OrderBy: "importance",
		Limit:   3,
	})
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}

	if len(nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(nodes))
	}

	// Should be ordered by importance DESC
	if nodes[0].Content != "high" {
		t.Errorf("expected first node to be 'high' (importance 0.9), got %q (%.1f)", nodes[0].Content, nodes[0].Importance)
	}
	if nodes[1].Content != "mid" {
		t.Errorf("expected second node to be 'mid' (importance 0.5), got %q (%.1f)", nodes[1].Content, nodes[1].Importance)
	}
	if nodes[2].Content != "low" {
		t.Errorf("expected third node to be 'low' (importance 0.2), got %q (%.1f)", nodes[2].Content, nodes[2].Importance)
	}
}

func TestGetNodesByIDs(t *testing.T) {
	s := testStore(t)

	id1, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "concept one", Importance: 0.8})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	id2, err := s.AddNode(&AddNodeOpts{Type: TypeEpisode, Content: "episode one", Importance: 0.5})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	id3, err := s.AddNode(&AddNodeOpts{Type: TypeProcedure, Content: "procedure one", Importance: 0.7})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}

	// Fetch a subset
	nodes, err := s.GetNodesByIDs([]string{id1, id3})
	if err != nil {
		t.Fatalf("GetNodesByIDs: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}

	// Verify we got the right nodes (order not guaranteed)
	ids := map[string]bool{nodes[0].ID: true, nodes[1].ID: true}
	if !ids[id1] || !ids[id3] {
		t.Errorf("expected ids %s and %s, got %v", id1[:8], id3[:8], ids)
	}

	// Should not return consolidated nodes
	if err := s.MarkConsolidated([]string{id2}); err != nil {
		// id2 is episode, so this works
		t.Fatalf("MarkConsolidated: %v", err)
	}
	// Actually id2 is an episode and gets consolidated. But GetNodesByIDs
	// should only return active nodes.
	nodes2, err := s.GetNodesByIDs([]string{id1, id2, id3})
	if err != nil {
		t.Fatalf("GetNodesByIDs with consolidated: %v", err)
	}
	if len(nodes2) != 2 {
		t.Errorf("expected 2 active nodes (id2 consolidated), got %d", len(nodes2))
	}

	// Empty input should return empty
	empty, err := s.GetNodesByIDs([]string{})
	if err != nil {
		t.Fatalf("GetNodesByIDs empty: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("expected 0 nodes for empty input, got %d", len(empty))
	}

	// Nonexistent IDs should return nothing
	none, err := s.GetNodesByIDs([]string{"nonexistent-id"})
	if err != nil {
		t.Fatalf("GetNodesByIDs nonexistent: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("expected 0 nodes for nonexistent ID, got %d", len(none))
	}
}

func TestResolvePrefix(t *testing.T) {
	s := testStore(t)

	id1, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "alpha", Importance: 0.5})
	id2, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "beta", Importance: 0.5})

	// Full UUID resolves to itself
	resolved, err := s.ResolvePrefix(id1)
	if err != nil {
		t.Fatalf("ResolvePrefix full UUID: %v", err)
	}
	if resolved != id1 {
		t.Errorf("expected %s, got %s", id1, resolved)
	}

	// 8-char prefix of id1 should resolve (statistically unique)
	prefix := id1[:8]
	resolved, err = s.ResolvePrefix(prefix)
	if err != nil {
		t.Fatalf("ResolvePrefix 8-char: %v", err)
	}
	if resolved != id1 {
		t.Errorf("expected %s, got %s", id1, resolved)
	}

	// Nonexistent prefix should error (use short prefix, not full UUID)
	_, err = s.ResolvePrefix("zzzzzzzz")
	if err == nil {
		t.Fatal("expected error for nonexistent prefix")
	}

	// Empty prefix should error
	_, err = s.ResolvePrefix("")
	if err == nil {
		t.Fatal("expected error for empty prefix")
	}

	// Verify both nodes are independently resolvable
	resolved2, err := s.ResolvePrefix(id2[:8])
	if err != nil {
		t.Fatalf("ResolvePrefix id2: %v", err)
	}
	if resolved2 != id2 {
		t.Errorf("expected %s, got %s", id2, resolved2)
	}
}

// testStore creates a temporary store for testing.
func testStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.sqlite")
	s, err := Init(path)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
