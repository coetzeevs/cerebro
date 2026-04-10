package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/coetzeevs/cerebro/brain"
	"github.com/coetzeevs/cerebro/internal/store"
)

// testBrain creates a temporary brain for testing prime mode.
func testBrain(t *testing.T) *brain.Brain {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sqlite")
	b, err := brain.Init(path, brain.EmbedConfig{})
	if err != nil {
		t.Fatalf("brain.Init: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })
	return b
}

// TestPrimeStratifiedIncludesRecentStratum verifies that primeStratified includes
// a "recently changed" stratum that can surface low-importance nodes updated in last 48h.
func TestPrimeStratifiedIncludesRecentStratum(t *testing.T) {
	b := testBrain(t)

	const limit = 10

	// Fill the concept stratum (35% of 10 = 3 slots) with high-importance concepts.
	for i := 0; i < 6; i++ {
		if _, err := b.Add("filler concept", store.TypeConcept, brain.WithImportance(0.9)); err != nil {
			t.Fatalf("Add concept %d: %v", i, err)
		}
	}
	// Fill the procedure stratum (25% of 10 = 2 slots).
	for i := 0; i < 4; i++ {
		if _, err := b.Add("filler procedure", store.TypeProcedure, brain.WithImportance(0.9)); err != nil {
			t.Fatalf("Add procedure %d: %v", i, err)
		}
	}
	// Fill the reflection stratum (10% of 10 = 1 slot).
	for i := 0; i < 3; i++ {
		if _, err := b.Add("filler reflection", store.TypeReflection, brain.WithImportance(0.9)); err != nil {
			t.Fatalf("Add reflection %d: %v", i, err)
		}
	}

	// Add a very low-importance reflection. With the reflection stratum taking only
	// 1 slot and being filled by high-importance ones, this one would NOT appear
	// via the reflections stratum. But if it was recently updated, it should appear
	// via the recent stratum.
	lowID, err := b.Add("low importance but recently updated reflection",
		store.TypeReflection, brain.WithImportance(0.01))
	if err != nil {
		t.Fatalf("Add low reflection: %v", err)
	}
	// Update it so it gets updated_at set.
	if err := b.Update(lowID, brain.WithContent("low importance — updated 1 hour ago")); err != nil {
		t.Fatalf("Update low reflection: %v", err)
	}
	// Force updated_at to be clearly in the future relative to all fillers'
	// created_at (which use SQLite's 1-second-resolution CURRENT_TIMESTAMP).
	// This ensures lowID sorts first in "recently_changed" order even with
	// timestamp collisions from fast test execution.
	futureTime := time.Now().UTC().Add(time.Hour).Format("2006-01-02 15:04:05")
	if _, err := b.Store().DB().Exec(`UPDATE nodes SET updated_at = ? WHERE id = ?`, futureTime, lowID); err != nil {
		t.Fatalf("force updated_at for lowID: %v", err)
	}

	nodes := primeStratified(b, limit)

	var foundLow bool
	for _, n := range nodes {
		if n.ID == lowID {
			foundLow = true
		}
	}
	if !foundLow {
		t.Errorf("expected recently-updated low-importance reflection in prime results via recent stratum")
		for _, n := range nodes {
			t.Logf("  node: type=%s id=%s importance=%.2f", n.Type, n.ID[:8], n.Importance)
		}
	}
}

// TestPrimeStratifiedRecentStratumDeduplicates verifies that a node already
// selected by a type stratum is not duplicated by the recent stratum.
func TestPrimeStratifiedRecentStratumDeduplicates(t *testing.T) {
	b := testBrain(t)

	// Add a high-importance concept and update it recently.
	// It should appear in the concepts stratum AND qualify for recent stratum,
	// but must only appear once.
	conceptID, err := b.Add("important concept", store.TypeConcept, brain.WithImportance(0.95))
	if err != nil {
		t.Fatalf("Add concept: %v", err)
	}
	if err := b.Update(conceptID, brain.WithContent("important concept — updated")); err != nil {
		t.Fatalf("Update concept: %v", err)
	}

	nodes := primeStratified(b, 20)

	// Count occurrences of conceptID.
	count := 0
	for _, n := range nodes {
		if n.ID == conceptID {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected conceptID to appear exactly once, got %d times", count)
	}
}

// TestPrimeStratifiedBudgetRespectsLimit verifies that the total never exceeds limit.
func TestPrimeStratifiedBudgetRespectsLimit(t *testing.T) {
	b := testBrain(t)

	// Add enough nodes of each type to fill budgets.
	for i := 0; i < 5; i++ {
		if _, err := b.Add("concept", store.TypeConcept, brain.WithImportance(float64(i+1)*0.1)); err != nil {
			t.Fatalf("Add concept %d: %v", i, err)
		}
	}
	for i := 0; i < 4; i++ {
		if _, err := b.Add("procedure", store.TypeProcedure, brain.WithImportance(float64(i+1)*0.1)); err != nil {
			t.Fatalf("Add procedure %d: %v", i, err)
		}
	}
	for i := 0; i < 3; i++ {
		if _, err := b.Add("episode", store.TypeEpisode, brain.WithImportance(0.5)); err != nil {
			t.Fatalf("Add episode %d: %v", i, err)
		}
	}
	for i := 0; i < 2; i++ {
		if _, err := b.Add("reflection", store.TypeReflection, brain.WithImportance(float64(i+1)*0.2)); err != nil {
			t.Fatalf("Add reflection %d: %v", i, err)
		}
	}

	nodes := primeStratified(b, 10)

	if len(nodes) > 10 {
		t.Errorf("expected at most 10 nodes, got %d", len(nodes))
	}
	if len(nodes) == 0 {
		t.Fatal("expected some nodes from prime")
	}
}

// TestPrimeStratifiedPrimeScoreOrdering verifies that within a type stratum,
// a moderately-important but stale node can rank ahead of a high-importance
// recently-surfaced node.
func TestPrimeStratifiedPrimeScoreOrdering(t *testing.T) {
	b := testBrain(t)

	// Add two concepts:
	// - highImp: importance=0.9, just surfaced (surprise≈0) → PrimeScore≈0.54
	// - staleMed: importance=0.3, updated after last surfaced (surprise=1.0) → PrimeScore=0.58

	highImpID, err := b.Add("high importance concept", store.TypeConcept, brain.WithImportance(0.9))
	if err != nil {
		t.Fatalf("Add highImp: %v", err)
	}
	// Mark highImp as recently surfaced (small surprise).
	if err := b.Store().TouchSurfaced([]string{highImpID}); err != nil {
		t.Fatalf("TouchSurfaced highImp: %v", err)
	}

	staleMedID, err := b.Add("moderate importance stale concept", store.TypeConcept, brain.WithImportance(0.3))
	if err != nil {
		t.Fatalf("Add staleMed: %v", err)
	}
	// Set updated_at AFTER last_surfaced to trigger surprise=1.0.
	pastSurfaced := time.Now().UTC().Add(-2 * time.Hour).Format("2006-01-02 15:04:05")
	recentUpdated := time.Now().UTC().Add(-time.Hour).Format("2006-01-02 15:04:05")
	if _, err := b.Store().DB().Exec(
		`UPDATE nodes SET last_surfaced = ?, updated_at = ? WHERE id = ?`,
		pastSurfaced, recentUpdated, staleMedID,
	); err != nil {
		t.Fatalf("set staleMed timestamps: %v", err)
	}

	// With limit=2 concepts budget, primeStratified should return both
	// but stale should rank first.
	nodes := primeStratified(b, 10)

	// Find positions of both nodes.
	posHigh, posMed := -1, -1
	for i, n := range nodes {
		if n.ID == highImpID {
			posHigh = i
		}
		if n.ID == staleMedID {
			posMed = i
		}
	}

	if posHigh < 0 {
		t.Error("highImp node not found in prime results")
	}
	if posMed < 0 {
		t.Error("staleMed node not found in prime results")
	}

	if posHigh >= 0 && posMed >= 0 && posMed > posHigh {
		t.Errorf("stale+moderate (pos %d) should rank before high+recent (pos %d)", posMed, posHigh)
	}
}

// TestPrimeStratifiedTouchSurfaced verifies that after primeStratified, all
// selected nodes have last_surfaced set.
func TestPrimeStratifiedTouchSurfaced(t *testing.T) {
	b := testBrain(t)

	ids := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		id, err := b.Add("concept", store.TypeConcept, brain.WithImportance(0.7))
		if err != nil {
			t.Fatalf("Add concept %d: %v", i, err)
		}
		ids = append(ids, id)
	}

	// Verify last_surfaced is nil before prime.
	for _, id := range ids {
		node, err := b.Store().GetNode(id)
		if err != nil {
			t.Fatalf("GetNode: %v", err)
		}
		if node.LastSurfaced != nil {
			t.Errorf("expected nil last_surfaced before prime, got %v", node.LastSurfaced)
		}
	}

	_ = primeStratified(b, 10)

	// After prime, all returned nodes should have last_surfaced set.
	for _, id := range ids {
		node, err := b.Store().GetNode(id)
		if err != nil {
			t.Fatalf("GetNode post-prime: %v", err)
		}
		if node.LastSurfaced == nil {
			t.Errorf("expected last_surfaced set after prime for node %s", id[:8])
		}
	}
}

// TestPrimeStratifiedRecentWindowIs48h verifies that the recent stratum uses
// a 48-hour window (nodes updated more than 48h ago are excluded).
func TestPrimeStratifiedRecentWindowIs48h(t *testing.T) {
	b := testBrain(t)

	const limit = 10

	// Fill type strata with high-importance nodes so low-importance ones
	// cannot appear via type strata.
	for i := 0; i < 4; i++ {
		if _, err := b.Add("filler concept", store.TypeConcept, brain.WithImportance(0.9)); err != nil {
			t.Fatalf("Add filler concept: %v", err)
		}
	}
	for i := 0; i < 3; i++ {
		if _, err := b.Add("filler procedure", store.TypeProcedure, brain.WithImportance(0.9)); err != nil {
			t.Fatalf("Add filler procedure: %v", err)
		}
	}
	for i := 0; i < 2; i++ {
		if _, err := b.Add("filler reflection", store.TypeReflection, brain.WithImportance(0.9)); err != nil {
			t.Fatalf("Add filler reflection: %v", err)
		}
	}

	// Add a node that was updated 3 days ago (outside 48h window).
	oldUpdatedID, err := b.Add("stale node", store.TypeConcept, brain.WithImportance(0.01))
	if err != nil {
		t.Fatalf("Add stale node: %v", err)
	}
	if err := b.Update(oldUpdatedID, brain.WithContent("stale node — updated 3 days ago")); err != nil {
		t.Fatalf("Update stale node: %v", err)
	}
	// Force updated_at to 3 days ago (outside 48h window).
	oldTime := time.Now().UTC().Add(-72 * time.Hour).Format("2006-01-02 15:04:05")
	if _, err := b.Store().DB().Exec(`UPDATE nodes SET updated_at = ?, created_at = ? WHERE id = ?`,
		oldTime, oldTime, oldUpdatedID); err != nil {
		t.Fatalf("force old timestamps: %v", err)
	}

	// Add a node updated very recently (within 48h window).
	recentID, err := b.Add("fresh node", store.TypeConcept, brain.WithImportance(0.01))
	if err != nil {
		t.Fatalf("Add fresh node: %v", err)
	}
	if err := b.Update(recentID, brain.WithContent("fresh node — updated 1 hour ago")); err != nil {
		t.Fatalf("Update fresh node: %v", err)
	}
	// Force updated_at to be clearly in the future relative to all filler nodes'
	// created_at, ensuring it sorts first in "recently_changed" order.
	recentTime := time.Now().UTC().Add(time.Hour).Format("2006-01-02 15:04:05")
	if _, err := b.Store().DB().Exec(`UPDATE nodes SET updated_at = ? WHERE id = ?`, recentTime, recentID); err != nil {
		t.Fatalf("force recent updated_at: %v", err)
	}

	nodes := primeStratified(b, limit)

	var foundOldUpdated, foundRecent bool
	for _, n := range nodes {
		if n.ID == oldUpdatedID {
			foundOldUpdated = true
		}
		if n.ID == recentID {
			foundRecent = true
		}
	}

	if foundOldUpdated {
		t.Errorf("stale node (updated 3 days ago) should NOT appear via recent stratum")
	}
	if !foundRecent {
		t.Errorf("recently updated node should appear via recent stratum")
	}
}
