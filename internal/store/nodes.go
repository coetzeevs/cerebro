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

	return id, nil
}

// UpdateNodeOpts configures a node update. Only non-nil fields are applied.
type UpdateNodeOpts struct {
	Content    *string
	Metadata   json.RawMessage
	Importance *float64
}

// UpdateNode modifies an existing node's content and/or importance.
func (s *Store) UpdateNode(id string, opts UpdateNodeOpts) error {
	if opts.Content != nil {
		if _, err := s.db.Exec(`UPDATE nodes SET content = ? WHERE id = ?`, *opts.Content, id); err != nil {
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
	defer stmt.Close()

	for _, id := range ids {
		if _, err := stmt.Exec(id); err != nil {
			return fmt.Errorf("marking node %s consolidated: %w", id, err)
		}
	}

	return tx.Commit()
}

// GetNode retrieves a single node by ID.
func (s *Store) GetNode(id string) (*Node, error) {
	row := s.db.QueryRow(`SELECT id, type, subtype, content, metadata, importance, decay_rate,
		access_count, times_reinforced, status, embedding_model, created_at, last_accessed, last_reinforced
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
	Type    NodeType
	Status  string
	Since   *time.Time
	Limit   int
	OrderBy string // "importance", "created_at" (default: "created_at")
}

// ListNodes returns nodes matching the given filters.
func (s *Store) ListNodes(opts ListNodesOpts) ([]Node, error) {
	query := `SELECT id, type, subtype, content, metadata, importance, decay_rate,
		access_count, times_reinforced, status, embedding_model, created_at, last_accessed, last_reinforced
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
	if opts.Since != nil {
		query += ` AND created_at >= ?`
		args = append(args, opts.Since.Format(time.RFC3339))
	}

	switch opts.OrderBy {
	case "importance":
		query += ` ORDER BY importance DESC`
	default:
		query += ` ORDER BY created_at DESC`
	}

	if opts.Limit > 0 {
		query += fmt.Sprintf(` LIMIT %d`, opts.Limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing nodes: %w", err)
	}
	defer rows.Close()

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
	defer rows.Close()
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
	defer rows2.Close()
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

// helpers

func scanNode(row *sql.Row) (*Node, error) {
	n := &Node{}
	var subtype, metadata, lastReinforced sql.NullString
	err := row.Scan(
		&n.ID, &n.Type, &subtype, &n.Content, &metadata,
		&n.Importance, &n.DecayRate, &n.AccessCount, &n.TimesReinforced,
		&n.Status, &n.EmbeddingModel, &n.CreatedAt, &n.LastAccessed, &lastReinforced,
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
	return n, nil
}

func scanNodeFromRows(rows *sql.Rows) (*Node, error) {
	n := &Node{}
	var subtype, metadata, lastReinforced sql.NullString
	err := rows.Scan(
		&n.ID, &n.Type, &subtype, &n.Content, &metadata,
		&n.Importance, &n.DecayRate, &n.AccessCount, &n.TimesReinforced,
		&n.Status, &n.EmbeddingModel, &n.CreatedAt, &n.LastAccessed, &lastReinforced,
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
