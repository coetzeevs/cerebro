package brain

// search_asof_gate_test.go — TL-PI-N2: the --as-of edge-validity filter
// interacts with the lazy-expansion gate (agentic-73l6). When the gate FIRES,
// ExpandGraph is skipped entirely, so --as-of is a no-op (no edges traversed →
// nothing to filter). When the gate does NOT fire, the --as-of predicate
// applies at the edge-fetch SQL. Both paths are tested here with asOf set.

import (
	"context"
	"testing"
	"time"

	"github.com/coetzeevs/cerebro/internal/store"
)

// addWindowedNeighbor adds an edge-only node (no embedding — reachable ONLY via
// ExpandGraph) connected to anchorID by an edge with the given validity window.
// Its presence in Search output proves both that ExpandGraph ran AND that the
// edge was valid at the queried as-of instant.
func addWindowedNeighbor(t *testing.T, b *Brain, anchorID string, opts store.AddEdgeOpts) string {
	t.Helper()
	nid, err := b.store.AddNode(&store.AddNodeOpts{
		Type:       store.TypeConcept,
		Content:    "windowed edge-only neighbor",
		Importance: 0.5,
	})
	if err != nil {
		t.Fatalf("AddNode neighbor: %v", err)
	}
	if _, err := b.store.AddEdge(anchorID, nid, "relates_to", opts); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}
	return nid
}

func tptr(tm time.Time) *time.Time { return &tm }

// TestSearch_AsOfNoOpWhenGateFires (TL-PI-N2, gate-fires path): with a
// gate-triggering top-1 similarity, ExpandGraph is skipped, so the windowed
// edge-only neighbor never appears REGARDLESS of the as-of instant — even an
// as-of squarely inside the edge's window. The filter is a no-op here because
// no edges are traversed at all.
func TestSearch_AsOfNoOpWhenGateFires(t *testing.T) {
	// sims ≈ {1.0, 0.995, 0.981} — top-1 strictly exceeds 0.9 → gate fires.
	b, ids := gateBrainSeeded(t, []float64{0.0, 0.1, 0.2}, 8)
	// Edge valid [2026-01-01, 2026-06-01).
	neighbor := addWindowedNeighbor(t, b, ids[0], store.AddEdgeOpts{
		ValidAt:   tptr(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		InvalidAt: tptr(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)),
	})
	_ = b.store.SetMeta("config.expand_threshold", "0.9")
	_ = b.store.SetMeta("config.expand_spread_threshold", "0.0")

	// as-of INSIDE the window — would include the edge IF expansion ran.
	asOf := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	before := skipCounter(t, b)
	got, err := b.Search(context.Background(), "qqqqq", 10, 0.0, nil, &asOf)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if gateContainsID(got, neighbor) {
		t.Errorf("gate fired but neighbor %s surfaced — ExpandGraph ran; --as-of is supposed to be a no-op on the gated path", neighbor)
	}
	if d := skipCounter(t, b) - before; d != 1 {
		t.Errorf("expected exactly 1 skip event on the gated path, got delta %d", d)
	}
}

// TestSearch_AsOfFiltersWhenGateDoesNotFire (TL-PI-N2, gate-does-not-fire path):
// with a below-threshold top-1 similarity, ExpandGraph runs and the --as-of
// predicate applies. An as-of inside the edge's window surfaces the neighbor; an
// as-of outside it filters the neighbor out (the edge is not traversed).
func TestSearch_AsOfFiltersWhenGateDoesNotFire(t *testing.T) {
	// sims ≈ {0.894, 0.876, 0.857} — top-1 below 0.99 → gate does NOT fire.
	b, ids := gateBrainSeeded(t, []float64{0.50, 0.55, 0.60}, 8)
	neighbor := addWindowedNeighbor(t, b, ids[0], store.AddEdgeOpts{
		ValidAt:   tptr(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		InvalidAt: tptr(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)),
	})
	_ = b.store.SetMeta("config.expand_threshold", "0.99")
	_ = b.store.SetMeta("config.expand_spread_threshold", "0.0")

	// as-of INSIDE [2026-01-01, 2026-02-01): expansion runs AND the edge is
	// valid → neighbor present.
	inside := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	before := skipCounter(t, b)
	got, err := b.Search(context.Background(), "qqqqq", 10, 0.0, nil, &inside)
	if err != nil {
		t.Fatalf("Search (inside): %v", err)
	}
	if !gateContainsID(got, neighbor) {
		t.Errorf("gate did not fire and as-of is inside the window, but neighbor %s missing — expansion+filter did not surface a valid edge", neighbor)
	}
	if d := skipCounter(t, b) - before; d != 0 {
		t.Errorf("expected 0 skip events on the non-gated path, got delta %d", d)
	}

	// as-of OUTSIDE the window (after invalid_at): expansion runs but the edge is
	// filtered out at the SQL → neighbor absent. This is the filter-not-defeated
	// proof on the active expansion path.
	outside := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	got2, err := b.Search(context.Background(), "qqqqq", 10, 0.0, nil, &outside)
	if err != nil {
		t.Fatalf("Search (outside): %v", err)
	}
	if gateContainsID(got2, neighbor) {
		t.Errorf("as-of outside the window surfaced neighbor %s — the validity filter was defeated on the active expansion path", neighbor)
	}
}
