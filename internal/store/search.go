package store

import (
	"database/sql"
	"encoding/binary"
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
			n.created_at, n.last_accessed, n.last_reinforced,
			n.updated_at, n.last_surfaced,
			n.origin_actor, n.origin_channel, n.origin_session, n.origin_host
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
		var subtype, metadata, lastReinf, updatedAt, lastSurfaced interface{}
		var originActor, originChannel, originSession, originHost sql.NullString

		err := rows.Scan(
			&nodeID, &distance,
			&sn.ID, &sn.Type, &subtype, &sn.Content, &metadata, &sn.Importance, &sn.DecayRate,
			&sn.AccessCount, &sn.TimesReinforced, &sn.Status, &sn.EmbeddingModel,
			&sn.CreatedAt, &sn.LastAccessed, &lastReinf,
			&updatedAt, &lastSurfaced,
			&originActor, &originChannel, &originSession, &originHost,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning search result: %w", err)
		}

		if s, ok := subtype.(string); ok {
			sn.Subtype = s
		}
		sn.OriginActor = originActor.String
		sn.OriginChannel = originChannel.String
		sn.OriginSession = originSession.String
		sn.OriginHost = originHost.String
		if m, ok := metadata.(string); ok {
			sn.Metadata = []byte(m)
		}
		if lr, ok := lastReinf.(string); ok {
			t, _ := time.Parse(time.RFC3339, lr)
			sn.LastReinforced = &t
		}
		if ua, ok := updatedAt.(string); ok {
			t, _ := time.Parse(time.RFC3339, ua)
			sn.UpdatedAt = &t
		}
		if ls, ok := lastSurfaced.(string); ok {
			t, _ := time.Parse(time.RFC3339, ls)
			sn.LastSurfaced = &t
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

// CosineSimilarity returns the cosine similarity between two float32 vectors.
// Returns 0 for zero vectors (avoids NaN/Inf from division by zero).
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		ai, bi := float64(a[i]), float64(b[i])
		dot += ai * bi
		normA += ai * ai
		normB += bi * bi
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// GetEmbeddings batch-loads embeddings for the given node IDs from vec_nodes.
// Returns a map of nodeID → embedding. Nodes without embeddings are absent from the map.
// Returns an error only for unexpected failures; a missing vec_nodes table returns an empty map.
func (s *Store) GetEmbeddings(ids []string) (map[string][]float32, error) {
	if len(ids) == 0 {
		return map[string][]float32{}, nil
	}

	placeholders := make([]byte, 0, len(ids)*2)
	args := make([]any, len(ids))
	for i, id := range ids {
		if i > 0 {
			placeholders = append(placeholders, ',')
		}
		placeholders = append(placeholders, '?')
		args[i] = id
	}

	query := fmt.Sprintf(`SELECT node_id, embedding FROM vec_nodes WHERE node_id IN (%s)`, placeholders) //nolint:gosec // placeholders are ? not user input

	rows, err := s.db.Query(query, args...)
	if err != nil {
		// vec_nodes may not exist if the vector table was never initialized.
		return map[string][]float32{}, nil //nolint:nilerr // graceful degradation
	}
	defer rows.Close() //nolint:errcheck // best-effort cleanup

	result := make(map[string][]float32, len(ids))
	for rows.Next() {
		var nodeID string
		var raw []byte
		if err := rows.Scan(&nodeID, &raw); err != nil {
			continue
		}
		vec := deserializeFloat32(raw)
		if vec == nil {
			continue
		}
		result[nodeID] = vec
	}
	return result, rows.Err()
}

// Surprise computes the surprise signal for a node: how likely the agent is to
// have a stale view of this memory. Returns a value in [0, 1].
//
//   - 1.0: updated_at > last_surfaced, or updated_at set and last_surfaced is nil.
//     The agent is guaranteed to have stale information.
//   - 0.5: both updated_at and last_surfaced are nil. Unknown state — conservative default.
//   - 1 - exp(-0.01 * hours_since_surfaced): gradual growth when last_surfaced is set
//     and updated_at is nil or <= last_surfaced.
func Surprise(n *Node) float64 {
	if n.UpdatedAt != nil {
		if n.LastSurfaced == nil || n.UpdatedAt.After(*n.LastSurfaced) {
			return 1.0
		}
	}

	if n.LastSurfaced == nil {
		// Both nil: unknown state.
		return 0.5
	}

	// updated_at is nil or <= last_surfaced: gradual growth with time since surfaced.
	hoursSinceSurfaced := time.Since(*n.LastSurfaced).Hours()
	if hoursSinceSurfaced < 0 {
		hoursSinceSurfaced = 0
	}
	return 1.0 - math.Exp(-0.01*hoursSinceSurfaced)
}

// PrimeScore is the scoring function for session priming candidates.
// It combines importance (0.6) with surprise (0.4) to prefer high-value stale memories.
// Exported for use by brain layer and potential external consumers.
func PrimeScore(n *Node) float64 {
	return 0.6*n.Importance + 0.4*Surprise(n)
}

// compositeScore computes the four-signal retrieval score.
// Weights: relevance=0.35, importance=0.25, recency=0.25, structural=0.15
// The structural parameter should be 0-1 (scaled by edge weight).
func compositeScore(n *Node, similarity, structural float64) float64 {
	relevance := similarity

	// Importance with access reinforcement
	importance := n.Importance * (1.0 + math.Log1p(float64(n.AccessCount)))

	// Recency: exponential decay from last access. DecayRate is a PER-DAY
	// lambda (ADR-003's half-life table: episode 0.15 ~= 1-2 weeks). The
	// pre-N2 code applied it per-hour — ~24x faster than documented intent,
	// flooring the recency term for anything not touched today (C3 ruled
	// "wire-lite": fix the units, keep the model).
	daysSinceAccess := time.Since(n.LastAccessed).Hours() / 24
	recency := math.Exp(-n.DecayRate * daysSinceAccess)

	return 0.35*relevance + 0.25*importance + 0.25*recency + 0.15*structural
}

// ExpandGraph performs 1-hop graph expansion on search results.
// For each result node, it follows edges to discover connected nodes.
// Connected nodes not already in results are scored and merged in.
// Result nodes connected to other results get a structural bonus.
//
// When asOf is non-nil, only edges valid at that instant are traversed
// (agentic-xtzn): the validity filter is applied at the edge-fetch SQL, so a
// neighbour reachable only through an out-of-window edge is not expanded in. A
// nil asOf omits the predicate entirely (byte-identical to the pre-xtzn path).
func (s *Store) ExpandGraph(results []ScoredNode, limit int, asOf *time.Time) ([]ScoredNode, error) {
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
	edgeMap, err := s.GetEdgesBatch(resultIDs, asOf)
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

// deserializeFloat32 converts raw bytes (little-endian IEEE 754) to a float32 slice.
// This mirrors the format produced by sqlite_vec.SerializeFloat32.
// Returns nil if the byte slice length is not a multiple of 4.
func deserializeFloat32(b []byte) []float32 {
	if len(b)%4 != 0 {
		return nil
	}
	n := len(b) / 4
	vec := make([]float32, n)
	for i := 0; i < n; i++ {
		bits := binary.LittleEndian.Uint32(b[i*4 : (i+1)*4])
		vec[i] = math.Float32frombits(bits)
	}
	return vec
}
