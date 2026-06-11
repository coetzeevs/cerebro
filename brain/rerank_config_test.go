package brain

import (
	"context"
	"errors"
	"testing"

	"github.com/coetzeevs/cerebro/internal/rerank"
	"github.com/coetzeevs/cerebro/internal/store"
)

// M4: brain-side resolver owns the "config." prefix and the ""→false default.
func TestResolveRerankEnabled(t *testing.T) {
	b := testBrain(t)

	// Fresh brain: no override → disabled.
	if resolveRerankEnabled(b.Store()) {
		t.Error("fresh brain: expected rerank disabled, got enabled")
	}

	// "false" → disabled.
	_ = b.Store().SetMeta("config.rerank_enabled", "false")
	if resolveRerankEnabled(b.Store()) {
		t.Error("config.rerank_enabled=false: expected disabled")
	}

	// "true" → enabled.
	_ = b.Store().SetMeta("config.rerank_enabled", "true")
	if !resolveRerankEnabled(b.Store()) {
		t.Error("config.rerank_enabled=true: expected enabled")
	}

	// Any non-"true" value → disabled (defensive).
	_ = b.Store().SetMeta("config.rerank_enabled", "garbage")
	if resolveRerankEnabled(b.Store()) {
		t.Error("config.rerank_enabled=garbage: expected disabled")
	}
}

// M4 / S-INFO-3: rerank_command resolves config-key first, then env fallback.
func TestResolveRerankCommand_ConfigWinsOverEnv(t *testing.T) {
	b := testBrain(t)
	t.Setenv("CEREBRO_RERANK_COMMAND", "env-cmd")

	// No config row → env fallback.
	if got := resolveRerankCommand(b.Store()); got != "env-cmd" {
		t.Errorf("no config: got %q, want env fallback 'env-cmd'", got)
	}

	// Config set → config wins over env (no env override of a safer config).
	_ = b.Store().SetMeta("config.rerank_command", "config-cmd")
	if got := resolveRerankCommand(b.Store()); got != "config-cmd" {
		t.Errorf("config set: got %q, want 'config-cmd'", got)
	}
}

func TestResolveRerankCommand_EmptyWhenUnset(t *testing.T) {
	b := testBrain(t)
	// Ensure the env is clear for this test.
	t.Setenv("CEREBRO_RERANK_COMMAND", "")
	if got := resolveRerankCommand(b.Store()); got != "" {
		t.Errorf("unset command: got %q, want empty", got)
	}
}

// AC3a/M1: reorderByRerankScore reorders candidates by descending rerank score.
func TestReorderByRerankScore(t *testing.T) {
	nodes := []store.ScoredNode{
		{Node: store.Node{ID: "a"}, Score: 0.9},
		{Node: store.Node{ID: "b"}, Score: 0.8},
		{Node: store.Node{ID: "c"}, Score: 0.7},
	}
	// Rerank scores invert the order: c > b > a.
	scores := []float64{0.1, 0.5, 0.9}

	out := reorderByRerankScore(nodes, scores)
	gotOrder := []string{out[0].ID, out[1].ID, out[2].ID}
	want := []string{"c", "b", "a"}
	for i := range want {
		if gotOrder[i] != want[i] {
			t.Errorf("reorder position %d = %q, want %q (full: %v)", i, gotOrder[i], want[i], gotOrder)
		}
	}
	// Composite Score must be preserved (rerank governs ordering only).
	for i := range out {
		switch out[i].ID {
		case "a":
			if out[i].Score != 0.9 {
				t.Errorf("node a composite score mutated: %v", out[i].Score)
			}
		case "c":
			if out[i].Score != 0.7 {
				t.Errorf("node c composite score mutated: %v", out[i].Score)
			}
		}
	}
}

// AC4-NR floor protection: a length mismatch must not reorder (caller degrades).
func TestReorderByRerankScore_LengthMismatchIsNoop(t *testing.T) {
	nodes := []store.ScoredNode{
		{Node: store.Node{ID: "a"}, Score: 0.9},
		{Node: store.Node{ID: "b"}, Score: 0.8},
	}
	out := reorderByRerankScore(nodes, []float64{0.1}) // wrong length
	if out[0].ID != "a" || out[1].ID != "b" {
		t.Errorf("length mismatch should be a no-op, got %v", []string{out[0].ID, out[1].ID})
	}
}

// ── Reciprocal Rank Fusion (RRF) — the recall@10-dip fix ───────────────────

// THE LOAD-BEARING TEST (recall@10 dip investigation): pure-reorder lets the
// cross-encoder demote a composite-strong item below the cut even while it lifts
// the single best item (MRR up, recall@10 down). RRF fuses the composite rank
// AND the reranker rank, so a composite-top item the reranker buries still
// survives in the fused top-K. This is the displaced-node recovery, proven with
// synthetic rankings (no Ollama, no subprocess).
//
// Setup: 5 candidates in composite order [a,b,c,d,e]. The reranker LIFTS "e"
// (the single best, composite-rank 5) to reranker-rank 1, and BURIES "a"
// (composite-rank 1, a recall@10-relevant item) to reranker-rank 5.
//   - pure-reorder top-3 = [e, d, c]  → "a" displaced out of the top-3.
//   - RRF (k=60) top-3   = must still contain "a", because its composite rank 1
//     contributes 1/(60+1)=0.01639, the strongest single-list term available.
func TestFuseRRF_PreservesCompositeTopItemRerankerDemotes(t *testing.T) {
	nodes := []store.ScoredNode{
		{Node: store.Node{ID: "a"}, Score: 0.90}, // composite rank 1
		{Node: store.Node{ID: "b"}, Score: 0.80}, // composite rank 2
		{Node: store.Node{ID: "c"}, Score: 0.70}, // composite rank 3
		{Node: store.Node{ID: "d"}, Score: 0.60}, // composite rank 4
		{Node: store.Node{ID: "e"}, Score: 0.50}, // composite rank 5
	}
	// Reranker buries a (rank 1 → score lowest) and lifts e (rank 5 → highest).
	// Reranker order by descending score: e > d > c > b > a.
	scores := []float64{0.01, 0.20, 0.40, 0.60, 0.99}

	// Confirm the premise: pure-reorder displaces "a" out of the top-3.
	reorder := reorderByRerankScore(nodes, scores)
	reorderTop3 := map[string]bool{reorder[0].ID: true, reorder[1].ID: true, reorder[2].ID: true}
	if reorderTop3["a"] {
		t.Fatalf("premise broken: pure-reorder should have displaced 'a' from top-3, got %v",
			[]string{reorder[0].ID, reorder[1].ID, reorder[2].ID})
	}

	fused := fuseRRF(nodes, scores, defaultRRFK)
	if len(fused) != len(nodes) {
		t.Fatalf("fuseRRF should return all %d nodes, got %d", len(nodes), len(fused))
	}
	fusedTop3 := map[string]bool{fused[0].ID: true, fused[1].ID: true, fused[2].ID: true}

	// The recovery: "a" (composite-top, reranker-demoted) survives in the fused top-3.
	if !fusedTop3["a"] {
		t.Errorf("RRF should preserve composite-top item 'a' in the top-3, got %v",
			[]string{fused[0].ID, fused[1].ID, fused[2].ID})
	}
	// The MRR win is preserved: "e" (reranker-best) is also lifted by RRF (its
	// reranker rank 1 contributes 1/(60+1)); it should be at or near the top.
	if fused[0].ID != "a" && fused[0].ID != "e" {
		t.Errorf("RRF top item should be a composite-or-reranker leader (a or e), got %q", fused[0].ID)
	}
}

// fuseRRF must produce the exact Reciprocal Rank Fusion score ordering.
// Hand-computed for k=1 so the arithmetic is checkable:
// composite order [x,y,z] (ranks 1,2,3); reranker scores invert to [z,y,x].
//
//	x: 1/(1+1) + 1/(1+3) = 0.500 + 0.250 = 0.750
//	y: 1/(1+2) + 1/(1+2) = 0.333 + 0.333 = 0.667
//	z: 1/(1+3) + 1/(1+1) = 0.250 + 0.500 = 0.750
//
// x and z tie at 0.750; the stable sort keeps the composite-order winner (x)
// ahead of z. y (0.667) is last. Expected fused order: [x, z, y].
func TestFuseRRF_ExactScoreOrdering(t *testing.T) {
	nodes := []store.ScoredNode{
		{Node: store.Node{ID: "x"}, Score: 0.9},
		{Node: store.Node{ID: "y"}, Score: 0.8},
		{Node: store.Node{ID: "z"}, Score: 0.7},
	}
	scores := []float64{0.1, 0.5, 0.9} // reranker order: z > y > x

	out := fuseRRF(nodes, scores, 1)
	got := []string{out[0].ID, out[1].ID, out[2].ID}
	want := []string{"x", "z", "y"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("RRF(k=1) position %d = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
	// Composite Score must be preserved (fusion governs ordering only).
	for i := range out {
		if out[i].ID == "x" && out[i].Score != 0.9 {
			t.Errorf("node x composite score mutated: %v", out[i].Score)
		}
		if out[i].ID == "z" && out[i].Score != 0.7 {
			t.Errorf("node z composite score mutated: %v", out[i].Score)
		}
	}
}

// AC4-NR floor: a length mismatch must be a no-op (caller degrades to composite).
func TestFuseRRF_LengthMismatchIsNoop(t *testing.T) {
	nodes := []store.ScoredNode{
		{Node: store.Node{ID: "a"}, Score: 0.9},
		{Node: store.Node{ID: "b"}, Score: 0.8},
	}
	out := fuseRRF(nodes, []float64{0.1}, defaultRRFK) // wrong length
	if out[0].ID != "a" || out[1].ID != "b" {
		t.Errorf("length mismatch should be a no-op, got %v", []string{out[0].ID, out[1].ID})
	}
}

// A non-positive k is clamped to the default so the reciprocal never divides by
// a zero/negative denominator (defensive; k≥1 per the RRF definition).
func TestFuseRRF_NonPositiveKClampsToDefault(t *testing.T) {
	nodes := []store.ScoredNode{
		{Node: store.Node{ID: "a"}, Score: 0.9},
		{Node: store.Node{ID: "b"}, Score: 0.8},
	}
	scores := []float64{0.1, 0.9} // reranker order: b > a
	// k=0 must behave like k=defaultRRFK, not panic or divide by zero.
	out := fuseRRF(nodes, scores, 0)
	if len(out) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(out))
	}
}

// applyRerank in "rrf" mode must fuse, not pure-reorder. With the load-bearing
// scenario, the composite-top item the reranker demotes survives the cut.
func TestApplyRerank_RRFMode_PreservesDisplacedNode(t *testing.T) {
	nodes := []store.ScoredNode{
		{Node: store.Node{ID: "a"}, Score: 0.90},
		{Node: store.Node{ID: "b"}, Score: 0.80},
		{Node: store.Node{ID: "c"}, Score: 0.70},
		{Node: store.Node{ID: "d"}, Score: 0.60},
		{Node: store.Node{ID: "e"}, Score: 0.50},
	}
	// invertingReranker buries composite-rank-1 'a' and lifts composite-rank-5 'e'.
	out := applyRerankWithFusion(context.Background(), invertingReranker{}, "q", nodes, 3, fusionModeRRF)
	if len(out) != 3 {
		t.Fatalf("expected cut to 3, got %d", len(out))
	}
	top3 := map[string]bool{out[0].ID: true, out[1].ID: true, out[2].ID: true}
	if !top3["a"] {
		t.Errorf("rrf mode should keep displaced composite-top 'a' in top-3, got %v",
			[]string{out[0].ID, out[1].ID, out[2].ID})
	}
}

// applyRerank in "reorder" mode keeps the legacy pure-reorder behaviour: the
// composite-top item the reranker demotes IS displaced (the documented dip).
func TestApplyRerank_ReorderMode_LegacyPureReorder(t *testing.T) {
	nodes := []store.ScoredNode{
		{Node: store.Node{ID: "a"}, Score: 0.90},
		{Node: store.Node{ID: "b"}, Score: 0.80},
		{Node: store.Node{ID: "c"}, Score: 0.70},
		{Node: store.Node{ID: "d"}, Score: 0.60},
		{Node: store.Node{ID: "e"}, Score: 0.50},
	}
	out := applyRerankWithFusion(context.Background(), invertingReranker{}, "q", nodes, 3, fusionModeReorder)
	if len(out) != 3 {
		t.Fatalf("expected cut to 3, got %d", len(out))
	}
	top3 := map[string]bool{out[0].ID: true, out[1].ID: true, out[2].ID: true}
	if top3["a"] {
		t.Errorf("reorder mode (legacy) should displace 'a' from top-3, got %v",
			[]string{out[0].ID, out[1].ID, out[2].ID})
	}
}

// resolveRerankFusion: default is RRF; explicit "reorder" selects the legacy
// mode; any unknown value falls back to the RRF default (defensive).
func TestResolveRerankFusion(t *testing.T) {
	b := testBrain(t)

	if got := resolveRerankFusion(b.Store()); got != fusionModeRRF {
		t.Errorf("fresh brain: fusion mode = %q, want %q (rrf default)", got, fusionModeRRF)
	}

	_ = b.Store().SetMeta("config.rerank_fusion", "reorder")
	if got := resolveRerankFusion(b.Store()); got != fusionModeReorder {
		t.Errorf("config=reorder: fusion mode = %q, want %q", got, fusionModeReorder)
	}

	_ = b.Store().SetMeta("config.rerank_fusion", "rrf")
	if got := resolveRerankFusion(b.Store()); got != fusionModeRRF {
		t.Errorf("config=rrf: fusion mode = %q, want %q", got, fusionModeRRF)
	}

	_ = b.Store().SetMeta("config.rerank_fusion", "garbage")
	if got := resolveRerankFusion(b.Store()); got != fusionModeRRF {
		t.Errorf("config=garbage: fusion mode = %q, want %q (defensive default)", got, fusionModeRRF)
	}
}

// applyRerank is the composition seam (over-retrieve → rerank → cut). With a
// noop reranker the input order is preserved and the cut is applied.
func TestApplyRerank_NoopPreservesOrderAndCuts(t *testing.T) {
	nodes := []store.ScoredNode{
		{Node: store.Node{ID: "a"}, Score: 0.9},
		{Node: store.Node{ID: "b"}, Score: 0.8},
		{Node: store.Node{ID: "c"}, Score: 0.7},
	}
	out := applyRerank(context.Background(), noopReranker{}, "q", nodes, 2)
	if len(out) != 2 {
		t.Fatalf("expected cut to 2, got %d", len(out))
	}
	if out[0].ID != "a" || out[1].ID != "b" {
		t.Errorf("noop should preserve order, got %v", []string{out[0].ID, out[1].ID})
	}
}

// applyRerank degrades to the input order on reranker error (AC4-NR floor).
func TestApplyRerank_ErrorDegradesToInputOrder(t *testing.T) {
	nodes := []store.ScoredNode{
		{Node: store.Node{ID: "a"}, Score: 0.9},
		{Node: store.Node{ID: "b"}, Score: 0.8},
	}
	out := applyRerank(context.Background(), errReranker{}, "q", nodes, 10)
	if len(out) != 2 || out[0].ID != "a" || out[1].ID != "b" {
		t.Errorf("error should degrade to input order, got %v", out)
	}
}

func TestApplyRerank_EmptyInput(t *testing.T) {
	out := applyRerank(context.Background(), noopReranker{}, "q", nil, 10)
	if len(out) != 0 {
		t.Errorf("expected empty output, got %d", len(out))
	}
}

// A nil reranker (rerank enabled but no command configured) must degrade to
// composite order without panicking and still apply the cut (AC4-NR floor).
func TestApplyRerank_NilRerankerDegrades(t *testing.T) {
	nodes := []store.ScoredNode{
		{Node: store.Node{ID: "a"}, Score: 0.9},
		{Node: store.Node{ID: "b"}, Score: 0.8},
		{Node: store.Node{ID: "c"}, Score: 0.7},
	}
	out := applyRerank(context.Background(), nil, "q", nodes, 2)
	if len(out) != 2 || out[0].ID != "a" || out[1].ID != "b" {
		t.Errorf("nil reranker should degrade to composite order + cut, got %v", out)
	}
}

// --- test rerankers ---

type noopReranker struct{}

func (noopReranker) Rerank(_ context.Context, _ string, cands []rerank.Candidate) ([]float64, error) {
	scores := make([]float64, len(cands))
	for i := range cands {
		scores[i] = float64(len(cands) - i)
	}
	return scores, nil
}
func (noopReranker) Name() string { return "noop" }

type errReranker struct{}

func (errReranker) Rerank(_ context.Context, _ string, _ []rerank.Candidate) ([]float64, error) {
	return nil, errors.New("boom")
}
func (errReranker) Name() string { return "err" }

// invertingReranker scores candidates in reverse composite order: the first
// (composite-best) candidate gets the lowest score and the last gets the
// highest. This is the worst case for pure-reorder — it buries the composite
// leader — used to prove RRF recovers it.
type invertingReranker struct{}

func (invertingReranker) Rerank(_ context.Context, _ string, cands []rerank.Candidate) ([]float64, error) {
	scores := make([]float64, len(cands))
	for i := range cands {
		scores[i] = float64(i) // index 0 → 0.0 (lowest), last → highest
	}
	return scores, nil
}
func (invertingReranker) Name() string { return "inverting" }
