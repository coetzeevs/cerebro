package store

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestExportBundle(t *testing.T) {
	s := testStore(t)

	// Set up test data: nodes, edges, meta
	id1, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "concept one", Importance: 0.8})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	id2, err := s.AddNode(&AddNodeOpts{Type: TypeEpisode, Content: "episode one", Importance: 0.5, Metadata: json.RawMessage(`{"key":"val"}`)})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	if _, err := s.AddEdge(id1, id2, "relates_to"); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}
	if err := s.SetMeta("embedding_provider", "ollama"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}
	if err := s.SetMeta("embedding_model", "nomic-embed-text"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}

	bundle, err := s.Export()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	if bundle.Version != ExportVersion {
		t.Errorf("expected version=%s, got %s", ExportVersion, bundle.Version)
	}
	if bundle.ExportedAt.IsZero() {
		t.Error("expected non-zero ExportedAt")
	}
	if len(bundle.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(bundle.Nodes))
	}
	if len(bundle.Edges) != 1 {
		t.Errorf("expected 1 edge, got %d", len(bundle.Edges))
	}
	if bundle.Meta["embedding_provider"] != "ollama" {
		t.Errorf("expected meta embedding_provider=ollama, got %q", bundle.Meta["embedding_provider"])
	}
	if bundle.Meta["embedding_model"] != "nomic-embed-text" {
		t.Errorf("expected meta embedding_model=nomic-embed-text, got %q", bundle.Meta["embedding_model"])
	}
}

func TestExportIncludesAllStatuses(t *testing.T) {
	s := testStore(t)

	id1, _ := s.AddNode(&AddNodeOpts{Type: TypeEpisode, Content: "ep1", Importance: 0.5})
	if _, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "c1", Importance: 0.7}); err != nil {
		t.Fatalf("AddNode: %v", err)
	}

	// Mark one as consolidated
	if err := s.MarkConsolidated([]string{id1}); err != nil {
		t.Fatalf("MarkConsolidated: %v", err)
	}

	bundle, err := s.Export()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	// Should include both active and consolidated nodes
	if len(bundle.Nodes) != 2 {
		t.Errorf("expected 2 nodes (all statuses), got %d", len(bundle.Nodes))
	}
}

func TestExportJSON(t *testing.T) {
	s := testStore(t)

	if _, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "test", Importance: 0.5}); err != nil {
		t.Fatalf("AddNode: %v", err)
	}

	bundle, err := s.Export()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	data, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// Should round-trip cleanly
	var decoded ExportBundle
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(decoded.Nodes) != 1 {
		t.Errorf("expected 1 node after round-trip, got %d", len(decoded.Nodes))
	}
	if decoded.Version != ExportVersion {
		t.Errorf("expected version=%s after round-trip, got %s", ExportVersion, decoded.Version)
	}
}

func TestListAllEdges(t *testing.T) {
	s := testStore(t)

	id1, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "n1", Importance: 0.5})
	id2, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "n2", Importance: 0.5})
	id3, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "n3", Importance: 0.5})

	if _, err := s.AddEdge(id1, id2, "relates_to"); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}
	if _, err := s.AddEdge(id2, id3, "supports"); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}

	edges, err := s.ListAllEdges()
	if err != nil {
		t.Fatalf("ListAllEdges: %v", err)
	}
	if len(edges) != 2 {
		t.Errorf("expected 2 edges, got %d", len(edges))
	}
}

func TestListAllEdgesEmpty(t *testing.T) {
	s := testStore(t)

	edges, err := s.ListAllEdges()
	if err != nil {
		t.Fatalf("ListAllEdges: %v", err)
	}
	if len(edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(edges))
	}
}

func TestGetAllMeta(t *testing.T) {
	s := testStore(t)

	if err := s.SetMeta("key1", "val1"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}
	if err := s.SetMeta("key2", "val2"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}

	meta, err := s.GetAllMeta()
	if err != nil {
		t.Fatalf("GetAllMeta: %v", err)
	}

	if meta["key1"] != "val1" {
		t.Errorf("expected key1=val1, got %q", meta["key1"])
	}
	if meta["key2"] != "val2" {
		t.Errorf("expected key2=val2, got %q", meta["key2"])
	}
	// schema_version is set by Init
	if meta["schema_version"] != "1" {
		t.Errorf("expected schema_version=1, got %q", meta["schema_version"])
	}
}

func TestAddNodeWithID(t *testing.T) {
	s := testStore(t)

	err := s.AddNodeWithID("custom-id-123", &AddNodeOpts{
		Type:       TypeConcept,
		Content:    "imported concept",
		Importance: 0.7,
	})
	if err != nil {
		t.Fatalf("AddNodeWithID: %v", err)
	}

	node, err := s.GetNode("custom-id-123")
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if node.Content != "imported concept" {
		t.Errorf("expected content='imported concept', got %q", node.Content)
	}
	if node.Importance != 0.7 {
		t.Errorf("expected importance=0.7, got %f", node.Importance)
	}
}

func TestAddNodeWithIDConflictSkip(t *testing.T) {
	s := testStore(t)

	err := s.AddNodeWithID("dup-id", &AddNodeOpts{
		Type: TypeConcept, Content: "original", Importance: 0.5,
	})
	if err != nil {
		t.Fatalf("AddNodeWithID: %v", err)
	}

	// Insert again with skip — should not error, should keep original
	err = s.AddNodeWithID("dup-id", &AddNodeOpts{
		Type: TypeConcept, Content: "duplicate", Importance: 0.9,
	})
	if err != nil {
		t.Fatalf("AddNodeWithID skip duplicate: %v", err)
	}

	node, _ := s.GetNode("dup-id")
	if node.Content != "original" {
		t.Errorf("expected original content preserved, got %q", node.Content)
	}
}

func TestImportBundle(t *testing.T) {
	// Create source store with data
	src := testStore(t)
	id1, _ := src.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "concept", Importance: 0.8})
	id2, _ := src.AddNode(&AddNodeOpts{Type: TypeEpisode, Content: "episode", Importance: 0.5})
	if _, err := src.AddEdge(id1, id2, "relates_to"); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}

	bundle, err := src.Export()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	// Import into fresh store
	dst := testStore(t)
	result, err := dst.Import(bundle, ImportOptions{OnConflict: ConflictSkip})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	if result.NodesImported != 2 {
		t.Errorf("expected 2 nodes imported, got %d", result.NodesImported)
	}
	if result.EdgesImported != 1 {
		t.Errorf("expected 1 edge imported, got %d", result.EdgesImported)
	}
	if result.NodesSkipped != 0 {
		t.Errorf("expected 0 nodes skipped, got %d", result.NodesSkipped)
	}

	// Verify data is accessible
	node, err := dst.GetNode(id1)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if node.Content != "concept" {
		t.Errorf("expected content='concept', got %q", node.Content)
	}

	edges, err := dst.ListAllEdges()
	if err != nil {
		t.Fatalf("ListAllEdges: %v", err)
	}
	if len(edges) != 1 {
		t.Errorf("expected 1 edge, got %d", len(edges))
	}
}

func TestImportBundleConflictSkip(t *testing.T) {
	dst := testStore(t)

	// Pre-populate with a node
	if err := dst.AddNodeWithID("existing-id", &AddNodeOpts{
		Type: TypeConcept, Content: "original content", Importance: 0.5,
	}); err != nil {
		t.Fatalf("AddNodeWithID: %v", err)
	}

	// Build bundle with conflicting ID
	bundle := &ExportBundle{
		Version: ExportVersion,
		Nodes: []Node{
			{ID: "existing-id", Type: TypeConcept, Content: "imported content", Importance: 0.9, DecayRate: 0.02, Status: "active"},
			{ID: "new-id", Type: TypeEpisode, Content: "new episode", Importance: 0.5, DecayRate: 0.15, Status: "active"},
		},
	}

	result, err := dst.Import(bundle, ImportOptions{OnConflict: ConflictSkip})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	if result.NodesImported != 1 {
		t.Errorf("expected 1 node imported, got %d", result.NodesImported)
	}
	if result.NodesSkipped != 1 {
		t.Errorf("expected 1 node skipped, got %d", result.NodesSkipped)
	}

	// Original content should be preserved
	node, _ := dst.GetNode("existing-id")
	if node.Content != "original content" {
		t.Errorf("expected original content preserved, got %q", node.Content)
	}
}

func TestImportBundleConflictReplace(t *testing.T) {
	dst := testStore(t)

	// Pre-populate
	if err := dst.AddNodeWithID("existing-id", &AddNodeOpts{
		Type: TypeConcept, Content: "original", Importance: 0.5,
	}); err != nil {
		t.Fatalf("AddNodeWithID: %v", err)
	}

	bundle := &ExportBundle{
		Version: ExportVersion,
		Nodes: []Node{
			{ID: "existing-id", Type: TypeConcept, Content: "replaced", Importance: 0.9, DecayRate: 0.02, Status: "active"},
		},
	}

	result, err := dst.Import(bundle, ImportOptions{OnConflict: ConflictReplace})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	if result.NodesImported != 1 {
		t.Errorf("expected 1 node imported, got %d", result.NodesImported)
	}

	node, _ := dst.GetNode("existing-id")
	if node.Content != "replaced" {
		t.Errorf("expected replaced content, got %q", node.Content)
	}
}

func TestImportUpdatesMetaFromBundle(t *testing.T) {
	dst := testStore(t)

	bundle := &ExportBundle{
		Version: ExportVersion,
		Meta: map[string]string{
			"embedding_provider": "ollama",
			"embedding_model":    "nomic-embed-text",
		},
	}

	if _, err := dst.Import(bundle, ImportOptions{OnConflict: ConflictSkip}); err != nil {
		t.Fatalf("Import: %v", err)
	}

	provider, _ := dst.GetMeta("embedding_provider")
	if provider != "ollama" {
		t.Errorf("expected embedding_provider=ollama, got %q", provider)
	}
}

func TestExportEmptyStore(t *testing.T) {
	s := testStore(t)

	bundle, err := s.Export()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	if len(bundle.Nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(bundle.Nodes))
	}
	if len(bundle.Edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(bundle.Edges))
	}
}

func TestImportEmptyBundle(t *testing.T) {
	dst := testStore(t)

	bundle := &ExportBundle{Version: ExportVersion}
	result, err := dst.Import(bundle, ImportOptions{OnConflict: ConflictSkip})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if result.NodesImported != 0 {
		t.Errorf("expected 0 nodes imported, got %d", result.NodesImported)
	}
}

func TestExportToSQLite(t *testing.T) {
	s := testStore(t)

	if _, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "test", Importance: 0.5}); err != nil {
		t.Fatalf("AddNode: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "export.sqlite")
	if err := s.ExportSQLite(outPath); err != nil {
		t.Fatalf("ExportSQLite: %v", err)
	}

	// Open the exported file and verify
	exported, err := Open(outPath)
	if err != nil {
		t.Fatalf("Open exported: %v", err)
	}
	defer func() { _ = exported.Close() }()

	nodes, err := exported.ListNodes(ListNodesOpts{})
	if err != nil {
		t.Fatalf("ListNodes from export: %v", err)
	}
	if len(nodes) != 1 {
		t.Errorf("expected 1 node in export, got %d", len(nodes))
	}
}
