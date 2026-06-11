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
