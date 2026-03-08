package store

import (
	"fmt"
	"math"
	"sort"
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

	// vec0 is configured with distance_metric=cosine, returning cosine distance
	// (0 = identical, 1 = orthogonal, 2 = opposite).
	// We convert to similarity: 1 - distance.
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
	defer rows.Close() //nolint:errcheck // rows.Close in defer is idiomatic

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

		// Convert cosine distance to similarity.
		// Cosine distance: 0 = identical, 1 = orthogonal, 2 = opposite.
		similarity := 1.0 - distance
		if similarity < threshold {
			continue
		}

		sn.Similarity = similarity
		sn.Score = compositeScore(&sn.Node, similarity, 0)
		results = append(results, sn)

		if len(results) >= limit {
			break
		}
	}

	return results, rows.Err()
}

// compositeScore computes the four-signal retrieval score.
// Weights: relevance=0.35, importance=0.25, recency=0.25, structural=0.15
// The structural parameter should be 0-1 (scaled by edge weight).
func compositeScore(n *Node, similarity, structural float64) float64 {
	relevance := similarity

	// Importance with access reinforcement
	importance := n.Importance * (1.0 + math.Log1p(float64(n.AccessCount)))

	// Recency: exponential decay from last access
	hoursSinceAccess := time.Since(n.LastAccessed).Hours()
	recency := math.Exp(-n.DecayRate * hoursSinceAccess)

	return 0.35*relevance + 0.25*importance + 0.25*recency + 0.15*structural
}

// ExpandGraph performs 1-hop graph expansion on search results.
// For each result node, it follows edges to discover connected nodes.
// Connected nodes not already in results are scored and merged in.
// Result nodes connected to other results get a structural bonus.
func (s *Store) ExpandGraph(results []ScoredNode, limit int) ([]ScoredNode, error) {
	if len(results) == 0 {
		return results, nil
	}

	// Build a set and map of result node IDs
	resultIDs := make([]string, len(results))
	resultSet := make(map[string]bool, len(results))
	resultIdx := make(map[string]int, len(results))
	for i := range results {
		resultIDs[i] = results[i].ID
		resultSet[results[i].ID] = true
		resultIdx[results[i].ID] = i
	}

	// Batch-fetch edges for all result nodes
	edgeMap, err := s.GetEdgesBatch(resultIDs)
	if err != nil {
		return nil, fmt.Errorf("expanding graph: %w", err)
	}

	// Identify neighbors and compute structural bonuses
	neighborIDs := make(map[string]float64) // neighbor ID → max edge weight
	for i := range results {
		edges := edgeMap[results[i].ID]
		for j := range edges {
			// Find the other end of the edge
			otherID := edges[j].TargetID
			if otherID == results[i].ID {
				otherID = edges[j].SourceID
			}

			weight := edges[j].Weight
			if weight <= 0 {
				weight = 1.0
			}

			if resultSet[otherID] {
				// Both ends in results — boost both with structural bonus
				results[i].Score = compositeScore(&results[i].Node, results[i].Similarity, weight)
			} else {
				// Other end is a neighbor — track max weight
				if w, exists := neighborIDs[otherID]; !exists || weight > w {
					neighborIDs[otherID] = weight
				}
			}
		}
	}

	// Fetch neighbor nodes (active only)
	if len(neighborIDs) > 0 {
		ids := make([]string, 0, len(neighborIDs))
		for id := range neighborIDs {
			ids = append(ids, id)
		}

		neighbors, err := s.GetNodesByIDs(ids)
		if err != nil {
			return nil, fmt.Errorf("fetching neighbors: %w", err)
		}

		for i := range neighbors {
			weight := neighborIDs[neighbors[i].ID]
			score := compositeScore(&neighbors[i], 0, weight) // similarity=0
			results = append(results, ScoredNode{
				Node:  neighbors[i],
				Score: score,
			})
		}
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Cap at limit
	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}
