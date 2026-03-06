package brain

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/coetzeevs/cerebro/internal/store"
)

func testBrain(t *testing.T) *Brain {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.sqlite")
	b, err := Init(path, EmbedConfig{Provider: "none"})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { b.Close() })
	return b
}

func TestInit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.sqlite")
	b, err := Init(path, EmbedConfig{Provider: "none"})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer b.Close()

	// Verify meta was set
	provider, err := b.store.GetMeta("embedding_provider")
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if provider != "none" {
		t.Errorf("expected provider=none, got %q", provider)
	}

	model, _ := b.store.GetMeta("embedding_model")
	if model != "none" {
		t.Errorf("expected model=none, got %q", model)
	}

	dims, _ := b.store.GetMeta("embedding_dimensions")
	if dims != "0" {
		t.Errorf("expected dimensions=0, got %q", dims)
	}
}

func TestInitCreatesDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "deep", "test.sqlite")
	b, err := Init(path, EmbedConfig{Provider: "none"})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	b.Close()
}

func TestOpenExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.sqlite")
	b, err := Init(path, EmbedConfig{Provider: "none"})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	b.Close()

	b2, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer b2.Close()

	// Should have loaded embedding config from meta
	if b2.embedder.Model() != "none" {
		t.Errorf("expected embedder model=none, got %q", b2.embedder.Model())
	}
}

func TestOpenNonexistent(t *testing.T) {
	_, err := Open(filepath.Join(t.TempDir(), "nope.sqlite"))
	if err == nil {
		t.Fatal("expected error opening nonexistent brain")
	}
}

func TestAddAndGet(t *testing.T) {
	b := testBrain(t)

	id, err := b.Add("test memory", store.TypeConcept, WithImportance(0.8))
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty ID")
	}

	nwe, err := b.Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if nwe.Content != "test memory" {
		t.Errorf("expected content='test memory', got %q", nwe.Content)
	}
	if nwe.Importance != 0.8 {
		t.Errorf("expected importance=0.8, got %f", nwe.Importance)
	}
	if nwe.Type != store.TypeConcept {
		t.Errorf("expected type=concept, got %s", nwe.Type)
	}
}

func TestAddWithSubtype(t *testing.T) {
	b := testBrain(t)

	id, err := b.Add("test", store.TypeEpisode, WithSubtype("debug_session"))
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	nwe, _ := b.Get(id)
	if nwe.Subtype != "debug_session" {
		t.Errorf("expected subtype=debug_session, got %q", nwe.Subtype)
	}
}

func TestAddWithMetadata(t *testing.T) {
	b := testBrain(t)

	meta := json.RawMessage(`{"project":"cerebro"}`)
	id, err := b.Add("test", store.TypeConcept, WithMetadata(meta))
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	nwe, _ := b.Get(id)
	if string(nwe.Metadata) != `{"project":"cerebro"}` {
		t.Errorf("expected metadata={\"project\":\"cerebro\"}, got %s", nwe.Metadata)
	}
}

func TestAddDefaultImportance(t *testing.T) {
	b := testBrain(t)

	id, err := b.Add("test", store.TypeConcept)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	nwe, _ := b.Get(id)
	if nwe.Importance != 0.5 {
		t.Errorf("expected default importance=0.5, got %f", nwe.Importance)
	}
}

func TestUpdate(t *testing.T) {
	b := testBrain(t)

	id, _ := b.Add("original", store.TypeConcept)

	if err := b.Update(id, WithContent("updated")); err != nil {
		t.Fatalf("Update content: %v", err)
	}

	nwe, _ := b.Get(id)
	if nwe.Content != "updated" {
		t.Errorf("expected content='updated', got %q", nwe.Content)
	}

	if err := b.Update(id, WithUpdatedImportance(0.9)); err != nil {
		t.Fatalf("Update importance: %v", err)
	}

	nwe, _ = b.Get(id)
	if nwe.Importance != 0.9 {
		t.Errorf("expected importance=0.9, got %f", nwe.Importance)
	}
}

func TestSupersede(t *testing.T) {
	b := testBrain(t)

	oldID, _ := b.Add("old fact", store.TypeConcept)
	newID, err := b.Supersede(oldID, "new fact", store.TypeConcept, WithImportance(0.7))
	if err != nil {
		t.Fatalf("Supersede: %v", err)
	}

	old, _ := b.Get(oldID)
	if old.Status != "superseded" {
		t.Errorf("old node status: expected superseded, got %s", old.Status)
	}

	new_, _ := b.Get(newID)
	if new_.Status != "active" {
		t.Errorf("new node status: expected active, got %s", new_.Status)
	}
	if new_.Content != "new fact" {
		t.Errorf("new node content: expected 'new fact', got %q", new_.Content)
	}

	// Supersedes edge should exist
	if len(new_.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(new_.Edges))
	}
	if new_.Edges[0].Relation != "supersedes" {
		t.Errorf("expected relation=supersedes, got %s", new_.Edges[0].Relation)
	}
}

func TestReinforce(t *testing.T) {
	b := testBrain(t)

	id, _ := b.Add("test", store.TypeConcept)
	if err := b.Reinforce(id); err != nil {
		t.Fatalf("Reinforce: %v", err)
	}

	nwe, _ := b.Get(id)
	if nwe.AccessCount != 1 {
		t.Errorf("expected access_count=1, got %d", nwe.AccessCount)
	}
}

func TestAddEdge(t *testing.T) {
	b := testBrain(t)

	id1, _ := b.Add("node1", store.TypeConcept)
	id2, _ := b.Add("node2", store.TypeConcept)

	edgeID, err := b.AddEdge(id1, id2, "relates_to")
	if err != nil {
		t.Fatalf("AddEdge: %v", err)
	}
	if edgeID == 0 {
		t.Error("expected non-zero edge ID")
	}

	nwe, _ := b.Get(id1)
	if len(nwe.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(nwe.Edges))
	}
	if nwe.Edges[0].Relation != "relates_to" {
		t.Errorf("expected relation=relates_to, got %s", nwe.Edges[0].Relation)
	}
}

func TestMarkConsolidated(t *testing.T) {
	b := testBrain(t)

	id1, _ := b.Add("ep1", store.TypeEpisode)
	id2, _ := b.Add("ep2", store.TypeEpisode)

	if err := b.MarkConsolidated([]string{id1, id2}); err != nil {
		t.Fatalf("MarkConsolidated: %v", err)
	}

	n1, _ := b.Get(id1)
	n2, _ := b.Get(id2)
	if n1.Status != "consolidated" {
		t.Errorf("node1 status: expected consolidated, got %s", n1.Status)
	}
	if n2.Status != "consolidated" {
		t.Errorf("node2 status: expected consolidated, got %s", n2.Status)
	}
}

func TestList(t *testing.T) {
	b := testBrain(t)

	if _, err := b.Add("ep1", store.TypeEpisode); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := b.Add("c1", store.TypeConcept); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := b.Add("c2", store.TypeConcept); err != nil {
		t.Fatalf("Add: %v", err)
	}

	all, err := b.List(store.ListNodesOpts{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(all))
	}

	concepts, _ := b.List(store.ListNodesOpts{Type: store.TypeConcept})
	if len(concepts) != 2 {
		t.Errorf("expected 2 concepts, got %d", len(concepts))
	}
}

func TestStats(t *testing.T) {
	b := testBrain(t)

	if _, err := b.Add("ep1", store.TypeEpisode); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := b.Add("c1", store.TypeConcept); err != nil {
		t.Fatalf("Add: %v", err)
	}

	stats, err := b.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.TotalNodes != 2 {
		t.Errorf("expected 2 total nodes, got %d", stats.TotalNodes)
	}
	if stats.ActiveNodes != 2 {
		t.Errorf("expected 2 active nodes, got %d", stats.ActiveNodes)
	}
	if stats.SchemaVersion != "1" {
		t.Errorf("expected schema_version=1, got %q", stats.SchemaVersion)
	}
}

func TestSearchWithoutEmbedder(t *testing.T) {
	b := testBrain(t)

	_, err := b.Search(context.Background(), "test query", 10, 0.7)
	if err == nil {
		t.Fatal("expected error searching without embedder")
	}
}

func TestStore(t *testing.T) {
	b := testBrain(t)
	if b.Store() == nil {
		t.Fatal("expected non-nil store")
	}
}

func TestProjectPath(t *testing.T) {
	p1 := ProjectPath("/some/project")
	p2 := ProjectPath("/some/project")
	p3 := ProjectPath("/other/project")

	if p1 != p2 {
		t.Errorf("same input should produce same path: %q != %q", p1, p2)
	}
	if p1 == p3 {
		t.Error("different inputs should produce different paths")
	}
	if filepath.Ext(p1) != ".sqlite" {
		t.Errorf("expected .sqlite extension, got %q", filepath.Ext(p1))
	}
}

func TestGlobalPath(t *testing.T) {
	p := GlobalPath()
	if filepath.Base(p) != "global.sqlite" {
		t.Errorf("expected global.sqlite, got %q", filepath.Base(p))
	}
}

func TestNewEmbedder(t *testing.T) {
	tests := []struct {
		provider string
		model    string
	}{
		{"none", "none"},
		{"", "none"},
		{"unknown", "none"},
	}

	for _, tt := range tests {
		e := newEmbedder(EmbedConfig{Provider: tt.provider})
		if e.Model() != tt.model {
			t.Errorf("provider=%q: expected model=%q, got %q", tt.provider, tt.model, e.Model())
		}
	}
}
