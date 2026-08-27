package store

// inbox_test.go — agentic-m8m3 (TDD): capture-with-approval. Candidates
// carry status='candidate' — a quarantine invisible to EVERY retrieval
// surface (vector, keyword, prime, GC all filter status='active') until an
// explicit approve; discard removes the candidate entirely. The
// team-tier governance research's lesson: auto-capture and safety coexist
// only through quarantine — capture for CURATION, never auto-commit.

import "testing"

func TestCandidate_QuarantinedFromRetrieval(t *testing.T) {
	s := testStoreWithVec(t, 4)
	id, err := s.AddCandidateNode(&AddNodeOpts{Type: TypeEpisode, Content: "quarantined zebra fact", Importance: 0.5, OriginActor: "agent", OriginChannel: "hook"})
	if err != nil {
		t.Fatalf("AddCandidateNode: %v", err)
	}
	// Structural quarantine: a candidate is NOT a node at all.
	if _, err := s.GetNode(id); err == nil {
		t.Fatal("candidate readable as a node — quarantine must be structural")
	}
	list, err := s.ListCandidates()
	if err != nil || len(list) != 1 {
		t.Fatalf("ListCandidates: %v (%d)", err, len(list))
	}
	if list[0].Status != "candidate" || list[0].OriginActor != "agent" {
		t.Errorf("candidate presentation wrong: %+v", list[0])
	}
	// Invisible to the keyword lane even if an FTS row existed.
	if s.FTSAvailable() {
		hits, err := s.KeywordSearch("zebra", 5)
		if err != nil {
			t.Fatalf("KeywordSearch: %v", err)
		}
		if len(hits) != 0 {
			t.Errorf("candidate leaked into keyword search: %d hits", len(hits))
		}
	}
	// GC must not evaluate candidates.
	res, err := s.GC(0.99, true)
	if err != nil {
		t.Fatalf("GC: %v", err)
	}
	if res.Evaluated != 0 {
		t.Errorf("GC evaluated %d nodes; candidates must be exempt", res.Evaluated)
	}
}

func TestListCandidates_OldestFirst(t *testing.T) {
	s := testStore(t)
	a, _ := s.AddCandidateNode(&AddNodeOpts{Type: TypeEpisode, Content: "first", Importance: 0.5})
	if _, err := s.db.Exec(`UPDATE nodes SET created_at = datetime('now','-2 hours') WHERE id = ?`, a); err != nil {
		t.Fatal(err)
	}
	b, _ := s.AddCandidateNode(&AddNodeOpts{Type: TypeEpisode, Content: "second", Importance: 0.5})
	// Noise: an active node never shows in the inbox.
	if _, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "active", Importance: 0.5}); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListCandidates()
	if err != nil {
		t.Fatalf("ListCandidates: %v", err)
	}
	if len(list) != 2 || list[0].ID != a || list[1].ID != b {
		t.Fatalf("inbox order/content wrong: %d items", len(list))
	}
}

func TestApproveCandidate_ActivatesAndIndexes(t *testing.T) {
	s := testStore(t)
	id, _ := s.AddCandidateNode(&AddNodeOpts{Type: TypeEpisode, Content: "approvable xylograph fact", Importance: 0.5})
	if err := s.ApproveCandidate(id); err != nil {
		t.Fatalf("ApproveCandidate: %v", err)
	}
	n, err := s.GetNode(id)
	if err != nil {
		t.Fatalf("approved candidate not a node: %v", err)
	}
	if n.Status != "active" {
		t.Fatalf("approved status: got %s want active", n.Status)
	}
	if s.FTSAvailable() {
		hits, err := s.KeywordSearch("xylograph", 5)
		if err != nil {
			t.Fatalf("KeywordSearch: %v", err)
		}
		if len(hits) != 1 {
			t.Errorf("approved candidate not keyword-indexed: %d hits", len(hits))
		}
	}
	// Approving again errors (the inbox row is gone — no silent re-approve).
	if err := s.ApproveCandidate(id); err == nil {
		t.Error("double-approve must error")
	}
}

func TestDiscardCandidate_RemovesEntirely(t *testing.T) {
	s := testStore(t)
	id, _ := s.AddCandidateNode(&AddNodeOpts{Type: TypeEpisode, Content: "never was a memory", Importance: 0.5})
	if err := s.DiscardCandidate(id); err != nil {
		t.Fatalf("DiscardCandidate: %v", err)
	}
	if list, _ := s.ListCandidates(); len(list) != 0 {
		t.Error("discarded candidate still in inbox")
	}
	// Discarding an ACTIVE node must refuse — discard is inbox-only, not a
	// general delete (forget/gc own those paths).
	active, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "real memory", Importance: 0.5})
	if err := s.DiscardCandidate(active); err == nil {
		t.Error("discarding an active node must error")
	}
}
