package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// AddNodeOpts configures a new node.
type AddNodeOpts struct {
	Type           NodeType
	Subtype        string
	Content        string
	Metadata       json.RawMessage
	Importance     float64
	EmbeddingModel string
}

// AddNode inserts a new memory node and returns its ID.
func (s *Store) AddNode(opts *AddNodeOpts) (string, error) {
	id := uuid.New().String()
	decayRate := DefaultDecayRate(opts.Type)

	importance := opts.Importance
	if importance <= 0 {
		importance = 0.5
	}

	_, err := s.db.Exec(`
		INSERT INTO nodes (id, type, subtype, content, metadata, importance, decay_rate, embedding_model)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, opts.Type, nullString(opts.Subtype), opts.Content, nullJSON(opts.Metadata),
		importance, decayRate, opts.EmbeddingModel,
	)
	if err != nil {
		return "", fmt.Errorf("inserting node: %w", err)
	}

	// Sync the keyword index (agentic-2lak). Best-effort on the connection path:
	// a transient FTS failure must not fail an AddNode whose nodes write already
	// committed (graceful degrade, D2/S-PI-N2). No-op when nodes_fts is absent.
	_ = s.syncFTSInsert(s.db, false, id, opts.Content, opts.Subtype)

	return id, nil
}

// UpdateNodeOpts configures a node update. Only non-nil fields are applied.
type UpdateNodeOpts struct {
	Content    *string
	Metadata   json.RawMessage
	Importance *float64
	// Subtype, when non-nil, updates the node's subtype.
	// A pointer to an empty string (&"") clears the subtype to NULL.
	// A pointer to a non-empty string sets the subtype to that value.
	// Nil leaves the existing subtype unchanged.
	// Note: subtype changes stamp updated_at because subtype is knowledge-classification
	// metadata — it changes what the memory means to the retrieval taxonomy.
	Subtype *string
}

// UpdateNode modifies an existing node's content and/or importance.
// When Content is set, updated_at is stamped to CURRENT_TIMESTAMP to record
// that the knowledge content changed. Importance-only and metadata-only updates
// do NOT touch updated_at — they are scoring/bookkeeping adjustments, not
// knowledge refreshes.
func (s *Store) UpdateNode(id string, opts UpdateNodeOpts) error {
	if opts.Content != nil {
		if _, err := s.db.Exec(
			`UPDATE nodes SET content = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
			*opts.Content, id,
		); err != nil {
			return fmt.Errorf("updating content: %w", err)
		}
	}
	if opts.Importance != nil {
		if _, err := s.db.Exec(`UPDATE nodes SET importance = ? WHERE id = ?`, *opts.Importance, id); err != nil {
			return fmt.Errorf("updating importance: %w", err)
		}
	}
	if opts.Metadata != nil {
		if _, err := s.db.Exec(`UPDATE nodes SET metadata = ? WHERE id = ?`, opts.Metadata, id); err != nil {
			return fmt.Errorf("updating metadata: %w", err)
		}
	}
	// Subtype: nil = no-op; &"" = clear to NULL; &"x" = set to "x".
	// updated_at is stamped because subtype is knowledge-classification metadata.
	if opts.Subtype != nil {
		if _, err := s.db.Exec(
			`UPDATE nodes SET subtype = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
			nullString(*opts.Subtype), id,
		); err != nil {
			return fmt.Errorf("updating subtype: %w", err)
		}
	}

	// Refresh the keyword index (agentic-2lak) when content or subtype changed.
	// Re-read the node's current content+subtype so the FTS row mirrors the full
	// post-update state regardless of which field changed. Best-effort: a stale
	// FTS row never fails an UpdateNode whose nodes write succeeded (D2). No-op
	// when nodes_fts is absent.
	if (opts.Content != nil || opts.Subtype != nil) && s.ftsAvailable() {
		var content string
		var subtype sql.NullString
		if err := s.db.QueryRow(`SELECT content, subtype FROM nodes WHERE id = ?`, id).Scan(&content, &subtype); err == nil {
			_ = s.syncFTSInsert(s.db, false, id, content, subtype.String)
		}
	}
	return nil
}

// SupersedeNode marks an old node as superseded and creates a new one with a
// 'supersedes' edge. Returns the new node ID.
func (s *Store) SupersedeNode(oldID string, opts *AddNodeOpts) (string, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return "", fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Mark old node as superseded
	res, err := tx.Exec(`UPDATE nodes SET status = 'superseded' WHERE id = ? AND status = 'active'`, oldID)
	if err != nil {
		return "", fmt.Errorf("superseding old node: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return "", fmt.Errorf("node %s not found or not active", oldID)
	}

	// Insert new node
	newID := uuid.New().String()
	decayRate := DefaultDecayRate(opts.Type)
	importance := opts.Importance
	if importance <= 0 {
		importance = 0.5
	}

	_, err = tx.Exec(`
		INSERT INTO nodes (id, type, subtype, content, metadata, importance, decay_rate, embedding_model)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		newID, opts.Type, nullString(opts.Subtype), opts.Content, nullJSON(opts.Metadata),
		importance, decayRate, opts.EmbeddingModel,
	)
	if err != nil {
		return "", fmt.Errorf("inserting new node: %w", err)
	}

	// Create supersedes edge
	_, err = tx.Exec(`
		INSERT INTO edges (source_id, target_id, relation) VALUES (?, ?, 'supersedes')`,
		newID, oldID,
	)
	if err != nil {
		return "", fmt.Errorf("creating supersedes edge: %w", err)
	}

	// Sync the keyword index inside the SAME transaction (S-PI-N2): the old node
	// becomes inactive (drop its FTS row) and the new active node is indexed. On
	// the tx path the FTS write MUST succeed — a sync error propagates and rolls
	// the whole supersede back, so nodes and nodes_fts can never half-commit. The
	// helpers no-op gracefully when nodes_fts is absent (no-fts5 binary).
	if err := s.deleteFTS(tx, true, oldID); err != nil {
		return "", err
	}
	if err := s.syncFTSInsert(tx, true, newID, opts.Content, opts.Subtype); err != nil {
		return "", err
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("committing transaction: %w", err)
	}

	return newID, nil
}

// ReinforceNode increments access_count and updates last_accessed.
func (s *Store) ReinforceNode(id string) error {
	res, err := s.db.Exec(`
		UPDATE nodes SET
			access_count = access_count + 1,
			times_reinforced = times_reinforced + 1,
			last_accessed = CURRENT_TIMESTAMP,
			last_reinforced = CURRENT_TIMESTAMP
		WHERE id = ? AND status = 'active'`, id)
	if err != nil {
		return fmt.Errorf("reinforcing node: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("node %s not found or not active", id)
	}
	return nil
}

// MarkConsolidated sets the given node IDs to status='consolidated'.
func (s *Store) MarkConsolidated(ids []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`UPDATE nodes SET status = 'consolidated' WHERE id = ? AND type = 'episode' AND status = 'active'`)
	if err != nil {
		return fmt.Errorf("preparing statement: %w", err)
	}
	defer stmt.Close() //nolint:errcheck // best-effort cleanup

	for _, id := range ids {
		if _, err := stmt.Exec(id); err != nil {
			return fmt.Errorf("marking node %s consolidated: %w", id, err)
		}
	}

	return tx.Commit()
}

// ResolvePrefix resolves a short ID prefix to a full UUID.
// Accepts full UUIDs (returned as-is) or unique prefixes (minimum 4 chars).
// Returns an error if the prefix is ambiguous or matches no nodes.
func (s *Store) ResolvePrefix(prefix string) (string, error) {
	if prefix == "" {
		return "", fmt.Errorf("empty ID prefix")
	}

	// Full UUID — return as-is (36 chars = 8-4-4-4-12 with hyphens)
	if len(prefix) == 36 {
		return prefix, nil
	}

	if len(prefix) < 4 {
		return "", fmt.Errorf("ID prefix too short (minimum 4 characters): %q", prefix)
	}

	rows, err := s.db.Query(`SELECT id FROM nodes WHERE id LIKE ?`, prefix+"%")
	if err != nil {
		return "", fmt.Errorf("resolving prefix: %w", err)
	}
	defer rows.Close() //nolint:errcheck // best-effort cleanup

	var matches []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", fmt.Errorf("scanning prefix match: %w", err)
		}
		matches = append(matches, id)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterating prefix matches: %w", err)
	}

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no node found with prefix %q", prefix)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("ambiguous prefix %q matches %d nodes", prefix, len(matches))
	}
}

// GetNode retrieves a single node by ID.
func (s *Store) GetNode(id string) (*Node, error) {
	row := s.db.QueryRow(`SELECT id, type, subtype, content, metadata, importance, decay_rate,
		access_count, times_reinforced, status, embedding_model, created_at, last_accessed, last_reinforced,
		updated_at, last_surfaced
		FROM nodes WHERE id = ?`, id)
	return scanNode(row)
}

// GetNodeWithEdges retrieves a node and all its connected edges.
func (s *Store) GetNodeWithEdges(id string) (*NodeWithEdges, error) {
	node, err := s.GetNode(id)
	if err != nil {
		return nil, err
	}

	edges, err := s.getEdgesForNode(id)
	if err != nil {
		return nil, err
	}

	return &NodeWithEdges{Node: *node, Edges: edges}, nil
}

// ListNodesOpts configures node listing filters.
type ListNodesOpts struct {
	Type   NodeType
	Status string
	// Subtype, when non-nil, filters nodes by subtype.
	// A pointer to an empty string (&"") matches only nodes with NULL subtype.
	// A pointer to a non-empty string matches only nodes with that exact subtype.
	// Nil means no filter — all subtypes (including NULL) are returned.
	// This asymmetry with update (where &"" clears to NULL) is intentional:
	// on list/recall, &"" means "show me untagged memories".
	Subtype      *string
	Since        *time.Time // filters on created_at >= ?
	SinceChanged *time.Time // filters on COALESCE(updated_at, created_at) >= ?
	Limit        int
	OrderBy      string // "importance", "created_at", "recently_changed" (default: "created_at")
}

// ListNodes returns nodes matching the given filters.
func (s *Store) ListNodes(opts ListNodesOpts) ([]Node, error) { //nolint:gocritic // hugeParam: ListNodesOpts is intentionally a value type for clarity; cost is copy-on-call not heap alloc
	query := `SELECT id, type, subtype, content, metadata, importance, decay_rate,
		access_count, times_reinforced, status, embedding_model, created_at, last_accessed, last_reinforced,
		updated_at, last_surfaced
		FROM nodes WHERE 1=1`
	var args []any

	if opts.Type != "" {
		query += ` AND type = ?`
		args = append(args, opts.Type)
	}
	if opts.Status != "" {
		query += ` AND status = ?`
		args = append(args, opts.Status)
	}
	// Subtype filter: nil = no filter; &"" = NULL-only; &"x" = exact match.
	// The IS NULL branch uses a constant SQL literal; no user input is concatenated.
	// The non-empty branch uses ? parameter binding — injection-impossible by construction.
	if opts.Subtype != nil {
		if *opts.Subtype == "" {
			query += ` AND subtype IS NULL`
		} else {
			query += ` AND subtype = ?`
			args = append(args, *opts.Subtype)
		}
	}
	if opts.Since != nil {
		query += ` AND created_at >= ?`
		args = append(args, opts.Since.UTC().Format("2006-01-02 15:04:05"))
	}
	if opts.SinceChanged != nil {
		query += ` AND COALESCE(updated_at, created_at) >= ?`
		args = append(args, opts.SinceChanged.UTC().Format("2006-01-02 15:04:05"))
	}

	switch opts.OrderBy {
	case "importance":
		query += ` ORDER BY importance DESC`
	case "recently_changed":
		query += ` ORDER BY COALESCE(updated_at, created_at) DESC`
	default:
		query += ` ORDER BY created_at DESC`
	}

	if opts.Limit > 0 {
		query += fmt.Sprintf(` LIMIT %d`, opts.Limit) //nolint:gosec // limit is an int, not user input
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing nodes: %w", err)
	}
	defer rows.Close() //nolint:errcheck // best-effort cleanup

	var nodes []Node
	for rows.Next() {
		n, err := scanNodeFromRows(rows)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, *n)
	}
	return nodes, rows.Err()
}

// GetStats returns brain health metrics.
func (s *Store) GetStats() (*Stats, error) {
	stats := &Stats{
		NodesByType: make(map[string]int),
	}

	// Total and by-status counts
	rows, err := s.db.Query(`SELECT status, COUNT(*) FROM nodes GROUP BY status`)
	if err != nil {
		return nil, fmt.Errorf("counting nodes by status: %w", err)
	}
	defer rows.Close() //nolint:errcheck // best-effort cleanup
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		stats.TotalNodes += count
		switch status {
		case "active":
			stats.ActiveNodes = count
		case "consolidated":
			stats.ConsolidatedNodes = count
		case "superseded":
			stats.SupersededNodes = count
		case "archived":
			stats.ArchivedNodes = count
		}
	}

	// By-type counts
	rows2, err := s.db.Query(`SELECT type, COUNT(*) FROM nodes WHERE status = 'active' GROUP BY type`)
	if err != nil {
		return nil, fmt.Errorf("counting nodes by type: %w", err)
	}
	defer rows2.Close() //nolint:errcheck // best-effort cleanup
	for rows2.Next() {
		var t string
		var count int
		if err := rows2.Scan(&t, &count); err != nil {
			return nil, err
		}
		stats.NodesByType[t] = count
	}

	// Edge count
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM edges`).Scan(&stats.TotalEdges); err != nil {
		return nil, fmt.Errorf("counting edges: %w", err)
	}

	// Pending embeddings (nodes with no vector entry)
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM nodes WHERE embedding_model = '' AND status = 'active'`).Scan(&stats.PendingEmbeddings); err != nil {
		return nil, fmt.Errorf("counting pending embeddings: %w", err)
	}

	// Meta
	stats.EmbeddingModel, _ = s.GetMeta("embedding_model")
	stats.EmbeddingDimensions, _ = s.GetMeta("embedding_dimensions")
	stats.SchemaVersion, _ = s.GetMeta("schema_version")

	return stats, nil
}

// GetNodesByIDs retrieves multiple active nodes by their IDs in a single query.
func (s *Store) GetNodesByIDs(ids []string) ([]Node, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	// Build IN clause with placeholders
	placeholders := make([]byte, 0, len(ids)*2)
	args := make([]any, len(ids))
	for i, id := range ids {
		if i > 0 {
			placeholders = append(placeholders, ',')
		}
		placeholders = append(placeholders, '?')
		args[i] = id
	}

	query := fmt.Sprintf(`SELECT id, type, subtype, content, metadata, importance, decay_rate,
		access_count, times_reinforced, status, embedding_model, created_at, last_accessed, last_reinforced,
		updated_at, last_surfaced
		FROM nodes WHERE id IN (%s) AND status = 'active'`, placeholders) //nolint:gosec // placeholders are ? not user input

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("batch get nodes: %w", err)
	}
	defer rows.Close() //nolint:errcheck // rows.Close in defer is idiomatic

	var nodes []Node
	for rows.Next() {
		n, err := scanNodeFromRows(rows)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, *n)
	}
	return nodes, rows.Err()
}

// TouchSurfaced batch-updates last_surfaced = CURRENT_TIMESTAMP for the given node IDs.
// This is called after session priming to record that the agent was shown these memories.
// Empty slice is a no-op.
func (s *Store) TouchSurfaced(ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("beginning TouchSurfaced transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`UPDATE nodes SET last_surfaced = CURRENT_TIMESTAMP WHERE id = ?`)
	if err != nil {
		return fmt.Errorf("preparing TouchSurfaced statement: %w", err)
	}
	defer stmt.Close() //nolint:errcheck // best-effort cleanup

	for _, id := range ids {
		if _, err := stmt.Exec(id); err != nil {
			return fmt.Errorf("touching surfaced for node %s: %w", id, err)
		}
	}

	return tx.Commit()
}

// helpers

func scanNode(row *sql.Row) (*Node, error) {
	n := &Node{}
	var subtype, metadata, lastReinforced, updatedAt, lastSurfaced sql.NullString
	err := row.Scan(
		&n.ID, &n.Type, &subtype, &n.Content, &metadata,
		&n.Importance, &n.DecayRate, &n.AccessCount, &n.TimesReinforced,
		&n.Status, &n.EmbeddingModel, &n.CreatedAt, &n.LastAccessed, &lastReinforced,
		&updatedAt, &lastSurfaced,
	)
	if err != nil {
		return nil, fmt.Errorf("scanning node: %w", err)
	}
	n.Subtype = subtype.String
	if metadata.Valid {
		n.Metadata = json.RawMessage(metadata.String)
	}
	if lastReinforced.Valid {
		t, _ := time.Parse(time.RFC3339, lastReinforced.String)
		n.LastReinforced = &t
	}
	if updatedAt.Valid {
		t, _ := time.Parse(time.RFC3339, updatedAt.String)
		n.UpdatedAt = &t
	}
	if lastSurfaced.Valid {
		t, _ := time.Parse(time.RFC3339, lastSurfaced.String)
		n.LastSurfaced = &t
	}
	return n, nil
}

func scanNodeFromRows(rows *sql.Rows) (*Node, error) {
	n := &Node{}
	var subtype, metadata, lastReinforced, updatedAt, lastSurfaced sql.NullString
	err := rows.Scan(
		&n.ID, &n.Type, &subtype, &n.Content, &metadata,
		&n.Importance, &n.DecayRate, &n.AccessCount, &n.TimesReinforced,
		&n.Status, &n.EmbeddingModel, &n.CreatedAt, &n.LastAccessed, &lastReinforced,
		&updatedAt, &lastSurfaced,
	)
	if err != nil {
		return nil, fmt.Errorf("scanning node: %w", err)
	}
	n.Subtype = subtype.String
	if metadata.Valid {
		n.Metadata = json.RawMessage(metadata.String)
	}
	if lastReinforced.Valid {
		t, _ := time.Parse(time.RFC3339, lastReinforced.String)
		n.LastReinforced = &t
	}
	if updatedAt.Valid {
		t, _ := time.Parse(time.RFC3339, updatedAt.String)
		n.UpdatedAt = &t
	}
	if lastSurfaced.Valid {
		t, _ := time.Parse(time.RFC3339, lastSurfaced.String)
		n.LastSurfaced = &t
	}
	return n, nil
}

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func nullJSON(data json.RawMessage) sql.NullString {
	if len(data) == 0 {
		return sql.NullString{}
	}
	return sql.NullString{String: string(data), Valid: true}
}
