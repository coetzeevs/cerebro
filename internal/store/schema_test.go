package store

import "testing"

// IncrMeta (agentic-73l6 AC4): atomic integer increment on a schema_meta key.
// Missing key starts at 1; subsequent calls increment; a non-integer existing
// value resets via the documented CAST semantics (SQLite CAST of non-numeric
// text → 0, then +1 = 1). Used by the lazy-expansion skip counter
// (stats.expansion_skips), which is observability data, not ledger data (R5).
func TestIncrMeta_StartsAtOneAndIncrements(t *testing.T) {
	s := testStore(t)

	const key = "stats.expansion_skips"

	// Missing key → first increment writes "1".
	if err := s.IncrMeta(key); err != nil {
		t.Fatalf("IncrMeta (fresh): %v", err)
	}
	got, err := s.GetMeta(key)
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if got != "1" {
		t.Errorf("after first IncrMeta, value = %q, want \"1\"", got)
	}

	// Second increment → "2".
	if err := s.IncrMeta(key); err != nil {
		t.Fatalf("IncrMeta (second): %v", err)
	}
	got, _ = s.GetMeta(key)
	if got != "2" {
		t.Errorf("after second IncrMeta, value = %q, want \"2\"", got)
	}
}

// R5 (documented behaviour): a corrupted non-integer value resets to 1 via
// CAST — accepted for an observability counter.
func TestIncrMeta_NonIntegerValueResetsToOne(t *testing.T) {
	s := testStore(t)

	const key = "stats.expansion_skips"
	if err := s.SetMeta(key, "garbage"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}
	if err := s.IncrMeta(key); err != nil {
		t.Fatalf("IncrMeta over non-integer: %v", err)
	}
	got, _ := s.GetMeta(key)
	if got != "1" {
		t.Errorf("IncrMeta over %q = %q, want \"1\" (CAST reset semantics)", "garbage", got)
	}
}

// IncrMeta must not disturb sibling schema_meta rows (config.* lives in the
// same table).
func TestIncrMeta_DoesNotTouchOtherKeys(t *testing.T) {
	s := testStore(t)

	if err := s.SetMeta("config.expand_threshold", "0.5"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}
	if err := s.IncrMeta("stats.expansion_skips"); err != nil {
		t.Fatalf("IncrMeta: %v", err)
	}
	got, _ := s.GetMeta("config.expand_threshold")
	if got != "0.5" {
		t.Errorf("sibling key mutated: config.expand_threshold = %q, want \"0.5\"", got)
	}
}
