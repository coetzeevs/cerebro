package store

import (
	"bytes"
	"database/sql"
	"testing"
	"time"
)

// TestExportSQLRoundTripEdgeValidity guards the agentic-0p3w defect: the
// `--format sql` text dump must emit valid_at/invalid_at so a dump -> replay
// preserves every edge's bi-temporal window instead of silently re-opening it
// to NULL/NULL. Bounds must round-trip in the storage layout
// ("2006-01-02 15:04:05"), NOT RFC3339 — the as-of predicate compares raw
// strings, so a layout drift would silently defeat --as-of after reimport.
func TestExportSQLRoundTripEdgeValidity(t *testing.T) {
	src := testStore(t)

	a, err := src.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "edge src", Importance: 0.5})
	if err != nil {
		t.Fatalf("AddNode a: %v", err)
	}
	b, err := src.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "edge dst", Importance: 0.5})
	if err != nil {
		t.Fatalf("AddNode b: %v", err)
	}

	validAt := time.Date(2026, 6, 1, 9, 30, 0, 0, time.UTC)
	invalidAt := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	if _, err := src.AddEdge(a, b, "supports", AddEdgeOpts{ValidAt: &validAt, InvalidAt: &invalidAt}); err != nil {
		t.Fatalf("AddEdge windowed: %v", err)
	}
	// A second, unbounded edge must stay NULL/NULL after replay.
	if _, err := src.AddEdge(b, a, "relates_to", AddEdgeOpts{}); err != nil {
		t.Fatalf("AddEdge unbounded: %v", err)
	}

	var buf bytes.Buffer
	if err := src.ExportSQL(&buf); err != nil {
		t.Fatalf("ExportSQL: %v", err)
	}

	dst := testStore(t)
	if _, err := dst.db.Exec(buf.String()); err != nil {
		t.Fatalf("replaying SQL dump: %v", err)
	}

	// (1) The dump itself must carry the bounds in the storage layout — asserted
	// on the raw SQL text because a DATETIME-typed scan through the driver
	// re-stringifies as RFC3339 and would mask a layout drift.
	wantVA := validAt.Format(storageTimeLayout)
	wantIA := invalidAt.Format(storageTimeLayout)
	if !bytes.Contains(buf.Bytes(), []byte("'"+wantVA+"'")) {
		t.Errorf("SQL dump does not contain valid_at literal %q in storage layout", wantVA)
	}
	if !bytes.Contains(buf.Bytes(), []byte("'"+wantIA+"'")) {
		t.Errorf("SQL dump does not contain invalid_at literal %q in storage layout", wantIA)
	}

	// (2) Semantics: the replayed window must match the original instants.
	var va, ia sql.NullTime
	if err := dst.db.QueryRow(
		`SELECT valid_at, invalid_at FROM edges WHERE source_id = ? AND target_id = ? AND relation = 'supports'`,
		a, b,
	).Scan(&va, &ia); err != nil {
		if err == sql.ErrNoRows {
			t.Fatal("ExportSQL dump did not insert the windowed edge at all")
		}
		t.Fatalf("reading window after SQL replay: %v", err)
	}
	if !va.Valid || !va.Time.UTC().Equal(validAt) {
		t.Errorf("valid_at lost or drifted on SQL round-trip: got %+v, want %v", va, validAt)
	}
	if !ia.Valid || !ia.Time.UTC().Equal(invalidAt) {
		t.Errorf("invalid_at lost or drifted on SQL round-trip: got %+v, want %v", ia, invalidAt)
	}

	// (3) The as-of predicate must actually see the replayed window: an as-of
	// inside [validAt, invalidAt) matches; outside it does not.
	inside := validAt.Add(24 * time.Hour)
	outside := invalidAt.Add(24 * time.Hour)
	edgesInside, err := dst.GetEdgesBatch([]string{a}, &inside)
	if err != nil {
		t.Fatalf("GetEdgesBatch inside window: %v", err)
	}
	if len(edgesInside[a]) == 0 {
		t.Error("as-of inside the replayed window returned no edges — window semantics lost")
	}
	edgesOutside, err := dst.GetEdgesBatch([]string{a}, &outside)
	if err != nil {
		t.Fatalf("GetEdgesBatch outside window: %v", err)
	}
	for _, e := range edgesOutside[a] {
		if e.Relation == "supports" {
			t.Error("as-of after invalid_at still matched the windowed edge — predicate defeated (layout drift?)")
		}
	}

	// (4) The unbounded edge stays NULL/NULL.
	var va2, ia2 sql.NullTime
	if err := dst.db.QueryRow(
		`SELECT valid_at, invalid_at FROM edges WHERE source_id = ? AND target_id = ? AND relation = 'relates_to'`,
		b, a,
	).Scan(&va2, &ia2); err != nil {
		t.Fatalf("reading unbounded edge after SQL replay: %v", err)
	}
	if va2.Valid || ia2.Valid {
		t.Errorf("unbounded edge gained spurious bounds on round-trip: valid_at=%+v invalid_at=%+v", va2, ia2)
	}
}
