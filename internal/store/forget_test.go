package store

// forget_test.go — agentic-dpgh (TDD): subject-scoped bulk forget, distinct
// from gc (targets by content/subtype, not score). Cascades to embeddings,
// FTS, and edges so nothing stays retrievable; archives by default or
// hard-deletes on request. The irreversible operation demands a dry-run.

import "testing"

func seedForgetFixture(t *testing.T) (s *Store, ids map[string]string) {
	t.Helper()
	s = testStore(t)
	if err := s.InitVectorTable(4); err != nil {
		t.Fatalf("InitVectorTable: %v", err)
	}
	ids = map[string]string{}
	add := func(key, content, subtype string) {
		id, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Subtype: subtype, Content: content, Importance: 0.5})
		if err != nil {
			t.Fatalf("AddNode %s: %v", key, err)
		}
		if err := s.StoreEmbedding(id, []float32{1, 0, 0, 0}); err != nil {
			t.Fatalf("StoreEmbedding: %v", err)
		}
		ids[key] = id
	}
	add("px1", "Project X uses the atlantis pipeline", "infra")
	add("px2", "the PROJECT x credentials rotate monthly", "ops")
	add("keep", "Project Y is unrelated", "infra")
	if _, err := s.AddEdge(ids["px1"], ids["keep"], "relates_to", AddEdgeOpts{}); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}
	if _, err := s.AddEdge(ids["px2"], ids["px1"], "depends_on", AddEdgeOpts{}); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}
	return s, ids
}

func countRows(t *testing.T, s *Store, q string, args ...any) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", q, err)
	}
	return n
}

// Dry-run reports matches and touches NOTHING.
func TestForgetSubject_DryRunIsPure(t *testing.T) {
	s, ids := seedForgetFixture(t)
	res, err := s.ForgetSubject("project x", "", false, true)
	if err != nil {
		t.Fatalf("ForgetSubject dry: %v", err)
	}
	if len(res.Matched) != 2 {
		t.Fatalf("dry-run matched %d, want 2 (case-insensitive substring)", len(res.Matched))
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM nodes WHERE status='active'`); n != 3 {
		t.Errorf("dry-run mutated nodes: %d active, want 3", n)
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM edges`); n != 2 {
		t.Errorf("dry-run mutated edges: %d, want 2", n)
	}
	_ = ids
}

// Archive mode: matched nodes flip to archived; embeddings, FTS presence,
// and every touching edge are removed; unmatched nodes untouched.
func TestForgetSubject_ArchiveCascades(t *testing.T) {
	s, ids := seedForgetFixture(t)
	res, err := s.ForgetSubject("project x", "", false, false)
	if err != nil {
		t.Fatalf("ForgetSubject: %v", err)
	}
	if len(res.Matched) != 2 || res.EdgesRemoved != 2 || res.EmbeddingsRemoved != 2 {
		t.Fatalf("unexpected result: %+v", res)
	}
	for _, k := range []string{"px1", "px2"} {
		n, err := s.GetNode(ids[k])
		if err != nil {
			t.Fatalf("GetNode %s: %v", k, err)
		}
		if n.Status != "archived" {
			t.Errorf("%s status: got %s want archived", k, n.Status)
		}
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM edges`); n != 0 {
		t.Errorf("edges not cascaded: %d remain", n)
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM vec_nodes WHERE node_id IN (?,?)`, ids["px1"], ids["px2"]); n != 0 {
		t.Errorf("embeddings not cascaded: %d remain", n)
	}
	keep, _ := s.GetNode(ids["keep"])
	if keep.Status != "active" {
		t.Errorf("unmatched node touched: %s", keep.Status)
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM vec_nodes WHERE node_id = ?`, ids["keep"]); n != 1 {
		t.Errorf("unmatched embedding removed")
	}
}

// Hard mode removes the rows entirely.
func TestForgetSubject_HardDeletes(t *testing.T) {
	s, ids := seedForgetFixture(t)
	res, err := s.ForgetSubject("project x", "", true, false)
	if err != nil {
		t.Fatalf("ForgetSubject hard: %v", err)
	}
	if len(res.Matched) != 2 {
		t.Fatalf("matched %d, want 2", len(res.Matched))
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM nodes`); n != 1 {
		t.Errorf("hard delete left %d nodes, want 1", n)
	}
	if _, err := s.GetNode(ids["px1"]); err == nil {
		t.Error("hard-deleted node still readable")
	}
	// FTS must not surface the deleted content.
	if s.FTSAvailable() {
		hits, err := s.KeywordSearch("atlantis", 5)
		if err != nil {
			t.Fatalf("KeywordSearch: %v", err)
		}
		if len(hits) != 0 {
			t.Errorf("FTS still surfaces forgotten content: %d hits", len(hits))
		}
	}
}

// Subtype narrows the match.
func TestForgetSubject_SubtypeFilter(t *testing.T) {
	s, ids := seedForgetFixture(t)
	res, err := s.ForgetSubject("project x", "ops", false, false)
	if err != nil {
		t.Fatalf("ForgetSubject: %v", err)
	}
	if len(res.Matched) != 1 || res.Matched[0].ID != ids["px2"] {
		t.Fatalf("subtype filter: got %+v want just px2", res.Matched)
	}
	n, _ := s.GetNode(ids["px1"])
	if n.Status != "active" {
		t.Errorf("out-of-subtype node touched")
	}
}
