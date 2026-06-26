package store

import (
	"encoding/json"
	"time"
)

// NodeType represents the cognitive category of a memory.
type NodeType string

const (
	TypeEpisode    NodeType = "episode"
	TypeConcept    NodeType = "concept"
	TypeProcedure  NodeType = "procedure"
	TypeReflection NodeType = "reflection"
)

// DefaultDecayRate returns the decay rate (λ) for a given node type.
// See ADR-003 for rationale.
func DefaultDecayRate(t NodeType) float64 {
	switch t {
	case TypeEpisode:
		return 0.15 // half-life ~1-2 weeks
	case TypeConcept:
		return 0.02 // half-life ~2-3 months
	case TypeProcedure:
		return 0.005 // half-life ~6+ months
	case TypeReflection:
		return 0.05 // half-life ~3-4 weeks
	default:
		return 0.1
	}
}

// Node represents a memory node in the graph.
type Node struct {
	ID              string          `json:"id"`
	Type            NodeType        `json:"type"`
	Subtype         string          `json:"subtype,omitempty"`
	Content         string          `json:"content"`
	Metadata        json.RawMessage `json:"metadata,omitempty"`
	Importance      float64         `json:"importance"`
	DecayRate       float64         `json:"decay_rate"`
	AccessCount     int             `json:"access_count"`
	TimesReinforced int             `json:"times_reinforced"`
	Status          string          `json:"status"`
	EmbeddingModel  string          `json:"embedding_model"`
	CreatedAt       time.Time       `json:"created_at"`
	LastAccessed    time.Time       `json:"last_accessed"`
	LastReinforced  *time.Time      `json:"last_reinforced,omitempty"`
	UpdatedAt       *time.Time      `json:"updated_at,omitempty"`
	LastSurfaced    *time.Time      `json:"last_surfaced,omitempty"`
}

// Edge represents a directed relationship between two nodes.
//
// ValidAt/InvalidAt carry the bi-temporal valid-time window (agentic-xtzn): the
// half-open interval [ValidAt, InvalidAt) during which the asserted relationship
// holds in the world. Both are nullable (*time.Time):
//   - ValidAt   == nil  => valid from -inf (no lower bound).
//   - InvalidAt == nil  => still valid / open-ended (no upper bound).
//
// This is the valid-time axis, orthogonal to CreatedAt (transaction time, when
// the row was written). See ADR-015.
type Edge struct {
	ID        int64           `json:"id"`
	SourceID  string          `json:"source_id"`
	TargetID  string          `json:"target_id"`
	Relation  string          `json:"relation"`
	Weight    float64         `json:"weight"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	ValidAt   *time.Time      `json:"valid_at,omitempty"`
	InvalidAt *time.Time      `json:"invalid_at,omitempty"`
}

// AddEdgeOpts carries the optional bi-temporal validity bounds for AddEdge
// (agentic-xtzn). A nil pointer means the corresponding bound is SQL NULL
// (open-ended). The agent writes these bounds explicitly; cerebro never infers
// them (no auto-invalidation, no LLM in the loop). On a re-add of an existing
// (source, target, relation) edge, AddEdge re-asserts the FULL window: a nil
// bound OVERWRITES any prior non-NULL value to NULL — it is not a partial patch.
type AddEdgeOpts struct {
	ValidAt   *time.Time // nil => NULL (valid from -inf)
	InvalidAt *time.Time // nil => NULL (still valid / open-ended)
}

// ScoredNode is a node with a computed retrieval score.
type ScoredNode struct {
	Node
	Score      float64   `json:"score"`
	Similarity float64   `json:"similarity,omitempty"` // cosine similarity from vector search
	Embedding  []float32 `json:"-"`                    // loaded on demand for MMR diversity
}

// NodeWithEdges is a node along with its connected edges.
type NodeWithEdges struct {
	Node
	Edges []Edge `json:"edges"`
}

// Stats contains brain health metrics.
type Stats struct {
	TotalNodes          int            `json:"total_nodes"`
	ActiveNodes         int            `json:"active_nodes"`
	ConsolidatedNodes   int            `json:"consolidated_nodes"`
	SupersededNodes     int            `json:"superseded_nodes"`
	ArchivedNodes       int            `json:"archived_nodes"`
	NodesByType         map[string]int `json:"nodes_by_type"`
	TotalEdges          int            `json:"total_edges"`
	PendingEmbeddings   int            `json:"pending_embeddings"`
	EmbeddingModel      string         `json:"embedding_model"`
	EmbeddingDimensions string         `json:"embedding_dimensions"`
	SchemaVersion       string         `json:"schema_version"`
}
