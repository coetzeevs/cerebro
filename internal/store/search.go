package store

import (
	"fmt"
	"math"
	"time"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
)

// StoreEmbedding inserts or replaces a vector embedding for a node.
func (s *Store) StoreEmbedding(nodeID string, vec []float32) error {
	serialized, err := sqlite_vec.SerializeFloat32(vec)
	if err != nil {
		return fmt.Errorf("serializing vector: %w", err)
	}

	// Delete existing embedding for this node (upsert)
	if _, err := s.db.Exec(`DELETE FROM vec_nodes WHERE node_id = ?`, nodeID); err != nil {
		return fmt.Errorf("deleting old embedding for %s: %w", nodeID, err)
	}

	_, err = s.db.Exec(`INSERT INTO vec_nodes (node_id, embedding) VALUES (?, ?)`, nodeID, serialized)
	if err != nil {
		return fmt.Errorf("storing embedding for %s: %w", nodeID, err)
	}

	return nil
}

// VectorSearch finds nodes similar to the given vector using sqlite-vec.
func (s *Store) VectorSearch(vec []float32, limit int, threshold float64) ([]ScoredNode, error) {
	if limit <= 0 {
		limit = 10
	}

	serialized, err := sqlite_vec.SerializeFloat32(vec)
	if err != nil {
		return nil, fmt.Errorf("serializing query vector: %w", err)
	}

	// sqlite-vec returns cosine distance (0 = identical, 2 = opposite).
	// We convert to similarity: 1 - (distance / 2).
	//
	// vec0 virtual tables don't support JOINs inside the WHERE clause,
	// so we fetch candidates from vec_nodes first, then JOIN with nodes
	// to get full node data and filter by status.
	rows, err := s.db.Query(`
		SELECT
			v.node_id,
			v.distance,
			n.id, n.type, n.subtype, n.content, n.metadata, n.importance, n.decay_rate,
			n.access_count, n.times_reinforced, n.status, n.embedding_model,
			n.created_at, n.last_accessed, n.last_reinforced
		FROM (
			SELECT node_id, distance
			FROM vec_nodes
			WHERE embedding MATCH ?
			ORDER BY distance
			LIMIT ?
		) v
		JOIN nodes n ON n.id = v.node_id
		WHERE n.status = 'active'
		ORDER BY v.distance ASC`,
		serialized, limit*3, // fetch extra to account for non-active filtered out
	)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	defer rows.Close()

	var results []ScoredNode
	for rows.Next() {
		var sn ScoredNode
		var nodeID string
		var distance float64
		var subtype, metadata, lastReinf interface{}

		err := rows.Scan(
			&nodeID, &distance,
			&sn.ID, &sn.Type, &subtype, &sn.Content, &metadata, &sn.Importance, &sn.DecayRate,
			&sn.AccessCount, &sn.TimesReinforced, &sn.Status, &sn.EmbeddingModel,
			&sn.CreatedAt, &sn.LastAccessed, &lastReinf,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning search result: %w", err)
		}

		if s, ok := subtype.(string); ok {
			sn.Subtype = s
		}
		if m, ok := metadata.(string); ok {
			sn.Metadata = []byte(m)
		}
		if lr, ok := lastReinf.(string); ok {
			t, _ := time.Parse(time.RFC3339, lr)
			sn.LastReinforced = &t
		}

		// Convert cosine distance to similarity
		similarity := 1.0 - (distance / 2.0)
		if similarity < threshold {
			continue
		}

		sn.Similarity = similarity
		sn.Score = compositeScore(&sn.Node, similarity)
		results = append(results, sn)

		if len(results) >= limit {
			break
		}
	}

	return results, rows.Err()
}

// compositeScore computes the four-signal retrieval score.
// Weights: relevance=0.35, importance=0.25, recency=0.25, structural=0.15
// Structural bonus is computed separately during graph expansion.
func compositeScore(n *Node, similarity float64) float64 {
	relevance := similarity

	// Importance with access reinforcement
	importance := n.Importance * (1.0 + math.Log1p(float64(n.AccessCount)))

	// Recency: exponential decay from last access
	hoursSinceAccess := time.Since(n.LastAccessed).Hours()
	recency := math.Exp(-n.DecayRate * hoursSinceAccess)

	return 0.35*relevance + 0.25*importance + 0.25*recency
	// Structural (0.15) is added during graph expansion in recall
}
