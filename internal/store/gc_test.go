package store

import (
	"testing"
	"time"
)

func TestGCEvictsDecayedNodes(t *testing.T) {
	s := testStore(t)

	// Add an episode (high decay_rate=0.15) and set last_accessed to 30 days ago.
	// retention = 0.5 * (0.1 * 1) + 0.5 * exp(-0.15 * 720) ≈ 0.05
	id, err := s.AddNode(&AddNodeOpts{Type: TypeEpisode, Content: "old episode", Importance: 0.1})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	setLastAccessed(t, s, id, time.Now().Add(-30*24*time.Hour))

	result, err := s.GC(0.1, false)
	if err != nil {
		t.Fatalf("GC: %v", err)
	}

	if result.Archived != 1 {
		t.Errorf("expected 1 archived, got %d", result.Archived)
	}

	// Node should no longer be active
	node, err := s.GetNode(id)
	if err == nil && node != nil {
		t.Errorf("expected node to be deleted from nodes table, but it still exists")
	}
}

func TestGCPreservesRecentNodes(t *testing.T) {
	s := testStore(t)

	// Add a recently accessed node
	id, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "fresh concept", Importance: 0.8})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}

	result, err := s.GC(0.1, false)
	if err != nil {
		t.Fatalf("GC: %v", err)
	}

	if result.Archived != 0 {
		t.Errorf("expected 0 archived, got %d", result.Archived)
	}

	// Node should still exist
	node, err := s.GetNode(id)
	if err != nil {
		t.Fatalf("GetNode after GC: %v", err)
	}
	if node.Status != "active" {
		t.Errorf("expected status=active, got %q", node.Status)
	}
	_ = id
}

func TestGCDryRun(t *testing.T) {
	s := testStore(t)

	id, err := s.AddNode(&AddNodeOpts{Type: TypeEpisode, Content: "old episode", Importance: 0.1})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	setLastAccessed(t, s, id, time.Now().Add(-60*24*time.Hour))

	result, err := s.GC(0.5, true) // dry-run=true
	if err != nil {
		t.Fatalf("GC dry-run: %v", err)
	}

	// Should report candidates but not actually archive
	if result.Archived != 1 {
		t.Errorf("dry-run should report 1 candidate, got %d", result.Archived)
	}

	// Node should still exist in nodes table
	node, err := s.GetNode(id)
	if err != nil {
		t.Fatalf("GetNode after dry-run: %v", err)
	}
	if node.Status != "active" {
		t.Errorf("expected node still active after dry-run, got %q", node.Status)
	}
}

func TestGCArchiveContents(t *testing.T) {
	s := testStore(t)

	id, err := s.AddNode(&AddNodeOpts{
		Type:       TypeEpisode,
		Subtype:    "debug",
		Content:    "archived episode content",
		Importance: 0.2,
	})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	setLastAccessed(t, s, id, time.Now().Add(-90*24*time.Hour))

	_, err = s.GC(0.5, false)
	if err != nil {
		t.Fatalf("GC: %v", err)
	}

	// Verify archive entry
	var content, archiveReason, nodeType string
	err = s.db.QueryRow(`SELECT content, archive_reason, type FROM nodes_archive WHERE id = ?`, id).
		Scan(&content, &archiveReason, &nodeType)
	if err != nil {
		t.Fatalf("querying archive: %v", err)
	}
	if content != "archived episode content" {
		t.Errorf("expected archived content, got %q", content)
	}
	if archiveReason != "decayed" {
		t.Errorf("expected archive_reason='decayed', got %q", archiveReason)
	}
	if nodeType != "episode" {
		t.Errorf("expected type='episode', got %q", nodeType)
	}
}

func TestGCCleansEmbeddings(t *testing.T) {
	s := testStoreWithVec(t, 4)

	id, err := s.AddNode(&AddNodeOpts{Type: TypeEpisode, Content: "embedded episode", Importance: 0.1})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	if err := s.StoreEmbedding(id, []float32{1, 0, 0, 0}); err != nil {
		t.Fatalf("StoreEmbedding: %v", err)
	}
	setLastAccessed(t, s, id, time.Now().Add(-60*24*time.Hour))

	// Verify embedding exists before GC
	var countBefore int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM vec_nodes WHERE node_id = ?`, id).Scan(&countBefore); err != nil {
		t.Fatalf("counting embeddings before: %v", err)
	}
	if countBefore != 1 {
		t.Fatalf("expected 1 embedding before GC, got %d", countBefore)
	}

	_, err = s.GC(0.5, false)
	if err != nil {
		t.Fatalf("GC: %v", err)
	}

	// Embedding should be cleaned up
	var countAfter int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM vec_nodes WHERE node_id = ?`, id).Scan(&countAfter); err != nil {
		t.Fatalf("counting embeddings after: %v", err)
	}
	if countAfter != 0 {
		t.Errorf("expected 0 embeddings after GC, got %d", countAfter)
	}
}

func TestGCRespectsImportance(t *testing.T) {
	s := testStore(t)

	// Both 30 days old, but different importance
	lowID, err := s.AddNode(&AddNodeOpts{Type: TypeEpisode, Content: "low importance", Importance: 0.1})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	highID, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "high importance", Importance: 0.9})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	setLastAccessed(t, s, lowID, time.Now().Add(-30*24*time.Hour))
	setLastAccessed(t, s, highID, time.Now().Add(-30*24*time.Hour))

	// Use a threshold that should evict the low-importance episode
	// but preserve the high-importance concept
	result, err := s.GC(0.1, false)
	if err != nil {
		t.Fatalf("GC: %v", err)
	}

	// Low-importance episode should be evicted (high decay + low importance)
	_, err = s.GetNode(lowID)
	if err == nil {
		t.Error("expected low-importance episode to be evicted")
	}

	// High-importance concept should survive (low decay + high importance)
	node, err := s.GetNode(highID)
	if err != nil {
		t.Fatalf("expected high-importance concept to survive: %v", err)
	}
	if node.Status != "active" {
		t.Errorf("expected active status, got %q", node.Status)
	}

	if result.Archived != 1 {
		t.Errorf("expected 1 archived, got %d", result.Archived)
	}
}

func TestGCResultCounts(t *testing.T) {
	s := testStore(t)

	// Create 3 nodes, 2 should be evicted
	for _, content := range []string{"ep1", "ep2"} {
		id, err := s.AddNode(&AddNodeOpts{Type: TypeEpisode, Content: content, Importance: 0.1})
		if err != nil {
			t.Fatalf("AddNode: %v", err)
		}
		setLastAccessed(t, s, id, time.Now().Add(-60*24*time.Hour))
	}
	// This one stays
	if _, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "keeper", Importance: 0.9}); err != nil {
		t.Fatalf("AddNode: %v", err)
	}

	result, err := s.GC(0.1, false)
	if err != nil {
		t.Fatalf("GC: %v", err)
	}

	if result.Evaluated != 3 {
		t.Errorf("expected 3 evaluated, got %d", result.Evaluated)
	}
	if result.Archived != 2 {
		t.Errorf("expected 2 archived, got %d", result.Archived)
	}
	if result.ByType["episode"] != 2 {
		t.Errorf("expected 2 episodes archived, got %d", result.ByType["episode"])
	}
}

// setLastAccessed directly updates the last_accessed timestamp for testing.
func setLastAccessed(t *testing.T, s *Store, nodeID string, when time.Time) {
	t.Helper()
	_, err := s.db.Exec(`UPDATE nodes SET last_accessed = ? WHERE id = ?`, when.Format(time.RFC3339), nodeID)
	if err != nil {
		t.Fatalf("setting last_accessed: %v", err)
	}
}

