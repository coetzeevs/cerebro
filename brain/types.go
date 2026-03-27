package brain

import "github.com/coetzeevs/cerebro/internal/store"

// Re-exported types for external consumers of the brain/ package.
// These types appear in brain.Brain's public API signatures but are defined
// in internal/store/, which cannot be imported by external Go modules.
type (
	Node          = store.Node
	Edge          = store.Edge
	ScoredNode    = store.ScoredNode
	NodeWithEdges = store.NodeWithEdges
	NodeType      = store.NodeType
	ListNodesOpts = store.ListNodesOpts
	Stats         = store.Stats
	GCResult      = store.GCResult

	// Export/Import types
	ExportBundle     = store.ExportBundle
	ImportOptions    = store.ImportOptions
	ImportResult     = store.ImportResult
	ConflictStrategy = store.ConflictStrategy
)

// Re-exported node type constants.
const (
	Episode    = store.TypeEpisode
	Concept    = store.TypeConcept
	Procedure  = store.TypeProcedure
	Reflection = store.TypeReflection
)

// Re-exported conflict strategy constants.
const (
	ConflictSkip    = store.ConflictSkip
	ConflictReplace = store.ConflictReplace
)

// Re-exported export format version.
const ExportVersion = store.ExportVersion
