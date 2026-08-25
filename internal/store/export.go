package store

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

// ExportVersion is the current export format version.
const ExportVersion = "1"

// ExportBundle is the portable serialization format for a brain.
type ExportBundle struct {
	Version    string            `json:"version"`
	ExportedAt time.Time         `json:"exported_at"`
	Meta       map[string]string `json:"meta,omitempty"`
	Nodes      []Node            `json:"nodes"`
	Edges      []Edge            `json:"edges"`
}

// ConflictStrategy controls how import handles ID collisions.
type ConflictStrategy string

const (
	ConflictSkip    ConflictStrategy = "skip"
	ConflictReplace ConflictStrategy = "replace"
)

// ImportOptions configures an import operation.
type ImportOptions struct {
	OnConflict ConflictStrategy
}

// ImportResult reports what happened during import.
type ImportResult struct {
	NodesImported int `json:"nodes_imported"`
	NodesSkipped  int `json:"nodes_skipped"`
	EdgesImported int `json:"edges_imported"`
	EdgesSkipped  int `json:"edges_skipped"`
}

// Export produces a complete ExportBundle of all nodes, edges, and metadata.
func (s *Store) Export() (*ExportBundle, error) {
	nodes, err := s.ListNodes(ListNodesOpts{})
	if err != nil {
		return nil, fmt.Errorf("listing nodes: %w", err)
	}

	edges, err := s.ListAllEdges()
	if err != nil {
		return nil, fmt.Errorf("listing edges: %w", err)
	}

	meta, err := s.GetAllMeta()
	if err != nil {
		return nil, fmt.Errorf("reading meta: %w", err)
	}

	if nodes == nil {
		nodes = []Node{}
	}
	if edges == nil {
		edges = []Edge{}
	}

	return &ExportBundle{
		Version:    ExportVersion,
		ExportedAt: time.Now().UTC(),
		Meta:       meta,
		Nodes:      nodes,
		Edges:      edges,
	}, nil
}

// ListAllEdges returns every edge in the store.
func (s *Store) ListAllEdges() ([]Edge, error) {
	rows, err := s.db.Query(`SELECT id, source_id, target_id, relation, weight, metadata, created_at, valid_at, invalid_at FROM edges ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("listing edges: %w", err)
	}
	defer rows.Close() //nolint:errcheck // best-effort cleanup

	var edges []Edge
	for rows.Next() {
		e, err := scanEdge(rows)
		if err != nil {
			return nil, err
		}
		edges = append(edges, *e)
	}
	return edges, rows.Err()
}

// GetAllMeta returns all key-value pairs from schema_meta.
func (s *Store) GetAllMeta() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT key, value FROM schema_meta`)
	if err != nil {
		return nil, fmt.Errorf("reading meta: %w", err)
	}
	defer rows.Close() //nolint:errcheck // best-effort cleanup

	meta := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("scanning meta: %w", err)
		}
		meta[k] = v
	}
	return meta, rows.Err()
}

// AddNodeWithID inserts a node with a caller-specified ID (for import).
// On conflict, the existing node is kept (INSERT OR IGNORE).
func (s *Store) AddNodeWithID(id string, opts *AddNodeOpts) error {
	decayRate := DefaultDecayRate(opts.Type)
	importance := opts.Importance
	if importance <= 0 {
		importance = 0.5
	}

	_, err := s.db.Exec(`
		INSERT OR IGNORE INTO nodes (id, type, subtype, content, metadata, importance, decay_rate, embedding_model, origin_actor, origin_channel, origin_session, origin_host)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, opts.Type, nullString(opts.Subtype), opts.Content, nullJSON(opts.Metadata),
		importance, decayRate, opts.EmbeddingModel,
		nullString(opts.OriginActor), nullString(opts.OriginChannel), nullString(opts.OriginSession), nullString(opts.OriginHost),
	)
	if err != nil {
		return fmt.Errorf("inserting node with id: %w", err)
	}
	return nil
}

// Import loads an ExportBundle into the store.
func (s *Store) Import(bundle *ExportBundle, opts ImportOptions) (*ImportResult, error) {
	result := &ImportResult{}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Import meta (skip schema_version — keep destination's)
	for k, v := range bundle.Meta {
		if k == "schema_version" {
			continue
		}
		if _, err := tx.Exec(
			`INSERT INTO schema_meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
			k, v,
		); err != nil {
			return nil, fmt.Errorf("setting meta %s: %w", k, err)
		}
	}

	// Import nodes
	var insertSQL string
	switch opts.OnConflict {
	case ConflictReplace:
		insertSQL = `INSERT OR REPLACE INTO nodes (id, type, subtype, content, metadata, importance, decay_rate,
			access_count, times_reinforced, status, embedding_model, created_at, last_accessed, last_reinforced,
			updated_at, last_surfaced, provenance_root, origin_actor, origin_channel, origin_session, origin_host)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	default: // skip
		insertSQL = `INSERT OR IGNORE INTO nodes (id, type, subtype, content, metadata, importance, decay_rate,
			access_count, times_reinforced, status, embedding_model, created_at, last_accessed, last_reinforced,
			updated_at, last_surfaced, provenance_root, origin_actor, origin_channel, origin_session, origin_host)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	}

	nodeStmt, err := tx.Prepare(insertSQL)
	if err != nil {
		return nil, fmt.Errorf("preparing node insert: %w", err)
	}
	defer nodeStmt.Close() //nolint:errcheck // best-effort cleanup

	for i := range bundle.Nodes {
		n := &bundle.Nodes[i]
		var lastReinforced, updatedAt, lastSurfaced any
		if n.LastReinforced != nil {
			lastReinforced = n.LastReinforced.UTC().Format(time.RFC3339)
		}
		if n.UpdatedAt != nil {
			updatedAt = n.UpdatedAt.UTC().Format(time.RFC3339)
		}
		if n.LastSurfaced != nil {
			lastSurfaced = n.LastSurfaced.UTC().Format(time.RFC3339)
		}
		// Origin of entry is never silently blank (agentic-goc7): a bundle node
		// that carries no recorded actor arrived here THROUGH an import, so the
		// import itself is stamped as actor/channel. A recorded origin is
		// carried through untouched.
		originActor, originChannel := n.OriginActor, n.OriginChannel
		if originActor == "" {
			originActor, originChannel = "import", "import"
		}

		res, err := nodeStmt.Exec(
			n.ID, n.Type, nullString(n.Subtype), n.Content, nullJSON(n.Metadata),
			n.Importance, n.DecayRate, n.AccessCount, n.TimesReinforced,
			n.Status, n.EmbeddingModel,
			n.CreatedAt.UTC().Format(time.RFC3339),
			n.LastAccessed.UTC().Format(time.RFC3339),
			lastReinforced,
			updatedAt,
			lastSurfaced,
			boolToInt(n.ProvenanceRoot),
			nullString(originActor), nullString(originChannel),
			nullString(n.OriginSession), nullString(n.OriginHost),
		)
		if err != nil {
			return nil, fmt.Errorf("importing node %s: %w", n.ID, err)
		}
		rows, _ := res.RowsAffected()
		if rows > 0 {
			result.NodesImported++
		} else {
			result.NodesSkipped++
		}
	}

	// Import edges. valid_at/invalid_at are carried through so a bundle exported
	// with bi-temporal windows round-trips them (agentic-xtzn): ListAllEdges now
	// emits the two columns, so the import must persist them to stay symmetric.
	edgeStmt, err := tx.Prepare(`INSERT OR IGNORE INTO edges (source_id, target_id, relation, weight, metadata, valid_at, invalid_at) VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return nil, fmt.Errorf("preparing edge insert: %w", err)
	}
	defer edgeStmt.Close() //nolint:errcheck // best-effort cleanup

	for i := range bundle.Edges {
		e := &bundle.Edges[i]
		res, err := edgeStmt.Exec(e.SourceID, e.TargetID, e.Relation, e.Weight, nullJSON(e.Metadata), formatBound(e.ValidAt), formatBound(e.InvalidAt))
		if err != nil {
			return nil, fmt.Errorf("importing edge %s->%s: %w", e.SourceID, e.TargetID, err)
		}
		rows, _ := res.RowsAffected()
		if rows > 0 {
			result.EdgesImported++
		} else {
			result.EdgesSkipped++
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing import: %w", err)
	}

	return result, nil
}

// ExportSQLite copies the database file to the given path.
func (s *Store) ExportSQLite(dstPath string) error {
	// Checkpoint WAL to ensure all data is in the main file
	if _, err := s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return fmt.Errorf("checkpointing WAL: %w", err)
	}

	src, err := os.Open(s.path)
	if err != nil {
		return fmt.Errorf("opening source: %w", err)
	}
	defer src.Close() //nolint:errcheck // read-only

	dst, err := os.Create(dstPath) //nolint:gosec // export path is user-specified
	if err != nil {
		return fmt.Errorf("creating destination: %w", err)
	}
	defer dst.Close() //nolint:errcheck // best-effort cleanup

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("copying database: %w", err)
	}

	return dst.Close()
}

// ExportSQL writes the brain as SQL INSERT statements to the given writer.
func (s *Store) ExportSQL(w io.Writer) error {
	bundle, err := s.Export()
	if err != nil {
		return err
	}

	// Meta
	for k, v := range bundle.Meta {
		if _, err := fmt.Fprintf(w, "INSERT OR REPLACE INTO schema_meta (key, value) VALUES ('%s', '%s');\n",
			sqlEscape(k), sqlEscape(v)); err != nil {
			return err
		}
	}

	// Nodes
	for i := range bundle.Nodes {
		n := &bundle.Nodes[i]
		metadata := "NULL"
		if len(n.Metadata) > 0 {
			metadata = fmt.Sprintf("'%s'", sqlEscape(string(n.Metadata)))
		}
		subtype := "NULL"
		if n.Subtype != "" {
			subtype = fmt.Sprintf("'%s'", sqlEscape(n.Subtype))
		}
		lastReinforced := "NULL"
		if n.LastReinforced != nil {
			lastReinforced = fmt.Sprintf("'%s'", n.LastReinforced.UTC().Format(time.RFC3339))
		}
		updatedAt := "NULL"
		if n.UpdatedAt != nil {
			updatedAt = fmt.Sprintf("'%s'", n.UpdatedAt.UTC().Format(time.RFC3339))
		}
		lastSurfaced := "NULL"
		if n.LastSurfaced != nil {
			lastSurfaced = fmt.Sprintf("'%s'", n.LastSurfaced.UTC().Format(time.RFC3339))
		}
		sqlNullable := func(v string) string {
			if v == "" {
				return "NULL"
			}
			return fmt.Sprintf("'%s'", sqlEscape(v))
		}
		if _, err := fmt.Fprintf(w,
			"INSERT OR IGNORE INTO nodes (id, type, subtype, content, metadata, importance, decay_rate, access_count, times_reinforced, status, embedding_model, created_at, last_accessed, last_reinforced, updated_at, last_surfaced, provenance_root, origin_actor, origin_channel, origin_session, origin_host) VALUES ('%s', '%s', %s, '%s', %s, %f, %f, %d, %d, '%s', '%s', '%s', '%s', %s, %s, %s, %d, %s, %s, %s, %s);\n",
			sqlEscape(n.ID), n.Type, subtype, sqlEscape(n.Content), metadata,
			n.Importance, n.DecayRate, n.AccessCount, n.TimesReinforced,
			n.Status, sqlEscape(n.EmbeddingModel),
			n.CreatedAt.UTC().Format(time.RFC3339),
			n.LastAccessed.UTC().Format(time.RFC3339),
			lastReinforced,
			updatedAt,
			lastSurfaced,
			boolToInt(n.ProvenanceRoot),
			sqlNullable(n.OriginActor), sqlNullable(n.OriginChannel),
			sqlNullable(n.OriginSession), sqlNullable(n.OriginHost),
		); err != nil {
			return err
		}
	}

	// Edges
	for i := range bundle.Edges {
		e := &bundle.Edges[i]
		metadata := "NULL"
		if len(e.Metadata) > 0 {
			metadata = fmt.Sprintf("'%s'", sqlEscape(string(e.Metadata)))
		}
		// Validity bounds emit in the storage layout ("2006-01-02 15:04:05"),
		// never RFC3339 — the as-of predicate compares raw strings, so a
		// layout drift here would silently defeat --as-of after a reimport.
		validAt := "NULL"
		if e.ValidAt != nil {
			validAt = fmt.Sprintf("'%s'", e.ValidAt.UTC().Format(storageTimeLayout))
		}
		invalidAt := "NULL"
		if e.InvalidAt != nil {
			invalidAt = fmt.Sprintf("'%s'", e.InvalidAt.UTC().Format(storageTimeLayout))
		}
		if _, err := fmt.Fprintf(w,
			"INSERT OR IGNORE INTO edges (source_id, target_id, relation, weight, metadata, valid_at, invalid_at) VALUES ('%s', '%s', '%s', %f, %s, %s, %s);\n",
			sqlEscape(e.SourceID), sqlEscape(e.TargetID), sqlEscape(e.Relation), e.Weight, metadata, validAt, invalidAt,
		); err != nil {
			return err
		}
	}

	return nil
}

// sqlEscape escapes single quotes for SQL string literals.
func sqlEscape(s string) string {
	result := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			result = append(result, '\'', '\'')
		} else {
			result = append(result, s[i])
		}
	}
	return string(result)
}

// ExportJSON writes the bundle as formatted JSON to the given writer.
func (s *Store) ExportJSON(w io.Writer) error {
	bundle, err := s.Export()
	if err != nil {
		return err
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(bundle)
}
