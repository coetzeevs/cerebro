package store

import (
	"bytes"
	"testing"
)

// ---- goc7: origin identity ----

// TestFreshInitHasOriginAtV6: a freshly-Init'ed brain is schema v6 with the
// four origin columns, the origin convention boundary stamped at birth, and
// the relation registry seeded with the built-ins.
func TestFreshInitHasOriginAtV6(t *testing.T) {
	s := testStore(t)

	v, err := s.GetMeta("schema_version")
	if err != nil || v != "7" {
		t.Fatalf("schema_version: got %q err=%v, want \"7\"", v, err)
	}
	cols := nodeColumns(t, s)
	for _, c := range []string{"origin_actor", "origin_channel", "origin_session", "origin_host"} {
		if !cols[c] {
			t.Errorf("fresh init missing nodes.%s", c)
		}
	}
	if b, err := s.OriginBoundary(); err != nil || b == nil {
		t.Errorf("origin boundary not stamped at birth: %v err=%v", b, err)
	}
	rels, err := s.ListRelations()
	if err != nil {
		t.Fatalf("ListRelations: %v", err)
	}
	seeded := map[string]bool{}
	for _, r := range rels {
		seeded[r.Name] = true
	}
	for _, want := range []string{"derived_from", "supports", "contradicts", "supersedes"} {
		if !seeded[want] {
			t.Errorf("built-in relation %q not seeded", want)
		}
	}
}

// TestAddNodeStampsOrigin: origin identity supplied at write time persists;
// absent fields stay NULL (never fabricated).
func TestAddNodeStampsOrigin(t *testing.T) {
	s := testStore(t)
	id, err := s.AddNode(&AddNodeOpts{
		Type: TypeEpisode, Content: "origin-stamped", Importance: 0.5,
		OriginActor: "agent", OriginChannel: "cli", OriginSession: "sess-123", OriginHost: "machine-a",
	})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	n, err := s.GetNode(id)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if n.OriginActor != "agent" || n.OriginChannel != "cli" || n.OriginSession != "sess-123" || n.OriginHost != "machine-a" {
		t.Errorf("origin not persisted: %+v", n)
	}

	bare, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "no origin", Importance: 0.5})
	if err != nil {
		t.Fatalf("AddNode bare: %v", err)
	}
	nb, _ := s.GetNode(bare)
	if nb.OriginActor != "" || nb.OriginHost != "" {
		t.Errorf("bare add fabricated origin: %+v", nb)
	}
}

// TestSupersedeCarriesNewWriterOrigin: the replacement node carries the
// SUPERSEDER's identity, not the original author's.
func TestSupersedeCarriesNewWriterOrigin(t *testing.T) {
	s := testStore(t)
	old, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "v1", Importance: 0.5, OriginActor: "human", OriginHost: "machine-a"})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	repl, err := s.SupersedeNode(old, &AddNodeOpts{Type: TypeConcept, Content: "v2", Importance: 0.5, OriginActor: "agent", OriginHost: "machine-b"})
	if err != nil {
		t.Fatalf("SupersedeNode: %v", err)
	}
	n, _ := s.GetNode(repl)
	if n.OriginActor != "agent" || n.OriginHost != "machine-b" {
		t.Errorf("replacement origin wrong: %+v", n)
	}
}

// TestOriginStatusClassification: recorded (actor present), legacy (absent,
// created before the boundary), unknown (absent, created at/after it).
func TestOriginStatusClassification(t *testing.T) {
	s := testStore(t)
	rec, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "rec", Importance: 0.5, OriginActor: "human"})
	unk, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "unk", Importance: 0.5})
	leg, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "leg", Importance: 0.5})
	// Backdate the legacy node before the boundary.
	if _, err := s.db.Exec(`UPDATE nodes SET created_at = datetime('now', '-10 days') WHERE id = ?`, leg); err != nil {
		t.Fatalf("backdating: %v", err)
	}

	boundary, err := s.OriginBoundary()
	if err != nil || boundary == nil {
		t.Fatalf("OriginBoundary: %v err=%v", boundary, err)
	}
	check := func(id, want string) {
		n, _ := s.GetNode(id)
		if got := OriginStatusFor(n, boundary); got != want {
			t.Errorf("origin status for %s: got %q want %q", n.Content, got, want)
		}
	}
	check(rec, "recorded")
	check(unk, "unknown")
	check(leg, "legacy")
}

// TestExportImportRoundTripOrigin (JSON bundle): origin survives; a bundle
// node WITHOUT origin gets stamped actor/channel "import" so provenance of
// entry is never silently blank going forward.
func TestExportImportRoundTripOrigin(t *testing.T) {
	src := testStore(t)
	withID, _ := src.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "carries origin", Importance: 0.5, OriginActor: "agent", OriginChannel: "hook", OriginHost: "machine-a"})
	bareID, _ := src.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "no origin in bundle", Importance: 0.5})

	bundle, err := src.Export()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	dst := testStore(t)
	if _, err := dst.Import(bundle, ImportOptions{OnConflict: ConflictSkip}); err != nil {
		t.Fatalf("Import: %v", err)
	}

	n, err := dst.GetNode(withID)
	if err != nil {
		t.Fatalf("GetNode imported: %v", err)
	}
	if n.OriginActor != "agent" || n.OriginChannel != "hook" || n.OriginHost != "machine-a" {
		t.Errorf("origin lost on JSON round-trip: %+v", n)
	}
	nb, err := dst.GetNode(bareID)
	if err != nil {
		t.Fatalf("GetNode bare: %v", err)
	}
	if nb.OriginActor != "import" || nb.OriginChannel != "import" {
		t.Errorf("origin-less import not stamped as import: actor=%q channel=%q", nb.OriginActor, nb.OriginChannel)
	}
}

// TestExportSQLRoundTripOrigin: the SQL text dump carries origin columns too
// (the 0p3w lockstep lesson — emitters widen together or drift silently).
func TestExportSQLRoundTripOrigin(t *testing.T) {
	src := testStore(t)
	id, _ := src.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "sql origin", Importance: 0.5, OriginActor: "subagent", OriginHost: "machine-c"})

	var buf bytes.Buffer
	if err := src.ExportSQL(&buf); err != nil {
		t.Fatalf("ExportSQL: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("origin_actor")) {
		t.Fatal("SQL dump omits origin columns")
	}
	dst := testStore(t)
	if _, err := dst.db.Exec(buf.String()); err != nil {
		t.Fatalf("replaying dump: %v", err)
	}
	n, err := dst.GetNode(id)
	if err != nil {
		t.Fatalf("GetNode after replay: %v", err)
	}
	if n.OriginActor != "subagent" || n.OriginHost != "machine-c" {
		t.Errorf("origin lost on SQL round-trip: %+v", n)
	}
}

// ---- 8l2g: typed-relation registry ----

func TestRelationRegistryCRUD(t *testing.T) {
	s := testStore(t)

	if ok, err := s.RelationRegistered("derived_from"); err != nil || !ok {
		t.Errorf("built-in derived_from should be registered: ok=%v err=%v", ok, err)
	}
	if ok, _ := s.RelationRegistered("my_custom_rel"); ok {
		t.Error("unregistered relation reported as registered")
	}
	if err := s.RegisterRelation("my_custom_rel", "topical"); err != nil {
		t.Fatalf("RegisterRelation: %v", err)
	}
	if ok, _ := s.RelationRegistered("my_custom_rel"); !ok {
		t.Error("registered relation not found")
	}
	rels, _ := s.ListRelations()
	found := false
	for _, r := range rels {
		if r.Name == "my_custom_rel" && r.TraversalClass == "topical" {
			found = true
		}
	}
	if !found {
		t.Error("ListRelations missing my_custom_rel/topical")
	}
	if err := s.RemoveRelation("my_custom_rel"); err != nil {
		t.Fatalf("RemoveRelation: %v", err)
	}
	if ok, _ := s.RelationRegistered("my_custom_rel"); ok {
		t.Error("removed relation still registered")
	}
	// Registering an existing relation is idempotent, not an error.
	if err := s.RegisterRelation("supports", ""); err != nil {
		t.Errorf("re-registering built-in should be idempotent: %v", err)
	}
}

// ---- v5 -> v6 upgrade path ----

func TestMigrationFromV5AddsOriginAndRelations(t *testing.T) {
	path := t.TempDir() + "/v5.sqlite"
	s5, err := buildV5Database(t, path)
	if err != nil {
		t.Fatalf("building v5 store: %v", err)
	}
	// Insert with raw v5-shaped SQL: the store's AddNode writer is v6-shaped
	// (it names the origin columns), which a genuine v5 table cannot accept.
	oldID := "11111111-2222-3333-4444-555555555555"
	// created_at is backdated: a real pre-migration node always predates the
	// migration instant, and the boundary compare is second-granular.
	if _, err := s5.db.Exec(
		`INSERT INTO nodes (id, type, content, importance, decay_rate, created_at) VALUES (?, 'concept', 'pre-origin memory', 0.5, 0.005, datetime('now', '-1 hour'))`,
		oldID,
	); err != nil {
		t.Fatalf("inserting v5 node: %v", err)
	}
	_ = s5.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open (migrating): %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	if v, _ := s.GetMeta("schema_version"); v != "7" {
		t.Fatalf("migrated version: got %q want 7", v)
	}
	cols := nodeColumns(t, s)
	for _, c := range []string{"origin_actor", "origin_channel", "origin_session", "origin_host"} {
		if !cols[c] {
			t.Errorf("migration missing nodes.%s", c)
		}
	}
	boundary, err := s.OriginBoundary()
	if err != nil || boundary == nil {
		t.Fatalf("boundary not stamped by migration: %v err=%v", boundary, err)
	}
	n, err := s.GetNode(oldID)
	if err != nil {
		t.Fatalf("GetNode pre-existing: %v", err)
	}
	if got := OriginStatusFor(n, boundary); got != "legacy" {
		t.Errorf("pre-migration node should classify legacy, got %q", got)
	}
	if ok, err := s.RelationRegistered("derived_from"); err != nil || !ok {
		t.Errorf("migration did not seed relations: ok=%v err=%v", ok, err)
	}
}

// buildV5Database constructs a store frozen at schema v5 (origin-less), the
// exact shape a real brain has after the v3.x migrations.
func buildV5Database(t *testing.T, path string) (*Store, error) {
	t.Helper()
	s, err := open(path)
	if err != nil {
		return nil, err
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS schema_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS nodes (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL CHECK (type IN ('episode', 'concept', 'procedure', 'reflection')),
			subtype TEXT,
			content TEXT NOT NULL,
			metadata JSON,
			importance REAL DEFAULT 0.5,
			decay_rate REAL NOT NULL,
			access_count INTEGER DEFAULT 0,
			times_reinforced INTEGER DEFAULT 0,
			status TEXT DEFAULT 'active',
			embedding_model TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_accessed DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_reinforced DATETIME,
			updated_at DATETIME,
			last_surfaced DATETIME,
			provenance_root INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS edges (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source_id TEXT NOT NULL,
			target_id TEXT NOT NULL,
			relation TEXT NOT NULL,
			weight REAL DEFAULT 1.0,
			metadata JSON,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			valid_at DATETIME,
			invalid_at DATETIME,
			UNIQUE (source_id, target_id, relation)
		)`,
		`INSERT OR IGNORE INTO schema_meta (key, value) VALUES ('schema_version', '5')`,
		`INSERT OR IGNORE INTO schema_meta (key, value) VALUES ('` + MetaProvenanceConventionSince + `', datetime('now', '-30 days'))`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			_ = s.Close()
			return nil, err
		}
	}
	// Backdate created_at on subsequent adds is unnecessary: the origin
	// boundary is stamped at MIGRATION time, after this node exists.
	return s, nil
}

// ---- goc7 AC: recall surfaces origin — both search lanes carry the fields ----

func TestVectorSearchCarriesOrigin(t *testing.T) {
	s := testStoreWithVec(t, 4)
	id, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "vector origin carrier", Importance: 0.5, OriginActor: "human", OriginHost: "machine-a"})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	if err := s.StoreEmbedding(id, []float32{0.9, 0.1, 0.0, 0.0}); err != nil {
		t.Fatalf("StoreEmbedding: %v", err)
	}
	results, err := s.VectorSearch([]float32{0.9, 0.1, 0.0, 0.0}, 5, 0)
	if err != nil {
		t.Fatalf("VectorSearch: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("no vector results")
	}
	if results[0].OriginActor != "human" || results[0].OriginHost != "machine-a" {
		t.Errorf("vector search dropped origin: %+v", results[0].Node)
	}
}

func TestKeywordSearchCarriesOrigin(t *testing.T) {
	s := testStore(t)
	if !s.FTSAvailable() {
		t.Skip("FTS5 not available in this build")
	}
	if _, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "keyword origin xylophone", Importance: 0.5, OriginActor: "agent", OriginHost: "machine-b"}); err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	results, err := s.KeywordSearch("xylophone", 5)
	if err != nil {
		t.Fatalf("KeywordSearch: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("no keyword results")
	}
	if results[0].OriginActor != "agent" || results[0].OriginHost != "machine-b" {
		t.Errorf("keyword search dropped origin: %+v", results[0].Node)
	}
}
