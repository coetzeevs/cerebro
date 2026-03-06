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
	defer rows.Close()

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
