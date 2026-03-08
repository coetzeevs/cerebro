package store

import (
	"fmt"
	"math"
	"time"
)

// GCResult contains the outcome of a garbage collection run.
type GCResult struct {
	Evaluated int             `json:"evaluated"`
	Archived  int             `json:"archived"`
	ByType    map[string]int  `json:"by_type"`
	Evicted   []GCEvictedNode `json:"evicted,omitempty"`
}

// GCEvictedNode records a node that was (or would be) evicted.
type GCEvictedNode struct {
	ID             string   `json:"id"`
	Type           NodeType `json:"type"`
	Content        string   `json:"content"`
	Importance     float64  `json:"importance"`
	RetentionScore float64  `json:"retention_score"`
}

// GC evaluates all active nodes and archives those whose retention score
// falls below the threshold. If dryRun is true, it reports what would be
// archived without modifying data.
func (s *Store) GC(threshold float64, dryRun bool) (*GCResult, error) {
	result := &GCResult{
		ByType: make(map[string]int),
	}

	// Fetch all active nodes
	nodes, err := s.ListNodes(ListNodesOpts{Status: "active"})
	if err != nil {
		return nil, fmt.Errorf("listing active nodes: %w", err)
	}
	result.Evaluated = len(nodes)

	// Identify candidates for eviction
	type candidate struct {
		node           Node
		retentionScore float64
	}
	var candidates []candidate
	for i := range nodes {
		score := retentionScore(&nodes[i])
		if score < threshold {
			candidates = append(candidates, candidate{node: nodes[i], retentionScore: score})
		}
	}
	result.Archived = len(candidates)

	for i := range candidates {
		result.ByType[string(candidates[i].node.Type)]++
		result.Evicted = append(result.Evicted, GCEvictedNode{
			ID:             candidates[i].node.ID,
			Type:           candidates[i].node.Type,
			Content:        candidates[i].node.Content,
			Importance:     candidates[i].node.Importance,
			RetentionScore: candidates[i].retentionScore,
		})
	}

	if dryRun || len(candidates) == 0 {
		return result, nil
	}

	// Archive candidates in a single transaction
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("beginning gc transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	archiveStmt, err := tx.Prepare(`INSERT INTO nodes_archive (id, type, subtype, content, metadata, importance, status, archive_reason, original_created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'decayed', ?)`)
	if err != nil {
		return nil, fmt.Errorf("preparing archive statement: %w", err)
	}
	defer archiveStmt.Close() //nolint:errcheck // best-effort cleanup

	deleteVecStmt, err := tx.Prepare(`DELETE FROM vec_nodes WHERE node_id = ?`)
	if err != nil {
		// vec_nodes may not exist if no embeddings configured — that's OK
		deleteVecStmt = nil
	}
	if deleteVecStmt != nil {
		defer deleteVecStmt.Close() //nolint:errcheck // best-effort cleanup
	}

	deleteNodeStmt, err := tx.Prepare(`DELETE FROM nodes WHERE id = ?`)
	if err != nil {
		return nil, fmt.Errorf("preparing delete statement: %w", err)
	}
	defer deleteNodeStmt.Close() //nolint:errcheck // best-effort cleanup

	for i := range candidates {
		n := candidates[i].node
		if _, err := archiveStmt.Exec(n.ID, n.Type, nullString(n.Subtype), n.Content, nullJSON(n.Metadata), n.Importance, n.Status, n.CreatedAt.Format(time.RFC3339)); err != nil {
			return nil, fmt.Errorf("archiving node %s: %w", n.ID, err)
		}
		if deleteVecStmt != nil {
			if _, err := deleteVecStmt.Exec(n.ID); err != nil {
				return nil, fmt.Errorf("deleting embedding for %s: %w", n.ID, err)
			}
		}
		if _, err := deleteNodeStmt.Exec(n.ID); err != nil {
			return nil, fmt.Errorf("deleting node %s: %w", n.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing gc transaction: %w", err)
	}

	return result, nil
}

// retentionScore computes how worth keeping a node is, based on
// importance (with reinforcement) and recency (exponential decay).
// No similarity component — GC has no query vector.
func retentionScore(n *Node) float64 {
	importance := n.Importance * (1.0 + math.Log1p(float64(n.AccessCount)))
	hoursSinceAccess := time.Since(n.LastAccessed).Hours()
	recency := math.Exp(-n.DecayRate * hoursSinceAccess)
	return 0.5*importance + 0.5*recency
}
