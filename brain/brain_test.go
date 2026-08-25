package brain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
	t.Cleanup(func() { _ = b.Close() })
	return b
}

func TestInit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.sqlite")
	b, err := Init(path, EmbedConfig{Provider: "none"})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer func() { _ = b.Close() }()

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
	_ = b.Close()
}

func TestOpenExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.sqlite")
	b, err := Init(path, EmbedConfig{Provider: "none"})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	_ = b.Close()

	b2, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = b2.Close() }()

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

	nwe, err := b.Get(id, nil)
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

	nwe, _ := b.Get(id, nil)
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

	nwe, _ := b.Get(id, nil)
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

	nwe, _ := b.Get(id, nil)
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

	nwe, _ := b.Get(id, nil)
	if nwe.Content != "updated" {
		t.Errorf("expected content='updated', got %q", nwe.Content)
	}

	if err := b.Update(id, WithUpdatedImportance(0.9)); err != nil {
		t.Fatalf("Update importance: %v", err)
	}

	nwe, _ = b.Get(id, nil)
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

	old, _ := b.Get(oldID, nil)
	if old.Status != "superseded" {
		t.Errorf("old node status: expected superseded, got %s", old.Status)
	}

	new_, _ := b.Get(newID, nil)
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

	nwe, _ := b.Get(id, nil)
	if nwe.AccessCount != 1 {
		t.Errorf("expected access_count=1, got %d", nwe.AccessCount)
	}
}

func TestAddEdge(t *testing.T) {
	b := testBrain(t)

	id1, _ := b.Add("node1", store.TypeConcept)
	id2, _ := b.Add("node2", store.TypeConcept)

	edgeID, err := b.AddEdge(id1, id2, "relates_to", store.AddEdgeOpts{})
	if err != nil {
		t.Fatalf("AddEdge: %v", err)
	}
	if edgeID == 0 {
		t.Error("expected non-zero edge ID")
	}

	nwe, _ := b.Get(id1, nil)
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

	n1, _ := b.Get(id1, nil)
	n2, _ := b.Get(id2, nil)
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
	if stats.SchemaVersion != "6" {
		t.Errorf("expected schema_version=6, got %q", stats.SchemaVersion)
	}
}

func TestSearchWithoutEmbedder(t *testing.T) {
	b := testBrain(t)

	_, err := b.Search(context.Background(), "test query", 10, 0.7, nil, nil)
	if err == nil {
		t.Fatal("expected error searching without embedder")
	}
}

// TestSearchWithoutEmbedder_WithSubtypeFilter verifies that Search with a
// non-nil subtypeFilter still returns the embedder-absent error (AC scenario 6
// backward compat — the subtype filter does not mask embedder errors).
func TestSearchWithoutEmbedder_WithSubtypeFilter(t *testing.T) {
	b := testBrain(t)
	subtype := "routing-discovery"

	_, err := b.Search(context.Background(), "test query", 10, 0.7, &subtype, nil)
	if err == nil {
		t.Fatal("expected error searching without embedder (subtypeFilter should not mask error)")
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

// TestProjectPath_ResolvesSymlinks verifies that ProjectPath resolves symlinks
// before hashing so that a symlinked project path and its realpath produce
// identical brain paths (HS-008, Ontology §5.14 rule 26).
func TestProjectPath_ResolvesSymlinks(t *testing.T) {
	realDir := t.TempDir()
	symlinkDir := filepath.Join(t.TempDir(), "symlink-project")

	if err := os.Symlink(realDir, symlinkDir); err != nil {
		t.Skipf("cannot create symlink (may require elevated privileges on this OS): %v", err)
	}

	pathViaReal := ProjectPath(realDir)
	pathViaSym := ProjectPath(symlinkDir)

	if pathViaReal != pathViaSym {
		t.Errorf("symlinked and real paths should produce the same brain path:\n  real: %s\n  sym:  %s", pathViaReal, pathViaSym)
	}
}

// TestProjectPath_FallbackOnNonexistent verifies that ProjectPath falls back to
// filepath.Abs (pre-HS-008 behaviour) when the path does not exist, so that
// CI bootstrap callers (path created post brain-init) still get a deterministic hash.
func TestProjectPath_FallbackOnNonexistent(t *testing.T) {
	nonexistent := filepath.Join(t.TempDir(), "does-not-exist", "project")

	// Should not panic; should return a valid path with .sqlite extension.
	p := ProjectPath(nonexistent)
	if filepath.Ext(p) != ".sqlite" {
		t.Errorf("expected .sqlite extension even for nonexistent path, got %q", filepath.Ext(p))
	}
	if p == "" {
		t.Error("expected non-empty path for nonexistent input")
	}
}

// TestProjectPath_DarwinTmpSymlink exercises the macOS motivating case where
// /tmp is a symlink to /private/tmp. Skipped on non-Darwin platforms.
// Uses a directory created under /private/tmp so that both /tmp/<dir> and
// /private/tmp/<dir> actually exist and EvalSymlinks can resolve them.
func TestProjectPath_DarwinTmpSymlink(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS-specific /tmp → /private/tmp test")
	}
	// /tmp is a well-known symlink on macOS; /private/tmp is the realpath.
	// Both should resolve to the same brain path.
	const privateTmp = "/private/tmp"

	// Verify the symlink actually exists (guard for future macOS changes).
	resolved, err := filepath.EvalSymlinks("/tmp")
	if err != nil || resolved != privateTmp {
		t.Skipf("/tmp does not resolve to /private/tmp on this system (resolved=%q, err=%v)", resolved, err)
	}

	// Create a real directory under /private/tmp so EvalSymlinks can resolve
	// the /tmp/<subdir> path through the symlink.
	realDir, err := os.MkdirTemp(privateTmp, "hs008-test-*")
	if err != nil {
		t.Skipf("cannot create temp dir under /private/tmp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(realDir) })

	// Derive the /tmp/... alias path via the known /tmp → /private/tmp symlink.
	// Build the path as a string to avoid filepath.Join linter complaints about separator usage.
	subdir := filepath.Base(realDir)
	symlinkDir := string(filepath.Separator) + "tmp" + string(filepath.Separator) + subdir

	pathViaTmp := ProjectPath(symlinkDir)
	pathViaPrivate := ProjectPath(realDir)

	if pathViaTmp != pathViaPrivate {
		t.Errorf("/tmp and /private/tmp project paths should produce the same brain path:\n  /tmp alias:   %s\n  /private/tmp: %s", pathViaTmp, pathViaPrivate)
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

// --- Promote tests ---

func TestPromote_BasicCopy(t *testing.T) {
	src := testBrain(t)
	dst := testBrain(t)

	srcID, err := src.Add("Go prefers explicit error handling over exceptions", store.TypeConcept, WithImportance(0.8))
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	globalID, err := src.Promote(context.Background(), srcID, dst)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if globalID == "" {
		t.Fatal("expected non-empty global ID")
	}

	gNode, err := dst.Get(globalID, nil)
	if err != nil {
		t.Fatalf("Get from global: %v", err)
	}
	if gNode.Content != "Go prefers explicit error handling over exceptions" {
		t.Errorf("content mismatch: %q", gNode.Content)
	}
	if gNode.Importance != 0.5 {
		t.Errorf("expected importance=0.5, got %f", gNode.Importance)
	}
	if gNode.Type != store.TypeConcept {
		t.Errorf("expected type=concept, got %s", gNode.Type)
	}
}

func TestPromote_WithContentOverride(t *testing.T) {
	src := testBrain(t)
	dst := testBrain(t)

	srcID, _ := src.Add("In the cerebro project, we use Go 1.24", store.TypeConcept)

	globalID, err := src.Promote(context.Background(), srcID, dst,
		WithPromoteContent("Memory systems benefit from project-scoped storage"))
	if err != nil {
		t.Fatalf("Promote with content: %v", err)
	}

	gNode, err := dst.Get(globalID, nil)
	if err != nil {
		t.Fatalf("Get from global: %v", err)
	}
	if gNode.Content != "Memory systems benefit from project-scoped storage" {
		t.Errorf("content not overridden: %q", gNode.Content)
	}
}

func TestPromote_ProvenanceMetadata(t *testing.T) {
	src := testBrain(t)
	dst := testBrain(t)

	srcID, _ := src.Add("test concept", store.TypeConcept)

	globalID, err := src.Promote(context.Background(), srcID, dst)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}

	gNode, err := dst.Get(globalID, nil)
	if err != nil {
		t.Fatalf("Get from global: %v", err)
	}

	var meta map[string]any
	if err := json.Unmarshal(gNode.Metadata, &meta); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if meta["promoted_from_node"] != srcID {
		t.Errorf("expected promoted_from_node=%q, got %v", srcID, meta["promoted_from_node"])
	}
	if _, ok := meta["promoted_at"]; !ok {
		t.Error("expected promoted_at in metadata")
	}
}

func TestPromote_SourceMetadataUpdated(t *testing.T) {
	src := testBrain(t)
	dst := testBrain(t)

	srcID, _ := src.Add("test concept", store.TypeConcept)
	globalID, err := src.Promote(context.Background(), srcID, dst)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}

	srcNode, err := src.Get(srcID, nil)
	if err != nil {
		t.Fatalf("Get source node: %v", err)
	}
	var meta map[string]any
	if err := json.Unmarshal(srcNode.Metadata, &meta); err != nil {
		t.Fatalf("unmarshal source metadata: %v", err)
	}
	if meta["promoted_to_global"] != globalID {
		t.Errorf("expected promoted_to_global=%q, got %v", globalID, meta["promoted_to_global"])
	}
}

func TestPromote_NonexistentNode(t *testing.T) {
	src := testBrain(t)
	dst := testBrain(t)

	_, err := src.Promote(context.Background(), "nonexistent-id", dst)
	if err == nil {
		t.Fatal("expected error promoting nonexistent node")
	}
}

func TestPromote_PreservesType(t *testing.T) {
	src := testBrain(t)
	dst := testBrain(t)

	for _, nodeType := range []store.NodeType{
		store.TypeEpisode, store.TypeConcept, store.TypeProcedure, store.TypeReflection,
	} {
		srcID, _ := src.Add("content for "+string(nodeType), nodeType)
		globalID, err := src.Promote(context.Background(), srcID, dst)
		if err != nil {
			t.Fatalf("Promote %s: %v", nodeType, err)
		}
		gNode, _ := dst.Get(globalID, nil)
		if gNode.Type != nodeType {
			t.Errorf("type not preserved: expected %s, got %s", nodeType, gNode.Type)
		}
	}
}

// --- Merge / Global search tests ---

func TestMergeSearchResults(t *testing.T) {
	projectResults := []store.ScoredNode{
		{Node: store.Node{ID: "proj-1"}, Score: 0.9},
		{Node: store.Node{ID: "proj-2"}, Score: 0.7},
	}
	globalResults := []store.ScoredNode{
		{Node: store.Node{ID: "glob-1"}, Score: 0.8},
		{Node: store.Node{ID: "proj-1"}, Score: 0.85}, // duplicate — project wins
	}

	merged := mergeSearchResults(projectResults, globalResults, 10)

	// proj-1 should appear exactly once (project version wins)
	ids := make(map[string]int)
	for _, r := range merged {
		ids[r.ID]++
	}
	if ids["proj-1"] != 1 {
		t.Errorf("proj-1 should appear exactly once, got %d", ids["proj-1"])
	}

	// Total unique: proj-1, proj-2, glob-1
	if len(merged) != 3 {
		t.Errorf("expected 3 merged results, got %d", len(merged))
	}

	// glob-1's score should be weighted by 0.7
	for _, r := range merged {
		if r.ID == "glob-1" {
			expected := 0.8 * 0.7
			diff := r.Score - expected
			if diff < -1e-9 || diff > 1e-9 {
				t.Errorf("glob-1 score: expected %.4f (0.8*0.7), got %.4f", expected, r.Score)
			}
		}
	}

	// Results should be sorted by score descending
	for i := 1; i < len(merged); i++ {
		if merged[i].Score > merged[i-1].Score {
			t.Errorf("results not sorted: [%d]=%.3f > [%d]=%.3f",
				i, merged[i].Score, i-1, merged[i-1].Score)
		}
	}
}

func TestMergeSearchResults_CapAtLimit(t *testing.T) {
	var project []store.ScoredNode
	for i := 0; i < 5; i++ {
		project = append(project, store.ScoredNode{
			Node:  store.Node{ID: fmt.Sprintf("p%d", i)},
			Score: float64(5-i) / 5,
		})
	}
	var global []store.ScoredNode
	for i := 0; i < 5; i++ {
		global = append(global, store.ScoredNode{
			Node:  store.Node{ID: fmt.Sprintf("g%d", i)},
			Score: float64(5-i) / 5,
		})
	}

	merged := mergeSearchResults(project, global, 3)
	if len(merged) != 3 {
		t.Errorf("expected 3 results with limit=3, got %d", len(merged))
	}
}

func TestSearchWithGlobal_NoEmbedder(t *testing.T) {
	src := testBrain(t)
	dst := testBrain(t)

	_, err := src.SearchWithGlobal(context.Background(), "query", 10, 0.3, dst, nil, nil)
	if err == nil {
		t.Fatal("expected error without embedder")
	}
}

// --- Subtype update tests [OO-011] ---

// TestBrainUpdate_WithUpdatedSubtype verifies that WithUpdatedSubtype correctly
// sets the subtype on an existing node via the Brain.Update path.
func TestBrainUpdate_WithUpdatedSubtype(t *testing.T) {
	b := testBrain(t)

	id, err := b.Add("a concept with no subtype", store.TypeConcept, WithImportance(0.7))
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Set subtype via Brain.Update
	if err := b.Update(id, WithUpdatedSubtype("routing-discovery")); err != nil {
		t.Fatalf("Update(WithUpdatedSubtype): %v", err)
	}

	nwe, err := b.Get(id, nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if nwe.Subtype != "routing-discovery" {
		t.Errorf("expected subtype=routing-discovery after WithUpdatedSubtype, got %q", nwe.Subtype)
	}
}

// TestBrainUpdate_WithUpdatedSubtype_Clear verifies that WithUpdatedSubtype("")
// clears the subtype to NULL via the Brain.Update path.
func TestBrainUpdate_WithUpdatedSubtype_Clear(t *testing.T) {
	b := testBrain(t)

	id, err := b.Add("a concept with subtype", store.TypeConcept,
		WithSubtype("routing-discovery"),
		WithImportance(0.7),
	)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Verify initial subtype
	nwe, err := b.Get(id, nil)
	if err != nil {
		t.Fatalf("Get before clear: %v", err)
	}
	if nwe.Subtype != "routing-discovery" {
		t.Fatalf("expected initial subtype=routing-discovery, got %q", nwe.Subtype)
	}

	// Clear subtype with empty string
	if err := b.Update(id, WithUpdatedSubtype("")); err != nil {
		t.Fatalf("Update(WithUpdatedSubtype clear): %v", err)
	}

	nwe, err = b.Get(id, nil)
	if err != nil {
		t.Fatalf("Get after clear: %v", err)
	}
	if nwe.Subtype != "" {
		t.Errorf("expected empty subtype after clear, got %q", nwe.Subtype)
	}
}

// TestBrainUpdate_WithUpdatedSubtype_NoContentReembed verifies that
// WithUpdatedSubtype does not trigger re-embedding (subtype is metadata, not content).
func TestBrainUpdate_WithUpdatedSubtype_NoContentReembed(t *testing.T) {
	b := testBrain(t)

	id, err := b.Add("stable content", store.TypeConcept, WithImportance(0.8))
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Update subtype only — should succeed even if embedder is noop
	if err := b.Update(id, WithUpdatedSubtype("operator-safety")); err != nil {
		t.Fatalf("Update(WithUpdatedSubtype only) should not fail: %v", err)
	}

	// Content must be unchanged
	nwe, err := b.Get(id, nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if nwe.Content != "stable content" {
		t.Errorf("expected content unchanged after subtype update, got %q", nwe.Content)
	}
}

// TestFilterScoredNodesBySubtype verifies the post-search subtype filter helper.
// This tests the pure-Go filter that sits after ExpandGraph in Search.
func TestFilterScoredNodesBySubtype(t *testing.T) {
	nodes := []store.ScoredNode{
		{Node: store.Node{ID: "a", Subtype: "routing-discovery"}, Score: 0.9},
		{Node: store.Node{ID: "b", Subtype: ""}, Score: 0.8},
		{Node: store.Node{ID: "c", Subtype: "operator-safety"}, Score: 0.7},
		{Node: store.Node{ID: "d", Subtype: "routing-discovery"}, Score: 0.6},
	}

	// nil filter: no change — backward compat
	result := filterScoredNodesBySubtype(nodes, nil)
	if len(result) != len(nodes) {
		t.Errorf("nil filter: expected %d results, got %d", len(nodes), len(result))
	}

	// &"routing-discovery" filter: only nodes a and d
	rd := "routing-discovery"
	result = filterScoredNodesBySubtype(nodes, &rd)
	if len(result) != 2 {
		t.Errorf("subtype=routing-discovery: expected 2, got %d", len(result))
	}
	for _, r := range result {
		if r.Subtype != "routing-discovery" {
			t.Errorf("expected all results to have subtype=routing-discovery, got %q", r.Subtype)
		}
	}

	// &"" filter: only nodes with empty subtype (node b)
	empty := ""
	result = filterScoredNodesBySubtype(nodes, &empty)
	if len(result) != 1 {
		t.Errorf("subtype=\"\": expected 1 NULL-subtype node, got %d", len(result))
	}
	if len(result) > 0 && result[0].ID != "b" {
		t.Errorf("expected node b (empty subtype), got %s", result[0].ID)
	}

	// &"unicorn": no matches
	unicorn := "unicorn"
	result = filterScoredNodesBySubtype(nodes, &unicorn)
	if len(result) != 0 {
		t.Errorf("subtype=unicorn: expected 0, got %d", len(result))
	}
}

// --- Export / Import tests ---

func TestExport(t *testing.T) {
	b := testBrain(t)

	id1, _ := b.Add("concept one", store.TypeConcept, WithImportance(0.8))
	id2, _ := b.Add("episode one", store.TypeEpisode)
	if _, err := b.AddEdge(id1, id2, "relates_to", store.AddEdgeOpts{}); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}

	bundle, err := b.Export()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	if len(bundle.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(bundle.Nodes))
	}
	if len(bundle.Edges) != 1 {
		t.Errorf("expected 1 edge, got %d", len(bundle.Edges))
	}
	if bundle.Version != store.ExportVersion {
		t.Errorf("expected version=%s, got %s", store.ExportVersion, bundle.Version)
	}
}

func TestImport(t *testing.T) {
	src := testBrain(t)
	id1, _ := src.Add("concept", store.TypeConcept, WithImportance(0.8))
	id2, _ := src.Add("episode", store.TypeEpisode)
	if _, err := src.AddEdge(id1, id2, "relates_to", store.AddEdgeOpts{}); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}

	bundle, err := src.Export()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	dst := testBrain(t)
	result, err := dst.Import(bundle, store.ImportOptions{OnConflict: store.ConflictSkip})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	if result.NodesImported != 2 {
		t.Errorf("expected 2 imported, got %d", result.NodesImported)
	}
	if result.EdgesImported != 1 {
		t.Errorf("expected 1 edge imported, got %d", result.EdgesImported)
	}

	// Verify data in destination
	nwe, err := dst.Get(id1, nil)
	if err != nil {
		t.Fatalf("Get from dst: %v", err)
	}
	if nwe.Content != "concept" {
		t.Errorf("expected content='concept', got %q", nwe.Content)
	}

	// Should mark pending embeddings
	pending, _ := dst.Store().GetMeta("has_pending_embeddings")
	if pending != "true" {
		t.Errorf("expected has_pending_embeddings=true, got %q", pending)
	}
}

func TestImportFromJSON(t *testing.T) {
	src := testBrain(t)
	if _, err := src.Add("test node", store.TypeConcept, WithImportance(0.7)); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Export to JSON buffer
	var buf bytes.Buffer
	if err := src.ExportJSON(&buf); err != nil {
		t.Fatalf("ExportJSON: %v", err)
	}

	// Import from JSON
	dst := testBrain(t)
	result, err := dst.ImportFromJSON(&buf, store.ImportOptions{OnConflict: store.ConflictSkip})
	if err != nil {
		t.Fatalf("ImportFromJSON: %v", err)
	}
	if result.NodesImported != 1 {
		t.Errorf("expected 1 node imported, got %d", result.NodesImported)
	}
}

func TestExportSQLite(t *testing.T) {
	b := testBrain(t)
	if _, err := b.Add("test", store.TypeConcept); err != nil {
		t.Fatalf("Add: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "export.sqlite")
	if err := b.ExportSQLite(outPath); err != nil {
		t.Fatalf("ExportSQLite: %v", err)
	}

	// Open and verify
	dst, err := Open(outPath)
	if err != nil {
		t.Fatalf("Open exported: %v", err)
	}
	defer func() { _ = dst.Close() }()

	nodes, err := dst.List(store.ListNodesOpts{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(nodes) != 1 {
		t.Errorf("expected 1 node in export, got %d", len(nodes))
	}
}

func TestExportSQL(t *testing.T) {
	b := testBrain(t)
	if _, err := b.Add("node with 'quotes'", store.TypeConcept, WithImportance(0.6)); err != nil {
		t.Fatalf("Add: %v", err)
	}

	var buf bytes.Buffer
	if err := b.ExportSQL(&buf); err != nil {
		t.Fatalf("ExportSQL: %v", err)
	}

	sql := buf.String()
	if sql == "" {
		t.Fatal("expected non-empty SQL output")
	}
	// Single quotes should be escaped
	if !bytes.Contains(buf.Bytes(), []byte("''quotes''")) {
		t.Errorf("expected escaped quotes in SQL, got: %s", sql)
	}
}
