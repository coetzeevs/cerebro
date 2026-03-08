package store

import (
	"encoding/json"
	"fmt"

	"database/sql"
)

// AddEdge creates a directed relationship between two nodes.
func (s *Store) AddEdge(sourceID, targetID, relation string) (int64, error) {
	res, err := s.db.Exec(`
		INSERT INTO edges (source_id, target_id, relation)
		VALUES (?, ?, ?)
		ON CONFLICT (source_id, target_id, relation) DO NOTHING`,
		sourceID, targetID, relation,
	)
	if err != nil {
		return 0, fmt.Errorf("inserting edge: %w", err)
	}
	id, _ := res.LastInsertId()
	return id, nil
}

// getEdgesForNode returns all edges where the node is source or target.
func (s *Store) getEdgesForNode(nodeID string) ([]Edge, error) {
	rows, err := s.db.Query(`
		SELECT id, source_id, target_id, relation, weight, metadata, created_at
		FROM edges WHERE source_id = ? OR target_id = ?
		ORDER BY created_at`, nodeID, nodeID)
	if err != nil {
		return nil, fmt.Errorf("querying edges: %w", err)
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

// GetEdgesBatch returns edges for multiple nodes in a single query.
// The result maps each input node ID to its edges (where it appears as source or target).
func (s *Store) GetEdgesBatch(nodeIDs []string) (map[string][]Edge, error) {
	result := make(map[string][]Edge)
	if len(nodeIDs) == 0 {
		return result, nil
	}

	// Build IN clause
	placeholders := make([]byte, 0, len(nodeIDs)*2)
	// We need the IDs twice (source and target)
	args := make([]any, 0, len(nodeIDs)*2)
	for i, id := range nodeIDs {
		if i > 0 {
			placeholders = append(placeholders, ',')
		}
		placeholders = append(placeholders, '?')
		args = append(args, id)
	}
	// Duplicate args for the second IN clause
	for _, id := range nodeIDs {
		args = append(args, id)
	}

	query := fmt.Sprintf(`SELECT id, source_id, target_id, relation, weight, metadata, created_at
		FROM edges WHERE source_id IN (%s) OR target_id IN (%s)
		ORDER BY created_at`, placeholders, placeholders) //nolint:gosec  // G201: placeholders are ? not user input

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("batch get edges: %w", err)
	}
	defer rows.Close() //nolint:errcheck // best-effort cleanup

	// Build a set for fast lookup
	idSet := make(map[string]bool, len(nodeIDs))
	for _, id := range nodeIDs {
		idSet[id] = true
	}

	for rows.Next() {
		e, err := scanEdge(rows)
		if err != nil {
			return nil, err
		}
		// Map edge to each input node it connects to
		if idSet[e.SourceID] {
			result[e.SourceID] = append(result[e.SourceID], *e)
		}
		if idSet[e.TargetID] && e.SourceID != e.TargetID {
			result[e.TargetID] = append(result[e.TargetID], *e)
		}
	}
	return result, rows.Err()
}

func scanEdge(rows *sql.Rows) (*Edge, error) {
	e := &Edge{}
	var metadata sql.NullString
	err := rows.Scan(&e.ID, &e.SourceID, &e.TargetID, &e.Relation, &e.Weight, &metadata, &e.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("scanning edge: %w", err)
	}
	if metadata.Valid {
		e.Metadata = json.RawMessage(metadata.String)
	}
	return e, nil
}
