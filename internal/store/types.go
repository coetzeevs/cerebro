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
}

// Edge represents a directed relationship between two nodes.
type Edge struct {
	ID        int64           `json:"id"`
	SourceID  string          `json:"source_id"`
	TargetID  string          `json:"target_id"`
	Relation  string          `json:"relation"`
	Weight    float64         `json:"weight"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

// ScoredNode is a node with a computed retrieval score.
type ScoredNode struct {
	Node
	Score      float64 `json:"score"`
	Similarity float64 `json:"similarity,omitempty"` // cosine similarity from vector search
}

// NodeWithEdges is a node along with its connected edges.
type NodeWithEdges struct {
	Node
	Edges []Edge `json:"edges"`
}

// Stats contains brain health metrics.
type Stats struct {
	TotalNodes         int            `json:"total_nodes"`
	ActiveNodes        int            `json:"active_nodes"`
	ConsolidatedNodes  int            `json:"consolidated_nodes"`
	SupersededNodes    int            `json:"superseded_nodes"`
	ArchivedNodes      int            `json:"archived_nodes"`
	NodesByType        map[string]int `json:"nodes_by_type"`
	TotalEdges         int            `json:"total_edges"`
	PendingEmbeddings  int            `json:"pending_embeddings"`
	EmbeddingModel     string         `json:"embedding_model"`
	EmbeddingDimensions string        `json:"embedding_dimensions"`
	SchemaVersion      string         `json:"schema_version"`
}
