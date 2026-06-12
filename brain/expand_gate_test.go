package brain

import (
	"math"
	"path/filepath"
	"testing"

	"github.com/coetzeevs/cerebro/internal/store"
)

// gateNode builds a minimal ScoredNode for predicate/cut tests. Only ID,
// Score and Similarity are load-bearing for the gate logic (pure functions —
// f132ce9c pattern, no Ollama needed).
func gateNode(id string, score, sim float64) store.ScoredNode {
	return store.ScoredNode{
		Node:       store.Node{ID: id},
		Score:      score,
		Similarity: sim,
	}
}

// simsToNodes maps a similarity slice to ScoredNodes (Score mirrors Similarity
// unless the test cares about composite order).
func simsToNodes(sims []float64) []store.ScoredNode {
	out := make([]store.ScoredNode, len(sims))
	for i, s := range sims {
		out[i] = gateNode(string(rune('a'+i)), s, s)
	}
	return out
}

// ── shouldSkipExpansion: the T1 ∨ T2 predicate (Design §2) ──────────────────

func TestShouldSkipExpansion(t *testing.T) {
	cases := []struct {
		name      string
		sims      []float64
		requested int
		topTh     float64
		spreadTh  float64
		want      bool
	}{
		// AC2a — T1 fires when top-1 similarity strictly exceeds the threshold.
		{"AC2a_T1_fires_above_threshold", []float64{0.9, 0.7}, 5, 0.8, 0.0, true},
		{"T1_strict_gt_equal_does_not_fire", []float64{0.8, 0.7}, 5, 0.8, 0.0, false},
		{"T1_threshold_1.0_effectively_off_sim_1.0", []float64{1.0, 0.9}, 5, 1.0, 0.0, false},
		{"T1_fires_on_singleton_K1", []float64{0.95}, 5, 0.8, 0.0, true},

		// AC2b — T2 fires when the full-set spread is strictly below threshold.
		{"AC2b_T2_fires_small_spread_full_set", []float64{0.70, 0.695, 0.69, 0.685, 0.68}, 5, 0.0, 0.05, true},
		{"T2_partial_set_K_lt_R_guard", []float64{0.70, 0.695, 0.69, 0.685, 0.68}, 10, 0.0, 0.05, false},
		{"T2_singleton_K1_guard", []float64{0.7}, 1, 0.0, 0.5, false},
		{"T2_strict_lt_equal_spread_does_not_fire", []float64{0.75, 0.70}, 2, 0.0, 0.05, false},
		{"T2_large_spread_does_not_fire", []float64{0.9, 0.5}, 2, 0.0, 0.05, false},

		// AC2c — neither condition met: gate must not fire.
		{"AC2c_neither_fires", []float64{0.9, 0.5}, 5, 0.99, 0.0, false},

		// Edge cases (Design §2, all decided to DISABLE rather than fire).
		{"empty_set_K0", nil, 5, 0.8, 0.5, false},
		{"ties_at_zero_zero", []float64{0.7, 0.7, 0.7}, 3, 0.0, 0.0, false},
		{"sim_1.0_at_zero_zero", []float64{1.0}, 1, 0.0, 0.0, false},

		// Disabled sentinels: 0.0 = structurally off (the `> 0` guards — without
		// them, strict-> at 0.0 would invert into "always fire").
		{"T1_zero_sentinel_off", []float64{0.99}, 1, 0.0, 0.0, false},
		{"T2_zero_sentinel_off_tiny_spread", []float64{0.70, 0.699}, 2, 0.0, 0.0, false},

		// Order-independence: maxSim/minSim are computed over the set, not
		// positionally, so a future reorder upstream cannot silently break T1/T2.
		{"T1_order_independent_max_not_first", []float64{0.5, 0.95, 0.6}, 5, 0.8, 0.0, true},
		{"T2_order_independent_unsorted_full_set", []float64{0.69, 0.70, 0.685}, 3, 0.0, 0.05, true},

		// Out-of-range stored values (raw-sqlite3 bypass of validateUnitFloat —
		// TL Minor 3): the predicate fails open to the baseline (full expansion).
		{"T1_negative_threshold_disables", []float64{0.9}, 1, -1.0, 0.0, false},
		{"T1_above_one_threshold_inert", []float64{1.0}, 1, 5.0, 0.0, false},
		{"T2_negative_threshold_disables", []float64{0.70, 0.699}, 2, 0.0, -0.5, false},

		// NaN thresholds (bypass-written; CLI rejects post-S-1): NaN compares
		// false to everything, so both `> 0` guards fail and the gate DISABLES.
		{"T1_NaN_threshold_disables", []float64{0.99}, 1, math.NaN(), 0.0, false},
		{"T2_NaN_threshold_disables", []float64{0.70, 0.699}, 2, 0.0, math.NaN(), false},
		{"both_NaN_disable", []float64{0.99, 0.98}, 2, math.NaN(), math.NaN(), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldSkipExpansion(simsToNodes(tc.sims), tc.requested, tc.topTh, tc.spreadTh)
			if got != tc.want {
				t.Errorf("shouldSkipExpansion(sims=%v, K=%d, R=%d, top=%v, spread=%v) = %v, want %v",
					tc.sims, len(tc.sims), tc.requested, tc.topTh, tc.spreadTh, got, tc.want)
			}
		})
	}
}

// AC3(a) — property: at (0.0, 0.0) the predicate is constant-false over the
// whole edge-case corpus, so the gate-disabled pipeline is the pre-feature
// path by construction (the else-branch executes today's code verbatim).
func TestShouldSkipExpansion_ZeroZeroConstantFalse(t *testing.T) {
	corpus := [][]float64{
		nil,                  // empty
		{0.95},               // K=1, high confidence
		{1.0},                // exact-duplicate vector
		{0.7, 0.7, 0.7},      // ties
		{0.5, 0.95, 0.6},     // unsorted
		{0.70, 0.695, 0.69},  // tiny spread
		{0.0},                // zero similarity
		{1.0, 0.0},           // max spread
		{0.487, 0.45, 0.432}, // live low-confidence shape (q01)
	}
	for _, sims := range corpus {
		for _, requested := range []int{0, 1, len(sims), 20} {
			if shouldSkipExpansion(simsToNodes(sims), requested, 0.0, 0.0) {
				t.Errorf("predicate fired at (0.0, 0.0) for sims=%v requestedK=%d — 0.0 must mean OFF", sims, requested)
			}
		}
	}
}

// ── cutByScore: the skipped path's sort+cap (ExpandGraph tail parity) ───────

func TestCutByScore(t *testing.T) {
	t.Run("sorts_by_score_descending", func(t *testing.T) {
		in := []store.ScoredNode{
			gateNode("low", 0.2, 0.9),
			gateNode("high", 0.9, 0.5),
			gateNode("mid", 0.5, 0.7),
		}
		got := cutByScore(in, 10)
		wantOrder := []string{"high", "mid", "low"}
		if len(got) != 3 {
			t.Fatalf("len = %d, want 3", len(got))
		}
		for i, id := range wantOrder {
			if got[i].ID != id {
				t.Errorf("position %d = %q, want %q (Score-desc comparator, search.go ExpandGraph parity)", i, got[i].ID, id)
			}
		}
	})

	t.Run("caps_at_limit", func(t *testing.T) {
		in := []store.ScoredNode{
			gateNode("a", 0.9, 0), gateNode("b", 0.8, 0), gateNode("c", 0.7, 0),
		}
		got := cutByScore(in, 2)
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2 (cap at limit)", len(got))
		}
		if got[0].ID != "a" || got[1].ID != "b" {
			t.Errorf("capped order = [%s %s], want [a b]", got[0].ID, got[1].ID)
		}
	})

	t.Run("under_limit_no_cap", func(t *testing.T) {
		in := []store.ScoredNode{gateNode("a", 0.9, 0), gateNode("b", 0.8, 0)}
		if got := cutByScore(in, 10); len(got) != 2 {
			t.Errorf("len = %d, want 2 (K <= limit must not shrink)", len(got))
		}
	})

	t.Run("empty_identity", func(t *testing.T) {
		if got := cutByScore(nil, 10); len(got) != 0 {
			t.Errorf("len = %d, want 0", len(got))
		}
	})

	t.Run("limit_zero_empties", func(t *testing.T) {
		// ExpandGraph parity: `if len(results) > limit { results = results[:limit] }`
		// with limit 0 returns an empty slice.
		in := []store.ScoredNode{gateNode("a", 0.9, 0)}
		if got := cutByScore(in, 0); len(got) != 0 {
			t.Errorf("len = %d, want 0 (limit 0)", len(got))
		}
	})
}

// ── resolvers: brain-side config reads (env-free, GetMeta idiom) ────────────

func gateTestBrain(t *testing.T) *Brain {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gate.sqlite")
	b, err := Init(path, EmbedConfig{Provider: "none"})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })
	return b
}

func TestResolveExpandThresholds(t *testing.T) {
	b := gateTestBrain(t)

	// Unset → compiled defaults: T1 ACTIVE at 0.80, T2 OFF at 0.0 (Design §5).
	if got := resolveExpandThreshold(b.store); got != defaultExpandThreshold {
		t.Errorf("resolveExpandThreshold (unset) = %v, want %v", got, defaultExpandThreshold)
	}
	if got := resolveExpandSpreadThreshold(b.store); got != defaultExpandSpreadThreshold {
		t.Errorf("resolveExpandSpreadThreshold (unset) = %v, want %v", got, defaultExpandSpreadThreshold)
	}

	// Set → parsed value wins.
	_ = b.store.SetMeta("config.expand_threshold", "0.5")
	_ = b.store.SetMeta("config.expand_spread_threshold", "0.05")
	if got := resolveExpandThreshold(b.store); got != 0.5 {
		t.Errorf("resolveExpandThreshold (0.5) = %v, want 0.5", got)
	}
	if got := resolveExpandSpreadThreshold(b.store); got != 0.05 {
		t.Errorf("resolveExpandSpreadThreshold (0.05) = %v, want 0.05", got)
	}

	// Unparseable → default (mirrors resolveConfigFloat).
	_ = b.store.SetMeta("config.expand_threshold", "not_a_number")
	if got := resolveExpandThreshold(b.store); got != defaultExpandThreshold {
		t.Errorf("resolveExpandThreshold (garbage) = %v, want default %v", got, defaultExpandThreshold)
	}

	// Parseable-but-out-of-range passes through (TL Minor 3): the PREDICATE's
	// guards fail open to full expansion; the resolver does not clamp.
	_ = b.store.SetMeta("config.expand_threshold", "5.0")
	if got := resolveExpandThreshold(b.store); got != 5.0 {
		t.Errorf("resolveExpandThreshold (5.0) = %v, want pass-through 5.0", got)
	}
}

// Compiled defaults pin the Design §5 shipped values. The cmd/cerebro
// configRegistry Default strings, README rows and CHANGELOG must agree with
// these constants (TL Minor 4 — four-surface consistency).
func TestExpandGateShippedDefaults(t *testing.T) {
	if defaultExpandThreshold != 0.80 {
		t.Errorf("defaultExpandThreshold = %v, want 0.80 (Design §5, sweep-confirmed)", defaultExpandThreshold)
	}
	if defaultExpandSpreadThreshold != 0.0 {
		t.Errorf("defaultExpandSpreadThreshold = %v, want 0.0 (spread ships OFF — live anti-correlation, ledger #14)", defaultExpandSpreadThreshold)
	}
}

// ── noteExpansionSkipped: best-effort AC4 counter ───────────────────────────

func TestNoteExpansionSkipped_IncrementsCounter(t *testing.T) {
	b := gateTestBrain(t)

	// Fresh brain: counter row absent.
	if v, _ := b.store.GetMeta(metaExpansionSkips); v != "" {
		t.Fatalf("fresh brain has %s = %q, want absent", metaExpansionSkips, v)
	}

	noteExpansionSkipped(b.store)
	if v, _ := b.store.GetMeta(metaExpansionSkips); v != "1" {
		t.Errorf("after one skip, %s = %q, want \"1\"", metaExpansionSkips, v)
	}
	noteExpansionSkipped(b.store)
	if v, _ := b.store.GetMeta(metaExpansionSkips); v != "2" {
		t.Errorf("after two skips, %s = %q, want \"2\"", metaExpansionSkips, v)
	}
}
