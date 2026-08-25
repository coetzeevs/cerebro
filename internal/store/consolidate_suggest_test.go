package store

// consolidate_suggest_test.go — agentic-eq7a candidate selection (TDD).
//
// Model B: the AGENT synthesizes; cerebro only surfaces candidates. The
// selection is deterministic SQL — active episodes grouped by subtype,
// biggest groups first, oldest episodes first within a group — so the agent
// gets stable clusters to reason over without any embedding cost.

import "testing"

func TestConsolidationCandidates_GroupsActiveEpisodesBySubtype(t *testing.T) {
	s := testStore(t)
	for i := 0; i < 3; i++ {
		if _, err := s.AddNode(&AddNodeOpts{Type: TypeEpisode, Subtype: "build", Content: "build episode", Importance: 0.5}); err != nil {
			t.Fatalf("AddNode: %v", err)
		}
	}
	for i := 0; i < 2; i++ {
		if _, err := s.AddNode(&AddNodeOpts{Type: TypeEpisode, Subtype: "debug", Content: "debug episode", Importance: 0.5}); err != nil {
			t.Fatalf("AddNode: %v", err)
		}
	}
	// Noise that must NOT surface: a concept, and a consolidated episode.
	if _, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Subtype: "build", Content: "a concept", Importance: 0.5}); err != nil {
		t.Fatalf("AddNode concept: %v", err)
	}
	doneID, err := s.AddNode(&AddNodeOpts{Type: TypeEpisode, Subtype: "build", Content: "already consolidated", Importance: 0.5})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	if _, err := s.db.Exec(`UPDATE nodes SET status = 'consolidated' WHERE id = ?`, doneID); err != nil {
		t.Fatalf("flipping status: %v", err)
	}

	groups, err := s.ConsolidationCandidates(10)
	if err != nil {
		t.Fatalf("ConsolidationCandidates: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2 (build, debug): %+v", len(groups), groups)
	}
	if groups[0].Subtype != "build" || groups[0].Count != 3 {
		t.Errorf("largest group first: got %q/%d want build/3", groups[0].Subtype, groups[0].Count)
	}
	if groups[1].Subtype != "debug" || groups[1].Count != 2 {
		t.Errorf("second group: got %q/%d want debug/2", groups[1].Subtype, groups[1].Count)
	}
	for _, g := range groups {
		for _, n := range g.Nodes {
			if n.Type != TypeEpisode || n.Status != "active" {
				t.Errorf("non-candidate surfaced: %s/%s", n.Type, n.Status)
			}
		}
	}
}

func TestConsolidationCandidates_PerGroupLimitOldestFirst(t *testing.T) {
	s := testStore(t)
	var ids []string
	for i := 0; i < 5; i++ {
		id, err := s.AddNode(&AddNodeOpts{Type: TypeEpisode, Content: "ep", Importance: 0.5})
		if err != nil {
			t.Fatalf("AddNode: %v", err)
		}
		ids = append(ids, id)
	}
	// Stagger created_at so ordering is observable (oldest = ids[0]).
	for i, id := range ids {
		if _, err := s.db.Exec(`UPDATE nodes SET created_at = datetime('now', ?) WHERE id = ?`,
			// -5h, -4h, ... -1h
			"-"+string(rune('5'-i))+" hours", id); err != nil {
			t.Fatalf("backdating: %v", err)
		}
	}

	groups, err := s.ConsolidationCandidates(3)
	if err != nil {
		t.Fatalf("ConsolidationCandidates: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("got %d groups, want 1", len(groups))
	}
	g := groups[0]
	if g.Count != 5 {
		t.Errorf("group Count must be the TOTAL (5), not the listed slice: got %d", g.Count)
	}
	if len(g.Nodes) != 3 {
		t.Fatalf("per-group limit not applied: got %d nodes, want 3", len(g.Nodes))
	}
	if g.Nodes[0].ID != ids[0] {
		t.Errorf("oldest-first ordering broken: first listed %s, want %s", g.Nodes[0].ID, ids[0])
	}
	// Groups with a single episode are not rollup material but Count keeps
	// the total honest; singletons are included only when nothing else exists
	// (the caller decides). No assertion needed here beyond the above.
}
