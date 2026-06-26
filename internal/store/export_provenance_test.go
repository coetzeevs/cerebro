package store

import (
	"bytes"
	"database/sql"
	"testing"
)

// TestExportImportRoundTripProvenanceRoot is the lockstep-emitter regression
// guard (agentic-lbjg AC1, mirrors the xtzn ExportSQL round-trip): a node seeded
// with provenance_root=1 must survive an Export -> Import into a fresh brain with
// provenance_root=1 intact. This fails if any of the three FULL-column node
// emitters in export.go (Import INSERT OR REPLACE / INSERT OR IGNORE) drops the
// new column on re-import.
func TestExportImportRoundTripProvenanceRoot(t *testing.T) {
	src := testStore(t)

	rootID, err := src.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "provenance root node", Importance: 0.8, ProvenanceRoot: true})
	if err != nil {
		t.Fatalf("AddNode (root): %v", err)
	}
	plainID, err := src.AddNode(&AddNodeOpts{Type: TypeEpisode, Content: "plain node", Importance: 0.5})
	if err != nil {
		t.Fatalf("AddNode (plain): %v", err)
	}

	// Sanity: source reads back the flag correctly via GetNode (full-column SELECT).
	rootNode, err := src.GetNode(rootID)
	if err != nil {
		t.Fatalf("GetNode (root): %v", err)
	}
	if !rootNode.ProvenanceRoot {
		t.Fatal("precondition: source root node should read provenance_root=true")
	}

	bundle, err := src.Export()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	// Import into a fresh brain.
	dst := testStore(t)
	if _, err := dst.Import(bundle, ImportOptions{OnConflict: ConflictReplace}); err != nil {
		t.Fatalf("Import: %v", err)
	}

	gotRoot, err := dst.GetNode(rootID)
	if err != nil {
		t.Fatalf("GetNode after import (root): %v", err)
	}
	if !gotRoot.ProvenanceRoot {
		t.Errorf("Export->Import dropped provenance_root: root node imported with provenance_root=false")
	}
	gotPlain, err := dst.GetNode(plainID)
	if err != nil {
		t.Fatalf("GetNode after import (plain): %v", err)
	}
	if gotPlain.ProvenanceRoot {
		t.Errorf("Export->Import corrupted provenance_root: plain node imported with provenance_root=true")
	}
}

// TestExportSQLRoundTripProvenanceRoot is the second lockstep guard: the
// `--format sql` text dump (ExportSQL) must emit provenance_root so a re-import
// of the SQL preserves it. This fails if the ExportSQL format-string node emitter
// drops the column.
func TestExportSQLRoundTripProvenanceRoot(t *testing.T) {
	src := testStore(t)

	rootID, err := src.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "sql dump root", Importance: 0.7, ProvenanceRoot: true})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}

	var buf bytes.Buffer
	if err := src.ExportSQL(&buf); err != nil {
		t.Fatalf("ExportSQL: %v", err)
	}

	// Build a fresh v5 brain, then replay the SQL dump into it.
	dst := testStore(t)
	if _, err := dst.db.Exec(buf.String()); err != nil {
		t.Fatalf("replaying SQL dump: %v", err)
	}

	var pr int
	if err := dst.db.QueryRow(`SELECT provenance_root FROM nodes WHERE id = ?`, rootID).Scan(&pr); err != nil {
		if err == sql.ErrNoRows {
			t.Fatal("ExportSQL dump did not insert the node at all")
		}
		t.Fatalf("reading provenance_root after SQL replay: %v", err)
	}
	if pr != 1 {
		t.Errorf("ExportSQL --format sql dropped provenance_root: got %d, want 1", pr)
	}
}

// TestExportSQLProvenanceColumnPresent asserts the ExportSQL emitter literally
// names provenance_root in its INSERT column list (defence against a silent
// column-list drift that would re-introduce the xtzn ExportSQL gap-class).
func TestExportSQLProvenanceColumnPresent(t *testing.T) {
	src := testStore(t)
	if _, err := src.AddNode(&AddNodeOpts{Type: TypeEpisode, Content: "x", Importance: 0.5}); err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	var buf bytes.Buffer
	if err := src.ExportSQL(&buf); err != nil {
		t.Fatalf("ExportSQL: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("provenance_root")) {
		t.Error("ExportSQL node INSERT omits provenance_root column")
	}
}
