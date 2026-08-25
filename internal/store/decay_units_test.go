package store

import (
	"math"
	"testing"
	"time"
)

// TestCompositeRecencyDecaysPerDay pins the agentic-N2 (C3 "wire-lite") fix:
// decay_rate is a PER-DAY lambda per ADR-003's half-life table (episode 0.15
// ~= 1-2 weeks). The shipped code applied it per-HOUR, making effective decay
// ~24x faster than documented intent (episode half-life 4.6h) and driving the
// recency term to ~0 for anything not touched today.
func TestCompositeRecencyDecaysPerDay(t *testing.T) {
	yesterday := time.Now().Add(-24 * time.Hour)
	n := &Node{Importance: 0.5, DecayRate: 0.15, AccessCount: 0, LastAccessed: yesterday}

	got := compositeScore(n, 0, 0)
	// 0.25*importance_term + 0.25*exp(-0.15 * 1 day)
	want := 0.25*0.5 + 0.25*math.Exp(-0.15)
	if math.Abs(got-want) > 0.005 {
		t.Errorf("per-day decay violated: got %.4f want %.4f (per-hour would give ~%.4f)",
			got, want, 0.25*0.5+0.25*math.Exp(-0.15*24))
	}
}

// TestRetentionRecencyDecaysPerDay: same unit fix in the GC retention score.
// A 30-day-old procedure (lambda 0.005 ~= 6+ month half-life) must retain a
// recency term of exp(-0.15) ~= 0.86, not the per-hour exp(-3.6) ~= 0.03.
func TestRetentionRecencyDecaysPerDay(t *testing.T) {
	monthAgo := time.Now().Add(-30 * 24 * time.Hour)
	n := &Node{Importance: 0.5, DecayRate: 0.005, AccessCount: 0, LastAccessed: monthAgo}

	got := retentionScore(n)
	want := 0.5*0.5 + 0.5*math.Exp(-0.005*30)
	if math.Abs(got-want) > 0.005 {
		t.Errorf("per-day retention violated: got %.4f want %.4f", got, want)
	}
}

// TestTouchAccessed pins the recall->telemetry wire (N2): a batched,
// increment-and-timestamp touch for retrieved nodes, so the scoring model's
// access/recency terms finally receive the signal they were designed around
// (live telemetry showed access_count=0 on every node after 6 months).
func TestTouchAccessed(t *testing.T) {
	s := testStore(t)
	a, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "touch a", Importance: 0.5})
	b, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "touch b", Importance: 0.5})

	if err := s.TouchAccessed([]string{a, b}); err != nil {
		t.Fatalf("TouchAccessed: %v", err)
	}
	if err := s.TouchAccessed([]string{a}); err != nil {
		t.Fatalf("TouchAccessed second: %v", err)
	}

	var ac int
	if err := s.db.QueryRow(`SELECT access_count FROM nodes WHERE id = ?`, a).Scan(&ac); err != nil {
		t.Fatalf("read a: %v", err)
	}
	if ac != 2 {
		t.Errorf("node a access_count: got %d want 2", ac)
	}
	if err := s.db.QueryRow(`SELECT access_count FROM nodes WHERE id = ?`, b).Scan(&ac); err != nil {
		t.Fatalf("read b: %v", err)
	}
	if ac != 1 {
		t.Errorf("node b access_count: got %d want 1", ac)
	}
	// Empty input is a no-op, not an error (best-effort contract).
	if err := s.TouchAccessed(nil); err != nil {
		t.Errorf("TouchAccessed(nil) should no-op: %v", err)
	}
}
