package brain

import (
	"testing"

	"github.com/coetzeevs/cerebro/internal/store"
)

func sn(id string, score float64) store.ScoredNode {
	return store.ScoredNode{Node: store.Node{ID: id}, Score: score}
}

func ids(nodes []store.ScoredNode) []string {
	out := make([]string, len(nodes))
	for i := range nodes {
		out[i] = nodes[i].ID
	}
	return out
}

// TestFuseRecallRRF_EmptyKeywordIsIdentity (the AC4-NR floor contract, §5): an
// empty keyword set returns the vector set UNCHANGED (same nodes, same order).
func TestFuseRecallRRF_EmptyKeywordIsIdentity(t *testing.T) {
	vec := []store.ScoredNode{sn("a", 0.9), sn("b", 0.8), sn("c", 0.7)}
	out := fuseRecallRRF(vec, nil, defaultRRFK)

	got := ids(out)
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("identity broken: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("identity order broken at %d: got %v, want %v", i, got, want)
		}
	}
}

// TestFuseRecallRRF_EmptyVectorReturnsKeyword — symmetry: an empty vector set
// returns the keyword set (a node present in only one lane still contributes).
func TestFuseRecallRRF_EmptyVectorReturnsKeyword(t *testing.T) {
	kw := []store.ScoredNode{sn("x", 0), sn("y", 0)}
	out := fuseRecallRRF(nil, kw, defaultRRFK)
	if len(out) != 2 {
		t.Fatalf("expected 2 nodes from keyword-only fusion, got %d (%v)", len(out), ids(out))
	}
}

// TestFuseRecallRRF_KeywordOnlyNodeIsAdded — a node present ONLY in the keyword
// lane (the exact-identifier case vector missed) is added to the fused set.
func TestFuseRecallRRF_KeywordOnlyNodeIsAdded(t *testing.T) {
	vec := []store.ScoredNode{sn("a", 0.9), sn("b", 0.8)}
	kw := []store.ScoredNode{sn("z", 0)} // z is NOT in the vector lane
	out := fuseRecallRRF(vec, kw, defaultRRFK)

	set := map[string]bool{}
	for _, id := range ids(out) {
		set[id] = true
	}
	if !set["z"] {
		t.Fatalf("keyword-only node 'z' missing from fused set: %v", ids(out))
	}
	if !set["a"] || !set["b"] {
		t.Fatalf("vector nodes dropped from fused set: %v", ids(out))
	}
}

// TestFuseRecallRRF_DedupesOverlap — a node in BOTH lanes appears once, and its
// presence in both lanes lifts it (two reciprocal-rank terms).
func TestFuseRecallRRF_DedupesOverlap(t *testing.T) {
	// "shared" is rank-3 in vector but rank-1 in keyword. Two terms should lift it
	// above vector-rank-2 "b" (one term).
	vec := []store.ScoredNode{sn("a", 0.9), sn("b", 0.8), sn("shared", 0.7)}
	kw := []store.ScoredNode{sn("shared", 0), sn("q", 0)}
	out := fuseRecallRRF(vec, kw, defaultRRFK)

	// No duplicate "shared".
	count := 0
	for _, id := range ids(out) {
		if id == "shared" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected 'shared' exactly once, got %d (%v)", count, ids(out))
	}
}

// TestFuseRecallRRF_PreservesCompositeScore — fusion governs ordering only; the
// composite Score carried by each vector node is preserved.
func TestFuseRecallRRF_PreservesCompositeScore(t *testing.T) {
	vec := []store.ScoredNode{sn("a", 0.91), sn("b", 0.82)}
	kw := []store.ScoredNode{sn("a", 0)}
	out := fuseRecallRRF(vec, kw, defaultRRFK)
	for i := range out {
		if out[i].ID == "a" && out[i].Score != 0.91 {
			t.Errorf("composite Score for 'a' mutated by fusion: %v", out[i].Score)
		}
	}
}
