package store

import (
	"testing"
	"time"
)

// TestSurpriseUpdatedAfterSurfaced verifies surprise = 1.0 when updated_at > last_surfaced.
func TestSurpriseUpdatedAfterSurfaced(t *testing.T) {
	past := time.Now().Add(-24 * time.Hour)
	recent := time.Now().Add(-1 * time.Hour)
	n := &Node{
		LastSurfaced: &past,
		UpdatedAt:    &recent,
	}
	got := Surprise(n)
	if got != 1.0 {
		t.Errorf("Surprise(updated_at > last_surfaced) = %f, want 1.0", got)
	}
}

// TestSurpriseUpdatedAtSetLastSurfacedNil verifies surprise = 1.0 when
// updated_at is set but last_surfaced is nil.
func TestSurpriseUpdatedAtSetLastSurfacedNil(t *testing.T) {
	recent := time.Now().Add(-1 * time.Hour)
	n := &Node{
		UpdatedAt:    &recent,
		LastSurfaced: nil,
	}
	got := Surprise(n)
	if got != 1.0 {
		t.Errorf("Surprise(updated_at set, last_surfaced nil) = %f, want 1.0", got)
	}
}

// TestSurpriseBothNil verifies surprise = 0.5 when both fields are nil.
func TestSurpriseBothNil(t *testing.T) {
	n := &Node{
		UpdatedAt:    nil,
		LastSurfaced: nil,
	}
	got := Surprise(n)
	if got != 0.5 {
		t.Errorf("Surprise(both nil) = %f, want 0.5", got)
	}
}

// TestSurpriseNeverUpdatedRecentlySurfaced verifies gradual growth when
// last_surfaced > updated_at (or updated_at is nil).
func TestSurpriseNeverUpdatedRecentlySurfaced(t *testing.T) {
	recentlySurfaced := time.Now().Add(-time.Hour)
	n := &Node{
		UpdatedAt:    nil, // never updated
		LastSurfaced: &recentlySurfaced,
	}
	got := Surprise(n)
	// With rate 0.01 and 1 hour (~1 hour = 1 unit), expect small value > 0.
	// 1 - exp(-0.01 * 1) ≈ 0.01 (very small).
	if got <= 0 || got >= 1 {
		t.Errorf("Surprise(recently surfaced, no update) = %f, want in (0, 1)", got)
	}
	// Should be a small value (node was surfaced 1 hour ago, rate=0.01).
	if got > 0.1 {
		t.Errorf("Surprise(1 hour since surfaced, rate=0.01) = %f, want < 0.1", got)
	}
}

// TestSurpriseNotSurfacedInLongTime verifies that surprise grows over time.
func TestSurpriseNotSurfacedInLongTime(t *testing.T) {
	longAgo := time.Now().Add(-240 * time.Hour) // 10 days
	n := &Node{
		UpdatedAt:    nil,
		LastSurfaced: &longAgo,
	}
	got := Surprise(n)
	// 1 - exp(-0.01 * 240) = 1 - exp(-2.4) ≈ 0.91
	if got < 0.8 {
		t.Errorf("Surprise(10 days since surfaced) = %f, want > 0.8", got)
	}
	if got >= 1.0 {
		t.Errorf("Surprise should be < 1.0 for non-nil last_surfaced with no update, got %f", got)
	}
}

// TestSurpriseReturnsValueInRange verifies [0, 1] bounds for all cases.
func TestSurpriseReturnsValueInRange(t *testing.T) {
	now := time.Now()
	past := now.Add(-100 * time.Hour)
	future := now.Add(time.Hour)

	cases := []*Node{
		{UpdatedAt: nil, LastSurfaced: nil},
		{UpdatedAt: &past, LastSurfaced: nil},
		{UpdatedAt: nil, LastSurfaced: &past},
		{UpdatedAt: &now, LastSurfaced: &past},   // updated after surfaced
		{UpdatedAt: &past, LastSurfaced: &now},   // surfaced after updated
		{UpdatedAt: &future, LastSurfaced: &now}, // updated in future (updated_at > last_surfaced)
	}
	for i, n := range cases {
		got := Surprise(n)
		if got < 0 || got > 1 {
			t.Errorf("case %d: Surprise = %f, out of [0, 1]", i, got)
		}
	}
}

// TestPrimeScoreBlends verifies the 0.6/0.4 weight split.
func TestPrimeScoreBlends(t *testing.T) {
	// Use recently surfaced (surprise ≈ 0): surfaced 1s ago.
	recentSurf := time.Now().Add(-time.Second)
	n1 := &Node{Importance: 1.0, UpdatedAt: nil, LastSurfaced: &recentSurf}
	s1 := PrimeScore(n1)
	// surprise(n1) ≈ 1 - exp(-0.01 * (1/3600)) ≈ 0 (tiny)
	// PrimeScore ≈ 0.6 * 1.0 + 0.4 * ~0 ≈ 0.6
	if s1 < 0.55 || s1 > 0.65 {
		t.Errorf("PrimeScore(imp=1.0, surprise≈0) = %f, want ≈0.6", s1)
	}

	// Node with importance=0.0 and surprise=1.0 → PrimeScore = 0.4
	past := time.Now().Add(-time.Hour)
	recent := time.Now().Add(-time.Minute)
	n2 := &Node{Importance: 0.0, UpdatedAt: &recent, LastSurfaced: &past}
	s2 := PrimeScore(n2)
	if s2 < 0.35 || s2 > 0.45 {
		t.Errorf("PrimeScore(imp=0.0, surprise=1.0) = %f, want ≈0.4", s2)
	}

	// Higher importance should yield higher PrimeScore when surprise is equal.
	n3 := &Node{Importance: 0.3, UpdatedAt: nil, LastSurfaced: nil} // surprise=0.5
	n4 := &Node{Importance: 0.9, UpdatedAt: nil, LastSurfaced: nil} // surprise=0.5
	s3 := PrimeScore(n3)
	s4 := PrimeScore(n4)
	if s4 <= s3 {
		t.Errorf("higher importance should yield higher PrimeScore: got s3=%f, s4=%f", s3, s4)
	}
}

// TestPrimeScoreHighImportanceVsMaxSurprise verifies the crossover described in ADR-007:
// importance=0.3 + surprise=1.0 should outscore importance=0.9 + surprise=0.0.
func TestPrimeScoreHighImportanceVsMaxSurprise(t *testing.T) {
	// importance=0.3, surprise=1.0 → 0.6*0.3 + 0.4*1.0 = 0.58
	past := time.Now().Add(-time.Hour)
	recent := time.Now().Add(-time.Minute)
	highSurprise := &Node{Importance: 0.3, UpdatedAt: &recent, LastSurfaced: &past}

	// importance=0.9, surprise≈0 → 0.6*0.9 + 0.4*0 = 0.54
	justSurfaced := time.Now().Add(-time.Second)
	highImportance := &Node{Importance: 0.9, UpdatedAt: nil, LastSurfaced: &justSurfaced}

	s1 := PrimeScore(highSurprise)
	s2 := PrimeScore(highImportance)

	if s1 <= s2 {
		t.Errorf("high-surprise moderate-importance (%.3f) should outscore high-importance low-surprise (%.3f)", s1, s2)
	}
}
