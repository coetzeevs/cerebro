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

	s.Close()

	// Open existing database
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s2.Close()
}

func TestAddAndGetNode(t *testing.T) {
	s := testStore(t)

	id, err := s.AddNode(AddNodeOpts{
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

	oldID, err := s.AddNode(AddNodeOpts{
		Type:       TypeConcept,
		Content:    "old fact",
		Importance: 0.5,
	})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}

	newID, err := s.SupersedeNode(oldID, AddNodeOpts{
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

	id, _ := s.AddNode(AddNodeOpts{Type: TypeConcept, Content: "test", Importance: 0.5})

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

	s.AddNode(AddNodeOpts{Type: TypeEpisode, Content: "ep1", Importance: 0.5})
	s.AddNode(AddNodeOpts{Type: TypeConcept, Content: "c1", Importance: 0.5})
	s.AddNode(AddNodeOpts{Type: TypeConcept, Content: "c2", Importance: 0.5})

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

	id1, _ := s.AddNode(AddNodeOpts{Type: TypeEpisode, Content: "ep1", Importance: 0.5})
	id2, _ := s.AddNode(AddNodeOpts{Type: TypeEpisode, Content: "ep2", Importance: 0.5})

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

	s.AddNode(AddNodeOpts{Type: TypeEpisode, Content: "ep1", Importance: 0.5})
	s.AddNode(AddNodeOpts{Type: TypeConcept, Content: "c1", Importance: 0.5})
	s.AddEdge("a", "b", "relates_to") // edges with nonexistent nodes won't be added due to FK

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

// testStore creates a temporary store for testing.
func testStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.sqlite")
	s, err := Init(path)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
